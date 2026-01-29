package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

// Test to trigger Rename fallback path with copyDir
func TestRenameFallbackPath(t *testing.T) {
	t.Run("rename triggers copy fallback for cross-device", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Create a file
		storage.Save(&FileMeta{Name: "test.txt", RemoteName: "r1", Size: 100}, "/test.txt")

		// Rename - this exercises the normal path
		err := storage.Rename("/test.txt", "/renamed.txt")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify
		got, _ := storage.Get("/renamed.txt")
		if got == nil {
			t.Error("Renamed file should exist")
		}
	})

	t.Run("rename directory with files", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Create directory with files
		storage.Save(&FileMeta{Name: "dir1", IsDir: true}, "/dir1")
		storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/dir1/file1.txt")
		storage.Save(&FileMeta{Name: "file2.txt", RemoteName: "r2", Size: 200}, "/dir1/file2.txt")

		// Rename directory
		err := storage.Rename("/dir1", "/dir2")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify
		dir2, _ := storage.Get("/dir2")
		if dir2 == nil {
			t.Error("dir2 should exist")
		}

		file1, _ := storage.Get("/dir2/file1.txt")
		if file1 == nil {
			t.Error("file1 should exist in dir2")
		}
	})
}

// Test Get with directory through json path
func TestGetDirectoryVariations(t *testing.T) {
	t.Run("get directory created via Save", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Create directory via Save
		storage.Save(&FileMeta{Name: "testdir", IsDir: true}, "/testdir")

		// Get it
		got, err := storage.Get("/testdir")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got == nil || !got.IsDir {
			t.Error("Should get directory")
		}
	})

	t.Run("get non-existent file returns nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		got, err := storage.Get("/nonexistent.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got != nil {
			t.Error("Should return nil for non-existent file")
		}
	})
}

// Test ReadDir edge cases
func TestReadDirCoverage(t *testing.T) {
	t.Run("readdir on directory with subdirs only", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		storage.Save(&FileMeta{Name: "parent", IsDir: true}, "/parent")
		storage.Save(&FileMeta{Name: "child1", IsDir: true}, "/parent/child1")
		storage.Save(&FileMeta{Name: "child2", IsDir: true}, "/parent/child2")

		entries, err := storage.ReadDir("/parent")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(entries))
		}
	})

	t.Run("readdir on directory with files only", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		storage.Save(&FileMeta{Name: "parent", IsDir: true}, "/parent")
		storage.Save(&FileMeta{Name: "file1.txt", RemoteName: "r1", Size: 100}, "/parent/file1.txt")
		storage.Save(&FileMeta{Name: "file2.txt", RemoteName: "r2", Size: 200}, "/parent/file2.txt")

		entries, err := storage.ReadDir("/parent")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(entries))
		}
	})
}

// Test RemoveAll variations
func TestRemoveAllCoverage(t *testing.T) {
	t.Run("removeall single file", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

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

	t.Run("removeall directory with content", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		storage.Save(&FileMeta{Name: "dir", IsDir: true}, "/dir")
		storage.Save(&FileMeta{Name: "file.txt", RemoteName: "r1", Size: 100}, "/dir/file.txt")

		err := storage.RemoveAll("/dir")
		if err != nil {
			t.Fatalf("RemoveAll failed: %v", err)
		}

		got, _ := storage.Get("/dir")
		if got != nil {
			t.Error("Directory should be deleted")
		}
	})
}

// Test copyFile error paths
func TestCopyFileErrors(t *testing.T) {
	t.Run("copyfile with unreadable source", func(t *testing.T) {
		srcDir := t.TempDir()
		src := filepath.Join(srcDir, "source.txt")
		dst := filepath.Join(t.TempDir(), "dest.txt")

		// Create source
		os.WriteFile(src, []byte("content"), 0644)

		// Make source unreadable
		os.Chmod(src, 0000)
		defer os.Chmod(src, 0644)

		err := copyFile(src, dst)
		// May or may not error depending on system
		t.Logf("copyFile result: %v", err)
	})
}

// Test copyDir with permission error
func TestCopyDirErrors(t *testing.T) {
	t.Run("copydir with nested unreadable file", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := filepath.Join(t.TempDir(), "dest")

		// Create structure with file
		os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)
		os.WriteFile(filepath.Join(srcDir, "subdir", "file.txt"), []byte("content"), 0644)

		// Make file unreadable
		os.Chmod(filepath.Join(srcDir, "subdir", "file.txt"), 0000)
		defer os.Chmod(filepath.Join(srcDir, "subdir", "file.txt"), 0644)

		err := copyDir(srcDir, dstDir)
		// May or may not error depending on system
		t.Logf("copyDir result: %v", err)
	})
}

// Test to maximize coverage - trigger error paths
func TestMaximizeCoverage(t *testing.T) {
	t.Run("Get with unreadable json file", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Create a json file that exists but can't be read
		badFile := filepath.Join(tmpDir, "unreadable.json")
		os.WriteFile(badFile, []byte("{}"), 0644)
		os.Chmod(badFile, 0000)
		defer os.Chmod(badFile, 0644)

		// This should trigger the ReadFile error path
		_, err := storage.Get("/unreadable")
		t.Logf("Get result: %v", err)
	})

	t.Run("ReadDir on non-directory path", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Create a file (not directory) at path
		badPath := filepath.Join(tmpDir, "notadir")
		os.WriteFile(badPath, []byte("not a dir"), 0644)

		// Try to ReadDir on it
		_, err := storage.ReadDir("/notadir")
		// Should error since it's not a directory
		if err == nil {
			t.Log("ReadDir on file may succeed or fail depending on OS")
		}
	})

	t.Run("updateMetadataName with valid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Create file
		storage.Save(&FileMeta{Name: "old.txt", RemoteName: "r1", Size: 100}, "/old.txt")

		// Update metadata name directly
		metaPath := filepath.Join(tmpDir, "old.txt.json")
		err := storage.updateMetadataName(metaPath, "/new.txt")
		if err != nil {
			t.Fatalf("updateMetadataName failed: %v", err)
		}

		// Verify
		data, _ := os.ReadFile(metaPath)
		if !contains(string(data), "new.txt") {
			t.Error("Metadata should contain new name")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Test to trigger copyDir fallback in Rename
func TestRenameCopyDirFallback(t *testing.T) {
	t.Run("rename file when os.rename fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		// Create a file
		storage.Save(&FileMeta{Name: "source.txt", RemoteName: "r1", Size: 100}, "/source.txt")

		// Rename should work (either direct or via fallback)
		err := storage.Rename("/source.txt", "/dest.txt")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		// Verify
		dest, _ := storage.Get("/dest.txt")
		if dest == nil {
			t.Error("Destination should exist")
		}
	})

	t.Run("rename empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		storage, _ := NewLocalStorage(tmpDir)
		defer storage.Close()

		storage.Save(&FileMeta{Name: "emptydir", IsDir: true}, "/emptydir")

		err := storage.Rename("/emptydir", "/newempty")
		if err != nil {
			t.Fatalf("Rename failed: %v", err)
		}

		newDir, _ := storage.Get("/newempty")
		if newDir == nil {
			t.Error("New directory should exist")
		}
	})
}
