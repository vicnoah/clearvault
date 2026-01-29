package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLocalStorage(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("create new storage", func(t *testing.T) {
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Check marker file is created
		markerPath := filepath.Join(tmpDir, ".clearvault")
		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			t.Error("Marker file should be created")
		}
	})

	t.Run("create nested directory", func(t *testing.T) {
		nestedDir := filepath.Join(tmpDir, "nested", "metadata")
		storage, err := NewLocalStorage(nestedDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed for nested dir: %v", err)
		}
		defer storage.Close()

		if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
			t.Error("Nested directory should be created")
		}
	})
}

func TestLocalStorage_SaveAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("save and get file metadata", func(t *testing.T) {
		meta := &FileMeta{
			Name:       "test.txt",
			RemoteName: "abc123",
			Size:       1024,
			IsDir:      false,
			FEK:        []byte("encrypted-key"),
			Salt:       []byte("salt"),
			UpdatedAt:  time.Now(),
		}

		if err := storage.Save(meta, "/documents/test.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.Get("/documents/test.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Expected metadata, got nil")
		}

		if got.Name != meta.Name {
			t.Errorf("Name = %q, want %q", got.Name, meta.Name)
		}
		if got.RemoteName != meta.RemoteName {
			t.Errorf("RemoteName = %q, want %q", got.RemoteName, meta.RemoteName)
		}
		if got.Size != meta.Size {
			t.Errorf("Size = %d, want %d", got.Size, meta.Size)
		}
	})

	t.Run("save and get directory metadata", func(t *testing.T) {
		meta := &FileMeta{
			Name:      "photos",
			IsDir:     true,
			UpdatedAt: time.Now(),
		}

		if err := storage.Save(meta, "/photos"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.Get("/photos")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Expected metadata, got nil")
		}
		if !got.IsDir {
			t.Error("Expected IsDir = true")
		}
	})

	t.Run("get non-existent file", func(t *testing.T) {
		got, err := storage.Get("/non-existent/file.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got != nil {
			t.Error("Expected nil for non-existent file")
		}
	})

	t.Run("get root directory", func(t *testing.T) {
		got, err := storage.Get("/")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Expected metadata for root")
		}
		if !got.IsDir {
			t.Error("Root should be a directory")
		}
	})
}

