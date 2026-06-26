//go:build !windows
// +build !windows

package handlers

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	clientbusybox "github.com/NHAS/reverse_ssh/internal/client/busybox"
)

var busyBoxFallback atomic.Bool

func SetBusyBoxFallback(enabled bool) {
	busyBoxFallback.Store(enabled)
}

func busyBoxFallbackEnabled() bool {
	return busyBoxFallback.Load()
}

func busyBoxFallbackCommand(command string, args []string) (string, []string, string, error) {
	if !busyBoxFallbackEnabled() {
		return "", nil, "", errors.New("busybox fallback is disabled")
	}

	busyboxPath, err := clientbusybox.Ensure()
	if err != nil {
		return "", nil, "", err
	}

	applet := busyBoxApplet(command)
	fallbackArgs := make([]string, 0, len(args)+1)
	fallbackArgs = append(fallbackArgs, applet)
	fallbackArgs = append(fallbackArgs, args...)
	return busyboxPath, fallbackArgs, applet, nil
}

func busyBoxApplet(command string) string {
	base := strings.TrimSpace(filepath.Base(command))
	switch base {
	case "", ".", string(filepath.Separator):
		return "sh"
	case "bash", "zsh", "fish", "csh", "ksh", "tcsh":
		return "sh"
	default:
		return base
	}
}

func commandMissing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	value := err.Error()
	return strings.Contains(value, "executable file not found") ||
		strings.Contains(value, "no such file or directory")
}

func busyBoxFallbackError(original, fallback error) error {
	if fallback == nil {
		return original
	}
	if original == nil {
		return fallback
	}
	return fmt.Errorf("%s; busybox fallback failed: %w", original.Error(), fallback)
}
