//go:build fuse

package tests

import (
	"bytes"
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/webdav"
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	localwebdav "golang.org/x/net/webdav"
)

// TestFUSEOperations 测试FUSE文件系统操作
func TestFUSEOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping FUSE test in short mode")
	}

	// Setup mock remote storage
	mockRemoteDir, err := os.MkdirTemp("", "fuse_remote_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	// Setup metadata
	metaDir, err := os.MkdirTemp("", "fuse_meta_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(metaDir)

	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	cfg := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: key},
	}
	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	// Setup proxy
	remoteClient, err := webdav.NewClient(webdav.WebDAVConfig{URL: cfg.Remote.URL})
	if err != nil {
		t.Fatalf("Failed to create remote client: %v", err)
	}

	proxy, err := proxy.NewProxy(meta, remoteClient, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Setup FUSE filesystem
	fs := proxy.NewFileSystem()

	testCases := []struct {
		name string
		test func(*testing.T)
	}{
		{"WriteAndRead", testFUSEWriteAndRead},
		{"FileAttributes", testFUSEFileAttributes},
		{"DirectoryOperations", testFUSEDirectoryOperations},
		{"FileCreateAndTruncate", testFUSEFileCreateAndTruncate},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.test(t)
		})
	}
}

func testFUSEWriteAndRead(t *testing.T, fs *proxy.FileSystem) {
	testPath := "/test-fuse-write.txt"
	testContent := []byte("FUSE write and read test content")

	// Write file
	err := fs.WriteFile(context.Background(), testPath, testContent, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Read file
	readContent, err := fs.ReadFile(context.Background(), testPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if !bytes.Equal(testContent, readContent) {
		t.Errorf("Content mismatch")
		t.Logf("Expected: %s", string(testContent))
		t.Logf("Got: %s", string(readContent))
	}
}

func testFUSEFileAttributes(t *testing.T, fs *proxy.FileSystem) {
	testPath := "/test-attributes.txt"
	testContent := []byte("Attributes test content")
	testSize := int64(len(testContent))

	// Create file
	err := fs.WriteFile(context.Background(), testPath, testContent, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Stat file
	info, err := fs.Stat(context.Background(), testPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Size() != testSize {
		t.Errorf("Size mismatch: got %d, want %d", info.Size(), testSize)
	}

	if info.Name() != filepath.Base(testPath) {
		t.Errorf("Name mismatch: got %s, want %s", info.Name(), filepath.Base(testPath))
	}

	if info.IsDir() {
		t.Error("File should not be directory")
	}

	if info.Mode().IsDir() {
		t.Error("Mode should indicate regular file")
	}
}

func testFUSEDirectoryOperations(t *testing.T, fs *proxy.FileSystem) {
	dirPath := "/test-fuse-dir"

	// Create directory
	err := fs.Mkdir(context.Background(), dirPath, 0755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Verify directory exists
	info, err := fs.Stat(context.Background(), dirPath)
	if err != nil {
		t.Fatalf("Stat directory failed: %v", err)
	}

	if !info.IsDir() {
		t.Error("Should be directory")
	}

	// Create file in directory
	filePath := dirPath + "/file.txt"
	fileContent := []byte("File in directory")
	err = fs.WriteFile(context.Background(), filePath, fileContent, 0644)
	if err != nil {
		t.Fatalf("WriteFile in directory failed: %v", err)
	}

	// List directory
	entries, err := fs.ReadDir(context.Background(), dirPath)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Name() != "file.txt" {
		t.Errorf("Entry name mismatch: got %s", entries[0].Name())
	}
}

func testFUSEFileCreateAndTruncate(t *testing.T, fs *proxy.FileSystem) {
	testPath := "/test-truncate.txt"

	// Create file with content
	initialContent := []byte("Initial content for truncation test")
	err := fs.WriteFile(context.Background(), testPath, initialContent, 0644)
	if err != nil {
		t.Fatalf("Initial WriteFile failed: %v", err)
	}

	// Truncate to smaller size
	err = fs.Truncate(context.Background(), testPath, 10)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify truncated
	info, err := fs.Stat(context.Background(), testPath)
	if err != nil {
		t.Fatalf("Stat after truncate failed: %v", err)
	}

	if info.Size() != 10 {
		t.Errorf("Size after truncate: got %d, want 10", info.Size())
	}

	// Truncate to larger size
	err = fs.Truncate(context.Background(), testPath, 100)
	if err != nil {
		t.Fatalf("Truncate to larger failed: %v", err)
	}

	info, err = fs.Stat(context.Background(), testPath)
	if err != nil {
		t.Fatalf("Stat after larger truncate failed: %v", err)
	}

	if info.Size() != 100 {
		t.Errorf("Size after larger truncate: got %d, want 100", info.Size())
	}
}

// TestFUSEProxyIntegration 测试FUSE与代理的集成
func TestFUSEProxyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping FUSE integration test in short mode")
	}

	// Setup
	mockRemoteDir, err := os.MkdirTemp("", "fuse_int_remote_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	metaDir, err := os.MkdirTemp("", "fuse_int_meta_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(metaDir)

	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	cfg := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: key},
	}
	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	remoteClient, err := webdav.NewClient(webdav.WebDAVConfig{URL: cfg.Remote.URL})
	if err != nil {
		t.Fatalf("Failed to create remote client: %v", err)
	}

	proxy, err := proxy.NewProxy(meta, remoteClient, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	fs := proxy.NewFileSystem()

	// Test end-to-end flow: WriteFile -> Proxy -> Remote -> ReadFile -> Proxy -> Remote
	testPath := "/integration-test.txt"
	testContent := []byte("Integration test content with some data to encrypt and decrypt")

	err = fs.WriteFile(context.Background(), testPath, testContent, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify through proxy
	readContent, err := fs.ReadFile(context.Background(), testPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if !bytes.Equal(testContent, readContent) {
		t.Errorf("Content mismatch in integration test")
		t.Logf("Original hash: %s", calculateHash(testContent))
		t.Logf("Read hash: %s", calculateHash(readContent))
	}

	// Verify metadata exists
	metaInfo, err := proxy.GetFileMeta(testPath)
	if err != nil {
		t.Fatalf("GetFileMeta failed: %v", err)
	}

	if metaInfo == nil {
		t.Error("Metadata not found")
	}

	if metaInfo.Size != int64(len(testContent)) {
		t.Errorf("Metadata size mismatch: got %d, want %d", metaInfo.Size, len(testContent))
	}
}

// TestFUSEErrorHandling 测试FUSE错误处理
func TestFUSEErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping FUSE error test in short mode")
	}

	// Setup
	mockRemoteDir, err := os.MkdirTemp("", "fuse_err_remote_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	metaDir, err := os.MkdirTemp("", "fuse_err_meta_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(metaDir)

	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	cfg := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: key},
	}
	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	remoteClient, err := webdav.NewClient(webdav.WebDAVConfig{URL: cfg.Remote.URL})
	if err != nil {
		t.Fatalf("Failed to create remote client: %v", err)
	}

	proxy, err := proxy.NewProxy(meta, remoteClient, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	fs := proxy.NewFileSystem()

	t.Run("ReadNonExistent", func(t *testing.T) {
		_, err := fs.ReadFile(context.Background(), "/nonexistent.txt")
		if err == nil {
			t.Error("Expected error reading non-existent file")
		}
	})

	t.Run("StatNonExistent", func(t *testing.T) {
		_, err := fs.Stat(context.Background(), "/nonexistent.txt")
		if err == nil {
			t.Error("Expected error stating non-existent file")
		}
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		err := fs.RemoveAll(context.Background(), "/nonexistent.txt")
		if err == nil {
			t.Error("Expected error deleting non-existent file")
		}
	})

	t.Run("ReadDirNonExistent", func(t *testing.T) {
		_, err := fs.ReadDir(context.Background(), "/nonexistent-dir")
		if err == nil {
			t.Error("Expected error reading non-existent directory")
		}
	})
}

// TestFUSELargeFile 测试大文件处理
func TestFUSELargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping FUSE large file test in short mode")
	}

	// Setup
	mockRemoteDir, err := os.MkdirTemp("", "fuse_large_remote_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	metaDir, err := os.MkdirTemp("", "fuse_large_meta_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(metaDir)

	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	cfg := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: key},
	}
	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	remoteClient, err := webdav.NewClient(webdav.WebDAVConfig{URL: cfg.Remote.URL})
	if err != nil {
		t.Fatalf("Failed to create remote client: %v", err)
	}

	proxy, err := proxy.NewProxy(meta, remoteClient, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	fs := proxy.NewFileSystem()

	// Test with 5MB file
	testPath := "/large-file-test.bin"
	fileSize := int64(5 * 1024 * 1024)

	testData := make([]byte, fileSize)
	for i := int64(0); i < fileSize; i++ {
		testData[i] = byte(i % 256)
	}

	t.Log("Writing 5MB file through FUSE...")
	startTime := time.Now()

	err = fs.WriteFile(context.Background(), testPath, testData, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	writeTime := time.Since(startTime)
	t.Logf("Write took: %v (%.2f MB/s)", writeTime, float64(fileSize)/writeTime.Seconds()/1024/1024)

	t.Log("Reading 5MB file through FUSE...")
	startTime = time.Now()

	readData, err := fs.ReadFile(context.Background(), testPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	readTime := time.Since(startTime)
	t.Logf("Read took: %v (%.2f MB/s)", readTime, float64(fileSize)/readTime.Seconds()/1024/1024)

	if !bytes.Equal(testData, readData) {
		t.Error("Large file data mismatch")

		// Find first mismatch
		for i := 0; i < len(testData) && i < len(readData); i++ {
			if testData[i] != readData[i] {
				t.Logf("First mismatch at byte %d: got %02x, want %02x", i, readData[i], testData[i])
				break
			}
		}
	}

	// Verify checksums
	origHash := calculateHash(testData)
	readHash := calculateHash(readData)
	if origHash != readHash {
		t.Errorf("Hash mismatch: orig=%s, read=%s", origHash, readHash)
	}
}

// TestFUSEConcurrentOperations 测试并发操作
func TestFUSEConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping FUSE concurrent test in short mode")
	}

	// Setup
	mockRemoteDir, err := os.MkdirTemp("", "fuse_conc_remote_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	metaDir, err := os.MkdirTemp("", "fuse_conc_meta_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(metaDir)

	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	cfg := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: key},
	}
	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	remoteClient, err := webdav.NewClient(webdav.WebDAVConfig{URL: cfg.Remote.URL})
	if err != nil {
		t.Fatalf("Failed to create remote client: %v", err)
	}

	proxy, err := proxy.NewProxy(meta, remoteClient, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	fs := proxy.NewFileSystem()

	// Concurrent writes
	fileCount := 20
	done := make(chan bool, fileCount)

	for i := 0; i < fileCount; i++ {
		go func(index int) {
			path := filepath.Join("/", fmt.Sprintf("concurrent-%d.txt", index))
			content := []byte(fmt.Sprintf("Concurrent file %d content", index))

			err := fs.WriteFile(context.Background(), path, content, 0644)
			if err != nil {
				t.Logf("Write %d failed: %v", index, err)
				done <- false
				return
			}

			// Read back to verify
			readContent, err := fs.ReadFile(context.Background(), path)
			if err != nil {
				t.Logf("Read %d failed: %v", index, err)
				done <- false
				return
			}

			if !bytes.Equal(content, readContent) {
				t.Logf("Content mismatch for file %d", index)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Wait for all operations
	success := 0
	for i := 0; i < fileCount; i++ {
		if <-done {
			success++
		}
	}

	if success != fileCount {
		t.Errorf("Only %d/%d operations succeeded", success, fileCount)
	}
}

// TestFUSEFileOperations 测试文件操作细节
func TestFUSEFileOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping FUSE file operations test in short mode")
	}

	// Setup
	mockRemoteDir, err := os.MkdirTemp("", "fuse_ops_remote_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	metaDir, err := os.MkdirTemp("", "fuse_ops_meta_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(metaDir)

	key := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	cfg := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: key},
	}
	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	remoteClient, err := webdav.NewClient(webdav.WebDAVConfig{URL: cfg.Remote.URL})
	if err != nil {
		t.Fatalf("Failed to create remote client: %v", err)
	}

	proxy, err := proxy.NewProxy(meta, remoteClient, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	fs := proxy.NewFileSystem()

	t.Run("CreateEmptyFile", func(t *testing.T) {
		path := "/empty-file.txt"
		err := fs.WriteFile(context.Background(), path, []byte{}, 0644)
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		info, err := fs.Stat(context.Background(), path)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if info.Size() != 0 {
			t.Errorf("Empty file size: got %d, want 0", info.Size())
		}
	})

	t.Run("RenameFile", func(t *testing.T) {
		oldPath := "/rename-old.txt"
		newPath := "/rename-new.txt"
		content := []byte("Rename test")

		err := fs.WriteFile(context.Background(), oldPath, content, 0644)
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		err = fs.Rename(context.Background(), oldPath, newPath)
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify old doesn't exist
		_, err = fs.Stat(context.Background(), oldPath)
		if err == nil {
			t.Error("Old file should not exist after rename")
		}

		// Verify new exists
		info, err := fs.Stat(context.Background(), newPath)
		if err != nil {
			t.Fatalf("Stat new path failed: %v", err)
		}

		if info.Size() != int64(len(content)) {
			t.Errorf("Renamed file size mismatch")
		}
	})

	t.Run("DeleteFile", func(t *testing.T) {
		path := "/delete-me.txt"
		content := []byte("Delete test")

		err := fs.WriteFile(context.Background(), path, content, 0644)
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		err = fs.RemoveAll(context.Background(), path)
		if err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		_, err = fs.Stat(context.Background(), path)
		if err == nil {
			t.Error("File should not exist after deletion")
		}
	})

	t.Run("NestedDirectory", func(t *testing.T) {
		nestedPath := "/a/b/c/d/e.txt"
		content := []byte("Nested file")

		err := fs.WriteFile(context.Background(), nestedPath, content, 0644)
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		readContent, err := fs.ReadFile(context.Background(), nestedPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		if !bytes.Equal(content, readContent) {
			t.Error("Nested file content mismatch")
		}
	})
}
