package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var (
	ErrInvalidKey      = errors.New("invalid key size: must be 16, 24, or 32 bytes")
	ErrInvalidCiphertext = errors.New("invalid ciphertext: too short")
)

// Encryptor AES-256加解密器
type Encryptor struct {
	key []byte
}

// NewEncryptor 创建加密器（key必须是16/24/32字节对应AES-128/192/256）
func NewEncryptor(key string) (*Encryptor, error) {
	keyBytes := []byte(key)
	keyLen := len(keyBytes)
	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		return nil, ErrInvalidKey
	}
	return &Encryptor{key: keyBytes}, nil
}

// Encrypt 加密明文，返回Base64编码的密文
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密Base64编码的密文，返回明文
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// MustEncrypt 加密，失败时panic
func (e *Encryptor) MustEncrypt(plaintext string) string {
	result, err := e.Encrypt(plaintext)
	if err != nil {
		panic(err)
	}
	return result
}

// MustDecrypt 解密，失败时panic
func (e *Encryptor) MustDecrypt(ciphertext string) string {
	result, err := e.Decrypt(ciphertext)
	if err != nil {
		panic(err)
	}
	return result
}
