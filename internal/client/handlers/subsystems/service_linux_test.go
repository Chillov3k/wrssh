//go:build linux

package subsystems

import (
	"strings"
	"testing"
)

func TestServiceUnitName(t *testing.T) {
	unitName, err := serviceUnitName("salt-updater")
	if err != nil {
		t.Fatalf("serviceUnitName returned error: %v", err)
	}
	if unitName != "salt-updater.service" {
		t.Fatalf("unit name = %q", unitName)
	}

	unitName, err = serviceUnitName("salt-updater.service")
	if err != nil {
		t.Fatalf("serviceUnitName with suffix returned error: %v", err)
	}
	if unitName != "salt-updater.service" {
		t.Fatalf("unit name with suffix = %q", unitName)
	}
}

func TestSystemdUnitContents(t *testing.T) {
	unit := systemdUnitContents("/opt/salt-updater")
	for _, expected := range []string{
		"Description=burunya",
		"After=network.target",
		"Type=simple",
		`ExecStart="/opt/salt-updater" --foreground`,
		"Restart=always",
		"RestartSec=120",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, expected) {
			t.Fatalf("unit does not contain %q:\n%s", expected, unit)
		}
	}
}
