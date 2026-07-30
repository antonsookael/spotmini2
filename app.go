package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/hotkey"

	"spotmini-gui/backend"
	"spotmini-gui/hotkeys"
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
	hotkeyConfig  hotkeys.HotkeyConfig
	activeHotkeys map[string]*hotkey.Hotkey
}

const (
	collapsedHeight = 50
	expandedHeight  = 540

	// snapThreshold is how close (in px) the window has to be to a
	// screen edge, once the drag is released, before it snaps flush
	// against it.
	snapThreshold = 24
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
		token := backend.GetAccessTokenFull()
		a.setTokens(token.AccessToken, token.RefreshToken)
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

// applyHotkey registers binding for action, swapping out and unregistering
// any hotkey previously registered for that action. The old hotkey is only
// torn down after the new one registers successfully, so a bad binding
// (e.g. one already taken by the OS) doesn't leave the action dead.
func (a *App) applyHotkey(action string, binding hotkeys.HotkeyBinding) error {
	hk, err := hotkeys.BindingToHotkey(binding)
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
	case "loop":
		return a.ToggleLoop
	}
	return func() {}
}

// GetHotkeyConfig returns the currently active hotkey bindings.
func (a *App) GetHotkeyConfig() hotkeys.HotkeyConfig {
	a.hotkeyMu.Lock()
	defer a.hotkeyMu.Unlock()
	return a.hotkeyConfig
}

// SetHotkeyBinding rebinds action to the given modifiers/key, persists it,
// and applies it immediately. The previous binding for action stays active
// if the new one fails to register (e.g. it's already taken by the OS).
func (a *App) SetHotkeyBinding(action string, mods []string, key string) error {
	a.hotkeyMu.Lock()
	if _, ok := a.hotkeyConfig.Binding(action); !ok {
		a.hotkeyMu.Unlock()
		return fmt.Errorf("unknown hotkey action %q", action)
	}
	a.hotkeyMu.Unlock()

	binding := hotkeys.HotkeyBinding{Mods: mods, Key: key}
	if err := a.applyHotkey(action, binding); err != nil {
		return err
	}

	a.hotkeyMu.Lock()
	a.hotkeyConfig.SetBinding(action, binding)
	cfg := a.hotkeyConfig
	a.hotkeyMu.Unlock()

	return hotkeys.SaveConfig(cfg)
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
			a.setAbsoluteWindowPosition(x, y-delta)
		} else {
			a.openedUpward = false
		}
		runtime.WindowSetSize(a.ctx, 320, expandedHeight)
	} else {
		runtime.WindowSetSize(a.ctx, 320, collapsedHeight)
		if a.openedUpward {
			x, y := runtime.WindowGetPosition(a.ctx)
			a.setAbsoluteWindowPosition(x, y+delta)
			a.openedUpward = false
		}
	}
	runtime.EventsEmit(a.ctx, "toggle-settings", a.isExpanded)
}

// setAbsoluteWindowPosition moves the window to an absolute
// virtual-desktop coordinate. On Windows, runtime.WindowSetPosition
// doesn't actually take one - see workAreaOriginAt - so this
// compensates for that before calling through; on other platforms
// it's a passthrough.
func (a *App) setAbsoluteWindowPosition(x, y int) {
	curX, curY := runtime.WindowGetPosition(a.ctx)
	width, height := runtime.WindowGetSize(a.ctx)

	originX, originY, ok := workAreaOriginAt(curX+width/2, curY+height/2)
	if !ok {
		runtime.WindowSetPosition(a.ctx, x, y)
		return
	}
	runtime.WindowSetPosition(a.ctx, x-originX, y-originY)
}

// DragWindowTo moves the window to an absolute virtual-desktop
// coordinate. The frontend calls this on every mousemove while
// dragging (see main.js) instead of calling the Wails runtime's
// WindowSetPosition directly, since only Go can correctly translate an
// absolute target into whatever coordinate space it actually expects
// on this platform.
func (a *App) DragWindowTo(x, y int) {
	a.setAbsoluteWindowPosition(x, y)
}

// currentScreen returns the screen the window currently sits on (or the
// first one reported, if none is flagged current), and false if no
// screen info is available at all.
func (a *App) currentScreen() (runtime.Screen, bool) {
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err != nil || len(screens) == 0 {
		return runtime.Screen{}, false
	}
	for _, s := range screens {
		if s.IsCurrent {
			return s, true
		}
	}
	return screens[0], true
}

