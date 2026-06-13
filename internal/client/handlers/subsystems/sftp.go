package subsystems

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/pkg/sftp"
)

type sftpModule struct{}

func newSFTPModule() Module {
	return &sftpModule{}
}

func (s *sftpModule) Manifest() Manifest {
	return Manifest{
		Name:        "sftp",
		Description: "Serve an SFTP session over the SSH channel.",
		Usage:       "sftp",
		Limits: ModuleLimits{
			TimeoutSeconds: -1,
			OutputBytes:    -1,
			MaxArgs:        0,
		},
	}
}

func (s *sftpModule) Run(_ context.Context, moduleIO ModuleIO, _ []string) error {
	options := make([]sftp.ServerOption, 0, 1)
	if runtime.GOOS != "windows" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			options = append(options, sftp.WithServerWorkingDirectory(home))
		}
	}

	server, err := sftp.NewServer(moduleIO, options...)
	if err != nil {
		return err
	}

	err = server.Serve()
	if err != io.EOF && err != nil {
		return fmt.Errorf("sftp server had an error: %s", err.Error())
	}

	return nil
}
