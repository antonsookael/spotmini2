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
	expires := a.tokenExpiresAt
	a.tokenMu.RUnlock()

	if refresh == "" || time.Now().Before(expires.Add(-tokenRefreshBuffer)) {
		return token
	}

	a.refresh(token)

	a.tokenMu.RLock()
	defer a.tokenMu.RUnlock()
	return a.accessToken
}

// refresh trades the refresh token for a new access token, and reports
// whether the token in play is now a good one.
//
// usedToken is the access token the caller was working with - either the
// stale one getToken found, or the one Spotify just rejected. It is what
// makes this single-flight: a caller that arrives to find the token has
// already moved on has nothing left to do, so a burst of rejected
// requests produces one refresh between them rather than one each. That
// matters beyond mere waste, because Spotify rotates the refresh token
// and invalidates the old one, so two refreshes racing with the same
// value means one of them comes back invalid_grant.
//
// The network call deliberately happens with no lock held. It used to run
// under tokenMu's write lock, which meant every hotkey and every poll
// queued behind it - and with no timeout on the client, a connection that
// stalled instead of failing froze the whole app for as long as the OS
// took to give up.
func (a *App) refresh(usedToken string) bool {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()

	a.tokenMu.RLock()
	refreshTok := a.refreshTok
	current := a.accessToken
	failedAt := a.refreshFailedAt
	a.tokenMu.RUnlock()

	if refreshTok == "" {
		return false
	}
	// Someone refreshed while we waited our turn, so the token this
	// caller had trouble with isn't the one in play any more.
	if current != usedToken {
		return true
	}
	// A refresh that just failed will not have started working seconds
	// later, and the callers here include a poll. Without this, one dead
	// refresh token means a request to Spotify's token endpoint every
	// couple of seconds for as long as the app is open.
	if time.Since(failedAt) < refreshRetryCooldown {
		return false
	}

	newToken, err := backend.RefreshToken(refreshTok)
	if err != nil {
		logging.Printf("Token refresh failed: %v", err)
		a.tokenMu.Lock()
		a.refreshFailedAt = time.Now()
		a.tokenMu.Unlock()
		return false
	}

	a.tokenMu.Lock()
	a.accessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		a.refreshTok = newToken.RefreshToken
	}
	a.tokenExpiresAt = newToken.ExpiresAt
	a.refreshFailedAt = time.Time{}
	a.tokenMu.Unlock()

	// To the log file, not just stdout: a built app has no console, and
	// a refresh is infrequent enough to be worth a line - it's the thing
	// that goes quiet first when auth starts failing.
	logging.Printf("Access token refreshed")
	return true
}

// retryOnAuthFailure runs read, and if Spotify rejected the token,
// refreshes it and runs read once more.
//
// One retry, not a loop: either the new token works or the refresh
// itself is the problem, and hammering a rejected credential is how an
// account ends up rate limited on top of everything else.
func retryOnAuthFailure[T any](a *App, read func(token string) (T, error)) (T, error) {
	token := a.getToken()
	value, err := read(token)
	if !errors.Is(err, playback.ErrAuthExpired) {
		return value, err
	}
	if !a.refresh(token) {
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
		// Through refresh() like everything else, so a tick that lands
		// while a command is already refreshing doesn't spend the same
		// rotating refresh token a second time.
		a.refresh(a.currentAccessToken())
	}
}

// currentAccessToken reads the token without the expiry check getToken
// does - only the refresh paths want this, to say which token they were
// working from.
func (a *App) currentAccessToken() string {
	a.tokenMu.RLock()
	defer a.tokenMu.RUnlock()
	return a.accessToken
}
