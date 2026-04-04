package runtimeagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceUserKeysReplacesDirectoryContents(t *testing.T) {
	dataDir := t.TempDir()
	keysDir := filepath.Join(dataDir, "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		t.Fatalf("mkdir keys dir: %v", err)
	}

	legacyPath := filepath.Join(keysDir, "legacy")
	if err := os.WriteFile(legacyPath, []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGllegacy legacy\n"), 0600); err != nil {
		t.Fatalf("seed legacy key: %v", err)
	}

	users := []accessUser{
		{
			Username:       "alice",
			AuthorizedKeys: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGQe0V3rU3P5g8m2L3K6z4qvXfH6xj8F0T3g3q4nH6Aa alice@example",
		},
	}

	count, err := replaceUserKeys(dataDir, users)
	if err != nil {
		t.Fatalf("replace user keys: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected user count %d", count)
	}

	if _, err := os.Stat(filepath.Join(keysDir, "alice")); err != nil {
		t.Fatalf("expected alice keys to exist: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy key to be deleted, got %v", err)
	}
}

func TestReplaceUserKeysRejectsBadUsernames(t *testing.T) {
	_, err := replaceUserKeys(t.TempDir(), []accessUser{{
		Username:       "../escape",
		AuthorizedKeys: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGQe0V3rU3P5g8m2L3K6z4qvXfH6xj8F0T3g3q4nH6Aa alice@example",
	}})
	if err == nil {
		t.Fatal("expected invalid username error")
	}
}
