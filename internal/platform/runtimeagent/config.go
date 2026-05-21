package runtimeagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultRuntimeAgentAddr         = ":8081"
	defaultRuntimeDBDriver          = "postgres"
	defaultRuntimeDBHost            = "postgres"
	defaultRuntimeDBPort            = "5432"
	defaultRuntimeDBSSLMode         = "disable"
	defaultRuntimeDBTimeZone        = "UTC"
	defaultRuntimeDBReadyTimeoutSec = 20
	defaultRuntimeDBReadyPollSec    = 1
	defaultRuntimeRSSHListenAddr    = ":2222"
	defaultRuntimeRSSHTimeout       = 5
	defaultReadHeaderTimeoutSec     = 5
	defaultReadTimeoutSec           = 600
	defaultWriteTimeoutSec          = 600
	defaultIdleTimeoutSec           = 60
)

type Config struct {
	HTTPAddr              string
	DataDir               string
	ProjectName           string
	AgentToken            string
	DBDriver              string
	DBHost                string
	DBPort                string
	DBName                string
	DBUser                string
	DBPassword            string
	DBSSLMode             string
	DBTimeZone            string
	DBReadyTimeoutSeconds int
	DBReadyPollSeconds    int
	RSSHListenAddr        string
	ExternalAddress       string
	TLSCertPath           string
	TLSKeyPath            string
	Insecure              bool
	EnableDownloads       bool
	EnableTLS             bool
	OpenProxy             bool
	Timeout               int
	ReadHeaderTimeoutSec  int
	ReadTimeoutSec        int
	WriteTimeoutSec       int
	IdleTimeoutSec        int
}

func LoadConfig() (Config, error) {
	cfg := Config{
		HTTPAddr:              strings.TrimSpace(env("RUNTIME_AGENT_ADDR", defaultRuntimeAgentAddr)),
		DataDir:               env("RSSH_DATA_DIR", "./data"),
		ProjectName:           strings.TrimSpace(env("RUNTIME_PROJECT_NAME", "")),
		AgentToken:            strings.TrimSpace(env("RUNTIME_AGENT_TOKEN", "")),
		DBDriver:              strings.TrimSpace(env("RSSH_DB_DRIVER", defaultRuntimeDBDriver)),
		DBHost:                firstNonEmptyEnv("RSSH_DB_HOST", "POSTGRES_HOST", defaultRuntimeDBHost),
		DBPort:                firstNonEmptyEnv("RSSH_DB_PORT", "POSTGRES_PORT", defaultRuntimeDBPort),
		DBName:                firstNonEmptyEnv("RSSH_DB_NAME", "POSTGRES_DB", "rssh"),
		DBUser:                firstNonEmptyEnv("RSSH_DB_USER", "POSTGRES_USER", "rssh"),
		DBPassword:            firstNonEmptyEnv("RSSH_DB_PASSWORD", "POSTGRES_PASSWORD", ""),
		DBSSLMode:             firstNonEmptyEnv("RSSH_DB_SSLMODE", "POSTGRES_SSLMODE", defaultRuntimeDBSSLMode),
		DBTimeZone:            firstNonEmptyEnv("RSSH_DB_TIMEZONE", "TZ", defaultRuntimeDBTimeZone),
		DBReadyTimeoutSeconds: envInt("RUNTIME_DB_READY_TIMEOUT_SECONDS", defaultRuntimeDBReadyTimeoutSec),
		DBReadyPollSeconds:    envInt("RUNTIME_DB_READY_POLL_INTERVAL_SECONDS", defaultRuntimeDBReadyPollSec),
		RSSHListenAddr:        strings.TrimSpace(env("RSSH_LISTEN_ADDR", defaultRuntimeRSSHListenAddr)),
		ExternalAddress:       strings.TrimSpace(env("RSSH_EXTERNAL_ADDRESS", "")),
		TLSCertPath:           strings.TrimSpace(env("RSSH_TLS_CERT_PATH", "")),
		TLSKeyPath:            strings.TrimSpace(env("RSSH_TLS_KEY_PATH", "")),
		Insecure:              false,
		EnableDownloads:       true,
		EnableTLS:             envBool("RSSH_TLS", false),
		OpenProxy:             false,
		Timeout:               defaultRuntimeRSSHTimeout,
		ReadHeaderTimeoutSec:  defaultReadHeaderTimeoutSec,
		ReadTimeoutSec:        defaultReadTimeoutSec,
		WriteTimeoutSec:       defaultWriteTimeoutSec,
		IdleTimeoutSec:        defaultIdleTimeoutSec,
	}

	dataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve runtime data dir: %w", err)
	}
	cfg.DataDir = dataDir

	if cfg.AgentToken == "" {
		return Config{}, fmt.Errorf("RUNTIME_AGENT_TOKEN must not be empty")
	}
	if cfg.DBReadyTimeoutSeconds <= 0 || cfg.DBReadyPollSeconds <= 0 {
		return Config{}, fmt.Errorf("runtime database wait settings must be greater than zero")
	}
	if cfg.ReadHeaderTimeoutSec <= 0 || cfg.ReadTimeoutSec <= 0 || cfg.WriteTimeoutSec <= 0 || cfg.IdleTimeoutSec <= 0 {
		return Config{}, fmt.Errorf("runtime agent HTTP timeout settings must be greater than zero")
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func firstNonEmptyEnv(keysAndFallback ...string) string {
	if len(keysAndFallback) == 0 {
		return ""
	}
	last := keysAndFallback[len(keysAndFallback)-1]
	for _, key := range keysAndFallback[:len(keysAndFallback)-1] {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return last
}
