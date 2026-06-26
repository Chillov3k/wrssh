package api

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
)

type runHostModuleRequest struct {
	ConnectionID     string   `json:"connectionId"`
	Args             []string `json:"args"`
	Stdin            string   `json:"stdin"`
	StdinBase64      string   `json:"stdinBase64"`
	TimeoutSeconds   int      `json:"timeoutSeconds"`
	OutputLimitBytes int64    `json:"outputLimitBytes"`
}

const (
	maxModuleStdinBytes       = int64(8 * 1024 * 1024)
	maxModuleRequestBodyBytes = int64(12 * 1024 * 1024)
	hostModuleConcurrency     = 4
)

type runHostsModuleRequest struct {
	Targets          []executeHostsTarget `json:"targets"`
	Args             []string             `json:"args"`
	Stdin            string               `json:"stdin"`
	StdinBase64      string               `json:"stdinBase64"`
	TimeoutSeconds   int                  `json:"timeoutSeconds"`
	OutputLimitBytes int64                `json:"outputLimitBytes"`
}

type runHostsModuleResult struct {
	Key          string `json:"key"`
	StableID     string `json:"stableId"`
	ConnectionID string `json:"connectionId,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	Output       string `json:"output,omitempty"`
	Error        string `json:"error,omitempty"`
	TimedOut     bool   `json:"timedOut,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
}

func (s *Server) handleHostModules(w http.ResponseWriter, r *http.Request) {
	target, ok := s.resolveHostModuleTarget(w, r, r.URL.Query().Get("connectionId"))
	if !ok {
		return
	}

	sessionUID := s.startModuleSession(currentUser(r), target, "web-module-list", "list", []string{"--json"})
	status := "completed"
	errText := ""
	defer func() {
		_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
		_ = s.store.TouchHostActivity(target.host.StableID, time.Now())
	}()

	var (
		modules []rssh.ModuleManifest
		err     error
	)
	if target.useRuntime {
		modules, err = s.runtimes.ListModules(r.Context(), target.projectName, target.connectionID)
	} else {
		modules, err = s.rsshService.ListModulesOnConnection(r.Context(), target.connectionID)
	}
	if err != nil {
		status = "failed"
		errText = err.Error()
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": modules})
}

func (s *Server) handleRunHostModule(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxModuleRequestBodyBytes)
	request := runHostModuleRequest{}
	if err := decodeJSON(r, &request); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "module request body is too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	module := strings.TrimSpace(r.PathValue("module"))
	if module == "" {
		writeError(w, http.StatusBadRequest, "module is required")
		return
	}

	target, ok := s.resolveHostModuleTarget(w, r, request.ConnectionID)
	if !ok {
		return
	}
	stdin, stdinErr := decodeModuleStdin(request.Stdin, request.StdinBase64)
	if stdinErr != nil {
		writeError(w, moduleStdinStatusCode(stdinErr), stdinErr.Error())
		return
	}

	opts := rssh.SubsystemExecutionOptions{
		Timeout:          time.Duration(request.TimeoutSeconds) * time.Second,
		OutputLimitBytes: request.OutputLimitBytes,
	}

	sessionUID := s.startModuleSession(currentUser(r), target, "web-module", module, request.Args)
	status := "completed"
	errText := ""
	defer func() {
		_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
		_ = s.store.TouchHostActivity(target.host.StableID, time.Now())
	}()

	var (
		result rssh.SubsystemExecution
		err    error
	)
	if target.useRuntime {
		result, err = s.runtimes.RunModule(r.Context(), target.projectName, target.connectionID, module, request.Args, stdin, opts)
	} else {
		var stdinReader io.Reader
		if len(stdin) > 0 {
			stdinReader = bytes.NewReader(stdin)
		}
		result, err = s.rsshService.ExecuteSubsystemOnConnection(r.Context(), target.connectionID, module, request.Args, stdinReader, opts)
	}
	if err != nil {
		status = "failed"
		errText = err.Error()
	}

	response := map[string]any{
		"output":    result.Output,
		"timedOut":  result.TimedOut,
		"truncated": result.Truncated,
	}
	if err != nil {
		response["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleRunHostsModule(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := strings.TrimSpace(requestedProject(r))
	if projectName == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}
	if !requireProjectAccess(w, user, projectName) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxModuleRequestBodyBytes)
	request := runHostsModuleRequest{}
	if err := decodeJSON(r, &request); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "module request body is too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	module := strings.TrimSpace(r.PathValue("module"))
	if module == "" {
		writeError(w, http.StatusBadRequest, "module is required")
		return
	}
	if len(request.Targets) == 0 {
		writeError(w, http.StatusBadRequest, "at least one target is required")
		return
	}

	stdin, stdinErr := decodeModuleStdin(request.Stdin, request.StdinBase64)
	if stdinErr != nil {
		writeError(w, moduleStdinStatusCode(stdinErr), stdinErr.Error())
		return
	}

	useRuntime, err := s.projectUsesRemoteRuntime(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var remoteConnections map[string][]hostConnectionResponse
	if useRuntime {
		remoteConnections, err = s.syncProjectRuntimeHosts(r.Context(), projectName)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	opts := rssh.SubsystemExecutionOptions{
		Timeout:          time.Duration(request.TimeoutSeconds) * time.Second,
		OutputLimitBytes: request.OutputLimitBytes,
	}

	results := make([]runHostsModuleResult, len(request.Targets))
	sem := make(chan struct{}, hostModuleConcurrency)
	var wg sync.WaitGroup

	for index, target := range request.Targets {
		wg.Add(1)
		go func(i int, item executeHostsTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[i] = s.runHostModuleTarget(r, user, projectName, useRuntime, remoteConnections, module, request.Args, stdin, opts, item)
		}(index, target)
	}

	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"items": results})
}