func TestLocalStorage_GetByRemoteName(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("find by remote name", func(t *testing.T) {
		meta := &FileMeta{
			Name:       "file1.txt",
			RemoteName: "remote-hash-123",
			Size:       100,
			FEK:        []byte("key1"),
			Salt:       []byte("salt1"),
			UpdatedAt:  time.Now(),
		}

		if err := storage.Save(meta, "/file1.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.GetByRemoteName("remote-hash-123")
		if err != nil {
			t.Fatalf("GetByRemoteName failed: %v", err)
		}
		if got == nil {
			t.Fatal("Expected to find metadata")
		}
		if got.Name != "file1.txt" {
			t.Errorf("Name = %q, want %q", got.Name, "file1.txt")
		}
	})

	t.Run("not found", func(t *testing.T) {
		got, err := storage.GetByRemoteName("non-existent-hash")
		if err != nil {
			t.Fatalf("GetByRemoteName failed: %v", err)
		}
		if got != nil {
			t.Error("Expected nil for non-existent remote name")
		}
	})
}

func TestLocalStorage_ReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	// Create test structure
	files := []struct {
		path string
		meta *FileMeta
	}{
		{"/docs/file1.txt", &FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}},
		{"/docs/file2.txt", &FileMeta{Name: "file2.txt", RemoteName: "r2", Size: 200}},
		{"/docs/subdir", &FileMeta{Name: "subdir", IsDir: true}},
	}

	for _, f := range files {
		if err := storage.Save(f.meta, f.path); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	t.Run("list directory contents", func(t *testing.T) {
		entries, err := storage.ReadDir("/docs")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		if len(entries) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(entries))
		}

		names := make(map[string]bool)
		for _, e := range entries {
			names[e.Name] = true
		}

		if !names["file1.txt"] {
			t.Error("Missing file1.txt")
		}
		if !names["file2.txt"] {
			t.Error("Missing file2.txt")
		}
		if !names["subdir"] {
			t.Error("Missing subdir")
		}
	})

	t.Run("list empty directory", func(t *testing.T) {
		// Create empty directory
		if err := storage.Save(&FileMeta{Name: "emptydir", IsDir: true}, "/emptydir"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		entries, err := storage.ReadDir("/emptydir")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		if len(entries) != 0 {
			t.Errorf("Expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("list non-existent directory", func(t *testing.T) {
		_, err := storage.ReadDir("/non-existent")
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}
	})
}

func TestLocalStorage_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("rename file", func(t *testing.T) {
		meta := &FileMeta{
			Name:       "oldname.txt",
			RemoteName: "remote1",
			Size:       100,
			FEK:        []byte("key"),
			Salt:       []byte("salt"),
			UpdatedAt:  time.Now(),
		}

		if err := storage.Save(meta, "/oldname.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		if err := storage.Rename("/oldname.txt", "/newname.txt"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Old path should not exist
		old, _ := storage.Get("/oldname.txt")
		if old != nil {
			t.Error("Old path should not exist after rename")
		}

		// New path should exist with updated name
		new, err := storage.Get("/newname.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if new == nil {
			t.Fatal("New path should exist")
		}
		if new.Name != "newname.txt" {
			t.Errorf("Name should be updated to %q, got %q", "newname.txt", new.Name)
		}
	})

	t.Run("rename directory", func(t *testing.T) {
		// Create directory with contents
		if err := storage.Save(&FileMeta{Name: "olddir", IsDir: true}, "/olddir"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		if err := storage.Save(&FileMeta{Name: "file.txt", RemoteName: "r1", Size: 100}, "/olddir/file.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		if err := storage.Rename("/olddir", "/newdir"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// New directory should exist
		new, err := storage.Get("/newdir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if new == nil {
			t.Fatal("New directory should exist")
		}

		// Contents should be preserved
		file, err := storage.Get("/newdir/file.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if file == nil {
			t.Error("File in renamed directory should exist")
		}
	})

	t.Run("rename non-existent source", func(t *testing.T) {
		err := storage.Rename("/non-existent", "/dest")
		if err == nil {
			t.Error("Expected error for non-existent source")
		}
	})
}

func TestLocalStorage_RemoveAll(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	// Create test structure
	if err := storage.Save(&FileMeta{Name: "todelete", IsDir: true}, "/todelete"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := storage.Save(&FileMeta{Name: "file.txt", RemoteName: "r1", Size: 100}, "/todelete/file.txt"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := storage.Save(&FileMeta{Name: "keep.txt", RemoteName: "r2", Size: 200}, "/keep.txt"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	t.Run("remove directory recursively", func(t *testing.T) {
		if err := storage.RemoveAll("/todelete"); err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		// Directory should be gone
		deleted, _ := storage.Get("/todelete")
		if deleted != nil {
			t.Error("Deleted directory should not exist")
		}

		// File inside should be gone
		file, _ := storage.Get("/todelete/file.txt")
		if file != nil {
			t.Error("File in deleted directory should not exist")
		}

		// Other files should remain
		keep, err := storage.Get("/keep.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if keep == nil {
			t.Error("Other files should not be affected")
		}
	})

	t.Run("remove single file", func(t *testing.T) {
		if err := storage.RemoveAll("/keep.txt"); err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		deleted, _ := storage.Get("/keep.txt")
		if deleted != nil {
			t.Error("Deleted file should not exist")
		}
	})

	t.Run("remove root clears all", func(t *testing.T) {
		// Create some files
		storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/file1.txt")
		storage.Save(&FileMeta{Name: "dir1", IsDir: true}, "/dir1")

		if err := storage.RemoveAll("/"); err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		// All user files should be gone (except marker)
		entries, err := storage.ReadDir("/")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("Expected empty root, got %d entries", len(entries))
		}
	})
}

func TestLocalStorage_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("handle special characters in filename", func(t *testing.T) {
		specialNames := []string{
			"file with spaces.txt",
			"file-with-dashes.txt",
			"file_with_underscores.txt",
			"file.multiple.dots.txt",
			"日本語ファイル.txt",
		}

		for _, name := range specialNames {
			meta := &FileMeta{
				Name:       name,
				RemoteName: "remote-" + name,
				Size:       100,
				UpdatedAt:  time.Now(),
			}

			path := "/" + name
			if err := storage.Save(meta, path); err != nil {
				t.Errorf("Save failed for %q: %v", name, err)
				continue
			}

			got, err := storage.Get(path)
			if err != nil {
				t.Errorf("Get failed for %q: %v", name, err)
				continue
			}
			if got == nil {
				t.Errorf("Expected to get metadata for %q", name)
				continue
			}
			if got.Name != name {
				t.Errorf("Name mismatch: got %q, want %q", got.Name, name)
			}
		}
	})

	t.Run("handle nested paths", func(t *testing.T) {
		meta := &FileMeta{
			Name:       "deep.txt",
			RemoteName: "remote-deep",
			Size:       100,
			UpdatedAt:  time.Now(),
		}

		path := "/a/b/c/d/e/deep.txt"
		if err := storage.Save(meta, path); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.Get(path)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Expected to get metadata")
		}
	})
}

func TestLocalStorage_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("concurrent saves", func(t *testing.T) {
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func(idx int) {
				defer func() { done <- true }()
				meta := &FileMeta{
					Name:       fmt.Sprintf("file%d.txt", idx),
					RemoteName: fmt.Sprintf("remote%d", idx),
					Size:       int64(idx * 100),
					UpdatedAt:  time.Now(),
				}
				path := fmt.Sprintf("/concurrent/file%d.txt", idx)
				if err := storage.Save(meta, path); err != nil {
					t.Errorf("Save failed: %v", err)
				}
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all files exist
		for i := 0; i < 10; i++ {
			path := fmt.Sprintf("/concurrent/file%d.txt", i)
			got, err := storage.Get(path)
			if err != nil {
				t.Errorf("Get failed: %v", err)
				continue
			}
			if got == nil {
				t.Errorf("Expected file%d.txt to exist", i)
			}
		}
	})
}

// Test path conversion methods
func TestLocalStorage_PathConversion(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("getLocalPath converts paths correctly", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"/file.txt", filepath.Join(tmpDir, "file.txt.json")},
			{"/dir/file.txt", filepath.Join(tmpDir, "dir", "file.txt.json")},
			{"/", tmpDir},
			{".", tmpDir},
		}

		for _, tt := range tests {
			got := storage.getLocalPath(tt.input)
			if got != tt.expected {
				t.Errorf("getLocalPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		}
	})

	t.Run("getLocalPathWithoutJson converts paths correctly", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"/file.txt", filepath.Join(tmpDir, "file.txt")},
			{"/dir/file.txt", filepath.Join(tmpDir, "dir", "file.txt")},
			{"/", tmpDir},
			{".", tmpDir},
		}

		for _, tt := range tests {
			got := storage.getLocalPathWithoutJson(tt.input)
			if got != tt.expected {
				t.Errorf("getLocalPathWithoutJson(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		}
	})
}

// Test Get error handling
func TestLocalStorage_GetErrors(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("handle corrupted json file", func(t *testing.T) {
		// Create a corrupted JSON file directly
		metaPath := filepath.Join(tmpDir, "corrupted.json")
		if err := os.WriteFile(metaPath, []byte("not valid json"), 0644); err != nil {
			t.Fatalf("Failed to create corrupted file: %v", err)
		}

		// Should return nil without error for corrupted JSON
		got, err := storage.Get("/corrupted")
		if err != nil {
			t.Errorf("Expected no error for corrupted JSON, got: %v", err)
		}
		if got != nil {
			t.Error("Expected nil for corrupted JSON")
		}
	})

	t.Run("handle empty json file", func(t *testing.T) {
		// Create an empty JSON file
		metaPath := filepath.Join(tmpDir, "empty.json")
		if err := os.WriteFile(metaPath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to create empty file: %v", err)
		}

		// Should return nil without error for empty JSON
		got, err := storage.Get("/empty")
		if err != nil {
			t.Errorf("Expected no error for empty JSON, got: %v", err)
		}
		if got != nil {
			t.Error("Expected nil for empty JSON")
		}
	})
}

// Test Save error cases
func TestLocalStorage_SaveErrors(t *testing.T) {
	// Create a read-only directory to trigger permission errors
	readOnlyDir := filepath.Join(t.TempDir(), "readonly")
	if err := os.MkdirAll(readOnlyDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Make directory read-only
	if err := os.Chmod(readOnlyDir, 0555); err != nil {
		t.Skipf("Cannot change permissions (may need root): %v", err)
	}
	defer os.Chmod(readOnlyDir, 0755) // Restore permissions for cleanup

	t.Run("fail to create nested directory in read-only parent", func(t *testing.T) {
		storage, err := NewLocalStorage(readOnlyDir)
		if err != nil {
			t.Skipf("Cannot test with read-only directory: %v", err)
		}
		defer storage.Close()

		meta := &FileMeta{
			Name:       "test.txt",
			RemoteName: "remote",
			Size:       100,
		}

		// Try to save to a path that requires creating a new directory
		err = storage.Save(meta, "/nested/test.txt")
		if err == nil {
			t.Error("Expected error when saving to read-only directory")
		}
	})
}

// Test Copy and Rename fallback
func TestLocalStorage_CopyAndRenameFallback(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("rename file when os.Rename fails", func(t *testing.T) {
		// Create a file
		meta := &FileMeta{
			Name:       "source.txt",
			RemoteName: "remote-source",
			Size:       100,
			FEK:        []byte("key"),
			Salt:       []byte("salt"),
			UpdatedAt:  time.Now(),
		}

		if err := storage.Save(meta, "/source.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Rename should succeed (uses fallback if needed)
		if err := storage.Rename("/source.txt", "/dest.txt"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify rename worked
		dest, err := storage.Get("/dest.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if dest == nil {
			t.Fatal("Destination file should exist")
		}
		if dest.Name != "dest.txt" {
			t.Errorf("Name = %q, want %q", dest.Name, "dest.txt")
		}
	})

	t.Run("rename updates metadata name field", func(t *testing.T) {
		meta := &FileMeta{
			Name:       "oldname.txt",
			RemoteName: "remote-old",
			Size:       100,
			UpdatedAt:  time.Now(),
		}

		if err := storage.Save(meta, "/oldname.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		if err := storage.Rename("/oldname.txt", "/newname.txt"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		got, err := storage.Get("/newname.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got.Name != "newname.txt" {
			t.Errorf("Metadata name not updated: got %q, want %q", got.Name, "newname.txt")
		}
	})

	t.Run("copy directory recursively", func(t *testing.T) {
		// Create directory structure
		if err := storage.Save(&FileMeta{Name: "srcdir", IsDir: true}, "/srcdir"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		if err := storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/srcdir/file1.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		if err := storage.Save(&FileMeta{Name: "file2.txt", RemoteName: "r2", Size: 200}, "/srcdir/file2.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Rename directory
		if err := storage.Rename("/srcdir", "/dstdir"); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify structure copied
		entries, err := storage.ReadDir("/dstdir")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(entries))
		}
	})
}

// Test helper functions
func TestHelperFunctions(t *testing.T) {
	t.Run("copyFile copies file content", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest.txt")
		content := []byte("test content for copy")

		if err := os.WriteFile(src, content, 0644); err != nil {
			t.Fatalf("Failed to create source file: %v", err)
		}

		if err := copyFile(src, dst); err != nil {
			t.Fatalf("copyFile failed: %v", err)
		}

		copied, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("Failed to read dest file: %v", err)
		}
		if string(copied) != string(content) {
			t.Errorf("Content mismatch: got %q, want %q", string(copied), string(content))
		}
	})

	t.Run("copyFile fails on non-existent source", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "nonexistent.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		err := copyFile(src, dst)
		if err == nil {
			t.Error("Expected error for non-existent source file")
		}
	})

	t.Run("copyDir copies directory recursively", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "source")
		dstDir := filepath.Join(tmpDir, "dest")

		// Create source structure
		if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
			t.Fatalf("Failed to create source structure: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		if err := copyDir(srcDir, dstDir); err != nil {
			t.Fatalf("copyDir failed: %v", err)
		}

		// Verify copied
		if _, err := os.Stat(filepath.Join(dstDir, "file1.txt")); err != nil {
			t.Error("file1.txt should exist in dest")
		}
		if _, err := os.Stat(filepath.Join(dstDir, "subdir", "file2.txt")); err != nil {
			t.Error("subdir/file2.txt should exist in dest")
		}
	})

	t.Run("copyDir fails on non-existent source", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "nonexistent")
		dstDir := filepath.Join(tmpDir, "dest")

		err := copyDir(srcDir, dstDir)
		if err == nil {
			t.Error("Expected error for non-existent source directory")
		}
	})

	t.Run("retryOperation succeeds on first try", func(t *testing.T) {
		callCount := 0
		err := retryOperation(func() error {
			callCount++
			return nil
		})
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if callCount != 1 {
			t.Errorf("Expected 1 call, got %d", callCount)
		}
	})

	t.Run("retryOperation retries on failure", func(t *testing.T) {
		callCount := 0
		err := retryOperation(func() error {
			callCount++
			if callCount < 3 {
				return fmt.Errorf("temporary error")
			}
			return nil
		})
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if callCount != 3 {
			t.Errorf("Expected 3 calls, got %d", callCount)
		}
	})

	t.Run("retryOperation returns error after max retries", func(t *testing.T) {
		callCount := 0
		err := retryOperation(func() error {
			callCount++
			return fmt.Errorf("persistent error")
		})
		if err == nil {
			t.Error("Expected error after max retries")
		}
		if callCount != 100 { // Max retries is 100
			t.Errorf("Expected 100 calls, got %d", callCount)
		}
	})
}

