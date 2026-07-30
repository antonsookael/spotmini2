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

	isExpanded   bool
	openedUpward bool
}

const (
	collapsedHeight = 50
	expandedHeight  = 250
)

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

	// The hotkey MUST be registered on the thread startup() runs on.
	// On macOS this has to be the OS main thread, or the OS never
	// delivers key events to it even though Register() reports success.
	hkSettings := hotkey.New(settingsHotkeyMods(), hotkey.KeyC)
	if err := hkSettings.Register(); err != nil {
		fmt.Printf("Failed to register Settings hotkey: %v\n", err)
		return
	}
	fmt.Println("Listening for the customize hotkey...")

	go a.watchSettingsHotkey(hkSettings)
}

func (a *App) watchSettingsHotkey(hkSettings *hotkey.Hotkey) {
	defer hkSettings.Unregister()

	delta := expandedHeight - collapsedHeight

	for range hkSettings.Keydown() {
		a.isExpanded = !a.isExpanded

		if a.isExpanded {
			x, y := runtime.WindowGetPosition(a.ctx)
			screenHeight := a.currentScreenHeight()

			if screenHeight > 0 && y+expandedHeight > screenHeight {
				a.openedUpward = true
				runtime.WindowSetPosition(a.ctx, x, y-delta)
			} else {
				a.openedUpward = false
			}
			runtime.WindowSetSize(a.ctx, 320, expandedHeight)
		} else {
			runtime.WindowSetSize(a.ctx, 320, collapsedHeight)
			if a.openedUpward {
				x, y := runtime.WindowGetPosition(a.ctx)
				runtime.WindowSetPosition(a.ctx, x, y+delta)
				a.openedUpward = false
			}
		}
		runtime.EventsEmit(a.ctx, "toggle-settings", a.isExpanded)
	}
}

// currentScreenHeight returns the logical height of the screen the window
// currently sits on, or 0 if it can't be determined.
func (a *App) currentScreenHeight() int {
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err != nil || len(screens) == 0 {
		return 0
	}
	for _, s := range screens {
		if s.IsCurrent {
			return s.Size.Height
		}
	}
	return screens[0].Size.Height
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
