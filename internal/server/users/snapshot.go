package users

import (
	"net"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh"
)

type ClientSnapshot struct {
	ConnectionID         string   `json:"connectionId"`
	StableID             string   `json:"stableId"`
	Hostname             string   `json:"hostname"`
	RemoteAddr           string   `json:"remoteAddr"`
	RemoteIP             string   `json:"remoteIp"`
	Version              string   `json:"version"`
	Owners               []string `json:"owners"`
	Comment              string   `json:"comment"`
	PublicKeyFingerprint string   `json:"publicKeyFingerprint"`
	Aliases              []string `json:"aliases"`
	IsPublic             bool     `json:"isPublic"`
	Connected            bool     `json:"connected"`
}

func ListClientSnapshots() []ClientSnapshot {
	lck.RLock()
	defer lck.RUnlock()

	snapshots := make([]ClientSnapshot, 0, len(allClients))
	for id, conn := range allClients {
		snapshots = append(snapshots, snapshotFromConnLocked(id, conn))
	}

	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Hostname == snapshots[j].Hostname {
			return snapshots[i].ConnectionID < snapshots[j].ConnectionID
		}
		return snapshots[i].Hostname < snapshots[j].Hostname
	})

	return snapshots
}

func GetClientSnapshotByConnectionID(id string) (ClientSnapshot, bool) {
	lck.RLock()
	defer lck.RUnlock()

	conn, ok := allClients[id]
	if !ok {
		return ClientSnapshot{}, false
	}

	return snapshotFromConnLocked(id, conn), true
}

func GetClientSnapshotByFingerprint(fp string) (ClientSnapshot, bool) {
	lck.RLock()
	defer lck.RUnlock()

	for id, conn := range allClients {
		if conn.Permissions.Extensions["pubkey-fp"] == fp {
			return snapshotFromConnLocked(id, conn), true
		}
	}

	return ClientSnapshot{}, false
}

func ListClientSnapshotsByFingerprint(fp string) []ClientSnapshot {
	lck.RLock()
	defer lck.RUnlock()

	snapshots := make([]ClientSnapshot, 0, 2)
	for id, conn := range allClients {
		if conn.Permissions.Extensions["pubkey-fp"] == fp {
			snapshots = append(snapshots, snapshotFromConnLocked(id, conn))
		}
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ConnectionID < snapshots[j].ConnectionID
	})

	return snapshots
}

func GetClientConnection(id string) (*ssh.ServerConn, bool) {
	lck.RLock()
	defer lck.RUnlock()

	conn, ok := allClients[id]
	return conn, ok
}

func GetClientConnectionByFingerprint(fp string) (*ssh.ServerConn, string, bool) {
	lck.RLock()
	defer lck.RUnlock()

	for id, conn := range allClients {
		if conn.Permissions.Extensions["pubkey-fp"] == fp {
			return conn, id, true
		}
	}

	return nil, "", false
}

func snapshotFromConnLocked(id string, conn *ssh.ServerConn) ClientSnapshot {
	aliasesCopy := append([]string(nil), uniqueIdToAllAliases[id]...)
	sort.Strings(aliasesCopy)

	owners := splitOwners(conn.Permissions.Extensions["owners"])
	return ClientSnapshot{
		ConnectionID:         id,
		StableID:             id,
		Hostname:             NormaliseHostname(conn.User()),
		RemoteAddr:           conn.RemoteAddr().String(),
		RemoteIP:             remoteIP(conn.RemoteAddr().String()),
		Version:              string(conn.ClientVersion()),
		Owners:               owners,
		Comment:              conn.Permissions.Extensions["comment"],
		PublicKeyFingerprint: conn.Permissions.Extensions["pubkey-fp"],
		Aliases:              aliasesCopy,
		IsPublic:             len(owners) == 0,
		Connected:            true,
	}
}

func splitOwners(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	owners := make([]string, 0, len(parts))
	for _, owner := range parts {
		owner = strings.TrimSpace(owner)
		if owner == "" {
			continue
		}
		owners = append(owners, owner)
	}

	return owners
}

func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return strings.Trim(strings.Trim(host, "]"), "[")
	}

	return addr
}
