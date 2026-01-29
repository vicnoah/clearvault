//go:build fuse

package fuse

import (
	"bytes"
	"io"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"clearvault/internal/proxy"

	"github.com/winfsp/cgofuse/fuse"
)

type writeHandle struct {
	path       string
	uploadPath string
	renameTo   string
	started    bool
	expected   int64
	pw         *io.PipeWriter
	uploadCh   chan error

	lastWriteLogAt    time.Time
	lastWriteLogBytes int64
}

type ClearVaultFS struct {
	fuse.FileSystemBase
	proxy *proxy.Proxy
	uid   uint32
	gid   uint32

	mu      sync.Mutex
	nextFH  uint64
	writers map[uint64]*writeHandle
}

func NewClearVaultFS(p *proxy.Proxy) *ClearVaultFS {
	var uid uint32
	var gid uint32
	if v := os.Getenv("FUSE_UID"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			uid = uint32(n)
		}
	}
	if v := os.Getenv("FUSE_GID"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			gid = uint32(n)
		}
	}
	return &ClearVaultFS{
		proxy:   p,
		uid:     uid,
		gid:     gid,
		nextFH:  1,
		writers: make(map[uint64]*writeHandle),
	}
}

func decodeOpenFlags(flags int) string {
	parts := make([]string, 0, 8)
	switch flags & (fuse.O_WRONLY | fuse.O_RDWR) {
	case fuse.O_WRONLY:
		parts = append(parts, "WRONLY")
	case fuse.O_RDWR:
		parts = append(parts, "RDWR")
	default:
		parts = append(parts, "RDONLY")
	}

	if flags&fuse.O_CREAT != 0 {
		parts = append(parts, "CREAT")
	}
	if flags&fuse.O_EXCL != 0 {
		parts = append(parts, "EXCL")
	}
	if flags&fuse.O_TRUNC != 0 {
		parts = append(parts, "TRUNC")
	}
	if flags&fuse.O_APPEND != 0 {
		parts = append(parts, "APPEND")
	}
	return strings.Join(parts, "|")
}

// Getattr gets file attributes
func (fs *ClearVaultFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	if path == "/" {
		tmsp := fuse.Now()
		stat.Mode = fuse.S_IFDIR | 0775
		stat.Uid = fs.uid
		stat.Gid = fs.gid
		stat.Atim = tmsp
		stat.Mtim = tmsp
		stat.Ctim = tmsp
		stat.Birthtim = tmsp
		return 0
	}

	meta, err := fs.proxy.GetFileMeta(path)
	if err != nil {
		return -fuse.ENOENT
	}
	if meta == nil {
		if size, ok := fs.pendingSize(path); ok {
			tmsp := fuse.Now()
			stat.Mode = fuse.S_IFREG | 0664
			stat.Size = size
			stat.Uid = fs.uid
			stat.Gid = fs.gid
			stat.Atim = tmsp
			stat.Mtim = tmsp
			stat.Ctim = tmsp
			stat.Birthtim = tmsp
			return 0
		}
		if fs.proxy.HasPlaceholder(path) {
			tmsp := fuse.Now()
			stat.Mode = fuse.S_IFREG | 0664
			stat.Size = 0
			stat.Uid = fs.uid
			stat.Gid = fs.gid
			stat.Atim = tmsp
			stat.Mtim = tmsp
			stat.Ctim = tmsp
			stat.Birthtim = tmsp
			log.Printf("FUSE Getattr placeholder path=%q", path)
			return 0
		}
		log.Printf("FUSE Getattr ENOENT path=%q", path)
		return -fuse.ENOENT
	}

	if meta.IsDir {
		stat.Mode = fuse.S_IFDIR | 0775
	} else {
		stat.Mode = fuse.S_IFREG | 0664
		stat.Size = meta.Size
	}
	stat.Uid = fs.uid
	stat.Gid = fs.gid

	tmsp := fuse.Timespec{Sec: meta.UpdatedAt.Unix()}
	stat.Mtim = tmsp
	stat.Ctim = tmsp
	stat.Atim = tmsp
	stat.Birthtim = tmsp
	return 0
}

