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

func (s *LocalStorage) getLocalPath(p string) string {
	safePath := path.Clean("/" + filepath.ToSlash(p))
	if safePath == "/" || safePath == "." {
		return s.baseDir
	}
	rel := strings.TrimPrefix(safePath, "/")
	return filepath.Join(s.baseDir, rel)
}

func (s *LocalStorage) Get(p string) (*FileMeta, error) {
	local := s.getLocalPath(p)
	stat, err := os.Stat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if stat.IsDir() {
		return &FileMeta{
			Path:      p,
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
	meta.Path = p
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
				rel, _ := filepath.Rel(s.baseDir, p)
				found.Path = path.Clean("/" + filepath.ToSlash(rel))
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found, err
}

func (s *LocalStorage) Save(meta *FileMeta) error {
	local := s.getLocalPath(meta.Path)
	if meta.IsDir {
		return os.MkdirAll(local, 0755)
	}

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
	log.Printf("LocalStorage: RemoveAll '%s' -> '%s'", p, local)
	if local == s.baseDir {
		// Don't delete baseDir itself, just children
		items, err := os.ReadDir(local)
		if err != nil {
			log.Printf("LocalStorage: ReadDir failed for baseDir: %v", err)
			return err
		}
		for _, item := range items {
			if item.Name() == ".clearvault" {
				continue
			}
			childPath := filepath.Join(local, item.Name())
			if err := retryOperation(func() error {
				return os.RemoveAll(childPath)
			}); err != nil {
				log.Printf("LocalStorage: RemoveAll child '%s' retry failed: %v", item.Name(), err)
			}
		}
		return nil
	}
	err := retryOperation(func() error {
		return os.RemoveAll(local)
	})
	if err != nil {
		log.Printf("LocalStorage: RemoveAll '%s' retry failed: %v", local, err)
	}
	return err
}

func (s *LocalStorage) ReadDir(p string) ([]FileMeta, error) {
	local := s.getLocalPath(p)
	entries, err := os.ReadDir(local)
	if err != nil {
		return nil, err
	}

	var results []FileMeta
	for _, entry := range entries {
		if entry.Name() == ".clearvault" {
			continue
		}
		childPath := path.Join(p, entry.Name())
		meta, err := s.Get(childPath)
		if err != nil {
			log.Printf("LocalStorage: ReadDir Get child '%s' error: %v", childPath, err)
			continue
		}
		if meta != nil {
			results = append(results, *meta)
		}
	}
	return results, nil
}

func (s *LocalStorage) Rename(oldPath, newPath string) error {
	oldLocal := s.getLocalPath(oldPath)
	newLocal := s.getLocalPath(newPath)

	log.Printf("LocalStorage: Rename '%s' -> '%s'", oldLocal, newLocal)

	if err := os.MkdirAll(filepath.Dir(newLocal), 0755); err != nil {
		log.Printf("LocalStorage: MkdirAll failed for Rename: %v", err)
		return err
	}

	err := retryOperation(func() error {
		return os.Rename(oldLocal, newLocal)
	})
	if err == nil {
		return nil
	}

	log.Printf("LocalStorage: Rename failed, attempting fallback Copy+Delete: %v", err)

	// Fallback: Recursive Copy then Delete
	if err := copyDir(oldLocal, newLocal); err != nil {
		log.Printf("LocalStorage: Fallback Copy failed: %v", err)
		return err
	}

	if err := s.RemoveAll(oldPath); err != nil {
		log.Printf("LocalStorage: Fallback Delete failed: %v", err)
	}
	return nil
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
