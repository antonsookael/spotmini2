package app

import (
	"time"

	"spotmini-gui/internal/backend"
	"spotmini-gui/internal/logging"
)

// Refresh this far before the real expiry, so a call landing right on
// the boundary doesn't use an already-stale token.
const tokenRefreshBuffer = 60 * time.Second

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
