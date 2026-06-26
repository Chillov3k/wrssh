package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/NHAS/reverse_ssh/internal/terminal"
	"github.com/NHAS/reverse_ssh/internal/terminal/autocomplete"
	"golang.org/x/crypto/ssh"
)

type moduleCommand struct{}

func (m *moduleCommand) ValidArgs() map[string]string {
	return map[string]string{
		"q":   "Quiet, no output",
		"raw": "Do not label output blocks with the client they came from",
	}
}

func (m *moduleCommand) Run(user *users.User, tty io.ReadWriter, line terminal.ParsedLine) error {
	if len(line.Arguments) < 2 {
		return fmt.Errorf("not enough arguments supplied. Usage: module <host-selector> <module-name> [module args...]")
	}

	filter := line.Arguments[0].Value()
	moduleLine := strings.TrimSpace(line.RawLine[line.Arguments[0].End():])
	if moduleLine == "" {
		return fmt.Errorf("module name is required")
	}
	parsedModuleLine := terminal.ParseLine(moduleLine, 0)
	if parsedModuleLine.Command == nil || strings.TrimSpace(parsedModuleLine.Command.Value()) == "" {
		return fmt.Errorf("module name is required")
	}

	matchingClients, err := user.SearchClients(filter)
	if err != nil {
		return err
	}
	if len(matchingClients) == 0 {
		return fmt.Errorf("unable to find match for %q", filter)
	}

	payload := ssh.Marshal(struct {
		Name string
	}{Name: moduleLine})

	for id, client := range matchingClients {
		if !(line.IsSet("q") || line.IsSet("raw")) {
			fmt.Fprint(tty, "\n\n")
			fmt.Fprintf(tty, "%s (%s) module output:\n", id, client.User()+"@"+client.RemoteAddr().String())
		}

		newChan, requests, err := client.OpenChannel("session", nil)
		if err != nil {
			if !line.IsSet("q") {
				fmt.Fprintf(tty, "Failed: %s\n", err)
			}
			continue
		}
		go ssh.DiscardRequests(requests)

		response, err := newChan.SendRequest("subsystem", true, payload)
		if err != nil {
			newChan.Close()
			if !line.IsSet("q") {
				fmt.Fprintf(tty, "Failed: %s\n", err)
			}
			continue
		}
		if !response {
			newChan.Close()
			if !line.IsSet("q") {
				fmt.Fprintf(tty, "Failed: client refused\n")
			}
			continue
		}

		if line.IsSet("q") {
			io.Copy(io.Discard, newChan)
			newChan.Close()
			continue
		}

		io.Copy(tty, newChan)
		newChan.Close()
	}

	fmt.Fprint(tty, "\n")
	return nil
}

func (m *moduleCommand) Expect(line terminal.ParsedLine) []string {
	return []string{autocomplete.RemoteId}
}

func (m *moduleCommand) Help(explain bool) string {
	if explain {
		return "Run a client subsystem module"
	}

	return terminal.MakeHelpText(m.ValidArgs(),
		"module [OPTIONS] host|filter module-name [module args...]",
		"Filter uses glob matching against target attributes. The module is sent as an SSH subsystem request.",
	)
}
