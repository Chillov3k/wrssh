//go:build linux

package subsystems

import (
	"context"
	"fmt"
	"strconv"
	"syscall"
)

type setuidModule struct{}

func newSetuidModule() Module {
	return &setuidModule{}
}

func (su *setuidModule) Manifest() Manifest {
	return Manifest{
		Name:        "setuid",
		Description: "Set the client process UID.",
		Usage:       "setuid <uid>",
		Dangerous:   true,
		Platforms:   []string{"linux"},
		Limits: ModuleLimits{
			TimeoutSeconds: 5,
			OutputBytes:    16 * 1024,
			MaxArgs:        1,
		},
	}
}

func (su *setuidModule) Run(_ context.Context, io ModuleIO, args []string) error {
	if len(args) != 1 {
		fmt.Fprintf(io, "setuid only takes one argument, the uid to set rssh to.")
		return nil
	}

	uid, err := strconv.Atoi(args[0])

	if err != nil {
		fmt.Fprintf(io, "%s", err.Error())
		return nil
	}

	return syscall.Setuid(uid)
}
