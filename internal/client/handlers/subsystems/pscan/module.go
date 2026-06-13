//go:build pscan

package pscan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems/pscan/engine"
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Manifest() subsystems.Manifest {
	return subsystems.Manifest{
		Name:        "pscan",
		Description: "TCP connect scanner with lightweight web title collection on common web ports.",
		Version:     "1",
		Usage:       "pscan -h <host,ip,cidr,...> [-p <port,range,all>] [-t 600] [-time 3] [--json]",
		BuildTags:   []string{"pscan"},
		Limits: subsystems.ModuleLimits{
			TimeoutSeconds: int(engine.MaxScanDuration.Seconds()),
			OutputBytes:    1024 * 1024,
			MaxArgs:        16,
		},
	}
}

func (m *Module) Run(ctx context.Context, io subsystems.ModuleIO, args []string) error {
	cfg, err := engine.ParseArgs(args)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(io)
	return engine.Scan(ctx, cfg, func(result engine.Result) error {
		if cfg.JSON {
			return encoder.Encode(result)
		}
		if result.Open {
			_, err := fmt.Fprintln(io, formatOpenResult(result))
			return err
		}
		return nil
	})
}

func formatOpenResult(result engine.Result) string {
	parts := []string{fmt.Sprintf("%s:%d open", result.IP, result.Port)}
	if result.Web != nil {
		parts = append(parts, result.Web.Scheme)
		if result.Web.StatusCode > 0 {
			parts = append(parts, fmt.Sprintf("status=%d", result.Web.StatusCode))
		}
		if result.Web.Title != "" {
			parts = append(parts, fmt.Sprintf("title=%q", result.Web.Title))
		}
		if result.Web.Server != "" {
			parts = append(parts, fmt.Sprintf("server=%q", result.Web.Server))
		}
	}
	return strings.Join(parts, " ")
}
