package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/store"
)

const (
	hostCommandTimeout     = 60 * time.Second
	hostCommandConcurrency = 8
)

type executeHostsRequest struct {
	Command string               `json:"command"`
	Targets []executeHostsTarget `json:"targets"`
}

type executeHostsTarget struct {
	StableID     string `json:"stableId"`
	ConnectionID string `json:"connectionId,omitempty"`
}

type executeHostsResult struct {
	Key          string `json:"key"`
	StableID     string `json:"stableId"`
	ConnectionID string `json:"connectionId,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	Output       string `json:"output,omitempty"`
	Error        string `json:"error,omitempty"`
	TimedOut     bool   `json:"timedOut,omitempty"`
}

func (s *Server) handleExecuteHosts(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := strings.TrimSpace(requestedProject(r))
	if projectName == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}
	if !requireProjectAccess(w, user, projectName) {
		return
	}

	request := executeHostsRequest{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	request.Command = strings.TrimSpace(request.Command)
	if request.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	if len(request.Targets) == 0 {
		writeError(w, http.StatusBadRequest, "at least one target is required")
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

	results := make([]executeHostsResult, len(request.Targets))
	sem := make(chan struct{}, hostCommandConcurrency)
	var wg sync.WaitGroup

	for index, target := range request.Targets {
		wg.Add(1)
		go func(i int, item executeHostsTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[i] = s.executeHostCommandTarget(r, user, projectName, useRuntime, remoteConnections, request.Command, item)
		}(index, target)
	}

	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"items": results})
}

func (s *Server) executeHostCommandTarget(r *http.Request, user store.WebUser, projectName string, useRuntime bool, remoteConnections map[string][]hostConnectionResponse, command string, target executeHostsTarget) executeHostsResult {
	stableID := strings.TrimSpace(target.StableID)
	connectionID := strings.TrimSpace(target.ConnectionID)

	result := executeHostsResult{
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
	if strings.TrimSpace(host.Project) != projectName {
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

	sessionUID := fmt.Sprintf("web-command-%d-%s", time.Now().UnixNano(), resolvedConnectionID)
	_, _ = s.store.StartSession(store.SessionRecord{
		SessionUID:       sessionUID,
		Type:             "web-command",
		Status:           "active",
		Source:           "web",
		Username:         user.Username,
		Role:             user.Role,
		HostStableID:     host.StableID,
		HostConnectionID: resolvedConnectionID,
		Hostname:         host.Hostname,
		RemoteAddr:       host.RemoteAddr,
		Command:          command,
		StartedAt:        time.Now(),
	})

	var status = "completed"
	var errText string

	defer func() {
		_ = s.store.FinishSession(sessionUID, status, errText, time.Now())
		_ = s.store.TouchHostActivity(host.StableID, time.Now())
	}()

	if useRuntime {
		execution, execErr := s.runtimes.ExecuteCommand(r.Context(), projectName, resolvedConnectionID, command)
		result.Output = execution.Output
		result.TimedOut = execution.TimedOut
		if execErr != nil {
			result.Error = execErr.Error()
			status = "failed"
			errText = result.Error
		}
		return result
	}

	execution, execErr := s.rsshService.ExecuteCommandOnConnection(resolvedConnectionID, command, hostCommandTimeout)
	result.Output = execution.Output
	result.TimedOut = execution.TimedOut
	if execErr != nil {
		result.Error = execErr.Error()
		status = "failed"
		errText = result.Error
	}

	return result
}

func hostCommandTargetKey(stableID, connectionID string) string {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		connectionID = "offline"
	}
	return strings.TrimSpace(stableID) + ":" + connectionID
}
