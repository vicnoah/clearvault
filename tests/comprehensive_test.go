package tests

import (
	"bytes"
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/webdav"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	localwebdav "golang.org/x/net/webdav"
)

// generateRandomContent generates random content of specific size
func generateRandomContent(t *testing.T, size int64) []byte {
	b := make([]byte, size)
	_, err := rand.Read(b)
	if err != nil {
		t.Fatalf("Failed to generate random content: %v", err)
	}
	return b
}

// calculateHash calculates SHA256 hash of content
func calculateHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func TestComprehensiveExportImport(t *testing.T) {
	// 1. Shared Remote Setup
	mockRemoteDir, err := os.MkdirTemp("", "mock_remote_comp_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mockRemoteDir)

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)
	defer ts.Close()

	// 2. User Setup
	// User A (Sender)
	metaDirA, _ := os.MkdirTemp("", "meta_a_comp_*")
	defer os.RemoveAll(metaDirA)
	keyA := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	cfgA := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: keyA},
	}
	metaA, _ := metadata.NewLocalStorage(metaDirA)
	remoteClientA, _ := webdav.NewClient(webdav.WebDAVConfig{URL: cfgA.Remote.URL})
	proxyA, _ := proxy.NewProxy(metaA, remoteClientA, cfgA.Security.MasterKey)

	// User B (Recipient)
	metaDirB, _ := os.MkdirTemp("", "meta_b_comp_*")
	defer os.RemoveAll(metaDirB)
	keyB := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="
	cfgB := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: keyB},
	}
	metaB, _ := metadata.NewLocalStorage(metaDirB)
	remoteClientB, _ := webdav.NewClient(webdav.WebDAVConfig{URL: cfgB.Remote.URL})
	proxyB, _ := proxy.NewProxy(metaB, remoteClientB, cfgB.Security.MasterKey)

	// 3. Data Generation & Upload (User A)
	files := map[string][]byte{
		"/small.txt":              []byte("Small text file"),
		"/empty.txt":              []byte(""),
		"/large.bin":              generateRandomContent(t, 5*1024*1024), // 5MB
		"/dir1/file1.txt":         []byte("File in subdir"),
		"/dir1/subdir/nested.txt": []byte("Deeply nested file"),
	}

	t.Log("User A: Uploading files...")
	for fpath, content := range files {
		// Ensure directory structure exists in metadata (proxy handles this mostly on upload but strict structure helps)
		// Actually Proxy.UploadFile doesn't auto-create parent dir metadata usually, but let's see.
		// The current implementation of UploadFile takes a full path.
		// LocalStorage.Save creates directories on disk for metadata.
		// But strictly speaking we should create directory entries too for hierarchy.
		// Let's rely on how `encrypt` does it or just simple upload.
		// For proper tree structure in export, we usually need the directory metadata to exist.
		// Let's manually create directory metadata for User A to simulate a real usage.

		dir := path.Dir(fpath)
		if dir != "/" && dir != "." {
			// recursively create dirs
			parts := splitPath(dir)
			current := ""
			for _, part := range parts {
				current = path.Join(current, part)
				// Create dir metadata if not exists
				m, _ := metaA.Get(current)
				if m == nil {
					// We need to create directory metadata
					// Proxy doesn't expose Mkdir directly?
					// Let's just mock it by using metaA.Save directly or just rely on UploadFile creating file entries
					// The Export function walks the metadata tree.
					// If we only have file metadata, `ReadDir` on root might not find them if they are in subdirs
					// depending on implementation.
					// LocalStorage.ReadDir uses `os.ReadDir` on the metadata directory.
					// So if we save "/dir1/file1.txt", it creates "dir1" folder on disk.
					// But "dir1" itself might not have a ".json" or exist as a FileMeta entry if we don't create it.
					// Let's create directory metadata explicitly.
					proxyA.SavePlaceholder(current) // This is for file placeholder.

					// Use internal meta save for directory
					// In real usage, `Mkdir` would be called.
					// Since Proxy doesn't have Mkdir exposed in interface here easily (it's in FileSystem),
					// let's just cheat and use metaA directly for dirs.
					metaA.Save(&metadata.FileMeta{
						Name:  path.Base(current),
						IsDir: true,
					}, current)
				}
			}
		}

		err := proxyA.UploadFile(fpath, bytes.NewReader(content), int64(len(content)))
		if err != nil {
			t.Fatalf("Failed to upload %s: %v", fpath, err)
		}
	}

	// 4. Export (User A)
	t.Log("User A: Exporting metadata...")
	exportDir, _ := os.MkdirTemp("", "export_comp_*")
	defer os.RemoveAll(exportDir)
	shareKey := "complex-share-key-!@#"

	// We want to export everything under root.
	// We need to pass the paths.
	// pathsToExport := []string{"/"} // Export root? Or list of files?
	// If we export "/", tar_util needs to handle recursion.
	// addDirectoryToTar does recursion.
	// But "/" might be special.
	// Let's export top level items.

	// Get top level items
	topLevel, _ := metaA.ReadDir("/")
	var exportPaths []string
	for _, item := range topLevel {
		exportPaths = append(exportPaths, "/"+item.Name)
	}

	tarPath, err := proxyA.CreateSharePackage(exportPaths, exportDir, shareKey)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	t.Logf("Exported to %s", tarPath)

	// 5. Import (User B)
	t.Log("User B: Importing metadata...")
	err = proxyB.ReceiveSharePackage(tarPath, shareKey)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// 6. Verification (User B)
	t.Log("User B: Verifying files...")
	for fpath, originalContent := range files {
		t.Run(fmt.Sprintf("Verify %s", fpath), func(t *testing.T) {
			// Check metadata existence
			m, err := metaB.Get(fpath)
			if err != nil {
				t.Fatalf("Metadata lookup failed: %v", err)
			}
			if m == nil {
				t.Fatalf("Metadata missing for %s", fpath)
			}

			// Check content
			rc, err := proxyB.DownloadFile(fpath)
			if err != nil {
				t.Fatalf("Download failed: %v", err)
			}
			defer rc.Close()

			downloaded, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			// Compare Hashes
			origHash := calculateHash(originalContent)
			downHash := calculateHash(downloaded)

			if origHash != downHash {
				t.Errorf("Hash mismatch for %s!\nExpected: %s\nGot:      %s", fpath, origHash, downHash)
			} else {
				// t.Logf("Hash matched for %s", fpath)
			}
		})
	}
}

func splitPath(p string) []string {
	// split /a/b/c into [a, b, c]
	dir, file := path.Split(p)
	var parts []string
	if dir != "/" && dir != "" {
		parts = splitPath(path.Clean(dir))
	}
	if file != "" {
		parts = append(parts, file)
	}
	return parts
}
