package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal"
	"github.com/NHAS/reverse_ssh/internal/platform/auth"
	platformconfig "github.com/NHAS/reverse_ssh/internal/platform/config"
	"github.com/NHAS/reverse_ssh/internal/platform/orchestrator"
	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"github.com/NHAS/reverse_ssh/internal/platform/system"
	"github.com/NHAS/reverse_ssh/internal/server/commands"
	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/NHAS/reverse_ssh/internal/server/webserver"
)

type Server struct {
	cfg         platformconfig.Config
	store       *store.Store
	auth        *auth.Manager
	rsshService *rssh.Service
	runtimes    *orchestrator.Manager
	loginGuard  *loginThrottle
}

type contextKey string

const userContextKey contextKey = "webUser"

func NewServer(cfg platformconfig.Config, store *store.Store, authManager *auth.Manager, rsshService *rssh.Service, runtimeManager *orchestrator.Manager) *Server {
	return &Server{
		cfg:         cfg,
		store:       store,
		auth:        authManager,
		rsshService: rsshService,
		runtimes:    runtimeManager,
		loginGuard:  newLoginThrottle(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	mux.Handle("GET /api/auth/me", s.requireUser(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/profile", s.requireUser(http.HandlerFunc(s.handleProfile)))
	mux.Handle("PATCH /api/profile", s.requireUser(http.HandlerFunc(s.handleUpdateProfile)))
	mux.Handle("GET /api/users", s.requireUser(http.HandlerFunc(s.handleUsers)))
	mux.Handle("POST /api/users", s.requireUser(http.HandlerFunc(s.handleCreateUser)))
	mux.Handle("GET /api/users/{username}", s.requireUser(http.HandlerFunc(s.handleUser)))
	mux.Handle("PATCH /api/users/{username}", s.requireUser(http.HandlerFunc(s.handleUpdateUser)))
	mux.Handle("DELETE /api/users/{username}", s.requireUser(http.HandlerFunc(s.handleDeleteUser)))
	mux.Handle("GET /api/projects", s.requireUser(http.HandlerFunc(s.handleProjects)))
	mux.Handle("POST /api/projects", s.requireUser(http.HandlerFunc(s.handleCreateProject)))
	mux.Handle("GET /api/projects/{projectName}", s.requireUser(http.HandlerFunc(s.handleProject)))
	mux.Handle("PATCH /api/projects/{projectName}", s.requireUser(http.HandlerFunc(s.handleUpdateProject)))
	mux.Handle("DELETE /api/projects/{projectName}", s.requireUser(http.HandlerFunc(s.handleDeleteProject)))
	mux.Handle("GET /api/dashboard", s.requireUser(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /api/system/options", s.requireUser(http.HandlerFunc(s.handleSystemOptions)))
	mux.Handle("GET /api/hosts", s.requireUser(http.HandlerFunc(s.handleHosts)))
	mux.Handle("GET /api/hosts/{stableID}", s.requireUser(http.HandlerFunc(s.handleHost)))
	mux.Handle("POST /api/hosts/exec", s.requireUser(http.HandlerFunc(s.handleExecuteHosts)))
	mux.Handle("PATCH /api/hosts/{stableID}", s.requireUser(http.HandlerFunc(s.handleUpdateHost)))
	mux.Handle("DELETE /api/hosts/{stableID}", s.requireUser(http.HandlerFunc(s.handleDeleteHost)))
	mux.Handle("GET /api/artifacts/{urlPath}", s.requireUser(http.HandlerFunc(s.handleArtifact)))
	mux.Handle("GET /api/artifacts", s.requireUser(http.HandlerFunc(s.handleArtifacts)))
	mux.Handle("POST /api/artifacts", s.requireUser(http.HandlerFunc(s.handleCreateArtifact)))
	mux.Handle("DELETE /api/artifacts/{urlPath}", s.requireUser(http.HandlerFunc(s.handleDeleteArtifact)))
	mux.Handle("GET /ws/terminal/{stableID}", s.websocketHandler())

	return s.loggingMiddleware(mux)
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[web-api] %s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := s.auth.ParseSessionCookie(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		user, err := s.store.GetUserByID(session.UserID)
		if err != nil || !user.Enabled {
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		if session.Version != user.SessionVersion {
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		if user.MustChangePassword && r.URL.Path != "/api/auth/me" && r.URL.Path != "/api/profile" {
			writeError(w, http.StatusForbidden, "password change required")
			return
		}

		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	credentials := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}
	if err := decodeJSON(r, &credentials); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	loginKey := loginThrottleKey(r, credentials.Username)
	now := time.Now()
	if retryAfter, blocked := s.loginGuard.retryAfter(loginKey, now); blocked {
		writeRetryAfter(w, retryAfter)
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	user, err := s.store.Authenticate(credentials.Username, credentials.Password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCredentials) {
			if retryAfter := s.loginGuard.registerFailure(loginKey, now); retryAfter > 0 {
				writeRetryAfter(w, retryAfter)
				writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.loginGuard.reset(loginKey)

	if err := s.auth.SetSessionCookie(w, user.ID, user.SessionVersion, 12*time.Hour, requestIsSecure(r)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, userResponse(user))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if session, err := s.auth.ParseSessionCookie(r); err == nil {
		_, _ = s.store.RotateUserSessionVersionByID(session.UserID)
	}
	s.auth.ClearSessionCookie(w, requestIsSecure(r))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userResponse(currentUser(r)))
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	selectedProject := requestedProject(r)
	if !requireProjectAccess(w, user, selectedProject) {
		return
	}

	var remoteConnections map[string][]hostConnectionResponse
	if strings.TrimSpace(selectedProject) != "" {
		var err error
		remoteConnections, err = s.syncProjectRuntimeHosts(r.Context(), selectedProject)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	hosts, err := s.store.ListHosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filteredHosts := filterHostsByProject(filterHostsForWebUser(hosts, user), selectedProject)

	summary, err := s.store.Summary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	artifacts, err := s.artifactsForProject(r.Context(), selectedProject)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	artifactProjects, err := s.store.ListArtifactProjectAssignments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	artifacts = filterArtifactsByProject(artifacts, selectedProject, artifactProjects)

	projects := make(map[string]struct{}, len(filteredHosts))
	var connected int64
	var offline int64
	var activeSessions int64
	var tagged int64

	for _, host := range filteredHosts {
		activeCount := len(s.activeConnectionsForProjectHost(selectedProject, host.StableID, remoteConnections))
		if activeCount > 0 {
			connected++
			activeSessions += int64(activeCount)
		} else {
			offline++
		}

		if strings.TrimSpace(host.Project) != "" {
			projects[host.Project] = struct{}{}
		}
		if len(store.DecodeTags(host.Tags)) > 0 {
			tagged++
		}
	}

	summary.ConnectedHosts = connected
	summary.OfflineHosts = offline
	summary.ActiveSessions = activeSessions
	summary.AvailableBuilds = int64(len(artifacts))
	summary.ProjectsCount = int64(len(projects))
	summary.TaggedHostsCount = tagged
	if selectedProject != "" || user.Role != "admin" {
		hostStableIDs := make([]string, 0, len(filteredHosts))
		for _, host := range filteredHosts {
			hostStableIDs = append(hostStableIDs, host.StableID)
		}
		completedToday, err := s.store.CountCompletedSessionsForHostsSince(hostStableIDs, time.Now().Truncate(24*time.Hour))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		summary.CompletedToday = completedToday
		summary.ConfiguredHooks = 0
	}

	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleSystemOptions(w http.ResponseWriter, r *http.Request) {
	projectName := requestedProject(r)
	if projectName != "" && !requireProjectAccess(w, currentUser(r), projectName) {
		return
	}
	options := system.DiscoverBuildOptions(s.jumpAddressForProject(r.Context(), projectName), s.cfg.RSSHListenAddr, s.cfg.AdvertisedAddrs)
	options.BuildFlagHelp = commands.BuildOptionHelp()
	writeJSON(w, http.StatusOK, options)
}

func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	selectedProject := requestedProject(r)
	if !requireProjectAccess(w, user, selectedProject) {
		return
	}

	remoteConnections, err := s.syncProjectRuntimeHosts(r.Context(), selectedProject)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	hosts, err := s.store.ListHosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filtered := filterHostsByProject(filterHostsForWebUser(hosts, user), selectedProject)
	response := make([]hostResponse, 0, len(filtered))
	jumpAddress := s.jumpAddressForProject(r.Context(), selectedProject)
	for _, host := range filtered {
		response = append(response, s.makeHostResponse(currentUser(r), host, s.activeConnectionsForProjectHost(selectedProject, host.StableID, remoteConnections), jumpAddress))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": response,
	})
}

func (s *Server) handleHost(w http.ResponseWriter, r *http.Request) {
	selectedProject := requestedProject(r)
	remoteConnections, err := s.syncProjectRuntimeHosts(r.Context(), selectedProject)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	host, ok := s.authorizedHost(r)
	if !ok || !store.ProjectMatches(host.Project, selectedProject) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}

	writeJSON(w, http.StatusOK, s.makeHostResponse(currentUser(r), host, s.activeConnectionsForProjectHost(selectedProject, host.StableID, remoteConnections), s.jumpAddressForProject(r.Context(), host.Project)))
}

func (s *Server) handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	host, ok := s.authorizedHost(r)
	if !ok || !store.ProjectMatches(host.Project, requestedProject(r)) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}

	request := struct {
		Project  *string   `json:"project"`
		Tags     *[]string `json:"tags"`
		Hostname *string   `json:"hostname"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if request.Project != nil && !canAccessProject(user, *request.Project) {
		writeError(w, http.StatusForbidden, "project access denied")
		return
	}
	if request.Project != nil {
		targetProject := store.DisplayProjectName(*request.Project)
		currentProject := store.DisplayProjectName(host.Project)
		if targetProject != currentProject {
			currentRuntime, err := s.projectUsesRemoteRuntime(currentProject)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			targetRuntime, err := s.projectUsesRemoteRuntime(targetProject)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if currentRuntime || targetRuntime {
				writeError(w, http.StatusBadRequest, "moving hosts across isolated project runtimes is not supported yet")
				return
			}
		}
	}

	updated, err := s.store.PatchHostMetadata(host.StableID, store.HostMetadataPatch{
		Project:     request.Project,
		Tags:        request.Tags,
		DisplayName: request.Hostname,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	remoteConnections, err := s.syncProjectRuntimeHosts(r.Context(), updated.Project)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, s.makeHostResponse(currentUser(r), updated, s.activeConnectionsForProjectHost(updated.Project, updated.StableID, remoteConnections), s.jumpAddressForProject(r.Context(), updated.Project)))
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	host, ok := s.authorizedHost(r)
	if !ok || !store.ProjectMatches(host.Project, requestedProject(r)) {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}

	remoteConnections, err := s.syncProjectRuntimeHosts(r.Context(), host.Project)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	activeConnections := s.activeConnectionsForProjectHost(host.Project, host.StableID, remoteConnections)
	useRuntime, err := s.projectUsesRemoteRuntime(host.Project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, connection := range activeConnections {
		if useRuntime {
			err = s.runtimes.KillConnection(r.Context(), host.Project, connection.ConnectionID)
		} else {
			err = s.rsshService.KillConnection(connection.ConnectionID)
		}
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	if err := s.store.DeleteHost(host.StableID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":           true,
		"stableId":          host.StableID,
		"killedConnections": len(activeConnections),
	})
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := requestedProject(r)
	if !requireProjectAccess(w, user, projectName) {
		return
	}

	artifacts, err := s.artifactsForProject(r.Context(), projectName)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	assignments, err := s.store.ListArtifactProjectAssignments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	artifacts = filterArtifactsForWebUser(artifacts, user, assignments)

	artifacts = filterArtifactsByProject(artifacts, projectName, assignments)

	writeJSON(w, http.StatusOK, map[string]any{
		"items": artifacts,
	})
}

func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !requireProjectAccess(w, user, requestedProject(r)) {
		return
	}

	urlPath := path.Clean(strings.TrimPrefix(r.PathValue("urlPath"), "/"))
	if urlPath == "." || urlPath == "" {
		writeError(w, http.StatusBadRequest, "artifact path is required")
		return
	}
	assignments, err := s.store.ListArtifactProjectAssignments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessArtifact(user, urlPath, assignments) {
		writeError(w, http.StatusForbidden, "project access denied")
		return
	}
	if !artifactBelongsToProject(urlPath, requestedProject(r), assignments) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}

	useRuntime, err := s.projectUsesRemoteRuntime(requestedProject(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var artifact rssh.Artifact
	if useRuntime {
		artifact, err = s.runtimes.GetArtifact(r.Context(), requestedProject(r), urlPath)
	} else {
		artifact, err = s.rsshService.GetArtifact(urlPath)
	}
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, artifact)
}

func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	request := struct {
		Name             string `json:"name"`
		Comment          string `json:"comment"`
		Owners           string `json:"owners"`
		Project          string `json:"project"`
		GOOS             string `json:"goos"`
		GOARCH           string `json:"goarch"`
		GOARM            string `json:"goarm"`
		ConnectBackHost  string `json:"connectBackHost"`
		ConnectBackPort  string `json:"connectBackPort"`
		Transport        string `json:"transport"`
		Proxy            string `json:"proxy"`
		SNI              string `json:"sni"`
		LogLevel         string `json:"logLevel"`
		WorkingDirectory string `json:"workingDirectory"`
		SharedObject     bool   `json:"sharedObject"`
		Garble           bool   `json:"garble"`
		UPX              bool   `json:"upx"`
		LZMA             bool   `json:"lzma"`
		DisableLibC      bool   `json:"disableLibC"`
		UseHostHeader    bool   `json:"useHostHeader"`
		RawDownload      bool   `json:"rawDownload"`
		UseKerberos      bool   `json:"useKerberos"`
		VersionString    string `json:"versionString"`
		NTLMProxyCreds   string `json:"ntlmProxyCreds"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	projectName := strings.TrimSpace(request.Project)
	if projectName == "" {
		projectName = requestedProject(r)
	}
	if projectName == "" {
		projectName = store.UnassignedProjectName
	}
	if !requireProjectAccess(w, user, projectName) {
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		generatedName, err := internal.RandomString(16)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		request.Name = generatedName
	}

	options := system.DiscoverBuildOptions(s.jumpAddressForProject(r.Context(), projectName), s.cfg.RSSHListenAddr, s.cfg.AdvertisedAddrs)
	if !contains(options.GOOS, strings.TrimSpace(request.GOOS)) {
		writeError(w, http.StatusBadRequest, "unsupported GOOS for this rssh build")
		return
	}
	if !contains(options.GOARCH, strings.TrimSpace(request.GOARCH)) {
		writeError(w, http.StatusBadRequest, "unsupported GOARCH for this rssh build")
		return
	}

	connectBackAddress := s.jumpAddressForProject(r.Context(), projectName)
	if host := strings.TrimSpace(request.ConnectBackHost); host != "" {
		port := strings.TrimSpace(request.ConnectBackPort)
		if port == "" {
			port = options.DefaultPort
		}
		connectBackAddress = net.JoinHostPort(host, port)
	}

	buildConfig := webserver.BuildConfig{
		Name:              strings.TrimSpace(request.Name),
		Comment:           strings.TrimSpace(request.Comment),
		Owners:            strings.TrimSpace(request.Owners),
		GOOS:              strings.TrimSpace(request.GOOS),
		GOARCH:            strings.TrimSpace(request.GOARCH),
		GOARM:             strings.TrimSpace(request.GOARM),
		ConnectBackAdress: applyTransport(connectBackAddress, request.Transport),
		Proxy:             strings.TrimSpace(request.Proxy),
		SNI:               strings.TrimSpace(request.SNI),
		LogLevel:          fallback(request.LogLevel, "INFO"),
		UseKerberosAuth:   request.UseKerberos,
		SharedLibrary:     request.SharedObject,
		UPX:               request.UPX,
		Lzma:              request.LZMA,
		Garble:            request.Garble,
		DisableLibC:       request.DisableLibC,
		RawDownload:       request.RawDownload,
		UseHostHeader:     request.UseHostHeader,
		WorkingDirectory:  strings.TrimSpace(request.WorkingDirectory),
		NTLMProxyCreds:    strings.TrimSpace(request.NTLMProxyCreds),
		VersionString:     strings.TrimSpace(request.VersionString),
	}

	useRuntime, err := s.projectUsesRemoteRuntime(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var buildResult rssh.BuildResult
	if useRuntime {
		buildResult, err = s.runtimes.CreateBuild(r.Context(), projectName, buildConfig)
	} else {
		buildResult, err = s.rsshService.CreateBuild(buildConfig)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if err := s.store.AssignArtifactProject(buildConfig.Name, projectName); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if buildResult.ClientStableID != "" {
		if err := s.store.AssignHostProjectHint(buildResult.ClientStableID, projectName); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"result":          buildResult.Result,
		"downloadUrl":     buildResult.Result,
		"callbackAddress": buildConfig.ConnectBackAdress,
		"project":         store.DisplayProjectName(projectName),
		"clientStableId":  buildResult.ClientStableID,
	})
}

func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !requireProjectAccess(w, user, requestedProject(r)) {
		return
	}

	urlPath := path.Clean(strings.TrimPrefix(r.PathValue("urlPath"), "/"))
	if urlPath == "." || urlPath == "" {
		writeError(w, http.StatusBadRequest, "artifact path is required")
		return
	}
	assignments, err := s.store.ListArtifactProjectAssignments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !canAccessArtifact(user, urlPath, assignments) {
		writeError(w, http.StatusForbidden, "project access denied")
		return
	}
	if !artifactBelongsToProject(urlPath, requestedProject(r), assignments) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}

	useRuntime, err := s.projectUsesRemoteRuntime(requestedProject(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if useRuntime {
		err = s.runtimes.DeleteArtifact(r.Context(), requestedProject(r), urlPath)
	} else {
		err = s.rsshService.DeleteArtifact(urlPath)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.store.DeleteArtifactProject(urlPath); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) authorizedHost(r *http.Request) (store.HostRecord, bool) {
	return s.authorizeHostForUser(currentUser(r), r.PathValue("stableID"))
}

func (s *Server) makeHostResponse(user store.WebUser, host store.HostRecord, activeConnections []hostConnectionResponse, jumpAddress string) hostResponse {
	defaultTarget := preferredConnectTarget(host, activeConnections)
	alias := preferredAlias(host)
	displayName := hostDisplayName(host)
	jumpTarget := jumpAddressForUser(user, jumpAddress)
	return hostResponse{
		StableID:               host.StableID,
		HostID:                 fallback(defaultTarget, fallback(host.ActiveConnectionID, host.LastConnectionID)),
		ActiveConnectionID:     host.ActiveConnectionID,
		Hostname:               displayName,
		ObservedHostname:       host.Hostname,
		DisplayName:            host.DisplayName,
		IP:                     host.RemoteIP,
		RemoteAddr:             host.RemoteAddr,
		Comment:                host.Comment,
		Version:                host.Version,
		Owners:                 decodeCSV(host.Owners),
		IsPublic:               host.IsPublic,
		Connected:              len(activeConnections) > 0,
		Project:                store.DisplayProjectName(host.Project),
		Tags:                   store.DecodeTags(host.Tags),
		DateAdded:              host.FirstSeenAt,
		LastActivityAt:         host.LastActivityAt,
		LastConnectionAt:       host.LastConnectedAt,
		LastDisconnectAt:       host.LastDisconnectedAt,
		PreferredAlias:         alias,
		ActiveConnections:      activeConnections,
		ActiveConnectionsCount: len(activeConnections),
		Commands: commandTemplates{
			SSH:           fmt.Sprintf("ssh -J %s %s", jumpTarget, defaultTarget),
			SCP:           fmt.Sprintf("scp -J %s %s:/path/to/file .", jumpTarget, defaultTarget),
			DynamicSocks:  fmt.Sprintf("ssh -D 9050 -J %s %s", jumpTarget, defaultTarget),
			RemoteForward: fmt.Sprintf("ssh -R 1234:localhost:1234 -J %s %s", jumpTarget, defaultTarget),
		},
	}
}

func jumpAddressForUser(user store.WebUser, jumpAddress string) string {
	address := strings.TrimSpace(jumpAddress)
	if address == "" || strings.Contains(address, "@") {
		return address
	}

	username := strings.TrimSpace(user.RSSHUsername)
	if username == "" {
		username = strings.TrimSpace(user.Username)
	}
	if username == "" {
		return address
	}

	return username + "@" + address
}

func (s *Server) activeConnectionsForHost(stableID string) []hostConnectionResponse {
	snapshots := make([]users.ClientSnapshot, 0, 1)
	if snapshot, ok := users.GetClientSnapshotByConnectionID(stableID); ok {
		snapshots = append(snapshots, snapshot)
	} else {
		snapshots = users.ListClientSnapshotsByFingerprint(stableID)
	}
	result := make([]hostConnectionResponse, 0, len(snapshots))
	for _, snapshot := range snapshots {
		result = append(result, hostConnectionResponse{
			ConnectionID: snapshot.ConnectionID,
			Hostname:     snapshot.Hostname,
			RemoteAddr:   snapshot.RemoteAddr,
			RemoteIP:     snapshot.RemoteIP,
			Version:      snapshot.Version,
			Comment:      snapshot.Comment,
		})
	}
	return result
}

func (s *Server) resolveConnectionIDForHost(host store.HostRecord, requested string, remote map[string][]hostConnectionResponse) (string, error) {
	activeConnections := s.activeConnectionsForProjectHost(host.Project, host.StableID, remote)
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, connection := range activeConnections {
			if connection.ConnectionID == requested {
				return requested, nil
			}
		}
		return "", fmt.Errorf("connection id %q is not active for this host", requested)
	}

	switch len(activeConnections) {
	case 0:
		return "", fmt.Errorf("host is not currently connected")
	case 1:
		return activeConnections[0].ConnectionID, nil
	default:
		return "", fmt.Errorf("multiple active connections detected; choose a specific connection id")
	}
}

func preferredConnectTarget(host store.HostRecord, activeConnections []hostConnectionResponse) string {
	if len(activeConnections) > 0 {
		return activeConnections[0].ConnectionID
	}
	return preferredAlias(host)
}

func (s *Server) sshJumpAddress() string {
	address := s.cfg.ExternalAddress
	if strings.Contains(address, "://") {
		if parsed, err := url.Parse(address); err == nil && parsed.Host != "" {
			return parsed.Host
		}
	}
	return address
}

func withUser(ctx context.Context, user store.WebUser) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func currentUser(r *http.Request) store.WebUser {
	if value := r.Context().Value(userContextKey); value != nil {
		if user, ok := value.(store.WebUser); ok {
			return user
		}
	}
	return store.WebUser{}
}

func requestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); strings.EqualFold(forwarded, "https") {
		return true
	}
	return false
}

type userResponsePayload struct {
	ID                 uint     `json:"id"`
	Username           string   `json:"username"`
	Role               string   `json:"role"`
	RSSHUsername       string   `json:"rsshUsername"`
	CanCreateProjects  bool     `json:"canCreateProjects"`
	AllowedProjects    []string `json:"allowedProjects"`
	MustChangePassword bool     `json:"mustChangePassword"`
}

func userResponse(user store.WebUser) userResponsePayload {
	allowedProjects := user.AllowedProjectList()
	if allowedProjects == nil {
		allowedProjects = []string{}
	}
	return userResponsePayload{
		ID:                 user.ID,
		Username:           user.Username,
		Role:               user.Role,
		RSSHUsername:       user.RSSHUsername,
		CanCreateProjects:  store.UserCanCreateProjects(user),
		AllowedProjects:    allowedProjects,
		MustChangePassword: user.MustChangePassword,
	}
}

type hostResponse struct {
	StableID               string                   `json:"stableId"`
	HostID                 string                   `json:"hostId"`
	ActiveConnectionID     string                   `json:"activeConnectionId"`
	Hostname               string                   `json:"hostname"`
	ObservedHostname       string                   `json:"observedHostname"`
	DisplayName            string                   `json:"displayName"`
	IP                     string                   `json:"ip"`
	RemoteAddr             string                   `json:"remoteAddr"`
	Comment                string                   `json:"comment"`
	Version                string                   `json:"version"`
	Owners                 []string                 `json:"owners"`
	IsPublic               bool                     `json:"isPublic"`
	Connected              bool                     `json:"connected"`
	Project                string                   `json:"project"`
	Tags                   []string                 `json:"tags"`
	DateAdded              *time.Time               `json:"dateAdded"`
	LastActivityAt         *time.Time               `json:"lastActivityAt"`
	LastConnectionAt       *time.Time               `json:"lastConnectionAt"`
	LastDisconnectAt       *time.Time               `json:"lastDisconnectAt"`
	PreferredAlias         string                   `json:"preferredAlias"`
	ActiveConnections      []hostConnectionResponse `json:"activeConnections"`
	ActiveConnectionsCount int                      `json:"activeConnectionsCount"`
	Commands               commandTemplates         `json:"commands"`
}

type hostConnectionResponse struct {
	ConnectionID string `json:"connectionId"`
	Hostname     string `json:"hostname"`
	RemoteAddr   string `json:"remoteAddr"`
	RemoteIP     string `json:"remoteIp"`
	Version      string `json:"version"`
	Comment      string `json:"comment"`
}

type commandTemplates struct {
	SSH           string `json:"ssh"`
	SCP           string `json:"scp"`
	DynamicSocks  string `json:"dynamicSocks"`
	RemoteForward string `json:"remoteForward"`
}

func preferredAlias(host store.HostRecord) string {
	switch {
	case strings.TrimSpace(host.DisplayName) != "":
		return strings.TrimSpace(host.DisplayName)
	case host.Comment != "":
		return host.Comment
	case host.Hostname != "":
		return host.Hostname
	case host.ActiveConnectionID != "":
		return host.ActiveConnectionID
	case host.LastConnectionID != "":
		return host.LastConnectionID
	default:
		return host.StableID
	}
}

func hostDisplayName(host store.HostRecord) string {
	if strings.TrimSpace(host.DisplayName) != "" {
		return strings.TrimSpace(host.DisplayName)
	}
	return strings.TrimSpace(host.Hostname)
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return strings.TrimSpace(value)
}

func decodeCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func applyTransport(address, transport string) string {
	address = strings.TrimSpace(address)
	switch strings.TrimSpace(transport) {
	case "", "ssh":
		return address
	case "tls", "wss", "ws", "stdio", "http", "https":
		return transport + "://" + address
	default:
		return address
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
