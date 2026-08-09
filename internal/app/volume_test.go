package app

import (
	"sync"
	"testing"
	"time"
)

// Presses arriving while a change is in flight must fold into the one
// already running, not queue behind it. Queued, each landed as its own
// write seconds after the user stopped pressing - so five taps during a
// slow device revival ramped the volume long after the fact.
func TestAdjustVolumeFoldsConcurrentPresses(t *testing.T) {
	a := &App{}

	// Stand in for the in-flight change: claim the flag the way
	// adjustVolume does, then let more presses arrive.
	a.volumeMu.Lock()
	a.applyingVolume = true
	a.volumeMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.adjustVolume(volumeStep)
		}()
	}
	wg.Wait()

	a.volumeMu.Lock()
	pending := a.pendingDelta
	a.volumeMu.Unlock()

	if pending != 5*volumeStep {
		t.Errorf("pendingDelta = %d, want %d - presses were dropped or not folded", pending, 5*volumeStep)
	}
}

// Opposite directions have to cancel rather than both being applied:
// pressing up then down during one revival should end where it started.
func TestAdjustVolumeCancelsOpposingPresses(t *testing.T) {
	a := &App{}
	a.volumeMu.Lock()
	a.applyingVolume = true
	a.lastVolumeAt = time.Now()
	a.volumeMu.Unlock()

	a.adjustVolume(volumeStep)
	a.adjustVolume(-volumeStep)

	a.volumeMu.Lock()
	defer a.volumeMu.Unlock()
	if a.pendingDelta != 0 {
		t.Errorf("pendingDelta = %d, want 0", a.pendingDelta)
	}
}
