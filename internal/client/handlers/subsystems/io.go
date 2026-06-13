package subsystems

import (
	"fmt"
	"io"
	"sync"
)

type sshModuleIO struct {
	reader io.Reader
	writer io.Writer
	closer io.Closer
	stderr io.Writer
}

func NewModuleIO(reader io.Reader, writer io.Writer, closer io.Closer, stderr io.Writer) ModuleIO {
	if stderr == nil {
		stderr = writer
	}
	return &sshModuleIO{
		reader: reader,
		writer: writer,
		closer: closer,
		stderr: stderr,
	}
}

func (m *sshModuleIO) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

func (m *sshModuleIO) Write(p []byte) (int, error) {
	return m.writer.Write(p)
}

func (m *sshModuleIO) Close() error {
	if m.closer == nil {
		return nil
	}
	return m.closer.Close()
}

func (m *sshModuleIO) Stderr() io.Writer {
	return m.stderr
}

type cappedModuleIO struct {
	base     ModuleIO
	stdout   *cappedWriter
	stderr   *cappedWriter
	stderrIO io.Writer
}

func newCappedModuleIO(base ModuleIO, limit int64) (*cappedModuleIO, error) {
	if base == nil {
		return nil, fmt.Errorf("nil module io")
	}
	if limit <= 0 {
		return &cappedModuleIO{
			base:     base,
			stderrIO: base.Stderr(),
		}, nil
	}
	shared := &outputBudget{remaining: limit}
	stdout := &cappedWriter{writer: base, budget: shared}
	stderr := &cappedWriter{writer: base.Stderr(), budget: shared}
	return &cappedModuleIO{
		base:     base,
		stdout:   stdout,
		stderr:   stderr,
		stderrIO: stderr,
	}, nil
}

func (m *cappedModuleIO) Read(p []byte) (int, error) {
	return m.base.Read(p)
}

func (m *cappedModuleIO) Write(p []byte) (int, error) {
	if m.stdout == nil {
		return m.base.Write(p)
	}
	return m.stdout.Write(p)
}

func (m *cappedModuleIO) Close() error {
	return m.base.Close()
}

func (m *cappedModuleIO) Stderr() io.Writer {
	return m.stderrIO
}

func (m *cappedModuleIO) Truncated() bool {
	if m.stdout != nil && m.stdout.Truncated() {
		return true
	}
	return m.stderr != nil && m.stderr.Truncated()
}

type outputBudget struct {
	mu        sync.Mutex
	remaining int64
	truncated bool
}

type cappedWriter struct {
	writer io.Writer
	budget *outputBudget
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	w.budget.mu.Lock()
	if w.budget.remaining <= 0 {
		w.budget.truncated = true
		w.budget.mu.Unlock()
		return len(p), nil
	}

	allowed := int64(len(p))
	if allowed > w.budget.remaining {
		allowed = w.budget.remaining
		w.budget.truncated = true
	}
	w.budget.remaining -= allowed
	w.budget.mu.Unlock()

	if allowed == 0 {
		return len(p), nil
	}
	if _, err := w.writer.Write(p[:allowed]); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *cappedWriter) Truncated() bool {
	w.budget.mu.Lock()
	defer w.budget.mu.Unlock()
	return w.budget.truncated
}
