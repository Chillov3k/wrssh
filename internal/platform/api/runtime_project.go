package api

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"gorm.io/gorm"
)

func (s *Server) projectUsesRemoteRuntime(project string) (bool, error) {
	if s.runtimes == nil {
		return false, nil
	}

	projectName := strings.TrimSpace(project)
	if projectName == "" {
		return false, nil
	}

	return s.runtimes.HasProjectRuntime(projectName)
}

func (s *Server) projectRuntimeState(project string) (store.ProjectRuntimeRecord, bool, error) {
	if s.runtimes == nil {
		return store.ProjectRuntimeRecord{}, false, nil
	}

	projectName := strings.TrimSpace(project)
	if projectName == "" {
		return store.ProjectRuntimeRecord{}, false, nil
	}

	runtime, err := s.store.GetProjectRuntime(projectName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.ProjectRuntimeRecord{}, false, nil
		}
		return store.ProjectRuntimeRecord{}, false, err
	}

	switch runtime.Status {
	case store.RuntimeStatusRunning, store.RuntimeStatusDegraded:
		return runtime, true, nil
	default:
		return runtime, false, nil
	}
}

func (s *Server) syncProjectRuntimeHosts(ctx context.Context, project string) (map[string][]hostConnectionResponse, error) {
	useRuntime, err := s.projectUsesRemoteRuntime(project)
	if err != nil || !useRuntime {
		return nil, err
	}
	_, ready, err := s.projectRuntimeState(project)
	if err != nil {
		return nil, err
	}
	if !ready {
		return map[string][]hostConnectionResponse{}, nil
	}

	snapshots, err := s.runtimes.ListClientSnapshots(ctx, project)
	if err != nil {
		return nil, err
	}

	result := map[string][]hostConnectionResponse{}
	now := time.Now()
	stableIDs := make([]string, 0, len(snapshots))
	seen := make(map[string]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		if _, err := s.store.UpsertHostFromSnapshotForProject(snapshot, now, project); err != nil {
			return nil, err
		}
		if _, ok := seen[snapshot.StableID]; !ok {
			seen[snapshot.StableID] = struct{}{}
			stableIDs = append(stableIDs, snapshot.StableID)
		}

		result[snapshot.StableID] = append(result[snapshot.StableID], hostConnectionResponse{
			ConnectionID: snapshot.ConnectionID,
			Hostname:     snapshot.Hostname,
			RemoteAddr:   snapshot.RemoteAddr,
			RemoteIP:     snapshot.RemoteIP,
			InternalIP:   snapshot.InternalIP,
			Version:      snapshot.Version,
			Comment:      snapshot.Comment,
		})
	}

	for stableID := range result {
		sort.Slice(result[stableID], func(i, j int) bool {
			return result[stableID][i].ConnectionID < result[stableID][j].ConnectionID
		})
	}

	if err := s.store.ReconcileProjectConnectedHosts(project, stableIDs); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Server) activeConnectionsForProjectHost(project, stableID string, remote map[string][]hostConnectionResponse) []hostConnectionResponse {
	if remote != nil {
		if connections, ok := remote[stableID]; ok {
			return connections
		}
		return nil
	}

	return s.activeConnectionsForHost(stableID)
}

func (s *Server) artifactsForProject(ctx context.Context, project string) ([]rssh.Artifact, error) {
	useRuntime, err := s.projectUsesRemoteRuntime(project)
	if err != nil {
		return nil, err
	}
	if useRuntime {
		_, ready, err := s.projectRuntimeState(project)
		if err != nil {
			return nil, err
		}
		if !ready {
			return []rssh.Artifact{}, nil
		}
		artifacts, err := s.runtimes.ListArtifacts(ctx, project)
		if err != nil {
			return nil, err
		}
		for _, artifact := range artifacts {
			if assignErr := s.store.AssignArtifactProject(artifact.URLPath, project); assignErr != nil {
				return nil, assignErr
			}
		}
		return artifacts, nil
	}
	return s.rsshService.ListArtifacts()
}

func (s *Server) jumpAddressForProject(ctx context.Context, project string) string {
	useRuntime, err := s.projectUsesRemoteRuntime(project)
	if err == nil && useRuntime {
		if address, runtimeErr := s.runtimes.RuntimeExternalAddress(project); runtimeErr == nil && strings.TrimSpace(address) != "" {
			return address
		}
	}
	return s.sshJumpAddress()
}
