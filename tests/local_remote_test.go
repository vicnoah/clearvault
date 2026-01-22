package tests

import (
	"bytes"
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/remote"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalRemoteStorage(t *testing.T) {
	// 1. Setup Environment
	// Temp dir for local remote storage
	localRemoteRoot, err := os.MkdirTemp("", "local_remote_root_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(localRemoteRoot)

	// Temp dir for metadata
	metaDir, err := os.MkdirTemp("", "meta_local_remote_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(metaDir)

	// 2. Configure ClearVault to use local filesystem as remote
	cfg := &config.Config{
		Remote: config.RemoteConfig{
			Type:      "local",
			LocalPath: localRemoteRoot,
		},
		Security: config.SecurityConfig{
			MasterKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		},
	}

	// 3. Initialize Components
	meta, err := metadata.NewLocalStorage(metaDir)
	if err != nil {
		t.Fatalf("Failed to init metadata: %v", err)
	}
	defer meta.Close()

	remoteStorage, err := remote.NewRemoteStorage(cfg.Remote)
	if err != nil {
		t.Fatalf("Failed to init remote storage: %v", err)
	}
	defer remoteStorage.Close()

	p, err := proxy.NewProxy(meta, remoteStorage, cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("Failed to init proxy: %v", err)
	}

	fs := proxy.NewFileSystem(p)

	// 4. Test Upload
	testFile := "/test_local_remote.txt"
	content := []byte("Content stored in local filesystem remote")
	
	t.Logf("Uploading %s...", testFile)
	err = p.UploadFile(testFile, bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// 5. Verify file exists in "remote" location
	// We need to find the remote name from metadata
	metaInfo, err := meta.Get(testFile)
	if err != nil || metaInfo == nil {
		t.Fatalf("Metadata not found for %s", testFile)
	}
	
	expectedRemotePath := filepath.Join(localRemoteRoot, metaInfo.RemoteName)
	if _, err := os.Stat(expectedRemotePath); os.IsNotExist(err) {
		t.Fatalf("Remote file not found at %s", expectedRemotePath)
	} else {
		t.Logf("Verified remote file exists at: %s", expectedRemotePath)
	}

	// 6. Test Stat via Proxy
	info, err := fs.Stat(context.Background(), testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != int64(len(content)) {
		t.Errorf("Size mismatch. Expected: %d, Got: %d", len(content), info.Size())
	}

	// 7. Test Download
	t.Logf("Downloading %s...", testFile)
	rc, err := p.DownloadFile(testFile)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	defer rc.Close()

	downloaded, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Read download failed: %v", err)
	}

	if !bytes.Equal(content, downloaded) {
		t.Errorf("Content mismatch.\nExpected: %s\nGot: %s", string(content), string(downloaded))
	}

	// 8. Test Delete
	t.Logf("Deleting %s...", testFile)
	err = p.RemoveAll(testFile)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify remote file is gone
	if _, err := os.Stat(expectedRemotePath); !os.IsNotExist(err) {
		t.Errorf("Remote file should have been deleted: %s", expectedRemotePath)
	}
}
