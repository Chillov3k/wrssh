package commands

import "testing"

func TestBuildOptionHelpUsesOriginalLinkDescriptions(t *testing.T) {
	help := BuildOptionHelp()

	if help["garble"] != "Use garble to obfuscate the binary (requires garble to be installed)" {
		t.Fatalf("unexpected garble help: %q", help["garble"])
	}

	if help["upx"] != "Use upx to compress the final binary (requires upx to be installed)" {
		t.Fatalf("unexpected upx help: %q", help["upx"])
	}

	if help["lzma"] != "Use lzma compression for smaller binary at the cost of overhead at execution (requires upx flag to be set)" {
		t.Fatalf("unexpected lzma help: %q", help["lzma"])
	}

	if help["shared-object"] != "Generate shared object file" {
		t.Fatalf("unexpected shared-object help: %q", help["shared-object"])
	}

	if help["raw-download"] != "Download over raw TCP, outputs bash downloader rather than http" {
		t.Fatalf("unexpected raw-download help: %q", help["raw-download"])
	}

	if help["use-host-header"] != "Use HTTP Host header as callback address when generating download template (add .sh to your download urls and find out)" {
		t.Fatalf("unexpected use-host-header help: %q", help["use-host-header"])
	}

	if help["no-history-save"] != "Detach startup and reduce shell history persistence for commands run through this agent" {
		t.Fatalf("unexpected no-history-save help: %q", help["no-history-save"])
	}

	if help["busybox-fallback"] != "Embed a Linux BusyBox fallback for distroless targets where shell or common command executables are missing" {
		t.Fatalf("unexpected busybox-fallback help: %q", help["busybox-fallback"])
	}

	if help["pscan"] != "Compile the optional TCP connect scanner module into the client" {
		t.Fatalf("unexpected pscan help: %q", help["pscan"])
	}

	if help["execass"] != "Compile the optional Windows-only .NET execute-assembly module into the client" {
		t.Fatalf("unexpected execass help: %q", help["execass"])
	}
}
