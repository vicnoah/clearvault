package remote

import (
	"io"
	"os"
)

// RemoteStorage 远端存储抽象接口
// 支持多种存储后端：WebDAV、S3、MinIO、Cloudflare R2 等
type RemoteStorage interface {
	// Upload 上传文件到远端存储
	// name: 远端文件名（路径）
	// data: 文件数据流
	// size: 文件大小（-1 表示未知大小）
	Upload(name string, data io.Reader, size int64) error

	// Download 从远端存储下载文件
	// name: 远端文件名（路径）
	// 返回: 文件数据流（需要调用者关闭）
	Download(name string) (io.ReadCloser, error)

	// DownloadRange 从远端存储下载文件的指定范围
	// name: 远端文件名（路径）
	// start: 起始字节偏移
	// length: 要读取的字节数（0 表示到文件末尾）
	DownloadRange(name string, start, length int64) (io.ReadCloser, error)

	// Delete 从远端存储删除文件
	// path: 远端文件路径
	Delete(path string) error

	// Rename 重命名/移动远端文件
	// 注意：S3 不支持原生重命名，需要使用 copy+delete 实现
	// oldPath: 旧文件路径
	// newPath: 新文件路径
	Rename(oldPath, newPath string) error

	// Stat 获取远端文件信息
	// path: 远端文件路径
	// 返回: 文件信息（实现了 os.FileInfo 接口）
	Stat(path string) (os.FileInfo, error)

	// Close 清理资源（如连接池等）
	// 注意：某些实现可能不需要显式关闭
	Close() error
}
