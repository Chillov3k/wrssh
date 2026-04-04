package keys

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NHAS/reverse_ssh/internal"
)

func TestGetPrivateKeyGeneratesWhenUnset(t *testing.T) {
	EmbeddedPrivateKeyBase64 = ""

	signer, err := GetPrivateKey()
	if err != nil {
		t.Fatalf("GetPrivateKey() error = %v", err)
	}
	if signer == nil {
		t.Fatal("GetPrivateKey() returned nil signer")
	}

	secondSigner, err := GetPrivateKey()
	if err != nil {
		t.Fatalf("second GetPrivateKey() error = %v", err)
	}

	if !bytes.Equal(signer.PublicKey().Marshal(), secondSigner.PublicKey().Marshal()) {
		t.Fatal("GetPrivateKey() should reuse the generated key within the same process")
	}
}

func TestSetPrivateKeyAndAuthorisedKeysLine(t *testing.T) {
	privateKey, err := internal.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey() error = %v", err)
	}

	if err := SetPrivateKey(string(privateKey)); err != nil {
		t.Fatalf("SetPrivateKey() error = %v", err)
	}

	line, err := AuthorisedKeysLine()
	if err != nil {
		t.Fatalf("AuthorisedKeysLine() error = %v", err)
	}

	if !strings.HasPrefix(line, "ssh-ed25519 ") {
		t.Fatalf("AuthorisedKeysLine() unexpected value = %q", line)
	}
}
