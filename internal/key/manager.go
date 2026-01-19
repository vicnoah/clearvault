package key

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// KeyManager 管理临时 RSA 密钥对（用于分享）
type KeyManager struct{}

// NewKeyManager 创建密钥管理器
func NewKeyManager() (*KeyManager, error) {
	return &KeyManager{}, nil
}

// GenerateTempKeyPair 生成临时 RSA 密钥对
func (km *KeyManager) GenerateTempKeyPair(bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}
	return privateKey, &privateKey.PublicKey, nil
}

// SerializePrivateKey 序列化私钥为 PEM 格式
func (km *KeyManager) SerializePrivateKey(privKey *rsa.PrivateKey) ([]byte, error) {
	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privKeyBytes,
	}
	return pem.EncodeToMemory(pemBlock), nil
}

// DeserializePrivateKey 反序列化 PEM 格式的私钥
func (km *KeyManager) DeserializePrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// SerializePublicKey 序列化公钥为 PEM 格式
func (km *KeyManager) SerializePublicKey(pubKey *rsa.PublicKey) ([]byte, error) {
	pubKeyBytes := x509.MarshalPKCS1PublicKey(pubKey)
	pemBlock := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubKeyBytes,
	}
	return pem.EncodeToMemory(pemBlock), nil
}

// DeserializePublicKey 反序列化 PEM 格式的公钥
func (km *KeyManager) DeserializePublicKey(pemData []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParsePKCS1PublicKey(block.Bytes)
}
