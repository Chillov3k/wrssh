package store

import "testing"

func TestListProjectAccessUsersIncludesAdminsAndAssignedUsers(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.CreateProject("alpha", "desc", nil); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.CreateProject("beta", "desc", nil); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if err := store.EnsureUser("admin", "secret", "admin", "admin"); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}

	adminKeys := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFL8hB3tT3lcsdJJx6v0v8J8rjTegZs1l2vg92N3cX6p admin@example"
	admin, err := store.UpdateManagedUser("admin", WebUserPatch{
		SSHAuthorizedKeys: &adminKeys,
	})
	if err != nil {
		t.Fatalf("update admin keys: %v", err)
	}
	if admin.Role != "admin" {
		t.Fatalf("expected admin role, got %+v", admin)
	}

	if _, err := store.CreateManagedUser("alice", "secret", false, []string{"alpha"}, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILj7i8U85D8jkWl02X2H5Q7+3J35RjX7sgtM5G5QFq10 alice@example"); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, err := store.CreateManagedUser("bob", "secret", false, []string{"beta"}, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJzWl7zth4+8mHmqM1cnfP9J6x+gttY9D7V6v0XG3U8n bob@example"); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	users, err := store.ListProjectAccessUsers("alpha")
	if err != nil {
		t.Fatalf("list project access users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %+v", users)
	}
	if users[0].Username != "admin" || users[1].Username != "alice" {
		t.Fatalf("unexpected users: %+v", users)
	}
}
