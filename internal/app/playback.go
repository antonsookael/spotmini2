package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
)

// previousRestartMs is how far into a track Previous restarts it rather
// than skipping to the one before.
const previousRestartMs = 5000

// reportCommandFailure surfaces a command that gave up before it could
// run, instead of letting it look like a dead hotkey.
//
// The read-then-write commands can't carry on without their read -
// play/pause has to know which one it is, and repeat has to know what
// to advance from - so bailing is right. Bailing *quietly* was not:
// a rejected token, a missing OAuth scope and an unregistered hotkey
// all presented identically, as a keypress that did nothing at all,
// with nothing on screen and nothing in the log to tell them apart.
func (a *App) reportCommandFailure(name string, err error) {
	if errors.Is(err, playback.ErrPremiumRequired) {
		runtime.EventsEmit(a.ctx, "premium-required")
		return
	}
	if errors.Is(err, playback.ErrNoActiveDevice) {
		runtime.EventsEmit(a.ctx, "no-device")
		return
	}

	logging.Printf("[command] %s abandoned - could not read playback state: %v", name, err)
	runtime.EventsEmit(a.ctx, "command-failed", failureMessage(err))
}

// failureMessage is what the bar says about err. Every branch names a
// cause, because the bar is the only place this can be said: a released
// build has no console, and telling someone to go and find a log file is
// telling them nothing.
//
// Kept short enough to fit the bar at its default width - anything
// longer is silently ellipsized, which loses the end of the sentence,
// where the useful half tends to be.
func failureMessage(err error) string {
	switch {
	case errors.Is(err, playback.ErrUnreachable):
		return "No connection to Spotify"
	case errors.Is(err, playback.ErrAuthExpired):
		// Only reaches the bar once a refresh has already been tried and
		// failed, so the login genuinely needs redoing - which restarting
		// does, by falling through to the browser flow.
		return "Spotify sign-in expired - restart app"
	case errors.Is(err, playback.ErrRateLimited):
		return "Too many requests - wait a moment"
	case errors.Is(err, playback.ErrSpotifyDown):
		return "Spotify is down - try again later"
	}

	// Nothing recognised it, so the status code goes in the message
	// itself. Meaningless to most people, but it's a thing they can
	// quote in a bug report, which "something went wrong" is not.
	var statusErr *playback.StatusError
	if errors.As(err, &statusErr) {
		return fmt.Sprintf("Spotify error %d - try again", statusErr.Status)
	}
	return "Spotify request failed - try again"
}

func (a *App) PlayPause() {
	playing, err := retryOnAuthFailure(a, playback.IsPlaying)
	if err != nil {
		a.reportCommandFailure("playPause", err)
		return
	}
	if playing {
		a.withDeviceRevival("pause", playback.PausePlayback)
	} else {
		a.withDeviceRevival("play", playback.PlayPlayback)
	}
}

func (a *App) NextTrack() {
	a.withTrackChange("next", playback.NextTrack)
}

// PreviousTrack restarts the current track once you're past
// previousRestartMs into it, and only skips back before that - the same
// behaviour as Spotify's own clients. Falls back to skipping if the
// position can't be read, since that's the pre-existing behaviour and
// better than doing nothing.
func (a *App) PreviousTrack() {
	state, err := retryOnAuthFailure(a, playback.NowPlaying)
	if err == nil && state.ProgressMs > previousRestartMs {
		// Restarting the same track, so no change to wait for - only the
		// skip below lands somewhere new.
		a.withDeviceRevival("restart", func(token string) error {
			return playback.Seek(token, 0)
		})
		return
	}
	a.withTrackChange("previous", playback.PreviousTrack)
}

func (a *App) ToggleShuffle() {
	shuffled, err := retryOnAuthFailure(a, playback.IsShuffled)
	if err != nil {
		a.reportCommandFailure("shuffle", err)
		return
	}
	a.withDeviceRevival("shuffle", func(token string) error {
		return playback.ToggleShuffle(token, !shuffled)
	})
}

// ToggleLoop advances Spotify's repeat mode one step: off -> context
// (repeat playlist/album) -> track (repeat song) -> off.
func (a *App) ToggleLoop() {
	current, err := retryOnAuthFailure(a, playback.GetRepeatState)
	if err != nil {
		a.reportCommandFailure("loop", err)
		return
	}
	a.withDeviceRevival("loop", func(token string) error {
		_, err := playback.ToggleLoop(token, current)
		return err
	})
}

