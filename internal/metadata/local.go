package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

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
	// On Windows, filepath.Join with a leading slash might be tricky,
	// but path.Clean ensures it's relative-like except for the root.
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
	// Path in FileMeta should match requested path
	meta.Path = p
	return &meta, nil
}

func (s *LocalStorage) GetByRemoteName(remoteName string) (*FileMeta, error) {
	// Re-implementing walk for RemoteName lookups
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
				// Calculate virtual path
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
	if local == s.baseDir {
		// Don't delete baseDir itself, just children
		items, err := os.ReadDir(local)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Name() == ".clearvault" {
				continue
			}
			os.RemoveAll(filepath.Join(local, item.Name()))
		}
		return nil
	}
	return os.RemoveAll(local)
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

	if err := os.MkdirAll(filepath.Dir(newLocal), 0755); err != nil {
		return err
	}

	return os.Rename(oldLocal, newLocal)
}

func (s *LocalStorage) Close() error {
	return nil
}
