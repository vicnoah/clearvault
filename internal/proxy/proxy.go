package proxy

import (
	"bytes"
	"clearvault/internal/crypto"
	"clearvault/internal/metadata"
	"clearvault/internal/webdav"
	sysrand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"time"
)

type Proxy struct {
	meta      metadata.Storage
	remote    *webdav.RemoteClient
	masterKey []byte
}

func NewProxy(meta metadata.Storage, remote *webdav.RemoteClient, masterKeyBase64 string) (*Proxy, error) {
	key, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key: %w", err)
	}
	return &Proxy{
		meta:      meta,
		remote:    remote,
		masterKey: key,
	}, nil
}

// encryptFEK encrypts the File Encryption Key with the Master Key.
func (p *Proxy) encryptFEK(fek []byte) ([]byte, error) {
	block, err := crypto.NewEngine(p.masterKey)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	// Use a fixed zero nonce for FEK encryption as FEK is random itself
	nonce := make([]byte, 12)
	err = block.EncryptStream(bytes.NewReader(fek), buf, nonce)
	return buf.Bytes(), err
}

// decryptFEK decrypts the File Encryption Key with the Master Key.
func (p *Proxy) decryptFEK(encryptedFEK []byte) ([]byte, error) {
	block, err := crypto.NewEngine(p.masterKey)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	nonce := make([]byte, 12)
	err = block.DecryptStream(bytes.NewReader(encryptedFEK), buf, nonce)
	return buf.Bytes(), err
}

func (p *Proxy) generateRemoteName() string {
	b := make([]byte, 32)
	// We use crypto/rand directly since we aliased internal crypto.
	// Wait, I didn't alias it in the previous chunk. I'll just use "crypto/rand" by its default name "rand".
	// But "crypto" is already used for internal crypto.
	// I'll use sysrand.
	if _, err := io.ReadFull(sysrand.Reader, b); err != nil {
		hash := sha256.Sum256([]byte(time.Now().String()))
		return hex.EncodeToString(hash[:])
	}
	return hex.EncodeToString(b)
}

func (p *Proxy) GetFileMeta(path string) (*metadata.FileMeta, error) {
	return p.meta.Get(path)
}

func (p *Proxy) normalizePath(pname string) string {
	cleaned := path.Clean("/" + filepath.ToSlash(pname))
	return cleaned
}

func (p *Proxy) UploadFile(pname string, r io.Reader) error {
	pname = p.normalizePath(pname)
	log.Printf("Proxy: UploadFile starting for '%s'", pname)
	fek, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		return err
	}
	salt, err := crypto.GenerateRandomBytes(12) // Use as base nonce
	if err != nil {
		return err
	}

	encryptedFEK, err := p.encryptFEK(fek)
	if err != nil {
		return err
	}

	remoteName := p.generateRemoteName()
	log.Printf("Proxy: Uploading to remote as '%s'", remoteName)

	// Track size
	var cleartextSize int64
	sizeReader := io.TeeReader(r, &sizeWriter{size: &cleartextSize})

	// Create pipe for streaming encryption
	pr, pw := io.Pipe()
	engine, err := crypto.NewEngine(fek)
	if err != nil {
		return err
	}

	go func() {
		err := engine.EncryptStream(sizeReader, pw, salt)
		pw.CloseWithError(err)
	}()

	err = p.remote.Upload(remoteName, pr)
	if err != nil {
		log.Printf("Proxy: Remote Upload failed for '%s': %v", pname, err)
		return err
	}

	meta := &metadata.FileMeta{
		Path:       pname,
		RemoteName: remoteName,
		Size:       cleartextSize,
		IsDir:      false,
		FEK:        encryptedFEK,
		Salt:       salt,
		UpdatedAt:  time.Now(),
	}
	err = p.meta.Save(meta)
	log.Printf("Proxy: UploadFile finished for '%s' (size: %d, err: %v)", pname, cleartextSize, err)
	return err
}

type sizeWriter struct {
	size *int64
}

func (sw *sizeWriter) Write(p []byte) (n int, err error) {
	*sw.size += int64(len(p))
	return len(p), nil
}

func (p *Proxy) RemoveAll(pname string) error {
	pname = p.normalizePath(pname)
	log.Printf("Proxy: RemoveAll '%s'", pname)

	meta, err := p.meta.Get(pname)
	if err != nil {
		return err
	}
	if meta == nil {
		return os.ErrNotExist
	}

	if !meta.IsDir {
		log.Printf("Proxy: Deleting single remote file '%s'", meta.RemoteName)
		err = p.remote.Delete(meta.RemoteName)
		if err != nil {
			log.Printf("Proxy: Warning: Failed to delete remote file %s: %v", meta.RemoteName, err)
		}
	} else {
		// Recursive delete for directory.
		// We need to delete ALL remote files belonging to this tree.
		// This is still a bit tricky because LocalStorage/SqliteStorage don't know about remote names across the tree efficiently.
		// Wait, ReadDir only gives immediate children.
		// For RemoveAll to be efficient, the Storage should maybe return all remote names in a tree?
		// Or we just do a recursive walk here.
		p.recursiveRemoteDelete(pname)
	}

	return p.meta.RemoveAll(pname)
}

func (p *Proxy) recursiveRemoteDelete(pname string) {
	children, err := p.meta.ReadDir(pname)
	if err != nil {
		return
	}
	for _, child := range children {
		if child.IsDir {
			p.recursiveRemoteDelete(child.Path)
		} else {
			log.Printf("Proxy: Deleting remote file '%s' for '%s'", child.RemoteName, child.Path)
			p.remote.Delete(child.RemoteName)
		}
	}
}

func (p *Proxy) RenameFile(oldPath, newPath string) error {
	oldPath = p.normalizePath(oldPath)
	newPath = p.normalizePath(newPath)

	log.Printf("Proxy: RenameFile from '%s' to '%s' (metadata layer)", oldPath, newPath)
	return p.meta.Rename(oldPath, newPath)
}

func (p *Proxy) DownloadFile(pname string) (io.ReadCloser, error) {
	pname = p.normalizePath(pname)
	meta, err := p.meta.Get(pname)
	if err != nil || meta == nil {
		return nil, fmt.Errorf("file not found: %s", pname)
	}

	fek, err := p.decryptFEK(meta.FEK)
	if err != nil {
		return nil, err
	}

	engine, err := crypto.NewEngine(fek)
	if err != nil {
		return nil, err
	}

	cipherRC, err := p.remote.Download(meta.RemoteName)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		err := engine.DecryptStream(cipherRC, pw, meta.Salt)
		cipherRC.Close()
		pw.CloseWithError(err)
	}()

	return pr, nil
}