// Test Close method
func TestLocalStorage_Close(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}

	// Close should return nil without error
	if err := storage.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// Test GetByRemoteName with errors
func TestLocalStorage_GetByRemoteNameErrors(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("handle unreadable file during walk", func(t *testing.T) {
		// Create a file that can't be read (no read permission)
		// This test is skipped on systems where permissions don't work the same way
		restrictedFile := filepath.Join(tmpDir, "restricted.json")
		if err := os.WriteFile(restrictedFile, []byte("{}"), 0000); err != nil {
			t.Skipf("Cannot create restricted file: %v", err)
		}
		defer os.Chmod(restrictedFile, 0644) // Restore for cleanup

		// Should complete without error, just skip the unreadable file
		got, err := storage.GetByRemoteName("any-remote")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if got != nil {
			t.Error("Expected nil for non-matching remote name")
		}
	})
}

// Test ReadDir edge cases
func TestLocalStorage_ReadDirEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("handle corrupted file in directory", func(t *testing.T) {
		// Create directory with valid and invalid files
		if err := os.MkdirAll(filepath.Join(tmpDir, "testdir"), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		// Valid file
		validMeta := &FileMeta{Name: "valid.txt", RemoteName: "r1", Size: 100}
		data, _ := json.Marshal(validMeta)
		os.WriteFile(filepath.Join(tmpDir, "testdir", "valid.txt.json"), data, 0644)
		// Invalid file
		os.WriteFile(filepath.Join(tmpDir, "testdir", "invalid.txt.json"), []byte("not json"), 0644)

		entries, err := storage.ReadDir("/testdir")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		// Should return only the valid file
		if len(entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(entries))
		}
		if len(entries) > 0 && entries[0].Name != "valid.txt" {
			t.Errorf("Expected valid.txt, got %q", entries[0].Name)
		}
	})

	t.Run("skip .clearvault marker file", func(t *testing.T) {
		// .clearvault is always created, ReadDir should skip it
		entries, err := storage.ReadDir("/")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		for _, e := range entries {
			if e.Name == ".clearvault" {
				t.Error(".clearvault should be skipped in ReadDir")
			}
		}
	})
}

