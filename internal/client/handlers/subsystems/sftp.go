package subsystems

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/NHAS/reverse_ssh/internal/terminal"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type subSftp bool

func (s *subSftp) Execute(_ terminal.ParsedLine, connection ssh.Channel, subsystemReq *ssh.Request) error {
	options := make([]sftp.ServerOption, 0, 1)
	if runtime.GOOS != "windows" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			options = append(options, sftp.WithServerWorkingDirectory(home))
		}
	}

	server, err := sftp.NewServer(connection, options...)
	if err != nil {
		subsystemReq.Reply(false, []byte(err.Error()))
		return err
	}

	subsystemReq.Reply(true, nil)

	err = server.Serve()
	if err != io.EOF && err != nil {
		return fmt.Errorf("sftp server had an error: %s", err.Error())
	}

	return nil
}
