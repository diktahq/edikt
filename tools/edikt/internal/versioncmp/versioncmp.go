// Package versioncmp is the ONE place edikt orders two version strings.
//
// ── WHY IT EXISTS ────────────────────────────────────────────────────────────
//
// `package cmd` carried semverParts/semverGreater/semverMajor, a three-line
// truncating parser whose failure mode was to answer PLAUSIBLY rather than to
// refuse. semverParts read the numeric prefix of each dot-separated field and
// stopped at the first non-digit, so:
//
//	semverParts("dev")          == [0 0 0]   a channel name, dead last
//	semverParts("")             == [0 0 0]   a failed fetch, dead last
//	semverParts("0.8.0-rc1")    == [0 8 0]   a pre-release, equal to its release
//
// Three different KINDS of input collapsed onto one number, and every consumer
// then compared that number as though it meant something. `edikt upgrade` read
// a developer's dev checkout as version 0.0.0 and every published release as
// newer than it; the pin warning told every developer on this project that
// their pin was AHEAD of their binary and advised against the one command that
// would have cleared it.
//
// ── THE BOUNDARY THIS PACKAGE CLAIMS, AND THE ONE IT DOES NOT ────────────────
//
// It answers ONE question: how do two version strings relate in a release
// ordering? It does not fetch, resolve, normalise tags for display, or decide
// what a caller should do with the answer. Two other packages in this tree
// claim singularity over an adjacent question and must not drift into this
// one: internal/globmatch decides whether a path matches a glob, and (from
// this spec's later phase) internal/pathid decides whether two paths name the
// same location. Ordering, matching and identity are three questions; each has
// exactly one implementation here.
//
// ── INCOMPARABLE IS AN ANSWER, NOT AN ERROR ──────────────────────────────────
//
// A channel name has no position in a release ordering. `dev` is neither ahead
// of nor behind 0.4.5, and any total order placed on it is a fabrication. So
// Compare returns Incomparable, a first-class verdict a caller must handle,
// rather than a bool that has to mean something. What it must NOT do is refuse
// a question it can answer: two channel names that are the same STRING are
// equal, and Compare says so.
//
// ── WHAT THIS PACKAGE DELIBERATELY DOES NOT DO YET ───────────────────────────
//
// An EMPTY version is not a channel name — it is a failed fetch, an absence.
// Compare currently answers Incomparable for it, which is safe at every
// consumer (no ordering means no advance) but is not the same as making the
// caller HANDLE the absence. The fixture row that records the difference
// (VO-12) is deliberately left disagreeing here: the requirement that closes it
// is the empty-LATEST_VERSION guard in the tier-1 shell block, and it is owned
// by a later phase. Stated rather than silently implemented, because a defect
// closed outside the requirement that owns it is a defect nobody reviewed.
//
//	spec: SPEC-024 SR-002, comparison and status defects  // edikt-guard:allow
//	rule: semver 2.0.0 §10, §11.2, §11.3, §11.4
package versioncmp

import (
	"strconv"
	"strings"
)

// Ordering is how two version strings relate.
//
// The four constants carry the fixture table's vocabulary VERBATIM, so a test
// can compare a recorded `intended:` against a returned Ordering with no
// translation table in between — a translation table being one more place two
// values are compared without first being brought into the same form, which is
// the defect class this package was written for.
type Ordering string

const (
	// LeftNewer — left sorts after right.
	LeftNewer Ordering = "left_newer"
	// RightNewer — right sorts after left.
	RightNewer Ordering = "right_newer"
	// Equal — the two name the same point in the ordering.
	Equal Ordering = "equal"
	// Incomparable — at least one side has no position in a release ordering
	// (a channel name such as `dev`, or an absence such as ""), and the two
	// are not the same string. Not an error: the honest answer.
	Incomparable Ordering = "incomparable"
)

// Version is a parsed release version. Build metadata (§10) is discarded
// during parsing because it takes no part in ordering (§11.2).
type Version struct {
	// Major, Minor and Patch are the numeric core.
	Major int
	Minor int
	Patch int
	// Pre holds the dot-separated pre-release identifiers, nil when the
	// version has none. nil and an empty slice mean different things here —
	// nil is "this is a release", and a release sorts AFTER any pre-release
	// of the same core (§11.3) — so callers must not build one by hand.
	Pre []string
}

// IsPreRelease reports whether the version carries a pre-release suffix.
func (v Version) IsPreRelease() bool { return len(v.Pre) > 0 }

