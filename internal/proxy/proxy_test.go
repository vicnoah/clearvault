package proxy

import (
	"clearvault/internal/metadata"
	"clearvault/internal/webdav"
	"os"
	"testing"
)

func TestSimpleProxy(t *testing.T) {
	dbPath := "./test_simple.db"
	defer os.Remove(dbPath)

	meta, _ := metadata.NewManager(dbPath)
	defer meta.Close()

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
