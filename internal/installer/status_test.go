package installer

import "testing"

func TestPathListContainsWindowsSemantics(t *testing.T) {
	got := pathListContains(`C:\Windows;C:\Users\Lee\AppData\Local\Programs\GitMake\;D:\Tools`, `c:\users\lee\appdata\local\programs\gitmake`, ";", true)
	if !got {
		t.Fatal("expected case-insensitive Windows PATH match")
	}
}

func TestPathListContainsRejectsPrefixOnly(t *testing.T) {
	got := pathListContains(`C:\Tools\GitMake-Old;C:\Windows`, `C:\Tools\GitMake`, ";", true)
	if got {
		t.Fatal("prefix-only path must not count as an exact PATH entry")
	}
}

func TestPathStatusHealthySignals(t *testing.T) {
	cases := []struct {
		name string
		s    PathStatus
		want bool
	}{
		{"missing binary", PathStatus{CommandAvailable: true}, false},
		{"resolved", PathStatus{InstalledBinary: true, CommandAvailable: true}, true},
		{"current installed copy", PathStatus{InstalledBinary: true, CurrentIsInstalledCopy: true}, true},
		{"user path registered", PathStatus{InstalledBinary: true, UserPathHasInstall: true}, true},
		{"process path registered", PathStatus{InstalledBinary: true, ProcessPathHasInstall: true}, true},
		{"binary only", PathStatus{InstalledBinary: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.Healthy(); got != tc.want {
				t.Fatalf("Healthy()=%v want %v", got, tc.want)
			}
		})
	}
}
