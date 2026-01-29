package tests

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/studio-b12/gowebdav"
)

const (
	swebEndpoint = "http://localhost:8081"
	swebUser     = "admin"
	swebPass     = "admin123"
	swebBasePath = "/webdav"
)

func TestWebDAVConnection(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	// Test connection
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}

	t.Log("WebDAV connection successful")
	
	// Cleanup
	client.RemoveAll(swebBasePath)
}

func TestWebDAVBasicOperations(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}
	defer client.RemoveAll(swebBasePath)

	testPath := swebBasePath + "/test-basic.txt"
	testContent := []byte("WebDAV basic operations test")

	// Test PUT
	err = client.WriteStream(testPath, bytes.NewReader(testContent), 0644)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}

	// Test GET
	readContent, err := client.ReadStream(testPath)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer readContent.Close()

	downloaded, err := io.ReadAll(readContent)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if !bytes.Equal(testContent, downloaded) {
		t.Error("Content mismatch")
	}

	// Test PROPFIND (Stat)
	info, err := client.Stat(testPath)
	if err != nil {
		t.Fatalf("STAT failed: %v", err)
	}

	if info.Size() != int64(len(testContent)) {
		t.Errorf("Size mismatch: got %d, want %d", info.Size(), len(testContent))
	}

	// Test DELETE
	err = client.Remove(testPath)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}

	// Verify deleted
	_, err = client.Stat(testPath)
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

func TestWebDAVDirectories(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}
	defer client.RemoveAll(swebBasePath)

	// Create directories
	dirs := []string{
		swebBasePath + "/dir1",
		swebBasePath + "/dir1/subdir1",
		swebBasePath + "/dir1/subdir2",
		swebBasePath + "/dir2",
	}

	for _, dir := range dirs {
		err = client.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("MkdirAll %s failed: %v", dir, err)
		}
	}

	// List root
	files, err := client.ReadDir(swebBasePath)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	foundDirs := 0
	for _, f := range files {
		if f.IsDir() && (f.Name() == "dir1" || f.Name() == "dir2") {
			foundDirs++
		}
	}

	if foundDirs != 2 {
		t.Errorf("Expected 2 directories, found %d", foundDirs)
	}

	// List subdirectory
	subfiles, err := client.ReadDir(swebBasePath + "/dir1")
	if err != nil {
		t.Fatalf("ReadDir /dir1 failed: %v", err)
	}

	foundSubDirs := 0
	for _, f := range subfiles {
		if f.IsDir() && (f.Name() == "subdir1" || f.Name() == "subdir2") {
			foundSubDirs++
		}
	}

	if foundSubDirs != 2 {
		t.Errorf("Expected 2 subdirectories, found %d", foundSubDirs)
	}
}

func TestWebDAVRangeRequests(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}
	defer client.RemoveAll(swebBasePath)

	testPath := swebBasePath + "/test-range.txt"
	testContent := bytes.Repeat([]byte("0123456789"), 1000) // 10KB

	err = client.WriteStream(testPath, bytes.NewReader(testContent), 0644)
	if err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	testCases := []struct {
		name   string
		start  int64
		length int64
	}{
		{"first_100", 0, 100},
		{"middle", 5000, 100},
		{"last_100", int64(len(testContent)) - 100, 100},
		{"single_byte", 5000, 1},
		{"full_range", 0, int64(len(testContent))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader, err := client.ReadStreamRange(testPath, tc.start, tc.length)
			if err != nil {
				t.Fatalf("ReadStreamRange failed: %v", err)
			}
			defer reader.Close()

			downloaded, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			expected := testContent[tc.start : tc.start+tc.length]
			if !bytes.Equal(downloaded, expected) {
				t.Error("Range content mismatch")
			}
		})
	}
}

func TestWebDAVCopyMove(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}
	defer client.RemoveAll(swebBasePath)

	testPath := swebBasePath + "/test-copy.txt"
	copyPath := swebBasePath + "/test-copy-copy.txt"
	movePath := swebBasePath + "/test-move.txt"
	testContent := []byte("Test content for copy/move")

	err = client.WriteStream(testPath, bytes.NewReader(testContent), 0644)
	if err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	// Test COPY
	err = client.Copy(testPath, copyPath, false)
	if err != nil {
		t.Fatalf("COPY failed: %v", err)
	}

	// Verify copy exists
	readContent, err := client.ReadStream(copyPath)
	if err != nil {
		t.Fatalf("Read copy failed: %v", err)
	}
	defer readContent.Close()

	downloaded, err := io.ReadAll(readContent)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if !bytes.Equal(testContent, downloaded) {
		t.Error("Copy content mismatch")
	}

	// Verify original still exists
	readContent, err = client.ReadStream(testPath)
	if err != nil {
		t.Fatalf("Read original failed: %v", err)
	}
	defer readContent.Close()

	// Test MOVE
	err = client.Rename(testPath, movePath, false)
	if err != nil {
		t.Fatalf("MOVE failed: %v", err)
	}

	// Verify move exists
	readContent, err = client.ReadStream(movePath)
	if err != nil {
		t.Fatalf("Read move failed: %v", err)
	}
	defer readContent.Close()

	downloaded, err = io.ReadAll(readContent)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if !bytes.Equal(testContent, downloaded) {
		t.Error("Move content mismatch")
	}

	// Verify original no longer exists
	_, err = client.Stat(testPath)
	if err == nil {
		t.Error("Original file should not exist after move")
	}
}

