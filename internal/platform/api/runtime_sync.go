package api

import (
	"context"
	"sort"

	"github.com/NHAS/reverse_ssh/internal/platform/store"
)

func (s *Server) syncRuntimeAccessForUser(ctx context.Context, users ...store.WebUser) error {
	if s.runtimes == nil {
		return nil
	}

	projects, err := s.runtimeProjectsForUsers(users...)
	if err != nil {
		return err
	}

	for _, project := range projects {
		if err := s.runtimes.SyncProjectAccess(ctx, project); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) runtimeProjectsForUsers(users ...store.WebUser) ([]string, error) {
	if s.runtimes == nil {
		return nil, nil
	}

	seen := map[string]struct{}{}
	for _, user := range users {
		if user.Role == "admin" {
			projects, err := s.store.ListProjects()
			if err != nil {
				return nil, err
			}
			for _, project := range projects {
				seen[project.Name] = struct{}{}
			}
			continue
		}

		for _, project := range user.AllowedProjectList() {
			seen[store.DisplayProjectName(project)] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for project := range seen {
		result = append(result, project)
	}
	sort.Strings(result)
	return result, nil
}
