package runtimeagent

import "testing"

func TestLoadConfigUsesEnvOverrides(t *testing.T) {
	t.Setenv("RUNTIME_AGENT_ADDR", ":48081")
	t.Setenv("RUNTIME_AGENT_TOKEN", "token")
	t.Setenv("RSSH_LISTEN_ADDR", "0.0.0.0:4022")
	t.Setenv("RUNTIME_DB_READY_TIMEOUT_SECONDS", "150")
	t.Setenv("RUNTIME_DB_READY_POLL_INTERVAL_SECONDS", "3")
	t.Setenv("RSSH_DB_HOST", "runtime-postgres")
	t.Setenv("RSSH_DB_PORT", "6543")
	t.Setenv("RSSH_DB_NAME", "runtime")
	t.Setenv("RSSH_DB_USER", "runtime")
	t.Setenv("RSSH_DB_PASSWORD", "secret")
	t.Setenv("RSSH_DB_SSLMODE", "require")
	t.Setenv("RSSH_DB_TIMEZONE", "UTC")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.RSSHListenAddr != "0.0.0.0:4022" || cfg.Timeout != defaultRuntimeRSSHTimeout || cfg.Insecure || cfg.OpenProxy {
		t.Fatalf("unexpected runtime RSSH config: %+v", cfg)
	}
	if cfg.HTTPAddr != ":48081" || cfg.DBDriver != "postgres" || cfg.DBHost != "runtime-postgres" || cfg.DBPort != "6543" || cfg.DBSSLMode != "require" || cfg.DBTimeZone != "UTC" {
		t.Fatalf("unexpected runtime DB config: %+v", cfg)
	}
	if cfg.DBReadyTimeoutSeconds != 150 || cfg.DBReadyPollSeconds != 3 {
		t.Fatalf("unexpected runtime DB wait config: %+v", cfg)
	}
	if cfg.ReadHeaderTimeoutSec != defaultReadHeaderTimeoutSec || cfg.ReadTimeoutSec != defaultReadTimeoutSec || cfg.WriteTimeoutSec != defaultWriteTimeoutSec || cfg.IdleTimeoutSec != defaultIdleTimeoutSec {
		t.Fatalf("unexpected runtime HTTP timeout config: %+v", cfg)
	}
}
