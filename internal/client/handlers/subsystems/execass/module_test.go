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

var _ subsystems.ModuleIO = (*testIO)(nil)
