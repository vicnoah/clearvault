package metadata

import (
	"time"
)

type FileMeta struct {
	Path       string    `json:"path"`
	RemoteName string    `json:"remote_name"`
	IsDir      bool      `json:"is_dir"`
	Size       int64     `json:"size"`
	FEK        []byte    `json:"fek"`
	Salt       []byte    `json:"salt"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Storage interface {
	Get(path string) (*FileMeta, error)
	GetByRemoteName(remoteName string) (*FileMeta, error)
	Save(meta *FileMeta) error
	RemoveAll(path string) error             // Recursive delete
	ReadDir(path string) ([]FileMeta, error) // List immediate children
	Rename(oldPath, newPath string) error    // Recursive rename
	Close() error
}
