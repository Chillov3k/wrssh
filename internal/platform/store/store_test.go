package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	store, err := New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	return store
}

func TestEnsureUserAndAuthenticate(t *testing.T) {
	store := newTestStore(t)

	if err := store.EnsureUser("admin", "secret", "admin", "admin"); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	user, err := store.Authenticate("admin", "secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	if user.Username != "admin" || user.Role != "admin" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if user.MustChangePassword {
		t.Fatalf("bootstrap admin should not require password change: %+v", user)
	}
}

func TestManagedUserStartsWithTemporaryPassword(t *testing.T) {
	store := newTestStore(t)

	user, err := store.CreateManagedUser("alice", "TempPass!42", false, nil, "")
	if err != nil {
		t.Fatalf("create managed user: %v", err)
	}
	if !user.MustChangePassword {
		t.Fatalf("expected managed user to start with temporary password: %+v", user)
	}

	authenticated, err := store.Authenticate("alice", "TempPass!42")
	if err != nil {
		t.Fatalf("authenticate managed user: %v", err)
	}
	if !authenticated.MustChangePassword {
		t.Fatalf("expected temporary password flag to persist: %+v", authenticated)
	}
}

func TestUpdateManagedUserCanClearTemporaryPassword(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.CreateManagedUser("alice", "TempPass!42", false, nil, ""); err != nil {
		t.Fatalf("create managed user: %v", err)
	}

	password := "RealPass!42"
	mustChangePassword := false
	updated, err := store.UpdateManagedUser("alice", WebUserPatch{
		Password:           &password,
		MustChangePassword: &mustChangePassword,
	})
	if err != nil {
		t.Fatalf("update managed user: %v", err)
	}
	if updated.MustChangePassword {
		t.Fatalf("expected temporary flag to be cleared: %+v", updated)
	}

	if _, err := store.Authenticate("alice", password); err != nil {
		t.Fatalf("authenticate updated managed user: %v", err)
	}
}

func TestRotateUserSessionVersion(t *testing.T) {
	store := newTestStore(t)

	if err := store.EnsureUser("admin", "secret", "admin", "admin"); err != nil {
		t.Fatalf("ensure user: %v", err)
	}

	user, err := store.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.SessionVersion != 0 {
		t.Fatalf("expected initial session version 0, got %d", user.SessionVersion)
	}

	rotated, err := store.RotateUserSessionVersion("admin")
	if err != nil {
		t.Fatalf("rotate by username: %v", err)
	}
	if rotated.SessionVersion != 1 {
		t.Fatalf("expected session version 1 after first rotation, got %d", rotated.SessionVersion)
	}

	rotated, err = store.RotateUserSessionVersionByID(rotated.ID)
	if err != nil {
		t.Fatalf("rotate by id: %v", err)
	}
	if rotated.SessionVersion != 2 {
		t.Fatalf("expected session version 2 after second rotation, got %d", rotated.SessionVersion)
	}
}

func TestValidatePasswordComplexity(t *testing.T) {
	if err := ValidatePasswordComplexity("weakpass"); err == nil {
		t.Fatal("expected weak password to fail")
	}
	if err := ValidatePasswordComplexity("NoSpecial123"); err == nil {
		t.Fatal("expected password without special character to fail")
	}
	if err := ValidatePasswordComplexity("StrongPass!"); err != nil {
		t.Fatalf("expected strong password to pass: %v", err)
	}
}

func TestUpsertHostAndFilterForOperator(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	_, err := store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID:         "conn-1",
		StableID:             "fp-1",
		Hostname:             "host-one",
		RemoteAddr:           "10.0.0.5:2222",
		RemoteIP:             "10.0.0.5",
		Version:              "SSH-test",
		Owners:               []string{"alice"},
		PublicKeyFingerprint: "fp-1",
	}, now)
	if err != nil {
		t.Fatalf("upsert private host: %v", err)
	}

	_, err = store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID:         "conn-2",
		StableID:             "fp-2",
		Hostname:             "host-two",
		RemoteAddr:           "10.0.0.6:2222",
		RemoteIP:             "10.0.0.6",
		Version:              "SSH-test",
		Owners:               nil,
		IsPublic:             true,
		PublicKeyFingerprint: "fp-2",
	}, now)
	if err != nil {
		t.Fatalf("upsert public host: %v", err)
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}

	filtered := FilterHostsForUser(hosts, WebUser{Role: "operator", RSSHUsername: "alice"})
	if len(filtered) != 2 {
		t.Fatalf("expected operator to see owned and public hosts, got %d", len(filtered))
	}

	filtered = FilterHostsForUser(hosts, WebUser{Role: "operator", RSSHUsername: "bob"})
	if len(filtered) != 1 || filtered[0].StableID != "fp-2" {
		t.Fatalf("expected operator bob to see only public host, got %+v", filtered)
	}
}