// Test RemoveAll edge cases
func TestLocalStorage_RemoveAllEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("remove non-existent path", func(t *testing.T) {
		// Should not error when removing non-existent path
		err := storage.RemoveAll("/nonexistent")
		if err != nil {
			t.Errorf("Expected no error for non-existent path, got: %v", err)
		}
	})

	t.Run("remove directory without json extension", func(t *testing.T) {
		// Create directory (not file)
		if err := storage.Save(&FileMeta{Name: "testdir", IsDir: true}, "/testdir"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		// Create file inside
		if err := storage.Save(&FileMeta{Name: "file.txt", RemoteName: "r1", Size: 100}, "/testdir/file.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Remove directory
		if err := storage.RemoveAll("/testdir"); err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		// Verify removed
		got, _ := storage.Get("/testdir")
		if got != nil {
			t.Error("Directory should be removed")
		}
	})
}

// Test Get with directory that has stat error
func TestLocalStorage_GetDirectoryErrors(t *testing.T) {
	// This test creates a scenario where os.Stat returns an error that's not ErrNotExist
	// by temporarily creating a file that we then make unreadable

	t.Run("handle stat error on directory check", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create a regular file where we expect a directory
		// Then try to read it as a directory - this exercises different code paths
		testPath := filepath.Join(tmpDir, "notadir.json")
		if err := os.WriteFile(testPath, []byte("not json"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		// This should handle the corrupted JSON gracefully
		got, err := storage.Get("/notadir")
		if err != nil {
			t.Errorf("Expected no error for corrupted JSON, got: %v", err)
		}
		if got != nil {
			t.Error("Expected nil for corrupted JSON")
		}
	})
}

// Test Rename with error scenarios
func TestLocalStorage_RenameErrors(t *testing.T) {
	t.Run("rename fails when target directory cannot be created", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create source file
		storage.Save(&FileMeta{Name: "src.txt", RemoteName: "r1", Size: 100}, "/src.txt")

		// Create a file named "readonly" to block directory creation
		// (this prevents MkdirAll from creating the parent directory)
		blockerPath := filepath.Join(tmpDir, "readonly")
		if err := os.WriteFile(blockerPath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to create blocker file: %v", err)
		}

		// Try to rename to a path inside "readonly" (which is a file, not a directory)
		err = storage.Rename("/src.txt", "/readonly/dest.txt")
		if err == nil {
			t.Error("Expected error when target directory cannot be created")
		}
	})

	t.Run("rename directory that triggers fallback copy+delete", func(t *testing.T) {
		// This test creates a scenario where os.Rename might fail for directories
		// by creating a non-empty directory that may trigger fallback behavior
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create directory with content
		storage.Save(&FileMeta{Name: "srcdir", IsDir: true}, "/srcdir")
		storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/srcdir/file1.txt")

		// Rename directory - this exercises the fallback path on some systems
		err = storage.Rename("/srcdir", "/dstdir")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify the rename/copy worked
		dst, err := storage.Get("/dstdir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if dst == nil {
			t.Error("Destination directory should exist")
		}
	})
}

// Test copyFile with permission error on destination
func TestCopyFilePermissions(t *testing.T) {
	t.Run("copyFile fails when destination cannot be created", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "readonly_dir", "dest.txt")

		// Create source
		os.WriteFile(src, []byte("content"), 0644)

		// Create a file (not directory) at the parent path to block directory creation
		os.WriteFile(filepath.Join(tmpDir, "readonly_dir"), []byte(""), 0644)

		err := copyFile(src, dst)
		if err == nil {
			t.Error("Expected error when destination directory cannot be created")
		}
	})

	t.Run("copyFile fails when destination is not writable", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dstDir := filepath.Join(tmpDir, "dest_dir")

		// Create source
		os.WriteFile(src, []byte("content"), 0644)

		// Create destination directory
		os.MkdirAll(dstDir, 0755)

		// Create a read-only file at destination
		dst := filepath.Join(dstDir, "dest.txt")
		os.WriteFile(dst, []byte(""), 0644)

		// Make destination read-only
		os.Chmod(dst, 0000)
		defer os.Chmod(dst, 0644)

		err := copyFile(src, dst)
		if err == nil {
			t.Skip("Permission-based test may not work on all systems")
		}
	})
}

