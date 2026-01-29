package key

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// TestNewKeyManager tests creating a new key manager
func TestNewKeyManager(t *testing.T) {
	km, err := NewKeyManager()
	if err != nil {
		t.Fatalf("NewKeyManager() failed: %v", err)
	}
	if km == nil {
		t.Error("NewKeyManager() returned nil")
	}
}

// TestKeyManager_GenerateTempKeyPair tests RSA key pair generation
func TestKeyManager_GenerateTempKeyPair(t *testing.T) {
	km, _ := NewKeyManager()

	tests := []struct {
		name    string
		bits    int
		wantErr bool
	}{
		{
			name:    "RSA-2048",
			bits:    2048,
			wantErr: false,
		},
		{
			name:    "RSA-3072",
			bits:    3072,
			wantErr: false,
		},
		{
			name:    "RSA-4096",
			bits:    4096,
			wantErr: false,
		},
		{
			name:    "RSA-1024",
			bits:    1024,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			privKey, pubKey, err := km.GenerateTempKeyPair(tt.bits)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateTempKeyPair() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if privKey == nil {
				t.Error("GenerateTempKeyPair() returned nil private key")
			}
			if pubKey == nil {
				t.Error("GenerateTempKeyPair() returned nil public key")
			}

			// Verify key size
			if privKey.N.BitLen() != tt.bits {
				t.Errorf("Private key has %d bits, want %d", privKey.N.BitLen(), tt.bits)
			}

			// Verify public key matches private key
			if pubKey.N.Cmp(privKey.N) != 0 {
				t.Error("Public key modulus doesn't match private key")
			}
			if pubKey.E != privKey.E {
				t.Error("Public key exponent doesn't match private key")
			}
		})
	}
}

// TestKeyManager_SerializeDeserializePrivateKey tests private key serialization round-trip
func TestKeyManager_SerializeDeserializePrivateKey(t *testing.T) {
	km, _ := NewKeyManager()

	// Generate a key pair
	privKey, _, err := km.GenerateTempKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Serialize
	pemData, err := km.SerializePrivateKey(privKey)
	if err != nil {
		t.Fatalf("SerializePrivateKey() failed: %v", err)
	}

	// Verify PEM format
	if !strings.Contains(string(pemData), "BEGIN RSA PRIVATE KEY") {
		t.Error("Serialized private key doesn't contain expected PEM header")
	}
	if !strings.Contains(string(pemData), "END RSA PRIVATE KEY") {
		t.Error("Serialized private key doesn't contain expected PEM footer")
	}

	// Deserialize
	deserializedKey, err := km.DeserializePrivateKey(pemData)
	if err != nil {
		t.Fatalf("DeserializePrivateKey() failed: %v", err)
	}

	// Verify keys match
	if deserializedKey.N.Cmp(privKey.N) != 0 {
		t.Error("Deserialized private key modulus doesn't match original")
	}
	if deserializedKey.D.Cmp(privKey.D) != 0 {
		t.Error("Deserialized private key exponent doesn't match original")
	}
}

// TestKeyManager_SerializeDeserializePublicKey tests public key serialization round-trip
func TestKeyManager_SerializeDeserializePublicKey(t *testing.T) {
	km, _ := NewKeyManager()

	// Generate a key pair
	privKey, _, err := km.GenerateTempKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}
	pubKey := &privKey.PublicKey

	// Serialize
	pemData, err := km.SerializePublicKey(pubKey)
	if err != nil {
		t.Fatalf("SerializePublicKey() failed: %v", err)
	}

	// Verify PEM format
	if !strings.Contains(string(pemData), "BEGIN RSA PUBLIC KEY") {
		t.Error("Serialized public key doesn't contain expected PEM header")
	}
	if !strings.Contains(string(pemData), "END RSA PUBLIC KEY") {
		t.Error("Serialized public key doesn't contain expected PEM footer")
	}

	// Deserialize
	deserializedKey, err := km.DeserializePublicKey(pemData)
	if err != nil {
		t.Fatalf("DeserializePublicKey() failed: %v", err)
	}

	// Verify keys match
	if deserializedKey.N.Cmp(pubKey.N) != 0 {
		t.Error("Deserialized public key modulus doesn't match original")
	}
	if deserializedKey.E != pubKey.E {
		t.Error("Deserialized public key exponent doesn't match original")
	}
}

// TestKeyManager_FullKeyLifecycle tests complete key generation and serialization cycle
func TestKeyManager_FullKeyLifecycle(t *testing.T) {
	km, _ := NewKeyManager()

	// Generate key pair
	privKey, pubKey, err := km.GenerateTempKeyPair(2048)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Serialize both keys
	privKeyPEM, err := km.SerializePrivateKey(privKey)
	if err != nil {
		t.Fatalf("Failed to serialize private key: %v", err)
	}

	pubKeyPEM, err := km.SerializePublicKey(pubKey)
	if err != nil {
		t.Fatalf("Failed to serialize public key: %v", err)
	}

	// Deserialize both keys
	deserializedPrivKey, err := km.DeserializePrivateKey(privKeyPEM)
	if err != nil {
		t.Fatalf("Failed to deserialize private key: %v", err)
	}

	deserializedPubKey, err := km.DeserializePublicKey(pubKeyPEM)
	if err != nil {
		t.Fatalf("Failed to deserialize public key: %v", err)
	}

	// Verify the deserialized keys can be used for encryption/decryption
	message := []byte("test message for encryption")

	// Encrypt with deserialized public key
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, deserializedPubKey, message, nil)
	if err != nil {
		t.Fatalf("Encryption with deserialized public key failed: %v", err)
	}

	// Decrypt with deserialized private key
	decrypted, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, deserializedPrivKey, ciphertext, nil)
	if err != nil {
		t.Fatalf("Decryption with deserialized private key failed: %v", err)
	}

	if !bytes.Equal(decrypted, message) {
		t.Error("Decrypted message doesn't match original")
	}
}

