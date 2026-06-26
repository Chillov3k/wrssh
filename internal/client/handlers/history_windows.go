//go:build windows
// +build windows

package handlers

import "strings"

func noHistoryStartupCommand(command string) string {
	if !historySaveDisabled() {
		return ""
	}

	lower := strings.ToLower(command)
	if !strings.Contains(lower, "powershell") && !strings.Contains(lower, "pwsh") {
		return ""
	}

	return "try { Set-PSReadLineOption -HistorySaveStyle SaveNothing } catch {}\r\n"
}
