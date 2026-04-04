package runtimeagent

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/NHAS/reverse_ssh/internal/server/webserver"
)

func (s *Server) handleClients(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": users.ListClientSnapshots(),
	})
}

func (s *Server) handleArtifacts(w http.ResponseWriter, _ *http.Request) {
	artifacts, err := s.service.ListArtifacts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": artifacts,
	})
}

func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	urlPath := path.Clean(strings.TrimPrefix(r.PathValue("urlPath"), "/"))
	if urlPath == "." || urlPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "artifact path is required"})
		return
	}

	artifact, err := s.service.GetArtifact(urlPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, artifact)
}

func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	request := webserver.BuildConfig{}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json payload"})
		return
	}

	result, err := s.service.CreateBuild(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	urlPath := path.Clean(strings.TrimPrefix(r.PathValue("urlPath"), "/"))
	if urlPath == "." || urlPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "artifact path is required"})
		return
	}

	if err := s.service.DeleteArtifact(urlPath); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleKillConnection(w http.ResponseWriter, r *http.Request) {
	connectionID := strings.TrimSpace(r.PathValue("connectionID"))
	if connectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connection id is required"})
		return
	}

	if err := s.service.KillConnection(connectionID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}
