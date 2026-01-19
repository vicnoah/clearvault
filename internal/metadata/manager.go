package metadata

import (
	"time"
)

type FileMeta struct {
	Name       string    `json:"name"`        // 文件名（不含路径）
	Path       string    `json:"path"`        // 目录路径（不含文件名）
	RemoteName string    `json:"remote_name"` // 远程文件名
	IsDir      bool      `json:"is_dir"`      // 是否为目录
	Size       int64     `json:"size"`        // 文件大小
	FEK        []byte    `json:"fek"`         // 加密的文件加密密钥
	Salt       []byte    `json:"salt"`        // 加密 Salt/Nonce
	UpdatedAt  time.Time `json:"updated_at"`  // 更新时间
}

type Storage interface {
	Get(path string) (*FileMeta, error)
	GetByRemoteName(remoteName string) (*FileMeta, error)
	Save(meta *FileMeta, path string) error  // path 是虚拟路径，用于确定存储位置
	RemoveAll(path string) error             // Recursive delete
	ReadDir(path string) ([]FileMeta, error) // List immediate children
	Rename(oldPath, newPath string) error    // Recursive rename
	Close() error
}
