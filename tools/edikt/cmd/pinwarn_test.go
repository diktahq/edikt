package cmd

import (
	"strings"
	"testing"
)

// THE DIRECTION DECIDES THE REMEDY.
//
// `edikt upgrade-pin` rewrites the pin to match whatever binary is installed.
// That is the fix in one direction and a silent downgrade in the other: a
// project pinned 0.4.5 running a 0.4.3 binary would have its declared version
// moved BACKWARD, resolving the warning by discarding the fact it reported.
// The warning gave that advice unconditionally until this test existed.
func TestPinRemedy_PinAheadDoesNotRecommendUpgradePin(t *testing.T) {
	got := pinRemedy("0.4.5", "0.4.3")
	if !strings.Contains(got, "AHEAD") {
		t.Errorf("remedy does not say the pin is ahead, so the user cannot tell which way to move:\n%s", got)
	}
	if !strings.Contains(got, "do NOT run") {
		t.Errorf("remedy fails to warn against `upgrade-pin`, which here would downgrade the pin:\n%s", got)
	}
	if !strings.Contains(got, "Upgrade the binary") {
		t.Errorf("remedy does not name the actual fix (upgrade the binary):\n%s", got)
	}
}

// The other direction is the ordinary case and `upgrade-pin` really is right.
func TestPinRemedy_PinBehindRecommendsUpgradePin(t *testing.T) {
	got := pinRemedy("0.7.1", "0.8.0")
	if !strings.Contains(got, "upgrade-pin") {
		t.Errorf("a pin behind the install should be moved forward with `upgrade-pin`:\n%s", got)
	}
	if strings.Contains(got, "do NOT run") {
		t.Errorf("the ahead-warning leaked into the behind case, telling users not to run the correct fix:\n%s", got)
	}
}

// THE FLIP RECORDED IN docs/internal/plans/verdicts/SPEC-024-flip-list.md,
// ENTRY (c). This test used to forbid any direction between 0.8.1-rc1 and
// 0.8.1, on the stated ground that the ordering was undefined. It was not
// undefined, it was UNSEEN: the truncating parser read both as [0,8,1].
//
// A pre-release sorts BEFORE its release (semver 2.0.0 §11.3), so the direction
// IS defensible and the honest answer is to state it: a pin of 0.8.1-rc1
// against an installed 0.8.1 is BEHIND, and `upgrade-pin` moves it forward onto
// the release rather than backward onto the rc. PC-03 is the mirror direction.
//
// THE SUBJECT DID NOT CHANGE: pinRemedy("0.8.1-rc1", "0.8.1"), pinned first and
// active second. Reversing the arguments turns this into PC-03 and silently
// drops the case.
func TestPinRemedy_PreReleasePinIsBehindItsRelease(t *testing.T) {
	got := pinRemedy("0.8.1-rc1", "0.8.1")
	if !strings.Contains(got, "behind") {
		t.Errorf("remedy does not say the pin is behind, so the user cannot tell which way to move:\n%s", got)
	}
	if strings.Contains(got, "AHEAD") {
		t.Errorf("remedy claims the pin is ahead of an install that is strictly newer:\n%s", got)
	}
}

// The fourth answer, and the one this project itself produced on every command
// until SR-002 landed: a pin against a CHANNEL. `dev` has no position in a
// release ordering, so neither direction is available and claiming one — as
// the truncating parser did, by reading `dev` as [0,0,0] — told every developer
// here that their pin was AHEAD and warned them off the fix.
func TestPinRemedy_ChannelInstallHasNoDirection(t *testing.T) {
	got := pinRemedy("0.8.1-rc1", "dev")
	if strings.Contains(got, "AHEAD") {
		t.Errorf("remedy claims the pin is AHEAD of a channel install, which is the defect SR-002 names verbatim:\n%s", got)
	}
	if strings.Contains(got, "behind") {
		t.Errorf("remedy claims a direction against a channel name, which has no position in a release ordering:\n%s", got)
	}
	if !strings.Contains(got, "not comparable") {
		t.Errorf("remedy does not SAY the two are not comparable, so the user is left to infer it from silence:\n%s", got)
	}
}
