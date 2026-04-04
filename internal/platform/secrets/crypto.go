package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const masterKeyLength = 32

func ParseMasterKey(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}

	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("runtime secret key must be base64: %w", err)
	}
	if len(key) != masterKeyLength {
		return nil, fmt.Errorf("runtime secret key must decode to %d bytes", masterKeyLength)
	}

	return key, nil
}

func EncryptString(key []byte, plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", nil
	}
	if len(key) != masterKeyLength {
		return "", fmt.Errorf("invalid encryption key length")
	}

	block, err := aes.NewCipher(key)
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

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func DecryptString(key []byte, ciphertext string) (string, error) {
	value := strings.TrimSpace(ciphertext)
	if value == "" {
		return "", nil
	}
	if len(key) != masterKeyLength {
		return "", fmt.Errorf("invalid decryption key length")
	}

	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("ciphertext must be base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext is too short")
	}

	nonce := raw[:gcm.NonceSize()]
	payload := raw[gcm.NonceSize():]
	opened, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", err
	}

	return string(opened), nil
}
