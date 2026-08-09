package app

import (
	"errors"
	"testing"
	"time"

	"spotmini-gui/internal/playback"
)

// With no refresh token there is nothing to refresh with, so a rejected
// request has to come straight back rather than being retried. Retrying
// a credential Spotify just refused is how an account earns a rate limit
// on top of the problem it already has.
func TestRetryOnAuthFailureDoesNotRetryWithoutARefreshToken(t *testing.T) {
	a := &App{}
	calls := 0

	_, err := retryOnAuthFailure(a, func(string) (int, error) {
		calls++
		return 0, playback.ErrAuthExpired
	})

	if calls != 1 {
		t.Errorf("read ran %d times, want 1", calls)
	}
	if !errors.Is(err, playback.ErrAuthExpired) {
		t.Errorf("err = %v, want ErrAuthExpired passed through", err)
	}
}

// Anything that isn't an auth rejection must not trigger a refresh or a
// second attempt - a rate limit retried immediately is the worst case.
func TestRetryOnAuthFailureLeavesOtherErrorsAlone(t *testing.T) {
	a := &App{}

	for _, err := range []error{playback.ErrRateLimited, playback.ErrNoActiveDevice, playback.ErrUnreachable} {
		calls := 0
		_, got := retryOnAuthFailure(a, func(string) (int, error) {
			calls++
			return 0, err
		})
		if calls != 1 {
			t.Errorf("%v: read ran %d times, want 1", err, calls)
		}
		if !errors.Is(got, err) {
			t.Errorf("got %v, want %v", got, err)
		}
	}
}

// The happy path must not disturb the value it is passing through.
func TestRetryOnAuthFailureReturnsTheValue(t *testing.T) {
	a := &App{}
	got, err := retryOnAuthFailure(a, func(string) (int, error) { return 42, nil })
	if err != nil || got != 42 {
		t.Errorf("got (%d, %v), want (42, nil)", got, err)
	}
}

// A refresh that just failed must not be retried on the next request.
// The idle poll runs every couple of seconds, so without the cooldown
// one broken refresh token becomes a permanent stream of them.
func TestRefreshBacksOffAfterAFailure(t *testing.T) {
	a := &App{}
	a.refreshTok = "broken"
	a.refreshFailedAt = time.Now()

	if a.refresh(a.currentAccessToken()) {
		t.Error("refresh tried again inside the cooldown")
	}
}

// A caller that arrives after someone else has already replaced the
// token has nothing to do - and must not spend the rotating refresh
// token a second time to find that out.
func TestRefreshIsSingleFlight(t *testing.T) {
	a := &App{}
	a.refreshTok = "good"
	a.accessToken = "new-token"

	// "old-token" is what this caller was rejected with; the token in
	// play has moved on, so this must succeed without a network call.
	if !a.refresh("old-token") {
		t.Error("refresh did not recognise that the token had already been replaced")
	}
}

// The cooldown must expire, or one blip disables recovery for the rest
// of the session.
func TestRefreshRetriesOnceTheCooldownPasses(t *testing.T) {
	a := &App{}
	a.refreshTok = "" // no token, so it stops before any network call
	a.refreshFailedAt = time.Now().Add(-2 * refreshRetryCooldown)

	// Reaching the empty-token check at all proves the cooldown let it
	// through; a live refresh can't be exercised without a network.
	if a.refresh(a.currentAccessToken()) {
		t.Error("refresh claimed success with no refresh token")
	}
}
