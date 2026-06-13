package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
)

type runHostModuleRequest struct {
	ConnectionID     string   `json:"connectionId"`
	Args             []string `json:"args"`
	Stdin            string   `json:"stdin"`
	TimeoutSeconds   int      `json:"timeoutSeconds"`
	OutputLimitBytes int64    `json:"outputLimitBytes"`
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
	request := runHostModuleRequest{}
	if err := decodeJSON(r, &request); err != nil {
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
		result, err = s.runtimes.RunModule(r.Context(), target.projectName, target.connectionID, module, request.Args, request.Stdin, opts)
	} else {
		var stdinReader io.Reader
		if request.Stdin != "" {
			stdinReader = strings.NewReader(request.Stdin)
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
