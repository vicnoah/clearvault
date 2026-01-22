package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"clearvault/internal/key"
	"clearvault/internal/metadata"
)

func TestCreateTarPackage(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "clearvault_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建临时 RSA 密钥对
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}
	pub := &priv.PublicKey

	// 创建 AES 密钥
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatalf("Failed to generate AES key: %v", err)
	}

	// 创建元数据存储
	metaStorage, err := metadata.NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("Failed to create metadata storage: %v", err)
	}
	defer metaStorage.Close()

	// 创建测试元数据
	fek := make([]byte, 32)
	if _, err := rand.Read(fek); err != nil {
		t.Fatalf("Failed to generate FEK: %v", err)
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("Failed to generate salt: %v", err)
	}
	testMeta := &metadata.FileMeta{
		Name:       "test.txt",
		RemoteName: "abc123",
		IsDir:      false,
		Size:       1024,
		FEK:        nil,
		Salt:       salt,
		Path:       "/",
	}

	masterKeyRaw := make([]byte, 32)
	if _, err := rand.Read(masterKeyRaw); err != nil {
		t.Fatalf("Failed to generate master key: %v", err)
	}
	p, err := NewProxy(metaStorage, nil, base64.StdEncoding.EncodeToString(masterKeyRaw))
	if err != nil {
		t.Fatalf("Failed to initialize proxy: %v", err)
	}
	encryptedFEK, err := p.encryptFEK(fek)
	if err != nil {
		t.Fatalf("Failed to encrypt FEK: %v", err)
	}
	testMeta.FEK = encryptedFEK

	// 保存元数据
	err = metaStorage.Save(testMeta, "/test.txt")
	if err != nil {
		t.Fatalf("Failed to save metadata: %v", err)
	}

	// 创建 tar 包
	tarPath, err := p.CreateTarPackage(
		[]string{"/test.txt"},
		tempDir,
		priv,
		pub,
		aesKey,
	)
	if err != nil {
		t.Fatalf("Failed to create tar package: %v", err)
	}

	// 验证 tar 文件存在
	if _, err := os.Stat(tarPath); os.IsNotExist(err) {
		t.Fatalf("Tar file does not exist: %s", tarPath)
	}

	// 验证 tar 文件可以读取
	file, err := os.Open(tarPath)
	if err != nil {
		t.Fatalf("Failed to open tar file: %v", err)
	}
	defer file.Close()

	// 验证文件大小大于 0
	stat, err := file.Stat()
	if err != nil {
		t.Fatalf("Failed to stat tar file: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatalf("Tar file is empty")
	}

	t.Logf("Created tar package: %s (size: %d bytes)", tarPath, stat.Size())
}

func TestExtractTarPackage(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "clearvault_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建临时 RSA 密钥对
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}
	pub := &priv.PublicKey

	// 创建 AES 密钥
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatalf("Failed to generate AES key: %v", err)
	}

	// 创建元数据存储
	metaStorage, err := metadata.NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("Failed to create metadata storage: %v", err)
	}
	defer metaStorage.Close()

	// 创建测试元数据
	fek := make([]byte, 32)
	if _, err := rand.Read(fek); err != nil {
		t.Fatalf("Failed to generate FEK: %v", err)
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("Failed to generate salt: %v", err)
	}
	testMeta := &metadata.FileMeta{
		Name:       "test.txt",
		RemoteName: "abc123",
		IsDir:      false,
		Size:       1024,
		FEK:        nil,
		Salt:       salt,
		Path:       "/",
	}

	masterKey := make([]byte, 32)
	if _, err := rand.Read(masterKey); err != nil {
		t.Fatalf("Failed to generate master key: %v", err)
	}
	proxy := &Proxy{meta: metaStorage, masterKey: masterKey, pendingCache: NewPendingFileCache()}
	encryptedFEK, err := proxy.encryptFEK(fek)
	if err != nil {
		t.Fatalf("Failed to encrypt FEK: %v", err)
	}
	testMeta.FEK = encryptedFEK

	err = metaStorage.Save(testMeta, "/test.txt")
	if err != nil {
		t.Fatalf("Failed to save metadata: %v", err)
	}

	tarPath, err := proxy.CreateTarPackage(
		[]string{"/test.txt"},
		tempDir,
		priv,
		pub,
		aesKey,
	)
	if err != nil {
		t.Fatalf("Failed to create tar package: %v", err)
	}

	// 解压 tar 包
	pkg, err := proxy.ExtractTarPackage(tarPath, tempDir, priv)
	if err != nil {
		t.Fatalf("Failed to extract tar package: %v", err)
	}

	// 验证清单
	if pkg.Manifest == nil {
		t.Fatal("Manifest is nil")
	}
	if pkg.Manifest.PackageID == "" {
		t.Fatal("PackageID is empty")
	}
	if pkg.Manifest.Encryption != "rsa-aes" {
		t.Fatalf("Expected encryption type 'rsa-aes', got '%s'", pkg.Manifest.Encryption)
	}

	t.Logf("Extracted tar package successfully")
	t.Logf("Package ID: %s", pkg.Manifest.PackageID)
	t.Logf("Total size: %d", pkg.Manifest.TotalSize)
}

