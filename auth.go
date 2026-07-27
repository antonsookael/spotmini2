package main

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

var clientID, clientSecret string

const redirectURI = "http://127.0.0.1:8888/callback"
const scope = "user-read-playback-state user-modify-playback-state playlist-read-private"
const tokenFile = "token.json"

// tokenChan is how callbackHandler (running inside the temporary auth
// server) hands the finished token back to main(), which is waiting
// on the other end of this channel.
var tokenChan = make(chan TokenResponse)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func loadEnv() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: no .env file found - relying on real environment variables instead")
	}
	clientID = os.Getenv("clientID")
	clientSecret = os.Getenv("clientSecret")
	if clientID == "" || clientSecret == "" {
		fmt.Println("Missing CLIENT_ID or CLIENT_SECRET - check your .env file")
		os.Exit(1)
	}
}

// startLogin opens the login URL and runs a temporary local server
// just long enough to catch the callback. Unlike a bare
// http.ListenAndServe, an *http.Server can be told to Shutdown once
// we have what we need, instead of running forever.
func startLogin() {
	authURL := fmt.Sprintf(
		"https://accounts.spotify.com/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s",
		clientID, redirectURI, scope,
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

// refreshToken trades a refresh token for a brand new access token,
// without needing the browser at all.
func refreshToken(refresh string) (TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refresh)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

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

// getAccessTokenFull is the single entry point main() calls: try a
// saved token first, refresh if possible, otherwise fall back to a
// full browser login and wait for the result on tokenChan. Returns
// the full TokenResponse since the caller needs the refresh token
// too, for the ongoing background refresh loop.
func getAccessTokenFull() TokenResponse {
	loadEnv()

	saved, err := loadToken()
	if err == nil && saved.RefreshToken != "" {
		fmt.Println("Found saved token, refreshing instead of logging in again...")
		newToken, err := refreshToken(saved.RefreshToken)
		if err == nil {
			return newToken
		}
		fmt.Println("Refresh failed, falling back to full login:", err)
	}

	go startLogin()
	return <-tokenChan // blocks here until callbackHandler sends one
}
