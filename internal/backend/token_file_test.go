package backend

import (
	"os"
	"testing"

	"spotmini-gui/internal/paths"
)

// The mode has to be repaired on an existing file, not just set on a new
// one: os.WriteFile applies perm only when it creates the file, so every
// install predating the 0600 change kept 0644 forever.
func TestSaveTokenRepairsAnExistingWorldReadableFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

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
