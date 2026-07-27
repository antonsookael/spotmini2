package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/playback"
)

type App struct {
	ctx context.Context

	// tokenMu guards access to accessToken and refreshTok, since both
	// the periodic refresh goroutine and any JS-triggered method call
	// (PlayPause, GetNowPlaying, etc.) can read/write them concurrently.
	tokenMu     sync.RWMutex
	accessToken string
	refreshTok  string
}

func NewApp() *App {
	return &App{}
}

// getToken safely reads the current access token.
func (a *App) getToken() string {
	a.tokenMu.RLock()
	defer a.tokenMu.RUnlock()
	return a.accessToken
}

// setTokens safely writes new tokens after a login or refresh.
func (a *App) setTokens(access, refresh string) {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	a.accessToken = access
	if refresh != "" {
		a.refreshTok = refresh
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	go func() {
		token := getAccessTokenFull()
		a.setTokens(token.AccessToken, token.RefreshToken)
		runtime.EventsEmit(a.ctx, "logged-in")

		a.startTokenRefreshLoop()
	}()
}

// startTokenRefreshLoop refreshes the access token every 50 minutes.
// Spotify tokens expire after ~60 minutes, so refreshing at 50 leaves
// a safety margin instead of cutting it close to the actual deadline.
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

		newToken, err := refreshToken(refresh)
		if err != nil {
			fmt.Println("Background token refresh failed:", err)
			continue
		}

		a.setTokens(newToken.AccessToken, newToken.RefreshToken)
		fmt.Println("Access token refreshed in background")
	}
}

func (a *App) PlayPause() {
	token := a.getToken()
	playing, err := playback.IsPlaying(token)
	if err != nil {
		return
	}
	if playing {
		playback.PausePlayback(token)
	} else {
		playback.PlayPlayback(token)
	}
}

func (a *App) GetNowPlaying() playback.PlaybackState {
	state, err := playback.NowPlaying(a.getToken())
	if err != nil {
		return playback.PlaybackState{}
	}
	return state
}
