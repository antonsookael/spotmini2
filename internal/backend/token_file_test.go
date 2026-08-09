package backend

import (
	"os"
	"runtime"
	"testing"

	"spotmini-gui/internal/paths"
)

// The mode has to be repaired on an existing file, not just set on a new
// one: os.WriteFile applies perm only when it creates the file, so every
// install predating the 0600 change kept 0644 forever.
func TestSaveTokenRepairsAnExistingWorldReadableFile(t *testing.T) {
	// Windows has no mode bits to repair: os.Chmod there sets only the
	// read-only flag, so a writable file always reads back as 0666 and
	// 0600 is not a state it can be in. The protection that matters on
	// Windows is the ACL inherited from %AppData%, which this can't
	// assert on - so the check belongs to the platforms where the mode
	// is real.
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not enforced on Windows")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	// os.UserConfigDir reads %AppData% on Windows and ignores both of
	// the above, so without this the test wrote its dummy token over the
	// real one in the developer's own profile - silently destroying a
	// working login every time the suite ran.
	t.Setenv("APPDATA", dir)

	path, err := paths.ConfigFile(tokenFile)
	if err != nil {
		t.Fatalf("resolving path: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatalf("seeding a 0644 file: %v", err)
	}

	saveToken(TokenResponse{AccessToken: "a", RefreshToken: "r"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("token file left at %#o, want 0600", mode)
	}
}
