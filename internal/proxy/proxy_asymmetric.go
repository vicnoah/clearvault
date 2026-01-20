package proxy

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/pbkdf2"

	"clearvault/internal/crypto"
	"clearvault/internal/key"
)

// CreateSharePackage 创建分享包（密钥模式）
func (p *Proxy) CreateSharePackage(
	paths []string,
	outputDir string,
	shareKey string,
) (string, error) {
	// 1. 验证输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}

	// 2. 从密码派生密钥
	keyBytes := deriveKeyFromPassword(shareKey)

	// 3. 生成临时 RSA-2048 密钥对
	tempPriv, tempPub, err := generateTempRSAKeyPair()
	if err != nil {
		return "", err
	}

	// 4. 生成随机 AES-256 密钥（用于加密元数据）
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return "", err
	}

	// 5. 创建 tar 包（使用临时公钥加密 FEK）
	tarPath, err := p.CreateTarPackage(
		paths,
		outputDir,
		tempPriv,
		tempPub,
		aesKey,
	)
	if err != nil {
		return "", err
	}

	// 6. 序列化临时私钥
	keyManager, err := key.NewKeyManager()
	if err != nil {
		return "", err
	}
	privKeyPEM, err := keyManager.SerializePrivateKey(tempPriv)
	if err != nil {
		return "", err
	}

	// 7. 使用派生密钥加密临时私钥
	encryptedPrivKey, err := encryptPrivateKeyWithKey(privKeyPEM, keyBytes)
	if err != nil {
		return "", err
	}

	// 8. 将加密的私钥附加到 tar 包
	if err := p.appendEncryptedKey(tarPath, encryptedPrivKey); err != nil {
		return "", err
	}

	return tarPath, nil
}

// ReceiveSharePackage 接收分享包（密钥模式）
func (p *Proxy) ReceiveSharePackage(
	tarPath string,
	shareKey string,
) error {
	// 1. 从密码派生密钥
	keyBytes := deriveKeyFromPassword(shareKey)

	// 2. 从 tar 包提取加密的私钥
	encryptedPrivKey, err := p.extractEncryptedKey(tarPath)
	if err != nil {
		return err
	}

	// 3. 使用派生密钥解密私钥
	privKeyPEM, err := decryptPrivateKeyWithKey(encryptedPrivKey, keyBytes)
	if err != nil {
		return err
	}

	// 4. 反序列化私钥
	keyManager, err := key.NewKeyManager()
	if err != nil {
		return err
	}
	privateKey, err := keyManager.DeserializePrivateKey(privKeyPEM)
	if err != nil {
		return err
	}

	// 5. 创建临时目录
	tempDir, err := os.MkdirTemp("", "clearvault_import_*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// 6. 解压 tar 包（使用临时私钥解密 FEK）
	_, err = p.ExtractTarPackage(tarPath, tempDir, privateKey)
	if err != nil {
		return err
	}

	return nil
}

// 辅助函数：从密码派生密钥
func deriveKeyFromPassword(password string) []byte {
	// 使用 PBKDF2 派生 32 字节密钥
	salt := []byte("clearvault-share-salt-v1")
	return pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)
}

// 辅助函数：生成临时 RSA 密钥对
func generateTempRSAKeyPair() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	keyManager, err := key.NewKeyManager()
	if err != nil {
		return nil, nil, err
	}
	return keyManager.GenerateTempKeyPair(2048)
}

// 辅助函数：使用对称密钥加密私钥
func encryptPrivateKeyWithKey(privKeyPEM []byte, key []byte) ([]byte, error) {
	// 使用 AES-GCM 加密
	engine, err := crypto.NewEngine(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 12)
	buf := &bytes.Buffer{}
	err = engine.EncryptStream(bytes.NewReader(privKeyPEM), buf, nonce)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// 辅助函数：使用对称密钥解密私钥
func decryptPrivateKeyWithKey(encryptedKey []byte, key []byte) ([]byte, error) {
	// 使用 AES-GCM 解密
	engine, err := crypto.NewEngine(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 12)
	buf := &bytes.Buffer{}
	err = engine.DecryptStream(bytes.NewReader(encryptedKey), buf, nonce)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// 辅助函数：追加加密的私钥到 tar 包
func (p *Proxy) appendEncryptedKey(tarPath string, encryptedKey []byte) error {
	// 1. 截断文件，删除原来的结束标记（两个 512 字节的全零块）
	file, err := os.OpenFile(tarPath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// 获取文件大小
	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// 截断文件，删除最后的 1024 字节（两个 512 字节的结束标记）
	newSize := stat.Size() - 1024
	if newSize < 0 {
		newSize = 0
	}

	if err := file.Truncate(newSize); err != nil {
		return err
	}

	// 2. 定位到文件末尾
	if _, err := file.Seek(newSize, 0); err != nil {
		return err
	}

	// 3. 创建 tar writer 并写入加密的私钥
	tarWriter := tar.NewWriter(file)

	header := &tar.Header{
		Name: "private_key.enc",
		Mode: 0600,
		Size: int64(len(encryptedKey)),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		tarWriter.Close()
		return err
	}

	if _, err := tarWriter.Write(encryptedKey); err != nil {
		tarWriter.Close()
		return err
	}

	// 4. 关闭 tar writer（写入新的结束标记）
	if err := tarWriter.Close(); err != nil {
		return err
	}

	// 5. 强制刷新文件缓冲区
	if err := file.Sync(); err != nil {
		return err
	}

	return nil
}

// 辅助函数：从 tar 包提取加密的私钥
func (p *Proxy) extractEncryptedKey(tarPath string) ([]byte, error) {
	file, err := os.Open(tarPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	tarReader := tar.NewReader(file)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == "private_key.enc" {
			return io.ReadAll(tarReader)
		}
	}

	return nil, fmt.Errorf("encrypted private key not found in tar package")
}
