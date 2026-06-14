//go:build execass

package engine

import "testing"

func TestParseRequestExecassFlags(t *testing.T) {
	request, err := ParseRequest([]string{
		"--stdin",
		"--sha256", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"--in-process",
		"--runtime", "v4.0.30319",
		"--args", `--name "Alice Example"`,
		"--timeout", "15s",
		"--output-limit", "4096",
		"--debug",
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
	if !request.Debug {
		t.Fatalf("Debug = false")
	}
}

func TestParseRequestDefaultsToStdin(t *testing.T) {
	request, err := ParseRequest(nil)
	if err != nil {
		t.Fatalf("ParseRequest returned error: %v", err)
	}
	if !request.UseStdin {
		t.Fatalf("UseStdin = false")
	}
	if request.ArtifactPath != "" {
		t.Fatalf("ArtifactPath = %q", request.ArtifactPath)
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
