package webdav

import (
	"fmt"
	"io"
	"net/http"
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
	// Use native http.Client to ensure streaming with chunked encoding
	// gowebdav.WriteStream might buffer or handle things differently depending on version

	// Manually construct URL since FixPath is not exported or available
	// Simple join, assuming url has no trailing slash or handled
	// Note: name usually comes from our Proxy which normalizes it.

	// Trim leading slash from name to avoid double slash
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	fullUrl := c.url
	if fullUrl[len(fullUrl)-1] != '/' {
		fullUrl += "/"
	}
	fullUrl += name

	req, err := http.NewRequest("PUT", fullUrl, data)
	if err != nil {
		return err
	}

	req.ContentLength = -1 // Force chunked encoding
	req.Header.Set("Content-Type", "application/octet-stream")

	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("upload failed with status: %s", resp.Status)
	}

	return nil
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
