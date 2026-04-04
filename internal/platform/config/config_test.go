package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveRuntimePublishedHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "192.168.1.42:2222", want: "192.168.1.42"},
		{input: "https://rssh.example:9443", want: "rssh.example"},
		{input: ":2222", want: ""},
		{input: "rssh.example", want: "rssh.example"},
	}

	for _, test := range tests {
		if got := deriveRuntimePublishedHost(test.input); got != test.want {
			t.Fatalf("deriveRuntimePublishedHost(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestLoadUsesWebUserAndGeneratedSecrets(t *testing.T) {
	dataDir := t.TempDir()

	t.Setenv("RSSH_DATA_DIR", dataDir)
	t.Setenv("WEB_USER", "operator1")
	t.Setenv("POSTGRES_USER", "dbuser")
	t.Setenv("POSTGRES_PASSWORD", "dbpass")
	t.Setenv("POSTGRES_DB", "rssh")
	t.Setenv("RSSH_EXTERNAL_ADDRESS", "192.168.1.42:2222")
	t.Setenv("RSSH_PORT", "3022")
	t.Setenv("WEB_RUNTIME_SSH_PORT_START", "2300")
	t.Setenv("WEB_RUNTIME_SSH_PORT_END", "2399")
	t.Setenv("WEB_RUNTIME_AGENT_PORT_START", "2400")
	t.Setenv("WEB_RUNTIME_AGENT_PORT_END", "2499")
	t.Setenv("WEB_RUNTIME_AGENT_HOST", "host.docker.internal")
	t.Setenv("WEB_RUNTIME_BIND_IP", "127.0.0.1")
	t.Setenv("WEB_SESSION_SECRET", "should-be-ignored")
	t.Setenv("WEB_RUNTIME_SECRET_KEY", "not-a-valid-master-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AdminUsername != "operator1" {
		t.Fatalf("AdminUsername = %q, want %q", cfg.AdminUsername, "operator1")
	}
	if cfg.AdminRSSHUsername != "operator1" {
		t.Fatalf("AdminRSSHUsername = %q, want %q", cfg.AdminRSSHUsername, "operator1")
	}
	if !cfg.ProjectRuntimesEnabled {
		t.Fatalf("ProjectRuntimesEnabled = false, want true")
	}
	if got := cfg.SessionSecret; got == "" || got == "should-be-ignored" {
		t.Fatalf("SessionSecret = %q, expected generated persisted secret", got)
	}
	if len(cfg.RuntimeSecretKey) != 32 {
		t.Fatalf("RuntimeSecretKey len = %d, want 32", len(cfg.RuntimeSecretKey))
	}
	if cfg.RSSHListenPort != 3022 {
		t.Fatalf("RSSHListenPort = %d, want 3022", cfg.RSSHListenPort)
	}
	if cfg.RSSHPort != 3022 || cfg.RSSHListenAddr != ":3022" {
		t.Fatalf("unexpected RSSH port config: port=%d listen=%q", cfg.RSSHPort, cfg.RSSHListenAddr)
	}
	if cfg.RSSHTimeout != defaultRSSHTimeout || cfg.RSSHInsecure || cfg.RSSHOpenProxy {
		t.Fatalf("unexpected RSSH flags: timeout=%d insecure=%t openProxy=%t", cfg.RSSHTimeout, cfg.RSSHInsecure, cfg.RSSHOpenProxy)
	}
	if cfg.RuntimeBindIP != "127.0.0.1" || cfg.RuntimeDBHostAlias != defaultRuntimeDBHostAlias || cfg.RuntimeDBPort != defaultRuntimeDBPort || cfg.RuntimeDBSSLMode != defaultRuntimeDBSSLMode {
		t.Fatalf("unexpected runtime network/db config: %+v", cfg)
	}
	if cfg.RuntimeAPITimeout != defaultRuntimeAPITimeout || cfg.RuntimeProvisionSec != defaultRuntimeProvisionSec || cfg.RuntimeHealthPollSec != defaultRuntimeHealthPollSec || cfg.RuntimeDBGraceSec != defaultRuntimeDBGraceSec || cfg.RuntimeDockerAPITimeoutSec != defaultDockerAPITimeoutSec {
		t.Fatalf("unexpected runtime timeout config: %+v", cfg)
	}
	if !strings.Contains(cfg.DBDSN, "user=dbuser") {
		t.Fatalf("DBDSN = %q, want postgres user from POSTGRES_USER", cfg.DBDSN)
	}
	if _, err := os.Stat(filepath.Join(dataDir, sessionSecretFileName)); err != nil {
		t.Fatalf("session secret file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, runtimeSecretFileName)); err != nil {
		t.Fatalf("runtime secret file missing: %v", err)
	}
}
