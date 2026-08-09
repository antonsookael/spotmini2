package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/paths"
)

// ErrNetwork means Spotify was never reached (DNS/connection failure),
// as opposed to a request it actually answered and rejected. The
// difference matters: an unreachable network is temporary and worth
// retrying with the token we already have, while a rejected token
// genuinely needs the user to log in again.
var ErrNetwork = errors.New("could not reach spotify")

// defaultClientID is spotmini's own Spotify app Client ID, baked in so
// downloaded builds work without every user registering their own
// Spotify app. That's safe with the Authorization Code + PKCE flow
// used below - unlike the plain Authorization Code flow, PKCE needs
// no client secret alongside the ID, only a per-login proof value
// (see pkce.go) that's generated fresh each time and never leaves
// this machine.
const defaultClientID = "da2657247b364ce1877a8bdef2a33c90"

var clientID string

// loginTimeout is how long the browser flow gets before it's declared
// abandoned. Generous, because it includes finding the tab, signing in
// and granting consent - but not unbounded, since the whole point is
// that the app stops waiting on something that is never coming.
const loginTimeout = 5 * time.Minute

// tokenClient talks to the token endpoint with a deadline. The default
// client has none, and this call is made while the app is waiting to
// start - a stalled connection there hangs the launch rather than
// failing it.
var tokenClient = &http.Client{Timeout: 15 * time.Second}

const redirectURI = "http://127.0.0.1:8888/callback"
const scope = "user-read-playback-state user-modify-playback-state playlist-read-private user-library-read user-library-modify"
const tokenFile = "token.json"

// loginResult is the outcome of one browser login flow: a token, or the
// reason the flow ended without one. Both have to travel back to
// GetAccessTokenFull - a failure that's simply dropped leaves it waiting
// on a token that can never arrive, which is what used to strand the app
// with no playback, no error and nothing retrying.
type loginResult struct {
	token TokenResponse
	err   error
}

// loginChan is how the temporary auth server hands that outcome back to
// GetAccessTokenFull, which is waiting on the other end of this channel.
var loginChan = make(chan loginResult, 1)

// reportLogin delivers a login outcome without ever blocking. The
// buffer takes the first result even if the receiver hasn't arrived
// yet; the default case drops any later one - a refreshed callback tab,
// or the server reporting its own shutdown after the token already went
// through - rather than parking a goroutine on a channel nobody will
// read again.
func reportLogin(r loginResult) {
	select {
	case loginChan <- r:
	default:
	}
}

// pkceVerifier holds the current login attempt's PKCE verifier between
// startLogin generating it and callbackHandler using it to complete
// the token exchange. Fine as a single package-level value since only
// one login flow ever runs at a time.
var pkceVerifier string

