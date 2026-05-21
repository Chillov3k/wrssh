package api

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
)

type hostFileTarget struct {
	host         store.HostRecord
	projectName  string
	connectionID string
	useRuntime   bool
}

func (s *Server) handleHostFilesystem(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveHostFileTarget(w, r)
	if !ok {
		return
	}

	remotePath := r.URL.Query().Get("path")
	var (
		listing rssh.FileList
		err     error
	)
	if target.useRuntime {
		listing, err = s.runtimes.ListFiles(r.Context(), target.projectName, target.connectionID, remotePath)
	} else {
		listing, err = s.rsshService.ListFilesOnConnection(target.connectionID, remotePath)
	}
	if err != nil {
		writeError(w, hostFilesystemStatusCode(err), err.Error())
		return
	}

	_ = s.store.TouchHostActivity(target.host.StableID, time.Now())
	writeJSON(w, http.StatusOK, listing)
}

func (s *Server) handleHostFilesystemDownload(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveHostFileTarget(w, r)
	if !ok {
		return
	}

	remotePath := r.URL.Query().Get("path")
	sessionUID := s.startFileSession(currentUser(r), target, "web-file-download", remotePath)
	var status = "completed"
	var errText string
	defer func() {
		_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
		_ = s.store.TouchHostActivity(target.host.StableID, time.Now())
	}()

	if target.useRuntime {
		download, err := s.runtimes.DownloadFile(r.Context(), target.projectName, target.connectionID, remotePath)
		if err != nil {
			status = "failed"
			errText = err.Error()
			writeError(w, hostFilesystemStatusCode(err), err.Error())
			return
		}
		defer download.Body.Close()
		if download.ContentLength > rssh.MaxFileTransferBytes {
			status = "failed"
			errText = maxFileTransferError()
			writeError(w, http.StatusRequestEntityTooLarge, errText)
			return
		}
		streamDownload(w, download.Filename, download.ContentType, download.ContentLength, download.Body)
		return
	}

	download, err := s.rsshService.OpenFileOnConnection(target.connectionID, remotePath)
	if err != nil {
		status = "failed"
		errText = err.Error()
		writeError(w, hostFilesystemStatusCode(err), err.Error())
		return
	}
	defer download.Close()
	if download.Size > rssh.MaxFileTransferBytes {
		status = "failed"
		errText = maxFileTransferError()
		writeError(w, http.StatusRequestEntityTooLarge, errText)
		return
	}

	streamDownload(w, download.Name, "application/octet-stream", download.Size, download)
}

func (s *Server) handleHostFilesystemPreview(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveHostFileTarget(w, r)
	if !ok {
		return
	}

	remotePath := r.URL.Query().Get("path")
	sessionUID := s.startFileSession(currentUser(r), target, "web-file-preview", remotePath)
	var status = "completed"
	var errText string
	defer func() {
		_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
		_ = s.store.TouchHostActivity(target.host.StableID, time.Now())
	}()

	var (
		preview rssh.FilePreview
		err     error
	)
	if target.useRuntime {
		preview, err = s.runtimes.PreviewFile(r.Context(), target.projectName, target.connectionID, remotePath)
	} else {
		preview, err = s.rsshService.PreviewFileOnConnection(target.connectionID, remotePath)
	}
	if err != nil {
		status = "failed"
		errText = err.Error()
		writeError(w, hostFilesystemStatusCode(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleHostFilesystemUpload(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveHostFileTarget(w, r)
	if !ok {
		return
	}

	file, filename, err := uploadFilePart(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	directory := r.URL.Query().Get("directory")
	sessionUID := s.startFileSession(currentUser(r), target, "web-file-upload", rssh.JoinRemotePath(directory, filename))
	var status = "completed"
	var errText string
	defer func() {
		_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
		_ = s.store.TouchHostActivity(target.host.StableID, time.Now())
	}()

	var result rssh.FileUploadResult
	if target.useRuntime {
		result, err = s.runtimes.UploadFile(r.Context(), target.projectName, target.connectionID, directory, filename, file)
	} else {
		result, err = s.rsshService.UploadFileOnConnection(target.connectionID, directory, filename, file)
	}
	if err != nil {
		status = "failed"
		errText = err.Error()
		writeError(w, hostFilesystemStatusCode(err), err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func maxFileTransferError() string {
	return fmt.Sprintf("file exceeds maximum transfer size of %d MiB", rssh.MaxFileTransferBytes/(1024*1024))
}

func (s *Server) resolveHostFileTarget(w http.ResponseWriter, r *http.Request) (hostFileTarget, bool) {
	user := currentUser(r)
	projectName := strings.TrimSpace(requestedProject(r))
	if projectName == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return hostFileTarget{}, false
	}
	if !requireProjectAccess(w, user, projectName) {
		return hostFileTarget{}, false
	}

	host, ok := s.authorizeHostForUser(user, r.PathValue("stableID"))
	if !ok || !store.ProjectMatches(host.Project, projectName) {
		writeError(w, http.StatusNotFound, "host not found")
		return hostFileTarget{}, false
	}

	useRuntime, err := s.projectUsesRemoteRuntime(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return hostFileTarget{}, false
	}

	var remoteConnections map[string][]hostConnectionResponse
	if useRuntime {
		remoteConnections, err = s.syncProjectRuntimeHosts(r.Context(), projectName)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return hostFileTarget{}, false
		}
	}

	connectionID, err := s.resolveConnectionIDForHost(host, r.URL.Query().Get("connectionId"), remoteConnections)
	if err != nil {
		writeError(w, hostFilesystemStatusCode(err), err.Error())
		return hostFileTarget{}, false
	}

	return hostFileTarget{
		host:         host,
		projectName:  projectName,
		connectionID: connectionID,
		useRuntime:   useRuntime,
	}, true
}

func (s *Server) startFileSession(user store.WebUser, target hostFileTarget, sessionType, remotePath string) string {
	sessionUID := fmt.Sprintf("%s-%d-%s", sessionType, time.Now().UnixNano(), target.connectionID)
	_, _ = s.store.StartSession(store.SessionRecord{
		SessionUID:       sessionUID,
		Type:             sessionType,
		Status:           "active",
		Source:           "web",
		Username:         user.Username,
		Role:             user.Role,
		HostStableID:     target.host.StableID,
		HostConnectionID: target.connectionID,
		Hostname:         target.host.Hostname,
		RemoteAddr:       target.host.RemoteAddr,
		Command:          remotePath,
		StartedAt:        time.Now(),
	})
	return sessionUID
}

func streamDownload(w http.ResponseWriter, filename, contentType string, size int64, reader io.Reader) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "download"
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
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

func hostFilesystemStatusCode(err error) int {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "not currently connected"), strings.Contains(message, "multiple active connections"):
		return http.StatusConflict
	case strings.Contains(message, "exceeds maximum transfer size"):
		return http.StatusRequestEntityTooLarge
	case strings.Contains(message, "permission denied"):
		return http.StatusForbidden
	case strings.Contains(message, "no such file"), strings.Contains(message, "not exist"), strings.Contains(message, "not found"):
		return http.StatusNotFound
	case strings.Contains(message, "required"), strings.Contains(message, "invalid filename"), strings.Contains(message, "is a directory"):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}
