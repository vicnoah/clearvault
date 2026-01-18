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
	"sync"
	"time"
)

type Proxy struct {
	meta         metadata.Storage
	remote       *webdav.RemoteClient
	masterKey    []byte
	pendingSizes sync.Map // path -> int64
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

func (p *Proxy) SetPendingSize(path string, size int64) {
	path = p.normalizePath(path)
	p.pendingSizes.Store(path, size)
}

func (p *Proxy) GetPendingSize(path string) int64 {
	path = p.normalizePath(path)
	if v, ok := p.pendingSizes.Load(path); ok {
		return v.(int64)
	}
	return 0
}

func (p *Proxy) ClearPendingSize(path string) {
	path = p.normalizePath(path)
	p.pendingSizes.Delete(path)
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

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	m, err := c.r.Read(p)
	c.n += int64(m)
	return m, err
}

func (p *Proxy) SavePlaceholder(pname string) error {
	pname = p.normalizePath(pname)
	log.Printf("Proxy: Saving temporary placeholder for 0-byte file '%s'", pname)

	// Even for a placeholder, we should generate keys so that we can support valid metadata
	// In case we want to support "reading" an empty file later (which returns 0 bytes anyway)
	fek, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		return err
	}
	salt, err := crypto.GenerateRandomBytes(12)
	if err != nil {
		return err
	}

	encryptedFEK, err := p.encryptFEK(fek)
	if err != nil {
		return err
	}

	meta := &metadata.FileMeta{
		Path:       pname,
		RemoteName: ".pending",
		Size:       0,
		IsDir:      false,
		FEK:        encryptedFEK,
		Salt:       salt,
		UpdatedAt:  time.Now(),
	}
	// Check if file already exists to update it instead of failing (though Save usually upserts)
	// But more importantly, we need to make sure we don't overwrite a REAL file with a placeholder if it already exists?
	// RaiDrive might do PUT 0-byte on existing file?
	// If it exists and is NOT .pending, we probably shouldn't overwrite it with .pending unless we are sure.
	// But standard WebDAV PUT overwrites. So yes, we should overwrite.

	return p.meta.Save(meta)
}

func (p *Proxy) UploadFile(pname string, r io.Reader, size int64) error {
	pname = p.normalizePath(pname)
	log.Printf("Proxy: UploadFile (Streaming) starting for '%s' (size: %d)", pname, size)

	// Check if there's a temporary placeholder from previous 0-byte upload
	existingMeta, _ := p.meta.Get(pname)
	if existingMeta != nil && existingMeta.RemoteName == ".pending" {
		log.Printf("Proxy: Replacing temporary placeholder with real file for '%s'", pname)
	}

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

	// Create pipe for streaming encryption
	pr, pw := io.Pipe()

	engine, err := crypto.NewEngine(fek)
	if err != nil {
		return err
	}

	cr := &countReader{r: r}
	errChan := make(chan error, 1)

	go func() {
		err := engine.EncryptStream(cr, pw, salt)
		pw.CloseWithError(err)
		errChan <- err
	}()

	var encSize int64
	if size > 0 {
		encSize = crypto.CalculateEncryptedSize(size)

		err = p.remote.Upload(remoteName, pr, encSize)
		if err != nil {
			log.Printf("Proxy: Remote Upload failed for '%s': %v", pname, err)
			return err
		}
	}

	// Check encryption error
	if encErr := <-errChan; encErr != nil {
		return fmt.Errorf("encryption failed: %w", encErr)
	}

	// Update metadata with actual size
	// Note: We need to handle the case where a .pending placeholder was created.
	// UploadFile logic is separate from SavePlaceholder.
	// If this UploadFile was called, it means we have data (or it's a 0-byte file that logic decided to upload real empty file? No, 0-byte goes to SavePlaceholder in FS).
	// Wait, FS Close calls UploadFile ONLY if written is true (so size > 0) OR if it logic changes.
	// In FS Close: if !f.written -> SavePlaceholder.
	// So UploadFile here always has data > 0?
	// Not necessarily, if someone calls UploadFile directly. But via FS, yes.

	meta := &metadata.FileMeta{
		Path:       pname,
		RemoteName: remoteName,
		Size:       cr.n,
		IsDir:      false,
		FEK:        encryptedFEK,
		Salt:       salt,
		UpdatedAt:  time.Now(),
	}
	err = p.meta.Save(meta)
	log.Printf("Proxy: UploadFile finished for '%s' (size: %d, err: %v)", pname, cr.n, err)
	return err
}

