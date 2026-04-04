package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NHAS/reverse_ssh/internal/platform/secrets"
)

const (
	defaultHTTPAddr              = ":8080"
	defaultDataDir               = "./data"
	defaultDBDriver              = "postgres"
	defaultDBTimeZone            = "UTC"
	defaultRSSHPort              = 2222
	defaultRSSHTimeout           = 5
	defaultAdminUsername         = "admin"
	defaultAdminRole             = "admin"
	defaultRuntimeSSHPortStart   = 2300
	defaultRuntimeSSHPortEnd     = 2399
	defaultRuntimeAgentPort      = 8081
	defaultRuntimeAgentPortStart = 2400
	defaultRuntimeAgentPortEnd   = 2499
	defaultRuntimeDockerSocket   = "/var/run/docker.sock"
	defaultRuntimeImage          = "wrssh-runtime:local"
	defaultRuntimePostgresImage  = "postgres:17-alpine"
	defaultRuntimeAgentHost      = "host.docker.internal"
	defaultRuntimeBindIP         = "0.0.0.0"
	defaultRuntimeDBHostAlias    = "postgres"
	defaultRuntimeDBPort         = "5432"
	defaultRuntimeDBSSLMode      = "disable"
	defaultRuntimeAPITimeout     = 25
	defaultRuntimeProvisionSec   = 120
	defaultRuntimeHealthPollSec  = 1
	defaultRuntimeDBGraceSec     = 15
	defaultDockerAPITimeoutSec   = 30
	sessionSecretFileName        = ".web_session_secret"
	runtimeSecretFileName        = ".runtime_master_key"
)

type Config struct {
	HTTPAddr                   string
	DataDir                    string
	DBDriver                   string
	DBDSN                      string
	DBTimeZone                 string
	AdvertisedAddrs            string
	ExternalAddress            string
	RSSHPort                   int
	RSSHListenAddr             string
	RSSHListenPort             int
	RSSHTLS                    bool
	RSSHTLSCertPath            string
	RSSHTLSKeyPath             string
	RSSHInsecure               bool
	RSSHOpenProxy              bool
	RSSHEnableLinks            bool
	RSSHTimeout                int
	AdminUsername              string
	AdminPassword              string
	AdminRole                  string
	AdminRSSHUsername          string
	SeedAuthorizedKeys         string
	SessionSecret              string
	ProjectRuntimesEnabled     bool
	RuntimeSSHPortStart        int
	RuntimeSSHPortEnd          int
	RuntimeAgentPort           int
	RuntimeAgentPortStart      int
	RuntimeAgentPortEnd        int
	RuntimeDockerSocket        string
	RuntimeImage               string
	RuntimePostgresImage       string
	RuntimeAgentHost           string
	RuntimePublishedHost       string
	RuntimeBindIP              string
	RuntimeDBHostAlias         string
	RuntimeDBPort              string
	RuntimeDBSSLMode           string
	RuntimeAPITimeout          int
	RuntimeProvisionSec        int
	RuntimeHealthPollSec       int
	RuntimeDBGraceSec          int
	RuntimeDockerAPITimeoutSec int
	RuntimeSecretKey           []byte
}