// loginState is the OAuth state value for the current login attempt:
// generated fresh, sent to Spotify, and required back unchanged on the
// callback. PKCE proves the token exchange came from whoever started
// the login, but says nothing about who called the callback endpoint -
// without this, /callback completes a flow for any code handed to it.
var loginState string

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`

	// ExpiresAt is ours, not Spotify's. Spotify only says how many
	// seconds the token lasts, which stops meaning anything the moment
	// it's written to disk - a saved file could say "3600 seconds" and
	// be a week old. Stamping the instant it actually expires is what
	// lets a later launch tell whether the token is still usable
	// instead of refreshing unconditionally to find out.
	ExpiresAt time.Time `json:"expires_at"`
}

// tokenValidityMargin is how much life a saved access token needs left
// before startup will use it as-is. Comfortably more than getToken's own
// refresh buffer, so a token accepted here doesn't turn around and need
// refreshing on the first playback call anyway.
const tokenValidityMargin = 5 * time.Minute

// stillValid reports whether a saved token can be used as it stands.
//
// A zero ExpiresAt means the file predates this field, so there's
// nothing to judge it by and refreshing is the safe answer - which is
// also the migration path, since the refreshed token gets stamped.
func (t TokenResponse) stillValid() bool {
	return t.AccessToken != "" &&
		!t.ExpiresAt.IsZero() &&
		time.Now().Add(tokenValidityMargin).Before(t.ExpiresAt)
}

// resolveClientID lets a Client ID be overridden via .env/environment
// (handy for local development against your own Spotify app) but
// otherwise just falls back to the one baked into the binary - no
// setup required for anyone downloading a built release.
func resolveClientID() {
	_ = godotenv.Load() // fine if there's no .env file - not required

	clientID = os.Getenv("SPOTIFY_CLIENT_ID")
	if clientID == "" {
		clientID = defaultClientID
	}
}

// startLogin opens the login URL and runs a temporary local server
// just long enough to catch the callback. Unlike a bare
// http.ListenAndServe, an *http.Server can be told to Shutdown once
// we have what we need, instead of running forever.
func startLogin() {
	verifier, err := generateCodeVerifier()
	if err != nil {
		reportLogin(loginResult{err: fmt.Errorf("generating PKCE verifier: %w", err)})
		return
	}
	state, err := randomURLSafe()
	if err != nil {
		reportLogin(loginResult{err: fmt.Errorf("generating login state: %w", err)})
		return
	}
	pkceVerifier = verifier
	loginState = state

	// Built through url.Values rather than fmt.Sprintf: scope is a
	// space-separated list, and pasting raw spaces into a query string
	// only ever worked because browsers quietly repaired it on the way
	// out. Encode() escapes every value properly.
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", scope)
	params.Set("code_challenge_method", "S256")
	params.Set("code_challenge", codeChallengeFor(verifier))
	params.Set("state", state)

	// Encode() writes spaces as "+", which only means a space to a
	// reader applying form-encoding rules - under RFC 3986 it's a
	// literal plus. scope is the one value here with spaces in it, and a
	// scope list run together with pluses is a set of scopes nobody
	// granted. %20 is a space under both readings, so it removes the
	// question entirely.
	//
	// Safe as a blanket replacement: a plus that was really in a value
	// comes out of Encode() as %2B, so a bare + in this string can only
	// ever be a space.
	authURL := "https://accounts.spotify.com/authorize?" +
		strings.ReplaceAll(params.Encode(), "+", "%20")
	fmt.Println("Opening login URL in your browser:")
	fmt.Println(authURL)
	openBrowser(authURL)

	mux := http.NewServeMux()
	// Bound to loopback, not every interface. redirectURI already points
	// at 127.0.0.1, so nothing needs to reach this from off the machine,
	// and ":8888" put the endpoint that completes a login on the LAN.
	server := &http.Server{Addr: "127.0.0.1:8888", Handler: mux}

	// Nothing arrives if the user closes the consent tab instead of
	// refusing, or never finds it because openBrowser failed - and a
	// login that simply stops has no event of its own to report. Without
	// this the startup goroutine waits on loginChan forever: no token, no
	// error, the bar reading "Loading..." for as long as the app is open,
	// and this server still holding port 8888.
	abandoned := time.AfterFunc(loginTimeout, func() {
		logging.Printf("Login abandoned after %s", loginTimeout)
		reportLogin(loginResult{err: fmt.Errorf("login not completed within %s", loginTimeout)})
		server.Shutdown(context.Background())
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Only a request that ended the flow takes the server down with
		// it. Anything else - a stray hit on this URL, a browser
		// prefetch, an old callback tab reloading with a previous
		// login's state - has to leave it listening, or the redirect
		// we're actually waiting for arrives at a closed port and
		// GetAccessTokenFull blocks on a result nothing will ever send.
		if !callbackHandler(w, r) {
			return
		}
		// The flow ended one way or the other, so it wasn't abandoned.
		abandoned.Stop()

		// Give the browser a moment to receive the response before we
		// shut the server down out from under it.
		go func() {
			server.Shutdown(context.Background())
		}()
	})

	fmt.Println("\nListening on http://127.0.0.1:8888 ...")

	// Blocks until server.Shutdown() is called, which returns
	// ErrServerClosed - the one outcome that isn't a failure, since it
	// means the callback already arrived and reported itself. Anything
	// else (port 8888 held by another copy of the app, most often) means
	// the callback can never arrive, so the wait has to be ended here
	// rather than left hanging on a server that isn't running.
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		reportLogin(loginResult{err: fmt.Errorf("callback server on %s: %w", server.Addr, err)})
	}
}

// RefreshToken trades a refresh token for a brand new access token,
// without needing the browser at all.
func RefreshToken(refresh string) (TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refresh)
	data.Set("client_id", clientID)

	token, err := exchangeForToken(data)
	if err != nil {
		return TokenResponse{}, err
	}

	if token.RefreshToken == "" {
		token.RefreshToken = refresh
	}

	saveToken(token)
	return token, nil
}

// exchangeForToken is shared by both the initial login and the refresh
// flow - both just POST different form data to the same endpoint.
func exchangeForToken(data url.Values) (TokenResponse, error) {
	resp, err := tokenClient.PostForm("https://accounts.spotify.com/api/token", data)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// ErrNetwork like the dial failure above: the connection dropped
		// mid-response, which says nothing about whether the refresh
		// token is any good. Returned bare, it skipped the retry in
		// refreshWhenReachable and sent a user with a perfectly valid
		// token off to log in again in a browser.
		return TokenResponse{}, fmt.Errorf("%w: %v", ErrNetwork, err)
	}

	// Spotify's error responses are still valid JSON (e.g.
	// {"error":"invalid_grant",...}), just not shaped like a
	// TokenResponse - unmarshaling one of those into TokenResponse
	// silently "succeeds" with every field empty instead of surfacing
	// the failure, unless the status code is checked here too.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("spotify returned status %d: %s", resp.StatusCode, string(body))
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return TokenResponse{}, fmt.Errorf("parsing response: %w (raw: %s)", err, string(body))
	}

	// Stamped here rather than at the call sites: every token in the
	// app - login and refresh alike - comes through this one function,
	// so this is the only place that knows the countdown started now.
	token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)

	return token, nil
}

func saveToken(token TokenResponse) {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		logging.Printf("Error saving token: %v", err)
		return
	}
	path, err := paths.ConfigFile(tokenFile)
	if err != nil {
		logging.Printf("Error resolving token file path: %v", err)
		return
	}
	// 0600, not 0644: this file holds the refresh token, which is a
	// standing credential for the account until it's revoked. Enforced
	// on macOS; on Windows the ACL inherited from %AppData% is what
	// actually applies.
	if err := os.WriteFile(path, data, 0600); err != nil {
		logging.Printf("Error writing token file: %v", err)
		return
	}

	// WriteFile's permission argument only applies when it *creates* the
	// file, so every install that predates the 0600 change kept its
	// world-readable 0644 no matter how many times the token was
	// rewritten. Chmod is what actually repairs those.
	if err := os.Chmod(path, 0600); err != nil {
		logging.Printf("Could not restrict permissions on the token file: %v", err)
	}
}

func loadToken() (TokenResponse, error) {
	path, err := paths.ConfigFile(tokenFile)
	if err != nil {
		return TokenResponse{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return TokenResponse{}, err
	}
	var token TokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return TokenResponse{}, err
	}
	return token, nil
}

// refreshWhenReachable retries a refresh for as long as the only thing
// wrong is that Spotify can't be reached, backing off up to a minute
// between attempts. Opening the laptop with no network used to leave
// the app permanently dead: the failed refresh was indistinguishable
// from a rejected token, so it launched a browser login and then
// blocked forever on a callback that could never arrive, with nothing
// retrying once the network returned. Any answer Spotify actually
// gives - including a genuinely invalid token - returns immediately so
// the real login flow still runs.
func refreshWhenReachable(refresh string) (TokenResponse, error) {
	const maxBackoff = 60 * time.Second
	backoff := 2 * time.Second

	for {
		token, err := RefreshToken(refresh)
		if !errors.Is(err, ErrNetwork) {
			return token, err
		}

		logging.Printf("Spotify unreachable, retrying token refresh in %s", backoff)
		time.Sleep(backoff)
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// GetAccessTokenFull is the single entry point the app calls on startup:
// try a saved token first, refresh if possible, otherwise fall back to a
// full browser login and wait for the result on loginChan. Returns the
// full TokenResponse since the caller needs the refresh token too, for
// the ongoing background refresh loop.
//
// The error is what a login that ends without a token looks like -
// consent refused, the callback server unable to listen, the code
// exchange rejected. There's no retry here: the caller reports it and
// the user relaunches, which is better than the previous behaviour of
// blocking forever on a token nothing was going to send.
func GetAccessTokenFull() (TokenResponse, error) {
	resolveClientID()

	saved, err := loadToken()
	if err != nil {
		logging.Printf("No saved token found (%v) - starting full login", err)
	} else if saved.RefreshToken == "" {
		logging.Printf("Saved token has no refresh token - starting full login")
	} else if saved.stillValid() {
		// The common case, and deliberately silent: it happens on every
		// launch inside the token's lifetime, and a line saying nothing
		// happened is exactly the bulk this log is meant not to have.
		return saved, nil
	} else {
		newToken, err := refreshWhenReachable(saved.RefreshToken)
		if err == nil {
			// Logged on the way out rather than on the way in, so the
			// line records something that actually happened. The old
			// "refreshing instead of logging in again" was written
			// before the attempt and told you nothing about how it went.
			logging.Printf("Saved token had expired - refreshed it")
			return newToken, nil
		}
		logging.Printf("Refresh failed, falling back to full login: %v", err)
	}

	go startLogin()

	result := <-loginChan // blocks here until the flow reports an outcome
	return result.token, result.err
}
