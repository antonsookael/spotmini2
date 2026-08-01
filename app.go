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

	tokenMu        sync.RWMutex
	accessToken    string
	refreshTok     string
	tokenExpiresAt time.Time

	// expandedPanel is "" when collapsed, otherwise "settings" or
	// "playlists" - only one panel can be expanded at a time, since
	// they share the same window-resize mechanism.
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

	volumeMu     sync.Mutex
	lastVolume   int
	lastVolumeAt time.Time
}

const (
	collapsedHeight = 50
	expandedHeight  = 330

	// snapThreshold is how close (in px) the window has to be to a
	// screen edge, once the drag is released, before it snaps flush
	// against it.
	snapThreshold = 24

	// volumeStep is how many percentage points each volume hotkey
	// press changes the active device's volume by.
	volumeStep = 10
)

func NewApp() *App {
	return &App{
		activeHotkeys: make(map[string]*hotkey.Hotkey),
	}
}

// tokenRefreshBuffer is how far ahead of Spotify's actual token expiry
// getToken proactively refreshes, so a call right at the boundary
// doesn't slip through with an already-stale token.
const tokenRefreshBuffer = 60 * time.Second

// getToken returns the current access token, refreshing it first if
// it's expired (or about to). This check runs on every call rather
// than trusting startTokenRefreshLoop's background ticker alone -
// that ticker is driven by Go's monotonic clock, which effectively
// pauses while the Mac is asleep, so its countdown only counts awake
// time. Spotify's actual expiry doesn't care about sleep, so after the
// Mac's been closed for a while the token can genuinely expire in real
// time well before the ticker's internal countdown catches up.
// Checking wall-clock expiry here, on every actual use, is correct
// regardless of how long the machine was asleep.
func (a *App) getToken() string {
	a.tokenMu.RLock()
	token := a.accessToken
	refresh := a.refreshTok
	stale := refresh != "" && time.Now().After(a.tokenExpiresAt.Add(-tokenRefreshBuffer))
	a.tokenMu.RUnlock()

	if !stale {
		return token
	}

	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()

	// Re-check after acquiring the write lock - another goroutine may
	// have already refreshed it while this one was waiting.
	if time.Now().Before(a.tokenExpiresAt.Add(-tokenRefreshBuffer)) {
		return a.accessToken
	}

	newToken, err := backend.RefreshToken(a.refreshTok)
	if err != nil {
		fmt.Println("On-demand token refresh failed:", err)
		return a.accessToken
	}

	a.accessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		a.refreshTok = newToken.RefreshToken
	}
	a.tokenExpiresAt = time.Now().Add(time.Duration(newToken.ExpiresIn) * time.Second)
	fmt.Println("Access token refreshed on demand")

	return a.accessToken
}

func (a *App) setTokens(access, refresh string, expiresIn int) {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	a.accessToken = access
	if refresh != "" {
		a.refreshTok = refresh
	}
	a.tokenExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

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
	case "playlists":
		return a.TogglePlaylistsPanel
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
	case "volumeUp":
		return a.VolumeUp
	case "volumeDown":
		return a.VolumeDown
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
	a.togglePanel("settings")
}

// TogglePlaylistsPanel opens/closes the playlist-picker panel, using the
// same expand/collapse mechanism as ToggleSettingsPanel.
func (a *App) TogglePlaylistsPanel() {
	a.togglePanel("playlists")
}

