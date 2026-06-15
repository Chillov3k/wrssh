package client

import (
	"net"
	"time"
)

func defaultRouteIP() string {
	for _, target := range []string{"8.8.8.8:80", "1.1.1.1:80", "[2001:4860:4860::8888]:80"} {
		conn, err := net.DialTimeout("udp", target, time.Second)
		if err != nil {
			continue
		}

		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && usableLocalIP(addr.IP) {
			_ = conn.Close()
			return addr.IP.String()
		}

		_ = conn.Close()
	}

	return firstInterfaceIP()
}

func firstInterfaceIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip := ipFromAddr(address)
			if usableLocalIP(ip) {
				return ip.String()
			}
		}
	}

	return ""
}

func ipFromAddr(address net.Addr) net.IP {
	switch typed := address.(type) {
	case *net.IPNet:
		return typed.IP
	case *net.IPAddr:
		return typed.IP
	default:
		return nil
	}
}

func usableLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}
