package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func retryOperation(op func() error) error {
	var err error
	for i := 0; i < 100; i++ {
		err = op()
		if err == nil {
			return nil
		}
		// On Windows, file locking issues are common with "Access is denied"
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}
	// Add a hidden marker file to confirm it's a Clearvault metadata directory
	marker := filepath.Join(baseDir, ".clearvault")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		os.WriteFile(marker, []byte("Clearvault Metadata Storage"), 0644)
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

// getLocalPath returns the local file path for a metadata file (with .json extension for files)
func (s *LocalStorage) getLocalPath(p string) string {
	safePath := path.Clean("/" + filepath.ToSlash(p))
	if safePath == "/" || safePath == "." {
		return s.baseDir
	}
	rel := strings.TrimPrefix(safePath, "/")
	// Add .json extension for file metadata
	return filepath.Join(s.baseDir, rel) + ".json"
}

// getLocalPathWithoutJson returns the path without .json extension (for directories)
func (s *LocalStorage) getLocalPathWithoutJson(p string) string {
	safePath := path.Clean("/" + filepath.ToSlash(p))
	if safePath == "/" || safePath == "." {
		return s.baseDir
	}
	rel := strings.TrimPrefix(safePath, "/")
	return filepath.Join(s.baseDir, rel)
}

func (s *LocalStorage) Get(p string) (*FileMeta, error) {
	// Check if it's a directory first (directories don't have .json extension)
	dirLocal := s.getLocalPathWithoutJson(p)
	stat, err := os.Stat(dirLocal)
	if err == nil && stat.IsDir() {
		return &FileMeta{
			Name:      path.Base(p),
			IsDir:     true,
			UpdatedAt: stat.ModTime(),
		}, nil
	}

	// Check for file metadata with .json extension
	local := s.getLocalPath(p)
	stat, err = os.Stat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if stat.IsDir() {
		return &FileMeta{
			Name:      path.Base(p),
			IsDir:     true,
			UpdatedAt: stat.ModTime(),
		}, nil
	}

	data, err := os.ReadFile(local)
	if err != nil {
		return nil, err
	}

	var meta FileMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	// Name is already stored in the JSON, no need to set it
	return &meta, nil
}

func (s *LocalStorage) GetByRemoteName(remoteName string) (*FileMeta, error) {
	var found *FileMeta
	err := filepath.WalkDir(s.baseDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == ".clearvault" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var meta FileMeta
		if err := json.Unmarshal(data, &meta); err == nil {
			if meta.RemoteName == remoteName {
				found = &meta
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found, err
}

func (s *LocalStorage) Save(meta *FileMeta, p string) error {
	if meta.IsDir {
		// For directories, use path without .json extension
		local := s.getLocalPathWithoutJson(p)
		return os.MkdirAll(local, 0755)
	}

	// For files, use path with .json extension
	local := s.getLocalPath(p)
	if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(local, data, 0644)
}

func (s *LocalStorage) RemoveAll(p string) error {
	local := s.getLocalPath(p)
	localWithoutJson := s.getLocalPathWithoutJson(p)
	log.Printf("LocalStorage: RemoveAll '%s' -> '%s' or '%s'", p, local, localWithoutJson)

	// Try to remove both the file with .json and the directory without .json
	// First check which one exists
	var targetPath string
	if _, err := os.Stat(local); err == nil {
		targetPath = local
	} else if _, err := os.Stat(localWithoutJson); err == nil {
		targetPath = localWithoutJson
	}

	if targetPath == "" || targetPath == s.baseDir {
		// Don't delete baseDir itself, just children
		items, err := os.ReadDir(s.baseDir)
		if err != nil {
			log.Printf("LocalStorage: ReadDir failed for baseDir: %v", err)
			return err
		}
		for _, item := range items {
			if item.Name() == ".clearvault" {
				continue
			}
			childPath := filepath.Join(s.baseDir, item.Name())
			if err := retryOperation(func() error {
				return os.RemoveAll(childPath)
			}); err != nil {
				log.Printf("LocalStorage: RemoveAll child '%s' retry failed: %v", item.Name(), err)
			}
		}
		return nil
	}

	err := retryOperation(func() error {
		return os.RemoveAll(targetPath)
	})
	if err != nil {
		log.Printf("LocalStorage: RemoveAll '%s' retry failed: %v", targetPath, err)
	}
	return err
}

func (s *LocalStorage) ReadDir(p string) ([]FileMeta, error) {
	local := s.getLocalPathWithoutJson(p)
	entries, err := os.ReadDir(local)
	if err != nil {
		return nil, err
	}

	var results []FileMeta
	for _, entry := range entries {
		if entry.Name() == ".clearvault" {
			continue
		}

		// Remove .json extension from filename to get the virtual path
		filename := entry.Name()
		childName := strings.TrimSuffix(filename, ".json")

		// If filename didn't have .json extension, it's a directory
		if childName == filename {
			// This is a directory
			childPath := path.Join(p, childName)
			meta, err := s.Get(childPath)
			if err != nil {
				log.Printf("LocalStorage: ReadDir Get child '%s' error: %v", childPath, err)
				continue
			}
			if meta != nil {
				results = append(results, *meta)
			}
		} else {
			// This is a file with .json extension
			childPath := path.Join(p, childName)
			meta, err := s.Get(childPath)
			if err != nil {
				log.Printf("LocalStorage: ReadDir Get child '%s' error: %v", childPath, err)
				continue
			}
			if meta != nil {
				results = append(results, *meta)
			}
		}
	}
	return results, nil
}

func (s *LocalStorage) Rename(oldPath, newPath string) error {
	oldLocal := s.getLocalPath(oldPath)
	newLocal := s.getLocalPath(newPath)
	oldLocalWithoutJson := s.getLocalPathWithoutJson(oldPath)
	newLocalWithoutJson := s.getLocalPathWithoutJson(newPath)

	log.Printf("LocalStorage: Rename '%s' -> '%s'", oldLocal, newLocal)

	// Determine which source path exists (with or without .json)
	var oldTarget, newTarget string
	isFile := false
	if _, err := os.Stat(oldLocal); err == nil {
		oldTarget = oldLocal
		newTarget = newLocal
		isFile = true
	} else if _, err := os.Stat(oldLocalWithoutJson); err == nil {
		oldTarget = oldLocalWithoutJson
		newTarget = newLocalWithoutJson
	} else {
		return fmt.Errorf("source path not found: %s", oldPath)
	}

	if err := os.MkdirAll(filepath.Dir(newTarget), 0755); err != nil {
		log.Printf("LocalStorage: MkdirAll failed for Rename: %v", err)
		return err
	}

	err := retryOperation(func() error {
		return os.Rename(oldTarget, newTarget)
	})
	if err == nil {
		// If it's a file, update the Name field in the metadata
		if isFile {
			return s.updateMetadataName(newTarget, newPath)
		}
		return nil
	}

	log.Printf("LocalStorage: Rename failed, attempting fallback Copy+Delete: %v", err)

	// Fallback: Recursive Copy then Delete
	if err := copyDir(oldTarget, newTarget); err != nil {
		log.Printf("LocalStorage: Fallback Copy failed: %v", err)
		return err
	}

	// If it's a file, update the Name field in the metadata
	if isFile {
		if err := s.updateMetadataName(newTarget, newPath); err != nil {
			log.Printf("LocalStorage: Failed to update metadata name: %v", err)
		}
	}

	if err := s.RemoveAll(oldPath); err != nil {
		log.Printf("LocalStorage: Fallback Delete failed: %v", err)
	}
	return nil
}

// updateMetadataName updates the Name field in the metadata file to match the new path
func (s *LocalStorage) updateMetadataName(metaPath, newPath string) error {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return err
	}

	var meta FileMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}

	// Update the Name field to match the new path
	meta.Name = path.Base(newPath)

	newData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, newData, 0644)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return copyFile(src, dst)
	}

	if err := os.MkdirAll(dst, si.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if err := copyDir(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *LocalStorage) Close() error {
	return nil
}
