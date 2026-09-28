package api

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/networkmap"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
)

type networkMapSummary struct {
	Captured   int64 `json:"captured"`
	Online     int64 `json:"online"`
	Discovered int64 `json:"discovered"`
}

type networkMapResponse struct {
	Nodes   []networkmap.Node `json:"nodes"`
	Edges   []networkmap.Edge `json:"edges"`
	Notes   []networkmap.Note `json:"notes"`
	Summary networkMapSummary `json:"summary"`
}

// capturedHost groups every host record that belongs to the same machine.
// The rssh server assigns a fresh stable id per connection, so one machine can
// accumulate several records (online and historical); the map renders exactly
// one computer for them.
type capturedHost struct {
	primaryIP   string
	records     []store.HostRecord
	ports       []networkmap.Port
	firstSeenAt *string
	lastSeenAt  *string
}

var platformVersionPattern = regexp.MustCompile(`-([A-Za-z0-9]+)_([A-Za-z0-9]+)$`)

func (s *Server) handleNetworkMap(w http.ResponseWriter, r *http.Request) {
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
	filteredHosts := filterHostsByProject(filterHostsForWebUser(hosts, user), selectedProject)

	discovered, edges, notes, err := s.storedNetworkMap(r.Context(), selectedProject)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	response := buildNetworkMap(filteredHosts, remoteConnections, discovered, edges, notes)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleDeleteNetworkMap(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := requestedProject(r)
	if strings.TrimSpace(projectName) == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}
	if !requireProjectAccess(w, user, projectName) {
		return
	}

	useRuntime, err := s.projectUsesRemoteRuntime(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if useRuntime {
		_, ready, stateErr := s.projectRuntimeState(projectName)
		if stateErr != nil {
			writeError(w, http.StatusInternalServerError, stateErr.Error())
			return
		}
		if ready {
			if err := s.runtimes.ClearNetworkMap(r.Context(), projectName); err != nil {
				writeError(w, http.StatusBadGateway, err.Error())
				return
			}
		}
	}

	if err := s.store.ClearNetworkScanForProject(projectName); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleUpsertNetworkNote(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := requestedProject(r)
	if strings.TrimSpace(projectName) == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}
	if !requireProjectAccess(w, user, projectName) {
		return
	}

	note := networkmap.Note{}
	if err := decodeJSON(r, &note); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	useRuntime, err := s.projectUsesRemoteRuntime(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if useRuntime {
		stored, err := s.runtimes.UpsertNetworkNote(r.Context(), projectName, note)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stored)
		return
	}

	stored, err := s.store.UpsertNetworkNoteForProject(projectName, note)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (s *Server) handleDeleteNetworkNote(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	projectName := requestedProject(r)
	if strings.TrimSpace(projectName) == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}
	if !requireProjectAccess(w, user, projectName) {
		return
	}

	noteID, err := strconv.ParseUint(strings.TrimSpace(r.PathValue("noteID")), 10, 64)
	if err != nil || noteID == 0 {
		writeError(w, http.StatusBadRequest, "note id is required")
		return
	}

	useRuntime, err := s.projectUsesRemoteRuntime(projectName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if useRuntime {
		if err := s.runtimes.DeleteNetworkNote(r.Context(), projectName, noteID); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if err := s.store.DeleteNetworkNoteForProject(projectName, noteID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// storedNetworkMap returns the persisted pscan discoveries for a project from
// the per-project runtime postgres when one exists, or from the control-plane
// database otherwise.
func (s *Server) storedNetworkMap(ctx context.Context, projectName string) ([]networkmap.Node, []networkmap.Edge, []networkmap.Note, error) {
	useRuntime, err := s.projectUsesRemoteRuntime(projectName)
	if err != nil {
		return nil, nil, nil, err
	}
	if useRuntime {
		_, ready, err := s.projectRuntimeState(projectName)
		if err != nil {
			return nil, nil, nil, err
		}
		if ready {
			stored, err := s.runtimes.GetNetworkMap(ctx, projectName)
			if err != nil {
				return nil, nil, nil, err
			}
			return stored.Nodes, stored.Edges, stored.Notes, nil
		}
		return []networkmap.Node{}, []networkmap.Edge{}, []networkmap.Note{}, nil
	}

	nodeRecords, edgeRecords, err := s.store.ListNetworkScanForProject(projectName)
	if err != nil {
		return nil, nil, nil, err
	}

	nodes := make([]networkmap.Node, 0, len(nodeRecords))
	for _, record := range nodeRecords {
		nodes = append(nodes, networkmap.Node{
			IP:          record.IP,
			Hostname:    strings.TrimSpace(record.Hostname),
			OS:          strings.TrimSpace(record.OS),
			Source:      networkmap.SourcePscan,
			Ports:       decodePortCSV(record.Ports),
			FirstSeenAt: formatStoreTime(record.FirstSeenAt),
			LastSeenAt:  formatStoreTime(record.LastSeenAt),
		})
	}

	edges := make([]networkmap.Edge, 0, len(edgeRecords))
	for _, record := range edgeRecords {
		edges = append(edges, networkmap.Edge{
			From:       record.FromIP,
			To:         record.ToIP,
			LastSeenAt: formatStoreTime(record.LastSeenAt),
		})
	}

	notes, err := s.store.ListNetworkNotesForProject(projectName)
	if err != nil {
		return nil, nil, nil, err
	}

	return nodes, edges, notes, nil
}

// buildNetworkMap merges stored pscan discoveries with captured hosts and
// their live sessions:
//
//   - one node per captured host (identified by its internal/remote IP),
//     showing the most privileged online session user;
//   - captured hosts that went offline stay on the map and turn gray;
//   - scan-discovered hosts without an agent render as plain IP nodes;
//   - scan edges point from the scanning host to each discovered host.
func buildNetworkMap(hosts []store.HostRecord, remoteConnections map[string][]hostConnectionResponse, discovered []networkmap.Node, edges []networkmap.Edge, notes []networkmap.Note) networkMapResponse {
	discoveredByIP := make(map[string]networkmap.Node, len(discovered))
	for _, node := range discovered {
		if strings.TrimSpace(node.IP) == "" {
			continue
		}
		discoveredByIP[node.IP] = node
	}

	captured := make([]capturedHost, 0, len(hosts))
	groupIndex := make(map[string]int, len(hosts))
	primaryIPByEndpoint := make(map[string]string, len(hosts)*2)
	for _, host := range hosts {
		internalIP := strings.TrimSpace(host.InternalIP)
		remoteIP := strings.TrimSpace(host.RemoteIP)
		primaryIP := internalIP
		if primaryIP == "" {
			primaryIP = remoteIP
		}
		if primaryIP == "" {
			continue
		}

		index, ok := groupIndex[primaryIP]
		if !ok {
			captured = append(captured, capturedHost{primaryIP: primaryIP})
			index = len(captured) - 1
			groupIndex[primaryIP] = index
		}
		captured[index].records = append(captured[index].records, host)

		if remoteIP != "" {
			primaryIPByEndpoint[remoteIP] = primaryIP
		}
		if internalIP != "" && internalIP != remoteIP {
			primaryIPByEndpoint[internalIP] = primaryIP
		}
	}

	// Merge discovered scan knowledge into the captured hosts it covers.
	covered := make(map[string]struct{})
	for ip, node := range discoveredByIP {
		primaryIP, ok := primaryIPByEndpoint[ip]
		if !ok {
			continue
		}
		index, ok := groupIndex[primaryIP]
		if !ok {
			continue
		}
		covered[ip] = struct{}{}
		captured[index].ports = mergePorts(captured[index].ports, node.Ports)
		captured[index].firstSeenAt = earliestTime(captured[index].firstSeenAt, node.FirstSeenAt)
		captured[index].lastSeenAt = latestTime(captured[index].lastSeenAt, node.LastSeenAt)
	}

	nodes := make([]networkmap.Node, 0, len(captured)+len(discovered))
	var summary networkMapSummary

	for _, entry := range captured {
		user, hostname, os, stableID, online := bestCapturedIdentity(entry.records, remoteConnections)
		lastActivityAt := newestHostActivity(entry.records)

		nodes = append(nodes, networkmap.Node{
			IP:          entry.primaryIP,
			ExternalIP:  strings.TrimSpace(entry.records[0].RemoteIP),
			InternalIP:  strings.TrimSpace(entry.records[0].InternalIP),
			Hostname:    hostname,
			User:        user,
			OS:          os,
			StableID:    stableID,
			Captured:    true,
			Online:      online,
			Source:      networkmap.SourceAgent,
			Ports:       entry.ports,
			FirstSeenAt: coalesceTime(entry.firstSeenAt, oldestHostSeen(entry.records)),
			LastSeenAt:  coalesceTime(entry.lastSeenAt, lastActivityAt),
		})

		summary.Captured++
		if online {
			summary.Online++
		}
	}

	for _, node := range discovered {
		if _, ok := covered[node.IP]; ok {
			continue
		}

		nodes = append(nodes, networkmap.Node{
			IP:          node.IP,
			Hostname:    node.Hostname,
			OS:          node.OS,
			Captured:    false,
			Online:      false,
			Source:      networkmap.SourcePscan,
			Ports:       node.Ports,
			FirstSeenAt: node.FirstSeenAt,
			LastSeenAt:  node.LastSeenAt,
		})
		summary.Discovered++
	}

	// Remap edge endpoints that belong to captured hosts onto the captured
	// host's primary IP, and keep endpoints that have no node renderable.
	seenEdges := make(map[networkmap.Edge]struct{}, len(edges))
	mergedEdges := make([]networkmap.Edge, 0, len(edges))
	missingEndpoints := make(map[string]struct{})
	for _, edge := range edges {
		from := resolveEndpoint(edge.From, primaryIPByEndpoint)
		to := resolveEndpoint(edge.To, primaryIPByEndpoint)
		if from == "" || to == "" || from == to {
			continue
		}
		key := networkmap.Edge{From: from, To: to}
		if _, ok := seenEdges[key]; ok {
			continue
		}
		seenEdges[key] = struct{}{}
		mergedEdges = append(mergedEdges, networkmap.Edge{From: from, To: to, LastSeenAt: edge.LastSeenAt})

		for _, endpoint := range []string{from, to} {
			if !nodeExists(nodes, endpoint) {
				missingEndpoints[endpoint] = struct{}{}
			}
		}
	}

	for endpoint := range missingEndpoints {
		nodes = append(nodes, networkmap.Node{
			IP:     endpoint,
			Source: networkmap.SourcePscan,
		})
		summary.Discovered++
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Captured != nodes[j].Captured {
			return nodes[i].Captured
		}
		if nodes[i].Online != nodes[j].Online {
			return nodes[i].Online
		}
		return compareIPs(nodes[i].IP, nodes[j].IP) < 0
	})
	sort.SliceStable(mergedEdges, func(i, j int) bool {
		if mergedEdges[i].From != mergedEdges[j].From {
			return mergedEdges[i].From < mergedEdges[j].From
		}
		return mergedEdges[i].To < mergedEdges[j].To
	})

	return networkMapResponse{
		Nodes:   nodes,
		Edges:   mergedEdges,
		Notes:   notes,
		Summary: summary,
	}
}

// bestCapturedIdentity collapses a group of host records (one machine, possibly
// several connection records) into a single identity:
//
//   - online sessions win, showing the most privileged user among them;
//   - with no session online the last known metadata is kept so the node stays
//     on the map in its offline state, again showing the most privileged user.
func bestCapturedIdentity(records []store.HostRecord, remoteConnections map[string][]hostConnectionResponse) (user, hostname, os, stableID string, online bool) {
	bestPrivilege := -1

	for _, host := range records {
		for _, connection := range remoteConnections[host.StableID] {
			connectionUser := usernameFromHostname(connection.Hostname)
			privilege := userPrivilege(connectionUser)
			if privilege > bestPrivilege {
				bestPrivilege = privilege
				user = connectionUser
				hostname = strings.TrimSpace(connection.Hostname)
				os = parsePlatformVersion(connection.Version)
				stableID = host.StableID
			}
		}
		if bestPrivilege >= 0 {
			online = true
		}
	}
	if online {
		return user, hostname, os, stableID, true
	}

	sorted := append([]store.HostRecord(nil), records...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		leftTime := hostActivityTime(left)
		rightTime := hostActivityTime(right)
		return rightTime.Before(leftTime)
	})
	for _, host := range sorted {
		knownUser := usernameFromHostname(host.Hostname)
		privilege := userPrivilege(knownUser)
		if privilege > bestPrivilege {
			bestPrivilege = privilege
			user = knownUser
			hostname = strings.TrimSpace(host.Hostname)
			os = parsePlatformVersion(host.Version)
			stableID = host.StableID
		}
	}
	if stableID == "" && len(sorted) > 0 {
		stableID = sorted[0].StableID
	}

	return user, hostname, os, stableID, false
}

func hostActivityTime(host store.HostRecord) time.Time {
	if host.LastActivityAt != nil {
		return *host.LastActivityAt
	}
	if host.LastSeenAt != nil {
		return *host.LastSeenAt
	}
	if host.FirstSeenAt != nil {
		return *host.FirstSeenAt
	}
	return host.CreatedAt
}

func newestHostActivity(records []store.HostRecord) *string {
	var newest *time.Time
	for index := range records {
		value := hostActivityTime(records[index])
		if value.IsZero() {
			continue
		}
		if newest == nil || value.After(*newest) {
			copied := value
			newest = &copied
		}
	}
	return formatStoreTime(newest)
}

func oldestHostSeen(records []store.HostRecord) *string {
	var oldest *time.Time
	for index := range records {
		value := records[index].FirstSeenAt
		if value == nil || value.IsZero() {
			continue
		}
		if oldest == nil || value.Before(*oldest) {
			copied := *value
			oldest = &copied
		}
	}
	return formatStoreTime(oldest)
}

// userPrivilege ranks session users so the most privileged one wins:
// root/SYSTEM beat administrators, administrators beat everyone else.
func userPrivilege(username string) int {
	value := strings.ToLower(strings.TrimSpace(username))
	switch {
	case value == "root" || value == "system":
		return 3
	case strings.Contains(value, "admin"):
		return 2
	case value == "":
		return -1
	default:
		return 1
	}
}

func usernameFromHostname(hostname string) string {
	value := strings.TrimSpace(hostname)
	dot := strings.Index(value, ".")
	if dot > 0 {
		return value[:dot]
	}
	return value
}

func parsePlatformVersion(version string) string {
	match := platformVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if len(match) < 2 {
		return ""
	}
	return strings.ToLower(match[1])
}

func resolveEndpoint(ip string, primaryIPByEndpoint map[string]string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	if primary, ok := primaryIPByEndpoint[ip]; ok {
		return primary
	}
	return ip
}

func nodeExists(nodes []networkmap.Node, ip string) bool {
	for _, node := range nodes {
		if node.IP == ip {
			return true
		}
	}
	return false
}

func mergePorts(existing, incoming []networkmap.Port) []networkmap.Port {
	combined := make([]networkmap.Port, 0, len(existing)+len(incoming))
	combined = append(combined, existing...)
	combined = append(combined, incoming...)

	seen := make(map[networkmap.Port]struct{}, len(combined))
	ports := make([]networkmap.Port, 0, len(combined))
	for _, port := range combined {
		if port.Port <= 0 {
			continue
		}
		if port.Protocol == "" {
			port.Protocol = "tcp"
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Port != ports[j].Port {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Protocol < ports[j].Protocol
	})
	if len(ports) == 0 {
		return nil
	}
	return ports
}

func earliestTime(left, right *string) *string {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if strings.Compare(*left, *right) <= 0 {
		return left
	}
	return right
}

func latestTime(left, right *string) *string {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if strings.Compare(*left, *right) >= 0 {
		return left
	}
	return right
}

func coalesceTime(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func formatStoreTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func decodePortCSV(raw string) []networkmap.Port {
	ports := []networkmap.Port{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		portText, protocol, ok := strings.Cut(item, "/")
		if !ok {
			continue
		}
		port, valid := atoiSafe(portText)
		if !valid || port <= 0 || port > 65535 {
			continue
		}
		ports = append(ports, networkmap.Port{Port: port, Protocol: protocol})
	}
	if len(ports) == 0 {
		return nil
	}
	return ports
}

// compareIPs orders IPv4 addresses numerically and everything else
// lexicographically, mirroring the hosts list sorting.
func compareIPs(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	if len(leftParts) == 4 && len(rightParts) == 4 {
		for index := 0; index < 4; index++ {
			leftValue, leftOK := atoiSafe(leftParts[index])
			rightValue, rightOK := atoiSafe(rightParts[index])
			if leftOK && rightOK && leftValue != rightValue {
				if leftValue < rightValue {
					return -1
				}
				return 1
			}
		}
		return 0
	}
	return strings.Compare(left, right)
}

func atoiSafe(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	result := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
		result = result*10 + int(char-'0')
	}
	return result, true
}

// recordPscanResults stores the discoveries of a finished pscan module run so
// they render on the network map. Failures are logged and never fail the
// module run itself.
func (s *Server) recordPscanResults(ctx context.Context, target hostFileTarget, module, output string) {
	if strings.TrimSpace(module) != "pscan" || strings.TrimSpace(output) == "" {
		return
	}

	discoveries := networkmap.ParsePscanOutput(output)
	if len(discoveries) == 0 {
		return
	}

	sourceIP := strings.TrimSpace(target.host.InternalIP)
	if sourceIP == "" {
		sourceIP = strings.TrimSpace(target.host.RemoteIP)
	}
	if sourceIP == "" {
		log.Printf("[network-map] skipping scan recording for host %s: no source ip known", target.host.StableID)
		return
	}

	payload := networkmap.ScanPayload{SourceIP: sourceIP, Nodes: discoveries}

	useRuntime, err := s.projectUsesRemoteRuntime(target.projectName)
	if err != nil {
		log.Printf("[network-map] resolve runtime for project %s: %v", target.projectName, err)
		return
	}
	if useRuntime {
		if _, err := s.runtimes.PushNetworkScan(ctx, target.projectName, payload); err != nil {
			log.Printf("[network-map] push scan results for project %s: %v", target.projectName, err)
		}
		return
	}

	if err := s.store.UpsertNetworkScanForProject(target.projectName, sourceIP, discoveries); err != nil {
		log.Printf("[network-map] store scan results for project %s: %v", target.projectName, err)
	}
}
