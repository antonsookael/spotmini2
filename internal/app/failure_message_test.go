package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"spotmini-gui/internal/playback"
)

func TestFailureMessageNamesEveryCause(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"no token yet", errNotSignedIn, "Signing in to Spotify..."},
		{"unreachable", fmt.Errorf("%w: dial tcp", playback.ErrUnreachable), "No connection to Spotify"},
		{"auth", playback.ErrAuthExpired, "Spotify sign-in expired - signing in again"},
		{"rate limited", playback.ErrRateLimited, "Too many requests - wait a moment"},
		{"down", playback.ErrSpotifyDown, "Spotify is down - try again later"},
		{"status", &playback.StatusError{Status: 418}, "Spotify error 418 - try again"},
		{"unknown", errors.New("boom"), "Spotify request failed - restart app"},
	}

	for _, tc := range cases {
		got := failureMessage(tc.err)
		if got != tc.want {
			t.Errorf("%s: failureMessage = %q, want %q", tc.name, got, tc.want)
		}
		// The bar ellipsizes past roughly this width; a message whose
		// tail is cut off loses the part that says what to do.
		if len(got) > 42 {
			t.Errorf("%s: message %q is %d chars, too long for the bar", tc.name, got, len(got))
		}
		if strings.Contains(strings.ToLower(got), "log") {
			t.Errorf("%s: message %q sends the user to the log", tc.name, got)
		}
	}
}
