package runtimeagent

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
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

func (s *Server) handleExecuteConnectionCommand(w http.ResponseWriter, r *http.Request) {
	connectionID := strings.TrimSpace(r.PathValue("connectionID"))
	if connectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connection id is required"})
		return
	}

	request := struct {
		Command string `json:"command"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json payload"})
		return
	}

	result, err := s.service.ExecuteCommandOnConnection(connectionID, request.Command, 60*time.Second)
	response := map[string]any{
		"output":   result.Output,
		"timedOut": result.TimedOut,
	}
	if err != nil {
		response["error"] = err.Error()
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleListConnectionFilesystem(w http.ResponseWriter, r *http.Request) {
	connectionID := strings.TrimSpace(r.PathValue("connectionID"))
	if connectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connection id is required"})
		return
	}

	listing, err := s.service.ListFilesOnConnection(connectionID, r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, filesystemStatusCode(err), map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, listing)
}

func (s *Server) handleDownloadConnectionFile(w http.ResponseWriter, r *http.Request) {
	connectionID := strings.TrimSpace(r.PathValue("connectionID"))
	if connectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connection id is required"})
		return
	}

	download, err := s.service.OpenFileOnConnection(connectionID, r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, filesystemStatusCode(err), map[string]any{"error": err.Error()})
		return
	}
	defer download.Close()
	if download.Size > rssh.MaxFileTransferBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": maxFileTransferError()})
		return
	}

	writeFileDownload(w, download.Name, download.Size, download)
}

func (s *Server) handlePreviewConnectionFile(w http.ResponseWriter, r *http.Request) {
	connectionID := strings.TrimSpace(r.PathValue("connectionID"))
	if connectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connection id is required"})
		return
	}

	preview, err := s.service.PreviewFileOnConnection(connectionID, r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, filesystemStatusCode(err), map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleUploadConnectionFile(w http.ResponseWriter, r *http.Request) {
	connectionID := strings.TrimSpace(r.PathValue("connectionID"))
	if connectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "connection id is required"})
		return
	}

	file, filename, err := uploadFilePart(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file is required"})
		return
	}
	defer file.Close()

	result, err := s.service.UploadFileOnConnection(connectionID, r.URL.Query().Get("directory"), filename, file)
	if err != nil {
		writeJSON(w, filesystemStatusCode(err), map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func writeFileDownload(w http.ResponseWriter, filename string, size int64, reader io.Reader) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func uploadFilePart(r *http.Request) (*multipart.Part, string, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, "", err
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
		if part.FormName() == "file" && strings.TrimSpace(part.FileName()) != "" {
			return part, part.FileName(), nil
		}
		_ = part.Close()
	}

	return nil, "", fmt.Errorf("file is required")
}

func maxFileTransferError() string {
	return fmt.Sprintf("file exceeds maximum transfer size of %d MiB", rssh.MaxFileTransferBytes/(1024*1024))
}

func filesystemStatusCode(err error) int {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "not currently connected"):
		return http.StatusConflict
	case strings.Contains(message, "exceeds maximum transfer size"):
		return http.StatusRequestEntityTooLarge
	case strings.Contains(message, "permission denied"):
		return http.StatusForbidden
	case strings.Contains(message, "no such file"), strings.Contains(message, "not exist"):
		return http.StatusNotFound
	case strings.Contains(message, "required"), strings.Contains(message, "invalid filename"), strings.Contains(message, "is a directory"):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}
