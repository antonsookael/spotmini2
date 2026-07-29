package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/hotkey"

	"spotmini-gui/playback"
)

type App struct {
	ctx context.Context

	tokenMu     sync.RWMutex
	accessToken string
	refreshTok  string

	// Tracks whether the settings panel is open
	isExpanded bool
}

func NewApp() *App {
	return &App{
		isExpanded: false,
	}
}

func (a *App) getToken() string {
	a.tokenMu.RLock()
	defer a.tokenMu.RUnlock()
	return a.accessToken
}

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

	// Launch hotkey listener safely in its own OS thread context if needed,
	// or standard goroutine depending on OS hooks
	go a.listenForHotkeys()
}

func (a *App) listenForHotkeys() {
	hkSettings := hotkey.New(settingsHotkeyMods(), hotkey.KeyC)

	if err := hkSettings.Register(); err != nil {
		fmt.Printf("Failed to register Settings hotkey: %v\n", err)
		return
	}
	defer hkSettings.Unregister()

	fmt.Println("Listening for Ctrl+Alt+C...")

	for range hkSettings.Keydown() {
		a.isExpanded = !a.isExpanded
		if a.isExpanded {
			runtime.WindowSetSize(a.ctx, 320, 250)
		} else {
			runtime.WindowSetSize(a.ctx, 320, 50)
		}
		runtime.EventsEmit(a.ctx, "toggle-settings", a.isExpanded)
	}
}

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
