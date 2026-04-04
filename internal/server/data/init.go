package data

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DatabaseConfig struct {
	Driver string
	DSN    string
}

var (
	db                  *gorm.DB
	dbPath              string
	defaultConfig       DatabaseConfig
	defaultConfigLoaded bool
	dbLocker            sync.Mutex
)

func LoadDatabase(defaultSQLitePath string) error {
	return LoadDatabaseWithConfig(resolveLoadDatabaseConfig(defaultSQLitePath))
}

func LoadDatabaseWithConfig(config DatabaseConfig) (err error) {
	config, err = normalizeDatabaseConfig(config)
	if err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("%s:%s", config.Driver, config.DSN)

	dbLocker.Lock()
	defer dbLocker.Unlock()

	if db != nil && dbPath == cacheKey {
		return nil
	}

	if db != nil {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		db = nil
	}

	switch config.Driver {
	case "postgres":
		db, err = gorm.Open(postgres.Open(config.DSN), &gorm.Config{})
	default:
		db, err = gorm.Open(sqlite.Open(config.DSN), &gorm.Config{})
	}
	if err != nil {
		return err
	}
	dbPath = cacheKey

	if config.Driver == "postgres" {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			sqlDB.SetMaxIdleConns(4)
			sqlDB.SetMaxOpenConns(16)
			sqlDB.SetConnMaxLifetime(30 * time.Minute)
		}
	}

	err = db.AutoMigrate(&Webhook{}, &Download{})
	if err != nil {
		return err
	}

	return nil
}

func SetDefaultDatabaseConfig(config DatabaseConfig) error {
	config, err := normalizeDatabaseConfig(config)
	if err != nil {
		return err
	}

	dbLocker.Lock()
	defer dbLocker.Unlock()

	defaultConfig = config
	defaultConfigLoaded = true
	return nil
}

func resolveLoadDatabaseConfig(defaultSQLitePath string) DatabaseConfig {
	dbLocker.Lock()
	defer dbLocker.Unlock()

	if defaultConfigLoaded {
		return defaultConfig
	}

	return resolveDatabaseConfig(defaultSQLitePath)
}

func resolveDatabaseConfig(defaultSQLitePath string) DatabaseConfig {
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("RSSH_DB_DRIVER")))
	if driver == "" {
		driver = "sqlite"
	}

	if driver == "postgresql" {
		driver = "postgres"
	}

	if driver == "postgres" {
		dsn := strings.TrimSpace(os.Getenv("RSSH_DB_DSN"))
		if dsn == "" {
			dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
		}
		if dsn == "" {
			dsn = buildPostgresDSNFromEnv()
		}
		return DatabaseConfig{
			Driver: "postgres",
			DSN:    dsn,
		}
	}

	sqlitePath := strings.TrimSpace(os.Getenv("RSSH_DB_PATH"))
	if sqlitePath == "" {
		sqlitePath = defaultSQLitePath
	}

	return DatabaseConfig{
		Driver: "sqlite",
		DSN:    sqlitePath,
	}
}

func normalizeDatabaseConfig(config DatabaseConfig) (DatabaseConfig, error) {
	driver := strings.ToLower(strings.TrimSpace(config.Driver))
	if driver == "" {
		driver = "sqlite"
	}

	if driver == "postgresql" {
		driver = "postgres"
	}

	config.Driver = driver
	config.DSN = strings.TrimSpace(config.DSN)

	switch config.Driver {
	case "postgres":
		if config.DSN == "" {
			config.DSN = buildPostgresDSNFromEnv()
		}
		if config.DSN == "" {
			return DatabaseConfig{}, fmt.Errorf("postgres configuration is incomplete: set RSSH_DB_DSN or RSSH_DB_HOST/USER/NAME")
		}
	case "sqlite":
		if config.DSN == "" {
			return DatabaseConfig{}, fmt.Errorf("sqlite database path must not be empty")
		}
	default:
		return DatabaseConfig{}, fmt.Errorf("unsupported database driver %q", config.Driver)
	}

	return config, nil
}

func buildPostgresDSNFromEnv() string {
	host := firstNonEmptyEnv("RSSH_DB_HOST", "POSTGRES_HOST")
	port := firstNonEmptyEnv("RSSH_DB_PORT", "POSTGRES_PORT")
	database := firstNonEmptyEnv("RSSH_DB_NAME", "POSTGRES_DB")
	username := firstNonEmptyEnv("RSSH_DB_USER", "POSTGRES_USER")
	password := firstNonEmptyEnv("RSSH_DB_PASSWORD", "POSTGRES_PASSWORD")
	sslMode := firstNonEmptyEnv("RSSH_DB_SSLMODE", "POSTGRES_SSLMODE")
	timeZone := firstNonEmptyEnv("RSSH_DB_TIMEZONE", "TZ")
	if host == "" || port == "" || database == "" || username == "" || sslMode == "" || timeZone == "" {
		return ""
	}

	parts := []string{
		fmt.Sprintf("host=%s", quoteDSNValue(host)),
		fmt.Sprintf("port=%s", quoteDSNValue(port)),
		fmt.Sprintf("user=%s", quoteDSNValue(username)),
		fmt.Sprintf("dbname=%s", quoteDSNValue(database)),
		fmt.Sprintf("sslmode=%s", quoteDSNValue(sslMode)),
		fmt.Sprintf("TimeZone=%s", quoteDSNValue(timeZone)),
	}

	if password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", quoteDSNValue(password)))
	}

	return strings.Join(parts, " ")
}

func quoteDSNValue(value string) string {
	if value == "" {
		return "''"
	}

	if !strings.ContainsAny(value, " \t\n\r'\\") {
		return value
	}

	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return "'" + escaped + "'"
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func DB() *gorm.DB {
	return db
}
