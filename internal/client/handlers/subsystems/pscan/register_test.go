//go:build pscan

package pscan

import (
	"testing"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

func TestPscanRegistersWithSubsystemRegistry(t *testing.T) {
	module, ok := subsystems.Lookup("pscan")
	if !ok {
		t.Fatalf("pscan module was not registered")
	}
	if module.Manifest().Name != "pscan" {
		t.Fatalf("registered manifest name = %q", module.Manifest().Name)
	}
}
