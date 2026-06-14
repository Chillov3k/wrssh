//go:build windows

package subsystems

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/NHAS/reverse_ssh/internal/terminal"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

type serviceModule struct{}

func newServiceModule() Module {
	return &serviceModule{}
}

func (s *serviceModule) Manifest() Manifest {
	return Manifest{
		Name:        "service",
		Description: "Install or remove this Windows client as the rssh service.",
		Usage:       "service (--install [path] | --uninstall)",
		Dangerous:   true,
		Platforms:   []string{"windows"},
		Limits: ModuleLimits{
			TimeoutSeconds: 30,
			OutputBytes:    64 * 1024,
			MaxArgs:        6,
		},
	}
}

func (s *serviceModule) Run(_ context.Context, _ ModuleIO, args []string) error {
	line := terminal.ParseLine("service "+joinArgsForParse(args), 0)

	name, err := line.GetArgString("name")
	if err == terminal.ErrFlagNotSet {
		name = "rssh"
	}

	installPath, err := line.GetArgString("install")
	if err != terminal.ErrFlagNotSet {
		flagErr := err

		currentPath, err := os.Executable()
		if err != nil {
			return errors.New("Unable to find the current binary location: " + err.Error())
		}

		//If no argument was supplied for install
		if flagErr != nil {
			installPath = currentPath

		} else if installPath != currentPath {

			input, err := ioutil.ReadFile(currentPath)
			if err != nil {
				return err
			}

			err = ioutil.WriteFile(installPath, input, 0644)
			if err != nil {
				return err
			}

		}

		return s.installService(name, installPath)
	}

	if line.IsSet("uninstall") {
		return s.uninstallService(name)
	}

	return errors.New(terminal.MakeHelpText(
		map[string]string{
			"install":   "Install this client as the default rssh service; optional path copies the current executable there first",
			"uninstall": "Uninstall the default rssh service",
		},
		"service (--install [path] | --uninstall)",
		"The service submodule installs or removes the rssh Windows service.",
	))
}

func (s *serviceModule) installService(name, location string) error {

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
	err = eventlog.InstallAsEventCreate(name, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		newService.Delete()
		return fmt.Errorf("SetupEventLogSource() failed: %s", err)
	}

	err = newService.Start()
	if err != nil {
		return fmt.Errorf("Starting rssh has failed: %s", err)
	}
	return nil

}

func (s *serviceModule) uninstallService(name string) error {
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
	err = serviceToRemove.Delete()
	if err != nil {
		return err
	}

	eventlog.Remove(name)
	return nil

}
