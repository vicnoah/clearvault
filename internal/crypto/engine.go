package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
)

const (
	ChunkSize       = 64 * 1024
	NonceSize       = 12
	TagSize         = 16
	CipherChunkSize = ChunkSize + TagSize
)

// CalculateEncryptedSize returns the size of the encrypted stream given the original plaintext size.
func CalculateEncryptedSize(originalSize int64) int64 {
	if originalSize == 0 {
		return 0
	}
	numChunks := (originalSize + ChunkSize - 1) / ChunkSize
	return originalSize + numChunks*TagSize
}

var (
	ErrInvalidKey  = errors.New("invalid key size")
	ErrDecryptFail = errors.New("decryption failed")
)

type Engine struct {
	key []byte
}

func NewEngine(key []byte) (*Engine, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return &Engine{key: key}, nil
}

// GenerateRandomBytes returns securely generated random bytes.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// deriveNonce generates a nonce for a specific chunk index.
// It uses the base nonce and XORs it with the chunk index.
func deriveNonce(baseNonce []byte, chunkIndex uint64) []byte {
	nonce := make([]byte, NonceSize)
	copy(nonce, baseNonce)

	// XOR the last 8 bytes with chunk index
	indexBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(indexBytes, chunkIndex)
	for i := 0; i < 8; i++ {
		nonce[4+i] ^= indexBytes[i]
	}
	return nonce
}

func (e *Engine) EncryptStream(r io.Reader, w io.Writer, baseNonce []byte) error {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// 1. 定义 overhead (GCM 通常是 16 字节 tag)
	overhead := aesgcm.Overhead()

	// 2. 准备缓冲区
	// 如果想要极致省内存，可以只用一个 buffer 并在原地加密（src 和 dst 同一个）
	// 但为了逻辑清晰和安全，这里演示 "复用输出缓冲区" 的方式

	// 输入缓冲区
	buf := make([]byte, ChunkSize)
	// 输出缓冲区：预先分配好容量，避免 Seal 内部反复 make
	outBuf := make([]byte, 0, ChunkSize+overhead)

	var chunkIndex uint64 = 0

	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			nonce := deriveNonce(baseNonce, chunkIndex)

			// 3. 【关键修改】复用 outBuf
			// outBuf[:0] 表示清空 slice 但保留底层 capacity
			// 这样 Seal 会直接把结果写进 outBuf 的底层数组，不再分配新内存
			ciphertext := aesgcm.Seal(outBuf[:0], nonce, buf[:n], nil)

			// 4. 写入 pipe
			// 此时 w.Write 会阻塞，直到 HTTP Client 读走这些数据
			if _, err := w.Write(ciphertext); err != nil {
				return err
			}
			chunkIndex++
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) DecryptStream(r io.Reader, w io.Writer, baseNonce []byte) error {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	buf := make([]byte, CipherChunkSize)
	var chunkIndex uint64 = 0

	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			nonce := deriveNonce(baseNonce, chunkIndex)
			plaintext, err := aesgcm.Open(nil, nonce, buf[:n], nil)
			if err != nil {
				return ErrDecryptFail
			}
			if _, err := w.Write(plaintext); err != nil {
				return err
			}
			chunkIndex++
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) DecryptStreamFrom(r io.Reader, w io.Writer, baseNonce []byte, startChunkIndex uint64) error {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	buf := make([]byte, CipherChunkSize)
	chunkIndex := startChunkIndex

	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			nonce := deriveNonce(baseNonce, chunkIndex)
			plaintext, err := aesgcm.Open(nil, nonce, buf[:n], nil)
			if err != nil {
				return ErrDecryptFail
			}
			if _, err := w.Write(plaintext); err != nil {
				return err
			}
			chunkIndex++
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// DecryptRange decrypts a specific byte range of a file.
func (e *Engine) DecryptRange(r io.ReaderAt, w io.Writer, baseNonce []byte, start, length int64) error {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	startChunk := uint64(start / ChunkSize)
	endChunk := uint64((start + length - 1) / ChunkSize)

	for i := startChunk; i <= endChunk; i++ {
		cipherOffset := int64(i) * CipherChunkSize
		cipherBuf := make([]byte, CipherChunkSize)

		// Read full chunk (might be smaller if it's the last chunk)
		// We need to know the actual size of the last chunk if possible,
		// but GCM Open will handle it if we provide the correct ciphertext.
		// However, ReaderAt doesn't return EOF easily like Reader.
		n, err := r.ReadAt(cipherBuf, cipherOffset)
		if n == 0 && err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		nonce := deriveNonce(baseNonce, i)
		plaintext, err := aesgcm.Open(nil, nonce, cipherBuf[:n], nil)
		if err != nil {
			return ErrDecryptFail
		}

		// Calculate how much of this plaintext to write
		chunkStart := int64(i) * ChunkSize
		writeStart := start - chunkStart
		if writeStart < 0 {
			writeStart = 0
		}

		writeEnd := (start + length) - chunkStart
		if writeEnd > int64(len(plaintext)) {
			writeEnd = int64(len(plaintext))
		}

		if writeStart < int64(len(plaintext)) {
			if _, err := w.Write(plaintext[writeStart:writeEnd]); err != nil {
				return err
			}
		}
	}

	return nil
}
