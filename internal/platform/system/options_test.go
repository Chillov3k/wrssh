package system

import "testing"

func TestDiscoverBuildOptionsUsesAdvertisedAddresses(t *testing.T) {
	options := DiscoverBuildOptions("192.168.1.42:2222", ":2222", "en0=192.168.1.138,lo0=127.0.0.1")

	if options.DefaultInterface != "192.168.1.42" {
		t.Fatalf("expected default interface to come from external address, got %q", options.DefaultInterface)
	}

	if options.DefaultPort != "2222" {
		t.Fatalf("expected default port 2222, got %q", options.DefaultPort)
	}

	if !hasAddress(options.Interfaces, "192.168.1.42", "external") {
		t.Fatalf("expected external address option to be present: %#v", options.Interfaces)
	}

	if !hasAddress(options.Interfaces, "192.168.1.138", "configured") {
		t.Fatalf("expected configured host interface to be present: %#v", options.Interfaces)
	}

	if !hasAddress(options.Interfaces, "127.0.0.1", "configured") {
		t.Fatalf("expected configured loopback to be present: %#v", options.Interfaces)
	}
}

func TestDiscoverBuildOptionsFallsBackToConfiguredHost(t *testing.T) {
	options := DiscoverBuildOptions(":2222", ":2222", "en0=192.168.1.138")

	if options.DefaultInterface != "192.168.1.138" {
		t.Fatalf("expected configured host as default interface, got %q", options.DefaultInterface)
	}

	if hasAddress(options.Interfaces, "0.0.0.0", "") {
		t.Fatalf("wildcard bind address must not be exposed as a connect-back option: %#v", options.Interfaces)
	}

	if !hasAddress(options.Interfaces, "127.0.0.1", "loopback") {
		t.Fatalf("expected built-in loopback option to be present: %#v", options.Interfaces)
	}
}

func TestDiscoverBuildOptionsPrefersConfiguredLabelOverExternalDuplicate(t *testing.T) {
	options := DiscoverBuildOptions("192.168.1.138:2222", ":2222", "en0=192.168.1.138,lo0=127.0.0.1")

	if !hasNamedAddress(options.Interfaces, "en0", "192.168.1.138") {
		t.Fatalf("expected configured en0 label to be preserved: %#v", options.Interfaces)
	}

	if hasNamedAddress(options.Interfaces, "external", "192.168.1.138") {
		t.Fatalf("did not expect duplicate generic external label when a configured interface exists: %#v", options.Interfaces)
	}
}

func TestSupportedGOARCHIncludesMIPS(t *testing.T) {
	options := supportedGOARCH()
	for _, arch := range []string{"mips", "mipsle", "mips64", "mips64le"} {
		if !hasString(options, arch) {
			t.Fatalf("expected supported GOARCH list to include %q: %v", arch, options)
		}
	}
}

func TestSupportedGOOSIncludesFreeBSD(t *testing.T) {
	options := supportedGOOS()
	if !hasString(options, "freebsd") {
		t.Fatalf("expected supported GOOS list to include freebsd: %v", options)
	}
}

func hasAddress(options []InterfaceOption, address, source string) bool {
	for _, option := range options {
		if option.Address != address {
			continue
		}

		if source == "" || option.Source == source {
			return true
		}
	}

	return false
}

func hasString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}

	return false
}

func hasNamedAddress(options []InterfaceOption, name, address string) bool {
	for _, option := range options {
		if option.Name == name && option.Address == address {
			return true
		}
	}

	return false
}
