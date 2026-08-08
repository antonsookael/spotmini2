package app

import (
	"errors"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
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
		current, err = a.currentVolume(token)
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

// currentVolume reads the active device's volume, reviving a device
// first if the only reason there's nothing to read is that Spotify
// dropped the active one for being idle.
//
// Reviving here rather than leaving it to the SetVolume call below is
// what makes the volume hotkeys work at all on an idle device. The
// change is relative, so it needs a starting point, and /me/player
// answers 204 with no body until something is active again - which used
// to be read as "currently 0", turning a Vol+ press into "set the
// volume to one step" and dropping a device that had been at 70% to
// 10% the moment the retry landed on it.
func (a *App) currentVolume(token string) (int, error) {
	volume, err := playback.GetVolume(token)
	if !errors.Is(err, playback.ErrNoActiveDevice) {
		return volume, err
	}

	if !a.reviveDevice(token) {
		// Nothing connected to revive, so say so rather than leaving the
		// keypress looking ignored - the same warning the playback
		// commands give in this situation.
		runtime.EventsEmit(a.ctx, "no-device")
		return 0, err
	}

	// The transfer isn't instant, so the same backoff withDeviceRevival
	// uses applies to reading the volume back off the revived device.
	for _, delay := range revivalRetryDelays {
		time.Sleep(delay)
		if volume, err = playback.GetVolume(token); err == nil {
			return volume, nil
		}
	}
	logging.Printf("Could not read volume after device revival: %v", err)
	return 0, err
}
