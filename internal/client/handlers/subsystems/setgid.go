//go:build linux

package subsystems

import (
	"context"
	"fmt"
	"strconv"
	"syscall"
)

type setgidModule struct{}

func newSetgidModule() Module {
	return &setgidModule{}
}

func (su *setgidModule) Manifest() Manifest {
	return Manifest{
		Name:        "setgid",
		Description: "Set the client process GID.",
		Usage:       "setgid <gid>",
		Dangerous:   true,
		Platforms:   []string{"linux"},
		Limits: ModuleLimits{
			TimeoutSeconds: 5,
			OutputBytes:    16 * 1024,
			MaxArgs:        1,
		},
	}
}

func (su *setgidModule) Run(_ context.Context, io ModuleIO, args []string) error {
	if len(args) != 1 {
		fmt.Fprintf(io, "setgid only takes one argument, the uid to set rssh to.")
		return nil
	}

	gid, err := strconv.Atoi(args[0])

	if err != nil {
		fmt.Fprintf(io, "%s", err.Error())
		return nil
	}

	return syscall.Setgid(gid)
}
