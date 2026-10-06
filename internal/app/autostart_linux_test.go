//go:build linux

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopExecQuote(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"plain", "/usr/bin/spotmini", `"/usr/bin/spotmini"`},
		// The case that actually comes up: quoted, the space stays part
		// of one argument instead of splitting the path in two.
		{"space", "/home/me/My Apps/spotmini", `"/home/me/My Apps/spotmini"`},
		{"percent", "/opt/100%/spotmini", `"/opt/100%%/spotmini"`},
		// Each of these is escaped twice over: once as a quoted argument,
		// then again because that backslash sits in a string value.
		{"quote", `/opt/a"b/spotmini`, `"/opt/a\\"b/spotmini"`},
		{"dollar", "/opt/$HOME/spotmini", `"/opt/\\$HOME/spotmini"`},
		{"backtick", "/opt/a`b/spotmini", "\"/opt/a\\\\`b/spotmini\""},
		{"backslash", `/opt/a\b/spotmini`, `"/opt/a\\\\b/spotmini"`},
	}
	for _, tc := range cases {
		if got := desktopExecQuote(tc.path); got != tc.want {
			t.Errorf("%s: quoted %q as %s, want %s", tc.name, tc.path, got, tc.want)
		}
	}
}

func TestSetAutostartWritesAndRemovesTheEntry(t *testing.T) {
	// os.UserConfigDir reads this, so the entry lands in a temp dir
	// rather than switching autostart on for whoever runs the suite.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if isAutostartEnabled() {
		t.Fatal("reported enabled before anything was written")
	}

	if err := setAutostart(true); err != nil {
		t.Fatalf("enabling: %v", err)
	}
	if !isAutostartEnabled() {
		t.Fatal("reported disabled straight after enabling")
	}

	data, err := os.ReadFile(filepath.Join(dir, "autostart", autostartDesktopFile))
	if err != nil {
		t.Fatalf("reading the entry: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolving the executable: %v", err)
	}
	if want := "Exec=" + desktopExecQuote(exe) + "\n"; !strings.Contains(string(data), want) {
		t.Errorf("entry doesn't launch this binary - want a line %q in:\n%s", want, data)
	}

	if err := setAutostart(false); err != nil {
		t.Fatalf("disabling: %v", err)
	}
	if isAutostartEnabled() {
		t.Fatal("reported enabled straight after disabling")
	}

	// Already off is not a failure: the entry can be deleted by hand, and
	// the settings toggle still has to be able to say "off" afterwards.
	if err := setAutostart(false); err != nil {
		t.Errorf("disabling when already disabled: %v", err)
	}
}
