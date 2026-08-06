package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	// "playlists" - only one can be open at a time, since they share
	// the same window-resize mechanism. Guarded because hotkeys fire on
	// their own goroutines (see watchHotkey) while the frontend calls
	// the same togglePanel from another.
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

	volumeMu     sync.Mutex
	lastVolume   int
	lastVolumeAt time.Time
}

const (
	collapsedHeight = 50
	expandedHeight  = 355

	// snapThreshold is how close (in px) the window has to be to a
	// screen edge, once the drag is released, before it snaps flush
	// against it.
	snapThreshold = 24

	// volumeStep is how many percentage points each volume hotkey
	// press changes the active device's volume by.
	volumeStep = 10

	// previousRestartMs is how far into a track Previous restarts it
	// rather than skipping to the one before.
	previousRestartMs = 5000
)

func NewApp() *App {
	return &App{
		activeHotkeys: make(map[string]*hotkey.Hotkey),
	}
}

// Refresh this far before the real expiry, so a call landing right on
// the boundary doesn't use an already-stale token.
const tokenRefreshBuffer = 60 * time.Second

// getToken returns the access token, refreshing first if it's expired.
// Checks wall-clock expiry on every call rather than trusting
// startTokenRefreshLoop: that ticker uses the monotonic clock, which
// pauses while the machine sleeps, so it under-counts real elapsed
// time and can leave the token stale after a long sleep.
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

	// Another goroutine may have refreshed while we waited for the lock.
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

// applyHotkey registers binding for action, replacing any previous one.
// The old hotkey is only torn down once the new one registers, so a
// rejected binding doesn't leave the action dead.
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
	case "alwaysOnTop":
		return a.ToggleAlwaysOnTop
	}
	return func() {}
}

// GetHotkeyConfig returns the currently active hotkey bindings.
func (a *App) GetHotkeyConfig() hotkeys.HotkeyConfig {
	a.hotkeyMu.Lock()
	defer a.hotkeyMu.Unlock()
	return a.hotkeyConfig
}

// SetHotkeyBinding rebinds action, persists it, and applies it now.
// The previous binding stays active if the new one won't register.
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

// ToggleAlwaysOnTop lets the frontend flip the setting - it owns the
// value (localStorage) and the checkbox, so flipping it here would
// desync both.
func (a *App) ToggleAlwaysOnTop() {
	runtime.EventsEmit(a.ctx, "toggle-always-on-top")
}

