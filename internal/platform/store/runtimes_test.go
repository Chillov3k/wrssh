package store

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestReserveProjectRuntimeAllocatesMetadata(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.CreateProject("alpha", "desc", []string{"lab"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	runtime, err := store.ReserveProjectRuntime("alpha", ProjectRuntimeReservation{
		MinSSHPort:          2300,
		MaxSSHPort:          2305,
		MinAgentPort:        2400,
		MaxAgentPort:        2405,
		AgentListenPort:     8081,
		AgentBaseURLHost:    "host.docker.internal",
		AgentTokenEncrypted: "cipher-token",
		DBPasswordEncrypted: "cipher-db",
	})
	if err != nil {
		t.Fatalf("reserve project runtime: %v", err)
	}

	if runtime.Status != RuntimeStatusPending || runtime.DesiredState != RuntimeStatusRunning {
		t.Fatalf("unexpected runtime state: %+v", runtime)
	}
	if runtime.SSHPublishedPort != 2300 {
		t.Fatalf("expected port 2300, got %+v", runtime)
	}
	if runtime.AgentPublishedPort != 2400 {
		t.Fatalf("expected agent port 2400, got %+v", runtime)
	}
	if runtime.AgentBaseURL != "http://host.docker.internal:2400" {
		t.Fatalf("unexpected runtime agent base URL: %+v", runtime)
	}
	if runtime.RuntimeContainer == "" || runtime.DatabaseContainer == "" || runtime.ControlPlaneNetwork == "" {
		t.Fatalf("expected generated runtime metadata, got %+v", runtime)
	}

	secrets, err := store.GetProjectRuntimeSecrets("alpha")
	if err != nil {
		t.Fatalf("get project runtime secrets: %v", err)
	}
	if secrets.AgentTokenEncrypted != "cipher-token" || secrets.DBPasswordEncrypted != "cipher-db" {
		t.Fatalf("unexpected stored secrets: %+v", secrets)
	}
}

func TestReserveProjectRuntimeUsesNextFreePort(t *testing.T) {
	store := newTestStore(t)

	for _, name := range []string{"alpha", "beta"} {
		if _, err := store.CreateProject(name, "desc", nil); err != nil {
			t.Fatalf("create project %s: %v", name, err)
		}
	}

	alpha, err := store.ReserveProjectRuntime("alpha", ProjectRuntimeReservation{
		MinSSHPort:          2300,
		MaxSSHPort:          2301,
		MinAgentPort:        2400,
		MaxAgentPort:        2401,
		AgentListenPort:     8081,
		AgentBaseURLHost:    "host.docker.internal",
		AgentTokenEncrypted: "cipher-a",
		DBPasswordEncrypted: "db-a",
	})
	if err != nil {
		t.Fatalf("reserve alpha runtime: %v", err)
	}
	beta, err := store.ReserveProjectRuntime("beta", ProjectRuntimeReservation{
		MinSSHPort:          2300,
		MaxSSHPort:          2301,
		MinAgentPort:        2400,
		MaxAgentPort:        2401,
		AgentListenPort:     8081,
		AgentBaseURLHost:    "host.docker.internal",
		AgentTokenEncrypted: "cipher-b",
		DBPasswordEncrypted: "db-b",
	})
	if err != nil {
		t.Fatalf("reserve beta runtime: %v", err)
	}

	if alpha.SSHPublishedPort != 2300 || beta.SSHPublishedPort != 2301 {
		t.Fatalf("unexpected allocated ports: alpha=%d beta=%d", alpha.SSHPublishedPort, beta.SSHPublishedPort)
	}
	if alpha.AgentPublishedPort != 2400 || beta.AgentPublishedPort != 2401 {
		t.Fatalf("unexpected allocated agent ports: alpha=%d beta=%d", alpha.AgentPublishedPort, beta.AgentPublishedPort)
	}
}

func TestReserveProjectRuntimePortExhaustion(t *testing.T) {
	store := newTestStore(t)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := store.CreateProject(name, "desc", nil); err != nil {
			t.Fatalf("create project %s: %v", name, err)
		}
	}

	for _, name := range []string{"alpha", "beta"} {
		if _, err := store.ReserveProjectRuntime(name, ProjectRuntimeReservation{
			MinSSHPort:          2300,
			MaxSSHPort:          2301,
			MinAgentPort:        2400,
			MaxAgentPort:        2401,
			AgentListenPort:     8081,
			AgentBaseURLHost:    "host.docker.internal",
			AgentTokenEncrypted: "cipher-" + name,
			DBPasswordEncrypted: "db-" + name,
		}); err != nil {
			t.Fatalf("reserve project runtime %s: %v", name, err)
		}
	}

	_, err := store.ReserveProjectRuntime("gamma", ProjectRuntimeReservation{
		MinSSHPort:          2300,
		MaxSSHPort:          2301,
		MinAgentPort:        2400,
		MaxAgentPort:        2401,
		AgentListenPort:     8081,
		AgentBaseURLHost:    "host.docker.internal",
		AgentTokenEncrypted: "cipher-gamma",
		DBPasswordEncrypted: "db-gamma",
	})
	if !errors.Is(err, ErrNoAvailableRuntimePort) {
		t.Fatalf("expected ErrNoAvailableRuntimePort, got %v", err)
	}
}

func TestProjectRuntimeFollowsProjectRename(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.CreateProject("alpha", "desc", nil); err != nil {
		t.Fatalf("create project: %v", err)
	}
	runtime, err := store.ReserveProjectRuntime("alpha", ProjectRuntimeReservation{
		MinSSHPort:          2300,
		MaxSSHPort:          2305,
		MinAgentPort:        2400,
		MaxAgentPort:        2405,
		AgentListenPort:     8081,
		AgentBaseURLHost:    "host.docker.internal",
		AgentTokenEncrypted: "cipher",
		DBPasswordEncrypted: "db",
	})
	if err != nil {
		t.Fatalf("reserve runtime: %v", err)
	}

	if _, err := store.UpdateProject("alpha", "beta", "renamed", nil); err != nil {
		t.Fatalf("rename project: %v", err)
	}

	updated, err := store.GetProjectRuntime("beta")
	if err != nil {
		t.Fatalf("get runtime by renamed project: %v", err)
	}
	if updated.ID != runtime.ID {
		t.Fatalf("expected runtime identity to survive rename, got old=%d new=%d", runtime.ID, updated.ID)
	}
}

func TestDeleteProjectCascadeRemovesProjectRuntime(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.CreateProject("alpha", "desc", nil); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.ReserveProjectRuntime("alpha", ProjectRuntimeReservation{
		MinSSHPort:          2300,
		MaxSSHPort:          2305,
		MinAgentPort:        2400,
		MaxAgentPort:        2405,
		AgentListenPort:     8081,
		AgentBaseURLHost:    "host.docker.internal",
		AgentTokenEncrypted: "cipher",
		DBPasswordEncrypted: "db",
	}); err != nil {
		t.Fatalf("reserve runtime: %v", err)
	}

	if err := store.DeleteProjectCascade("alpha"); err != nil {
		t.Fatalf("delete project cascade: %v", err)
	}

	if _, err := store.GetProjectRuntime("alpha"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected project runtime to be deleted, got %v", err)
	}
}

func TestUpdateProjectRuntimeStatus(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.CreateProject("alpha", "desc", nil); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := store.ReserveProjectRuntime("alpha", ProjectRuntimeReservation{
		MinSSHPort:          2300,
		MaxSSHPort:          2305,
		MinAgentPort:        2400,
		MaxAgentPort:        2405,
		AgentListenPort:     8081,
		AgentBaseURLHost:    "host.docker.internal",
		AgentTokenEncrypted: "cipher",
		DBPasswordEncrypted: "db",
	}); err != nil {
		t.Fatalf("reserve runtime: %v", err)
	}

	now := time.Now()
	status := RuntimeStatusProvisioning
	errText := "waiting for container health"
	agentBaseURL := "http://wrssh-runtime-p1-alpha:8081"
	updated, err := store.UpdateProjectRuntimeStatus("alpha", ProjectRuntimeStatusPatch{
		Status:            &status,
		AgentBaseURL:      &agentBaseURL,
		LastError:         &errText,
		LastProvisionedAt: &now,
	})
	if err != nil {
		t.Fatalf("update runtime status: %v", err)
	}
	if updated.Status != RuntimeStatusProvisioning || updated.LastError != errText || updated.AgentBaseURL != agentBaseURL {
		t.Fatalf("unexpected updated runtime: %+v", updated)
	}
}
