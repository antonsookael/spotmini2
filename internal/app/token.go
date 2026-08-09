package app

import (
	"errors"
	"time"

	"spotmini-gui/internal/backend"
	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
)

// Refresh this far before the real expiry, so a call landing right on
// the boundary doesn't use an already-stale token.
const tokenRefreshBuffer = 60 * time.Second

// How long to leave a failed forced refresh alone before trying again.
// Long enough that a poll every ten seconds can't turn one broken token
// into a steady stream of refresh attempts, short enough that a blip
// clears on its own well within a track.
const refreshRetryCooldown = 30 * time.Second

// getToken returns the access token, refreshing first if it's expired.
// Checks wall-clock expiry on every call rather than trusting
// startTokenRefreshLoop: that ticker uses the monotonic clock, which
// pauses while the machine sleeps, so it under-counts real elapsed
// time and can leave the token stale after a long sleep.
func (a *App) getToken() string {
	a.tokenMu.RLock()
	token := a.accessToken
	refresh := a.refreshTok
	stale := refresh != "" && time.Now().After(a.tokenExpiresAt.Add(-tokenRefreshBuffer))
	a.tokenMu.RUnlock()

	if !stale {
		return token
	}

	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()

	// Another goroutine may have refreshed while we waited for the lock.
	if time.Now().Before(a.tokenExpiresAt.Add(-tokenRefreshBuffer)) {
		return a.accessToken
	}

	newToken, err := backend.RefreshToken(a.refreshTok)
	if err != nil {
		logging.Printf("On-demand token refresh failed: %v", err)
		return a.accessToken
	}

	a.accessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		a.refreshTok = newToken.RefreshToken
	}
	a.tokenExpiresAt = newToken.ExpiresAt
	// To the log file, not just stdout: a built app has no console, and
	// a refresh is infrequent enough to be worth a line - it's the thing
	// that goes quiet first when auth starts failing.
	logging.Printf("Access token refreshed on demand")

	return a.accessToken
}

// refreshNow trades the refresh token for a new access token regardless
// of what the expiry says, and reports whether it worked.
//
// getToken's clock check catches the ordinary case, but it can only act
// on when the token was *supposed* to die. A token can be dead early -
// revoked from the Spotify account page, invalidated by a password
// change, or simply the stale one getToken hands back when its own
// refresh failed - and the only thing that ever finds out is a request
// coming back 401. That's what this is for.
func (a *App) refreshNow() bool {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()

	if a.refreshTok == "" {
		return false
	}

	// A refresh that just failed is not going to start working within
	// seconds, and the reads that trigger this include a poll running
	// every ten. Without the cooldown a broken refresh token means two
	// requests to Spotify every poll, forever - which is its own way of
	// earning a rate limit on top of whatever broke first.
	if time.Since(a.refreshFailedAt) < refreshRetryCooldown {
		return false
	}

	newToken, err := backend.RefreshToken(a.refreshTok)
	if err != nil {
		logging.Printf("Forced token refresh failed: %v", err)
		a.refreshFailedAt = time.Now()
		return false
	}
	a.refreshFailedAt = time.Time{}

	a.accessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		a.refreshTok = newToken.RefreshToken
	}
	a.tokenExpiresAt = newToken.ExpiresAt
	logging.Printf("Access token refreshed after a rejected request")
	return true
}

// retryOnAuthFailure runs read, and if Spotify rejected the token,
// refreshes it and runs read once more.
//
// One retry, not a loop: either the new token works or the refresh
// itself is the problem, and hammering a rejected credential is how an
// account ends up rate limited on top of everything else.
func retryOnAuthFailure[T any](a *App, read func(token string) (T, error)) (T, error) {
	value, err := read(a.getToken())
	if !errors.Is(err, playback.ErrAuthExpired) {
		return value, err
	}
	if !a.refreshNow() {
		return value, err
	}
	return read(a.getToken())
}

// setTokens takes the whole TokenResponse rather than its pieces so the
// expiry comes from the token itself.
//
// It used to be recomputed here as now+ExpiresIn, which is only right
// for a token minted this instant. A saved token reused at startup was
// issued at some earlier point, so that arithmetic would have dated it
// from launch and left getToken believing a nearly-dead token had a full
// hour on it.
func (a *App) setTokens(t backend.TokenResponse) {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	a.accessToken = t.AccessToken
	if t.RefreshToken != "" {
		a.refreshTok = t.RefreshToken
	}
	a.tokenExpiresAt = t.ExpiresAt
}

// startTokenRefreshLoop refreshes every 50 minutes so calls rarely
// stall on a refresh. Just an optimisation - getToken's expiry check
// is what actually guarantees a valid token.
func (a *App) startTokenRefreshLoop() {
	ticker := time.NewTicker(50 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		a.tokenMu.RLock()
		refresh := a.refreshTok
		a.tokenMu.RUnlock()

		if refresh == "" {
			continue
		}

		newToken, err := backend.RefreshToken(refresh)
		if err != nil {
			logging.Printf("Background token refresh failed: %v", err)
			continue
		}

		a.setTokens(newToken)
		logging.Printf("Access token refreshed in background")
	}
}
