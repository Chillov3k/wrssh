package busybox

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	extractMu     sync.Mutex
	extractedPath string
)

func Embedded() bool {
	return len(embeddedGzip) > 0
}

func Ensure() (string, error) {
	extractMu.Lock()
	defer extractMu.Unlock()

	if extractedPath != "" {
		if info, err := os.Stat(extractedPath); err == nil && !info.IsDir() {
			return extractedPath, nil
		}
		extractedPath = ""
	}

	if len(embeddedGzip) == 0 {
		return "", errors.New("busybox fallback was not embedded in this client")
	}

	reader, err := gzip.NewReader(bytes.NewReader(embeddedGzip))
	if err != nil {
		return "", fmt.Errorf("open embedded busybox: %w", err)
	}
	defer reader.Close()

	file, err := createTempExecutable()
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("write embedded busybox: %w", err)
	}
	if err := file.Chmod(0700); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("mark embedded busybox executable: %w", err)
	}

	extractedPath = file.Name()
	return extractedPath, nil
}

func createTempExecutable() (*os.File, error) {
	for _, dir := range tempDirs() {
		file, err := os.CreateTemp(dir, ".wrssh-busybox-*")
		if err == nil {
			return file, nil
		}
	}
	return nil, errors.New("create embedded busybox temp file: no writable temp directory found")
}

func tempDirs() []string {
	candidates := []string{
		os.TempDir(),
		"/tmp",
		"/var/tmp",
		"/dev/shm",
		".",
	}

	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		dir = filepath.Clean(strings.TrimSpace(dir))
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		result = append(result, dir)
	}
	return result
}