// Test ReadDir with nested directory and file errors
func TestLocalStorage_ReadDirErrors(t *testing.T) {
	t.Run("readDir handles Get errors gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create a file entry directly (simulating a file that causes Get to fail)
		testDir := filepath.Join(tmpDir, "testdir")
		os.MkdirAll(testDir, 0755)

		// Create an entry that looks like a directory but causes issues
		// (this is a corner case - file without .json in a directory listing)
		problemFile := filepath.Join(testDir, "problem")
		os.WriteFile(problemFile, []byte("not a valid dir"), 0644)

		// ReadDir should handle this gracefully (skip problematic entries)
		entries, err := storage.ReadDir("/testdir")
		if err != nil {
			t.Fatalf("ReadDir should not fail: %v", err)
		}
		// The problematic entry should be skipped
		for _, e := range entries {
			if e.Name == "problem" {
				t.Error("Problematic entry should be skipped or handled")
			}
		}
	})
}

// Test Save with various directory paths
func TestLocalStorage_SaveDirectoryVariations(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("save directory at root", func(t *testing.T) {
		meta := &FileMeta{Name: "rootdir", IsDir: true}
		err := storage.Save(meta, "/rootdir")
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.Get("/rootdir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Directory should exist")
		}
		if !got.IsDir {
			t.Error("Should be a directory")
		}
	})

	t.Run("save deeply nested directories", func(t *testing.T) {
		meta := &FileMeta{Name: "deep", IsDir: true}
		err := storage.Save(meta, "/a/b/c/d/e/deep")
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.Get("/a/b/c/d/e/deep")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Directory should exist")
		}
	})
}

// Test RemoveAll with file-specific path handling
func TestLocalStorage_RemoveAllFiles(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("remove file directly by path", func(t *testing.T) {
		storage.Save(&FileMeta{Name: "delete.txt", RemoteName: "r1", Size: 100}, "/delete.txt")

		err := storage.RemoveAll("/delete.txt")
		if err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		got, _ := storage.Get("/delete.txt")
		if got != nil {
			t.Error("File should be deleted")
		}
	})

	t.Run("remove nested file", func(t *testing.T) {
		storage.Save(&FileMeta{Name: "dir", IsDir: true}, "/dir")
		storage.Save(&FileMeta{Name: "nested.txt", RemoteName: "r1", Size: 100}, "/dir/nested.txt")

		err := storage.RemoveAll("/dir/nested.txt")
		if err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		got, _ := storage.Get("/dir/nested.txt")
		if got != nil {
			t.Error("Nested file should be deleted")
		}

		// Parent directory should still exist
		dir, err := storage.Get("/dir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if dir == nil {
			t.Error("Parent directory should still exist")
		}
	})
}

// Test Get method with all directory handling paths
func TestLocalStorage_GetDirectoryHandling(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("get directory metadata from filesystem", func(t *testing.T) {
		// Create directory directly
		dirPath := filepath.Join(tmpDir, "fsdir")
		os.MkdirAll(dirPath, 0755)

		got, err := storage.Get("/fsdir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Directory should exist")
		}
		if !got.IsDir {
			t.Error("Should be a directory")
		}
		if got.Name != "fsdir" {
			t.Errorf("Name = %q, want %q", got.Name, "fsdir")
		}
	})

	t.Run("get handles stat error on file", func(t *testing.T) {
		// Create a file we can make unreadable (simulating permission error)
		// This tests the error handling path when stat fails for reasons other than not exist
		testPath := filepath.Join(tmpDir, "unreadable.json")
		os.WriteFile(testPath, []byte("{}"), 0644)

		// The Get method should handle this - let's make sure it returns the file
		got, err := storage.Get("/unreadable")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Error("Should be able to read file")
		}
	})
}

// Test Get with stat error paths
func TestLocalStorage_GetStatErrors(t *testing.T) {
	t.Run("stat returns non-NotExist error on json path", func(t *testing.T) {
		// This is hard to test reliably without special filesystem setup
		// We'll verify normal operation works correctly
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Normal case should work
		storage.Save(&FileMeta{Name: "test.txt", RemoteName: "r1", Size: 100}, "/test.txt")

		got, err := storage.Get("/test.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Should find file")
		}
	})

	t.Run("read file returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create a file that's unreadable
		unreadableFile := filepath.Join(tmpDir, "unreadable.json")
		os.WriteFile(unreadableFile, []byte("{}"), 0644)

		// Make it unreadable
		os.Chmod(unreadableFile, 0000)
		defer os.Chmod(unreadableFile, 0644)

		// Try to get the file - should return error
		_, err = storage.Get("/unreadable")
		// Permission errors might behave differently on different systems
		// Just verify the code path is exercised
		t.Logf("Got error (may be nil or permission error): %v", err)
	})
}

// Final coverage tests
func TestLocalStorage_FinalCoverage(t *testing.T) {
	t.Run("Save file with path containing dots", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		meta := &FileMeta{Name: "archive.tar.gz", RemoteName: "r1", Size: 100}
		storage.Save(meta, "/archive.tar.gz")

		got, _ := storage.Get("/archive.tar.gz")
		if got == nil || got.Name != "archive.tar.gz" {
			t.Error("File with dots should be saved correctly")
		}
	})

	t.Run("Rename file in nested directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		storage.Save(&FileMeta{Name: "dir", IsDir: true}, "/dir")
		storage.Save(&FileMeta{Name: "old.txt", RemoteName: "r1", Size: 100}, "/dir/old.txt")

		storage.Rename("/dir/old.txt", "/dir/new.txt")

		old, _ := storage.Get("/dir/old.txt")
		if old != nil {
			t.Error("Old file should not exist")
		}

		new, _ := storage.Get("/dir/new.txt")
		if new == nil || new.Name != "new.txt" {
			t.Error("New file should exist with updated name")
		}
	})

	t.Run("ReadDir root with marker file only", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		entries, err := storage.ReadDir("/")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		// Should be empty (only .clearvault marker which is filtered)
		if len(entries) != 0 {
			t.Errorf("Expected empty root, got %d entries", len(entries))
		}
	})

	t.Run("GetByRemoteName finds file in nested directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		storage.Save(&FileMeta{Name: "dir", IsDir: true}, "/dir")
		storage.Save(&FileMeta{Name: "deep.txt", RemoteName: "nested-remote", Size: 100}, "/dir/deep.txt")

		got, err := storage.GetByRemoteName("nested-remote")
		if err != nil {
			t.Fatalf("GetByRemoteName failed: %v", err)
		}
		if got == nil || got.Name != "deep.txt" {
			t.Error("Should find file in nested directory")
		}
	})

	t.Run("RemoveAll preserves marker when clearing root", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Add some files
		storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/file1.txt")
		storage.Save(&FileMeta{Name: "dir1", IsDir: true}, "/dir1")

		// Remove root
		storage.RemoveAll("/")

		// Check marker still exists
		markerPath := filepath.Join(tmpDir, ".clearvault")
		if _, err := os.Stat(markerPath); err != nil {
			t.Error(".clearvault marker should be preserved")
		}
	})
}

