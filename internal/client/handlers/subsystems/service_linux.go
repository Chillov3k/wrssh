//go:build linux

package subsystems

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const systemdSystemDir = "/etc/systemd/system"

var systemdUnitNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]+$`)

func defaultServiceName() string {
	return "salt-updater"
}

func serviceDescription() string {
	return "Install or remove this Linux client as a systemd service."
}

func serviceUsage() string {
	return "service (--install [path] | --uninstall) [--name name]"
}

func servicePlatforms() []string {
	return []string{"linux"}
}

func serviceMaxArgs() int {
	return 8
}

func serviceHelpDescription() string {
	return "The service submodule installs or removes the client as a Linux systemd service."
}

func serviceCopyMode() os.FileMode {
	return 0755
}

func (s *serviceModule) installService(ctx context.Context, name, location string) error {
	unitName, err := serviceUnitName(name)
	if err != nil {
		return err
	}

	absoluteLocation, err := filepath.Abs(location)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absoluteLocation); err != nil {
		return err
	}

	unitPath := filepath.Join(systemdSystemDir, unitName)
	if _, err := os.Stat(unitPath); err == nil {
		return fmt.Errorf("service %s already exists", unitName)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.WriteFile(unitPath, []byte(systemdUnitContents(absoluteLocation)), 0644); err != nil {
		return err
	}

	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		_ = os.Remove(unitPath)
		return err
	}
	if err := runSystemctl(ctx, "enable", unitName); err != nil {
		_ = os.Remove(unitPath)
		_ = runSystemctl(ctx, "daemon-reload")
		return err
	}
	if err := runSystemctl(ctx, "start", unitName); err != nil {
		return err
	}

	return nil
}

func (s *serviceModule) uninstallService(ctx context.Context, name string) error {
	unitName, err := serviceUnitName(name)
	if err != nil {
		return err
	}

	unitPath := filepath.Join(systemdSystemDir, unitName)
	if _, err := os.Stat(unitPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("service %s is not installed", unitName)
		}
		return err
	}

	_ = runSystemctl(ctx, "stop", unitName)
	_ = runSystemctl(ctx, "disable", unitName)
	if err := os.Remove(unitPath); err != nil {
		return err
	}
	return runSystemctl(ctx, "daemon-reload")
}

func serviceUnitName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultServiceName()
	}
	name = strings.TrimSuffix(name, ".service")
	if name == "" || !systemdUnitNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid systemd service name %q", name)
	}
	return name + ".service", nil
}

func systemdUnitContents(execPath string) string {
	return fmt.Sprintf(`[Unit]
Description=burunya
After=network.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=120

[Install]
WantedBy=multi-user.target
`, systemdQuoteExecPath(execPath))
}

func systemdQuoteExecPath(execPath string) string {
	escaped := strings.ReplaceAll(execPath, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func runSystemctl(ctx context.Context, args ...string) error {
	output, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
