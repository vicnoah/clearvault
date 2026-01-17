package webdav

import (
	"io"
	"os"

	"github.com/studio-b12/gowebdav"
)

type RemoteClient struct {
	client *gowebdav.Client
	url    string
	user   string
	pass   string
}

func NewRemoteClient(url, user, pass string) *RemoteClient {
	c := gowebdav.NewClient(url, user, pass)
	return &RemoteClient{
		client: c,
		url:    url,
		user:   user,
		pass:   pass,
	}
}

func (c *RemoteClient) Upload(name string, data io.Reader) error {
	return c.client.WriteStream(name, data, 0644)
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
