//go:build execass && windows

package main

import (
	"os"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems/execass/engine"
)

func runExecassHelperIfRequested() bool {
	if len(os.Args) < 2 || os.Args[1] != engine.HelperArg {
		return false
	}
	os.Exit(engine.RunHelper(os.Stdin, os.Stdout, os.Stderr))
	return true
}