// Readdir reads directory
func (fs *ClearVaultFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
	// Fill . and ..
	fill(".", nil, 0)
	fill("..", nil, 0)

	// Since proxy.ReadDir is not directly exposed in the same way, we access metadata storage directly or via proxy if exposed.
	// But Proxy struct has unexported `meta`.
	// We need to add ReadDir to Proxy or expose `meta`.
	// Looking at proxy.go, there is no ReadDir.
	// But in `RemoveAll` it calls `p.meta.ReadDir`.
	// We should probably add ReadDir to Proxy interface or just use `p.meta` if we can.
	// Wait, `proxy.GetFileMeta` is exposed.
	// Let's check `internal/proxy/proxy.go`... `meta` is unexported.
	// I should probably add `ReadDir` to `Proxy` struct first.
	// For now, let's assume I will add it.

	children, err := fs.proxy.ReadDir(path)
	if err != nil {
		return -fuse.ENOENT
	}

	for _, child := range children {
		fill(child.Name, nil, 0)
	}
	return 0
}

// Open opens file
func (fs *ClearVaultFS) Open(path string, flags int) (int, uint64) {
	meta, err := fs.proxy.GetFileMeta(path)
	if err != nil {
		log.Printf("FUSE Open error path=%q flags=0x%x(%s) err=%v", path, flags, decodeOpenFlags(flags), err)
		return -fuse.ENOENT, 0
	}
	placeholder := false
	if meta == nil {
		if _, ok := fs.pendingSize(path); ok {
			log.Printf("FUSE Open pending path=%q flags=0x%x(%s)", path, flags, decodeOpenFlags(flags))
			return 0, 0
		}
		placeholder = fs.proxy.HasPlaceholder(path)
		if !placeholder {
			log.Printf("FUSE Open ENOENT path=%q flags=0x%x(%s)", path, flags, decodeOpenFlags(flags))
			return -fuse.ENOENT, 0
		}
	}
	if flags&(fuse.O_WRONLY|fuse.O_RDWR) != 0 {
		if meta != nil && meta.Size > 0 && flags&fuse.O_TRUNC == 0 {
			log.Printf("FUSE Open write not supported without TRUNC path=%q flags=0x%x(%s) size=%d", path, flags, decodeOpenFlags(flags), meta.Size)
			return -fuse.EOPNOTSUPP, 0
		}
		log.Printf("FUSE Open write path=%q flags=0x%x(%s) placeholder=%v", path, flags, decodeOpenFlags(flags), placeholder)
		_ = fs.proxy.RemoveAll(path)
		fh := fs.newWriteHandle(path)
		return 0, fh
	}
	if placeholder {
		log.Printf("FUSE Open read placeholder path=%q flags=0x%x(%s)", path, flags, decodeOpenFlags(flags))
		return 0, 0
	}
	return 0, 0
}

// Read reads data
func (fs *ClearVaultFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	rc, err := fs.proxy.DownloadRange(path, ofst, int64(len(buff)))
	if err != nil {
		log.Printf("FUSE Read error: path=%q offset=%d len=%d err=%v", path, ofst, len(buff), err)
		return -fuse.EIO
	}
	defer rc.Close()

	// 使用普通 Read 而不是 ReadFull，允许部分读取
	// 这样可以更好地处理网络/存储层的临时问题
	n, err := rc.Read(buff)
	if err != nil && err != io.EOF {
		// 检查是否是临时错误（如网络超时）
		if isTemporaryError(err) {
			log.Printf("FUSE Read temporary error: path=%q offset=%d n=%d err=%v", path, ofst, n, err)
			// 如果已经读取了一些数据，返回已读取的字节数
			if n > 0 {
				return n
			}
			return -fuse.EAGAIN
		}
		log.Printf("FUSE Read error: path=%q offset=%d n=%d err=%v", path, ofst, n, err)
		return -fuse.EIO
	}

	// 成功读取或到达文件末尾
	return n
}

// isTemporaryError 检查错误是否是临时性的（可以重试）
func isTemporaryError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// 常见的临时错误模式
	temporaryPatterns := []string{
		"timeout",
		"temporary",
		"retry",
		"connection reset",
		"broken pipe",
		"i/o timeout",
	}
	for _, pattern := range temporaryPatterns {
		if containsIgnoreCase(errStr, pattern) {
			return true
		}
	}
	return false
}

// containsIgnoreCase 检查字符串是否包含子串（忽略大小写）
func containsIgnoreCase(s, substr string) bool {
	return len(substr) <= len(s) && 
		(findSubstr(s, substr) >= 0 || findSubstr(toLower(s), toLower(substr)) >= 0)
}

