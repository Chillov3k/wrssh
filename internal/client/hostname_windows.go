//go:build windows
// +build windows

package client

import (
	"net"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func clientHostname() (string, error) {
	if hostname, err := windowsComputerName(windows.ComputerNameDnsFullyQualified); err == nil && hostname != "" {
		return hostname, nil
	}

	shortHostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	shortHostname = strings.TrimSpace(shortHostname)
	if shortHostname == "" {
		return shortHostname, nil
	}

	if canonical, err := net.LookupCNAME(shortHostname); err == nil {
		canonical = strings.TrimSuffix(strings.TrimSpace(canonical), ".")
		if canonical != "" {
			return canonical, nil
		}
	}

	return shortHostname, nil
}

func windowsComputerName(nameType uint32) (string, error) {
	size := uint32(256)
	for {
		buffer := make([]uint16, size)
		err := windows.GetComputerNameEx(nameType, &buffer[0], &size)
		if err == nil {
			return strings.TrimSpace(windows.UTF16ToString(buffer[:size])), nil
		}
		if err != windows.ERROR_MORE_DATA || size == 0 {
			return "", err
		}
	}
}
