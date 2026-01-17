package crypto

import (
	"bytes"
	"testing"
)

func TestEngine_EncryptDecrypt(t *testing.T) {
	key, _ := GenerateRandomBytes(32)
	baseNonce, _ := GenerateRandomBytes(12)
	engine, _ := NewEngine(key)

	data := []byte("Hello, Clearvault! This is a test message for chunked encryption. " +
		"We will use a relatively long string to ensure it spans across potential boundaries if we had small chunks.")

	// Encrypt
	plainBuf := bytes.NewReader(data)
	cipherBuf := &bytes.Buffer{}
	err := engine.EncryptStream(plainBuf, cipherBuf, baseNonce)
	if err != nil {
		t.Fatalf("EncryptStream failed: %v", err)
	}

	// Decrypt
	decryptedBuf := &bytes.Buffer{}
	err = engine.DecryptStream(cipherBuf, decryptedBuf, baseNonce)
	if err != nil {
		t.Fatalf("DecryptStream failed: %v", err)
	}

	if !bytes.Equal(data, decryptedBuf.Bytes()) {
		t.Errorf("Decrypted data does not match original. \nExpected: %s\nGot: %s", string(data), decryptedBuf.String())
	}
}

func TestEngine_DecryptRange(t *testing.T) {
	key, _ := GenerateRandomBytes(32)
	baseNonce, _ := GenerateRandomBytes(12)
	engine, _ := NewEngine(key)

	// Create data larger than 2 chunks
	data := make([]byte, ChunkSize*2+100)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Encrypt
	plainBuf := bytes.NewReader(data)
	cipherBuf := &bytes.Buffer{}
	_ = engine.EncryptStream(plainBuf, cipherBuf, baseNonce)

	cipherData := cipherBuf.Bytes()
	cipherReader := bytes.NewReader(cipherData)

	// Test various ranges
	testCases := []struct {
		start  int64
		length int64
	}{
		{0, 10},
		{100, 50},
		{ChunkSize - 10, 20}, // Crosses first chunk boundary
		{ChunkSize, 100},
		{ChunkSize * 2, 50},
		{0, int64(len(data))},
	}

	for _, tc := range testCases {
		outBuf := &bytes.Buffer{}
		err := engine.DecryptRange(cipherReader, outBuf, baseNonce, tc.start, tc.length)
		if err != nil {
			t.Errorf("DecryptRange(start=%d, len=%d) failed: %v", tc.start, tc.length, err)
			continue
		}

		expected := data[tc.start : tc.start+tc.length]
		if !bytes.Equal(expected, outBuf.Bytes()) {
			t.Errorf("DecryptRange(start=%d, len=%d) returned wrong data", tc.start, tc.length)
		}
	}
}
