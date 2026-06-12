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

func TestExpandHomeRemotePath(t *testing.T) {
	tests := []struct {
		remotePath string
		home       string
		want       string
		wantOK     bool
	}{
		{remotePath: "~", home: "/home/alice", want: "/home/alice", wantOK: true},
		{remotePath: "~/.ssh", home: "/home/alice", want: "/home/alice/.ssh", wantOK: true},
		{remotePath: "~/Documents/file.txt", home: "C:/Users/Alice", want: "C:/Users/Alice/Documents/file.txt", wantOK: true},
		{remotePath: "/etc/ssh", home: "/home/alice", want: "", wantOK: false},
	}

	for _, test := range tests {
		got, ok := expandHomeRemotePath(test.remotePath, test.home)
		if ok != test.wantOK {
			t.Fatalf("expandHomeRemotePath(%q, %q) ok = %v, want %v", test.remotePath, test.home, ok, test.wantOK)
		}
		if got != test.want {
			t.Fatalf("expandHomeRemotePath(%q, %q) = %q, want %q", test.remotePath, test.home, got, test.want)
		}
	}
}