func TestPatchHostMetadataUsesDisplayNameOverride(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	_, err := store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID: "conn-1",
		StableID:     "fp-override",
		Hostname:     "reported-host",
		RemoteAddr:   "10.0.0.50:2222",
		RemoteIP:     "10.0.0.50",
		Version:      "SSH-test",
	}, now)
	if err != nil {
		t.Fatalf("upsert host: %v", err)
	}

	displayName := "custom-host"
	tags := []string{"prod", "linux"}
	project := "lab-a"
	host, err := store.PatchHostMetadata("fp-override", HostMetadataPatch{
		DisplayName: &displayName,
		Tags:        &tags,
		Project:     &project,
	})
	if err != nil {
		t.Fatalf("patch host metadata: %v", err)
	}

	if host.DisplayName != "custom-host" {
		t.Fatalf("expected custom display name, got %+v", host)
	}
	if host.Hostname != "reported-host" {
		t.Fatalf("reported hostname should be preserved, got %+v", host)
	}
	if host.Project != "lab-a" {
		t.Fatalf("expected patched project, got %+v", host)
	}
	if got := DecodeTags(host.Tags); len(got) != 2 || got[0] != "linux" || got[1] != "prod" {
		t.Fatalf("unexpected tags: %+v", got)
	}
}

func TestHostsWithSameFingerprintRemainSeparateRecords(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	for _, snapshot := range []users.ClientSnapshot{
		{
			ConnectionID:         "conn-ubuntu",
			StableID:             "conn-ubuntu",
			Hostname:             "chillov3k.ubuntu",
			RemoteAddr:           "10.0.0.101:2222",
			RemoteIP:             "10.0.0.101",
			Version:              "SSH-test",
			PublicKeyFingerprint: "shared-fp",
		},
		{
			ConnectionID:         "conn-ubuntu1",
			StableID:             "conn-ubuntu1",
			Hostname:             "chillov3k.ubuntu1",
			RemoteAddr:           "10.0.0.102:2222",
			RemoteIP:             "10.0.0.102",
			Version:              "SSH-test",
			PublicKeyFingerprint: "shared-fp",
		},
		{
			ConnectionID:         "conn-ubuntu2",
			StableID:             "conn-ubuntu2",
			Hostname:             "chillov3k.ubuntu2",
			RemoteAddr:           "10.0.0.103:2222",
			RemoteIP:             "10.0.0.103",
			Version:              "SSH-test",
			PublicKeyFingerprint: "shared-fp",
		},
	} {
		if _, err := store.UpsertHostFromSnapshot(snapshot, now); err != nil {
			t.Fatalf("upsert host %s: %v", snapshot.ConnectionID, err)
		}
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("expected 3 separate host records, got %+v", hosts)
	}

	renamed := "custom-ubuntu1"
	host, err := store.PatchHostMetadata("conn-ubuntu1", HostMetadataPatch{
		DisplayName: &renamed,
	})
	if err != nil {
		t.Fatalf("patch host metadata: %v", err)
	}
	if host.DisplayName != renamed {
		t.Fatalf("expected renamed host, got %+v", host)
	}

	ubuntu, err := store.GetHostByStableID("conn-ubuntu")
	if err != nil {
		t.Fatalf("get ubuntu: %v", err)
	}
	if ubuntu.DisplayName != "" || ubuntu.Hostname != "chillov3k.ubuntu" {
		t.Fatalf("expected ubuntu host to stay untouched, got %+v", ubuntu)
	}

	ubuntu2, err := store.GetHostByStableID("conn-ubuntu2")
	if err != nil {
		t.Fatalf("get ubuntu2: %v", err)
	}
	if ubuntu2.DisplayName != "" || ubuntu2.Hostname != "chillov3k.ubuntu2" {
		t.Fatalf("expected ubuntu2 host to stay untouched, got %+v", ubuntu2)
	}
}