func TestWebDAVLargeFile(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}
	defer client.RemoveAll(swebBasePath)

	testPath := swebBasePath + "/test-large.bin"
	fileSize := int64(25 * 1024 * 1024) // 25MB

	// Generate test data with pattern
	data := make([]byte, fileSize)
	for i := int64(0); i < fileSize; i++ {
		data[i] = byte(i % 256)
	}

	t.Log("Uploading 25MB file...")
	startTime := time.Now()

	err = client.WriteStream(testPath, bytes.NewReader(data), 0644)
	if err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	uploadDuration := time.Since(startTime)
	t.Logf("Upload took: %v (%.2f MB/s)", uploadDuration, float64(fileSize)/uploadDuration.Seconds()/1024/1024)

	// Download and verify
	t.Log("Downloading 25MB file...")
	startTime = time.Now()

	readContent, err := client.ReadStream(testPath)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	defer readContent.Close()

	downloaded, err := io.ReadAll(readContent)
	downloadDuration := time.Since(startTime)
	t.Logf("Download took: %v (%.2f MB/s)", downloadDuration, float64(fileSize)/downloadDuration.Seconds()/1024/1024)

	if !bytes.Equal(data, downloaded) {
		t.Error("Large file data mismatch")
	}

	// Verify checksums
	origHash := calculateHash(data)
	downHash := calculateHash(downloaded)
	if origHash != downHash {
		t.Errorf("Hash mismatch: orig=%s, down=%s", origHash, downHash)
	}
}

func TestWebDAVConcurrentOperations(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}
	defer client.RemoveAll(swebBasePath)

	fileCount := 20
	done := make(chan bool, fileCount)

	// Concurrent uploads - each goroutine uses its own client instance
	// to avoid 423 Locked errors from shared WebDAV locks
	for i := 0; i < fileCount; i++ {
		go func(index int) {
			// Create separate client for each goroutine to avoid lock conflicts
			c := gowebdav.NewClient(url, swebUser, swebPass)
			if err := c.Connect(); err != nil {
				log.Printf("Client %d connect failed: %v", index, err)
				done <- false
				return
			}

			path := fmt.Sprintf("%s/concurrent-%d.txt", swebBasePath, index)
			content := []byte(fmt.Sprintf("Concurrent file %d", index))

			err := c.WriteStream(path, bytes.NewReader(content), 0644)
			if err != nil {
				log.Printf("Upload %d failed: %v", index, err)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Wait for uploads
	success := 0
	for i := 0; i < fileCount; i++ {
		if <-done {
			success++
		}
	}

	if success != fileCount {
		t.Errorf("Only %d/%d uploads succeeded", success, fileCount)
	}

	// Verify all files
	files, err := client.ReadDir(swebBasePath)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	if len(files) != fileCount {
		t.Errorf("Expected %d files, got %d", fileCount, len(files))
	}
}

// TestWebDAVConcurrentMixed tests concurrent mixed operations (read/write)
func TestWebDAVConcurrentMixed(t *testing.T) {
	url := swebEndpoint + swebBasePath
	client := gowebdav.NewClient(url, swebUser, swebPass)
	
	err := client.Connect()
	if err != nil {
		t.Skipf("WebDAV server not available: %v", err)
	}
	defer client.RemoveAll(swebBasePath)

	// Prepare test files first
	fileCount := 10
	for i := 0; i < fileCount; i++ {
		path := fmt.Sprintf("%s/mixed-%d.txt", swebBasePath, i)
		content := []byte(fmt.Sprintf("Test content for file %d", i))
		if err := client.WriteStream(path, bytes.NewReader(content), 0644); err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
	}

	done := make(chan bool, fileCount*2)

	// Concurrent reads
	for i := 0; i < fileCount; i++ {
		go func(index int) {
			c := gowebdav.NewClient(url, swebUser, swebPass)
			if err := c.Connect(); err != nil {
				done <- false
				return
			}

			path := fmt.Sprintf("%s/mixed-%d.txt", swebBasePath, index)
			reader, err := c.ReadStream(path)
			if err != nil {
				log.Printf("Read %d failed: %v", index, err)
				done <- false
				return
			}
			defer reader.Close()

			_, err = io.ReadAll(reader)
			if err != nil {
				log.Printf("ReadAll %d failed: %v", index, err)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Concurrent writes to different files
	for i := 0; i < fileCount; i++ {
		go func(index int) {
			c := gowebdav.NewClient(url, swebUser, swebPass)
			if err := c.Connect(); err != nil {
				done <- false
				return
			}

			path := fmt.Sprintf("%s/mixed-write-%d.txt", swebBasePath, index)
			content := []byte(fmt.Sprintf("Write test %d", index))

			if err := c.WriteStream(path, bytes.NewReader(content), 0644); err != nil {
				log.Printf("Write %d failed: %v", index, err)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Wait for all operations
	success := 0
	for i := 0; i < fileCount*2; i++ {
		if <-done {
			success++
		}
	}

	if success != fileCount*2 {
		t.Errorf("Only %d/%d operations succeeded", success, fileCount*2)
	}
}
