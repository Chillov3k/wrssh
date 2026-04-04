package api

import (
	"net/http"
	"strings"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
)

func isAdminUser(user store.WebUser) bool {
	return user.Role == "admin"
}

func canManageUsers(user store.WebUser) bool {
	return isAdminUser(user)
}

func canCreateProjects(user store.WebUser) bool {
	return store.UserCanCreateProjects(user)
}

func canAccessProject(user store.WebUser, project string) bool {
	return store.UserCanAccessProject(user, project)
}

func requireProjectAccess(w http.ResponseWriter, user store.WebUser, project string) bool {
	projectName := strings.TrimSpace(project)
	if projectName == "" {
		return true
	}
	if canAccessProject(user, projectName) {
		return true
	}
	writeError(w, http.StatusForbidden, "project access denied")
	return false
}

func filterHostsForWebUser(hosts []store.HostRecord, user store.WebUser) []store.HostRecord {
	if isAdminUser(user) {
		return hosts
	}

	filtered := make([]store.HostRecord, 0, len(hosts))
	for _, host := range hosts {
		if canAccessProject(user, host.Project) {
			filtered = append(filtered, host)
		}
	}
	return filtered
}

func canAccessArtifact(user store.WebUser, urlPath string, assignments map[string]string) bool {
	return canAccessProject(user, assignments[urlPath])
}

func filterArtifactsForWebUser(artifacts []rssh.Artifact, user store.WebUser, assignments map[string]string) []rssh.Artifact {
	if isAdminUser(user) {
		return artifacts
	}

	filtered := make([]rssh.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if canAccessArtifact(user, artifact.URLPath, assignments) {
			filtered = append(filtered, artifact)
		}
	}
	return filtered
}
