package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"clearvault/tests/testutil"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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

// getTestConfig returns test S3 configuration for zs3
func getTestConfig() S3Config {
	return S3Config{
		Endpoint:  "localhost:9000",
		Region:    "us-east-1",
		Bucket:    "clearvault-test",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
	}
}

// checkS3Available checks if zs3 server is available
func checkS3Available(t *testing.T) bool {
	cfg := getTestConfig()
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return false
	}

	ctx := context.Background()
	_, err = client.BucketExists(ctx, cfg.Bucket)
	return err == nil
}

// ensureTestBucket ensures the test bucket exists
func ensureTestBucket(t *testing.T) {
	cfg := getTestConfig()
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		t.Skipf("Failed to create minio client: %v", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		t.Skipf("Failed to check bucket: %v", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region})
		if err != nil {
			t.Skipf("Failed to create bucket (zs3 may not support bucket creation): %v", err)
		}
	}
}

// TestNewClient tests creating a new S3 client
func TestNewClient(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}

	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Error("NewClient() returned nil client")
	}
	if client.bucket != cfg.Bucket {
		t.Errorf("Expected bucket %s, got %s", cfg.Bucket, client.bucket)
	}
}

// TestNewClient_InvalidBucket tests creating client with non-existent bucket
func TestNewClient_InvalidBucket(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}

	cfg := getTestConfig()
	cfg.Bucket = "non-existent-bucket-12345"

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() with invalid bucket should return error")
	}
}

// TestS3Client_Upload tests file upload
func TestS3Client_Upload(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

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
			content:  "Hello, S3 World!",
			filename: "test-small.txt",
		},
		{
			name:     "Empty file",
			content:  "",
			filename: "test-empty.txt",
		},
		{
			name:     "Large file (1MB)",
			content:  strings.Repeat("Large file content data ", 50000), // ~1MB
			filename: "test-large.txt",
		},
		{
			name:     "File with path",
			content:  "Nested content",
			filename: "path/to/nested/file.txt",
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

// TestS3Client_Download tests file download
func TestS3Client_Download(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload test file
	content := "Test content for download"
	filename := "test-download.txt"
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

// TestS3Client_Download_NotFound tests downloading non-existent file
func TestS3Client_Download_NotFound(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	_, err = client.Download("non-existent-file-12345.txt")
	if err == nil {
		t.Error("Download() of non-existent file should return error")
	}
}

// TestS3Client_DownloadRange tests range download
func TestS3Client_DownloadRange(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload test file
	content := "0123456789ABCDEF"
	filename := "test-range.txt"
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
		{
			name:         "From offset to end",
			start:        10,
			length:       0,
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

// TestS3Client_Delete tests file deletion
func TestS3Client_Delete(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload and delete
	content := "To be deleted"
	filename := "test-delete.txt"
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

// TestS3Client_Rename tests file rename
func TestS3Client_Rename(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload file
	content := "Test content for rename"
	oldName := "test-old-name.txt"
	newName := "test-new-name.txt"
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

// TestS3Client_Stat tests file stat
func TestS3Client_Stat(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Upload file
	content := "Test content for stat"
	filename := "test-stat.txt"
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
	if info.IsDir() {
		t.Error("File should not be a directory")
	}
}

// TestS3Client_Stat_NotFound tests stat on non-existent file
func TestS3Client_Stat_NotFound(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

	cfg := getTestConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	_, err = client.Stat("non-existent-file-12345.txt")
	if err == nil {
		t.Error("Stat() of non-existent file should return error")
	}
}

// TestNormalizePath tests path normalization
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/path/to/file.txt", "path/to/file.txt"},
		{"path/to/file.txt", "path/to/file.txt"},
		{"/file.txt", "file.txt"},
		{"file.txt", "file.txt"},
		{"/path/to/dir/", "path/to/dir/"},
	}

	for _, tt := range tests {
		got := normalizePath(tt.input)
		if got != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// TestS3FileInfo tests the s3FileInfo implementation
func TestS3FileInfo(t *testing.T) {
	info := &s3FileInfo{
		name:    "test.txt",
		size:    1024,
		modTime: time.Now(),
		isDir:   false,
	}

	if info.Name() != "test.txt" {
		t.Error("Name() mismatch")
	}
	if info.Size() != 1024 {
		t.Error("Size() mismatch")
	}
	if info.Mode() != 0644 {
		t.Error("Mode() mismatch")
	}
	if info.IsDir() {
		t.Error("IsDir() should be false")
	}
	if info.Sys() != nil {
		t.Error("Sys() should be nil")
	}
}

// TestS3Client_ConcurrentOperations tests concurrent uploads and downloads
func TestS3Client_ConcurrentOperations(t *testing.T) {
	if !checkS3Available(t) {
		t.Skip("S3 server (zs3) not available")
	}
	ensureTestBucket(t)

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
			filename := "concurrent-" + string(rune('a'+id)) + ".txt"
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


