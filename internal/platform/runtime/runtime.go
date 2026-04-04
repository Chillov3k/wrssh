package runtime

import (
	"sync"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"github.com/NHAS/reverse_ssh/internal/server/observers"
	"github.com/NHAS/reverse_ssh/internal/server/users"
)

type Runtime struct {
	store              *store.Store
	mu                 sync.RWMutex
	connectionToStable map[string]string
	clientObserverID   string
}

func New(s *store.Store) *Runtime {
	return &Runtime{
		store:              s,
		connectionToStable: make(map[string]string),
	}
}

func (r *Runtime) Start() error {
	r.SyncConnectedClients(time.Now())

	r.clientObserverID = observers.ConnectionState.Register(func(event observers.ClientState) {
		r.handleClientEvent(event)
	})

	return nil
}

func (r *Runtime) Stop() {
	if r.clientObserverID != "" {
		observers.ConnectionState.Deregister(r.clientObserverID)
	}
}

func (r *Runtime) SyncConnectedClients(seenAt time.Time) {
	snapshots := users.ListClientSnapshots()
	r.resetConnectionIndex()

	liveStableIDs := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if _, err := r.store.UpsertHostFromSnapshot(snapshot, seenAt); err == nil {
			r.setConnectionStable(snapshot.ConnectionID, snapshot.StableID)
			liveStableIDs = append(liveStableIDs, snapshot.StableID)
		}
	}

	_ = r.store.ReconcileConnectedHosts(liveStableIDs)
}

func (r *Runtime) StableIDForConnection(connectionID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connectionToStable[connectionID]
}

func (r *Runtime) handleClientEvent(event observers.ClientState) {
	now := event.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	switch event.Status {
	case "connected":
		snapshot, ok := users.GetClientSnapshotByConnectionID(event.ID)
		if !ok {
			return
		}
		if _, err := r.store.UpsertHostFromSnapshot(snapshot, now); err == nil {
			r.setConnectionStable(snapshot.ConnectionID, snapshot.StableID)
		}
	case "disconnected":
		stableID := r.StableIDForConnection(event.ID)
		if stableID == "" {
			return
		}
		if stableID == event.ID {
			_ = r.store.MarkHostDisconnected(stableID, event.ID, now)
		} else if remaining := users.ListClientSnapshotsByFingerprint(stableID); len(remaining) > 0 {
			_, _ = r.store.UpsertHostFromSnapshot(remaining[0], now)
		} else {
			_ = r.store.MarkHostDisconnected(stableID, event.ID, now)
		}
		r.deleteConnectionStable(event.ID)
	}
}

func (r *Runtime) setConnectionStable(connectionID, stableID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectionToStable[connectionID] = stableID
}

func (r *Runtime) deleteConnectionStable(connectionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connectionToStable, connectionID)
}

func (r *Runtime) resetConnectionIndex() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectionToStable = make(map[string]string)
}
