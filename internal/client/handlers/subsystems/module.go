package subsystems

import (
	"context"
	"io"
)

type Module interface {
	Manifest() Manifest
	Run(ctx context.Context, io ModuleIO, args []string) error
}

type subsystemModule struct {
	manifest Manifest
	run      func(context.Context, ModuleIO, []string) error
}

func (m subsystemModule) Manifest() Manifest {
	return m.manifest
}

func (m subsystemModule) Run(ctx context.Context, io ModuleIO, args []string) error {
	return m.run(ctx, io, args)
}

type ModuleIO interface {
	io.Reader
	io.Writer
	io.Closer
	Stderr() io.Writer
}
