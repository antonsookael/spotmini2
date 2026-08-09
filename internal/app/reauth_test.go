package app

import (
	"testing"
	"time"
)

// The endless login loop: a sign-in completes, the very next request is
// rejected anyway, and that starts another sign-in. With GetNowPlaying
// polling every ten seconds it opened a browser tab every ten seconds
// for as long as the app was left running.
func TestOneCompletedSignInIsTheWholeBudget(t *testing.T) {
	a := &App{}

	if decision, _ := a.claimReauth(); decision != reauthClaimed {
		t.Fatalf("first sign-in: got %v, want reauthClaimed", decision)
	}

	// Finish it, the way the real flow does on success.
	a.reauthorizing = false
	a.reauthorizedAt = time.Now()

	decision, _ := a.claimReauth()
	if decision != reauthOnCooldown {
		t.Errorf("second sign-in: got %v, want reauthOnCooldown - this is the loop", decision)
	}
}

// The refusal reason has to be explicit. A sign-in that finished
// microseconds ago reports zero elapsed time on a coarse clock, so
// inferring the reason from the duration read a cooldown as an
// in-flight attempt - and silently dropped the user's only message.
func TestCooldownIsReportedEvenWhenNoTimeHasPassed(t *testing.T) {
	a := &App{reauthorizedAt: time.Now()}

	decision, since := a.claimReauth()
	if decision != reauthOnCooldown {
		t.Fatalf("got %v, want reauthOnCooldown", decision)
	}
	if since < 0 {
		t.Errorf("since = %v, want a non-negative duration", since)
	}
}

// The claim has to be exclusive, or a burst of rejected requests each
// opens its own tab before any of them records having tried.
func TestOnlyOneSignInAtATime(t *testing.T) {
	a := &App{}

	if decision, _ := a.claimReauth(); decision != reauthClaimed {
		t.Fatalf("first caller: got %v, want reauthClaimed", decision)
	}

	if decision, _ := a.claimReauth(); decision != reauthInFlight {
		t.Errorf("second caller: got %v, want reauthInFlight", decision)
	}
}

// Once the cooldown is behind it, a fresh attempt is allowed again -
// somebody who fixes things at Spotify's end shouldn't have to relaunch.
func TestSignInIsAllowedAgainAfterTheCooldown(t *testing.T) {
	a := &App{reauthorizedAt: time.Now().Add(-reauthCooldown - time.Minute)}

	if decision, _ := a.claimReauth(); decision != reauthClaimed {
		t.Errorf("past the cooldown: got %v, want reauthClaimed", decision)
	}
}

// A flow that failed or was abandoned proves nothing about the account,
// so it must not spend the single attempt.
func TestAFailedSignInDoesNotSpendTheBudget(t *testing.T) {
	a := &App{}

	if decision, _ := a.claimReauth(); decision != reauthClaimed {
		t.Fatalf("first sign-in: got %v, want reauthClaimed", decision)
	}
	// reauthorize's deferred cleanup runs on every path out; only the
	// success path stamps reauthorizedAt.
	a.reauthorizing = false

	if decision, _ := a.claimReauth(); decision != reauthClaimed {
		t.Errorf("after a failed sign-in: got %v, want reauthClaimed", decision)
	}
}
