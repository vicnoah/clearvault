package proxy

import (
	"clearvault/internal/crypto"
	"clearvault/internal/metadata"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"time"

	"golang.org/x/net/webdav"
)

type FileSystem struct {
	p *Proxy
}

func NewFileSystem(p *Proxy) *FileSystem {
	return &FileSystem{p: p}
}

func (fs *FileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	name = fs.p.normalizePath(name)
	log.Printf("FS Mkdir: '%s'", name)
	meta := &metadata.FileMeta{
		Path:       name,
		RemoteName: fs.p.generateRemoteName(), // Identity for virtual directory
		IsDir:      true,
		Size:       0,
		FEK:        []byte{}, // Satisfy NOT NULL
		Salt:       []byte{}, // Satisfy NOT NULL
		UpdatedAt:  time.Now(),
	}
	return fs.p.meta.Save(meta)
}

func (fs *FileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	name = fs.p.normalizePath(name)
	log.Printf("FS OpenFile: '%s' (flag: %d, perm: %v)", name, flag, perm)
	if name == "/" || name == "" || name == "." || name == "\\" {
		return &ProxyFile{
			fs:    fs,
			name:  "/",
			isDir: true,
		}, nil
	}

	if flag&os.O_CREATE != 0 {
		log.Printf("FS OpenFile: creating new file '%s'", name)
		return &ProxyFile{
			fs:    fs,
			name:  name,
			isNew: true,
		}, nil
	}

	meta, err := fs.p.GetFileMeta(name)
	if err != nil || meta == nil {
		log.Printf("FS OpenFile %s: not found", name)
		return nil, os.ErrNotExist
	}

	if meta.IsDir {
		return &ProxyFile{
			fs:    fs,
			name:  name,
			isDir: true,
		}, nil
	}

	return &ProxyFile{
		fs:   fs,
		name: name,
		meta: meta,
	}, nil
}

func (fs *FileSystem) RemoveAll(ctx context.Context, name string) error {
	return fs.p.RemoveAll(name)
}

func (fs *FileSystem) Rename(ctx context.Context, oldName, newName string) error {
	return fs.p.RenameFile(oldName, newName)
}

func (fs *FileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	// Normalize slashes
	name = fs.p.normalizePath(name)
	log.Printf("FS Stat: '%s'", name)
	if name == "/" || name == "" || name == "." || name == "\\" {
		return &FileInfo{name: "", isDir: true, modTime: time.Now()}, nil
	}

	meta, err := fs.p.GetFileMeta(name)
	if err != nil || meta == nil {
		log.Printf("FS Stat %s: not found", name)
		return nil, os.ErrNotExist
	}

	return &FileInfo{
		name:    path.Base(meta.Path),
		size:    meta.Size,
		isDir:   meta.IsDir,
		modTime: meta.UpdatedAt,
	}, nil
}

type ProxyFile struct {
	fs    *FileSystem
	name  string
	meta  *metadata.FileMeta
	isDir bool
	isNew bool

	// Upload fields
	pipeWriter  *io.PipeWriter
	uploadErr   chan error
	written     bool
	writtenSize int64

	// Download fields
	reader io.ReadCloser
	offset int64
}

func (f *ProxyFile) Close() error {
	if f.isNew {
		if !f.written {
			// No data written: save placeholder
			return f.fs.p.SavePlaceholder(f.name)
		}

		if f.pipeWriter != nil {
			log.Printf("FS Close: finishing upload for '%s'", f.name)
			f.pipeWriter.Close() // Close write end, reader gets EOF
			err := <-f.uploadErr // Wait for upload to finish
			return err
		}
		return nil
	}
	if f.reader != nil {
		err := f.reader.Close()
		f.reader = nil
		return err
	}
	return nil
}

