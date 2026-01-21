package tests

import (
	"bytes"
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/webdav"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	localwebdav "golang.org/x/net/webdav"
)

// Helper to setup mock remote
func setupMockRemote(t *testing.T) (string, *httptest.Server) {
	mockRemoteDir, err := os.MkdirTemp("", "mock_remote_*")
	if err != nil {
		t.Fatal(err)
	}

	mockHandler := &localwebdav.Handler{
		FileSystem: localwebdav.Dir(mockRemoteDir),
		LockSystem: localwebdav.NewMemLS(),
	}
	ts := httptest.NewServer(mockHandler)

	return mockRemoteDir, ts
}

func TestExportImportWithDifferentKeys(t *testing.T) {
	// 1. Setup Mock Remote
	mockRemoteDir, ts := setupMockRemote(t)
	defer os.RemoveAll(mockRemoteDir)
	defer ts.Close()

	// 2. Setup Sender (User A)
	metaDirA, _ := os.MkdirTemp("", "meta_a_*")
	defer os.RemoveAll(metaDirA)

	cfgA := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}, // 32 bytes base64
	}
	metaA, _ := metadata.NewLocalStorage(metaDirA)
	remoteClientA, _ := webdav.NewClient(webdav.WebDAVConfig{URL: cfgA.Remote.URL})
	proxyA, _ := proxy.NewProxy(metaA, remoteClientA, cfgA.Security.MasterKey)

	// 3. Setup Recipient (User B)
	metaDirB, _ := os.MkdirTemp("", "meta_b_*")
	defer os.RemoveAll(metaDirB)

	cfgB := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="}, // Different key
	}
	metaB, _ := metadata.NewLocalStorage(metaDirB)
	remoteClientB, _ := webdav.NewClient(webdav.WebDAVConfig{URL: cfgB.Remote.URL})
	proxyB, _ := proxy.NewProxy(metaB, remoteClientB, cfgB.Security.MasterKey)

	// 4. Sender uploads a file
	testFile := "/shared_doc.txt"
	content := []byte("Secret Content Shared Between Users")
	err := proxyA.UploadFile(testFile, bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Sender upload failed: %v", err)
	}

	// 5. Sender creates share package
	exportDir, _ := os.MkdirTemp("", "export_*")
	defer os.RemoveAll(exportDir)

	shareKey := "secure-password-123"
	tarPath, err := proxyA.CreateSharePackage([]string{testFile}, exportDir, shareKey)
	if err != nil {
		t.Fatalf("CreateSharePackage failed: %v", err)
	}

	// 6. Recipient imports share package
	err = proxyB.ReceiveSharePackage(tarPath, shareKey)
	if err != nil {
		t.Fatalf("ReceiveSharePackage failed: %v", err)
	}

	// 7. Recipient tries to download file
	// Note: We assume Recipient has access to the same remote storage (or files are public/shared on remote)
	// Since we use the same mock server, they do access the same files.
	rc, err := proxyB.DownloadFile(testFile)
	if err != nil {
		t.Fatalf("Recipient DownloadFile failed: %v", err)
	}
	defer rc.Close()

	downloaded, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Recipient ReadAll failed: %v", err)
	}

	if !bytes.Equal(content, downloaded) {
		t.Errorf("Content mismatch! \nOriginal: %s\nDecrypted: %s", string(content), string(downloaded))
	} else {
		t.Log("Success: Recipient decrypted file shared by Sender with different Master Key")
	}
}

func TestEncryptCommandUsage(t *testing.T) {
	// 1. Setup Mock Remote
	mockRemoteDir, ts := setupMockRemote(t)
	defer os.RemoveAll(mockRemoteDir)
	defer ts.Close()

	// 2. Setup Local Environment for Encrypt Command
	metaDir, _ := os.MkdirTemp("", "meta_encrypt_*")
	defer os.RemoveAll(metaDir)

	inputDir, _ := os.MkdirTemp("", "input_*")
	defer os.RemoveAll(inputDir)

	outputDir, _ := os.MkdirTemp("", "output_*")
	defer os.RemoveAll(outputDir)

	// Create a local file to encrypt
	testFileName := "local_secret.txt"
	testFilePath := filepath.Join(inputDir, testFileName)
	content := []byte("Offline Encrypted Content")
	os.WriteFile(testFilePath, content, 0644)

	cfg := &config.Config{
		Remote:   config.RemoteConfig{URL: ts.URL},
		Security: config.SecurityConfig{MasterKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},
	}
	meta, _ := metadata.NewLocalStorage(metaDir)

	// We don't need remote storage for ExportLocal, passing nil
	proxyObj, _ := proxy.NewProxy(meta, nil, cfg.Security.MasterKey)

	// 3. Run ExportLocal (Simulate `encrypt` command)
	// It encrypts inputDir to outputDir and saves metadata to metaDir
	err := proxyObj.ExportLocal(testFilePath, outputDir)
	if err != nil {
		t.Fatalf("ExportLocal failed: %v", err)
	}

	// 4. Verify metadata exists
	metaObj, err := meta.Get("/" + testFileName)
	if err != nil || metaObj == nil {
		t.Fatalf("Metadata not found after ExportLocal")
	}

	// 5. Simulate Upload: Copy files from outputDir to mockRemoteDir
	// The file in outputDir should be named as metaObj.RemoteName
	encryptedPath := filepath.Join(outputDir, metaObj.RemoteName)
	remotePath := filepath.Join(mockRemoteDir, metaObj.RemoteName)

	data, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("Failed to read encrypted file: %v", err)
	}
	err = os.WriteFile(remotePath, data, 0644)
	if err != nil {
		t.Fatalf("Failed to upload to remote: %v", err)
	}

	// 6. Try to download using a Proxy with RemoteStorage
	remoteClient, _ := webdav.NewClient(webdav.WebDAVConfig{URL: cfg.Remote.URL})
	// Re-create proxy with remote storage
	proxyWithRemote, _ := proxy.NewProxy(meta, remoteClient, cfg.Security.MasterKey)

	rc, err := proxyWithRemote.DownloadFile("/" + testFileName)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer rc.Close()

	downloaded, _ := io.ReadAll(rc)
	if !bytes.Equal(content, downloaded) {
		t.Errorf("Content mismatch! \nOriginal: %s\nDecrypted: %s", string(content), string(downloaded))
	} else {
		t.Log("Success: Encrypt command metadata used successfully after upload")
	}
}
