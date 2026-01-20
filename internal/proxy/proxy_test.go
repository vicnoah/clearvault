package proxy

import (
	"bytes"
	"clearvault/internal/metadata"
	"clearvault/internal/remote"
	"clearvault/internal/webdav"
	"io"
	"os"
	"testing"
	"time"
)

func TestSimpleProxy(t *testing.T) {
	metaDir := "./test_simple_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	// Use a mock or just the real client if possible, but let's see if NewProxy fails
	// Master key: 32 bytes base64 encoded
	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	remoteClient, _ := webdav.NewClient(webdav.WebDAVConfig{
		URL:  "https://example.com",
		User: "user",
		Pass: "pass",
	})
	var remoteStorage remote.RemoteStorage = remoteClient

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	if p == nil {
		t.Fatal("Proxy is nil")
	}
}

func TestZeroByteFileUpload(t *testing.T) {
	metaDir := "./test_zerobyte_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	// Use a mock remote client that doesn't actually make network calls
	// Master key: 32 bytes base64 encoded
	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="

	// Create a mock remote storage that accepts all uploads
	mockRemote := &mockRemoteStorage{}
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// 1. Upload 0-byte file (RaiDrive Phase 1)
	err = p.UploadFile("/test.txt", bytes.NewReader([]byte{}), 0)
	if err != nil {
		t.Fatalf("UploadFile (0-byte) failed: %v", err)
	}

	// 2. Verify metadata
	m, err := meta.Get("/test.txt")
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}
	if m.RemoteName == "" || m.RemoteName == ".pending" {
		t.Errorf("Expected RemoteName to be set, got '%s'", m.RemoteName)
	}
	if m.Size != 0 {
		t.Errorf("Expected Size to be 0, got %d", m.Size)
	}

	// 3. Verify Download returns empty content
	rc, err := p.DownloadFile("/test.txt")
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Failed to read downloaded content: %v", err)
	}
	if len(content) != 0 {
		t.Errorf("Expected 0 bytes content, got %d", len(content))
	}
}

// mockRemoteStorage is a mock implementation of RemoteStorage for testing
type mockRemoteStorage struct {
	files map[string][]byte
}

func newMockRemoteStorage() *mockRemoteStorage {
	return &mockRemoteStorage{
		files: make(map[string][]byte),
	}
}

func (m *mockRemoteStorage) ensureMap() {
	if m.files == nil {
		m.files = make(map[string][]byte)
	}
}

func (m *mockRemoteStorage) Upload(name string, data io.Reader, size int64) error {
	m.ensureMap()
	content, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	m.files[name] = content
	return nil
}

func (m *mockRemoteStorage) Download(name string) (io.ReadCloser, error) {
	content, ok := m.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (m *mockRemoteStorage) DownloadRange(name string, start, length int64) (io.ReadCloser, error) {
	content, ok := m.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	end := start + length
	if end > int64(len(content)) {
		end = int64(len(content))
	}
	return io.NopCloser(bytes.NewReader(content[start:end])), nil
}

func (m *mockRemoteStorage) Delete(path string) error {
	delete(m.files, path)
	return nil
}

func (m *mockRemoteStorage) Rename(oldPath, newPath string) error {
	m.ensureMap()
	content, ok := m.files[oldPath]
	if !ok {
		return os.ErrNotExist
	}
	m.files[newPath] = content
	delete(m.files, oldPath)
	return nil
}

func (m *mockRemoteStorage) Stat(path string) (os.FileInfo, error) {
	m.ensureMap()
	content, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &mockFileInfo{
		name: path,
		size: int64(len(content)),
	}, nil
}

func (m *mockRemoteStorage) Close() error {
	return nil
}

type mockFileInfo struct {
	name string
	size int64
}

func (fi *mockFileInfo) Name() string       { return fi.name }
func (fi *mockFileInfo) Size() int64        { return fi.size }
func (fi *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (fi *mockFileInfo) ModTime() time.Time { return time.Now() }
func (fi *mockFileInfo) IsDir() bool        { return false }
func (fi *mockFileInfo) Sys() interface{}   { return nil }
