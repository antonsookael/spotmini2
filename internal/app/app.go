// Package app is the Wails-bound application layer: it owns the window,
// the Spotify auth token, the global hotkeys, and every method exposed
// to the frontend. The Spotify HTTP calls themselves live in the
// playback package; this package decides when to make them and what to
// do with the results.
package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/hotkey"

	"spotmini-gui/internal/backend"
	"spotmini-gui/internal/hotkeys"
)

const (
	// Wide enough for a typical "Song - Artist" alongside the timer and
	// the five icons/buttons, which together take ~250px - at the old
	// 320 that left ~70px for the title, so almost anything ellipsized.
	// 440 fits most titles; past ~460 the extra width stops buying any.
	windowWidth = 440

	// The floor the OS enforces. Well below windowWidth so auto-fit can
	// shrink past the default when there's little to show - "Nothing
	// playing" hides the timer and icons and needs far less room.
	minWindowWidth = 150

	collapsedHeight = 50
	expandedHeight  = 355
)

// App holds the shared state behind every frontend-callable method.
//
// Each mutex guards only the fields grouped beneath it. They're separate
// rather than one big lock because these are touched by unrelated
// callers at unrelated times: hotkeys fire on their own goroutines (see
// watchHotkey) while the frontend calls in on another, so a single lock
// would have volume changes waiting on panel toggles.
type App struct {
	ctx context.Context

	tokenMu        sync.RWMutex
	accessToken    string
	refreshTok     string
	tokenExpiresAt time.Time

	// expandedPanel is "" when collapsed, otherwise "settings" or
	// "playlists" - only one can be open at a time, since they share
	// the same window-resize mechanism.
	panelMu       sync.Mutex
	expandedPanel string
	openedUpward  bool

	hotkeyMu      sync.Mutex
	hotkeyConfig  hotkeys.HotkeyConfig
	activeHotkeys map[string]*hotkey.Hotkey

	dragMu             sync.Mutex
	dragActive         bool
	dragOriginX        int
	dragOriginY        int
	dragOriginResolved bool
	dragWidth          int
	dragHeight         int

	volumeMu     sync.Mutex
	lastVolume   int
	lastVolumeAt time.Time
}

// New returns an App ready to be handed to Wails. Everything that needs
// a live window or network access happens in startup instead.
func New() *App {
	return &App{
		activeHotkeys: make(map[string]*hotkey.Hotkey),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.restoreWindowPosition()

	go func() {
		token := backend.GetAccessTokenFull()
		a.setTokens(token.AccessToken, token.RefreshToken, token.ExpiresIn)
		runtime.EventsEmit(a.ctx, "logged-in")

		a.startTokenRefreshLoop()
	}()

	// Hotkeys MUST be registered on the thread startup() runs on.
	// On macOS this has to be the OS main thread, or the OS never
	// delivers key events to it even though Register() reports success.
	a.hotkeyConfig = hotkeys.LoadConfig()
	for _, action := range hotkeys.Actions {
		binding, _ := a.hotkeyConfig.Binding(action)
		if err := a.applyHotkey(action, binding); err != nil {
			fmt.Printf("Failed to register %s hotkey: %v\n", action, err)
		}
	}
}

// shutdown persists the window's position so the next launch can
// restore it.
func (a *App) shutdown(ctx context.Context) {
	a.saveWindowPosition()
}
