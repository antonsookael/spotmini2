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
			"missing scope carries no reason, so stays generic",
			403,
			`{"error":{"status":403,"message":"Insufficient client scope"}}`,
			nil,
		},
		{
			"expired token stays generic",
			401,
			`{"error":{"status":401,"message":"The access token expired"}}`,
			nil,
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
				if tc.want != nil && !errors.Is(err, tc.want) {
					t.Errorf("%s: got %v, want %v", label, err, tc.want)
				}
				if tc.want == nil && (errors.Is(err, ErrPremiumRequired) || errors.Is(err, ErrNoActiveDevice)) {
					t.Errorf("%s: got sentinel %v, want a generic error", label, err)
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
