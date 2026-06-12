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
	shortHostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	shortHostname = cleanHostname(shortHostname)
	if shortHostname == "" {
		return shortHostname, nil
	}

	if canonical, err := windowsCanonicalName(shortHostname); err == nil && isQualifiedHostname(canonical) {
		return cleanHostname(canonical), nil
	}

	if canonical, err := net.LookupCNAME(shortHostname); err == nil {
		if isQualifiedHostname(canonical) {
			return cleanHostname(canonical), nil
		}
	}

	for _, nameType := range []uint32{
		windows.ComputerNameDnsFullyQualified,
		windows.ComputerNamePhysicalDnsFullyQualified,
	} {
		if hostname, err := windowsComputerName(nameType); err == nil && isQualifiedHostname(hostname) {
			return cleanHostname(hostname), nil
		}
	}

	if hostname := windowsJoinedHostname(windows.ComputerNameDnsHostname, windows.ComputerNameDnsDomain); isQualifiedHostname(hostname) {
		return hostname, nil
	}

	if hostname := windowsJoinedHostname(windows.ComputerNamePhysicalDnsHostname, windows.ComputerNamePhysicalDnsDomain); isQualifiedHostname(hostname) {
		return hostname, nil
	}

	if domain := cleanHostname(os.Getenv("USERDNSDOMAIN")); strings.Contains(domain, ".") {
		return shortHostname + "." + strings.ToLower(domain), nil
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

func windowsCanonicalName(hostname string) (string, error) {
	node, err := windows.UTF16PtrFromString(hostname)
	if err != nil {
		return "", err
	}

	hints := windows.AddrinfoW{
		Flags: windows.AI_CANONNAME,
	}
	var result *windows.AddrinfoW
	if err := windows.GetAddrInfoW(node, nil, &hints, &result); err != nil {
		return "", err
	}
	defer windows.FreeAddrInfoW(result)

	for item := result; item != nil; item = item.Next {
		if item.Canonname == nil {
			continue
		}
		canonical := cleanHostname(windows.UTF16PtrToString(item.Canonname))
		if canonical != "" {
			return canonical, nil
		}
	}

	return "", nil
}

func windowsJoinedHostname(hostNameType, domainNameType uint32) string {
	host, hostErr := windowsComputerName(hostNameType)
	domain, domainErr := windowsComputerName(domainNameType)
	if hostErr != nil || domainErr != nil {
		return ""
	}

	host = cleanHostname(host)
	domain = cleanHostname(domain)
	if host == "" || domain == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(domain)) {
		return host
	}
	return host + "." + domain
}

func isQualifiedHostname(hostname string) bool {
	hostname = cleanHostname(hostname)
	return strings.Contains(hostname, ".")
}

func cleanHostname(hostname string) string {
	return strings.TrimSuffix(strings.TrimSpace(hostname), ".")
}
