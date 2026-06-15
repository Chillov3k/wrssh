//go:build !linux && !windows

package subsystems

import (
	"context"
	"os"
	"runtime"
)

func defaultServiceName() string {
	return "salt-updater"
}

func serviceDescription() string {
	return "Install or remove this client as an OS service."
}

func serviceUsage() string {
	return "service (--install [path] | --uninstall) [--name name]"
}

func servicePlatforms() []string {
	return []string{"windows", "linux"}
}

func serviceMaxArgs() int {
	return 8
}

func serviceHelpDescription() string {
	return "The service submodule installs or removes the client as an OS service."
}

func serviceCopyMode() os.FileMode {
	return 0755
}

func (s *serviceModule) installService(context.Context, string, string) error {
	return unsupportedServicePlatform()
}

func (s *serviceModule) uninstallService(context.Context, string) error {
	return unsupportedServicePlatform()
}

func unsupportedServicePlatform() error {
	return &platformUnsupportedError{platform: runtime.GOOS}
}

type platformUnsupportedError struct {
	platform string
}

func (e *platformUnsupportedError) Error() string {
	return "service module is not supported on " + e.platform
}