// togglePanel shows panel (resizing/repositioning the window, same as
// the old ToggleSettingsPanel did) unless it's already the one
// expanded, in which case it collapses back down. Switching directly
// from one panel to the other - without collapsing in between - leaves
// the window's size/position alone, since both panels use the same
// expandedHeight.
func (a *App) togglePanel(panel string) {
	wasExpanded := a.expandedPanel != ""

	if a.expandedPanel == panel {
		a.expandedPanel = ""
	} else {
		a.expandedPanel = panel
	}

	delta := expandedHeight - collapsedHeight
	// Reuse whatever width is currently set, rather than hardcoding
	// 320 - otherwise this would stomp the width auto-fit sets from
	// the frontend (see updateAutoWidth in main.js) the moment the
	// panel was toggled.
	width, _ := runtime.WindowGetSize(a.ctx)

	if a.expandedPanel != "" {
		if !wasExpanded {
			x, y := runtime.WindowGetPosition(a.ctx)
			screenHeight := a.currentScreenHeight()

			if screenHeight > 0 && y+expandedHeight > screenHeight {
				a.openedUpward = true
				a.setAbsoluteWindowPosition(x, y-delta)
			} else {
				a.openedUpward = false
			}
			runtime.WindowSetSize(a.ctx, width, expandedHeight)
		}
		if panel == "playlists" {
			// A global hotkey can fire while some other app is
			// focused - AlwaysOnTop only affects z-order, so without
			// this the window becomes visible but never actually
			// takes keyboard focus, and the frontend's DOM-level
			// .focus() on the search input silently does nothing
			// (keystrokes keep going to whatever app was already
			// focused). Show() explicitly makes this window key and
			// activates the app to fix that.
			runtime.WindowShow(a.ctx)
		}
	} else {
		runtime.WindowSetSize(a.ctx, width, collapsedHeight)
		if a.openedUpward {
			x, y := runtime.WindowGetPosition(a.ctx)
			a.setAbsoluteWindowPosition(x, y+delta)
			a.openedUpward = false
		}
	}
	runtime.EventsEmit(a.ctx, "panel-changed", a.expandedPanel)
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

// BeginDrag captures the coordinate-compensation origin (see
// workAreaOriginAt) once at the start of a drag, so DragWindowTo can
// reuse it on every subsequent frame instead of re-resolving it (two
// syscalls, plus a position/size query) on every single mousemove -
// that per-frame cost was compounding with the OS's own redraw cost
// for a larger window (e.g. with the settings panel open) into
// noticeably laggy dragging.
func (a *App) BeginDrag() {
	x, y := runtime.WindowGetPosition(a.ctx)
	width, height := runtime.WindowGetSize(a.ctx)
	originX, originY, ok := workAreaOriginAt(x+width/2, y+height/2)

	a.dragMu.Lock()
	a.dragActive = true
	a.dragOriginResolved = ok
	a.dragOriginX, a.dragOriginY = originX, originY
	a.dragMu.Unlock()
}

// EndDrag marks the drag as finished, so a stray DragWindowTo call that
// arrives after mouseup (the frontend's own rAF throttling makes this
// unlikely, but not impossible) falls back to resolving its origin
// fresh rather than reusing a now-stale cached one.
func (a *App) EndDrag() {
	a.dragMu.Lock()
	a.dragActive = false
	a.dragMu.Unlock()
}

// DragWindowTo moves the window to an absolute virtual-desktop
// coordinate. The frontend calls this on every mousemove while
// dragging (see main.js) instead of calling the Wails runtime's
// WindowSetPosition directly, since only Go can correctly translate an
// absolute target into whatever coordinate space it actually expects
// on this platform.
func (a *App) DragWindowTo(x, y int) {
	a.dragMu.Lock()
	active := a.dragActive
	resolved := a.dragOriginResolved
	originX, originY := a.dragOriginX, a.dragOriginY
	a.dragMu.Unlock()

	if !active {
		a.setAbsoluteWindowPosition(x, y)
		return
	}
	if !resolved {
		runtime.WindowSetPosition(a.ctx, x, y)
		return
	}
	runtime.WindowSetPosition(a.ctx, x-originX, y-originY)
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

// startTokenRefreshLoop proactively refreshes the token every 50
// minutes while the app is actively running, so a call doesn't have to
// stall on a refresh mid-request in the common case. It's a nice-to-
// have, not the actual correctness guarantee: getToken's own
// wall-clock expiry check is what catches the token going stale during
// a long sleep, since this ticker's countdown can't.
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

		a.setTokens(newToken.AccessToken, newToken.RefreshToken, newToken.ExpiresIn)
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

// VolumeUp raises the active device's volume by volumeStep percentage
// points.
func (a *App) VolumeUp() {
	a.adjustVolume(volumeStep)
}

// VolumeDown lowers the active device's volume by volumeStep
// percentage points.
func (a *App) VolumeDown() {
	a.adjustVolume(-volumeStep)
}

// volumeCacheWindow is how long adjustVolume trusts the volume it last
// applied itself instead of re-fetching from Spotify. Rapid successive
// presses land faster than Spotify's read API reliably reflects a
// just-written value, so re-fetching "current" on every single press
// can read a stale pre-change volume and silently compute the same
// target twice, swallowing one of the presses. A short cache sidesteps
// that while still resyncing with any volume change made outside this
// app (Spotify's own UI, another device) after a brief idle period.
const volumeCacheWindow = 2 * time.Second

// adjustVolume changes the active device's volume by delta percentage
// points relative to its current value, then emits "volume-changed" so
// the frontend can show an indicator. Only caches/emits when the
// change actually landed - withDeviceRevival's return value tells us
// that, rather than assuming success the way the other playback
// actions get away with (they're idempotent-ish and self-correct via
// the next poll; volume isn't polled anywhere, so a failed change
// still "succeeding" here would desync lastVolume from Spotify's real
// value with nothing to ever notice or fix it).
//
// volumeMu is held for the whole read-adjust-write, not just around the
// cache access - volumeUp and volumeDown are separate hotkey actions,
// each watched by its own goroutine (see watchHotkey), so pressing both
// in quick succession runs two of these concurrently. Only serializing
// the cache access would still let both read the same starting point
// before either's write lands.
func (a *App) adjustVolume(delta int) {
	token := a.getToken()

	a.volumeMu.Lock()
	defer a.volumeMu.Unlock()

	current := a.lastVolume
	if time.Since(a.lastVolumeAt) >= volumeCacheWindow {
		var err error
		current, err = playback.GetVolume(token)
		if err != nil {
			return
		}
	}

	var applied int
	err := a.withDeviceRevival(func(token string) error {
		var err error
		applied, err = playback.SetVolume(token, current+delta)
		return err
	})
	if err != nil {
		return
	}

	a.lastVolume = applied
	a.lastVolumeAt = time.Now()
	runtime.EventsEmit(a.ctx, "volume-changed", applied)
}

// withDeviceRevival runs action and, if it fails specifically because
// Spotify has dropped the active device (which happens after one sits
// idle for a while), revives a device and retries the exact same
// action - so a command that only failed because of the idle timeout
// still ends up doing what it was asked to do. Emits "playback-changed"
// afterwards so the frontend can resync immediately instead of waiting
// for its next periodic poll, or "premium-required" instead if the
// account just isn't allowed to do this at all (a Free account gets
// this on every control command, not just an idle-device edge case -
// without surfacing it, the app just looks broken with no explanation).
// Returns the final error from action, for callers (like adjustVolume)
// that need to know whether it ultimately succeeded - most callers just
// ignore it, same fire-and-forget as before.
func (a *App) withDeviceRevival(action func(token string) error) error {
	token := a.getToken()
	err := action(token)
	if errors.Is(err, playback.ErrNoActiveDevice) && a.reviveDevice(token) {
		// Transferring playback doesn't take effect instantly - retrying
		// right away tends to hit Spotify before the device is actually
		// marked active again, so the retry itself fails. A short wait
		// gives the transfer time to land first.
		time.Sleep(1500 * time.Millisecond)
		err = action(token)
	}
	if errors.Is(err, playback.ErrPremiumRequired) {
		runtime.EventsEmit(a.ctx, "premium-required")
	}
	runtime.EventsEmit(a.ctx, "playback-changed")
	return err
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

// GetPlaylists lists the current user's playlists, for the playlist
// picker panel.
func (a *App) GetPlaylists() []playback.Playlist {
	playlists, err := playback.GetPlaylists(a.getToken())
	if err != nil {
		return nil
	}
	return playlists
}

// PlayPlaylist starts playback of the playlist at uri from the
// beginning.
func (a *App) PlayPlaylist(uri string) {
	a.withDeviceRevival(func(token string) error {
		return playback.PlayPlaylist(token, uri)
	})
}

// PlayLikedSongs starts playback of the user's most recently saved
// tracks - Liked Songs has no playlist URI to hand to PlayPlaylist, so
// this fetches actual track URIs first and plays those directly.
func (a *App) PlayLikedSongs() {
	token := a.getToken()
	uris, err := playback.GetLikedSongURIs(token)
	if err != nil {
		fmt.Println("Failed to fetch liked songs:", err)
		return
	}
	if len(uris) == 0 {
		return
	}
	a.withDeviceRevival(func(token string) error {
		return playback.PlayURIs(token, uris)
	})
}
