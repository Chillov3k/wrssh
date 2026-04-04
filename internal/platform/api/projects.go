package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"gorm.io/gorm"
)

type projectResponse struct {
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	Tags              []string   `json:"tags"`
	System            bool       `json:"system"`
	RuntimeStatus     string     `json:"runtimeStatus,omitempty"`
	RuntimeLastError  string     `json:"runtimeLastError,omitempty"`
	HostsCount        int        `json:"hostsCount"`
	OfflineHostsCount int        `json:"offlineHostsCount"`
	ActiveConnections int        `json:"activeConnections"`
	ArtifactsCount    int        `json:"artifactsCount"`
	LastActivityAt    *time.Time `json:"lastActivityAt"`
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projectCatalogForUser(r.Context(), currentUser(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": projects,
	})
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	projectName := strings.TrimSpace(r.PathValue("projectName"))
	if projectName == "" {
		writeError(w, http.StatusBadRequest, "project name is required")
		return
	}

	projects, err := s.projectCatalogForUser(r.Context(), currentUser(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, project := range projects {
		if project.Name == projectName {
			writeJSON(w, http.StatusOK, project)
			return
		}
	}

	writeError(w, http.StatusNotFound, "project not found")
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !canCreateProjects(user) {
		writeError(w, http.StatusForbidden, "project creation is not allowed")
		return
	}

	request := struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	project, err := s.store.CreateProject(request.Name, request.Description, request.Tags)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrProjectExists):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	if !isAdminUser(user) {
		if err := s.store.GrantUserProjectAccess(user.Username, project.Name); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	runtimeStatus := ""
	if s.runtimes != nil {
		runtimeStatus = store.RuntimeStatusProvisioning
		go s.provisionProjectAsync(project.Name)
	}

	writeJSON(w, http.StatusCreated, projectResponse{
		Name:          project.Name,
		Description:   project.Description,
		Tags:          store.DecodeTags(project.Tags),
		RuntimeStatus: runtimeStatus,
	})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := strings.TrimSpace(r.PathValue("projectName"))
	if projectName == "" {
		writeError(w, http.StatusBadRequest, "project name is required")
		return
	}
	if !canAccessProject(user, projectName) {
		writeError(w, http.StatusForbidden, "project access denied")
		return
	}

	request := struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	updated, err := s.store.UpdateProject(projectName, request.Name, request.Description, request.Tags)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrProjectExists):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	projects, err := s.projectCatalogForUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, project := range projects {
		if project.Name == updated.Name {
			writeJSON(w, http.StatusOK, project)
			return
		}
	}

	writeError(w, http.StatusNotFound, "project not found")
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := strings.TrimSpace(r.PathValue("projectName"))
	if projectName == "" {
		writeError(w, http.StatusBadRequest, "project name is required")
		return
	}
	if !canAccessProject(user, projectName) {
		writeError(w, http.StatusForbidden, "project access denied")
		return
	}

	if _, err := s.store.GetProject(projectName); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	if s.runtimes != nil {
		if _, err := s.store.GetProjectRuntime(projectName); err == nil {
			hosts, hostsErr := s.store.ListHostsForProject(projectName)
			artifactPaths, artifactsErr := s.store.ListArtifactPathsForProject(projectName)
			killedConnections := 0
			deletedArtifacts := 0
			if snapshots, runtimeErr := s.runtimes.ListClientSnapshots(r.Context(), projectName); runtimeErr == nil {
				killedConnections = len(snapshots)
			}
			if artifacts, runtimeErr := s.runtimes.ListArtifacts(r.Context(), projectName); runtimeErr == nil {
				deletedArtifacts = len(artifacts)
			} else if artifactsErr == nil {
				deletedArtifacts = len(artifactPaths)
			}
			deletedHosts := 0
			if hostsErr == nil {
				deletedHosts = len(hosts)
			}

			if err := s.runtimes.DeprovisionProject(r.Context(), projectName); err != nil {
				writeError(w, http.StatusBadGateway, err.Error())
				return
			}
			if err := s.store.DeleteProjectCascade(projectName); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"deleted":            true,
				"project":            projectName,
				"deletedHosts":       deletedHosts,
				"deletedArtifacts":   deletedArtifacts,
				"killedConnections":  killedConnections,
				"deletedProjectName": projectName,
				"runtimeDeleted":     true,
			})
			return
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	hosts, err := s.store.ListHostsForProject(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	killedConnections := 0
	for _, host := range hosts {
		for _, connection := range s.activeConnectionsForHost(host.StableID) {
			if err := s.rsshService.KillConnection(connection.ConnectionID); err != nil {
				writeError(w, http.StatusBadGateway, err.Error())
				return
			}
			killedConnections++
		}
	}

	artifactPaths, err := s.store.ListArtifactPathsForProject(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	artifacts, err := s.rsshService.ListArtifacts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	existingArtifacts := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		existingArtifacts[artifact.URLPath] = struct{}{}
	}

	deletedArtifacts := 0
	for _, urlPath := range artifactPaths {
		if _, ok := existingArtifacts[urlPath]; !ok {
			continue
		}
		if err := s.rsshService.DeleteArtifact(urlPath); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		deletedArtifacts++
	}

	if err := s.store.DeleteProjectCascade(projectName); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":             true,
		"project":             projectName,
		"deletedHosts":        len(hosts),
		"deletedArtifacts":    deletedArtifacts,
		"killedConnections":   killedConnections,
		"deletedProjectName":  projectName,
		"deletedArtifactRefs": len(artifactPaths),
	})
}

