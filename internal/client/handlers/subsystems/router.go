package subsystems

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/terminal"
	"golang.org/x/crypto/ssh"
)

func RunSubsystems(connection ssh.Channel, req *ssh.Request) error {
	var payload struct {
		Name string
	}
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		req.Reply(false, []byte("invalid subsystem payload"))
		return fmt.Errorf("invalid subsystem payload: %w", err)
	}

	line := terminal.ParseLine(payload.Name, 0)
	if line.Command == nil || strings.TrimSpace(line.Command.Value()) == "" {
		req.Reply(false, []byte("missing subsystem name"))
		return fmt.Errorf("missing subsystem name")
	}

	moduleName := line.Command.Value()
	module, ok := Lookup(moduleName)
	if !ok {
		req.Reply(false, []byte("Unknown subsystem"))
		return fmt.Errorf("unknown subsystem %q", moduleName)
	}

	args := []string{}
	if len(line.Chunks) > 1 {
		args = append(args, line.Chunks[1:]...)
	}

	manifest := module.Manifest()
	policy, err := enforcePolicy(context.Background(), manifest, args)
	if err != nil {
		req.Reply(false, []byte(err.Error()))
		audit(AuditEvent{
			Module:     manifest.Name,
			Args:       args,
			StartedAt:  time.Now(),
			FinishedAt: time.Now(),
			Success:    false,
			Error:      err.Error(),
			Dangerous:  manifest.Dangerous,
		})
		return err
	}

	ctx := context.Background()
	cancel := func() {}
	if !policy.NoTimeoutCap {
		ctx, cancel = context.WithTimeout(context.Background(), policy.Timeout)
	}
	defer cancel()

	moduleIO := NewModuleIO(connection, connection, connection, connection.Stderr())
	stdinLimit := policy.StdinBytes
	if policy.NoStdinCap {
		stdinLimit = 0
	}
	cappedIO, err := newCappedModuleIO(moduleIO, policy.OutputBytes, stdinLimit)
	if err != nil {
		req.Reply(false, []byte(err.Error()))
		return err
	}

	startedAt := time.Now()
	req.Reply(true, nil)

	runErr := module.Run(ctx, cappedIO, args)
	timedOut := ctx.Err() == context.DeadlineExceeded
	if runErr == nil && timedOut {
		runErr = fmt.Errorf("module %q timed out after %s", manifest.Name, policy.Timeout.Round(time.Second))
	}
	if runErr == nil && cappedIO.Truncated() {
		runErr = fmt.Errorf("module %q output exceeded %d bytes", manifest.Name, policy.OutputBytes)
	}

	finishedAt := time.Now()
	event := AuditEvent{
		Module:       manifest.Name,
		Args:         append([]string(nil), args...),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		Duration:     finishedAt.Sub(startedAt),
		Success:      runErr == nil,
		TimedOut:     timedOut,
		OutputCapped: cappedIO.Truncated(),
		Dangerous:    manifest.Dangerous,
	}
	if runErr != nil {
		event.Error = runErr.Error()
	}
	audit(event)

	return runErr
}
