package rssh

import "testing"

func TestCleanRemotePathPreservesWindowsDrivePaths(t *testing.T) {
	tests := map[string]string{
		"C:/":                   "C:/",
		"c:\\Windows\\System32": "C:/Windows/System32",
		"C:/Temp/../Windows":    "C:/Windows",
		"/d:/Data":              "D:/Data",
		"relative/path":         "/relative/path",
		"":                      "/",
	}

	for input, want := range tests {
		if got := CleanRemotePath(input); got != want {
			t.Fatalf("CleanRemotePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestJoinRemotePathPreservesWindowsDrivePaths(t *testing.T) {
	tests := []struct {
		parent string
		name   string
		want   string
	}{
		{parent: "C:/", name: "Windows", want: "C:/Windows"},
		{parent: "C:/Temp", name: "file.txt", want: "C:/Temp/file.txt"},
		{parent: "c:\\Temp", name: "file.txt", want: "C:/Temp/file.txt"},
		{parent: "/", name: "etc", want: "/etc"},
	}

	for _, test := range tests {
		if got := JoinRemotePath(test.parent, test.name); got != test.want {
			t.Fatalf("JoinRemotePath(%q, %q) = %q, want %q", test.parent, test.name, got, test.want)
		}
	}
}