func Load() (Config, error) {
	dataDir := env("RSSH_DATA_DIR", defaultDataDir)
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data dir: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return Config{}, fmt.Errorf("prepare data dir: %w", err)
	}

	sessionSecret, err := resolveSessionSecret(dataDir)
	if err != nil {
		return Config{}, err
	}
	runtimeSecretKey, err := resolveRuntimeSecretKey(dataDir)
	if err != nil {
		return Config{}, err
	}

	httpAddr := strings.TrimSpace(env("WEB_HTTP_ADDR", defaultHTTPAddr))
	runtimeAgentHost, err := envRequired("WEB_RUNTIME_AGENT_HOST")
	if err != nil {
		return Config{}, err
	}
	runtimeBindIP, err := envRequired("WEB_RUNTIME_BIND_IP")
	if err != nil {
		return Config{}, err
	}
	rsshPort, err := envIntRequired("RSSH_PORT")
	if err != nil {
		return Config{}, err
	}
	runtimeSSHPortStart := envInt("WEB_RUNTIME_SSH_PORT_START", defaultRuntimeSSHPortStart)
	runtimeSSHPortEnd := envInt("WEB_RUNTIME_SSH_PORT_END", defaultRuntimeSSHPortEnd)
	runtimeAgentPortStart := envInt("WEB_RUNTIME_AGENT_PORT_START", defaultRuntimeAgentPortStart)
	runtimeAgentPortEnd := envInt("WEB_RUNTIME_AGENT_PORT_END", defaultRuntimeAgentPortEnd)

	cfg := Config{
		HTTPAddr:                   httpAddr,
		DataDir:                    dataDir,
		DBDriver:                   defaultDBDriver,
		DBTimeZone:                 firstNonEmpty("RSSH_DB_TIMEZONE", "TZ", defaultDBTimeZone),
		AdvertisedAddrs:            env("RSSH_ADVERTISED_ADDRESSES", ""),
		ExternalAddress:            env("RSSH_EXTERNAL_ADDRESS", ""),
		RSSHPort:                   rsshPort,
		RSSHListenAddr:             fmt.Sprintf(":%d", rsshPort),
		RSSHTLS:                    envBool("RSSH_TLS", false),
		RSSHTLSCertPath:            env("RSSH_TLS_CERT_PATH", ""),
		RSSHTLSKeyPath:             env("RSSH_TLS_KEY_PATH", ""),
		RSSHInsecure:               false,
		RSSHOpenProxy:              false,
		RSSHEnableLinks:            true,
		RSSHTimeout:                defaultRSSHTimeout,
		AdminUsername:              firstNonEmpty("WEB_USER", "WEB_ADMIN_USERNAME", defaultAdminUsername),
		AdminPassword:              env("WEB_ADMIN_PASSWORD", "admin"),
		AdminRole:                  env("WEB_ADMIN_ROLE", defaultAdminRole),
		AdminRSSHUsername:          strings.TrimSpace(firstNonEmpty("WEB_RSSH_USER", "WEB_ADMIN_RSSH_USERNAME", "")),
		SeedAuthorizedKeys:         strings.TrimSpace(env("SEED_AUTHORIZED_KEYS", "")),
		SessionSecret:              sessionSecret,
		ProjectRuntimesEnabled:     true,
		RuntimeSSHPortStart:        runtimeSSHPortStart,
		RuntimeSSHPortEnd:          runtimeSSHPortEnd,
		RuntimeAgentPort:           defaultRuntimeAgentPort,
		RuntimeAgentPortStart:      runtimeAgentPortStart,
		RuntimeAgentPortEnd:        runtimeAgentPortEnd,
		RuntimeDockerSocket:        env("WEB_RUNTIME_DOCKER_SOCKET", defaultRuntimeDockerSocket),
		RuntimeImage:               env("WEB_RUNTIME_IMAGE", defaultRuntimeImage),
		RuntimePostgresImage:       env("WEB_RUNTIME_POSTGRES_IMAGE", defaultRuntimePostgresImage),
		RuntimeAgentHost:           runtimeAgentHost,
		RuntimePublishedHost:       strings.TrimSpace(env("WEB_RUNTIME_PUBLISHED_HOST", "")),
		RuntimeBindIP:              runtimeBindIP,
		RuntimeDBHostAlias:         defaultRuntimeDBHostAlias,
		RuntimeDBPort:              defaultRuntimeDBPort,
		RuntimeDBSSLMode:           defaultRuntimeDBSSLMode,
		RuntimeAPITimeout:          defaultRuntimeAPITimeout,
		RuntimeProvisionSec:        defaultRuntimeProvisionSec,
		RuntimeHealthPollSec:       defaultRuntimeHealthPollSec,
		RuntimeDBGraceSec:          defaultRuntimeDBGraceSec,
		RuntimeDockerAPITimeoutSec: defaultDockerAPITimeoutSec,
		RuntimeSecretKey:           runtimeSecretKey,
	}

	if cfg.AdminRole != "admin" && cfg.AdminRole != "operator" {
		return Config{}, fmt.Errorf("WEB_ADMIN_ROLE must be admin or operator")
	}

	cfg.AdminUsername = strings.TrimSpace(cfg.AdminUsername)
	if cfg.AdminRSSHUsername == "" {
		cfg.AdminRSSHUsername = cfg.AdminUsername
	}
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" || cfg.SessionSecret == "" {
		return Config{}, fmt.Errorf("admin username, password and session secret must not be empty")
	}
	if cfg.ExternalAddress == "" && cfg.RSSHEnableLinks {
		cfg.ExternalAddress = cfg.RSSHListenAddr
	}
	cfg.RSSHListenPort = cfg.RSSHPort
	if cfg.DBDriver == "postgres" {
		cfg.DBDSN = strings.TrimSpace(env("RSSH_DB_DSN", ""))
		if cfg.DBDSN == "" {
			cfg.DBDSN = defaultPostgresDSN()
		}
		if cfg.DBDSN == "" {
			return Config{}, fmt.Errorf("postgres configuration is incomplete: set RSSH_DB_DSN or RSSH_DB_HOST/PORT/NAME/USER/PASSWORD/SSLMODE/TIMEZONE")
		}
	}

	if len(cfg.RuntimeSecretKey) == 0 {
		return Config{}, fmt.Errorf("runtime master key must not be empty")
	}
	if cfg.RuntimeSSHPortStart <= 0 || cfg.RuntimeSSHPortEnd <= 0 || cfg.RuntimeSSHPortStart > cfg.RuntimeSSHPortEnd {
		return Config{}, fmt.Errorf("runtime SSH port range is invalid")
	}
	if cfg.RuntimeAgentPortStart <= 0 || cfg.RuntimeAgentPortEnd <= 0 || cfg.RuntimeAgentPortStart > cfg.RuntimeAgentPortEnd {
		return Config{}, fmt.Errorf("runtime agent port range is invalid")
	}
	if strings.TrimSpace(cfg.RuntimeAgentHost) == "" {
		return Config{}, fmt.Errorf("WEB_RUNTIME_AGENT_HOST must not be empty")
	}
	if strings.TrimSpace(cfg.RuntimeBindIP) == "" {
		return Config{}, fmt.Errorf("WEB_RUNTIME_BIND_IP must not be empty")
	}
	if cfg.RuntimePublishedHost == "" {
		cfg.RuntimePublishedHost = deriveRuntimePublishedHost(cfg.ExternalAddress)
	}
	if cfg.RuntimePublishedHost == "" {
		return Config{}, fmt.Errorf("WEB_RUNTIME_PUBLISHED_HOST must be set")
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envRequired(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be empty", key)
	}
	return strings.TrimSpace(value), nil
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

func envIntRequired(key string) (int, error) {
	value, err := envRequired(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func defaultPostgresDSN() string {
	host := firstNonEmpty("RSSH_DB_HOST", "POSTGRES_HOST", "postgres")
	port := firstNonEmpty("RSSH_DB_PORT", "POSTGRES_PORT", "5432")
	database := firstNonEmpty("RSSH_DB_NAME", "POSTGRES_DB", "rssh")
	username := firstNonEmpty("RSSH_DB_USER", "POSTGRES_USER", "rssh")
	password := firstNonEmpty("RSSH_DB_PASSWORD", "POSTGRES_PASSWORD", "")
	sslMode := firstNonEmpty("RSSH_DB_SSLMODE", "POSTGRES_SSLMODE", "disable")
	timeZone := firstNonEmpty("RSSH_DB_TIMEZONE", "TZ", defaultDBTimeZone)

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s", host, port, username, password, database, sslMode, timeZone)
}

func firstNonEmpty(keysAndFallback ...string) string {
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

func resolveSessionSecret(dataDir string) (string, error) {
	return loadOrCreateSecretFile(filepath.Join(dataDir, sessionSecretFileName), func() (string, error) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(raw), nil
	})
}

func resolveRuntimeSecretKey(dataDir string) ([]byte, error) {
	encoded, err := loadOrCreateSecretFile(filepath.Join(dataDir, runtimeSecretFileName), func() (string, error) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(raw), nil
	})
	if err != nil {
		return nil, err
	}

	return secrets.ParseMasterKey(encoded)
}

func loadOrCreateSecretFile(path string, generator func() (string, error)) (string, error) {
	if value, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(value)), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	value, err := generator()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0600); err != nil {
		return "", err
	}
	return value, nil
}

func deriveRuntimePublishedHost(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			return strings.TrimSpace(parsed.Hostname())
		}
	}

	host, _, err := net.SplitHostPort(trimmed)
	if err == nil {
		return strings.TrimSpace(host)
	}

	if strings.HasPrefix(trimmed, ":") {
		return ""
	}

	return trimmed
}
