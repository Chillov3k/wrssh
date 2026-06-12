//go:build windows
// +build windows

package handlers

import "errors"

func SetBusyBoxFallback(bool) {}

func busyBoxFallbackCommand(string, []string) (string, []string, string, error) {
	return "", nil, "", errors.New("busybox fallback is only available on linux clients")
}

func busyBoxFallbackError(original, fallback error) error {
	if fallback == nil {
		return original
	}
	return fallback
}

func commandMissing(error) bool {
	return false
}
