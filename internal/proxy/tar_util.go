package proxy

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"clearvault/internal/crypto"
	"clearvault/internal/metadata"
)

// Manifest 清单文件结构
type Manifest struct {
	PackageID       string    `json:"package_id"`
	Version         string    `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	Encryption      string    `json:"encryption"`        // "rsa-aes"
	EncryptedAESKey []byte    `json:"encrypted_aes_key"` // 加密的 AES 密钥
	MetadataFiles   []string  `json:"metadata_files"`    // 加密的元数据文件列表
	TotalSize       int64     `json:"total_size"`
}

// TarPackage tar 包结构
type TarPackage struct {
	Manifest *Manifest `json:"manifest"`
}

// generateRandomIDForTar 生成随机 ID
func generateRandomIDForTar() string {
	// 随机数（8字节）
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)

	return hex.EncodeToString(randomBytes)
}

// generateRandomID 生成随机 ID（时间戳 + 随机数 + 路径文件名哈希）
func generateRandomID(virtualPath string) string {
	// 时间戳（纳秒）
	timestamp := time.Now().UnixNano()

	// 随机数（8字节）
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)

	// 解析路径和文件名
	dir := filepath.Dir(virtualPath)
	name := filepath.Base(virtualPath)

	// 组合路径 + 文件名作为哈希输入
	hashInput := dir + "/" + name
	hash := sha256.Sum256([]byte(hashInput))
	nameHash := hex.EncodeToString(hash[:8])

	// 组合：时间戳_随机数_路径文件名哈希
	return fmt.Sprintf("%d_%s_%s",
		timestamp,
		hex.EncodeToString(randomBytes),
		nameHash)
}

// CreateTarPackage 创建 tar 包（密钥模式）
func (p *Proxy) CreateTarPackage(
	paths []string,
	outputDir string,
	tempPrivKey *rsa.PrivateKey,
	tempPubKey *rsa.PublicKey,
	aesKey []byte,
) (string, error) {
	// 1. 生成包 ID（使用第一个路径作为参考）
	packageID := generateRandomIDForTar()

	// 2. 创建输出文件
	outputPath := filepath.Join(outputDir, fmt.Sprintf("share_%s.tar", packageID))
	file, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 3. 创建 tar writer
	tarWriter := tar.NewWriter(file)
	defer tarWriter.Close()

	// 4. 处理每个路径
	var metadataFiles []string
	var totalSize int64

	for _, path := range paths {
		meta, err := p.meta.Get(path)
		if err != nil {
			return "", err
		}

		if meta.IsDir {
			// 处理目录
			size, err := p.addDirectoryToTar(tarWriter, path, tempPubKey, aesKey, &metadataFiles)
			if err != nil {
				return "", err
			}
			totalSize += size
		} else {
			// 处理文件
			size, err := p.addFileToTar(tarWriter, path, meta, tempPubKey, aesKey, &metadataFiles)
			if err != nil {
				return "", err
			}
			totalSize += size
		}
	}

	// 5. 加密 AES 密钥
	rsaEngine := crypto.NewAsymmetricEngine(tempPubKey, nil)
	encryptedAESKey, err := rsaEngine.EncryptKey(aesKey)
	if err != nil {
		return "", err
	}

	// 6. 创建清单文件
	manifest := &Manifest{
		PackageID:       packageID,
		Version:         "1.0",
		CreatedAt:       time.Now(),
		Encryption:      "rsa-aes",
		EncryptedAESKey: encryptedAESKey,
		MetadataFiles:   metadataFiles,
		TotalSize:       totalSize,
	}

	// 7. 添加清单文件到 tar
	if err := p.addManifestToTar(tarWriter, manifest); err != nil {
		return "", err
	}

	return outputPath, nil
}

// addFileToTar 添加单个文件到 tar
func (p *Proxy) addFileToTar(
	tarWriter *tar.Writer,
	virtualPath string,
	meta *metadata.FileMeta,
	tempPubKey *rsa.PublicKey,
	aesKey []byte,
	metadataFiles *[]string,
) (int64, error) {
	// 1. 为元数据注入 path 字段（目录路径，不包含文件名）
	dirPath := filepath.Dir(virtualPath)
	meta.Path = dirPath

	// 2. 序列化元数据
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return 0, err
	}

	// 3. 使用 AES 密钥加密元数据
	aesEngine, err := crypto.NewEngine(aesKey)
	if err != nil {
		return 0, err
	}

	nonce := make([]byte, 12)
	buf := &bytes.Buffer{}
	err = aesEngine.EncryptStream(bytes.NewReader(metaJSON), buf, nonce)
	if err != nil {
		return 0, err
	}
	encryptedMeta := buf.Bytes()

	// 4. 生成随机文件名（包含路径和文件名哈希）
	metaFileName := generateRandomID(virtualPath) + ".enc"

	// 5. 写入加密的元数据到 tar
	metaHeader := &tar.Header{
		Name: fmt.Sprintf("metadata/%s", metaFileName),
		Mode: 0644,
		Size: int64(len(encryptedMeta)),
	}
	if err := tarWriter.WriteHeader(metaHeader); err != nil {
		return 0, err
	}
	if _, err := tarWriter.Write(encryptedMeta); err != nil {
		return 0, err
	}

	// 6. 记录元数据文件名
	*metadataFiles = append(*metadataFiles, metaFileName)

	return int64(len(encryptedMeta)), nil
}

// addDirectoryToTar 添加目录到 tar
func (p *Proxy) addDirectoryToTar(
	tarWriter *tar.Writer,
	virtualPath string,
	tempPubKey *rsa.PublicKey,
	aesKey []byte,
	metadataFiles *[]string,
) (int64, error) {
	var totalSize int64

	// 读取目录
	children, err := p.meta.ReadDir(virtualPath)
	if err != nil {
		return 0, err
	}

	for _, child := range children {
		childPath := filepath.Join(virtualPath, child.Name)

		if child.IsDir {
			// 递归处理子目录
			size, err := p.addDirectoryToTar(tarWriter, childPath, tempPubKey, aesKey, metadataFiles)
			if err != nil {
				return 0, err
			}
			totalSize += size
		} else {
			// 处理文件
			size, err := p.addFileToTar(tarWriter, childPath, &child, tempPubKey, aesKey, metadataFiles)
			if err != nil {
				return 0, err
			}
			totalSize += size
		}
	}

	return totalSize, nil
}

// addManifestToTar 添加清单文件到 tar
func (p *Proxy) addManifestToTar(tarWriter *tar.Writer, manifest *Manifest) error {
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name: "manifest.json",
		Mode: 0644,
		Size: int64(len(manifestJSON)),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = tarWriter.Write(manifestJSON)
	return err
}

// ExtractTarPackage 解压 tar 包
func (p *Proxy) ExtractTarPackage(
	tarPath string,
	outputDir string,
	privateKey *rsa.PrivateKey,
) (*TarPackage, error) {
	// 1. 打开 tar 文件
	file, err := os.Open(tarPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 2. 创建 tar reader
	tarReader := tar.NewReader(file)

	// 3. 解析结果
	var manifest Manifest
	extractedMetadata := make(map[string][]byte)

	// 4. 遍历 tar 条目
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// 读取内容
		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, err
		}

		// 根据文件名处理
		switch {
		case header.Name == "manifest.json":
			// 解析清单
			if err := json.Unmarshal(content, &manifest); err != nil {
				return nil, err
			}

		case filepath.Dir(header.Name) == "metadata":
			// 存储加密的元数据
			fileName := filepath.Base(header.Name)
			extractedMetadata[fileName] = content
		}
	}

	// 5. 使用私钥解密 AES 密钥
	rsaEngine := crypto.NewAsymmetricEngine(nil, privateKey)
	aesKey, err := rsaEngine.DecryptKey(manifest.EncryptedAESKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// 6. 遍历 metadata/ 目录下的所有 .enc 文件
	for _, metaFile := range manifest.MetadataFiles {
		// 从 tar 包提取加密的元数据文件
		encryptedMeta, exists := extractedMetadata[metaFile]
		if !exists {
			continue
		}

		// 使用 AES 密钥解密元数据
		aesEngine, err := crypto.NewEngine(aesKey)
		if err != nil {
			return nil, err
		}

		nonce := make([]byte, 12)
		buf := &bytes.Buffer{}
		err = aesEngine.DecryptStream(bytes.NewReader(encryptedMeta), buf, nonce)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt metadata %s: %w", metaFile, err)
		}
		metaJSON := buf.Bytes()

		// 反序列化元数据
		var meta metadata.FileMeta
		if err := json.Unmarshal(metaJSON, &meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata %s: %w", metaFile, err)
		}

		// 使用主密钥重新加密 FEK
		encryptedFEK, err := p.encryptFEK(meta.FEK)
		if err != nil {
			return nil, fmt.Errorf("failed to re-encrypt FEK for %s: %w", meta.Name, err)
		}

		// 更新元数据
		meta.FEK = encryptedFEK

		// 保存到本地存储（使用 path + name 构建虚拟路径）
		virtualPath := filepath.Join(meta.Path, meta.Name)
		if err := p.meta.Save(&meta, virtualPath); err != nil {
			return nil, fmt.Errorf("failed to save metadata for %s: %w", virtualPath, err)
		}
	}

	return &TarPackage{Manifest: &manifest}, nil
}
