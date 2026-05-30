//go:build !windows
// +build !windows

package handlers

import "path/filepath"

func noHistoryShellArgs(command string, args []string) []string {
	if !historySaveDisabled() {
		return args
	}

	switch filepath.Base(command) {
	case "bash":
		return append([]string{"--noprofile", "--norc"}, args...)
	case "zsh", "csh", "tcsh":
		return append([]string{"-f"}, args...)
	case "fish":
		return append([]string{"--no-config"}, args...)
	default:
		return args
	}
}

func noHistoryStartupCommand(command string) string {
	return ""
}
