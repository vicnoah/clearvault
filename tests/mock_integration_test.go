package tests

import (
	"bytes"
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/webdav"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	localwebdav "golang.org/x/net/webdav"
)

func TestIntegrationLocalMock(t *testing.T) {
	// Setup a local WebDAV server as a mock remote
	mockRemoteDir := "./mock_remote"
	os.MkdirAll(mockRemoteDir, 0755)
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	// Setup Clearvault with local mock as remote
	dbPath := "./test_clearvault_mock.db"
	defer os.Remove(dbPath)

	cfg := &config.Config{
		Remote: config.RemoteConfig{
			URL:  ts.URL,
			User: "admin",
			Pass: "pass",
		},
		Security: config.SecurityConfig{
			MasterKey: "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk=",
		},
	}

	meta, _ := metadata.NewManager(dbPath)
	defer meta.Close()

	remote := webdav.NewRemoteClient(cfg.Remote.URL, cfg.Remote.User, cfg.Remote.Pass)
	p, _ := proxy.NewProxy(meta, remote, cfg.Security.MasterKey)
	fs := proxy.NewFileSystem(p)

	// Test Upload
	testFile := "/test.txt"
	content := []byte("Mock Integration Test Content")
	err := p.UploadFile(testFile, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify remote has an encrypted file (hash name)
	metas, _ := meta.GetByPath(testFile)
	if metas == nil {
		t.Fatal("Metadata not found after upload")
	}
	if _, err := os.Stat(mockRemoteDir + "/" + metas.RemoteName); err != nil {
		t.Fatalf("Encrypted file not found on remote: %v", err)
	}

	// Test Download
	rc, err := p.DownloadFile(testFile)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	defer rc.Close()

	downloaded, _ := io.ReadAll(rc)
	if !bytes.Equal(content, downloaded) {
		t.Errorf("Content mismatch.\nExp: %s\nGot: %s", string(content), string(downloaded))
	}

	// Test FS Stat
	info, err := fs.Stat(context.Background(), testFile)
	if err != nil {
		t.Fatalf("FS Stat failed: %v", err)
	}
	if info.Size() != int64(len(content)) {
		t.Errorf("Size mismatch. Exp: %d, Got: %d", len(content), info.Size())
	}
}
