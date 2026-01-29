package webdav

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"clearvault/tests/testutil"
)

// testServerManager holds the server manager for all tests in this package
var testServerManager *testutil.TestServerManager

// TestMain is the entry point for all tests in this package
func TestMain(m *testing.M) {
	// Create server manager
	manager, err := testutil.NewTestServerManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server manager: %v\n", err)
		os.Exit(1)
	}

	// Start servers automatically
	if err := manager.StartAll(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start test servers: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please ensure zs3 and sweb are installed in tools/ directory\n")
		os.Exit(1)
	}

	testServerManager = manager

	// Run tests
	code := m.Run()

	// Cleanup: Stop servers
	manager.StopAll()

	os.Exit(code)
}

// getTestConfig returns test WebDAV configuration for sweb
func getTestConfig() WebDAVConfig {
	return WebDAVConfig{
		URL:  "http://localhost:8081/webdav",
		User: "admin",
		Pass: "admin123",
	}
}

// checkWebDAVAvailable checks if sweb server is available
func checkWebDAVAvailable(t *testing.T) bool {
	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		return false
	}
	defer client.Close()

	// Try to stat root
	_, err = client.Stat("/")
	return err == nil
}

// TestNewClient tests creating a new WebDAV client
func TestNewClient(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Error("NewClient() returned nil client")
	}
	if client.url != cfg.URL {
		t.Errorf("Expected URL %s, got %s", cfg.URL, client.url)
	}
}

// TestWebDAVClient_Upload tests file upload
func TestWebDAVClient_Upload(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		content  string
		filename string
	}{
		{
			name:     "Small file",
			content:  "Hello, WebDAV World!",
			filename: "/test-small.txt",
		},
		{
			name:     "Empty file",
			content:  "",
			filename: "/test-empty.txt",
		},
		{
			name:     "Large file (100KB)",
			content:  strings.Repeat("Large file content data ", 5000),
			filename: "/test-large.txt",
		},
		{
			name:     "File in directory",
			content:  "Nested content",
			filename: "/testdir/nested.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := bytes.NewReader([]byte(tt.content))
			err := client.Upload(tt.filename, data, int64(len(tt.content)))
			if err != nil {
				t.Errorf("Upload() failed: %v", err)
			}

			// Cleanup
			defer client.Delete(tt.filename)
		})
	}
}

