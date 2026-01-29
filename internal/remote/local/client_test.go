package local

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNewClient(t *testing.T) {
	t.Run("create new client", func(t *testing.T) {
		tmpDir := t.TempDir()
		client, err := NewClient(tmpDir)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer client.Close()

		if client.rootPath != tmpDir {
			t.Errorf("rootPath = %q, want %q", client.rootPath, tmpDir)
		}
	})

	t.Run("create nested directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "storage")
		client, err := NewClient(nestedDir)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		defer client.Close()

		if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
			t.Error("Nested directory should be created")
		}
	})
}

func TestLocalClient_Upload(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	t.Run("upload file", func(t *testing.T) {
		content := []byte("Hello, World!")
		reader := bytes.NewReader(content)

		err := client.Upload("testfile.txt", reader, int64(len(content)))
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		// Verify file exists
		path := filepath.Join(tmpDir, "testfile.txt")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read uploaded file: %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("Content mismatch: got %q, want %q", data, content)
		}
	})

	t.Run("upload empty file", func(t *testing.T) {
		reader := bytes.NewReader([]byte{})

		err := client.Upload("emptyfile.txt", reader, 0)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		path := filepath.Join(tmpDir, "emptyfile.txt")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Failed to stat uploaded file: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("Size = %d, want 0", info.Size())
		}
	})

	t.Run("upload large file", func(t *testing.T) {
		content := make([]byte, 1024*1024) // 1MB
		for i := range content {
			content[i] = byte(i % 256)
		}
		reader := bytes.NewReader(content)

		err := client.Upload("largefile.bin", reader, int64(len(content)))
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		path := filepath.Join(tmpDir, "largefile.bin")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read uploaded file: %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Error("Large file content mismatch")
		}
	})
}

func TestLocalClient_Download(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	// Create test file
	content := []byte("Download test content")
	testFile := filepath.Join(tmpDir, "download_test.txt")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	t.Run("download existing file", func(t *testing.T) {
		reader, err := client.Download("download_test.txt")
		if err != nil {
			t.Fatalf("Download failed: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read download: %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("Content mismatch: got %q, want %q", data, content)
		}
	})

	t.Run("download non-existent file", func(t *testing.T) {
		_, err := client.Download("non_existent.txt")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

func TestLocalClient_DownloadRange(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	// Create test file
	content := []byte("0123456789ABCDEF")
	testFile := filepath.Join(tmpDir, "range_test.txt")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name     string
		start    int64
		length   int64
		expected string
	}{
		{"first 5 bytes", 0, 5, "01234"},
		{"middle 5 bytes", 5, 5, "56789"},
		{"last 6 bytes", 10, 6, "ABCDEF"},
		{"from offset to end", 5, 0, "56789ABCDEF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := client.DownloadRange("range_test.txt", tt.start, tt.length)
			if err != nil {
				t.Fatalf("DownloadRange failed: %v", err)
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("Failed to read range: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("Content = %q, want %q", data, tt.expected)
			}
		})
	}

	t.Run("range non-existent file", func(t *testing.T) {
		_, err := client.DownloadRange("non_existent.txt", 0, 10)
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

func TestLocalClient_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	t.Run("delete existing file", func(t *testing.T) {
		// Create file
		testFile := filepath.Join(tmpDir, "to_delete.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		err := client.Delete("to_delete.txt")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Error("File should be deleted")
		}
	})

	t.Run("delete non-existent file", func(t *testing.T) {
		err := client.Delete("non_existent.txt")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

func TestLocalClient_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	t.Run("rename existing file", func(t *testing.T) {
		// Create file
		oldFile := filepath.Join(tmpDir, "old_name.txt")
		content := []byte("rename test content")
		if err := os.WriteFile(oldFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		err := client.Rename("old_name.txt", "new_name.txt")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Old file should not exist
		if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
			t.Error("Old file should not exist")
		}

		// New file should exist
		newFile := filepath.Join(tmpDir, "new_name.txt")
		data, err := os.ReadFile(newFile)
		if err != nil {
			t.Fatalf("Failed to read renamed file: %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Error("Content should be preserved after rename")
		}
	})

	t.Run("rename non-existent file", func(t *testing.T) {
		err := client.Rename("non_existent.txt", "dest.txt")
		if err == nil {
			t.Error("Expected error for non-existent source")
		}
	})
}

func TestLocalClient_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	t.Run("stat existing file", func(t *testing.T) {
		// Create file
		content := []byte("stat test content")
		testFile := filepath.Join(tmpDir, "stat_test.txt")
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		info, err := client.Stat("stat_test.txt")
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if info.Name() != "stat_test.txt" {
			t.Errorf("Name = %q, want %q", info.Name(), "stat_test.txt")
		}
		if info.Size() != int64(len(content)) {
			t.Errorf("Size = %d, want %d", info.Size(), len(content))
		}
		if info.IsDir() {
			t.Error("Expected file, got directory")
		}
	})

	t.Run("stat non-existent file", func(t *testing.T) {
		_, err := client.Stat("non_existent.txt")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

func TestLocalClient_Close(t *testing.T) {
	tmpDir := t.TempDir()
	client, err := NewClient(tmpDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestLimitReadCloser(t *testing.T) {
	t.Run("read limited content", func(t *testing.T) {
		content := []byte("0123456789ABCDEF")
		tmpFile, err := os.CreateTemp("", "limit_test")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(content); err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
		tmpFile.Close()

		file, err := os.Open(tmpFile.Name())
		if err != nil {
			t.Fatalf("Failed to open: %v", err)
		}

		lrc := &limitReadCloser{
			r: io.LimitReader(file, 5),
			c: file,
		}

		data, err := io.ReadAll(lrc)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if string(data) != "01234" {
			t.Errorf("Content = %q, want %q", data, "01234")
		}

		err = lrc.Close()
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	})
}
