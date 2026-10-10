package hookmatch

// hookmatch — path resolution, the hook journal, and the governing matcher.
//
// It was the write-time injection tier's matcher until ADR-083 retired that  edikt-guard:allow
// tier; the injecting Match and its index reader are gone, MatchGoverning
// (governing.go) over the delivery map is what remains.
//
// Given a path being written, return the compiled directives scoped to it,
// with their grades PINNED at render time (never re-derived here — a consumer
// that re-derived grade could silently downgrade an invariant and nothing
// would report the difference).
//
// FAIL-OPEN, BUT NEVER SILENTLY
//
// This runs inside a PreToolUse hook. A matcher that hard-fails blocks the
// user's editor on a governance bug, which is a worse outcome than missing an
// injection — so every failure class returns "no directives" and lets the
// write through.
//
// That default has a specific danger, and it is the reason for Outcome. Once
// this is the ONLY channel for MUST-grade write-touch directives, a missing
// binary, a corrupt index, and a crafted path are BYTE-IDENTICAL to a clean
// zero-match at the hook's output. Fail-open stays; silent fail-open does not.
// So every result carries WHY it is empty, and the classes are distinct and
// journaled (INV-013: absence never renders as a pass).  edikt-guard:allow

import (
	"errors"
	"path/filepath"
	"strings"
)

// Outcome names WHY a match returned what it did.
//
// The empty cases are enumerated separately on purpose: "the index says
// nothing applies here" and "the index could not be read" are the same empty
// list and completely different facts.
type Outcome string

const (
	OutcomeMatched      Outcome = "matched"
	OutcomeNoMatch      Outcome = "no_match"      // index read fine; nothing covers this path
	OutcomeIndexMissing Outcome = "index_missing" // not compiled, or rolled back
	OutcomeIndexCorrupt Outcome = "index_corrupt" // present and unparseable
	OutcomeBadPath      Outcome = "path_rejected" // path could not be normalised
	OutcomeIndexEmpty   Outcome = "index_empty"   // compiled, but zero globs

	// OutcomeAgentExempt — the write came from a governance-PRODUCING agent,
	// so the write-time directive hook does not apply to it (ADR-074). This
	// is ROUTING, not absence: the artifact is still gated, by the
	// governance-specific checks named in that ADR (gov schema-check, phasea
	// verify, anchor verification, check-authored-directive-modality,
	// check-directive-scope-coverage), none of which run at write time.
	//
	// Journaled with its own outcome rather than folded into no_match
	// precisely so `hook report` can distinguish "governance chose not to run
	// here" from "governance found nothing" — INV-013's shape: an exemption
	// that reported as a clean no-match would be absence presented as a
	// passing measurement.
	OutcomeAgentExempt Outcome = "agent_exempt"
)

// `hook run <name>` outcomes (SPEC-014 FR-006 / SP-007, ADR-060:d18).
//
// The five above describe a MATCH. These four describe an INVOCATION of a
// migrated hook handler, journaled by internal/hook's dispatcher rather than
// by Match. They are hyphenated where the five above are snake_case, which is
// deliberate rather than sloppy: the spelling is the one SPEC-014 and
// PLAN-v080 name verbatim, and the visual split keeps the two families legible
// in a journal that now carries both.
//
// EACH IS ITS OWN TYPE FOR THE SAME REASON THE FIVE ABOVE ARE. At the hook's
// output, a handler that crashed, a name that resolved to nothing, and a
// payload that would not decode are the same observable — a hook that exited 0
// having said nothing. Reusing an existing value "because it is close enough"
// is exactly what ADR-060:d18 forbids.
const (
	// OutcomeHookRan — the handler was found, ran to completion, and returned.
	// NOT a suppression: this is the healthy class, and the exit code carried
	// on the same record is what says whether the hook allowed or denied.
	OutcomeHookRan Outcome = "hook-ran"

	// OutcomePanicRecovered — the handler ran and BROKE. The dispatcher caught
	// the panic at its single recovery site, emitted an empty hook-protocol
	// reply, and exited 0. Exiting 0 is not leniency: a Go panic exiting
	// non-zero at PreToolUse would BLOCK the user's write, which is strictly
	// worse than the missing-interpreter behaviour this migration replaces.
	OutcomePanicRecovered Outcome = "panic-recovered"

	// OutcomeUnregisteredHookName — the name resolved to no handler. Never a
	// silent no-op (SP-007): exit 0, a loud stderr diagnostic naming the name,
	// and this record.
	OutcomeUnregisteredHookName Outcome = "unregistered-hook-name"

	// OutcomePayloadUnparseable — the handler exists but never ran, because
	// the host payload could not be decoded. Distinct from panic-recovered
	// ("ran and broke") and from unregistered ("never found"): here the hook
	// was present and correct and still governed nothing.
	OutcomePayloadUnparseable Outcome = "payload-unparseable"

	// OutcomeBinaryAbsent — bin/edikt itself could not be found on the
	// shim's discovery ladder (SPEC-014 FR-003 / PRD-007 AC-003-1, ADR-060's
	// 2026-08-20 amendment promoting binary absence from an "unjournalable"
	// cause to a distinctly-typed one).
	//
	// The shim never writes this record itself: the amendment is explicit
	// that "a shim-side journal write would violate AC-004-1's
	// no-business-logic rule" — the shim has no state-directory contract and
	// no lock discipline. This value exists so the amendment's OTHER half,
	// the next successful `bin/edikt` invocation backfilling the absence
	// window it can now see, has a distinct outcome to write instead of
	// reusing one of the three above. A hook whose binary was entirely
	// missing is none of "ran and broke" (panic-recovered), "never found"
	// (unregistered-hook-name), or "found and never ran"
	// (payload-unparseable) — it is "nothing ran because there was nothing to
	// run it", and ADR-060:d18 forbids collapsing distinct fail-open causes
	// into one just because they are close.
	OutcomeBinaryAbsent Outcome = "binary-absent"
)

