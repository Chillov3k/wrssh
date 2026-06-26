//go:build !windows
// +build !windows

package client

import "os"

func clientHostname() (string, error) {
	return os.Hostname()
}
