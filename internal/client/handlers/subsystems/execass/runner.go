//go:build execass

package execass

import (
	"context"
	"errors"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

var (
	ErrUnsupported    = errors.New("execass is unsupported on this platform")
	ErrNotImplemented = errors.New("execass runner is not implemented in this ticket")
)

type runner interface {
	Run(context.Context, Request, subsystems.ModuleIO) error
}
