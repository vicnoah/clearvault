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

	// 1. Save memory placeholder (simulating Raidrive Phase 1)
	err = p.SavePlaceholder("/test.txt")
	if err != nil {
		t.Fatalf("SavePlaceholder failed: %v", err)
	}

	// 2. Verify memory placeholder exists
	if !p.pendingCache.Exists("/test.txt") {
		t.Error("Expected memory placeholder to exist")
	}

	// 3. Verify no metadata is created for placeholder
	m, err := meta.Get("/test.txt")
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}
	if m != nil {
		t.Errorf("Expected no metadata for memory placeholder, got %+v", m)
	}

	// 4. Upload actual content (simulating Raidrive Phase 2)
	err = p.UploadFile("/test.txt", bytes.NewReader([]byte("test content")), 12)
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	// 5. Verify memory placeholder is removed
	if p.pendingCache.Exists("/test.txt") {
		t.Error("Expected memory placeholder to be removed after upload")
	}

	// 6. Verify metadata is created
	m, err = meta.Get("/test.txt")
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}
	if m == nil {
		t.Fatal("Expected metadata to be created")
	}
	if m.Size != 12 {
		t.Errorf("Expected Size to be 12, got %d", m.Size)
	}

	// 7. Verify Download returns actual content
	rc, err := p.DownloadFile("/test.txt")
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Failed to read downloaded content: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("Expected 'test content', got '%s'", string(content))
	}
}

func TestMemoryPlaceholderExpiration(t *testing.T) {
	metaDir := "./test_placeholder_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	mockRemote := &mockRemoteStorage{}
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// Add a placeholder with very short TTL (100ms)
	p.pendingCache.Add("/test.txt", 100*time.Millisecond)

	// Verify placeholder exists immediately
	if !p.pendingCache.Exists("/test.txt") {
		t.Error("Expected memory placeholder to exist immediately after creation")
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Verify placeholder has expired
	if p.pendingCache.Exists("/test.txt") {
		t.Error("Expected memory placeholder to be expired after TTL")
	}
}

func TestMemoryPlaceholderManualRemoval(t *testing.T) {
	metaDir := "./test_placeholder_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	mockRemote := &mockRemoteStorage{}
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// Add a placeholder
	p.pendingCache.Add("/test.txt", 30*time.Second)

	// Verify placeholder exists
	if !p.pendingCache.Exists("/test.txt") {
		t.Error("Expected memory placeholder to exist")
	}

	// Manually remove placeholder
	p.pendingCache.Remove("/test.txt")

	// Verify placeholder is removed
	if p.pendingCache.Exists("/test.txt") {
		t.Error("Expected memory placeholder to be removed")
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

// TestLargeFileUpload tests uploading and downloading large files
func TestLargeFileUpload(t *testing.T) {
	metaDir := "./test_largefile_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	mockRemote := newMockRemoteStorage()
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// Test with 5MB of data
	fileSize := 5 * 1024 * 1024
	data := make([]byte, fileSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Upload
	err = p.UploadFile("/large.bin", bytes.NewReader(data), int64(fileSize))
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	// Download and verify
	rc, err := p.DownloadFile("/large.bin")
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer rc.Close()

	downloaded, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Failed to read downloaded content: %v", err)
	}

	if !bytes.Equal(data, downloaded) {
		t.Errorf("Large file content mismatch: expected %d bytes, got %d", len(data), len(downloaded))
	}
}

// TestSpecialCharactersFilename tests filenames with special characters
func TestSpecialCharactersFilename(t *testing.T) {
	metaDir := "./test_special_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	mockRemote := newMockRemoteStorage()
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	specialNames := []string{
		"/test file with spaces.txt",
		"/test-file-with-dashes.txt",
		"/test_file_with_underscores.txt",
		"/test.file.with.dots.txt",
		"/test(1).txt",
		"/test[1].txt",
		"/test{1}.txt",
		"/test+1.txt",
		"/test=1.txt",
		"/test@1.txt",
		"/test#1.txt",
		"/test%201.txt",
	}

	for _, name := range specialNames {
		content := []byte("content for " + name)
		err := p.UploadFile(name, bytes.NewReader(content), int64(len(content)))
		if err != nil {
			t.Errorf("Failed to upload %s: %v", name, err)
			continue
		}

		// Verify download
		rc, err := p.DownloadFile(name)
		if err != nil {
			t.Errorf("Failed to download %s: %v", name, err)
			continue
		}

		downloaded, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Errorf("Failed to read %s: %v", name, err)
			continue
		}

		if !bytes.Equal(content, downloaded) {
			t.Errorf("Content mismatch for %s", name)
		}
	}
}

// TestFileRename tests file rename operation
func TestFileRename(t *testing.T) {
	metaDir := "./test_rename_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	mockRemote := newMockRemoteStorage()
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// Create original file
	originalContent := []byte("original content")
	err = p.UploadFile("/original.txt", bytes.NewReader(originalContent), int64(len(originalContent)))
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	// Rename file
	err = p.RenameFile("/original.txt", "/renamed.txt")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Verify original doesn't exist
	_, err = p.DownloadFile("/original.txt")
	if err == nil {
		t.Error("Expected error when downloading original file after rename")
	}

	// Verify renamed file exists with correct content
	rc, err := p.DownloadFile("/renamed.txt")
	if err != nil {
		t.Fatalf("Failed to download renamed file: %v", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Failed to read renamed file: %v", err)
	}

	if !bytes.Equal(originalContent, content) {
		t.Errorf("Content mismatch after rename")
	}
}

// TestFileDelete tests file deletion
func TestFileDelete(t *testing.T) {
	metaDir := "./test_delete_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	mockRemote := newMockRemoteStorage()
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// Create file
	content := []byte("content to delete")
	err = p.UploadFile("/delete-me.txt", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	// Verify file exists
	_, err = p.DownloadFile("/delete-me.txt")
	if err != nil {
		t.Fatalf("File should exist before deletion")
	}

	// Delete file
	err = p.RemoveAll("/delete-me.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is deleted
	_, err = p.DownloadFile("/delete-me.txt")
	if err == nil {
		t.Error("Expected error when downloading deleted file")
	}
}

// TestEmptyContentUpload tests uploading empty content
func TestEmptyContentUpload(t *testing.T) {
	metaDir := "./test_empty_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	mockRemote := newMockRemoteStorage()
	var remoteStorage remote.RemoteStorage = mockRemote

	p, err := NewProxy(meta, remoteStorage, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// Upload empty content
	err = p.UploadFile("/empty.txt", bytes.NewReader([]byte{}), 0)
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	// Download and verify
	rc, err := p.DownloadFile("/empty.txt")
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Failed to read content: %v", err)
	}

	if len(content) != 0 {
		t.Errorf("Expected empty content, got %d bytes", len(content))
	}
}
