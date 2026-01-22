package local

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalClient struct {
	rootPath string
}

func NewClient(rootPath string) (*LocalClient, error) {
	// Create root directory if it doesn't exist
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create local remote storage root: %w", err)
	}
	return &LocalClient{rootPath: rootPath}, nil
}

func (c *LocalClient) getPath(name string) string {
	return filepath.Join(c.rootPath, name)
}

func (c *LocalClient) Upload(name string, data io.Reader, size int64) error {
	path := c.getPath(name)
	
	// Create parent directory if needed (though remote names are usually flat, but good to be safe)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, data)
	return err
}

func (c *LocalClient) Download(name string) (io.ReadCloser, error) {
	path := c.getPath(name)
	return os.Open(path)
}

func (c *LocalClient) DownloadRange(name string, start, length int64) (io.ReadCloser, error) {
	path := c.getPath(name)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	if _, err := file.Seek(start, 0); err != nil {
		file.Close()
		return nil, err
	}

	if length <= 0 {
		return file, nil
	}

	return &limitReadCloser{
		r: io.LimitReader(file, length),
		c: file,
	}, nil
}

func (c *LocalClient) Delete(name string) error {
	path := c.getPath(name)
	return os.Remove(path)
}

func (c *LocalClient) Rename(oldName, newName string) error {
	oldPath := c.getPath(oldName)
	newPath := c.getPath(newName)
	return os.Rename(oldPath, newPath)
}

func (c *LocalClient) Stat(name string) (os.FileInfo, error) {
	path := c.getPath(name)
	return os.Stat(path)
}

func (c *LocalClient) Close() error {
	return nil
}

// limitReadCloser wraps io.LimitReader to add Close method
type limitReadCloser struct {
	r io.Reader
	c io.Closer
}

func (l *limitReadCloser) Read(p []byte) (n int, err error) {
	return l.r.Read(p)
}

func (l *limitReadCloser) Close() error {
	return l.c.Close()
}
