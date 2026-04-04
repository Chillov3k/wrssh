package userkeys

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

func ValidateAuthorizedKeys(raw string) (string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	scanner := bufio.NewScanner(strings.NewReader(raw))

	lines := make([]string, 0)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line)); err != nil {
			return "", fmt.Errorf("invalid SSH public key on line %d: %w", lineNumber, err)
		}

		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}

func SyncUserAuthorizedKeys(dataDir, username, raw string) error {
	keysDir := filepath.Join(dataDir, "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return err
	}

	path := filepath.Join(keysDir, filepath.Clean(username))
	clean, err := ValidateAuthorizedKeys(raw)
	if err != nil {
		return err
	}

	if clean == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	content := append([]byte(clean), '\n')
	return os.WriteFile(path, content, 0600)
}

func RemoveUserAuthorizedKeys(dataDir, username string) error {
	path := filepath.Join(dataDir, "keys", filepath.Clean(username))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func KeyCount(raw string) int {
	clean, err := ValidateAuthorizedKeys(raw)
	if err != nil || clean == "" {
		return 0
	}
	return bytes.Count([]byte(clean), []byte("\n")) + 1
}