// TestKeyManager_DeserializePrivateKey_InvalidPEM tests deserialization with invalid PEM
func TestKeyManager_DeserializePrivateKey_InvalidPEM(t *testing.T) {
	km, _ := NewKeyManager()

	invalidPEMs := [][]byte{
		[]byte("not a valid PEM"),
		[]byte("-----BEGIN RSA PRIVATE KEY-----\ninvalid\n-----END RSA PRIVATE KEY-----"),
		[]byte{},
		[]byte("-----BEGIN PUBLIC KEY-----\ninvalid\n-----END PUBLIC KEY-----"),
	}

	for i, pem := range invalidPEMs {
		_, err := km.DeserializePrivateKey(pem)
		if err == nil {
			t.Errorf("Deserialization %d should have failed with invalid PEM", i)
		}
	}
}

// TestKeyManager_DeserializePublicKey_InvalidPEM tests deserialization with invalid PEM
func TestKeyManager_DeserializePublicKey_InvalidPEM(t *testing.T) {
	km, _ := NewKeyManager()

	invalidPEMs := [][]byte{
		[]byte("not a valid PEM"),
		[]byte("-----BEGIN RSA PUBLIC KEY-----\ninvalid\n-----END RSA PUBLIC KEY-----"),
		[]byte{},
		[]byte("-----BEGIN PRIVATE KEY-----\ninvalid\n-----END PRIVATE KEY-----"),
	}

	for i, pem := range invalidPEMs {
		_, err := km.DeserializePublicKey(pem)
		if err == nil {
			t.Errorf("Deserialization %d should have failed with invalid PEM", i)
		}
	}
}

// TestKeyManager_MultipleKeyPairs tests generating multiple independent key pairs
func TestKeyManager_MultipleKeyPairs(t *testing.T) {
	km, _ := NewKeyManager()

	// Generate multiple key pairs
	keyPairs := make([]*rsa.PrivateKey, 3)
	for i := 0; i < 3; i++ {
		privKey, _, err := km.GenerateTempKeyPair(2048)
		if err != nil {
			t.Fatalf("Failed to generate key pair %d: %v", i, err)
		}
		keyPairs[i] = privKey
	}

	// Verify all keys are different
	for i := 0; i < len(keyPairs); i++ {
		for j := i + 1; j < len(keyPairs); j++ {
			if keyPairs[i].N.Cmp(keyPairs[j].N) == 0 {
				t.Errorf("Key pair %d and %d have the same modulus", i, j)
			}
		}
	}
}

// TestKeyManager_KeySizes tests various RSA key sizes
func TestKeyManager_KeySizes(t *testing.T) {
	km, _ := NewKeyManager()

	sizes := []int{1024, 2048, 3072, 4096}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("RSA-%d", size), func(t *testing.T) {
			privKey, pubKey, err := km.GenerateTempKeyPair(size)
			if err != nil {
				t.Fatalf("Failed to generate %d-bit key: %v", size, err)
			}

			if privKey.N.BitLen() != size {
				t.Errorf("Private key has %d bits, want %d", privKey.N.BitLen(), size)
			}

			// Test serialization round-trip
			privPEM, err := km.SerializePrivateKey(privKey)
			if err != nil {
				t.Fatalf("Failed to serialize private key: %v", err)
			}

			pubPEM, err := km.SerializePublicKey(pubKey)
			if err != nil {
				t.Fatalf("Failed to serialize public key: %v", err)
			}

			_, err = km.DeserializePrivateKey(privPEM)
			if err != nil {
				t.Fatalf("Failed to deserialize private key: %v", err)
			}

			_, err = km.DeserializePublicKey(pubPEM)
			if err != nil {
				t.Fatalf("Failed to deserialize public key: %v", err)
			}
		})
	}
}

// TestKeyManager_ConcurrentGeneration tests concurrent key generation
func TestKeyManager_ConcurrentGeneration(t *testing.T) {
	km, _ := NewKeyManager()

	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			privKey, pubKey, err := km.GenerateTempKeyPair(2048)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: generation failed: %v", id, err)
				return
			}

			// Serialize
			privPEM, err := km.SerializePrivateKey(privKey)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: serialize failed: %v", id, err)
				return
			}

			pubPEM, err := km.SerializePublicKey(pubKey)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: serialize public key failed: %v", id, err)
				return
			}

			// Deserialize
			_, err = km.DeserializePrivateKey(privPEM)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: deserialize private key failed: %v", id, err)
				return
			}

			_, err = km.DeserializePublicKey(pubPEM)
			if err != nil {
				results <- fmt.Errorf("goroutine %d: deserialize public key failed: %v", id, err)
				return
			}

			results <- nil
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}