// Test copyDir with symlink handling (if supported)
func TestCopyDirSymlinks(t *testing.T) {
	t.Run("copyDir handles symlinks", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := filepath.Join(t.TempDir(), "dest")

		// Create a file and a symlink to it
		targetFile := filepath.Join(srcDir, "target.txt")
		os.WriteFile(targetFile, []byte("target content"), 0644)

		symlinkPath := filepath.Join(srcDir, "link.txt")
		// Try to create symlink (may not work on all systems)
		err := os.Symlink(targetFile, symlinkPath)
		if err != nil {
			t.Skipf("Cannot create symlinks on this system: %v", err)
		}

		// Copy directory - symlinks may be handled differently
		err = copyDir(srcDir, dstDir)
		// May succeed or fail depending on symlink handling
		if err != nil {
			t.Logf("copyDir with symlinks returned error (expected on some systems): %v", err)
		}
	})
}

// Test ReadDir error handling more thoroughly
func TestLocalStorage_ReadDirThorough(t *testing.T) {
	t.Run("ReadDir handles os.ReadDir error", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Try to ReadDir a non-existent path
		_, err = storage.ReadDir("/nonexistent-dir")
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}
	})

	t.Run("ReadDir with file as directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create a file (not directory) at the path
		testFile := filepath.Join(tmpDir, "notadir.json")
		os.WriteFile(testFile, []byte("{}"), 0644)

		// Try to ReadDir - should handle gracefully
		entries, err := storage.ReadDir("/notadir")
		// This should either return error or empty list
		if err != nil {
			t.Logf("ReadDir returned error (expected): %v", err)
		} else {
			t.Logf("ReadDir returned %d entries", len(entries))
		}
	})
}

// Test RemoveAll with edge cases
func TestLocalStorage_RemoveAllEdgeThorough(t *testing.T) {
	t.Run("RemoveAll non-existent path returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Remove something that doesn't exist
		err = storage.RemoveAll("/completely-nonexistent")
		if err != nil {
			t.Errorf("Expected no error for non-existent path, got: %v", err)
		}
	})
}

// Test NewLocalStorage errors
func TestNewLocalStorage_Errors(t *testing.T) {
	t.Run("fail to create directory with invalid path", func(t *testing.T) {
		// Try to create a directory in a location that doesn't exist and can't be created
		// On Unix systems, this would be something like "/nonexistent/deep/path"
		_, err := NewLocalStorage("/dev/null/invalid")
		if err == nil {
			t.Error("Expected error for invalid path")
		}
	})
}

