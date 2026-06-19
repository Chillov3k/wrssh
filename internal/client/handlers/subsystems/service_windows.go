//go:build windows

package subsystems

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func defaultServiceName() string {
	return "rssh"
}

func serviceDescription() string {
	return "Install or remove this Windows client as the rssh service."
}

func serviceUsage() string {
	return "service (--install [path] | --uninstall)"
}

func servicePlatforms() []string {
	return []string{"windows"}
}

func serviceMaxArgs() int {
	return 6
}

func serviceHelpDescription() string {
	return "The service submodule installs or removes the rssh Windows service."
}

func serviceCopyMode() os.FileMode {
	return 0644
}

func (s *serviceModule) installService(_ context.Context, name, location string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	newService, err := m.OpenService(name)
	if err == nil {
		newService.Close()
		return fmt.Errorf("service %s already exists", name)
	}

	newService, err = m.CreateService(name, location, mgr.Config{DisplayName: "", StartType: mgr.StartAutomatic})
	if err != nil {
		return err
	}
	defer newService.Close()

	if err := eventlog.InstallAsEventCreate(name, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		newService.Delete()
		return fmt.Errorf("SetupEventLogSource() failed: %s", err)
	}

	if err := newService.Start(); err != nil {
		return fmt.Errorf("Starting rssh has failed: %s", err)
	}
	return nil
}

func (s *serviceModule) uninstallService(_ context.Context, name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	serviceToRemove, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("service %s is not installed", name)
	}
	defer serviceToRemove.Close()

	if err := serviceToRemove.Delete(); err != nil {
		return err
	}

	eventlog.Remove(name)
	return nil
}
