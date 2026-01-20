package remote

import (
	"fmt"
	"strings"

	"clearvault/internal/config"
	"clearvault/internal/remote/s3"
	"clearvault/internal/remote/webdav"
)

// NewRemoteStorage 根据配置创建远程存储客户端
// 支持多种存储后端：WebDAV、S3、MinIO、Cloudflare R2 等
func NewRemoteStorage(cfg config.RemoteConfig) (RemoteStorage, error) {
	// 确定存储类型（默认为 WebDAV 以保持向后兼容）
	storageType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if storageType == "" {
		storageType = "webdav"
	}

	switch storageType {
	case "webdav", "dav":
		return newWebDAVClient(cfg)
	case "s3", "minio", "r2":
		return newS3Client(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s (supported: webdav, s3)", storageType)
	}
}

// newWebDAVClient 创建 WebDAV 客户端
func newWebDAVClient(cfg config.RemoteConfig) (RemoteStorage, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("webdav: URL is required")
	}

	return webdav.NewClient(webdav.WebDAVConfig{
		URL:  cfg.URL,
		User: cfg.User,
		Pass: cfg.Pass,
	})
}

// newS3Client 创建 S3 客户端
func newS3Client(cfg config.RemoteConfig) (RemoteStorage, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("s3: endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3: bucket is required")
	}
	if cfg.AccessKey == "" {
		return nil, fmt.Errorf("s3: access_key is required")
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("s3: secret_key is required")
	}

	// 默认区域
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	return s3.NewClient(s3.S3Config{
		Endpoint:  cfg.Endpoint,
		Region:    region,
		Bucket:    cfg.Bucket,
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
		UseSSL:    cfg.UseSSL,
	})
}
