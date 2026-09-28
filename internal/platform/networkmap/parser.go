package networkmap

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
)

// pscanResult mirrors the pscan engine JSONL result shape. It is duplicated
// here (instead of importing the build-tagged engine package) so the control
// plane can parse pscan output without linking client code.
type pscanResult struct {
	IP       string        `json:"ip"`
	Port     int           `json:"port"`
	Protocol string        `json:"protocol,omitempty"`
	State    string        `json:"state,omitempty"`
	Open     bool          `json:"open"`
	Web      *pscanWebInfo `json:"web,omitempty"`
	NetBIOS  *pscanNBInfo  `json:"netbios,omitempty"`
}

type pscanWebInfo struct {
	Scheme     string `json:"scheme,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	Title      string `json:"title,omitempty"`
	Server     string `json:"server,omitempty"`
}

type pscanNBInfo struct {
	Hostname string `json:"hostname,omitempty"`
	Domain   string `json:"domain,omitempty"`
	MAC      string `json:"mac,omitempty"`
}

type discoveryAccumulator struct {
	ip       string
	hostname string
	os       string
	ports    []Port
	seen     map[Port]struct{}
}

// ParsePscanOutput extracts discovered hosts from a pscan module run output.
// Both pscan modes are understood:
//
//   - JSONL mode (--json): one engine.Result object per line;
//   - text mode: "ip:port [state] [web...] [netbios...]" lines for open ports.
//
// Lines that are not results (warnings, banner text, progress notes) are
// skipped silently. Only open TCP ports and answered UDP probes produce
// discoveries.
func ParsePscanOutput(output string) []Discovery {
	accumulator := map[string]*discoveryAccumulator{}
	order := []string{}

	accept := func(ip, hostname, os string, port Port) {
		ip = normalizeIP(ip)
		if ip == "" || port.Port <= 0 || port.Port > 65535 {
			return
		}
		if port.Protocol == "" {
			port.Protocol = "tcp"
		}
		entry, ok := accumulator[ip]
		if !ok {
			entry = &discoveryAccumulator{ip: ip, seen: map[Port]struct{}{}}
			accumulator[ip] = entry
			order = append(order, ip)
		}
		if hostname != "" && entry.hostname == "" {
			entry.hostname = hostname
		}
		if os != "" && entry.os == "" {
			entry.os = os
		}
		if _, ok := entry.seen[port]; !ok {
			entry.seen[port] = struct{}{}
			entry.ports = append(entry.ports, port)
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var result pscanResult
			if err := json.Unmarshal([]byte(line), &result); err != nil {
				continue
			}
			if !result.Open || !isUsableState(result.State) {
				continue
			}
			hostname, osHint := enrichFromProbes(result)
			accept(result.IP, hostname, osHint, Port{Port: result.Port, Protocol: normalizeProtocol(result.Protocol)})
			continue
		}
		if ip, port, hostname, osHint, ok := parseTextResult(line); ok {
			accept(ip, hostname, osHint, port)
		}
	}

	discoveries := make([]Discovery, 0, len(order))
	for _, ip := range order {
		entry := accumulator[ip]
		discoveries = append(discoveries, Discovery{
			IP:       entry.ip,
			Hostname: entry.hostname,
			OS:       entry.os,
			Ports:    entry.ports,
		})
	}
	return discoveries
}

// parseTextResult handles pscan non-JSON open-port lines such as
// "192.168.1.10:22 open", "192.168.1.10:53/udp open", or enriched lines like
// "192.168.1.10:80 open http status=200 title=\"Router\" server=\"nginx\"" and
// "192.168.1.10:445 open netbios-host=\"WIN-DC01\" netbios-domain=\"CORP\"".
func parseTextResult(line string) (string, Port, string, string, bool) {
	endpoint := line
	rest := ""
	if index := strings.IndexAny(line, " \t"); index >= 0 {
		endpoint = strings.TrimSpace(line[:index])
		rest = strings.TrimSpace(line[index+1:])
	}
	if rest != "" && !strings.HasPrefix(rest, "open") {
		return "", Port{}, "", "", false
	}

	protocol := "tcp"
	if suffix, ok := strings.CutSuffix(endpoint, "/udp"); ok {
		protocol = "udp"
		endpoint = suffix
	} else if suffix, ok := strings.CutSuffix(endpoint, "/tcp"); ok {
		endpoint = suffix
	}

	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", Port{}, "", "", false
	}
	port := 0
	for _, char := range portText {
		if char < '0' || char > '9' {
			return "", Port{}, "", "", false
		}
		port = port*10 + int(char-'0')
		if port > 65535 {
			return "", Port{}, "", "", false
		}
	}
	if port <= 0 {
		return "", Port{}, "", "", false
	}

	ip := normalizeIP(host)
	if ip == "" {
		return "", Port{}, "", "", false
	}
	return ip, Port{Port: port, Protocol: protocol}, textField(rest, "netbios-host"), textOSHint(rest), true
}

// textField extracts a `key="value"` field from a pscan text result line.
func textField(rest, key string) string {
	prefix := key + "=\""
	index := strings.Index(rest, prefix)
	if index < 0 {
		return ""
	}
	value := rest[index+len(prefix):]
	if end := strings.Index(value, "\""); end >= 0 {
		return strings.TrimSpace(value[:end])
	}
	return ""
}

func textOSHint(rest string) string {
	lower := strings.ToLower(rest)
	if strings.Contains(lower, "netbios") {
		return "windows"
	}
	if strings.Contains(lower, "microsoft") || strings.Contains(lower, "iis") {
		return "windows"
	}
	return ""
}

func enrichFromProbes(result pscanResult) (string, string) {
	hostname := ""
	osHint := ""
	if result.NetBIOS != nil {
		hostname = strings.TrimSpace(result.NetBIOS.Hostname)
		osHint = "windows"
	}
	if result.Web != nil {
		server := strings.ToLower(result.Web.Server)
		if osHint == "" && (strings.Contains(server, "microsoft") || strings.Contains(server, "iis")) {
			osHint = "windows"
		}
	}
	return hostname, osHint
}

func isUsableState(state string) bool {
	switch state {
	case "", "open":
		return true
	default:
		return false
	}
}

func normalizeProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		return "udp"
	default:
		return "tcp"
	}
}

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed := net.ParseIP(value)
	if parsed == nil {
		return ""
	}
	return parsed.String()
}