// findSubstr 查找子串位置
func findSubstr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// toLower 将字符串转换为小写
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		result[i] = c
	}
	return string(result)
}

// Create creates a file
func (fs *ClearVaultFS) Create(path string, flags int, mode uint32) (int, uint64) {
	log.Printf("FUSE Create path=%q flags=0x%x(%s) mode=0%o", path, flags, decodeOpenFlags(flags), mode)
	_ = fs.proxy.RemoveAll(path)
	fh := fs.newWriteHandle(path)
	log.Printf("FUSE Create ok path=%q fh=%d", path, fh)
	return 0, fh
}

// Truncate changes file size
func (fs *ClearVaultFS) Truncate(path string, size int64, fh uint64) int {
	if size == 0 {
		if fh != 0 {
			return -fuse.EOPNOTSUPP
		}
		err := fs.proxy.UploadFile(path, bytes.NewReader(nil), 0)
		if err != nil {
			return -fuse.EIO
		}
		return 0
	}
	return -fuse.EOPNOTSUPP
}

// Write writes data
// Note: This is tricky for object storage because we can't random write.
// We usually need to buffer the whole file and upload on Close/Release.
// Or we only support sequential write (which many copiers do).
// For simplicity in this first version, let's implement a simple buffer or fail on non-sequential.
// Actually, `cgofuse` might handle some caching?
// Standard approach for object storage FUSE:
// 1. On Open(WRITE), create a temp local file.
// 2. Write to local file.
// 3. On Release/Flush, upload local file to remote.
func (fs *ClearVaultFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	h := fs.getWriteHandle(fh)
	if h == nil {
		log.Printf("FUSE Write EBADF path=%q fh=%d ofst=%d len=%d", path, fh, ofst, len(buff))
		return -fuse.EBADF
	}
	if ofst != h.expected {
		log.Printf("FUSE Write EOPNOTSUPP non-seq path=%q fh=%d ofst=%d expected=%d len=%d", path, fh, ofst, h.expected, len(buff))
		return -fuse.EOPNOTSUPP
	}
	if !h.started {
		pr, pw := io.Pipe()
		h.pw = pw
		h.uploadCh = make(chan error, 1)
		h.started = true
		h.uploadPath = h.path
		log.Printf("FUSE Write start upload path=%q fh=%d", h.uploadPath, fh)
		go func(p string) {
			err := fs.proxy.UploadFile(p, pr, -1)
			h.uploadCh <- err
		}(h.uploadPath)
	}
	n, err := h.pw.Write(buff)
	h.expected += int64(n)
	if err != nil {
		log.Printf("FUSE Write EIO path=%q fh=%d err=%v", path, fh, err)
		return -fuse.EIO
	}
	now := time.Now()
	shouldLog := false
	if h.lastWriteLogAt.IsZero() {
		shouldLog = true
	} else if h.expected-h.lastWriteLogBytes >= 1<<20 {
		shouldLog = true
	} else if now.Sub(h.lastWriteLogAt) >= 2*time.Second {
		shouldLog = true
	}
	if shouldLog {
		log.Printf("FUSE Write ok path=%q fh=%d ofst=%d n=%d total=%d", path, fh, ofst, n, h.expected)
		h.lastWriteLogAt = now
		h.lastWriteLogBytes = h.expected
	}
	return n
}

func (fs *ClearVaultFS) Flush(path string, fh uint64) int {
	return 0
}

func (fs *ClearVaultFS) Release(path string, fh uint64) int {
	h := fs.removeWriteHandle(fh)
	if h == nil {
		return 0
	}
	log.Printf("FUSE Release path=%q fh=%d started=%v size=%d renameTo=%q uploadPath=%q", h.path, fh, h.started, h.expected, h.renameTo, h.uploadPath)
	if !h.started {
		placeholderPath := h.path
		if h.renameTo != "" {
			placeholderPath = h.renameTo
		}
		if h.expected == 0 && isPlaceholderCandidate(h.path) {
			log.Printf("FUSE Release placeholder path=%q fh=%d", placeholderPath, fh)
			if err := fs.proxy.SavePlaceholder(placeholderPath); err != nil {
				return -fuse.EIO
			}
			return 0
		}
		log.Printf("FUSE Release upload empty path=%q fh=%d", h.path, fh)
		if err := fs.proxy.UploadFile(h.path, bytes.NewReader(nil), 0); err != nil {
			return -fuse.EIO
		}
		return 0
	}
	_ = h.pw.Close()
	err := <-h.uploadCh
	if err != nil {
		log.Printf("FUSE Release upload failed path=%q fh=%d err=%v", h.path, fh, err)
		return -fuse.EIO
	}
	if h.renameTo != "" && h.renameTo != h.uploadPath {
		log.Printf("FUSE Release apply deferred rename from=%q to=%q fh=%d", h.uploadPath, h.renameTo, fh)
		if err := fs.proxy.RenameFile(h.uploadPath, h.renameTo); err != nil {
			log.Printf("FUSE Release deferred rename failed from=%q to=%q fh=%d err=%v", h.uploadPath, h.renameTo, fh, err)
			return -fuse.EIO
		}
		log.Printf("FUSE Release deferred rename ok from=%q to=%q fh=%d", h.uploadPath, h.renameTo, fh)
	}
	log.Printf("FUSE Release upload ok path=%q fh=%d", h.path, fh)
	return 0
}