func (a *App) GetNowPlaying() playback.PlaybackState {
	state, err := retryOnAuthFailure(a, playback.NowPlaying)
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

// withDeviceRevival runs a command that leaves the current track alone -
// play/pause, shuffle, repeat, volume. See runPlayerCommand.
func (a *App) withDeviceRevival(name string, action func(token string) error) error {
	return a.runPlayerCommand(name, false, action)
}

// withTrackChange runs a command that lands on a *different* track, and
// tells the frontend to expect one.
//
// The distinction is the whole point. Spotify's read API lags its own
// writes, so the resync fired straight after a skip routinely comes back
// still describing the track we just skipped away from - and the bar
// then sat on that stale title until the next ten-second poll, which
// reads as the skip having done nothing at all. Knowing a change is
// coming lets the frontend keep reading until Spotify agrees, instead of
// trusting the first answer. It can't infer that itself: play/pause and
// shuffle emit the same event and legitimately keep the same track, so
// an unconditional retry loop would spin on every one of them.
func (a *App) withTrackChange(name string, action func(token string) error) error {
	return a.runPlayerCommand(name, true, action)
}

// runPlayerCommand runs action, retrying it after reviving a device if
// Spotify had dropped the active one for being idle. Emits
// "playback-changed" so the frontend resyncs immediately - except on a
// Premium rejection, which emits "premium-required" instead and skips
// the resync that would otherwise wipe that message. Returns action's
// final error; most callers ignore it.
//
// name is what the command is called in the timing line printed on the
// way out - "next", "shuffle" and so on. Without it the durations can't
// be told apart, since every command reaches Spotify through the same
// two helpers.
func (a *App) runPlayerCommand(name string, expectTrackChange bool, action func(token string) error) error {
	start := time.Now()
	// Reported on every path out, including the failures - a command
	// that gave up after a device revival is exactly the one worth
	// knowing the duration of.
	defer func() {
		fmt.Printf("[command] %s finished in %s\n", name, time.Since(start).Round(time.Millisecond))
	}()

	token := a.getToken()
	err := action(token)

	// A token Spotify rejects is worth one more go with a fresh one,
	// before any of the device handling below. getToken only knows when
	// the token was meant to expire; a 401 is the only thing that knows
	// it actually has - revoked from the account page, invalidated by a
	// password change, or the stale one getToken falls back to when its
	// own refresh failed. Recovering here is why this no longer surfaces
	// as a failed command the user is told to restart out of.
	if errors.Is(err, playback.ErrAuthExpired) && a.refreshNow() {
		token = a.getToken()
		err = action(token)
	}

	revived, reviveErr := false, error(nil)
	if errors.Is(err, playback.ErrNoActiveDevice) {
		revived, reviveErr = a.reviveDevice(token)
	}
	if errors.Is(reviveErr, playback.ErrPremiumRequired) {
		// The transfer named the real reason the command was refused.
		// "No active device" was the truth about the device and a red
		// herring about the cause: a Free account can't drive playback
		// at all, so the retries below would only re-earn the same
		// answer wearing a less useful error.
		err = reviveErr
	} else if revived {
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
	// Still nothing to play on, either because there was no device to
	// revive or because reviving didn't take. Worth saying out loud:
	// otherwise the command just appears to do nothing.
	if errors.Is(err, playback.ErrNoActiveDevice) {
		runtime.EventsEmit(a.ctx, "no-device")
		return err
	}
	// The start time rides along so the frontend can report a total that
	// runs from the keypress rather than from whenever this event
	// happened to reach it - the confirmation reads are most of the wait
	// on a skip, and timing only the Go half would hide that.
	runtime.EventsEmit(a.ctx, "playback-changed", expectTrackChange, start.UnixMilli())
	return err
}

// ReportTrackChange logs how long a skip took to become visible in the
// bar, measured from the keypress that started it. Called by the
// frontend once its confirmation reads settle, since only it knows when
// the new title actually went on screen.
func (a *App) ReportTrackChange(totalMs int, reads int, confirmed bool) {
	if !confirmed {
		// Gave up without Spotify ever reporting a different track. Goes
		// to the log file rather than stdout: the bar is showing a track
		// that may well be the wrong one, which is the failure this
		// whole path exists to catch.
		logging.Printf("[command] track change unconfirmed after %dms and %d reads", totalMs, reads)
		return
	}
	fmt.Printf("[command] track change visible after %dms and %d reads\n", totalMs, reads)
}

// reviveDevice transfers playback to an available device without
// starting it - the retried action decides what happens next. Reports
// whether it's worth retrying, and the error the transfer was refused
// with, if any.
//
// That error used to be discarded, which hid the one thing it was best
// placed to reveal. Transferring is a player command like any other, so
// a Free account is refused here just as it would be for the original
// command - but with the refusal thrown away, the retried command kept
// answering "no active device" and the user was told Spotify wasn't
// open when the real answer was that the account can't control playback
// at all.
func (a *App) reviveDevice(token string) (revived bool, transferErr error) {
	devices, err := playback.GetDevices(token)
	if err != nil {
		logging.Printf("Device revival: could not list devices: %v", err)
		return false, err
	}

	// An empty list is a different problem from an idle device: nothing
	// is connected to Spotify Connect at all, so the app is shut rather
	// than merely asleep. There's nothing to transfer to, and the Web
	// API can't start a closed app - the caller warns instead.
	if len(devices) == 0 {
		logging.Printf("Device revival: no devices available - Spotify isn't open anywhere")
		return false, nil
	}

	target := devices[0]
	for _, d := range devices {
		if !d.IsRestricted {
			target = d
			break
		}
	}

	if err := playback.TransferPlayback(token, target.ID, false); err != nil {
		logging.Printf("Device revival: transfer to %q was refused: %v", target.Name, err)

		// Only a flat refusal ends it here. Anything else could just be
		// the device still coming up, which is precisely what the
		// caller's retries exist for - so the transfer is still worth
		// following through on.
		if errors.Is(err, playback.ErrPremiumRequired) {
			return false, err
		}
	}
	return true, nil
}