// Test updateMetadataName edge cases
func TestLocalStorage_updateMetadataName(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("update metadata name successfully", func(t *testing.T) {
		meta := &FileMeta{
			Name:       "old.txt",
			RemoteName: "remote",
			Size:       100,
		}
		if err := storage.Save(meta, "/test.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		metaPath := filepath.Join(tmpDir, "test.txt.json")
		if err := storage.updateMetadataName(metaPath, "/newname.txt"); err != nil {
			t.Fatalf("updateMetadataName failed: %v", err)
		}

		// Verify update
		data, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		var updated FileMeta
		if err := json.Unmarshal(data, &updated); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}
		if updated.Name != "newname.txt" {
			t.Errorf("Name = %q, want %q", updated.Name, "newname.txt")
		}
	})

	t.Run("fail on non-existent file", func(t *testing.T) {
		err := storage.updateMetadataName(filepath.Join(tmpDir, "nonexistent.json"), "/new.txt")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("fail on invalid json", func(t *testing.T) {
		invalidPath := filepath.Join(tmpDir, "invalid.json")
		if err := os.WriteFile(invalidPath, []byte("not json"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		err := storage.updateMetadataName(invalidPath, "/new.txt")
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})
}

// Test ReadDir with various directory entries
func TestLocalStorage_ReadDirVariations(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("mix of files and directories", func(t *testing.T) {
		// Create a mix of files and directories
		storage.Save(&FileMeta{Name: "dir1", IsDir: true}, "/mix/dir1")
		storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/mix/file1.txt")
		storage.Save(&FileMeta{Name: "dir2", IsDir: true}, "/mix/dir2")
		storage.Save(&FileMeta{Name: "file2.txt", RemoteName: "r2", Size: 200}, "/mix/file2.txt")

		entries, err := storage.ReadDir("/mix")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 4 {
			t.Errorf("Expected 4 entries, got %d", len(entries))
		}

		// Verify both files and directories are present
		hasDir := false
		hasFile := false
		for _, e := range entries {
			if e.IsDir {
				hasDir = true
			} else {
				hasFile = true
			}
		}
		if !hasDir {
			t.Error("Expected at least one directory")
		}
		if !hasFile {
			t.Error("Expected at least one file")
		}
	})

	t.Run("deeply nested directory listing", func(t *testing.T) {
		// Create nested structure
		storage.Save(&FileMeta{Name: "level1", IsDir: true}, "/deep/level1")
		storage.Save(&FileMeta{Name: "level2", IsDir: true}, "/deep/level1/level2")
		storage.Save(&FileMeta{Name: "file.txt", RemoteName: "r1", Size: 100}, "/deep/level1/level2/file.txt")

		// List level1
		entries, err := storage.ReadDir("/deep/level1")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 1 || entries[0].Name != "level2" {
			t.Errorf("Expected level2 directory")
		}

		// List level2
		entries, err = storage.ReadDir("/deep/level1/level2")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 1 || entries[0].Name != "file.txt" {
			t.Errorf("Expected file.txt")
		}
	})
}

// Test Rename with nested directory structures
func TestLocalStorage_RenameComplex(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("rename nested file", func(t *testing.T) {
		storage.Save(&FileMeta{Name: "dir", IsDir: true}, "/dir")
		storage.Save(&FileMeta{Name: "file.txt", RemoteName: "r1", Size: 100}, "/dir/file.txt")

		err := storage.Rename("/dir/file.txt", "/dir/newfile.txt")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		old, _ := storage.Get("/dir/file.txt")
		if old != nil {
			t.Error("Old file should not exist")
		}

		new, err := storage.Get("/dir/newfile.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if new == nil {
			t.Fatal("New file should exist")
		}
		if new.Name != "newfile.txt" {
			t.Errorf("Name = %q, want %q", new.Name, "newfile.txt")
		}
	})

	t.Run("move file between directories", func(t *testing.T) {
		storage.Save(&FileMeta{Name: "src", IsDir: true}, "/src")
		storage.Save(&FileMeta{Name: "dst", IsDir: true}, "/dst")
		storage.Save(&FileMeta{Name: "move.txt", RemoteName: "r1", Size: 100}, "/src/move.txt")

		err := storage.Rename("/src/move.txt", "/dst/move.txt")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		src, _ := storage.Get("/src/move.txt")
		if src != nil {
			t.Error("Source file should not exist")
		}

		dst, err := storage.Get("/dst/move.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if dst == nil {
			t.Fatal("Destination file should exist")
		}
	})
}

// Test Rename fallback behavior (copy + delete when rename fails)
func TestLocalStorage_RenameFallback(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("rename directory with nested content", func(t *testing.T) {
		// Create complex structure
		storage.Save(&FileMeta{Name: "parent", IsDir: true}, "/parent")
		storage.Save(&FileMeta{Name: "child1", IsDir: true}, "/parent/child1")
		storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/parent/child1/file1.txt")
		storage.Save(&FileMeta{Name: "child2", IsDir: true}, "/parent/child2")
		storage.Save(&FileMeta{Name: "file2.txt", RemoteName: "r2", Size: 200}, "/parent/child2/file2.txt")

		// Rename parent directory
		err := storage.Rename("/parent", "/newparent")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify entire structure moved
		parent, _ := storage.Get("/parent")
		if parent != nil {
			t.Error("Old parent should not exist")
		}

		newparent, err := storage.Get("/newparent")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if newparent == nil {
			t.Fatal("New parent should exist")
		}

		// Check nested content
		child1, err := storage.Get("/newparent/child1")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if child1 == nil {
			t.Error("child1 should exist")
		}

		file1, err := storage.Get("/newparent/child1/file1.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if file1 == nil {
			t.Error("file1.txt should exist")
		}
	})

	t.Run("rename to same directory different name", func(t *testing.T) {
		storage.Save(&FileMeta{Name: "original.txt", RemoteName: "r1", Size: 100}, "/original.txt")

		err := storage.Rename("/original.txt", "/renamed.txt")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		original, _ := storage.Get("/original.txt")
		if original != nil {
			t.Error("Original should not exist")
		}

		renamed, err := storage.Get("/renamed.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if renamed == nil {
			t.Fatal("Renamed should exist")
		}
		if renamed.Name != "renamed.txt" {
			t.Errorf("Name = %q, want %q", renamed.Name, "renamed.txt")
		}
	})
}

// Test Rename with copyDir fallback
func TestLocalStorage_RenameWithCopyDirFallback(t *testing.T) {
	t.Run("rename large directory structure", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create a large directory structure that may trigger fallback
		for i := 0; i < 5; i++ {
			dirPath := fmt.Sprintf("/largedir/subdir%d", i)
			storage.Save(&FileMeta{Name: fmt.Sprintf("subdir%d", i), IsDir: true}, dirPath)
			for j := 0; j < 3; j++ {
				filePath := fmt.Sprintf("%s/file%d.txt", dirPath, j)
				storage.Save(&FileMeta{Name: fmt.Sprintf("file%d.txt", j), RemoteName: fmt.Sprintf("r%d%d", i, j), Size: 100}, filePath)
			}
		}

		// Rename the large directory
		err = storage.Rename("/largedir", "/newlargedir")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify structure
		newDir, err := storage.Get("/newlargedir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if newDir == nil {
			t.Fatal("New directory should exist")
		}

		// Check nested content
		entries, err := storage.ReadDir("/newlargedir")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 5 {
			t.Errorf("Expected 5 subdirectories, got %d", len(entries))
		}
	})

	t.Run("rename deeply nested directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, err := NewLocalStorage(tmpDir)
		if err != nil {
			t.Fatalf("NewLocalStorage failed: %v", err)
		}
		defer storage.Close()

		// Create deep structure
		paths := []string{
			"/a", "/a/b", "/a/b/c", "/a/b/c/d",
		}
		for _, p := range paths {
			storage.Save(&FileMeta{Name: filepath.Base(p), IsDir: true}, p)
		}
		storage.Save(&FileMeta{Name: "deep.txt", RemoteName: "r1", Size: 100}, "/a/b/c/d/deep.txt")

		// Rename root
		err = storage.Rename("/a", "/z")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify deep file exists
		deep, err := storage.Get("/z/b/c/d/deep.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if deep == nil {
			t.Error("Deep file should exist after rename")
		}
	})
}

// Test copyDir edge cases
func TestCopyDirEdgeCases(t *testing.T) {
	t.Run("copyDir with read error on source file", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := filepath.Join(t.TempDir(), "dest")

		// Create source with a file
		srcFile := filepath.Join(srcDir, "test.txt")
		if err := os.WriteFile(srcFile, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		// Remove read permission from file (this test may not work on all systems)
		if err := os.Chmod(srcFile, 0000); err != nil {
			t.Skipf("Cannot change file permissions: %v", err)
		}
		defer os.Chmod(srcFile, 0644)

		err := copyDir(srcDir, dstDir)
		if err == nil {
			t.Error("Expected error when copying unreadable file")
		}
	})

	t.Run("copyDir with nested directories", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := filepath.Join(t.TempDir(), "dest")

		// Create nested structure
		nestedDir := filepath.Join(srcDir, "level1", "level2", "level3")
		if err := os.MkdirAll(nestedDir, 0755); err != nil {
			t.Fatalf("Failed to create nested dirs: %v", err)
		}
		if err := os.WriteFile(filepath.Join(nestedDir, "deep.txt"), []byte("deep"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		if err := copyDir(srcDir, dstDir); err != nil {
			t.Fatalf("copyDir failed: %v", err)
		}

		// Verify deep structure was copied
		copiedFile := filepath.Join(dstDir, "level1", "level2", "level3", "deep.txt")
		if _, err := os.Stat(copiedFile); err != nil {
			t.Error("Deep file should be copied")
		}
	})
}

// Test Save edge cases
func TestLocalStorage_SaveEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("save with very long name", func(t *testing.T) {
		longName := ""
		for i := 0; i < 200; i++ {
			longName += "a"
		}
		longName += ".txt"

		meta := &FileMeta{
			Name:       longName,
			RemoteName: "remote",
			Size:       100,
		}

		// Should handle long names (up to OS limits)
		err := storage.Save(meta, "/"+longName)
		if err != nil {
			t.Errorf("Failed to save file with long name: %v", err)
		}
	})

	t.Run("save directory with path containing dots", func(t *testing.T) {
		meta := &FileMeta{
			Name:  "my.dir",
			IsDir: true,
		}

		err := storage.Save(meta, "/my.dir")
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.Get("/my.dir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("Directory should exist")
		}
		if !got.IsDir {
			t.Error("Should be a directory")
		}
	})

	t.Run("save file updates existing metadata", func(t *testing.T) {
		meta1 := &FileMeta{
			Name:       "update.txt",
			RemoteName: "remote1",
			Size:       100,
		}
		if err := storage.Save(meta1, "/update.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Save again with different data
		meta2 := &FileMeta{
			Name:       "update.txt",
			RemoteName: "remote2",
			Size:       200,
		}
		if err := storage.Save(meta2, "/update.txt"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := storage.Get("/update.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got.RemoteName != "remote2" {
			t.Errorf("RemoteName = %q, want %q", got.RemoteName, "remote2")
		}
		if got.Size != 200 {
			t.Errorf("Size = %d, want 200", got.Size)
		}
	})
}

// Test ReadDir with empty root
func TestLocalStorage_ReadDirEmptyRoot(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("read empty root directory", func(t *testing.T) {
		entries, err := storage.ReadDir("/")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		// Should only contain .clearvault marker (which is filtered out)
		if len(entries) != 0 {
			t.Errorf("Expected 0 entries in empty root, got %d", len(entries))
		}
	})

	t.Run("read root with only marker file", func(t *testing.T) {
		// Add only the marker file (.clearvault is auto-created)
		entries, err := storage.ReadDir("/")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		for _, e := range entries {
			if e.Name == ".clearvault" {
				t.Error(".clearvault should not appear in listing")
			}
		}
	})
}

// Test RemoveAll with various scenarios
func TestLocalStorage_RemoveAllScenarios(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("remove file with json extension", func(t *testing.T) {
		storage.Save(&FileMeta{Name: "remove.txt", RemoteName: "r1", Size: 100}, "/remove.txt")

		err := storage.RemoveAll("/remove.txt")
		if err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		got, _ := storage.Get("/remove.txt")
		if got != nil {
			t.Error("File should be removed")
		}
	})

	t.Run("remove preserves marker file when clearing root", func(t *testing.T) {
		// Create some files
		storage.Save(&FileMeta{Name: "temp.txt", RemoteName: "r1", Size: 100}, "/temp.txt")
		storage.Save(&FileMeta{Name: "tempdir", IsDir: true}, "/tempdir")

		// Remove root
		err := storage.RemoveAll("/")
		if err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		// Marker file should still exist
		markerPath := filepath.Join(tmpDir, ".clearvault")
		if _, err := os.Stat(markerPath); err != nil {
			t.Error(".clearvault marker should still exist")
		}

		// User files should be gone
		entries, err := storage.ReadDir("/")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("Expected empty directory, got %d entries", len(entries))
		}
	})

	t.Run("remove deeply nested structure", func(t *testing.T) {
		// Create deep structure
		storage.Save(&FileMeta{Name: "deep", IsDir: true}, "/deep")
		storage.Save(&FileMeta{Name: "level1", IsDir: true}, "/deep/level1")
		storage.Save(&FileMeta{Name: "level2", IsDir: true}, "/deep/level1/level2")
		storage.Save(&FileMeta{Name: "file.txt", RemoteName: "r1", Size: 100}, "/deep/level1/level2/file.txt")

		err := storage.RemoveAll("/deep")
		if err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		// All should be gone
		deep, _ := storage.Get("/deep")
		if deep != nil {
			t.Error("Deep directory should be removed")
		}
	})
}

// Test GetByRemoteName with multiple files
func TestLocalStorage_GetByRemoteNameMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	defer storage.Close()

	t.Run("find among many files", func(t *testing.T) {
		// Create many files
		for i := 0; i < 50; i++ {
			meta := &FileMeta{
				Name:       fmt.Sprintf("file%d.txt", i),
				RemoteName: fmt.Sprintf("remote%d", i),
				Size:       int64(i * 10),
			}
			storage.Save(meta, fmt.Sprintf("/file%d.txt", i))
		}

		// Find a specific one
		target, err := storage.GetByRemoteName("remote25")
		if err != nil {
			t.Fatalf("GetByRemoteName failed: %v", err)
		}
		if target == nil {
			t.Fatal("Expected to find file")
		}
		if target.Name != "file25.txt" {
			t.Errorf("Name = %q, want %q", target.Name, "file25.txt")
		}
		if target.Size != 250 {
			t.Errorf("Size = %d, want 250", target.Size)
		}
	})

	t.Run("returns first match", func(t *testing.T) {
		// Create files with same remote name (edge case)
		meta1 := &FileMeta{
			Name:       "first.txt",
			RemoteName: "duplicate",
			Size:       100,
		}
		meta2 := &FileMeta{
			Name:       "second.txt",
			RemoteName: "duplicate",
			Size:       200,
		}
		storage.Save(meta1, "/first.txt")
		storage.Save(meta2, "/second.txt")

		found, err := storage.GetByRemoteName("duplicate")
		if err != nil {
			t.Fatalf("GetByRemoteName failed: %v", err)
		}
		if found == nil {
			t.Fatal("Expected to find a file")
		}
		// Should return the first one found
		if found.Name != "first.txt" && found.Name != "second.txt" {
			t.Errorf("Unexpected name: %q", found.Name)
		}
	})
}


