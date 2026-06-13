//go:build execass

package execass

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"runtime"
	"testing"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
)

type testIO struct {
	data []byte
}

func (t *testIO) Read(p []byte) (int, error) {
	if len(t.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, t.data)
	t.data = t.data[n:]
	return n, nil
}

func (t *testIO) Write(p []byte) (int, error) {
	return len(p), nil
}

func (t *testIO) Close() error {
	return nil
}

func (t *testIO) Stderr() io.Writer {
	return anyWriter{}
}

type anyWriter struct{}

func (anyWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func TestExecassUnsupportedOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows unsupported behavior")
	}
	artifact := []byte("stub artifact")
	sum := sha256.Sum256(artifact)
	err := New().Run(context.Background(), &testIO{data: artifact}, []string{
		"--stdin",
		"--sha256", hex.EncodeToString(sum[:]),
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Run error = %v, want ErrUnsupported", err)
	}
}

func TestParseRequestExecassFlags(t *testing.T) {
	request, err := ParseRequest([]string{
		"--stdin",
		"--sha256", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"--in-process",
		"--runtime", "v4.0.30319",
		"--args", `--name "Alice Example"`,
		"--timeout", "15s",
		"--output-limit", "4096",
	})
	if err != nil {
		t.Fatalf("ParseRequest returned error: %v", err)
	}
	if !request.UseStdin || !request.InProcess {
		t.Fatalf("request flags not preserved: %+v", request)
	}
	if request.Runtime != "v4.0.30319" {
		t.Fatalf("Runtime = %q", request.Runtime)
	}
	if request.AssemblyArgs != `--name "Alice Example"` {
		t.Fatalf("AssemblyArgs = %q", request.AssemblyArgs)
	}
	if request.Timeout.String() != "15s" || request.OutputBytes != 4096 {
		t.Fatalf("limits = %s/%d", request.Timeout, request.OutputBytes)
	}
}

func TestParseRequestOutOfProcessFlags(t *testing.T) {
	request, err := ParseRequest([]string{
		"--stdin",
		"--process", "cmd.exe",
		"--process-args", `/c echo ok`,
		"--ppid", "1234",
	})
	if err != nil {
		t.Fatalf("ParseRequest returned error: %v", err)
	}
	if request.InProcess {
		t.Fatalf("InProcess = true")
	}
	if request.ProcessName != "cmd.exe" || request.ProcessArgs != `/c echo ok` || request.ParentPID != 1234 {
		t.Fatalf("process fields not preserved: %+v", request)
	}
}

func TestParseRequestRejectsNegativePPID(t *testing.T) {
	_, err := ParseRequest([]string{"--stdin", "--ppid", "-1"})
	if err == nil {
		t.Fatal("ParseRequest accepted negative --ppid")
	}
}

var _ subsystems.ModuleIO = (*testIO)(nil)
