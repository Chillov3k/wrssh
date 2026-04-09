package shellscripts

import (
	"strings"
	"testing"
)

func TestMakeTemplateShellQuotesDangerousValues(t *testing.T) {
	output, err := MakeTemplate(Args{
		Protocol:         "http",
		Host:             "127.0.0.1",
		Port:             "8080",
		Name:             `agent';touch /tmp/pwn;#`,
		WorkingDirectory: `/tmp/wrssh build`,
	}, "sh")
	if err != nil {
		t.Fatalf("MakeTemplate returned error: %v", err)
	}

	script := string(output)
	if !strings.Contains(script, `curl 'http://127.0.0.1:8080/agent'"'"';touch /tmp/pwn;#'`) {
		t.Fatalf("expected quoted download URL, got:\n%s", script)
	}
	if !strings.Contains(script, `download '/tmp/wrssh build'`) {
		t.Fatalf("expected quoted working directory, got:\n%s", script)
	}
}

func TestMakeTemplatePythonQuotesDangerousValues(t *testing.T) {
	output, err := MakeTemplate(Args{
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     "8080",
		Name:     "agent\"';touch",
		OS:       "linux",
		Arch:     "amd64",
	}, "py")
	if err != nil {
		t.Fatalf("MakeTemplate returned error: %v", err)
	}

	script := string(output)
	if !strings.Contains(script, `requests.get("http://127.0.0.1:8080/agent\"';touch")`) {
		t.Fatalf("expected quoted Python URL, got:\n%s", script)
	}
	if !strings.Contains(script, `with open("agent\"';touch", 'wb')`) {
		t.Fatalf("expected quoted Python filename, got:\n%s", script)
	}
}

func TestMakeTemplatePowerShellQuotesDangerousValues(t *testing.T) {
	output, err := MakeTemplate(Args{
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     "8080",
		Name:     "agent'';Start-Process calc",
	}, "ps1")
	if err != nil {
		t.Fatalf("MakeTemplate returned error: %v", err)
	}

	script := string(output)
	if !strings.Contains(script, `$baseFileName = 'agent'''';Start-Process calc'`) {
		t.Fatalf("expected quoted PowerShell filename, got:\n%s", script)
	}
	if !strings.Contains(script, `$wc.Downloadfile('http://127.0.0.1:8080/agent'''';Start-Process calc', $fullPath)`) {
		t.Fatalf("expected quoted PowerShell URL, got:\n%s", script)
	}
}
