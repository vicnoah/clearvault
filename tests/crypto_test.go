package tests

import (
	"bytes"
	"clearvault/internal/crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

// TestEngineCreation 测试加密引擎创建
func TestEngineCreation(t *testing.T) {
	validKey := make([]byte, 32) // AES-256 requires 32 bytes
	_, err := rand.Read(validKey)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	_, err = crypto.NewEngine(validKey)
	if err != nil {
		t.Fatalf("Failed to create engine with valid key: %v", err)
	}

	// Test invalid key sizes
	invalidKeys := [][]byte{
		make([]byte, 16),
		make([]byte, 24),
		make([]byte, 31),
		make([]byte, 33),
	}

	for _, key := range invalidKeys {
		_, err := crypto.NewEngine(key)
		if err == nil {
			t.Errorf("Expected error for key size %d, got nil", len(key))
		}
		if err != crypto.ErrInvalidKey {
			t.Errorf("Expected ErrInvalidKey, got %v", err)
		}
	}
}

// TestEncryptDecryptRoundTrip 测试加密解密往返
func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	testCases := []struct {
		name    string
		size    int
	}{
		{"empty", 0},
		{"single_byte", 1},
		{"small", 100},
		{"chunk_minus_1", crypto.ChunkSize - 1},
		{"exact_chunk", crypto.ChunkSize},
		{"chunk_plus_1", crypto.ChunkSize + 1},
		{"two_chunks", crypto.ChunkSize * 2},
		{"half_mb", 512 * 1024},
		{"one_mb", 1024 * 1024},
		{"three_chunks", crypto.ChunkSize * 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			original := make([]byte, tc.size)
			if tc.size > 0 {
				if _, err := rand.Read(original); err != nil {
					t.Fatalf("Failed to generate data: %v", err)
				}
			}

			var ciphertext bytes.Buffer
			err = engine.EncryptStream(bytes.NewReader(original), &ciphertext, nonce)
			if err != nil {
				t.Fatalf("EncryptStream failed: %v", err)
			}

			var decrypted bytes.Buffer
			err = engine.DecryptStream(&ciphertext, &decrypted, nonce)
			if err != nil {
				t.Fatalf("DecryptStream failed: %v", err)
			}

			if !bytes.Equal(original, decrypted.Bytes()) {
				t.Errorf("Data integrity check failed")
				t.Logf("Original hash: %s", calculateHash(original))
				t.Logf("Decrypted hash: %s", calculateHash(decrypted.Bytes()))
			}
		})
	}
}

