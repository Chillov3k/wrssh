//go:build !pscan

package pscan

import (
	"context"
	"fmt"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func init() {
	subsystems.Register(New())
}

func (m *Module) Manifest() subsystems.Manifest {
	return subsystems.Manifest{
		Name:        "pscan",
		Description: "TCP connect scanner stub; rebuild this artifact with --pscan to enable scanning.",
		Version:     "stub",
		Usage:       "pscan requires an artifact built with --pscan",
		BuildTags:   []string{"pscan"},
		Limits: subsystems.ModuleLimits{
			TimeoutSeconds: 5,
			OutputBytes:    4096,
			MaxArgs:        16,
		},
	}
}

func (m *Module) Run(context.Context, subsystems.ModuleIO, []string) error {
	return fmt.Errorf("pscan is not compiled into this artifact; rebuild the client with --pscan and start that new payload")
}
