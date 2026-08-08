package backend

import (
	"fmt"
	"net/http"
	"net/url"

	"spotmini-gui/internal/logging"
)

// callbackHandler completes the login flow and reports its outcome, one
// way or the other. Every path out of here that isn't the plain "not
// the redirect we're waiting for" case ends in a reportLogin call -
// GetAccessTokenFull is blocked until one arrives, so a path that just
// returns leaves the app permanently half-started.
//
// The bool says which of those two happened: true once an outcome has
// been reported and the flow is over, false for a request that wasn't
// our redirect. The caller shuts the callback server down on true only
// - tearing it down on a request we deliberately didn't report would
// strand the login just as thoroughly, since the redirect we're still
// waiting for would then have nothing left to arrive at.
func callbackHandler(w http.ResponseWriter, r *http.Request) bool {
	// Checked before anything else, including the error case, since
	// Spotify echoes state back on both. A mismatch means this request
	// didn't come from the login we started, so it's refused *without*
	// reporting a result - a forged callback shouldn't be able to end a
	// real login flow any more than it should be able to complete one.
	if state := r.URL.Query().Get("state"); loginState == "" || state != loginState {
		logging.Printf("Ignoring callback with unexpected state")
		fmt.Fprintln(w, "That link didn't come from this app.")
		return false
	}

	// Spotify redirects here with ?error=access_denied and no code at
	// all when consent is refused. Without this the flow fell through to
	// an exchange of the empty code, failed, and reported nothing.
	if reason := r.URL.Query().Get("error"); reason != "" {
		logging.Printf("Spotify login refused: %s", reason)
		fmt.Fprintln(w, "Login was cancelled. You can close this tab.")
		reportLogin(loginResult{err: fmt.Errorf("spotify login refused: %s", reason)})
		return true
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		// Matching state but neither a code nor an error - not a shape
		// Spotify sends, so there's nothing to exchange and nothing
		// diagnosable. Left for the real callback rather than reported,
		// on the same reasoning as the state mismatch above.
		fmt.Fprintln(w, "Waiting for the Spotify login to complete...")
		return false
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", clientID)
	data.Set("code_verifier", pkceVerifier)

	token, err := exchangeForToken(data)
	if err != nil {
		logging.Printf("Token exchange failed: %v", err)
		fmt.Fprintln(w, "Something went wrong finishing the login - check the log and try again.")
		reportLogin(loginResult{err: fmt.Errorf("exchanging code for token: %w", err)})
		return true
	}

	saveToken(token)
	fmt.Fprintln(w, "Logged in! You can close this tab and go back to the app.")

	// Hand the token back to GetAccessTokenFull, which is waiting on the
	// other end of this channel.
	reportLogin(loginResult{token: token})
	return true
}
