package subsystems

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/NHAS/reverse_ssh/internal/terminal"
)

type listModule struct{}

func newListModule() Module {
	return &listModule{}
}

func (l *listModule) Manifest() Manifest {
	return Manifest{
		Name:        "list",
		Description: "List available client subsystem modules.",
		Usage:       "list [--json]",
		Limits: ModuleLimits{
			TimeoutSeconds: 5,
			OutputBytes:    128 * 1024,
			MaxArgs:        1,
		},
	}
}

func (l *listModule) Run(_ context.Context, io ModuleIO, args []string) error {
	line := terminal.ParseLine("list "+joinArgsForParse(args), 0)
	if line.IsSet("json") {
		encoder := json.NewEncoder(io)
		encoder.SetIndent("", "  ")
		return encoder.Encode(Manifests())
	}

	for _, name := range Names() {
		fmt.Fprintf(io, "%s\n", name)
	}
	return nil
}

func joinArgsForParse(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}
