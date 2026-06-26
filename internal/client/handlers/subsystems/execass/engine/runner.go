//go:build execass

package engine

import (
	"context"
	"errors"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

var (
	ErrUnsupported = errors.New("execass is unsupported on this platform")
)

type Runner interface {
	Run(context.Context, Request, subsystems.ModuleIO) error
}

func NewRunner() Runner {
	return newRunner()
}
