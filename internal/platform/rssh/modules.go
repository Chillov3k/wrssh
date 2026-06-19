package rssh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/NHAS/reverse_ssh/internal/server/users"
	"golang.org/x/crypto/ssh"
)

const (
	DefaultSubsystemTimeout     = 60 * time.Second
	MaxSubsystemTimeout         = 10 * time.Minute
	DefaultSubsystemOutputBytes = int64(1024 * 1024)
	MaxSubsystemOutputBytes     = int64(4 * 1024 * 1024)
)

type ModuleLimits struct {
	TimeoutSeconds int   `json:"timeoutSeconds,omitempty"`
	OutputBytes    int64 `json:"outputBytes,omitempty"`
	StdinBytes     int64 `json:"stdinBytes,omitempty"`
	MaxArgs        int   `json:"maxArgs,omitempty"`
}

type ModuleManifest struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Version     string       `json:"version,omitempty"`
	Usage       string       `json:"usage,omitempty"`
	BuildTags   []string     `json:"buildTags,omitempty"`
	Platforms   []string     `json:"platforms,omitempty"`
	Dangerous   bool         `json:"dangerous,omitempty"`
	Disabled    bool         `json:"disabled,omitempty"`
	Limits      ModuleLimits `json:"limits,omitempty"`
}

type SubsystemExecutionOptions struct {
	Timeout          time.Duration
	OutputLimitBytes int64
}

type SubsystemExecution struct {
	Output    string `json:"output"`
	TimedOut  bool   `json:"timedOut,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

func (s *Service) ListModulesOnConnection(ctx context.Context, connectionID string) ([]ModuleManifest, error) {
	result, err := s.ExecuteSubsystemOnConnection(ctx, connectionID, "list", []string{"--json"}, nil, SubsystemExecutionOptions{
		Timeout:          10 * time.Second,
		OutputLimitBytes: 256 * 1024,
	})
	if err != nil {
		return nil, err
	}

	var manifests []ModuleManifest
	if err := json.Unmarshal([]byte(result.Output), &manifests); err != nil {
		return nil, fmt.Errorf("module list returned invalid json: %w", err)
	}
	return manifests, nil
}

func (s *Service) ExecuteSubsystemOnConnection(ctx context.Context, connectionID, module string, args []string, stdin io.Reader, opts SubsystemExecutionOptions) (SubsystemExecution, error) {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return SubsystemExecution{}, fmt.Errorf("connection id is required")
	}
	client, ok := users.GetClientConnection(connectionID)
	if !ok {
		return SubsystemExecution{}, fmt.Errorf("host is not currently connected")
	}

	return s.executeSubsystem(ctx, client, module, args, stdin, opts)
}

func (s *Service) executeSubsystem(ctx context.Context, client *ssh.ServerConn, module string, args []string, stdin io.Reader, opts SubsystemExecutionOptions) (SubsystemExecution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return SubsystemExecution{}, fmt.Errorf("host is not currently connected")
	}

	module = strings.TrimSpace(module)
	if module == "" {
		return SubsystemExecution{}, fmt.Errorf("module is required")
	}
	if strings.ContainsAny(module, " \t\r\n") {
		return SubsystemExecution{}, fmt.Errorf("module name must not contain whitespace")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultSubsystemTimeout
	}
	if opts.Timeout > MaxSubsystemTimeout {
		return SubsystemExecution{}, fmt.Errorf("subsystem timeout exceeds maximum of %s", MaxSubsystemTimeout)
	}
	if opts.OutputLimitBytes <= 0 {
		opts.OutputLimitBytes = DefaultSubsystemOutputBytes
	}
	if opts.OutputLimitBytes > MaxSubsystemOutputBytes {
		return SubsystemExecution{}, fmt.Errorf("subsystem output limit exceeds maximum of %d bytes", MaxSubsystemOutputBytes)
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	channel, requests, err := client.OpenChannel("session", nil)
	if err != nil {
		return SubsystemExecution{}, err
	}
	defer channel.Close()
	go ssh.DiscardRequests(requests)

	requestLine := subsystemRequestLine(module, args)
	okReply, err := channel.SendRequest("subsystem", true, ssh.Marshal(struct {
		Name string
	}{Name: requestLine}))
	if err != nil {
		return SubsystemExecution{}, err
	}
	if !okReply {
		return SubsystemExecution{}, fmt.Errorf("client refused subsystem request")
	}

	if stdin != nil {
		go func() {
			_, _ = io.Copy(channel, stdin)
			_ = channel.CloseWrite()
		}()
	}

	output := newLimitedOutput(opts.OutputLimitBytes)
	done := make(chan error, 1)
	go func() {
		var (
			stdoutErr error
			stderrErr error
			wg        sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, stdoutErr = io.Copy(output, channel)
			if stdoutErr == io.EOF {
				stdoutErr = nil
			}
		}()
		go func() {
			defer wg.Done()
			_, stderrErr = io.Copy(output, channel.Stderr())
			if stderrErr == io.EOF {
				stderrErr = nil
			}
		}()
		wg.Wait()

		switch {
		case stdoutErr != nil:
			done <- stdoutErr
		case stderrErr != nil:
			done <- stderrErr
		default:
			done <- nil
		}
	}()

	select {
	case copyErr := <-done:
		execution := SubsystemExecution{
			Output:    output.String(),
			Truncated: output.Truncated(),
		}
		if copyErr != nil {
			return execution, copyErr
		}
		if execution.Truncated {
			return execution, fmt.Errorf("module output exceeded %d bytes", opts.OutputLimitBytes)
		}
		return execution, nil
	case <-ctx.Done():
		_ = channel.Close()
		return SubsystemExecution{
			Output:    output.String(),
			TimedOut:  true,
			Truncated: output.Truncated(),
		}, fmt.Errorf("module timed out after %s", opts.Timeout.Round(time.Second))
	}
}

func subsystemRequestLine(module string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, module)
	for _, arg := range args {
		parts = append(parts, quoteSubsystemArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteSubsystemArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\r\n\\\"'") {
		return arg
	}

	var b strings.Builder
	b.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

type limitedOutput struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int64
	truncated bool
}

func newLimitedOutput(limit int64) *limitedOutput {
	return &limitedOutput{remaining: limit}
}

func (o *limitedOutput) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.remaining <= 0 {
		o.truncated = true
		return len(p), nil
	}
	allowed := int64(len(p))
	if allowed > o.remaining {
		allowed = o.remaining
		o.truncated = true
	}
	if allowed > 0 {
		_, _ = o.buffer.Write(p[:allowed])
		o.remaining -= allowed
	}
	return len(p), nil
}

func (o *limitedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func (o *limitedOutput) Truncated() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.truncated
}