func (fs *ClearVaultFS) Fsync(path string, datasync bool, fh uint64) int {
	return 0
}

func (fs *ClearVaultFS) Fsyncdir(path string, datasync bool, fh uint64) int {
	return 0
}

// Mkdir creates directory
func (fs *ClearVaultFS) Mkdir(path string, mode uint32) int {
	// Create directory metadata
	// Proxy doesn't expose `Mkdir` directly but we can create metadata manually if we access `meta`.
	// I should extend Proxy to support Mkdir.
	err := fs.proxy.Mkdir(path)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

// Unlink deletes file
func (fs *ClearVaultFS) Unlink(path string) int {
	err := fs.proxy.RemoveAll(path)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

// Rmdir deletes directory
func (fs *ClearVaultFS) Rmdir(path string) int {
	return fs.Unlink(path)
}

// Rename renames file
func (fs *ClearVaultFS) Rename(oldpath string, newpath string) int {
	if fs.deferRename(oldpath, newpath) {
		return 0
	}
	err := fs.proxy.RenameFile(oldpath, newpath)
	if err != nil {
		log.Printf("FUSE Rename EIO old=%q new=%q err=%v", oldpath, newpath, err)
		return -fuse.EIO
	}
	return 0
}

func (fs *ClearVaultFS) Statfs(path string, stat *fuse.Statfs_t) int {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Blocks = 1024 * 1024
	stat.Bfree = 1024 * 1024
	stat.Bavail = 1024 * 1024
	stat.Files = 1024 * 1024
	stat.Ffree = 1024 * 1024
	return 0
}

func (fs *ClearVaultFS) Access(path string, mask uint32) int {
	if path == "/" {
		return 0
	}
	if _, ok := fs.pendingSize(path); ok {
		return 0
	}
	meta, err := fs.proxy.GetFileMeta(path)
	if err != nil || meta == nil {
		if fs.proxy.HasPlaceholder(path) {
			log.Printf("FUSE Access placeholder path=%q mask=0x%x", path, mask)
			return 0
		}
		return -fuse.ENOENT
	}
	return 0
}

func (fs *ClearVaultFS) Opendir(path string) (int, uint64) {
	return 0, 0
}

func (fs *ClearVaultFS) Releasedir(path string, fh uint64) int {
	return 0
}

func (fs *ClearVaultFS) newWriteHandle(path string) uint64 {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fh := fs.nextFH
	fs.nextFH++
	fs.writers[fh] = &writeHandle{path: path, expected: 0}
	return fh
}

func (fs *ClearVaultFS) getWriteHandle(fh uint64) *writeHandle {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.writers[fh]
}

func (fs *ClearVaultFS) removeWriteHandle(fh uint64) *writeHandle {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	h := fs.writers[fh]
	delete(fs.writers, fh)
	return h
}

func (fs *ClearVaultFS) pendingSize(path string) (int64, bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, h := range fs.writers {
		if h.path == path || h.renameTo == path {
			return h.expected, true
		}
	}
	return 0, false
}

func (fs *ClearVaultFS) deferRename(oldpath, newpath string) bool {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, h := range fs.writers {
		if h.path == oldpath || h.uploadPath == oldpath {
			h.renameTo = newpath
			log.Printf("FUSE Rename deferred old=%q new=%q", oldpath, newpath)
			return true
		}
	}
	return false
}

func isPlaceholderCandidate(p string) bool {
	name := path.Base(p)
	return strings.Contains(name, ".~#")
}