func (s *Server) projectCatalogForUser(ctx context.Context, user store.WebUser) ([]projectResponse, error) {
	records, err := s.store.ListProjects()
	if err != nil {
		return nil, err
	}

	remoteProjects := make(map[string]bool, len(records))
	remoteArtifactCounts := make(map[string]int, len(records))
	remoteConnections := make(map[string]map[string][]hostConnectionResponse, len(records))
	for _, record := range records {
		if !canAccessProject(user, record.Name) {
			continue
		}

		useRuntime, err := s.projectUsesRemoteRuntime(record.Name)
		if err != nil {
			return nil, err
		}
		if !useRuntime {
			continue
		}

		remoteProjects[record.Name] = true
		connections, err := s.syncProjectRuntimeHosts(ctx, record.Name)
		if err != nil {
			log.Printf("[web-api] project runtime sync failed for %s: %v", record.Name, err)
			continue
		}
		remoteConnections[record.Name] = connections
		artifacts, err := s.artifactsForProject(ctx, record.Name)
		if err != nil {
			log.Printf("[web-api] project artifact sync failed for %s: %v", record.Name, err)
			continue
		}
		remoteArtifactCounts[record.Name] = len(artifacts)
	}

	hosts, err := s.store.ListHosts()
	if err != nil {
		return nil, err
	}
	filteredHosts := filterHostsForWebUser(hosts, user)

	artifacts, err := s.rsshService.ListArtifacts()
	if err != nil {
		return nil, err
	}

	artifactProjects, err := s.store.ListArtifactProjectAssignments()
	if err != nil {
		return nil, err
	}

	catalog := make(map[string]*projectResponse, len(records)+1)
	for _, record := range records {
		project := &projectResponse{
			Name:           record.Name,
			Description:    record.Description,
			Tags:           store.DecodeTags(record.Tags),
			ArtifactsCount: remoteArtifactCounts[record.Name],
		}
		if s.runtimes != nil {
			if runtime, runtimeErr := s.store.GetProjectRuntime(record.Name); runtimeErr == nil {
				project.RuntimeStatus = runtime.Status
				project.RuntimeLastError = runtime.LastError
			} else if !errors.Is(runtimeErr, gorm.ErrRecordNotFound) {
				return nil, runtimeErr
			}
		}
		catalog[record.Name] = project
	}

	for _, host := range filteredHosts {
		project := ensureProjectCatalogEntry(catalog, store.DisplayProjectName(host.Project))
		activeConnections := len(s.activeConnectionsForProjectHost(host.Project, host.StableID, remoteConnections[store.DisplayProjectName(host.Project)]))
		project.HostsCount++
		if activeConnections > 0 {
			project.ActiveConnections += activeConnections
		} else {
			project.OfflineHostsCount++
		}
		project.LastActivityAt = laterTime(project.LastActivityAt, host.LastActivityAt)
	}

	for _, artifact := range artifacts {
		projectName := store.DisplayProjectName(artifactProjects[artifact.URLPath])
		if remoteProjects[projectName] {
			continue
		}
		project := ensureProjectCatalogEntry(catalog, projectName)
		project.ArtifactsCount++
	}

	items := make([]projectResponse, 0, len(catalog))
	for _, project := range catalog {
		if !canAccessProject(user, project.Name) {
			continue
		}
		items = append(items, *project)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	return items, nil
}

func (s *Server) provisionProjectAsync(projectName string) {
	if s == nil || s.runtimes == nil || strings.TrimSpace(projectName) == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if _, err := s.runtimes.ProvisionProject(ctx, projectName); err != nil {
		log.Printf("[web-api] async project runtime provisioning failed for %s: %v", projectName, err)
	}
}

func requestedProject(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("project"))
}

func filterHostsByProject(hosts []store.HostRecord, selectedProject string) []store.HostRecord {
	if strings.TrimSpace(selectedProject) == "" {
		return hosts
	}

	filtered := make([]store.HostRecord, 0, len(hosts))
	for _, host := range hosts {
		if store.ProjectMatches(host.Project, selectedProject) {
			filtered = append(filtered, host)
		}
	}
	return filtered
}

func filterArtifactsByProject(artifacts []rssh.Artifact, selectedProject string, assignments map[string]string) []rssh.Artifact {
	if strings.TrimSpace(selectedProject) == "" {
		return artifacts
	}

	filtered := make([]rssh.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifactBelongsToProject(artifact.URLPath, selectedProject, assignments) {
			filtered = append(filtered, artifact)
		}
	}
	return filtered
}

func artifactBelongsToProject(urlPath, selectedProject string, assignments map[string]string) bool {
	return store.ProjectMatches(assignments[urlPath], selectedProject)
}

func ensureProjectCatalogEntry(catalog map[string]*projectResponse, projectName string) *projectResponse {
	projectName = store.DisplayProjectName(projectName)
	if existing, ok := catalog[projectName]; ok {
		return existing
	}

	project := &projectResponse{
		Name: projectName,
	}
	catalog[projectName] = project
	return project
}

func laterTime(left, right *time.Time) *time.Time {
	switch {
	case left == nil:
		return right
	case right == nil:
		return left
	case right.After(*left):
		return right
	default:
		return left
	}
}
