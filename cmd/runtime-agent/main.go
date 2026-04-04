package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/runtimeagent"
	"github.com/NHAS/reverse_ssh/internal/server"
	"github.com/NHAS/reverse_ssh/internal/server/data"
)

func main() {
	cfg, err := runtimeagent.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		log.Fatal(err)
	}

	dbConfig := data.DatabaseConfig{
		Driver: cfg.DBDriver,
		DSN: fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			cfg.DBHost,
			cfg.DBPort,
			cfg.DBUser,
			cfg.DBPassword,
			cfg.DBName,
			cfg.DBSSLMode,
			cfg.DBTimeZone,
		),
	}
	if err := data.SetDefaultDatabaseConfig(dbConfig); err != nil {
		log.Fatal(err)
	}

	if err := waitForRuntimeDatabase(dbConfig, time.Duration(cfg.DBReadyTimeoutSeconds)*time.Second, time.Duration(cfg.DBReadyPollSeconds)*time.Second); err != nil {
		log.Fatal(err)
	}

	go server.Run(
		cfg.RSSHListenAddr,
		cfg.DataDir,
		cfg.ExternalAddress,
		cfg.ExternalAddress == "",
		cfg.TLSCertPath,
		cfg.TLSKeyPath,
		cfg.Insecure,
		cfg.EnableDownloads,
		cfg.EnableTLS,
		cfg.OpenProxy,
		cfg.Timeout,
	)

	agent := runtimeagent.NewServer(cfg)
	httpServer := agent.HTTPServer()

	log.Printf("Starting runtime agent on %s for project %q", cfg.HTTPAddr, cfg.ProjectName)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func waitForRuntimeDatabase(config data.DatabaseConfig, timeout, pollInterval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		if err := data.LoadDatabaseWithConfig(config); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = fmt.Errorf("database is not ready")
			}
			return fmt.Errorf("wait for runtime database: %w", lastErr)
		}

		time.Sleep(pollInterval)
	}
}
