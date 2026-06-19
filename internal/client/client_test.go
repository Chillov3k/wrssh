package client

import "testing"

func TestClientAccountNameStripsWindowsDomain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "domain backslash", in: `corp\a_green`, want: "a_green"},
		{name: "domain slash", in: "corp/a_green", want: "a_green"},
		{name: "upn", in: "a_green@corp.local", want: "a_green"},
		{name: "plain", in: "a_green", want: "a_green"},
		{name: "empty", in: " ", want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientAccountName(tt.in); got != tt.want {
				t.Fatalf("clientAccountName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
