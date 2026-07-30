package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/joho/godotenv"
)

// defaultClientID is spotmini's own Spotify app Client ID, baked in so
// downloaded builds work without every user registering their own
// Spotify app. That's safe with the Authorization Code + PKCE flow
// used below - unlike the plain Authorization Code flow, PKCE needs
// no client secret alongside the ID, only a per-login proof value
// (see pkce.go) that's generated fresh each time and never leaves
// this machine.
const defaultClientID = "da2657247b364ce1877a8bdef2a33c90"

var clientID string

const redirectURI = "http://127.0.0.1:8888/callback"
const scope = "user-read-playback-state user-modify-playback-state playlist-read-private"
const tokenFile = "token.json"

// tokenChan is how callbackHandler (running inside the temporary auth
// server) hands the finished token back to GetAccessTokenFull, which is
// waiting on the other end of this channel.
var tokenChan = make(chan TokenResponse)

// pkceVerifier holds the current login attempt's PKCE verifier between
// startLogin generating it and callbackHandler using it to complete
// the token exchange. Fine as a single package-level value since only
// one login flow ever runs at a time.
var pkceVerifier string

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
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
		fmt.Println("Error generating PKCE verifier:", err)
		return
	}
	pkceVerifier = verifier

	authURL := fmt.Sprintf(
		"https://accounts.spotify.com/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&code_challenge_method=S256&code_challenge=%s",
		clientID, redirectURI, scope, codeChallengeFor(verifier),
	)
	fmt.Println("Opening login URL in your browser:")
	fmt.Println(authURL)
	openBrowser(authURL)

	mux := http.NewServeMux()
	server := &http.Server{Addr: ":8888", Handler: mux}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		callbackHandler(w, r)

		// Give the browser a moment to receive the response before we
		// shut the server down out from under it.
		go func() {
			server.Shutdown(context.Background())
		}()
	})

	fmt.Println("\nListening on http://127.0.0.1:8888 ...")
	server.ListenAndServe() // blocks until server.Shutdown() is called
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
	resp, err := http.PostForm("https://accounts.spotify.com/api/token", data)
	if err != nil {
		return TokenResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, err
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return TokenResponse{}, fmt.Errorf("parsing response: %w (raw: %s)", err, string(body))
	}

	return token, nil
}

func saveToken(token TokenResponse) {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		fmt.Println("Error saving token:", err)
		return
	}
	if err := os.WriteFile(tokenFile, data, 0644); err != nil {
		fmt.Println("Error writing token file:", err)
	}
}

func loadToken() (TokenResponse, error) {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return TokenResponse{}, err
	}
	var token TokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return TokenResponse{}, err
	}
	return token, nil
}

// GetAccessTokenFull is the single entry point the app calls on startup:
// try a saved token first, refresh if possible, otherwise fall back to a
// full browser login and wait for the result on tokenChan. Returns the
// full TokenResponse since the caller needs the refresh token too, for
// the ongoing background refresh loop.
func GetAccessTokenFull() TokenResponse {
	resolveClientID()

	saved, err := loadToken()
	if err == nil && saved.RefreshToken != "" {
		fmt.Println("Found saved token, refreshing instead of logging in again...")
		newToken, err := RefreshToken(saved.RefreshToken)
		if err == nil {
			return newToken
		}
		fmt.Println("Refresh failed, falling back to full login:", err)
	}

	go startLogin()
	return <-tokenChan // blocks here until callbackHandler sends one
}