// togglePanel expands panel, or collapses if it's already the open
// one. Switching straight between panels leaves the window size and
// position alone, since both use expandedHeight.
func (a *App) togglePanel(panel string) {
	a.panelMu.Lock()
	defer a.panelMu.Unlock()

	wasExpanded := a.expandedPanel != ""

	if a.expandedPanel == panel {
		a.expandedPanel = ""
	} else {
		a.expandedPanel = panel
	}

	delta := expandedHeight - collapsedHeight
	// Reuse the current width rather than hardcoding 320, which would
	// stomp whatever auto-fit set (see updateAutoWidth in main.js).
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
			// The hotkey can fire while another app is focused, and
			// AlwaysOnTop only affects z-order - without this the
			// window shows but never takes keyboard focus, so the
			// search input's .focus() does nothing.
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

// setAbsoluteWindowPosition moves the window to an absolute desktop
// coordinate. Windows needs compensation first (see workAreaOriginAt);
// elsewhere it's a passthrough.
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

// BeginDrag resolves the coordinate-compensation origin (see
// workAreaOriginAt) once per drag so DragWindowTo can reuse it. Doing
// it per-mousemove instead cost two syscalls plus a size query every
// frame, which made dragging visibly laggy.
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

// EndDrag marks the drag finished, so a stray DragWindowTo arriving
// after mouseup resolves a fresh origin instead of a stale cached one.
func (a *App) EndDrag() {
	a.dragMu.Lock()
	a.dragActive = false
	a.dragMu.Unlock()
}

// DragWindowTo moves the window to an absolute desktop coordinate.
// The frontend calls this per mousemove rather than the Wails runtime
// directly, since only Go can translate into the coordinate space each
// platform actually expects.
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

// SnapWindowToEdges snaps the window flush against a nearby screen
// edge. Called by the frontend on mouseup, so it only ever fires on a
// real release, never mid-drag.
func (a *App) SnapWindowToEdges() {
	x, y := runtime.WindowGetPosition(a.ctx)
	a.snapToEdges(x, y)
}

// snapToEdges moves the window flush against any screen edge it's
// within snapThreshold pixels of.
func (a *App) snapToEdges(x, y int) {
	width, height := runtime.WindowGetSize(a.ctx)

	// Prefer the real bounds of the monitor the window is on. Falling
	// back to Wails' screen size assumes a desktop origin, so it's only
	// correct on a single monitor.
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

// startTokenRefreshLoop refreshes every 50 minutes so calls rarely
// stall on a refresh. Just an optimisation - getToken's expiry check
// is what actually guarantees a valid token.
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

// PreviousTrack restarts the current track once you're past
// previousRestartMs into it, and only skips back before that - the same
// behaviour as Spotify's own clients. Falls back to skipping if the
// position can't be read, since that's the pre-existing behaviour and
// better than doing nothing.
func (a *App) PreviousTrack() {
	state, err := playback.NowPlaying(a.getToken())
	if err == nil && state.ProgressMs > previousRestartMs {
		a.withDeviceRevival(func(token string) error {
			return playback.Seek(token, 0)
		})
		return
	}
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

// How long adjustVolume trusts its own last-applied value instead of
// re-reading. Spotify's read API lags a just-written volume, so rapid
// presses would keep reading the pre-change value and compute the same
// target twice, swallowing presses. Short enough to still pick up
// changes made elsewhere.
const volumeCacheWindow = 2 * time.Second

// adjustVolume shifts the active device's volume by delta and emits
// "volume-changed" for the indicator. Only caches/emits on success:
// volume is never polled, so a failed change recorded as applied would
// desync lastVolume permanently.
//
// The lock covers the whole read-adjust-write, not just the cache -
// volumeUp and volumeDown run on separate goroutines, so guarding only
// the cache would still let both read the same starting value.
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

// withDeviceRevival runs action, and retries it once after reviving a
// device if Spotify had dropped the active one for being idle. Emits
// "playback-changed" so the frontend resyncs immediately - except on a
// Premium rejection, which emits "premium-required" instead and skips
// the resync that would otherwise wipe that message. Returns action's
// final error; most callers ignore it.
func (a *App) withDeviceRevival(action func(token string) error) error {
	token := a.getToken()
	err := action(token)
	if errors.Is(err, playback.ErrNoActiveDevice) && a.reviveDevice(token) {
		// The transfer isn't instant; retrying immediately hits
		// Spotify before the device is marked active again.
		time.Sleep(1500 * time.Millisecond)
		err = action(token)
	}
	if errors.Is(err, playback.ErrPremiumRequired) {
		runtime.EventsEmit(a.ctx, "premium-required")
		return err
	}
	runtime.EventsEmit(a.ctx, "playback-changed")
	return err
}

// reviveDevice transfers playback to an available device without
// starting it - the retried action decides what happens next. Returns
// true if a device was found.
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

// PlayLikedSongs plays the most recently saved tracks. Liked Songs has
// no playlist URI, so it plays explicit track URIs instead.
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

// SearchTracks searches Spotify's catalog for the picker's search box.
// A blank query short-circuits rather than hitting the API.
func (a *App) SearchTracks(query string) []playback.TrackResult {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	results, err := playback.SearchTracks(a.getToken(), query, 8)
	if err != nil {
		fmt.Println("Track search failed:", err)
		return nil
	}
	return results
}

// PlayTrack starts playback of a single track by URI - used when
// picking a song from search results.
func (a *App) PlayTrack(uri string) {
	a.withDeviceRevival(func(token string) error {
		return playback.PlayURIs(token, []string{uri})
	})
}

// SaveTrackToLiked adds a track to the current user's Liked Songs.
func (a *App) SaveTrackToLiked(id string) {
	if err := playback.SaveTrack(a.getToken(), id); err != nil {
		fmt.Println("Failed to save track:", err)
	}
}

// IsAutostartEnabled reports whether spotmini is currently set to
// launch automatically at login. Queries the OS directly (registry key
// on Windows, LaunchAgent file on macOS) rather than a cached setting,
// since it can be changed outside the app (Task Manager's Startup tab,
// deleting the LaunchAgent by hand, etc.).
func (a *App) IsAutostartEnabled() bool {
	return isAutostartEnabled()
}

// SetAutostart enables or disables launching spotmini automatically at
// login.
func (a *App) SetAutostart(enabled bool) error {
	return setAutostart(enabled)
}
