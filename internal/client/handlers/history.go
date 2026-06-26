package handlers

import (
	"strings"
	"sync/atomic"
)

var noHistorySave atomic.Bool

func SetNoHistorySave(enabled bool) {
	noHistorySave.Store(enabled)
}

func historySaveDisabled() bool {
	return noHistorySave.Load()
}

func noHistoryEnv(env []string) []string {
	if !historySaveDisabled() {
		return env
	}

	settings := []struct {
		key   string
		value string
	}{
		{"HISTFILE", "/dev/null"},
		{"HISTSIZE", "0"},
		{"HISTFILESIZE", "0"},
		{"HISTCONTROL", "ignorespace:ignoredups"},
		{"HISTIGNORE", "*"},
		{"SAVEHIST", "0"},
		{"LESSHISTFILE", "/dev/null"},
		{"PYTHON_HISTORY", "/dev/null"},
		{"NODE_REPL_HISTORY", "/dev/null"},
		{"MYSQL_HISTFILE", "/dev/null"},
		{"PSQL_HISTORY", "/dev/null"},
		{"SQLITE_HISTORY", "/dev/null"},
		{"REDISCLI_HISTFILE", "/dev/null"},
		{"fish_history", ""},
	}

	disabled := make(map[string]struct{}, len(settings))
	for _, setting := range settings {
		disabled[strings.ToUpper(setting.key)] = struct{}{}
	}

	next := make([]string, 0, len(env)+len(settings))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, blocked := disabled[strings.ToUpper(key)]; blocked {
			continue
		}
		next = append(next, item)
	}
	for _, setting := range settings {
		next = append(next, setting.key+"="+setting.value)
	}
	return next
}
