package runtimeagent

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal"
	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/userkeys"
	"golang.org/x/net/websocket"
)

var runtimeUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Server struct {
	cfg     Config
	service *rssh.Service
}

type accessUser struct {
	Username       string `json:"username"`
	AuthorizedKeys string `json:"authorizedKeys"`
}

func NewServer(cfg Config) *Server {
	return &Server{
		cfg:     cfg,
		service: rssh.NewService(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/health", s.handleHealth)
	mux.HandleFunc("PUT /internal/access/users", s.handleReplaceUsers)
	mux.HandleFunc("GET /internal/clients", s.handleClients)
	mux.HandleFunc("GET /internal/artifacts", s.handleArtifacts)
	mux.HandleFunc("POST /internal/artifacts", s.handleCreateArtifact)
	mux.HandleFunc("GET /internal/artifacts/{urlPath}", s.handleArtifact)
	mux.HandleFunc("DELETE /internal/artifacts/{urlPath}", s.handleDeleteArtifact)
	mux.HandleFunc("POST /internal/connections/{connectionID}/kill", s.handleKillConnection)
	mux.HandleFunc("POST /internal/connections/{connectionID}/exec", s.handleExecuteConnectionCommand)
	mux.HandleFunc("GET /internal/connections/{connectionID}/filesystem", s.handleListConnectionFilesystem)
	mux.HandleFunc("GET /internal/connections/{connectionID}/filesystem/download", s.handleDownloadConnectionFile)
	mux.HandleFunc("GET /internal/connections/{connectionID}/filesystem/preview", s.handlePreviewConnectionFile)
	mux.HandleFunc("POST /internal/connections/{connectionID}/filesystem/upload", s.handleUploadConnectionFile)
	mux.Handle("GET /internal/ws/terminal/{stableID}", websocket.Handler(s.handleTerminalWebsocket))
	return s.requireBearer(mux)
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: time.Duration(s.cfg.ReadHeaderTimeoutSec) * time.Second,
		ReadTimeout:       time.Duration(s.cfg.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(s.cfg.WriteTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(s.cfg.IdleTimeoutSec) * time.Second,
	}
}

func (s *Server) requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AgentToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "unauthorized",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"project":         s.cfg.ProjectName,
		"version":         internal.Version,
		"listenAddress":   s.cfg.RSSHListenAddr,
		"externalAddress": s.cfg.ExternalAddress,
		"downloads":       s.cfg.EnableDownloads,
		"time":            time.Now().UTC(),
	})
}

func (s *Server) handleReplaceUsers(w http.ResponseWriter, r *http.Request) {
	request := struct {
		Users []accessUser `json:"users"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid json payload",
		})
		return
	}

	count, err := replaceUserKeys(s.cfg.DataDir, request.Users)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"users": count,
	})
}

func replaceUserKeys(dataDir string, users []accessUser) (int, error) {
	keysDir := filepath.Join(dataDir, "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return 0, err
	}

	desired := make(map[string]string, len(users))
	for _, user := range users {
		username := strings.TrimSpace(user.Username)
		if !runtimeUsernamePattern.MatchString(username) {
			return 0, fmt.Errorf("invalid runtime username %q", username)
		}

		clean, err := userkeys.ValidateAuthorizedKeys(user.AuthorizedKeys)
		if err != nil {
			return 0, fmt.Errorf("invalid authorized keys for %s: %w", username, err)
		}
		if clean == "" {
			continue
		}
		desired[username] = clean
	}

	for username, keys := range desired {
		if err := userkeys.SyncUserAuthorizedKeys(dataDir, username, keys); err != nil {
			return 0, err
		}
	}

	entries, err := os.ReadDir(keysDir)
	if err != nil {
		return 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if _, ok := desired[name]; ok {
			continue
		}

		if err := userkeys.RemoveUserAuthorizedKeys(dataDir, name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
	}

	return len(desired), nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
