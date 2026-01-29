package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
)

// TestNewAsymmetricEngine tests creating a new asymmetric engine
func TestNewAsymmetricEngine(t *testing.T) {
	// Generate a test key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	tests := []struct {
		name       string
		publicKey  *rsa.PublicKey
		privateKey *rsa.PrivateKey
		wantNil    bool
	}{
		{
			name:       "With both keys",
			publicKey:  &privateKey.PublicKey,
			privateKey: privateKey,
			wantNil:    false,
		},
		{
			name:       "With only public key",
			publicKey:  &privateKey.PublicKey,
			privateKey: nil,
			wantNil:    false,
		},
		{
			name:       "With only private key",
			publicKey:  nil,
			privateKey: privateKey,
			wantNil:    false,
		},
		{
			name:       "With no keys",
			publicKey:  nil,
			privateKey: nil,
			wantNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewAsymmetricEngine(tt.publicKey, tt.privateKey)
			if (engine == nil) != tt.wantNil {
				t.Errorf("NewAsymmetricEngine() = %v, wantNil %v", engine, tt.wantNil)
			}
			if engine != nil {
				if engine.publicKey != tt.publicKey {
					t.Error("Public key mismatch")
				}
				if engine.privateKey != tt.privateKey {
					t.Error("Private key mismatch")
				}
			}
		})
	}
}

// TestAsymmetricEngine_EncryptDecryptRoundTrip tests full encrypt/decrypt cycle
func TestAsymmetricEngine_EncryptDecryptRoundTrip(t *testing.T) {
	// Generate a test key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	engine := NewAsymmetricEngine(&privateKey.PublicKey, privateKey)

	// Test data - must be smaller than key size minus padding (2048/8 - 2*32 - 2 = 190 bytes for OAEP)
	testData := []byte("this is a secret key for encryption testing")

	// Encrypt
	ciphertext, err := engine.EncryptKey(testData)
	if err != nil {
		t.Fatalf("EncryptKey() failed: %v", err)
	}

	// Verify ciphertext is different from plaintext
	if bytes.Equal(ciphertext, testData) {
		t.Error("Ciphertext should be different from plaintext")
	}

	// Decrypt
	decrypted, err := engine.DecryptKey(ciphertext)
	if err != nil {
		t.Fatalf("DecryptKey() failed: %v", err)
	}

	// Verify decrypted matches original
	if !bytes.Equal(decrypted, testData) {
		t.Errorf("Decrypted data doesn't match original.\nOriginal: %v\nDecrypted: %v", testData, decrypted)
	}
}

// TestAsymmetricEngine_EncryptKey_NilPublicKey tests encryption with nil public key
func TestAsymmetricEngine_EncryptKey_NilPublicKey(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	engine := NewAsymmetricEngine(nil, privateKey)

	_, err := engine.EncryptKey([]byte("test data"))
	if err == nil {
		t.Error("EncryptKey() with nil public key should return error")
	}
}

// TestAsymmetricEngine_DecryptKey_NilPrivateKey tests decryption with nil private key
func TestAsymmetricEngine_DecryptKey_NilPrivateKey(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	engine := NewAsymmetricEngine(&privateKey.PublicKey, nil)

	_, err := engine.DecryptKey([]byte("test data"))
	if err == nil {
		t.Error("DecryptKey() with nil private key should return error")
	}
}

// TestAsymmetricEngine_EncryptKey_DifferentKeys tests that different keys produce different ciphertexts
func TestAsymmetricEngine_EncryptKey_DifferentKeys(t *testing.T) {
	// Generate two different key pairs
	privateKey1, _ := rsa.GenerateKey(rand.Reader, 2048)
	privateKey2, _ := rsa.GenerateKey(rand.Reader, 2048)

	engine1 := NewAsymmetricEngine(&privateKey1.PublicKey, nil)
	engine2 := NewAsymmetricEngine(&privateKey2.PublicKey, nil)

	testData := []byte("test data for encryption")

	ciphertext1, err := engine1.EncryptKey(testData)
	if err != nil {
		t.Fatalf("EncryptKey() with key1 failed: %v", err)
	}

	ciphertext2, err := engine2.EncryptKey(testData)
	if err != nil {
		t.Fatalf("EncryptKey() with key2 failed: %v", err)
	}

	// Due to OAEP randomness, even same key produces different ciphertexts
	// But we can verify both decrypt correctly with their respective keys
	engine1Full := NewAsymmetricEngine(&privateKey1.PublicKey, privateKey1)
	engine2Full := NewAsymmetricEngine(&privateKey2.PublicKey, privateKey2)

	decrypted1, err := engine1Full.DecryptKey(ciphertext1)
	if err != nil {
		t.Fatalf("DecryptKey() with key1 failed: %v", err)
	}

	decrypted2, err := engine2Full.DecryptKey(ciphertext2)
	if err != nil {
		t.Fatalf("DecryptKey() with key2 failed: %v", err)
	}

	if !bytes.Equal(decrypted1, testData) {
		t.Error("Key1: decrypted data doesn't match original")
	}
	if !bytes.Equal(decrypted2, testData) {
		t.Error("Key2: decrypted data doesn't match original")
	}
}

