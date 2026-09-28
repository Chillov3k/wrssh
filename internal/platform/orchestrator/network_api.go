package orchestrator

import (
	"context"
	"net/http"
	"path"
	"strconv"

	"github.com/NHAS/reverse_ssh/internal/platform/networkmap"
)

// GetNetworkMap returns the scan-discovered network map stored in the
// project's runtime postgres.
func (m *Manager) GetNetworkMap(ctx context.Context, projectName string) (networkmap.MapResponse, error) {
	response := networkmap.MapResponse{}
	err := m.runtimeJSON(ctx, projectName, http.MethodGet, "/internal/network-map", nil, &response)
	if response.Nodes == nil {
		response.Nodes = []networkmap.Node{}
	}
	if response.Edges == nil {
		response.Edges = []networkmap.Edge{}
	}
	return response, err
}

// PushNetworkScan records one pscan run result in the project runtime.
func (m *Manager) PushNetworkScan(ctx context.Context, projectName string, payload networkmap.ScanPayload) (networkmap.ScanResponse, error) {
	response := networkmap.ScanResponse{}
	err := m.runtimeJSON(ctx, projectName, http.MethodPut, "/internal/network-map/scan", payload, &response)
	return response, err
}

// ClearNetworkMap removes all stored scan discoveries from the project runtime.
func (m *Manager) ClearNetworkMap(ctx context.Context, projectName string) error {
	return m.runtimeJSON(ctx, projectName, http.MethodDelete, path.Clean("/internal/network-map"), nil, nil)
}

// UpsertNetworkNote creates or updates one map note in the project runtime.
func (m *Manager) UpsertNetworkNote(ctx context.Context, projectName string, note networkmap.Note) (networkmap.Note, error) {
	stored := networkmap.Note{}
	err := m.runtimeJSON(ctx, projectName, http.MethodPut, "/internal/network-map/notes", note, &stored)
	return stored, err
}

// DeleteNetworkNote removes one map note from the project runtime.
func (m *Manager) DeleteNetworkNote(ctx context.Context, projectName string, noteID uint64) error {
	return m.runtimeJSON(ctx, projectName, http.MethodDelete, "/internal/network-map/notes/"+strconv.FormatUint(noteID, 10), nil, nil)
}
