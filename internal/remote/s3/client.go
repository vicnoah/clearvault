package s3

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config S3 客户端配置
type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// S3Client S3 远端客户端实现
type S3Client struct {
	client *minio.Client
	bucket string
}

// s3FileInfo 实现 os.FileInfo 接口
type s3FileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi *s3FileInfo) Name() string       { return fi.name }
func (fi *s3FileInfo) Size() int64        { return fi.size }
func (fi *s3FileInfo) Mode() os.FileMode  { return 0644 }
func (fi *s3FileInfo) ModTime() time.Time { return fi.modTime }
func (fi *s3FileInfo) IsDir() bool        { return fi.isDir }
func (fi *s3FileInfo) Sys() interface{}   { return nil }

// NewClient 创建 S3 客户端
func NewClient(cfg S3Config) (*S3Client, error) {
	ctx := context.Background()

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Region: cfg.Region,
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	// 验证 bucket 是否存在
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("bucket '%s' does not exist (zs3 does not support creating buckets)", cfg.Bucket)
	}

	return &S3Client{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Upload 上传文件到 S3
// minio-go 的 PutObject 会自动处理分片上传：
// - 文件 < 16MB: 使用普通 PUT
// - 文件 >= 16MB: 自动使用 Multipart Upload
func (c *S3Client) Upload(name string, data io.Reader, size int64) error {
	ctx := context.Background()

	// 路径规范化：移除 leading slash
	objectName := normalizePath(name)

	log.Printf("S3: Uploading '%s' to bucket '%s' (size: %d)", objectName, c.bucket, size)

	opts := minio.PutObjectOptions{}
	if size > 0 {
		opts.ContentType = "application/octet-stream"
	}

	_, err := c.client.PutObject(ctx, c.bucket, objectName, data, size, opts)
	if err != nil {
		return fmt.Errorf("failed to upload object '%s': %w", objectName, err)
	}

	return nil
}

// Download 从 S3 下载文件
func (c *S3Client) Download(name string) (io.ReadCloser, error) {
	ctx := context.Background()
	objectName := normalizePath(name)

	log.Printf("S3: Downloading '%s' from bucket '%s'", objectName, c.bucket)

	obj, err := c.client.GetObject(ctx, c.bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object '%s': %w", objectName, err)
	}

	// 验证对象是否存在
	_, err = obj.Stat()
	if err != nil {
		obj.Close()
		return nil, fmt.Errorf("object '%s' not found: %w", objectName, err)
	}

	return obj, nil
}

// DownloadRange 下载文件的指定字节范围
func (c *S3Client) DownloadRange(name string, start, length int64) (io.ReadCloser, error) {
	ctx := context.Background()
	objectName := normalizePath(name)

	log.Printf("S3: Downloading range [%d:%d] for '%s' from bucket '%s'", start, length, objectName, c.bucket)

	// 先获取文件大小，以确定实际的范围
	objInfo, err := c.client.StatObject(ctx, c.bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stat object '%s': %w", objectName, err)
	}

	// 如果请求的范围超出文件大小，调整到文件末尾
	actualLength := length
	log.Printf("S3: Range request: start=%d, length=%d, fileSize=%d", start, length, objInfo.Size)
	if length > 0 && start+length > objInfo.Size {
		actualLength = objInfo.Size - start
		if actualLength < 0 {
			actualLength = 0
		}
		log.Printf("S3: Adjusting range from [%d:%d] to [%d:%d] (file size: %d)", start, length, start, actualLength, objInfo.Size)
	}

	opts := minio.GetObjectOptions{}
	if actualLength > 0 {
		// S3 使用 inclusive range (bytes=start-end)
		end := start + actualLength - 1
		opts.SetRange(start, end)
		log.Printf("S3: Setting range [%d:%d]", start, end)
	} else if start > 0 {
		// 从 start 到文件末尾
		opts.SetRange(start, -1)
		log.Printf("S3: Setting range [%d:EOF]", start)
	}

	obj, err := c.client.GetObject(ctx, c.bucket, objectName, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get object '%s': %w", objectName, err)
	}

	return obj, nil
}

// Delete 从 S3 删除文件
func (c *S3Client) Delete(path string) error {
	ctx := context.Background()
	objectName := normalizePath(path)

	log.Printf("S3: Deleting '%s' from bucket '%s'", objectName, c.bucket)

	err := c.client.RemoveObject(ctx, c.bucket, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object '%s': %w", objectName, err)
	}

	return nil
}

// Rename 重命名 S3 文件（使用 copy + delete）
func (c *S3Client) Rename(oldPath, newPath string) error {
	ctx := context.Background()

	oldObjectName := normalizePath(oldPath)
	newObjectName := normalizePath(newPath)

	log.Printf("S3: Renaming '%s' to '%s' in bucket '%s'", oldObjectName, newObjectName, c.bucket)

	// S3 不支持原生重命名，使用 copy + delete
	// 1. 复制对象
	srcOpts := minio.CopySrcOptions{
		Bucket: c.bucket,
		Object: oldObjectName,
	}

	destOpts := minio.CopyDestOptions{
		Bucket: c.bucket,
		Object: newObjectName,
	}

	_, err := c.client.CopyObject(ctx, destOpts, srcOpts)
	if err != nil {
		return fmt.Errorf("failed to copy object from '%s' to '%s': %w", oldObjectName, newObjectName, err)
	}

	// 2. 删除原对象
	err = c.Delete(oldPath)
	if err != nil {
		// 如果删除失败，记录警告但不返回错误（新对象已存在）
		log.Printf("S3: Warning: Failed to delete original object '%s' after rename: %v", oldObjectName, err)
	}

	return nil
}

// Stat 获取 S3 文件信息
func (c *S3Client) Stat(path string) (os.FileInfo, error) {
	ctx := context.Background()
	objectName := normalizePath(path)

	objInfo, err := c.client.StatObject(ctx, c.bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stat object '%s': %w", objectName, err)
	}

	return &s3FileInfo{
		name:    filepath.Base(path),
		size:    objInfo.Size,
		modTime: objInfo.LastModified,
		isDir:   false, // S3 没有真正的目录
	}, nil
}

// Close 清理资源（S3 客户端不需要显式关闭）
func (c *S3Client) Close() error {
	return nil
}

// normalizePath 规范化 S3 对象键
// - 移除 leading slash
// - 转换为正斜杠
func normalizePath(path string) string {
	cleaned := filepath.ToSlash(path)
	cleaned = strings.TrimPrefix(cleaned, "/")
	return cleaned
}
