//go:build execass

package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	MaxArtifactBytes   = int64(8 * 1024 * 1024)
	DefaultTimeout     = 30 * time.Second
	MaxTimeout         = 2 * time.Minute
	DefaultOutputBytes = int64(1024 * 1024)
	DefaultProcessName = "notepad.exe"
)

type Request struct {
	ArtifactPath string
	UseStdin     bool
	SHA256       string
	Timeout      time.Duration
	OutputBytes  int64
	Debug        bool
	Artifact     []byte

	InProcess    bool
	Runtime      string
	ProcessName  string
	ProcessArgs  string
	ParentPID    int
	AssemblyArgs string
}

func ParseRequest(args []string) (Request, error) {
	request := Request{
		Timeout:     DefaultTimeout,
		OutputBytes: DefaultOutputBytes,
	}
	timeoutRaw := DefaultTimeout.String()
	outputLimit := DefaultOutputBytes

	fs := flag.NewFlagSet("execass", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&request.ArtifactPath, "artifact", "", "artifact path on the client host")
	fs.BoolVar(&request.UseStdin, "stdin", false, "read artifact bytes from subsystem stdin")
	fs.StringVar(&request.SHA256, "sha256", "", "expected artifact SHA-256 hex digest")
	fs.StringVar(&timeoutRaw, "timeout", DefaultTimeout.String(), "maximum runner timeout")
	fs.Int64Var(&outputLimit, "output-limit", DefaultOutputBytes, "maximum output bytes")
	fs.BoolVar(&request.Debug, "debug", false, "print execass diagnostic messages")
	fs.BoolVar(&request.InProcess, "in-process", false, "execute a .NET assembly in the current process")
	fs.StringVar(&request.Runtime, "runtime", "v4", "CLR runtime to use")
	fs.StringVar(&request.ProcessName, "process", DefaultProcessName, "legacy process name flag; custom process injection is not supported")
	fs.StringVar(&request.ProcessArgs, "process-args", "", "legacy process arguments flag; custom process injection is not supported")
	fs.IntVar(&request.ParentPID, "ppid", 0, "legacy parent process ID flag; custom process injection is not supported")
	fs.StringVar(&request.AssemblyArgs, "args", "", "assembly arguments")
	if err := fs.Parse(args); err != nil {
		return Request{}, err
	}
	if fs.NArg() != 0 {
		return Request{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}

	request.ArtifactPath = strings.TrimSpace(request.ArtifactPath)
	request.Runtime = strings.TrimSpace(request.Runtime)
	request.ProcessName = strings.TrimSpace(request.ProcessName)
	request.ProcessArgs = strings.TrimSpace(request.ProcessArgs)
	if request.ArtifactPath != "" && request.UseStdin {
		return Request{}, fmt.Errorf("--artifact and --stdin are mutually exclusive")
	}
	if request.ArtifactPath == "" {
		request.UseStdin = true
	}
	if request.Runtime == "" {
		return Request{}, fmt.Errorf("--runtime must not be empty")
	}
	if !request.InProcess && request.ProcessName == "" {
		return Request{}, fmt.Errorf("--process must not be empty")
	}
	if request.ParentPID < 0 {
		return Request{}, fmt.Errorf("--ppid must be non-negative")
	}

	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil {
		return Request{}, fmt.Errorf("invalid --timeout: %w", err)
	}
	if timeout <= 0 {
		return Request{}, fmt.Errorf("--timeout must be positive")
	}
	if timeout > MaxTimeout {
		return Request{}, fmt.Errorf("--timeout exceeds maximum of %s", MaxTimeout)
	}
	request.Timeout = timeout

	if outputLimit <= 0 {
		return Request{}, fmt.Errorf("--output-limit must be positive")
	}
	if outputLimit > DefaultOutputBytes {
		return Request{}, fmt.Errorf("--output-limit exceeds maximum of %d bytes", DefaultOutputBytes)
	}
	request.OutputBytes = outputLimit

	request.SHA256 = strings.TrimSpace(strings.ToLower(request.SHA256))
	if request.SHA256 != "" {
		decoded, err := hex.DecodeString(request.SHA256)
		if err != nil || len(decoded) != sha256.Size {
			return Request{}, fmt.Errorf("--sha256 must be a SHA-256 hex digest")
		}
	}

	return request, nil
}

func (r *Request) LoadArtifact(stdin io.Reader) error {
	var (
		data []byte
		err  error
	)

	if r.UseStdin {
		data, err = readArtifact(stdin)
	} else {
		file, openErr := os.Open(r.ArtifactPath)
		if openErr != nil {
			return openErr
		}
		defer file.Close()
		data, err = readArtifact(file)
	}
	if err != nil {
		return err
	}
	if r.SHA256 != "" {
		sum := sha256.Sum256(data)
		if !bytes.Equal(sum[:], mustDecodeSHA256(r.SHA256)) {
			return fmt.Errorf("artifact SHA-256 mismatch")
		}
	}
	r.Artifact = data
	return nil
}

func readArtifact(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds maximum size of %d bytes", MaxArtifactBytes)
	}
	return data, nil
}

func mustDecodeSHA256(value string) []byte {
	decoded, _ := hex.DecodeString(value)
	return decoded
}
