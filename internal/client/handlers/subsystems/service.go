package subsystems

import (
	"context"
	"errors"
	"os"

	"github.com/NHAS/reverse_ssh/internal/terminal"
)

type serviceModule struct{}

func newServiceModule() Module {
	return &serviceModule{}
}

func (s *serviceModule) Manifest() Manifest {
	return Manifest{
		Name:        "service",
		Description: serviceDescription(),
		Usage:       serviceUsage(),
		Dangerous:   true,
		Platforms:   servicePlatforms(),
		Limits: ModuleLimits{
			TimeoutSeconds: 30,
			OutputBytes:    64 * 1024,
			MaxArgs:        serviceMaxArgs(),
		},
	}
}

func (s *serviceModule) Run(ctx context.Context, _ ModuleIO, args []string) error {
	line := terminal.ParseLine("service "+joinArgsForParse(args), 0)

	name, err := line.GetArgString("name")
	if err == terminal.ErrFlagNotSet {
		name = defaultServiceName()
	}

	installPath, err := line.GetArgString("install")
	if err != terminal.ErrFlagNotSet {
		resolvedPath, resolveErr := resolveServiceInstallPath(installPath, err)
		if resolveErr != nil {
			return resolveErr
		}
		return s.installService(ctx, name, resolvedPath)
	}

	if line.IsSet("uninstall") {
		return s.uninstallService(ctx, name)
	}

	return errors.New(terminal.MakeHelpText(
		map[string]string{
			"install":   "Install this client as an OS service; optional path copies the current executable there first",
			"name":      "Service name; defaults to the platform default",
			"uninstall": "Uninstall the service",
		},
		serviceUsage(),
		serviceHelpDescription(),
	))
}

func resolveServiceInstallPath(requestedPath string, requestErr error) (string, error) {
	currentPath, err := os.Executable()
	if err != nil {
		return "", errors.New("Unable to find the current binary location: " + err.Error())
	}

	if requestErr != nil {
		return currentPath, nil
	}
	if requestedPath == currentPath {
		return currentPath, nil
	}

	input, err := os.ReadFile(currentPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(requestedPath, input, serviceCopyMode()); err != nil {
		return "", err
	}

	return requestedPath, nil
}