// TestAsymmetricEngine_WrongKeyDecryption tests decryption with wrong key
func TestAsymmetricEngine_WrongKeyDecryption(t *testing.T) {
	// Generate two different key pairs
	privateKey1, _ := rsa.GenerateKey(rand.Reader, 2048)
	privateKey2, _ := rsa.GenerateKey(rand.Reader, 2048)

	// Encrypt with key1
	engine1 := NewAsymmetricEngine(&privateKey1.PublicKey, nil)
	testData := []byte("test data for encryption")
	ciphertext, _ := engine1.EncryptKey(testData)

	// Try to decrypt with key2
	engine2 := NewAsymmetricEngine(&privateKey2.PublicKey, privateKey2)
	_, err := engine2.DecryptKey(ciphertext)
	if err == nil {
		t.Error("Decrypting with wrong key should fail")
	}
}

// TestAsymmetricEngine_CorruptedCiphertext tests decryption of corrupted data
func TestAsymmetricEngine_CorruptedCiphertext(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	engine := NewAsymmetricEngine(&privateKey.PublicKey, privateKey)

	testData := []byte("test data for encryption")
	ciphertext, _ := engine.EncryptKey(testData)

	// Corrupt the ciphertext
	ciphertext[0] ^= 0xFF
	ciphertext[10] ^= 0xFF

	_, err := engine.DecryptKey(ciphertext)
	if err == nil {
		t.Error("Decrypting corrupted ciphertext should fail")
	}
}

// TestAsymmetricEngine_LargeData tests encryption of data at the size limit
func TestAsymmetricEngine_LargeData(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	engine := NewAsymmetricEngine(&privateKey.PublicKey, privateKey)

	// For RSA-2048 with OAEP-SHA256, max message size is 190 bytes
	// (256 - 2*32 - 2 = 190)
	largeData := make([]byte, 190)
	rand.Read(largeData)

	ciphertext, err := engine.EncryptKey(largeData)
	if err != nil {
		t.Fatalf("EncryptKey() with max size data failed: %v", err)
	}

	decrypted, err := engine.DecryptKey(ciphertext)
	if err != nil {
		t.Fatalf("DecryptKey() failed: %v", err)
	}

	if !bytes.Equal(decrypted, largeData) {
		t.Error("Decrypted large data doesn't match original")
	}
}

// TestAsymmetricEngine_TooLargeData tests encryption of data exceeding the size limit
func TestAsymmetricEngine_TooLargeData(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	engine := NewAsymmetricEngine(&privateKey.PublicKey, privateKey)

	// Data larger than 190 bytes should fail
	tooLargeData := make([]byte, 200)
	rand.Read(tooLargeData)

	_, err := engine.EncryptKey(tooLargeData)
	if err == nil {
		t.Error("Encrypting data larger than key capacity should fail")
	}
}

// TestAsymmetricEngine_EmptyData tests encryption of empty data
func TestAsymmetricEngine_EmptyData(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	engine := NewAsymmetricEngine(&privateKey.PublicKey, privateKey)

	emptyData := []byte{}

	ciphertext, err := engine.EncryptKey(emptyData)
	if err != nil {
		t.Fatalf("EncryptKey() with empty data failed: %v", err)
	}

	decrypted, err := engine.DecryptKey(ciphertext)
	if err != nil {
		t.Fatalf("DecryptKey() failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Error("Decrypted empty data should be empty")
	}
}

// TestAsymmetricEngine_DifferentKeySizes tests with different RSA key sizes
func TestAsymmetricEngine_DifferentKeySizes(t *testing.T) {
	keySizes := []int{2048, 3072, 4096}

	for _, size := range keySizes {
		t.Run(fmt.Sprintf("RSA-%d", size), func(t *testing.T) {
			privateKey, err := rsa.GenerateKey(rand.Reader, size)
			if err != nil {
				t.Fatalf("Failed to generate %d-bit key: %v", size, err)
			}

			engine := NewAsymmetricEngine(&privateKey.PublicKey, privateKey)
			testData := []byte("test data for different key sizes")

			ciphertext, err := engine.EncryptKey(testData)
			if err != nil {
				t.Fatalf("EncryptKey() failed: %v", err)
			}

			decrypted, err := engine.DecryptKey(ciphertext)
			if err != nil {
				t.Fatalf("DecryptKey() failed: %v", err)
			}

			if !bytes.Equal(decrypted, testData) {
				t.Errorf("RSA-%d: decrypted data doesn't match original", size)
			}
		})
	}
}

// TestAsymmetricEngine_MultipleEncryptions tests multiple encryptions produce different ciphertexts
func TestAsymmetricEngine_MultipleEncryptions(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	engine := NewAsymmetricEngine(&privateKey.PublicKey, privateKey)

	testData := []byte("test data")

	ciphertexts := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		ct, err := engine.EncryptKey(testData)
		if err != nil {
			t.Fatalf("Encryption %d failed: %v", i, err)
		}
		ciphertexts[i] = ct

		// Verify each can be decrypted
		decrypted, err := engine.DecryptKey(ct)
		if err != nil {
			t.Fatalf("Decryption %d failed: %v", i, err)
		}
		if !bytes.Equal(decrypted, testData) {
			t.Errorf("Decryption %d: data mismatch", i)
		}
	}

	// Due to OAEP randomness, ciphertexts should be different
	allSame := true
	for i := 1; i < len(ciphertexts); i++ {
		if !bytes.Equal(ciphertexts[0], ciphertexts[i]) {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("Multiple encryptions should produce different ciphertexts due to OAEP randomness")
	}
}
