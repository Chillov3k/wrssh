//go:build execass && !windows

package engine

import (
	"context"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

type platformRunner struct{}

func newRunner() Runner {
	return platformRunner{}
}

func (platformRunner) Run(context.Context, Request, subsystems.ModuleIO) error {
	return ErrUnsupported
}
