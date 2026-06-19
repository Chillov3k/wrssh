package users

import "testing"

func TestNormaliseClientHostnameStripsWindowsDomain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "domain backslash", in: `corp\a_green.agreen`, want: "a_green.agreen"},
		{name: "fqdn domain backslash", in: `corp.local\a_green.agreen`, want: "a_green.agreen"},
		{name: "domain slash", in: "corp/a_green.agreen", want: "a_green.agreen"},
		{name: "plain", in: "a_green.agreen", want: "a_green.agreen"},
		{name: "unsafe chars", in: `corp\a green.AGREEN`, want: "a.green.agreen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormaliseClientHostname(tt.in); got != tt.want {
				t.Fatalf("NormaliseClientHostname(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
