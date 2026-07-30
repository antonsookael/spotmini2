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

	hotkeyMu      sync.Mutex
	hotkeyConfig  HotkeyConfig
	activeHotkeys map[string]*hotkey.Hotkey
}

const (
	collapsedHeight = 50
	expandedHeight  = 420
)

func NewApp() *App {
	return &App{
		isExpanded:    false,
		activeHotkeys: make(map[string]*hotkey.Hotkey),
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

	// Hotkeys MUST be registered on the thread startup() runs on.
	// On macOS this has to be the OS main thread, or the OS never
	// delivers key events to it even though Register() reports success.
	a.hotkeyConfig = loadHotkeyConfig()
	for _, action := range hotkeyActions {
		binding, _ := a.hotkeyConfig.binding(action)
		if err := a.applyHotkey(action, binding); err != nil {
			fmt.Printf("Failed to register %s hotkey: %v\n", action, err)
		}
	}
}

// applyHotkey registers binding for action, swapping out and unregistering
// any hotkey previously registered for that action. The old hotkey is only
// torn down after the new one registers successfully, so a bad binding
// (e.g. one already taken by the OS) doesn't leave the action dead.
func (a *App) applyHotkey(action string, binding HotkeyBinding) error {
	hk, err := bindingToHotkey(binding)
	if err != nil {
		return err
	}
	if err := hk.Register(); err != nil {
		return err
	}

	a.hotkeyMu.Lock()
	old := a.activeHotkeys[action]
	a.activeHotkeys[action] = hk
	a.hotkeyMu.Unlock()

	if old != nil {
		old.Unregister()
	}

	fmt.Printf("Listening for the %s hotkey...\n", action)
	go a.watchHotkey(action, hk)
	return nil
}

// watchHotkey runs until hk is unregistered (Unregister closes the channel
// Keydown reads from, so this loop exits cleanly on its own).
func (a *App) watchHotkey(action string, hk *hotkey.Hotkey) {
	fn := a.hotkeyAction(action)
	for range hk.Keydown() {
		fn()
	}
}

func (a *App) hotkeyAction(action string) func() {
	switch action {
	case "settings":
		return a.ToggleSettingsPanel
	case "playPause":
		return a.PlayPause
	case "next":
		return a.NextTrack
	case "previous":
		return a.PreviousTrack
	case "shuffle":
		return a.ToggleShuffle
	}
	return func() {}
}

// GetHotkeyConfig returns the currently active hotkey bindings.
func (a *App) GetHotkeyConfig() HotkeyConfig {
	a.hotkeyMu.Lock()
	defer a.hotkeyMu.Unlock()
	return a.hotkeyConfig
}

// SetHotkeyBinding rebinds action to the given modifiers/key, persists it,
// and applies it immediately. The previous binding for action stays active
// if the new one fails to register (e.g. it's already taken by the OS).
func (a *App) SetHotkeyBinding(action string, mods []string, key string) error {
	a.hotkeyMu.Lock()
	if _, ok := a.hotkeyConfig.binding(action); !ok {
		a.hotkeyMu.Unlock()
		return fmt.Errorf("unknown hotkey action %q", action)
	}
	a.hotkeyMu.Unlock()

	binding := HotkeyBinding{Mods: mods, Key: key}
	if err := a.applyHotkey(action, binding); err != nil {
		return err
	}

	a.hotkeyMu.Lock()
	a.hotkeyConfig.setBinding(action, binding)
	cfg := a.hotkeyConfig
	a.hotkeyMu.Unlock()

	return saveHotkeyConfig(cfg)
}

// ToggleSettingsPanel opens/closes the customize panel, resizing (and, if
// there isn't room to grow downward, repositioning) the window to fit it.
func (a *App) ToggleSettingsPanel() {
	a.isExpanded = !a.isExpanded

	delta := expandedHeight - collapsedHeight

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

func (a *App) NextTrack() {
	playback.NextTrack(a.getToken())
}

func (a *App) PreviousTrack() {
	playback.PreviousTrack(a.getToken())
}

func (a *App) ToggleShuffle() {
	token := a.getToken()
	shuffled, err := playback.IsShuffled(token)
	if err != nil {
		return
	}
	playback.ToggleShuffle(token, !shuffled)
}

func (a *App) GetNowPlaying() playback.PlaybackState {
	state, err := playback.NowPlaying(a.getToken())
	if err != nil {
		return playback.PlaybackState{}
	}
	return state
}