// Suppressed reports whether this outcome means governance did not run, as
// opposed to genuinely not applying. This is the predicate the detectability
// surface is built on.
// The `hook run` classes are here for a reason worth stating: without them, a
// hook that PANICKED would be counted as a healthy invocation by ReadSummary,
// and `hook probe` would exit 0 on a chain that crashed. A crash is the most
// literal form of "governance did not run" there is.
func (o Outcome) Suppressed() bool {
	switch o {
	case OutcomeIndexMissing, OutcomeIndexCorrupt, OutcomeBadPath, OutcomeIndexEmpty:
		return true
	case OutcomePanicRecovered, OutcomeUnregisteredHookName, OutcomePayloadUnparseable, OutcomeBinaryAbsent:
		return true
	}
	return false
}

// NormalizePath resolves a path the way the matcher must see it (AC-3.4).
//
// TWO steps, and both are load-bearing:
//
//	Clean       collapses "a/../b" and "./x", so a literal path cannot be
//	            dressed up to miss a glob it should match.
//	EvalSymlinks resolves the link to its target, so a symlink whose LITERAL
//	            path escapes a governed glob but whose TARGET is inside it
//	            still matches.
//
// The disagreement case is the whole point. `docs/link.go -> tools/edikt/x.go`
// matches nothing as a literal and matches `tools/**/*.go` as a target; the
// inverse is a link inside a governed directory pointing outside it. Matching
// on the literal alone lets an agent write governed code through a link, which
// is a governance bypass with no error anywhere.
//
// A path that does not exist yet is NOT an error: Write creates new files, and
// that is the most common case at PreToolUse. It is Cleaned and returned.
func NormalizePath(root, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("empty path")
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)

	// Resolve through the DEEPEST EXISTING ANCESTOR, then re-append the part
	// that does not exist yet.
	//
	// EvalSymlinks on the full path fails for a file Write is about to create —
	// the common PreToolUse case — leaving the path unresolved while the ROOT
	// below is resolved. On macOS that alone breaks matching for every new
	// file: /var is a symlink to /private/var, so root becomes /private/var/…
	// and the path stays /var/…, filepath.Rel returns a "../.." escape, and the
	// matcher silently falls back to an absolute path that matches no glob.
	//
	// Silently. That is the shape of the bug that matters: not an error, just
	// governance that stops applying to newly created files.
	abs = resolveExisting(abs)

	// Report relative to the project root, because that is what the compiled
	// globs are written against. A path outside the root is returned absolute
	// and will simply match nothing — which is correct, not an error: editing
	// a file outside the project is not a governance event.
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return abs, nil
	}
	if rootResolved, rerr := filepath.EvalSymlinks(rootAbs); rerr == nil {
		rootAbs = rootResolved
	}
	if rel, rerr := filepath.Rel(rootAbs, abs); rerr == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel), nil
	}
	return filepath.ToSlash(abs), nil
}

// resolveExisting resolves symlinks as deeply as the filesystem allows,
// re-appending any trailing components that do not exist yet.
func resolveExisting(abs string) string {
	rest := ""
	cur := abs
	for i := 0; i < 64; i++ { // bounded: a pathological path must not spin
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs // reached the root without resolving anything
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
	return abs
}
