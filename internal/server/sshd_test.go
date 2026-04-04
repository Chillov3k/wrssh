package server

import "testing"

func TestAuthorizedUserKeysPath(t *testing.T) {
	path, err := authorizedUserKeysPath("/data/keys", "alice")
	if err != nil {
		t.Fatalf("authorizedUserKeysPath returned error: %v", err)
	}
	if path != "/data/keys/alice" {
		t.Fatalf("authorizedUserKeysPath returned %q, want %q", path, "/data/keys/alice")
	}
}

func TestAuthorizedUserKeysPathRejectsTraversal(t *testing.T) {
	for _, username := range []string{"", ".", "..", "/alice", "../alice", "team/alice"} {
		if _, err := authorizedUserKeysPath("/data/keys", username); err == nil {
			t.Fatalf("authorizedUserKeysPath(%q) expected error", username)
		}
	}
}