// Parse reads a release version, reporting whether the string IS one.
//
// The bool is the whole point: it is what "dev", "nightly", "" and "0.8" have
// in common, and what the truncating parser this package replaces could not
// say. A caller that ignores it gets a zero Version, which is why every
// consumer in this repository goes through Compare or Major instead.
//
// Accepted: an optional leading `v`, then MAJOR.MINOR.PATCH with every field a
// non-empty run of digits, then an optional `-<pre-release>` whose identifiers
// are non-empty and alphanumeric-or-hyphen, then optional `+<build>` which is
// discarded.
func Parse(s string) (Version, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, false
	}
	s = strings.TrimPrefix(s, "v")

	// Build metadata takes no part in ordering (§11.2) and is dropped before
	// anything else, so `0.9.0+build.1` and `0.9.0` parse identically.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	core := s
	pre := ""
	hasPre := false
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core, pre, hasPre = s[:i], s[i+1:], true
	}

	fields := strings.Split(core, ".")
	if len(fields) != 3 {
		return Version{}, false
	}
	var nums [3]int
	for i, f := range fields {
		n, ok := numericIdentifier(f)
		if !ok {
			return Version{}, false
		}
		nums[i] = n
	}

	v := Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}
	if hasPre {
		ids := strings.Split(pre, ".")
		for _, id := range ids {
			if !isPreReleaseIdentifier(id) {
				return Version{}, false
			}
		}
		v.Pre = ids
	}
	return v, true
}

// Compare orders two version strings.
func Compare(left, right string) Ordering {
	l, lok := Parse(left)
	r, rok := Parse(right)

	if !lok || !rok {
		// At least one side is not a release version. The one comparison that
		// remains answerable is identity: two channel names that are the same
		// STRING are equal, and refusing that would over-apply Incomparable —
		// the helper is not allowed to refuse a question it can answer.
		//
		// An EMPTY string is excluded from that rule deliberately. It is an
		// absence rather than a name, and "" == "" says nothing about two
		// failed fetches being the same version.
		lt := strings.TrimSpace(left)
		if !lok && !rok && lt != "" && lt == strings.TrimSpace(right) {
			return Equal
		}
		return Incomparable
	}

	switch c := compareVersions(l, r); {
	case c > 0:
		return LeftNewer
	case c < 0:
		return RightNewer
	default:
		return Equal
	}
}

// Greater reports whether left sorts strictly after right.
//
// It is a CONVENIENCE OVER Compare AND NOT A REPLACEMENT FOR IT: a bool cannot
// represent Incomparable, so `!Greater(latest, current)` reads "not newer",
// which is true both for an older release and for a channel name that has no
// ordering at all. Callers whose decision differs between those two cases —
// every refusal in `edikt upgrade` does — must switch on Compare instead.
func Greater(left, right string) bool { return Compare(left, right) == LeftNewer }

// Major returns the major component, reporting whether the string is a release
// version at all. The bool is what stops a channel name reading as major 0 and
// a 1.x release then looking like a cross-major upgrade from a dev checkout.
func Major(v string) (int, bool) {
	p, ok := Parse(v)
	if !ok {
		return 0, false
	}
	return p.Major, true
}

// compareVersions orders two parsed versions: numeric core first (§11.2), then
// the pre-release rule (§11.3, §11.4).
func compareVersions(a, b Version) int {
	for _, pair := range [3][2]int{
		{a.Major, b.Major},
		{a.Minor, b.Minor},
		{a.Patch, b.Patch},
	} {
		if pair[0] != pair[1] {
			return sign(pair[0] - pair[1])
		}
	}
	return comparePreRelease(a.Pre, b.Pre)
}

// comparePreRelease implements semver 2.0.0 §11.3 and §11.4.
//
//	§11.3  a version WITH a pre-release sorts before the same version without.
//	§11.4.1 numeric identifiers compare NUMERICALLY.
//	§11.4.2 alphanumeric identifiers compare lexically in ASCII order.
//	§11.4.3 a numeric identifier always sorts BEFORE an alphanumeric one.
//	§11.4.4 with all preceding identifiers equal, the longer set sorts higher.
//
// §11.4.1 is the one that catches a string-comparison rewrite: beta.2 against
// beta.10 is "2" > "10" lexically and 2 < 10 numerically, and every other
// ordering case in edikt's fixture table passes under both readings.
func comparePreRelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1 // §11.3 — a release outranks its own pre-release.
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		an, aNum := numericIdentifier(a[i])
		bn, bNum := numericIdentifier(b[i])
		switch {
		case aNum && bNum:
			if an != bn {
				return sign(an - bn)
			}
		case aNum:
			return -1 // §11.4.3
		case bNum:
			return 1
		default:
			if c := strings.Compare(a[i], b[i]); c != 0 {
				return c
			}
		}
	}
	return sign(len(a) - len(b)) // §11.4.4
}

// numericIdentifier reports whether s is a non-empty run of ASCII digits, and
// its value. It never falls back to a partial parse: "rc1" is not the number 0
// and not the number 1, it is not a number, and saying so is the whole
// difference between this package and the one it replaces.
func numericIdentifier(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	// A version field long enough to overflow is not a version this tool has
	// any business ordering; refuse rather than wrap.
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// isPreReleaseIdentifier reports whether s is a legal pre-release identifier:
// non-empty, and drawn from [0-9A-Za-z-] (§9).
func isPreReleaseIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c == '-':
		default:
			return false
		}
	}
	return true
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}
