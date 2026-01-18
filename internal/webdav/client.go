package webdav

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"clearvault/pkg/gowebdav"
)

type RemoteClient struct {
	client *gowebdav.Client
	url    string
	user   string
	pass   string
}

func NewRemoteClient(url, user, pass string) *RemoteClient {
	c := gowebdav.NewClient(url, user, pass)

	t := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 60 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
	}
	c.SetTransport(t)

	return &RemoteClient{
		client: c,
		url:    url,
		user:   user,
		pass:   pass,
	}
}

func (c *RemoteClient) SetTransport(rt http.RoundTripper) {
	c.client.SetTransport(rt)
}

func (c *RemoteClient) Upload(name string, data io.Reader, size int64) error {
	log.Printf("WebDAV: Uploading '%s' (size: %d)", name, size)
	return c.client.WriteStreamWithLength(name, data, size, os.ModePerm)
}

func (c *RemoteClient) Download(name string) (io.ReadCloser, error) {
	return c.client.ReadStream(name)
}

func (c *RemoteClient) DownloadRange(name string, start, length int64) (io.ReadCloser, error) {
	// If length is 0, we can't easily use ReadStreamRange for "to the end"
	// without knowing the total size. However, the manual implementation
	// was also a bit ambiguous. Let's use ReadStreamRange as requested.
	// If the caller wants "to the end", they should ideally provide the length.
	return c.client.ReadStreamRange(name, start, length)
}

func (c *RemoteClient) Delete(path string) error {
	return c.client.RemoveAll(path)
}

func (c *RemoteClient) Rename(oldPath, newPath string) error {
	return c.client.Rename(oldPath, newPath, true)
}

func (c *RemoteClient) Stat(path string) (os.FileInfo, error) {
	return c.client.Stat(path)
}
