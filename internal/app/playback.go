package app

import (
	"errors"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
)

// previousRestartMs is how far into a track Previous restarts it rather
// than skipping to the one before.
const previousRestartMs = 5000

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

func (a *App) GetNowPlaying() playback.PlaybackState {
	state, err := playback.NowPlaying(a.getToken())
	if err != nil {
		return playback.PlaybackState{}
	}
	return state
}

// revivalRetryDelays is how long to wait before each retry after
// reviving a device. The transfer isn't instant, so retrying
// immediately just gets rejected again - but a single fixed wait long
// enough for the worst case makes every revival feel that slow. Backing
// off instead means the common case lands on the first retry, while
// still allowing longer overall than one wait would.
var revivalRetryDelays = []time.Duration{
	300 * time.Millisecond,
	500 * time.Millisecond,
	900 * time.Millisecond,
}

// withDeviceRevival runs action, retrying it after reviving a device if
// Spotify had dropped the active one for being idle. Emits
// "playback-changed" so the frontend resyncs immediately - except on a
// Premium rejection, which emits "premium-required" instead and skips
// the resync that would otherwise wipe that message. Returns action's
// final error; most callers ignore it.
func (a *App) withDeviceRevival(action func(token string) error) error {
	token := a.getToken()
	err := action(token)
	if errors.Is(err, playback.ErrNoActiveDevice) && a.reviveDevice(token) {
		// Safe to repeat even for something like NextTrack, which would
		// skip twice if it half-applied: these are rejections, so
		// nothing happened to repeat.
		for _, delay := range revivalRetryDelays {
			time.Sleep(delay)
			err = action(token)

			// Only a success or a flat refusal ends this. Retrying just
			// while the error stays ErrNoActiveDevice looks right but
			// loses the command: a device that's still coming up gets
			// rejected with plenty of other codes too, which fall
			// through request.go to a generic error - and treating one
			// of those as final gave up on the very case this exists to
			// handle.
			if err == nil || errors.Is(err, playback.ErrPremiumRequired) {
				break
			}
			logging.Printf("Retrying after device revival: %v", err)
		}
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