// currentScreenHeight returns the logical height of the screen the window
// currently sits on, or 0 if it can't be determined.
func (a *App) currentScreenHeight() int {
	screen, ok := a.currentScreen()
	if !ok {
		return 0
	}
	return screen.Size.Height
}

// SnapWindowToEdges snaps the window flush against any nearby screen
// edge. The frontend drives its own drag (tracking mousedown/mousemove/
// mouseup itself instead of relying on the OS's native drag, which
// swallows the mouseup) and calls this the instant the mouse button is
// released, so the snap only ever fires on an actual release - never
// mid-drag.
func (a *App) SnapWindowToEdges() {
	x, y := runtime.WindowGetPosition(a.ctx)
	a.snapToEdges(x, y)
}

// snapToEdges moves the window flush against any screen edge it's
// within snapThreshold pixels of.
func (a *App) snapToEdges(x, y int) {
	width, height := runtime.WindowGetSize(a.ctx)

	// Prefer the real bounds of whichever monitor the window is
	// actually on (accounts for monitor position on a multi-monitor
	// setup). Fall back to Wails' screen size, assuming it starts at
	// the desktop origin, when there's no native lookup for this OS -
	// only correct for a single monitor, but no worse than before.
	left, top, right, bottom, ok := monitorBoundsAt(x+width/2, y+height/2)
	if !ok {
		screen, found := a.currentScreen()
		if !found {
			return
		}
		left, top = 0, 0
		right, bottom = screen.Size.Width, screen.Size.Height
	}

	snappedX, snappedY := x, y

	if x-left <= snapThreshold {
		snappedX = left
	} else if right-(x+width) <= snapThreshold {
		snappedX = right - width
	}

	if y-top <= snapThreshold {
		snappedY = top
	} else if bottom-(y+height) <= snapThreshold {
		snappedY = bottom - height
	}

	if snappedX != x || snappedY != y {
		a.setAbsoluteWindowPosition(snappedX, snappedY)
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

		newToken, err := backend.RefreshToken(refresh)
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
		a.withDeviceRevival(playback.PausePlayback)
	} else {
		a.withDeviceRevival(playback.PlayPlayback)
	}
}

func (a *App) NextTrack() {
	a.withDeviceRevival(playback.NextTrack)
}

func (a *App) PreviousTrack() {
	a.withDeviceRevival(playback.PreviousTrack)
}

func (a *App) ToggleShuffle() {
	token := a.getToken()
	shuffled, err := playback.IsShuffled(token)
	if err != nil {
		return
	}
	a.withDeviceRevival(func(token string) error {
		return playback.ToggleShuffle(token, !shuffled)
	})
}

// ToggleLoop advances Spotify's repeat mode one step: off -> context
// (repeat playlist/album) -> track (repeat song) -> off.
func (a *App) ToggleLoop() {
	token := a.getToken()
	current, err := playback.GetRepeatState(token)
	if err != nil {
		return
	}
	a.withDeviceRevival(func(token string) error {
		_, err := playback.ToggleLoop(token, current)
		return err
	})
}

// withDeviceRevival runs action and, if it fails specifically because
// Spotify has dropped the active device (which happens after one sits
// idle for a while), revives a device and retries the exact same
// action - so a command that only failed because of the idle timeout
// still ends up doing what it was asked to do. Emits "playback-changed"
// afterwards so the frontend can resync immediately instead of waiting
// for its next periodic poll.
func (a *App) withDeviceRevival(action func(token string) error) {
	token := a.getToken()
	if err := action(token); errors.Is(err, playback.ErrNoActiveDevice) && a.reviveDevice(token) {
		// Transferring playback doesn't take effect instantly - retrying
		// right away tends to hit Spotify before the device is actually
		// marked active again, so the retry itself fails. A short wait
		// gives the transfer time to land first.
		time.Sleep(1500 * time.Millisecond)
		action(token)
	}
	runtime.EventsEmit(a.ctx, "playback-changed")
}

// reviveDevice transfers playback to an available device without
// forcing playback to start - the action that triggered the revival
// gets retried right after, and that's what decides what actually
// happens next. Returns true if a device was found to transfer to.
func (a *App) reviveDevice(token string) bool {
	devices, err := playback.GetDevices(token)
	if err != nil || len(devices) == 0 {
		return false
	}

	target := devices[0]
	for _, d := range devices {
		if !d.IsRestricted {
			target = d
			break
		}
	}

	playback.TransferPlayback(token, target.ID, false)
	return true
}

func (a *App) GetNowPlaying() playback.PlaybackState {
	state, err := playback.NowPlaying(a.getToken())
	if err != nil {
		return playback.PlaybackState{}
	}
	return state
}
