//go:build !pscan

package main

import (
	"strings"
	"testing"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

func TestPscanStubRegisteredWithoutBuildTag(t *testing.T) {
	module, ok := subsystems.Lookup("pscan")
	if !ok {
		t.Fatal("expected pscan stub to be registered without pscan build tag")
	}

	err := module.Run(t.Context(), nil, []string{"-h", "127.0.0.1"})
	if err == nil || !strings.Contains(err.Error(), "--pscan") {
		t.Fatalf("stub error = %v, want rebuild hint mentioning --pscan", err)
	}
}