func TestUpsertHostDefaultsToUnassignedProject(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	host, err := store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID: "conn-unassigned",
		StableID:     "fp-unassigned",
		Hostname:     "host-unassigned",
		RemoteAddr:   "10.0.0.70:2222",
		RemoteIP:     "10.0.0.70",
		Version:      "SSH-test",
	}, now)
	if err != nil {
		t.Fatalf("upsert host: %v", err)
	}

	if host.Project != UnassignedProjectName {
		t.Fatalf("expected default project %q, got %+v", UnassignedProjectName, host)
	}

	project, err := store.GetProject(UnassignedProjectName)
	if err != nil {
		t.Fatalf("get unassigned project: %v", err)
	}
	if project.Name != UnassignedProjectName {
		t.Fatalf("unexpected unassigned project: %+v", project)
	}
}

func TestAssignArtifactProjectBlankDefaultsToUnassigned(t *testing.T) {
	store := newTestStore(t)

	if err := store.AssignArtifactProject("artifact-default", ""); err != nil {
		t.Fatalf("assign blank artifact project: %v", err)
	}

	assignments, err := store.ListArtifactProjectAssignments()
	if err != nil {
		t.Fatalf("list artifact assignments: %v", err)
	}
	if assignments["artifact-default"] != UnassignedProjectName {
		t.Fatalf("expected artifact to default to %q, got %+v", UnassignedProjectName, assignments)
	}
}

func TestHostProjectHintIsAppliedOnFirstConnect(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	if _, err := store.CreateProject("test", "desc", []string{"lab"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if err := store.AssignHostProjectHint("fp-hinted", "test"); err != nil {
		t.Fatalf("assign host project hint: %v", err)
	}

	host, err := store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID: "conn-hinted",
		StableID:     "fp-hinted",
		Hostname:     "hinted-host",
		RemoteAddr:   "10.0.0.71:2222",
		RemoteIP:     "10.0.0.71",
		Version:      "SSH-test",
	}, now)
	if err != nil {
		t.Fatalf("upsert hinted host: %v", err)
	}

	if host.Project != "test" {
		t.Fatalf("expected hinted host to join project test, got %+v", host)
	}
}

func TestReconcileConnectedHostsMarksStaleEntriesOffline(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	for _, snapshot := range []users.ClientSnapshot{
		{
			ConnectionID: "conn-live",
			StableID:     "fp-live",
			Hostname:     "live-host",
			RemoteAddr:   "10.0.0.80:2222",
			RemoteIP:     "10.0.0.80",
			Version:      "SSH-test",
		},
		{
			ConnectionID: "conn-stale",
			StableID:     "fp-stale",
			Hostname:     "stale-host",
			RemoteAddr:   "10.0.0.81:2222",
			RemoteIP:     "10.0.0.81",
			Version:      "SSH-test",
		},
	} {
		if _, err := store.UpsertHostFromSnapshot(snapshot, now); err != nil {
			t.Fatalf("upsert host %s: %v", snapshot.StableID, err)
		}
	}

	if err := store.ReconcileConnectedHosts([]string{"fp-live"}); err != nil {
		t.Fatalf("reconcile connected hosts: %v", err)
	}

	liveHost, err := store.GetHostByStableID("fp-live")
	if err != nil {
		t.Fatalf("get live host: %v", err)
	}
	if !liveHost.Connected || liveHost.ActiveConnectionID != "conn-live" {
		t.Fatalf("expected live host to remain connected, got %+v", liveHost)
	}

	staleHost, err := store.GetHostByStableID("fp-stale")
	if err != nil {
		t.Fatalf("get stale host: %v", err)
	}
	if staleHost.Connected {
		t.Fatalf("expected stale host to be offline, got %+v", staleHost)
	}
	if staleHost.ActiveConnectionID != "" {
		t.Fatalf("expected stale active connection to be cleared, got %+v", staleHost)
	}
}

