package proxy

import (
	"clearvault/internal/metadata"
	"clearvault/internal/webdav"
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
