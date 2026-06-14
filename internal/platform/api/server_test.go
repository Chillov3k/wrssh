package api

import "testing"

func TestValidWindowsArtifactNameRequiresExeSuffix(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "", want: false},
		{name: "love", want: false},
		{name: "love.exe", want: true},
		{name: "LOVE.EXE", want: true},
		{name: " builds/love.exe ", want: true},
		{name: "love.exe.bak", want: false},
	}

	for _, test := range tests {
		if got := validWindowsArtifactName(test.name); got != test.want {
			t.Fatalf("validWindowsArtifactName(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}