func (p *Proxy) ExportLocal(inputPath, outputDir string) error {
	absInput, err := filepath.Abs(inputPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(absInput)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	root := absInput
	if !info.IsDir() {
		root = filepath.Dir(absInput)
	}
	log.Printf("Proxy: ExportLocal from '%s' (root '%s') to '%s'", absInput, root, outputDir)
	err = filepath.Walk(absInput, func(current string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		metaPath := "/" + filepath.ToSlash(rel)
		metaPath = p.normalizePath(metaPath)
		if fi.IsDir() {
			meta := &metadata.FileMeta{
				Path:       metaPath,
				RemoteName: p.generateRemoteName(),
				IsDir:      true,
				Size:       0,
				FEK:        []byte{},
				Salt:       []byte{},
				UpdatedAt:  fi.ModTime(),
			}
			return p.meta.Save(meta)
		}
		fek, err := crypto.GenerateRandomBytes(32)
		if err != nil {
			return err
		}
		salt, err := crypto.GenerateRandomBytes(12)
		if err != nil {
			return err
		}
		encryptedFEK, err := p.encryptFEK(fek)
		if err != nil {
			return err
		}
		engine, err := crypto.NewEngine(fek)
		if err != nil {
			return err
		}
		remoteName := p.generateRemoteName()
		outPath := filepath.Join(outputDir, remoteName)
		inFile, err := os.Open(current)
		if err != nil {
			return err
		}
		outFile, err := os.Create(outPath)
		if err != nil {
			inFile.Close()
			return err
		}
		err = engine.EncryptStream(inFile, outFile, salt)
		closeErr := outFile.Close()
		inFile.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		meta := &metadata.FileMeta{
			Path:       metaPath,
			RemoteName: remoteName,
			Size:       fi.Size(),
			IsDir:      false,
			FEK:        encryptedFEK,
			Salt:       salt,
			UpdatedAt:  fi.ModTime(),
		}
		return p.meta.Save(meta)
	})
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

	// If this is a temporary placeholder (0-byte upload), return empty reader
	if meta.RemoteName == ".pending" {
		log.Printf("Proxy: Returning empty content for temporary placeholder '%s'", pname)
		return io.NopCloser(bytes.NewReader([]byte{})), nil
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

func (p *Proxy) DownloadRange(pname string, offset, length int64) (io.ReadCloser, error) {
	pname = p.normalizePath(pname)
	meta, err := p.meta.Get(pname)
	if err != nil || meta == nil {
		return nil, fmt.Errorf("file not found: %s", pname)
	}

	if meta.RemoteName == ".pending" {
		return io.NopCloser(bytes.NewReader([]byte{})), nil
	}

	if meta.Size == 0 || offset >= meta.Size {
		return io.NopCloser(bytes.NewReader([]byte{})), nil
	}

	fek, err := p.decryptFEK(meta.FEK)
	if err != nil {
		return nil, err
	}

	engine, err := crypto.NewEngine(fek)
	if err != nil {
		return nil, err
	}

	// Calculate chunks
	startChunk := uint64(offset / crypto.ChunkSize)
	endChunk := uint64((offset + length - 1) / crypto.ChunkSize)

	// Cap endChunk to total chunks
	totalChunks := uint64((meta.Size + crypto.ChunkSize - 1) / crypto.ChunkSize)
	if endChunk >= totalChunks {
		if totalChunks == 0 {
			return io.NopCloser(bytes.NewReader([]byte{})), nil
		}
		endChunk = totalChunks - 1
	}

	encStart := int64(startChunk) * crypto.CipherChunkSize
	encLength := int64(endChunk-startChunk+1) * crypto.CipherChunkSize

	cipherRC, err := p.remote.DownloadRange(meta.RemoteName, encStart, encLength)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		err := engine.DecryptStreamFrom(cipherRC, pw, meta.Salt, startChunk)
		cipherRC.Close()
		pw.CloseWithError(err)
	}()

	return pr, nil
}
