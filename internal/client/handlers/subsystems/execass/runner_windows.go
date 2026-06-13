//go:build execass && windows

package execass

import (
	"context"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

type platformRunner struct{}

func newRunner() runner {
	return platformRunner{}
}

func (platformRunner) Run(context.Context, Request, subsystems.ModuleIO) error {
	return ErrNotImplemented
}
