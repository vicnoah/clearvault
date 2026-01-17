package proxy

import (
	"bytes"
	"clearvault/internal/metadata"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
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
		RemoteName: fs.p.generateRemoteName(name), // Identity for virtual directory
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
			fs:     fs,
			name:   name,
			isNew:  true,
			buffer: &bytes.Buffer{},
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
	name = fs.p.normalizePath(name)
	meta, err := fs.p.GetFileMeta(name)
	if err != nil || meta == nil {
		return os.ErrNotExist
	}

	if !meta.IsDir {
		err = fs.p.remote.Delete(meta.RemoteName)
		if err != nil {
			return err
		}
	}
	return fs.p.meta.Delete(name)
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
	fs     *FileSystem
	name   string
	meta   *metadata.FileMeta
	isDir  bool
	isNew  bool
	buffer *bytes.Buffer
	reader io.ReadCloser
	offset int64
}

func (f *ProxyFile) Close() error {
	if f.isNew && f.buffer != nil {
		log.Printf("FS Close: uploading new file '%s' (buffer size: %d)", f.name, f.buffer.Len())
		err := f.fs.p.UploadFile(f.name, f.buffer)
		if err != nil {
			log.Printf("FS Close: UploadFile failed for '%s': %v", f.name, err)
		}
		f.buffer = nil
		return err
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
		f.reader, err = f.fs.p.DownloadFile(f.name)
		if err != nil {
			return 0, err
		}
		// If offset > 0, we should skip bytes or use DownloadRange
		// For now, let's just skip if offset is small, but ideally we use DownloadRange
		if f.offset > 0 {
			// This is inefficient, but okay for an initial implementation.
			// Better: Close and re-open with DownloadRange in Seek.
			io.CopyN(io.Discard, f.reader, f.offset)
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

	metas, err := f.fs.p.meta.ListByPrefix(dirPath)
	if err != nil {
		return nil, err
	}

	infos := []os.FileInfo{}
	for _, m := range metas {
		// Standardize path from DB
		mPath := f.fs.p.normalizePath(m.Path)
		if mPath == dirPath {
			continue
		}

		rel := strings.TrimPrefix(mPath, dirPath)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}

		// Handle immediate children only
		parent, child := path.Split(rel)
		if parent != "" && parent != "." && parent != "/" {
			continue
		}

		infos = append(infos, &FileInfo{
			name:    child,
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
			size:    int64(f.buffer.Len()),
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
		n, err = f.buffer.Write(p)
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