// TestCalculateEncryptedSize 测试加密大小计算
func TestCalculateEncryptedSize(t *testing.T) {
	testCases := []struct {
		original    int64
		expected    int64
		description string
	}{
		{0, 0, "zero bytes"},
		{1, 1 + crypto.TagSize, "single byte"},
		{crypto.ChunkSize, crypto.ChunkSize + crypto.TagSize, "exact chunk"},
		{crypto.ChunkSize + 1, crypto.ChunkSize + 1 + crypto.TagSize*2, "chunk + 1 byte"},
		{crypto.ChunkSize * 2, crypto.ChunkSize*2 + crypto.TagSize*2, "two chunks"},
		{crypto.ChunkSize*2 + 100, crypto.ChunkSize*2 + 100 + crypto.TagSize*3, "two chunks + partial"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := crypto.CalculateEncryptedSize(tc.original)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

// TestDecryptRange 测试范围解密
func TestDecryptRange(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	// Create test data: 3 chunks
	original := make([]byte, crypto.ChunkSize*3)
	if _, err := rand.Read(original); err != nil {
		t.Fatalf("Failed to generate data: %v", err)
	}

	var ciphertext bytes.Buffer
	err = engine.EncryptStream(bytes.NewReader(original), &ciphertext, nonce)
	if err != nil {
		t.Fatalf("EncryptStream failed: %v", err)
	}

	testCases := []struct {
		name   string
		start  int64
		length int64
	}{
		{"first_byte", 0, 1},
		{"first_chunk_first_100", 0, 100},
		{"first_chunk_middle", 1024, 1024},
		{"first_chunk_end", crypto.ChunkSize - 100, 100},
		{"cross_boundary_1", crypto.ChunkSize - 50, 100},
		{"cross_boundary_2", crypto.ChunkSize * 2 - 50, 100},
		{"second_chunk_start", crypto.ChunkSize, crypto.ChunkSize},
		{"middle_range", crypto.ChunkSize + 1024, 2048},
		{"last_chunk_end", int64(len(original)) - 100, 100},
		{"from_start_to_end", 0, int64(len(original))},
		{"partial_last_chunk", crypto.ChunkSize * 2 + 1000, 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert ciphertext to ReaderAt
			ciphertextReader := bytes.NewReader(ciphertext.Bytes())
			
			var decrypted bytes.Buffer
			err = engine.DecryptRange(ciphertextReader, &decrypted, nonce, tc.start, tc.length)
			if err != nil {
				t.Fatalf("DecryptRange failed: %v", err)
			}

			expected := original[tc.start : tc.start+tc.length]
			if !bytes.Equal(expected, decrypted.Bytes()) {
				t.Errorf("Range decryption data mismatch")
				t.Logf("Expected hash: %s", calculateHash(expected))
				t.Logf("Got hash: %s", calculateHash(decrypted.Bytes()))
			}
		})
	}
}

// TestDecryptStreamFrom 测试从指定块开始解密
func TestDecryptStreamFrom(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	original := make([]byte, crypto.ChunkSize*3)
	if _, err := rand.Read(original); err != nil {
		t.Fatalf("Failed to generate data: %v", err)
	}

	var ciphertext bytes.Buffer
	err = engine.EncryptStream(bytes.NewReader(original), &ciphertext, nonce)
	if err != nil {
		t.Fatalf("EncryptStream failed: %v", err)
	}

	testCases := []struct {
		name          string
		startChunkIdx uint64
	}{
		{"start_from_0", 0},
		{"start_from_1", 1},
		{"start_from_2", 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var decrypted bytes.Buffer
			err = engine.DecryptStreamFrom(&ciphertext, &decrypted, nonce, tc.startChunkIdx)
			if err != nil {
				t.Fatalf("DecryptStreamFrom failed: %v", err)
			}

			expectedStart := tc.startChunkIdx * crypto.ChunkSize
			expected := original[expectedStart:]
			if !bytes.Equal(expected, decrypted.Bytes()) {
				t.Errorf("DecryptStreamFrom data mismatch")
				t.Logf("Expected size: %d, got: %d", len(expected), decrypted.Len())
			}
		})
	}
}

// TestCorruptedDataHandling 测试损坏数据处理
func TestCorruptedDataHandling(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	original := []byte("This is test data for corruption testing")
	var ciphertext bytes.Buffer
	err = engine.EncryptStream(bytes.NewReader(original), &ciphertext, nonce)
	if err != nil {
		t.Fatalf("EncryptStream failed: %v", err)
	}

	// Test 1: Corrupt ciphertext
	corrupted := make([]byte, ciphertext.Len())
	copy(corrupted, ciphertext.Bytes())
	corrupted[10] ^= 0xFF // Flip some bits

	var decrypted bytes.Buffer
	err = engine.DecryptStream(bytes.NewReader(corrupted), &decrypted, nonce)
	if err == nil {
		t.Error("Expected decryption error with corrupted data")
	}
	if err != crypto.ErrDecryptFail {
		t.Errorf("Expected ErrDecryptFail, got %v", err)
	}

	// Test 2: Truncated ciphertext
	truncated := ciphertext.Bytes()[:ciphertext.Len()-1]
	decrypted.Reset()
	err = engine.DecryptStream(bytes.NewReader(truncated), &decrypted, nonce)
	if err == nil {
		t.Error("Expected decryption error with truncated data")
	}
}

// TestDifferentNonces 测试不同nonce产生不同密文
func TestDifferentNonces(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce1, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce1: %v", err)
	}

	nonce2, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce2: %v", err)
	}
	// Ensure nonces are different
	nonce2[0] ^= 0xFF

	original := []byte("Same data with different nonces")

	var ciphertext1 bytes.Buffer
	err = engine.EncryptStream(bytes.NewReader(original), &ciphertext1, nonce1)
	if err != nil {
		t.Fatalf("First encrypt failed: %v", err)
	}

	var ciphertext2 bytes.Buffer
	err = engine.EncryptStream(bytes.NewReader(original), &ciphertext2, nonce2)
	if err != nil {
		t.Fatalf("Second encrypt failed: %v", err)
	}

	if bytes.Equal(ciphertext1.Bytes(), ciphertext2.Bytes()) {
		t.Error("Different nonces should produce different ciphertexts")
	}
}

