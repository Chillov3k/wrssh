package handlers

import "testing"

func TestNoHistoryEnvReplacesHistoryVariables(t *testing.T) {
	SetNoHistorySave(true)
	t.Cleanup(func() { SetNoHistorySave(false) })

	env := noHistoryEnv([]string{
		"PATH=/usr/bin",
		"HISTFILE=/home/user/.bash_history",
		"histSize=1000",
	})

	values := map[string]string{}
	for _, item := range env {
		key, value, ok := cutEnv(item)
		if ok {
			values[key] = value
		}
	}

	if values["PATH"] != "/usr/bin" {
		t.Fatalf("PATH was not preserved: %#v", env)
	}
	if values["HISTFILE"] != "/dev/null" {
		t.Fatalf("HISTFILE = %q, want /dev/null", values["HISTFILE"])
	}
	if values["HISTSIZE"] != "0" {
		t.Fatalf("HISTSIZE = %q, want 0", values["HISTSIZE"])
	}
}

func cutEnv(value string) (string, string, bool) {
	for i := 0; i < len(value); i++ {
		if value[i] == '=' {
			return value[:i], value[i+1:], true
		}
	}
	return "", "", false
}