func (f *ProxyFile) Read(p []byte) (n int, err error) {
	if f.isDir {
		return 0, os.ErrInvalid
	}
	if f.isNew {
		return 0, io.EOF
	}

	if f.reader == nil {
		// Calculate length needed to read rest of file (or until EOF)
		length := int64(0)
		if f.meta != nil {
			length = f.meta.Size - f.offset
		}

		if length <= 0 && f.meta.Size > 0 {
			return 0, io.EOF
		}

		f.reader, err = f.fs.p.DownloadRange(f.name, f.offset, length)
		if err != nil {
			return 0, err
		}

		// Discard bytes to align with f.offset
		startChunk := f.offset / crypto.ChunkSize
		chunkStart := int64(startChunk) * crypto.ChunkSize
		skip := f.offset - chunkStart

		if skip > 0 {
			// We must discard 'skip' bytes from the stream
			_, err = io.CopyN(io.Discard, f.reader, skip)
			if err != nil {
				f.reader.Close()
				f.reader = nil
				return 0, err
			}
		}
	}

	n, err = f.reader.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *ProxyFile) Seek(offset int64, whence int) (int64, error) {
	if f.isDir {
		return 0, os.ErrInvalid
	}

	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.offset + offset
	case io.SeekEnd:
		if f.meta == nil {
			return 0, fmt.Errorf("metadata missing")
		}
		newOffset = f.meta.Size + offset
	default:
		return 0, fmt.Errorf("invalid whence")
	}

	if newOffset < 0 {
		return 0, fmt.Errorf("negative offset")
	}

	if newOffset != f.offset {
		if f.reader != nil {
			f.reader.Close()
			f.reader = nil
		}
		f.offset = newOffset
	}

	return f.offset, nil
}

func (f *ProxyFile) Readdir(count int) ([]os.FileInfo, error) {
	if !f.isDir {
		return nil, os.ErrInvalid
	}

	dirPath := f.fs.p.normalizePath(f.name)
	log.Printf("FS Readdir: '%s'", dirPath)

	metas, err := f.fs.p.meta.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	infos := []os.FileInfo{}
	for _, m := range metas {
		infos = append(infos, &FileInfo{
			name:    path.Base(m.Path),
			size:    m.Size,
			isDir:   m.IsDir,
			modTime: m.UpdatedAt,
		})
	}

	if count > 0 && len(infos) > count {
		infos = infos[:count]
	}
	return infos, nil
}

func (f *ProxyFile) Stat() (os.FileInfo, error) {
	if f.isDir {
		return &FileInfo{name: path.Base(f.name), isDir: true, modTime: time.Now()}, nil
	}
	if f.isNew {
		return &FileInfo{
			name:    path.Base(f.name),
			size:    f.writtenSize,
			isDir:   false,
			modTime: time.Now(),
		}, nil
	}
	if f.meta != nil {
		return &FileInfo{
			name:    path.Base(f.meta.Path),
			size:    f.meta.Size,
			isDir:   false,
			modTime: f.meta.UpdatedAt,
		}, nil
	}
	return f.fs.Stat(context.Background(), f.name)
}

func (f *ProxyFile) Write(p []byte) (n int, err error) {
	if f.isDir {
		return 0, os.ErrInvalid
	}
	if f.isNew {
		if !f.written {
			pr, pw := io.Pipe()
			f.pipeWriter = pw
			f.uploadErr = make(chan error, 1)
			f.written = true

			go func() {
				// UploadFile closes pr when done reading
				err := f.fs.p.UploadFile(f.name, pr)
				f.uploadErr <- err
			}()
		}

		n, err = f.pipeWriter.Write(p)
		f.writtenSize += int64(n)
		f.offset += int64(n)
		return n, err
	}
	log.Printf("FS Write %s: permission denied (not a new file or O_CREATE not set)", f.name)
	return 0, os.ErrPermission
}

type FileInfo struct {
	name    string
	size    int64
	isDir   bool
	modTime time.Time
}

func (fi *FileInfo) Name() string { return fi.name }
func (fi *FileInfo) Size() int64  { return fi.size }
func (fi *FileInfo) Mode() os.FileMode {
	if fi.isDir {
		return os.ModeDir | 0755
	}
	return 0644
}
func (fi *FileInfo) ModTime() time.Time { return fi.modTime }
func (fi *FileInfo) IsDir() bool        { return fi.isDir }
func (fi *FileInfo) Sys() interface{}   { return nil }
