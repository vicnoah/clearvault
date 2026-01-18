package proxy

import (
	"clearvault/internal/metadata"
	"os"
	"path/filepath"
	"testing"
)

func TestExportLocalSimple(t *testing.T) {
	metaDir, err := os.MkdirTemp("", "cv_export_meta")
	if err != nil {
		t.Fatalf("MkdirTemp meta: %v", err)
	}
	defer os.RemoveAll(metaDir)

	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	masterKey := "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXk="
	p, err := NewProxy(meta, nil, masterKey)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	inputDir, err := os.MkdirTemp("", "cv_export_in")
	if err != nil {
		t.Fatalf("MkdirTemp input: %v", err)
	}
	defer os.RemoveAll(inputDir)

	subDir := filepath.Join(inputDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll sub: %v", err)
	}

	plainPath := filepath.Join(subDir, "file.txt")
	if err := os.WriteFile(plainPath, []byte("hello world"), 0644); err != nil {
		t.Fatalf("WriteFile plain: %v", err)
	}

	outputDir, err := os.MkdirTemp("", "cv_export_out")
	if err != nil {
		t.Fatalf("MkdirTemp output: %v", err)
	}
	defer os.RemoveAll(outputDir)

	if err := p.ExportLocal(inputDir, outputDir); err != nil {
		t.Fatalf("ExportLocal: %v", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ReadDir output: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no exported files")
	}

	metaEntry, err := meta.Get("/sub/file.txt")
	if err != nil {
		t.Fatalf("meta.Get: %v", err)
	}
	if metaEntry == nil {
		t.Fatalf("meta for /sub/file.txt is nil")
	}
	if metaEntry.RemoteName == "" {
		t.Fatalf("RemoteName empty")
	}
	outPath := filepath.Join(outputDir, metaEntry.RemoteName)
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("exported file not found: %v", err)
	}
}