// TestWebDAVClient_Download tests file download
func TestWebDAVClient_Download(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload test file
	content := "Test content for download"
	filename := "/test-download.txt"
	data := bytes.NewReader([]byte(content))
	err = client.Upload(filename, data, int64(len(content)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	defer client.Delete(filename)

	// Download and verify
	reader, err := client.Download(filename)
	if err != nil {
		t.Fatalf("Download() failed: %v", err)
	}
	defer reader.Close()

	downloaded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(downloaded) != content {
		t.Errorf("Downloaded content mismatch. Expected %q, got %q", content, string(downloaded))
	}
}

// TestWebDAVClient_Download_NotFound tests downloading non-existent file
func TestWebDAVClient_Download_NotFound(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	_, err = client.Download("/non-existent-file-12345.txt")
	if err == nil {
		t.Error("Download() of non-existent file should return error")
	}
}

// TestWebDAVClient_DownloadRange tests range download
func TestWebDAVClient_DownloadRange(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload test file
	content := "0123456789ABCDEF"
	filename := "/test-range.txt"
	data := bytes.NewReader([]byte(content))
	err = client.Upload(filename, data, int64(len(content)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	defer client.Delete(filename)

	tests := []struct {
		name         string
		start        int64
		length       int64
		expectedData string
	}{
		{
			name:         "First 5 bytes",
			start:        0,
			length:       5,
			expectedData: "01234",
		},
		{
			name:         "Middle 5 bytes",
			start:        5,
			length:       5,
			expectedData: "56789",
		},
		{
			name:         "Last 6 bytes",
			start:        10,
			length:       6,
			expectedData: "ABCDEF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := client.DownloadRange(filename, tt.start, tt.length)
			if err != nil {
				t.Fatalf("DownloadRange() failed: %v", err)
			}
			defer reader.Close()

			downloaded, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if string(downloaded) != tt.expectedData {
				t.Errorf("Range download mismatch. Expected %q, got %q", tt.expectedData, string(downloaded))
			}
		})
	}
}

// TestWebDAVClient_Delete tests file deletion
func TestWebDAVClient_Delete(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload and delete
	content := "To be deleted"
	filename := "/test-delete.txt"
	data := bytes.NewReader([]byte(content))
	err = client.Upload(filename, data, int64(len(content)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	err = client.Delete(filename)
	if err != nil {
		t.Errorf("Delete() failed: %v", err)
	}

	// Verify deletion
	_, err = client.Download(filename)
	if err == nil {
		t.Error("Download after delete should fail")
	}
}

// TestWebDAVClient_Rename tests file rename
func TestWebDAVClient_Rename(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload file
	content := "Test content for rename"
	oldName := "/test-old-name.txt"
	newName := "/test-new-name.txt"
	data := bytes.NewReader([]byte(content))
	err = client.Upload(oldName, data, int64(len(content)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	defer client.Delete(newName) // Cleanup

	// Rename
	err = client.Rename(oldName, newName)
	if err != nil {
		t.Fatalf("Rename() failed: %v", err)
	}

	// Verify old name doesn't exist
	_, err = client.Download(oldName)
	if err == nil {
		t.Error("Download with old name should fail after rename")
	}

	// Verify new name exists
	reader, err := client.Download(newName)
	if err != nil {
		t.Fatalf("Download with new name failed: %v", err)
	}
	defer reader.Close()

	downloaded, _ := io.ReadAll(reader)
	if string(downloaded) != content {
		t.Error("Content mismatch after rename")
	}
}

// TestWebDAVClient_Stat tests file stat
func TestWebDAVClient_Stat(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload file
	content := "Test content for stat"
	filename := "/test-stat.txt"
	data := bytes.NewReader([]byte(content))
	err = client.Upload(filename, data, int64(len(content)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	defer client.Delete(filename)

	// Stat
	info, err := client.Stat(filename)
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}

	if info.Name() != "test-stat.txt" {
		t.Errorf("Expected name 'test-stat.txt', got %q", info.Name())
	}
	if info.Size() != int64(len(content)) {
		t.Errorf("Expected size %d, got %d", len(content), info.Size())
	}
}

// TestWebDAVClient_Stat_NotFound tests stat on non-existent file
func TestWebDAVClient_Stat_NotFound(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	_, err = client.Stat("/non-existent-file-12345.txt")
	if err == nil {
		t.Error("Stat() of non-existent file should return error")
	}
}

// TestWebDAVClient_ConcurrentOperations tests concurrent uploads and downloads
func TestWebDAVClient_ConcurrentOperations(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Concurrent uploads
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			content := []byte("Concurrent content")
			filename := "/concurrent-" + string(rune('a'+id)) + ".txt"
			data := bytes.NewReader(content)
			err := client.Upload(filename, data, int64(len(content)))
			if err == nil {
				client.Delete(filename)
			}
			done <- err
		}(i)
	}

	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent upload failed: %v", err)
		}
	}
}

// TestWebDAVClient_DirectoryOperations tests directory operations
func TestWebDAVClient_DirectoryOperations(t *testing.T) {
	if !checkWebDAVAvailable(t) {
		t.Skip("WebDAV server (sweb) not available")
	}

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Create directory by uploading file to it
	dirPath := "/testdir"
	filePath := dirPath + "/file.txt"
	content := "File in directory"
	data := bytes.NewReader([]byte(content))
	err = client.Upload(filePath, data, int64(len(content)))
	if err != nil {
		t.Fatalf("Upload to directory failed: %v", err)
	}

	// Stat directory
	info, err := client.Stat(dirPath)
	if err != nil {
		t.Fatalf("Stat directory failed: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory to be a directory")
	}

	// Cleanup
	client.Delete(filePath)
	client.Delete(dirPath)
}