func TestUpdateAndDeleteProjectCascade(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	project, err := store.CreateProject("alpha", "first", []string{"red"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.Name != "alpha" {
		t.Fatalf("unexpected project: %+v", project)
	}

	_, err = store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID: "conn-alpha",
		StableID:     "fp-alpha",
		Hostname:     "alpha-host",
		RemoteAddr:   "10.0.0.60:2222",
		RemoteIP:     "10.0.0.60",
		Version:      "SSH-test",
	}, now)
	if err != nil {
		t.Fatalf("upsert host: %v", err)
	}

	projectName := "alpha"
	tags := []string{"linux"}
	if _, err := store.PatchHostMetadata("fp-alpha", HostMetadataPatch{
		Project: &projectName,
		Tags:    &tags,
	}); err != nil {
		t.Fatalf("assign host project: %v", err)
	}

	if _, err := store.StartSession(SessionRecord{
		SessionUID:   "session-alpha",
		Type:         "admin-console",
		Status:       "active",
		HostStableID: "fp-alpha",
		Hostname:     "alpha-host",
		StartedAt:    now,
	}); err != nil {
		t.Fatalf("start session: %v", err)
	}

	if err := store.AssignArtifactProject("artifact-alpha", "alpha"); err != nil {
		t.Fatalf("assign artifact project: %v", err)
	}

	updated, err := store.UpdateProject("alpha", "beta", "renamed", []string{"blue", "lab"})
	if err != nil {
		t.Fatalf("update project: %v", err)
	}
	if updated.Name != "beta" || updated.Description != "renamed" {
		t.Fatalf("unexpected updated project: %+v", updated)
	}

	host, err := store.GetHostByStableID("fp-alpha")
	if err != nil {
		t.Fatalf("get host after rename: %v", err)
	}
	if host.Project != "beta" {
		t.Fatalf("expected host project rename, got %+v", host)
	}

	assignments, err := store.ListArtifactProjectAssignments()
	if err != nil {
		t.Fatalf("list artifact assignments: %v", err)
	}
	if assignments["artifact-alpha"] != "beta" {
		t.Fatalf("expected artifact project rename, got %+v", assignments)
	}

	if err := store.DeleteProjectCascade("beta"); err != nil {
		t.Fatalf("delete project cascade: %v", err)
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("list hosts after delete: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected hosts to be deleted, got %+v", hosts)
	}

	sessions, err := store.ListSessions(10)
	if err != nil {
		t.Fatalf("list sessions after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected sessions to be deleted, got %+v", sessions)
	}

	assignments, err = store.ListArtifactProjectAssignments()
	if err != nil {
		t.Fatalf("list artifact assignments after delete: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected artifact assignments to be deleted, got %+v", assignments)
	}
}

func TestManagedUserProjectAccessFollowsProjectRenameAndDelete(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.CreateProject("alpha", "first", []string{"lab"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	user, err := store.CreateManagedUser("alice", "secret", true, []string{"alpha"}, "")
	if err != nil {
		t.Fatalf("create managed user: %v", err)
	}

	if !UserCanCreateProjects(user) {
		t.Fatalf("expected managed user to inherit create-projects capability")
	}
	if !UserCanAccessProject(user, "alpha") {
		t.Fatalf("expected managed user to access assigned project")
	}

	if _, err := store.UpdateProject("alpha", "beta", "renamed", []string{"prod"}); err != nil {
		t.Fatalf("rename project: %v", err)
	}

	updated, err := store.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("get managed user after rename: %v", err)
	}
	if got := updated.AllowedProjectList(); len(got) != 1 || got[0] != "beta" {
		t.Fatalf("expected renamed project grant, got %+v", got)
	}
	if !UserCanAccessProject(updated, "beta") {
		t.Fatalf("expected managed user to access renamed project")
	}

	if err := store.DeleteProjectCascade("beta"); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	updated, err = store.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("get managed user after delete: %v", err)
	}
	if got := updated.AllowedProjectList(); len(got) != 0 {
		t.Fatalf("expected deleted project grant to be removed, got %+v", got)
	}
}

func TestHostProjectHintFollowsRenameAndDelete(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	if _, err := store.CreateProject("alpha", "first", []string{"lab"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := store.AssignHostProjectHint("fp-future", "alpha"); err != nil {
		t.Fatalf("assign project hint: %v", err)
	}

	if _, err := store.UpdateProject("alpha", "beta", "renamed", []string{"prod"}); err != nil {
		t.Fatalf("rename project: %v", err)
	}

	host, err := store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID: "conn-future",
		StableID:     "fp-future",
		Hostname:     "future-host",
		RemoteAddr:   "10.0.0.91:2222",
		RemoteIP:     "10.0.0.91",
		Version:      "SSH-test",
	}, now)
	if err != nil {
		t.Fatalf("upsert host after rename: %v", err)
	}
	if host.Project != "beta" {
		t.Fatalf("expected renamed project hint to be applied, got %+v", host)
	}

	if err := store.DeleteProjectCascade("beta"); err != nil {
		t.Fatalf("delete project cascade: %v", err)
	}

	host, err = store.UpsertHostFromSnapshot(users.ClientSnapshot{
		ConnectionID: "conn-future-2",
		StableID:     "fp-future",
		Hostname:     "future-host",
		RemoteAddr:   "10.0.0.91:2222",
		RemoteIP:     "10.0.0.91",
		Version:      "SSH-test",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("upsert host after delete: %v", err)
	}
	if host.Project != UnassignedProjectName {
		t.Fatalf("expected deleted project hint to be removed, got %+v", host)
	}
}
