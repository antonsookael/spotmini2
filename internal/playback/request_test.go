package playback

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Reads and writes have to agree about what a rejection means. They
// didn't: the reason codes were only decoded on the write path, so a
// read refused because the account isn't Premium reached the caller as
// an unexplained error and was reported as one.
func TestReasonsClassifyTheSameForReadsAndWrites(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{
			"not premium",
			403,
			`{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`,
			ErrPremiumRequired,
		},
		{
			"device dropped for being idle",
			404,
			`{"error":{"status":404,"message":"Player command failed: No active device found","reason":"NO_ACTIVE_DEVICE"}}`,
			ErrNoActiveDevice,
		},
		{
			// Was generic until the bar had to name every cause. A
			// missing scope and an expired token are indistinguishable
			// from out here, and both are fixed by signing in again.
			"missing scope reads as a rejected login",
			403,
			`{"error":{"status":403,"message":"Insufficient client scope"}}`,
			ErrAuthExpired,
		},
		{
			"expired token reads as a rejected login",
			401,
			`{"error":{"status":401,"message":"The access token expired"}}`,
			ErrAuthExpired,
		},
		{
			"rate limit",
			429,
			`{"error":{"status":429,"message":"API rate limit exceeded"}}`,
			ErrRateLimited,
		},
		{
			"spotify having a bad day",
			503,
			`{"error":{"status":503,"message":"Service unavailable"}}`,
			ErrSpotifyDown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, readErr := doPlayerGet(srv.URL, "tok")
			writeErr := doPlayerRequest("PUT", srv.URL, "tok", nil)

			for label, err := range map[string]error{"read": readErr, "write": writeErr} {
				if err == nil {
					t.Fatalf("%s: expected an error", label)
				}
				if !errors.Is(err, tc.want) {
					t.Errorf("%s: got %v, want %v", label, err, tc.want)
				}
			}
		})
	}
}

// A 204 means nothing is active, which the volume read reports as
// ErrNoActiveDevice so the caller revives rather than treating an
// unknown volume as zero.
func TestEmptyBodyIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	body, err := doPlayerGet(srv.URL, "tok")
	if err != nil {
		t.Fatalf("204 should not be an error, got %v", err)
	}
	if body != nil {
		t.Errorf("204 should give a nil body, got %q", body)
	}
}

// Anything Spotify rejects with a status nobody has a name for still has
// to carry the number, so the bar can show something quotable rather
// than a shrug.
func TestUnrecognisedStatusKeepsItsNumber(t *testing.T) {
	var statusErr *StatusError

	err := rejectionError("GET /me/player", 418, []byte(`{}`))
	if !errors.As(err, &statusErr) {
		t.Fatalf("got %v, want a *StatusError", err)
	}
	if statusErr.Status != 418 {
		t.Errorf("StatusError.Status = %d, want 418", statusErr.Status)
	}
}

// A 404 without a reason code is a genuinely missing endpoint, not an
// idle device - classifying it as one would send the app off reviving
// devices to fix a URL.
func TestBareNotFoundIsNotAnIdleDevice(t *testing.T) {
	if err := rejectionError("GET /me/player", 404, []byte(`{}`)); errors.Is(err, ErrNoActiveDevice) {
		t.Errorf("a bare 404 classified as %v", err)
	}
}
