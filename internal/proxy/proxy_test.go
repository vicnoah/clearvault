package proxy

import (
	"bytes"
	"clearvault/internal/metadata"
	"clearvault/internal/webdav"
	"io"
	"os"
	"testing"
)

func TestSimpleProxy(t *testing.T) {
	metaDir := "./test_simple_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	// Use a mock or just the real client if possible, but let's see if NewProxy fails
	masterKey := "dmVyeS1zZWNyZXQtbWFzdGVyLWtleS1zaG91bGQtYmUtMzItYnl0ZXM="
	remote := webdav.NewRemoteClient("https://example.com", "user", "pass")

	p, err := NewProxy(meta, remote, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	if p == nil {
		t.Fatal("Proxy is nil")
	}
}

func TestZeroByteFileUpload(t *testing.T) {
	metaDir := "./test_zerobyte_meta"
	os.RemoveAll(metaDir)
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}

	// Use a dummy remote client. Since we expect NO remote calls for 0-byte files,
	// this should NOT fail even if the URL is unreachable.
	masterKey := "dmVyeS1zZWNyZXQtbWFzdGVyLWtleS1zaG91bGQtYmUtMzItYnl0ZXM="
	remote := webdav.NewRemoteClient("http://unreachable-url.local", "user", "pass")

	p, err := NewProxy(meta, remote, masterKey)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	// 1. Upload 0-byte file (RaiDrive Phase 1)
	err = p.UploadFile("/test.txt", bytes.NewReader([]byte{}), -1)
	if err != nil {
		t.Fatalf("UploadFile (0-byte) failed: %v", err)
	}

	// 2. Verify metadata
	m, err := meta.Get("/test.txt")
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}
	if m.RemoteName != ".pending" {
		t.Errorf("Expected RemoteName to be '.pending', got '%s'", m.RemoteName)
	}
	if m.Size != 0 {
		t.Errorf("Expected Size to be 0, got %d", m.Size)
	}

	// 3. Verify Download returns empty content
	rc, err := p.DownloadFile("/test.txt")
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Failed to read downloaded content: %v", err)
	}
	if len(content) != 0 {
		t.Errorf("Expected 0 bytes content, got %d", len(content))
	}
}