func TestEncryptDecryptPrivateKey(t *testing.T) {
	// 创建测试私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// 创建测试密钥
	testKey := make([]byte, 32)
	if _, err := rand.Read(testKey); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// 序列化私钥
	keyManager, err := key.NewKeyManager()
	if err != nil {
		t.Fatalf("Failed to create key manager: %v", err)
	}
	privKeyPEM, err := keyManager.SerializePrivateKey(priv)
	if err != nil {
		t.Fatalf("Failed to serialize private key: %v", err)
	}

	// 加密私钥
	encryptedPrivKey, err := encryptPrivateKeyWithKey(privKeyPEM, testKey)
	if err != nil {
		t.Fatalf("Failed to encrypt private key: %v", err)
	}

	// 解密私钥
	decryptedPrivKeyPEM, err := decryptPrivateKeyWithKey(encryptedPrivKey, testKey)
	if err != nil {
		t.Fatalf("Failed to decrypt private key: %v", err)
	}

	// 验证解密后的私钥与原始私钥相同
	if string(privKeyPEM) != string(decryptedPrivKeyPEM) {
		t.Fatal("Decrypted private key does not match original")
	}

	t.Log("Private key encryption/decryption successful")
}

func TestDeriveKeyFromPassword(t *testing.T) {
	password := "test-password-123"
	key1 := deriveKeyFromPassword(password)
	key2 := deriveKeyFromPassword(password)

	// 验证相同密码生成相同的密钥
	if len(key1) != 32 || len(key2) != 32 {
		t.Fatalf("Expected key length 32, got %d and %d", len(key1), len(key2))
	}

	// 验证密钥相同
	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("Same password should generate same key")
		}
	}

	t.Log("Key derivation successful")
}

func TestGenerateRandomID(t *testing.T) {
	id1 := generateRandomID("/test/file1.txt")
	id2 := generateRandomID("/test/file2.txt")

	// 验证 ID 长度（时间戳 + 随机数 + 哈希）
	// 时间戳：约 19 位，随机数：16 位，哈希：16 位，分隔符：2 位 = 约 53 位
	if len(id1) < 40 {
		t.Fatalf("Expected ID length >= 40, got %d", len(id1))
	}
	if len(id2) < 40 {
		t.Fatalf("Expected ID length >= 40, got %d", len(id2))
	}

	// 验证 ID 不同（不同文件名）
	if id1 == id2 {
		t.Fatal("Random IDs should be different for different paths")
	}

	// 验证相同路径生成不同的 ID（因为时间戳不同）
	id3 := generateRandomID("/test/file1.txt")
	if id1 == id3 {
		t.Fatal("Same path should generate different ID (timestamp differs)")
	}

	// 验证不同目录下的同名文件生成不同的 ID
	id4 := generateRandomID("/other/file1.txt")
	if id1 == id4 {
		t.Fatal("Different directories should generate different IDs")
	}

	// 验证相同目录下的不同文件生成不同的 ID
	id5 := generateRandomID("/test/file3.txt")
	if id1 == id5 {
		t.Fatal("Different files in same directory should generate different IDs")
	}

	t.Logf("Generated random IDs: %s, %s, %s, %s, %s", id1, id2, id3, id4, id5)
}

func TestAppendExtractEncryptedKey(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "clearvault_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建一个空的 tar 文件
	tarPath := filepath.Join(tempDir, "test.tar")
	file, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("Failed to create tar file: %v", err)
	}
	file.Close()

	// 创建测试加密密钥
	encryptedKey := []byte("test-encrypted-key")

	// 创建代理
	proxy := &Proxy{}

	// 追加加密的私钥
	err = proxy.appendEncryptedKey(tarPath, encryptedKey)
	if err != nil {
		t.Fatalf("Failed to append encrypted key: %v", err)
	}

	// 提取加密的私钥
	extractedKey, err := proxy.extractEncryptedKey(tarPath)
	if err != nil {
		t.Fatalf("Failed to extract encrypted key: %v", err)
	}

	// 验证提取的密钥与原始密钥相同
	if string(extractedKey) != string(encryptedKey) {
		t.Fatal("Extracted key does not match original")
	}

	t.Log("Append/extract encrypted key successful")
}
