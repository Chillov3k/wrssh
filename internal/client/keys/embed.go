package keys

import (
	"encoding/base64"
	"fmt"
	"log"

	"github.com/NHAS/reverse_ssh/internal"
	"golang.org/x/crypto/ssh"
)

// EmbeddedPrivateKeyBase64 can be injected at build time via ldflags.
// If it is empty or invalid, the client generates a fresh key at runtime.
var EmbeddedPrivateKeyBase64 string

func parseEmbeddedPrivateKey() (ssh.Signer, error) {
	if EmbeddedPrivateKeyBase64 == "" {
		return nil, fmt.Errorf("no embedded private key configured")
	}

	keyBytes, err := base64.StdEncoding.DecodeString(EmbeddedPrivateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("embedded private key base64 invalid: %w", err)
	}

	sshPriv, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("embedded private key invalid: %w", err)
	}

	return sshPriv, nil
}

func GetPrivateKey() (ssh.Signer, error) {
	sshPriv, err := parseEmbeddedPrivateKey()
	if err != nil {
		log.Println("Unable to load embedded private key: ", err)
		bs, err := internal.GeneratePrivateKey()
		if err != nil {
			return nil, err
		}
		EmbeddedPrivateKeyBase64 = base64.StdEncoding.EncodeToString(bs)

		sshPriv, err = ssh.ParsePrivateKey(bs)
		if err != nil {
			return nil, err
		}
	}

	return sshPriv, nil
}

func SetPrivateKey(key string) error {
	_, err := ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return fmt.Errorf("private key invalid: %w", err)
	}

	EmbeddedPrivateKeyBase64 = base64.StdEncoding.EncodeToString([]byte(key))
	return nil
}

func AuthorisedKeysLine() (string, error) {
	priv, err := parseEmbeddedPrivateKey()
	if err != nil {
		return "", fmt.Errorf("private key invalid: %w", err)
	}

	return string(ssh.MarshalAuthorizedKey(priv.PublicKey())), nil

}
