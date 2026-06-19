//go:build execass

package execass

import (
	"context"
	"fmt"
	"runtime"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems/execass/engine"
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Manifest() subsystems.Manifest {
	return subsystems.Manifest{
		Name:        "execass",
		Description: "Windows-only .NET assembly runner; reads the artifact from stdin by default.",
		Version:     "1",
		Usage:       "execass [--artifact <client-path>] [--sha256 <digest>] [--args '<assembly args>'] [--in-process] [--runtime v4] [--timeout 30s] [--output-limit bytes] [--debug]",
		BuildTags:   []string{"execass"},
		Platforms:   []string{"windows"},
		Dangerous:   true,
		Disabled:    runtime.GOOS != "windows",
		Limits: subsystems.ModuleLimits{
			TimeoutSeconds: int(engine.MaxTimeout.Seconds()),
			OutputBytes:    engine.DefaultOutputBytes,
			StdinBytes:     engine.MaxArtifactBytes,
			MaxArgs:        10,
		},
	}
}

func (m *Module) Run(ctx context.Context, io subsystems.ModuleIO, args []string) error {
	request, err := engine.ParseRequest(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()

	if err := request.LoadArtifact(io); err != nil {
		return err
	}

	if len(request.Artifact) == 0 {
		return fmt.Errorf("artifact is empty")
	}

	return engine.NewRunner().Run(ctx, request, io)
}
