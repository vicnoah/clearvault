package tests

import (
	"bytes"
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/webdav"
	"context"
	"io"
	"os"
	"testing"
)

func TestIntegration(t *testing.T) {
	// Use a temporary DB for testing
	dbPath := "./test_clearvault.db"
	defer os.Remove(dbPath)

	cfg := &config.Config{
		Remote: config.RemoteConfig{
			URL:  "https://pan.vicno.cc/dav/115/clearvault",
			User: "admin",
			Pass: "9776586516",
		},
		Security: config.SecurityConfig{
			MasterKey: "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk=",
		},
	}

	meta, err := metadata.NewManager(dbPath)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	remote := webdav.NewRemoteClient(cfg.Remote.URL, cfg.Remote.User, cfg.Remote.Pass)
	p, err := proxy.NewProxy(meta, remote, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to init proxy: %v", err)
	}

	fs := proxy.NewFileSystem(p)

	// Test Upload
	testFile := "/test_integration.txt"
	content := []byte("Clearvault Integration Test Content")

	t.Logf("Starting UploadFile...")
	err = p.UploadFile(testFile, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	t.Logf("Starting Stat...")
	// Test Stat
	info, err := fs.Stat(context.Background(), testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Name() != "test_integration.txt" {
		t.Errorf("Wrong filename in Stat: %s", info.Name())
	}

	t.Logf("Starting DownloadFile...")
	// Test Download
	rc, err := p.DownloadFile(testFile)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	defer rc.Close()

	downloaded, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Read downloaded failed: %v", err)
	}

	if !bytes.Equal(content, downloaded) {
		t.Errorf("Downloaded content mismatch.\nExpected: %s\nGot: %s", string(content), string(downloaded))
	}

	// Cleanup remote
	metaInfo, _ := p.GetFileMeta(testFile)
	if metaInfo != nil {
		remote.Delete(metaInfo.RemoteName)
	}
}