// TestDataIntegrityComprehensive 综合数据完整性测试
func TestDataIntegrityComprehensive(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	// Test various data patterns
	patterns := []struct {
		name  string
		data  []byte
	}{
		{"zeros", bytes.Repeat([]byte{0x00}, 1024)},
		{"ones", bytes.Repeat([]byte{0xFF}, 1024)},
		{"pattern_AA", bytes.Repeat([]byte{0xAA}, 1024)},
		{"pattern_55", bytes.Repeat([]byte{0x55}, 1024)},
		{"sequential", func() []byte {
			b := make([]byte, 1024)
			for i := range b {
				b[i] = byte(i % 256)
			}
			return b
		}()},
		{"ascii_text", []byte("This is a test of ASCII text with various characters: !@#$%^&*()")},
		{"utf8_text", []byte("UTF-8 测试: 你好世界 🌍")},
		{"mixed_nulls", func() []byte {
			b := make([]byte, 2048)
			for i := range b {
				if i%10 == 0 {
					b[i] = 0x00
				} else {
					b[i] = byte(i % 256)
				}
			}
			return b
		}()},
	}

	for _, tc := range patterns {
		t.Run(tc.name, func(t *testing.T) {
			originalHash := calculateHash(tc.data)

			var ciphertext bytes.Buffer
			err = engine.EncryptStream(bytes.NewReader(tc.data), &ciphertext, nonce)
			if err != nil {
				t.Fatalf("EncryptStream failed: %v", err)
			}

			var decrypted bytes.Buffer
			err = engine.DecryptStream(&ciphertext, &decrypted, nonce)
			if err != nil {
				t.Fatalf("DecryptStream failed: %v", err)
			}

			decryptedHash := calculateHash(decrypted.Bytes())

			if originalHash != decryptedHash {
				t.Errorf("Hash mismatch: original=%s, decrypted=%s", originalHash, decryptedHash)
			}
		})
	}
}

// TestStreamIO 测试流式I/O边界
func TestStreamIO(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	// Test with a reader that returns data in chunks
	data := bytes.Repeat([]byte{0x42}, 10*crypto.ChunkSize)
	reader := &chunkedReader{data: data}

	var ciphertext bytes.Buffer
	err = engine.EncryptStream(reader, &ciphertext, nonce)
	if err != nil {
		t.Fatalf("EncryptStream with chunked reader failed: %v", err)
	}

	var decrypted bytes.Buffer
	err = engine.DecryptStream(&ciphertext, &decrypted, nonce)
	if err != nil {
		t.Fatalf("DecryptStream failed: %v", err)
	}

	if !bytes.Equal(data, decrypted.Bytes()) {
		t.Error("Chunked I/O data mismatch")
	}
}

// chunkedReader 模拟返回小块数据的reader
type chunkedReader struct {
	data   []byte
	offset int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}

	// Return at most 1KB per read
	chunkSize := 1024
	if r.offset+chunkSize > len(r.data) {
		chunkSize = len(r.data) - r.offset
	}
	if chunkSize > len(p) {
		chunkSize = len(p)
	}

	n := copy(p, r.data[r.offset:r.offset+chunkSize])
	r.offset += n
	return n, nil
}

// hashBytes 计算SHA256哈希（避免与comprehensive_test.go冲突）
func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// TestEdgeCaseNonces 测试nonce边界情况
func TestEdgeCaseNonces(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Test with all-zero nonce
	zeroNonce := make([]byte, crypto.NonceSize)
	original := []byte("Test with zero nonce")

	var ciphertext bytes.Buffer
	err = engine.EncryptStream(bytes.NewReader(original), &ciphertext, zeroNonce)
	if err != nil {
		t.Fatalf("EncryptStream with zero nonce failed: %v", err)
	}

	var decrypted bytes.Buffer
	err = engine.DecryptStream(&ciphertext, &decrypted, zeroNonce)
	if err != nil {
		t.Fatalf("DecryptStream failed: %v", err)
	}

	if !bytes.Equal(original, decrypted.Bytes()) {
		t.Error("Zero nonce decryption failed")
	}
}

// TestConcurrentEncryption 测试并发加密
func TestConcurrentEncryption(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	engine, err := crypto.NewEngine(key)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	nonce, err := crypto.GenerateRandomBytes(crypto.NonceSize)
	if err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	data := []byte(strings.Repeat("Concurrent test data ", 1000))

	// Run multiple encryption/decryption operations concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			var ciphertext bytes.Buffer
			err = engine.EncryptStream(bytes.NewReader(data), &ciphertext, nonce)
			if err != nil {
				t.Errorf("Concurrent encrypt failed: %v", err)
				done <- false
				return
			}

			var decrypted bytes.Buffer
			err = engine.DecryptStream(&ciphertext, &decrypted, nonce)
			if err != nil {
				t.Errorf("Concurrent decrypt failed: %v", err)
				done <- false
				return
			}

			if !bytes.Equal(data, decrypted.Bytes()) {
				t.Error("Concurrent data mismatch")
				done <- false
				return
			}

			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		if !<-done {
			t.Error("One or more concurrent operations failed")
		}
	}
}
