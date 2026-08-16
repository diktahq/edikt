package cmd

import (
	"regexp"
	"strings"
	"testing"
)

// versionShaped matches anything a reader or a grep would take for a
// release version: digits and dots, optionally with a leading v and a
// prerelease suffix.
var versionShaped = regexp.MustCompile(`^v?\d+\.\d+`)

// TestCompilerVersionStamp_DevIsNotVersionShaped pins the provenance rule.
//
// A plain `go build .` leaves Version at its "dev" fallback, and init fed
// that straight into govrun.CompilerVersion, so every artifact a
// locally-built binary compiled was stamped `gov-compile vdev`. This
// repo's own governance.md carries that stamp today.
//
// The problem is not the word "dev" — it is that "vdev" is shaped like a
// version. It sits in the same field, with the same "v" prefix, as a real
// release, so nothing reading the artifact can tell a release-built
// compile from someone's working copy. An unversioned build reporting a
// version-shaped string is the same defect as an unmeasured control
// reporting a pass.
func TestCompilerVersionStamp_DevIsNotVersionShaped(t *testing.T) {
	stamp, released := compilerVersionStamp("dev")

	if released {
		t.Error(`"dev" reported as a released version`)
	}
	if versionShaped.MatchString(strings.TrimPrefix(stamp, "v")) {
		t.Errorf("dev stamp %q is shaped like a release version", stamp)
	}
	if !strings.Contains(strings.ToLower(stamp), "unversioned") {
		t.Errorf("dev stamp %q does not say the build is unversioned", stamp)
	}
}

// TestCompilerVersionStamp_ReleaseIsUnchanged is the control: the fix must
// not alter what a release-built binary stamps, or every compiled artifact
// churns and the golden corpus moves for no reason.
func TestCompilerVersionStamp_ReleaseIsUnchanged(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"0.6.0", "0.6.0"},
		{"v0.6.0", "0.6.0"},        // leading v stripped, as before
		{"0.6.0-rc4", "0.6.0-rc4"}, // prereleases are still releases
	} {
		stamp, released := compilerVersionStamp(tc.in)
		if !released {
			t.Errorf("%q not recognised as a release", tc.in)
		}
		if stamp != tc.want {
			t.Errorf("compilerVersionStamp(%q) = %q, want %q", tc.in, stamp, tc.want)
		}
	}
}

// TestCompilerVersionStamp_EmptyIsAlsoUnversioned covers the other way a
// build can arrive without provenance: an ldflag that resolved to nothing
// (a `-X` against an empty VERSION file). Silently stamping "" would be
// worse than "dev" — it reads as a field nobody set rather than a build
// nobody versioned.
func TestCompilerVersionStamp_EmptyIsAlsoUnversioned(t *testing.T) {
	stamp, released := compilerVersionStamp("")
	if released {
		t.Error("empty version reported as a release")
	}
	if !strings.Contains(strings.ToLower(stamp), "unversioned") {
		t.Errorf("empty stamp %q does not say the build is unversioned", stamp)
	}
}
