package subsystems

import (
	"strings"
	"testing"
)

func TestCappedModuleIOEnforcesStdinLimit(t *testing.T) {
	base := NewModuleIO(strings.NewReader("abcdef"), ioDiscardCloser{}, nil, nil)
	capped, err := newCappedModuleIO(base, 0, 3)
	if err != nil {
		t.Fatalf("newCappedModuleIO returned error: %v", err)
	}

	buf := make([]byte, 3)
	if n, err := capped.Read(buf); err != nil || n != 3 || string(buf) != "abc" {
		t.Fatalf("first read = %d/%q/%v, want 3/abc/nil", n, string(buf), err)
	}
	if _, err := capped.Read(buf); err == nil || !strings.Contains(err.Error(), "stdin exceeded") {
		t.Fatalf("second read error = %v, want stdin limit error", err)
	}
}

type ioDiscardCloser struct{}

func (ioDiscardCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (ioDiscardCloser) Close() error {
	return nil
}
