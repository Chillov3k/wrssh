package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/NHAS/reverse_ssh/internal/platform/api"
	"github.com/NHAS/reverse_ssh/internal/platform/auth"
	"github.com/NHAS/reverse_ssh/internal/platform/config"
	"github.com/NHAS/reverse_ssh/internal/platform/orchestrator"
	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	platformruntime "github.com/NHAS/reverse_ssh/internal/platform/runtime"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"github.com/NHAS/reverse_ssh/internal/server"
	"github.com/NHAS/reverse_ssh/internal/server/data"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		log.Fatal(err)
	}

	dbConfig := data.DatabaseConfig{Driver: cfg.DBDriver}
	dbConfig.DSN = cfg.DBDSN
	if cfg.DBDriver != "postgres" {
		dbConfig.DSN = filepath.Join(cfg.DataDir, "data.db")
	}

	if err := data.SetDefaultDatabaseConfig(dbConfig); err != nil {
		log.Fatal(err)
	}

	if err := data.LoadDatabaseWithConfig(dbConfig); err != nil {
		log.Fatal(err)
	}

	appStore, err := store.New(data.DB())
	if err != nil {
		log.Fatal(err)
	}

	if err := appStore.EnsureUser(cfg.AdminUsername, cfg.AdminPassword, cfg.AdminRole, cfg.AdminRSSHUsername); err != nil {
		log.Fatal(err)
	}

	projectRuntimes, err := orchestrator.NewManager(context.Background(), cfg, appStore)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.ProjectRuntimesEnabled {
		go func() {
			if err := projectRuntimes.EnsureProjectRuntimes(context.Background()); err != nil {
				log.Printf("Project runtime bootstrap incomplete: %v", err)
			}
		}()
	} else {
		runtime := platformruntime.New(appStore)
		if err := runtime.Start(); err != nil {
			log.Fatal(err)
		}
		defer runtime.Stop()

		go server.Run(
			cfg.RSSHListenAddr,
			cfg.DataDir,
			cfg.ExternalAddress,
			cfg.ExternalAddress == "",
			cfg.RSSHTLSCertPath,
			cfg.RSSHTLSKeyPath,
			cfg.RSSHInsecure,
			cfg.RSSHEnableLinks,
			cfg.RSSHTLS,
			cfg.RSSHOpenProxy,
			cfg.RSSHTimeout,
		)
	}

	apiServer := api.NewServer(
		cfg,
		appStore,
		auth.New(cfg.SessionSecret),
		rssh.NewService(),
		projectRuntimes,
	)

	log.Printf("Starting web API on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, apiServer.Handler()); err != nil {
		log.Fatal(err)
	}
}
