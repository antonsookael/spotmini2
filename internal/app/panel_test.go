package app

import "testing"

func TestSettingsToggleTarget(t *testing.T) {
	cases := []struct {
		open        string
		favoritesOn bool
		want        string
	}{
		{"", false, "settings"},
		{"", true, "favorites"},
		{"playlists", false, "settings"},
		{"playlists", true, "favorites"},
		// Anything reached through settings closes, whatever page it's on.
		{"settings", false, "settings"},
		{"settings", true, "settings"},
		{"hotkeys", false, "hotkeys"},
		{"stats", false, "stats"},
		{"favorites", true, "favorites"},
		{"favorites", false, "favorites"},
	}
	for _, tc := range cases {
		if got := settingsToggleTarget(tc.open, tc.favoritesOn); got != tc.want {
			t.Errorf("open %q, favorites on %v: toggles %q, want %q", tc.open, tc.favoritesOn, got, tc.want)
		}
	}
}
