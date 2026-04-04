package secrets

import (
	"encoding/base64"
	"testing"
)

func TestParseMasterKey(t *testing.T) {
	key := make([]byte, 32)
	encoded := base64.StdEncoding.EncodeToString(key)

	parsed, err := ParseMasterKey(encoded)
	if err != nil {
		t.Fatalf("parse master key: %v", err)
	}
	if len(parsed) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(parsed))
	}
}

func TestEncryptDecryptStringRoundTrip(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	plaintext := "super-secret-runtime-token"

	ciphertext, err := EncryptString(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt string: %v", err)
	}
	if ciphertext == "" || ciphertext == plaintext {
		t.Fatalf("expected encrypted output, got %q", ciphertext)
	}

	opened, err := DecryptString(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt string: %v", err)
	}
	if opened != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, opened)
	}
}