func (s *Server) runHostModuleTarget(r *http.Request, user store.WebUser, projectName string, useRuntime bool, remoteConnections map[string][]hostConnectionResponse, module string, args []string, stdin []byte, opts rssh.SubsystemExecutionOptions, target executeHostsTarget) runHostsModuleResult {
	stableID := strings.TrimSpace(target.StableID)
	connectionID := strings.TrimSpace(target.ConnectionID)

	result := runHostsModuleResult{
		Key:          hostCommandTargetKey(stableID, connectionID),
		StableID:     stableID,
		ConnectionID: connectionID,
	}

	if stableID == "" {
		result.Error = "stable id is required"
		return result
	}

	host, ok := s.authorizeHostForUser(user, stableID)
	if !ok {
		result.Error = "host not found"
		return result
	}
	if !store.ProjectMatches(host.Project, projectName) {
		result.Error = "host is not part of the selected project"
		return result
	}

	result.Hostname = preferredAlias(host)

	resolvedConnectionID, err := s.resolveConnectionIDForHost(host, connectionID, remoteConnections)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.ConnectionID = resolvedConnectionID
	result.Key = hostCommandTargetKey(stableID, resolvedConnectionID)

	targetInfo := hostFileTarget{
		host:         host,
		projectName:  projectName,
		connectionID: resolvedConnectionID,
		useRuntime:   useRuntime,
	}
	sessionUID := s.startModuleSession(user, targetInfo, "web-module", module, args)
	status := "completed"
	errText := ""
	defer func() {
		_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
		_ = s.store.TouchHostActivity(host.StableID, time.Now())
	}()

	var execution rssh.SubsystemExecution
	if useRuntime {
		execution, err = s.runtimes.RunModule(r.Context(), projectName, resolvedConnectionID, module, args, stdin, opts)
	} else {
		var stdinReader io.Reader
		if len(stdin) > 0 {
			stdinReader = bytes.NewReader(stdin)
		}
		execution, err = s.rsshService.ExecuteSubsystemOnConnection(r.Context(), resolvedConnectionID, module, args, stdinReader, opts)
	}

	result.Output = execution.Output
	result.TimedOut = execution.TimedOut
	result.Truncated = execution.Truncated
	if err != nil {
		result.Error = err.Error()
		status = "failed"
		errText = result.Error
	}
	return result
}

func decodeModuleStdin(text, encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if text != "" && encoded != "" {
		return nil, fmt.Errorf("use either stdin or stdinBase64, not both")
	}

	var stdin []byte
	if encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid stdinBase64")
		}
		stdin = decoded
	} else if text != "" {
		stdin = []byte(text)
	}
	if int64(len(stdin)) > maxModuleStdinBytes {
		return nil, fmt.Errorf("module stdin exceeds maximum size of %d bytes", maxModuleStdinBytes)
	}
	return stdin, nil
}

func moduleStdinStatusCode(err error) int {
	if strings.Contains(err.Error(), "exceeds") {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func (s *Server) resolveHostModuleTarget(w http.ResponseWriter, r *http.Request, requestedConnectionID string) (hostFileTarget, bool) {
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

	if strings.TrimSpace(requestedConnectionID) == "" {
		requestedConnectionID = r.URL.Query().Get("connectionId")
	}
	connectionID, err := s.resolveConnectionIDForHost(host, requestedConnectionID, remoteConnections)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return hostFileTarget{}, false
	}

	return hostFileTarget{
		host:         host,
		projectName:  projectName,
		connectionID: connectionID,
		useRuntime:   useRuntime,
	}, true
}

func (s *Server) startModuleSession(user store.WebUser, target hostFileTarget, sessionType, module string, args []string) string {
	command := strings.TrimSpace(module + " " + strings.Join(args, " "))
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
		Command:          command,
		StartedAt:        time.Now(),
	})
	return sessionUID
}
