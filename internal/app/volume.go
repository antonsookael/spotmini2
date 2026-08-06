package app

import (
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/playback"
)

const (
	// volumeStep is how many percentage points each volume hotkey press
	// changes the active device's volume by.
	volumeStep = 10

	// volumeCacheWindow is how long adjustVolume trusts its own
	// last-applied value instead of re-reading. Spotify's read API lags
	// a just-written volume, so rapid presses would keep reading the
	// pre-change value and compute the same target twice, swallowing
	// presses. Short enough to still pick up changes made elsewhere.
	volumeCacheWindow = 2 * time.Second
)

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
