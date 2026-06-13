//go:build pscan

package pscan

import "testing"

func TestParseArgs(t *testing.T) {
	cfg, err := ParseArgs([]string{
		"--ips", "127.0.0.1,127.0.0.2/32",
		"--ports", "22,80-81",
		"--timeout", "100ms",
		"--workers", "4",
		"--rate", "10",
		"--exclude", "127.0.0.2",
		"--json",
	})
	if err != nil {
		t.Fatalf("ParseArgs returned error: %v", err)
	}
	if len(cfg.Hosts) != 1 || cfg.Hosts[0].String() != "127.0.0.1" {
		t.Fatalf("hosts = %v, want only 127.0.0.1", cfg.Hosts)
	}
	if got, want := len(cfg.Ports), 3; got != want {
		t.Fatalf("port count = %d, want %d", got, want)
	}
	if cfg.Ports[0] != 22 || cfg.Ports[1] != 80 || cfg.Ports[2] != 81 {
		t.Fatalf("ports = %v", cfg.Ports)
	}
	if !cfg.JSON {
		t.Fatalf("JSON flag was not set")
	}
}

func TestParseArgsDefaultsPortsAndResolvesHostnames(t *testing.T) {
	cfg, err := ParseArgs([]string{
		"-h", "localhost",
	})
	if err != nil {
		t.Fatalf("ParseArgs returned error: %v", err)
	}
	if len(cfg.Hosts) == 0 {
		t.Fatalf("expected localhost to resolve at least one host")
	}
	if got, want := cfg.Ports, []int{21, 22, 80, 81, 135, 139, 443, 445, 1433, 1521, 3306, 5432, 6379, 7001, 8000, 8080, 8089, 9000, 9200, 11211, 27017}; !sameInts(got, want) {
		t.Fatalf("ports = %v, want %v", got, want)
	}
}

func TestParseArgsAcceptsFscanAliasesAndAllPorts(t *testing.T) {
	cfg, err := ParseArgs([]string{
		"-h127.0.0.1",
		"-p", "all",
		"-t", "7",
		"-time", "2",
	})
	if err != nil {
		t.Fatalf("ParseArgs returned error: %v", err)
	}
	if got, want := len(cfg.Ports), 65535; got != want {
		t.Fatalf("port count = %d, want %d", got, want)
	}
	if cfg.Ports[0] != 1 || cfg.Ports[len(cfg.Ports)-1] != 65535 {
		t.Fatalf("ports bounds = %d..%d, want 1..65535", cfg.Ports[0], cfg.Ports[len(cfg.Ports)-1])
	}
	if cfg.Workers != 7 {
		t.Fatalf("workers = %d, want 7", cfg.Workers)
	}
	if cfg.Timeout.String() != "2s" {
		t.Fatalf("timeout = %s, want 2s", cfg.Timeout)
	}
}

func TestParseArgsEnforcesLimits(t *testing.T) {
	if _, err := ParseArgs([]string{"--ips", "127.0.0.1", "--ports", "1", "--workers", "99999"}); err == nil {
		t.Fatalf("expected worker limit error")
	}
	if _, err := ParseArgs([]string{"--ips", "127.0.0.0/16", "--ports", "1"}); err == nil {
		t.Fatalf("expected host limit error")
	}
	if _, err := ParseArgs([]string{"--ips", "127.0.0.1", "--ports", "65536"}); err == nil {
		t.Fatalf("expected invalid port error")
	}
}

func sameInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
