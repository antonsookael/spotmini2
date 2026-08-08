package backend

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStillValid(t *testing.T) {
	cases := []struct {
		name  string
		token TokenResponse
		want  bool
	}{
		{
			"a token with an hour left is usable as-is",
			TokenResponse{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)},
			true,
		},
		{
			"an expired token is not",
			TokenResponse{AccessToken: "a", ExpiresAt: time.Now().Add(-time.Minute)},
			false,
		},
		{
			"nor is one expiring inside the margin",
			TokenResponse{AccessToken: "a", ExpiresAt: time.Now().Add(tokenValidityMargin / 2)},
			false,
		},
		{
			"no recorded expiry means nothing to trust",
			TokenResponse{AccessToken: "a"},
			false,
		},
		{
			"an expiry without a token is no use either",
			TokenResponse{ExpiresAt: time.Now().Add(time.Hour)},
			false,
		},
	}

	for _, tc := range cases {
		if got := tc.token.stillValid(); got != tc.want {
			t.Errorf("%s: stillValid() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The expiry has to survive being written to token.json and read back,
// or every launch would find no expiry and refresh regardless - which is
// the whole thing this replaced.
func TestExpiresAtSurvivesTheTokenFile(t *testing.T) {
	original := TokenResponse{
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}

	var loaded TokenResponse
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}

	if !loaded.ExpiresAt.Equal(original.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", loaded.ExpiresAt, original.ExpiresAt)
	}
	if !loaded.stillValid() {
		t.Error("a token with an hour left should still be valid after a round trip")
	}
}

// A token.json written before ExpiresAt existed has to fall through to a
// refresh rather than being read as "expires at the zero time" and
// somehow trusted - this is the upgrade path, and it only gets one shot
// at it before the refreshed token is stamped.
func TestTokenFileWithoutExpiryRefreshes(t *testing.T) {
	legacy := `{
	  "access_token": "a",
	  "token_type": "Bearer",
	  "expires_in": 3600,
	  "refresh_token": "r"
	}`

	var token TokenResponse
	if err := json.Unmarshal([]byte(legacy), &token); err != nil {
		t.Fatalf("unmarshalling legacy token: %v", err)
	}
	if token.RefreshToken != "r" {
		t.Fatalf("refresh token lost: %q", token.RefreshToken)
	}
	if token.stillValid() {
		t.Error("a token file predating ExpiresAt must not be treated as usable")
	}
}
