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
		Description: "TCP/UDP scanner with HTTP/HTTPS metadata probing on TCP open ports.",
		Version:     "1",
		Usage:       "pscan -h <host,ip,cidr,...> [-p <port,range,all>] [--udp|--tcp --udp] [-t workers] [-time timeout] [--max-duration duration] [--json]",
		BuildTags:   []string{"pscan"},
		Limits: subsystems.ModuleLimits{
			TimeoutSeconds: -1,
			OutputBytes:    1024 * 1024,
			MaxArgs:        20,
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
	endpoint := fmt.Sprintf("%s:%d", result.IP, result.Port)
	if result.Protocol == engine.ProtocolUDP {
		endpoint = fmt.Sprintf("%s:%d/udp", result.IP, result.Port)
	}
	state := result.State
	if state == "" {
		state = engine.StateOpen
	}
	parts := []string{endpoint, state}
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
	if result.NetBIOS != nil {
		if result.NetBIOS.Hostname != "" {
			parts = append(parts, fmt.Sprintf("netbios-host=%q", result.NetBIOS.Hostname))
		}
		if result.NetBIOS.Domain != "" {
			parts = append(parts, fmt.Sprintf("netbios-domain=%q", result.NetBIOS.Domain))
		}
		if result.NetBIOS.MAC != "" {
			parts = append(parts, fmt.Sprintf("netbios-mac=%q", result.NetBIOS.MAC))
		}
	}
	return strings.Join(parts, " ")
}
