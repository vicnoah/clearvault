package webdav

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"clearvault/pkg/gowebdav"
)

// WebDAVConfig WebDAV 客户端配置
type WebDAVConfig struct {
	URL  string
	User string
	Pass string
}

// WebDAVClient WebDAV 远端客户端实现
type WebDAVClient struct {
	client *gowebdav.Client
	url    string
}

// NewClient 创建 WebDAV 客户端
func NewClient(cfg WebDAVConfig) (*WebDAVClient, error) {
	c := gowebdav.NewClient(cfg.URL, cfg.User, cfg.Pass)

	t := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 60 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
	}
	c.SetTransport(t)

	return &WebDAVClient{
		client: c,
		url:    cfg.URL,
	}, nil
}

// SetTransport 设置 HTTP 传输层（用于测试）
func (c *WebDAVClient) SetTransport(rt http.RoundTripper) {
	c.client.SetTransport(rt)
}

// Upload 上传文件到 WebDAV 服务器
func (c *WebDAVClient) Upload(name string, data io.Reader, size int64) error {
	log.Printf("WebDAV: Uploading '%s' (size: %d)", name, size)
	return c.client.WriteStreamWithLength(name, data, size, os.ModePerm)
}

// Download 从 WebDAV 服务器下载文件
func (c *WebDAVClient) Download(name string) (io.ReadCloser, error) {
	return c.client.ReadStream(name)
}

// DownloadRange 从 WebDAV 服务器下载文件的指定范围
func (c *WebDAVClient) DownloadRange(name string, start, length int64) (io.ReadCloser, error) {
	return c.client.ReadStreamRange(name, start, length)
}

// Delete 从 WebDAV 服务器删除文件
func (c *WebDAVClient) Delete(path string) error {
	return c.client.RemoveAll(path)
}

// Rename 重命名 WebDAV 服务器上的文件
func (c *WebDAVClient) Rename(oldPath, newPath string) error {
	return c.client.Rename(oldPath, newPath, true)
}

// Stat 获取 WebDAV 文件信息
func (c *WebDAVClient) Stat(path string) (os.FileInfo, error) {
	return c.client.Stat(path)
}

// Close 清理资源（WebDAV 客户端不需要显式关闭）
func (c *WebDAVClient) Close() error {
	return nil
}
