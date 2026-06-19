package system

import (
	"net"
	"net/url"
	"sort"
	"strings"
)

type InterfaceOption struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Display  string `json:"display"`
	Loopback bool   `json:"loopback"`
	Source   string `json:"source"`
}

type BuildOptions struct {
	Interfaces       []InterfaceOption `json:"interfaces"`
	DefaultInterface string            `json:"defaultInterface"`
	DefaultPort      string            `json:"defaultPort"`
	GOOS             []string          `json:"goos"`
	GOARCH           []string          `json:"goarch"`
	BuildFlagHelp    map[string]string `json:"buildFlagHelp"`
}

func DiscoverBuildOptions(externalAddress, listenAddress, advertisedAddresses string) BuildOptions {
	port := addressPort(externalAddress)
	if port == "" {
		port = addressPort(listenAddress)
	}

	interfaces := discoverAdvertisedAddresses(port, externalAddress, advertisedAddresses)
	defaultHost := preferredHost(externalAddress, interfaces)

	return BuildOptions{
		Interfaces:       interfaces,
		DefaultInterface: defaultHost,
		DefaultPort:      port,
		GOOS:             supportedGOOS(),
		GOARCH:           supportedGOARCH(),
	}
}

func discoverAdvertisedAddresses(port, externalAddress, advertisedAddresses string) []InterfaceOption {
	var result []InterfaceOption
	seen := map[string]bool{}
	configured := parseAdvertisedAddresses(advertisedAddresses)

	appendOption := func(name, address, source string, loopback bool) {
		host := normalizedAdvertisedHost(address)
		if host == "" || seen[host] {
			return
		}

		seen[host] = true
		label := name
		if strings.TrimSpace(label) == "" {
			label = host
		}

		result = append(result, InterfaceOption{
			Name:     label,
			Address:  host,
			Display:  label + " · " + net.JoinHostPort(host, port),
			Loopback: loopback,
			Source:   source,
		})
	}

	if host := normalizedAdvertisedHost(externalAddress); host != "" && !containsAddress(configured, host) {
		appendOption("external", host, "external", host == "127.0.0.1")
	}

	for _, option := range configured {
		appendOption(option.Name, option.Address, "configured", option.Loopback)
	}

	appendOption("loopback", "127.0.0.1", "loopback", true)

	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return optionPriority(result[i].Source) < optionPriority(result[j].Source)
		}
		if result[i].Loopback != result[j].Loopback {
			return !result[i].Loopback
		}
		if result[i].Name == result[j].Name {
			return result[i].Address < result[j].Address
		}
		return result[i].Name < result[j].Name
	})

	return result
}

func containsAddress(options []InterfaceOption, address string) bool {
	for _, option := range options {
		if option.Address == address {
			return true
		}
	}

	return false
}

func parseAdvertisedAddresses(raw string) []InterfaceOption {
	chunks := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})

	result := make([]InterfaceOption, 0, len(chunks))
	for _, chunk := range chunks {
		value := strings.TrimSpace(chunk)
		if value == "" {
			continue
		}

		name := ""
		address := value
		if left, right, ok := strings.Cut(value, "="); ok {
			name = strings.TrimSpace(left)
			address = strings.TrimSpace(right)
		}

		host := normalizedAdvertisedHost(address)
		if host == "" {
			continue
		}

		if name == "" {
			name = host
		}

		result = append(result, InterfaceOption{
			Name:     name,
			Address:  host,
			Loopback: host == "127.0.0.1",
		})
	}

	return result
}

func optionPriority(source string) int {
	switch source {
	case "external":
		return 0
	case "configured":
		return 1
	case "loopback":
		return 2
	default:
		return 3
	}
}

func preferredHost(externalAddress string, options []InterfaceOption) string {
	host := normalizedAdvertisedHost(externalAddress)
	if host != "" {
		return host
	}

	for _, option := range options {
		if !option.Loopback {
			return option.Address
		}
	}

	if len(options) > 0 {
		return options[0].Address
	}

	return host
}

func supportedGOOS() []string {
	return []string{"linux", "windows", "darwin", "freebsd"}
}

func supportedGOARCH() []string {
	return []string{"amd64", "arm64", "386", "mips", "mipsle", "mips64", "mips64le"}
}

func normalizedAdvertisedHost(value string) string {
	host := addressHost(strings.TrimSpace(value))
	if host == "" || host == "0.0.0.0" || host == "::" {
		return ""
	}
	return host
}

func addressHost(value string) string {
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Host
		}
	}

	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return strings.Trim(host, "[]")
	}

	return strings.Trim(value, "[]")
}

func addressPort(value string) string {
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Host
		}
	}

	_, port, err := net.SplitHostPort(value)
	if err == nil {
		return port
	}

	return ""
}
