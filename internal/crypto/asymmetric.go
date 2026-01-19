package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
)

// AsymmetricEngine RSA 非对称加密引擎
type AsymmetricEngine struct {
	publicKey  *rsa.PublicKey
	privateKey *rsa.PrivateKey
}

// NewAsymmetricEngine 创建非对称加密引擎
func NewAsymmetricEngine(publicKey *rsa.PublicKey, privateKey *rsa.PrivateKey) *AsymmetricEngine {
	return &AsymmetricEngine{
		publicKey:  publicKey,
		privateKey: privateKey,
	}
}

// EncryptKey 使用公钥加密对称密钥（RSA-OAEP）
func (e *AsymmetricEngine) EncryptKey(plaintext []byte) ([]byte, error) {
	if e.publicKey == nil {
		return nil, fmt.Errorf("public key is nil")
	}

	// 使用 OAEP 填充（更安全）
	ciphertext, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		e.publicKey,
		plaintext,
		nil, // label (可选)
	)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key: %w", err)
	}

	return ciphertext, nil
}

// DecryptKey 使用私钥解密对称密钥（RSA-OAEP）
func (e *AsymmetricEngine) DecryptKey(ciphertext []byte) ([]byte, error) {
	if e.privateKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}

	plaintext, err := rsa.DecryptOAEP(
		sha256.New(),
		rand.Reader,
		e.privateKey,
		ciphertext,
		nil, // label (可选)
	)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	return plaintext, nil
}
