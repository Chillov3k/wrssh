package runtimeagent

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/NHAS/reverse_ssh/internal/platform/networkmap"
	"github.com/NHAS/reverse_ssh/internal/server/data"
)

func (s *Server) handleGetNetworkMap(w http.ResponseWriter, _ *http.Request) {
	nodes, err := data.ListNetworkNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	edges, err := data.ListNetworkEdges()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	notes, err := data.ListNetworkNotes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, networkmap.MapResponse{
		Nodes: networkNodesFromData(nodes),
		Edges: networkEdgesFromData(edges),
		Notes: notes,
	})
}

func (s *Server) handlePushNetworkScan(w http.ResponseWriter, r *http.Request) {
	payload := networkmap.ScanPayload{}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json payload"})
		return
	}

	targets := make([]data.NetworkScanTarget, 0, len(payload.Nodes))
	for _, node := range payload.Nodes {
		ports := make([]string, 0, len(node.Ports))
		for _, port := range node.Ports {
			ports = append(ports, port.Label())
		}
		targets = append(targets, data.NetworkScanTarget{
			IP:       node.IP,
			Hostname: node.Hostname,
			OS:       node.OS,
			Ports:    ports,
		})
	}

	if err := data.UpsertNetworkScan(payload.SourceIP, targets); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	nodes, _ := data.ListNetworkNodes()
	edges, _ := data.ListNetworkEdges()
	writeJSON(w, http.StatusOK, networkmap.ScanResponse{
		OK:    true,
		Nodes: len(nodes),
		Edges: len(edges),
	})
}

func (s *Server) handleClearNetworkMap(w http.ResponseWriter, _ *http.Request) {
	if err := data.ClearNetworkData(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func networkNodesFromData(records []data.NetworkNode) []networkmap.Node {
	nodes := make([]networkmap.Node, 0, len(records))
	for _, record := range records {
		nodes = append(nodes, networkmap.Node{
			IP:          record.IP,
			Hostname:    strings.TrimSpace(record.Hostname),
			OS:          strings.TrimSpace(record.OS),
			Source:      networkmap.SourcePscan,
			Ports:       parsePortCSV(record.Ports),
			FirstSeenAt: formatTimePointer(record.FirstSeenAt),
			LastSeenAt:  formatTimePointer(record.LastSeenAt),
		})
	}
	return nodes
}

func networkEdgesFromData(records []data.NetworkEdge) []networkmap.Edge {
	edges := make([]networkmap.Edge, 0, len(records))
	for _, record := range records {
		edges = append(edges, networkmap.Edge{
			From:       record.FromIP,
			To:         record.ToIP,
			LastSeenAt: formatTimePointer(record.LastSeenAt),
		})
	}
	return edges
}

func parsePortCSV(raw string) []networkmap.Port {
	ports := []networkmap.Port{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		port, protocol, ok := strings.Cut(item, "/")
		parsed, valid := parsePortNumber(port)
		if !ok || !valid {
			continue
		}
		ports = append(ports, networkmap.Port{Port: parsed, Protocol: protocol})
	}
	return ports
}

func parsePortNumber(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value := 0
	for _, char := range raw {
		if char < '0' || char > '9' {
			return 0, false
		}
		value = value*10 + int(char-'0')
		if value > 65535 {
			return 0, false
		}
	}
	if value <= 0 {
		return 0, false
	}
	return value, true
}

func (s *Server) handleUpsertNetworkNote(w http.ResponseWriter, r *http.Request) {
	note := networkmap.Note{}
	if err := decodeJSON(r, &note); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json payload"})
		return
	}

	stored, err := data.UpsertNetworkNote(note)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, stored)
}

func (s *Server) handleDeleteNetworkNote(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimSpace(r.PathValue("noteID"))
	id, err := strconv.ParseUint(idText, 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "note id is required"})
		return
	}

	if err := data.DeleteNetworkNote(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
