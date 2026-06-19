package subsystems

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type testModuleIO struct {
	bytes.Buffer
}

func (t *testModuleIO) Close() error {
	return nil
}

func (t *testModuleIO) Stderr() io.Writer {
	return &t.Buffer
}

func TestListModuleTextIncludesCoreModules(t *testing.T) {
	module := newListModule()
	out := &testModuleIO{}
	if err := module.Run(context.Background(), out, nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	text := out.String()
	for _, name := range []string{"list\n", "sftp\n"} {
		if !strings.Contains(text, name) {
			t.Fatalf("list output %q does not contain %q", text, name)
		}
	}
}

func TestListModuleJSONIncludesManifests(t *testing.T) {
	module := newListModule()
	out := &testModuleIO{}
	if err := module.Run(context.Background(), out, []string{"--json"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var manifests []Manifest
	if err := json.Unmarshal(out.Bytes(), &manifests); err != nil {
		t.Fatalf("list --json output is not JSON: %v\n%s", err, out.String())
	}

	seen := map[string]bool{}
	for _, manifest := range manifests {
		seen[manifest.Name] = true
	}
	if !seen["list"] || !seen["sftp"] {
		t.Fatalf("manifests missing core modules: %+v", manifests)
	}
}
