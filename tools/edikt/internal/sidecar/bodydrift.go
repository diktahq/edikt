package sidecar

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// Body drift is the SECOND of two staleness signals, and it exists because
// the first one answers a narrower question than it appears to.
//
//	ANCHOR DRIFT  an extracted source_excerpt.quote no longer matches live
//	              prose.  Meaning: the sidecar is WRONG. Directives claim
//	              support they no longer have. Urgent.
//	BODY DRIFT    the artifact body's normalized digest differs from the
//	              digest recorded at extraction. Meaning: the sidecar may be
//	              INCOMPLETE. Something changed that extraction never saw.
//	              A regeneration, not an emergency.
//
// Collapsing them into one stale/not-stale verdict is what produced the hole
// this file closes: "0 stale" means "nothing extracted has changed" and reads
// as "the corpus is in sync". Those differ by exactly one case — prose ADDED
// after the last extraction. Anchor drift is structurally blind to it, because
// added prose invalidates no existing quote. A governance rule could be
// written into an invariant, compile could report 0 stale, and the rule would
// never be extracted into an enforceable directive. Nothing in the pipeline
// would ever say so.
//
// THE CEILING, stated here because it must also be stated in the output:
// body drift reports THAT something changed, never WHAT. It cannot tell an
// added MUST from a fixed typo, and it does not try. Anything finer requires
// re-extraction, which is the remedy it recommends rather than a job it
// performs. A signal that overstated its resolution here would be worse than
// this one, because the response to "something changed" is cheap and the
// response to a confident wrong diagnosis is not.

// NormalizeBody collapses a governance artifact body to the form its digest
// is taken over.
//
// Whitespace is normalized so that a reflow does not read as a content
// change: re-wrapping a paragraph, changing list indentation, or converting
// CRLF to LF alters bytes without altering a single word, and a digest that
// fired on those would train readers to ignore it within a week — the noise
// clause, applied to the instrument itself.
//
// This deliberately uses the SAME normalization as anchor drift's
// whitespace-normalized fallback (normalizeWS: split on any run of Unicode
// whitespace, rejoin with single spaces). Two normalizations that could
// disagree about whether two bodies are "the same" would be two definitions
// of one rule, and the weaker would be the one that got used.
func NormalizeBody(body string) string {
	return normalizeWS(body)
}

// BodyDigest returns the hex SHA-256 of the normalized body. Stable across
// platforms and line endings; deterministic; no LLM, no clock, no filesystem.
func BodyDigest(body string) string {
	sum := sha256.Sum256([]byte(NormalizeBody(body)))
	return hex.EncodeToString(sum[:])
}

// BodyDriftVerdict is the per-artifact outcome. The three states are
// deliberately distinct rather than a bool: UNMEASURED is not a pass, and
// making it representable is the only way a caller can report it as itself
// (INV-013). A bool would have forced every unmeasurable artifact into
// "no drift", which is the exact defect this whole signal exists to close.
type BodyDriftVerdict int

const (
	// BodyUnmeasured — no recorded digest to compare against. Says nothing
	// about whether the body changed. MUST be reported as unmeasured, never
	// folded into the clean count.
	BodyUnmeasured BodyDriftVerdict = iota
	// BodyUnchanged — recorded digest matches the live normalized body.
	BodyUnchanged
	// BodyDrifted — recorded digest differs. Something changed that
	// extraction never saw. WHAT changed is out of scope by construction.
	BodyDrifted
)

func (v BodyDriftVerdict) String() string {
	switch v {
	case BodyUnchanged:
		return "unchanged"
	case BodyDrifted:
		return "drifted"
	default:
		return "unmeasured"
	}
}

// BodyDriftResult is one artifact's body-drift outcome.
type BodyDriftResult struct {
	ArtifactID string           `json:"artifact_id"`
	Verdict    BodyDriftVerdict `json:"-"`
	// Status is the string form, because a JSON consumer reading an integer
	// enum would silently decode an absent field to 0 — which here would
	// mean BodyUnmeasured, and an absent field decoding to a meaningful
	// zero value is itself on INV-013's enumerated list.
	Status string `json:"status"`
	// Reason is populated only for BodyUnmeasured, naming why. "I could not
	// check" is information; it is only worth emitting when there was a
	// subject to check.
	Reason string `json:"reason,omitempty"`
}

// CheckBodyDrift compares a live body against a recorded digest.
//
// recorded is the digest captured when the sidecar was last extracted. An
// empty recorded digest means no baseline exists — never "unchanged".
func CheckBodyDrift(artifactID, recorded, liveBody string) BodyDriftResult {
	r := BodyDriftResult{ArtifactID: artifactID}
	if strings.TrimSpace(recorded) == "" {
		r.Verdict = BodyUnmeasured
		r.Status = r.Verdict.String()
		r.Reason = "no digest recorded at extraction — regenerate this sidecar to establish a baseline"
		return r
	}
	if recorded == BodyDigest(liveBody) {
		r.Verdict = BodyUnchanged
	} else {
		r.Verdict = BodyDrifted
	}
	r.Status = r.Verdict.String()
	return r
}

// BodyDriftSummary aggregates per-artifact results for reporting.
//
// Every field is a count over a stated denominator. A summary that reported
// only Drifted would make a run over zero artifacts indistinguishable from a
// run over fifty clean ones.
type BodyDriftSummary struct {
	Total      int               `json:"total"`
	Unchanged  int               `json:"unchanged"`
	Drifted    int               `json:"drifted"`
	Unmeasured int               `json:"unmeasured"`
	Results    []BodyDriftResult `json:"results,omitempty"`
}

// SummarizeBodyDrift folds per-artifact results into counts. Results are
// carried through for the drifted and unmeasured entries only — listing
// fifty unchanged artifacts is the noise the signal is trying not to become.
func SummarizeBodyDrift(results []BodyDriftResult) BodyDriftSummary {
	s := BodyDriftSummary{Total: len(results)}
	for _, r := range results {
		switch r.Verdict {
		case BodyUnchanged:
			s.Unchanged++
		case BodyDrifted:
			s.Drifted++
			s.Results = append(s.Results, r)
		default:
			s.Unmeasured++
			s.Results = append(s.Results, r)
		}
	}
	return s
}

// ReportLine renders the human-facing summary line.
//
// It always carries the denominator, and it always carries the ceiling. The
// ceiling is not a footnote: a reader who takes "body drift: 3" as "3 rules
// are missing" has been misled by a signal that only ever knew "3 bodies
// changed". Stating what the instrument cannot see, in the same breath as
// what it saw, is the whole discipline.
func (s BodyDriftSummary) ReportLine() string {
	if s.Total == 0 {
		return "body drift: no artifacts to check (0 sidecars)"
	}
	var b strings.Builder
	b.WriteString("body drift: ")
	b.WriteString(itoa(s.Drifted))
	b.WriteString(" changed since extraction, ")
	b.WriteString(itoa(s.Unchanged))
	b.WriteString(" unchanged, ")
	b.WriteString(itoa(s.Unmeasured))
	b.WriteString(" unmeasured (of ")
	b.WriteString(itoa(s.Total))
	b.WriteString(")")
	if s.Drifted > 0 {
		b.WriteString("\n  A changed body means the sidecar may be INCOMPLETE — prose was")
		b.WriteString("\n  edited that extraction never saw. This says THAT something changed,")
		b.WriteString("\n  never WHAT: it cannot tell an added MUST from a fixed typo, and it")
		b.WriteString("\n  does not try. Regenerate the sidecar to find out.")
	}
	if s.Unmeasured > 0 {
		b.WriteString("\n  Unmeasured is NOT a pass — those artifacts have no recorded baseline,")
		b.WriteString("\n  so nothing was compared. Regenerating each one establishes it.")
	}
	return b.String()
}

// CheckBodyDriftAgainstParent reads the sidecar's parent .md and compares it
// against the recorded digest.
//
// Deliberately SEPARATE from (*Sidecar).IsStale rather than folded into it.
// Folding would recreate the single stale/not-stale verdict that produced the
// hole: a wrong sidecar and an incomplete one demand different responses on
// different urgencies, and one boolean cannot carry that. It would also make
// every prose edit dispatch a full LLM re-extraction, collapsing ADR-028's
// Phase A / Phase B separation.
//
// A parent that cannot be read is UNMEASURED, not unchanged — the same rule
// applied to this function's own failure path.
func (s *Sidecar) CheckBodyDriftAgainstParent(projectRoot string, readFile func(string) ([]byte, error)) BodyDriftResult {
	id := s.Topic
	if s.Path != "" {
		id = s.Path
	}
	parentPath := s.Path
	if !filepath.IsAbs(parentPath) {
		parentPath = filepath.Join(projectRoot, s.Path)
	}
	data, err := readFile(parentPath)
	if err != nil {
		return BodyDriftResult{
			ArtifactID: id,
			Verdict:    BodyUnmeasured,
			Status:     BodyUnmeasured.String(),
			Reason:     "could not read parent " + parentPath + ": " + err.Error(),
		}
	}
	return CheckBodyDrift(id, s.BodyDigest, string(data))
}

// StampBodyDigest records the parent body's digest into the sidecar on disk.
//
// It MUST be called only after an extraction that actually read the current
// parent body — a stamp written at any other moment is a baseline claiming an
// extraction saw prose it never saw, which converts this signal from "the
// sidecar may be incomplete" into a silent, confident lie. That is strictly
// worse than the unmeasured state it would be replacing, so the narrow call
// site is a correctness constraint and not a stylistic one.
//
// Idempotent: stamping an unchanged parent rewrites the same digest.
func StampBodyDigest(sidecarPath, parentPath string,
	readFile func(string) ([]byte, error),
	writeFile func(string, []byte) error,
) error {
	parent, err := readFile(parentPath)
	if err != nil {
		return err
	}
	sc, err := Load(sidecarPath)
	if err != nil {
		return err
	}
	sc.BodyDigest = BodyDigest(string(parent))
	// Marshal, NOT Marshal. Load caches the canonical bytes on the
	// struct and Marshal returns that cache verbatim, so a field mutated
	// after Load is silently dropped — the stamp becomes a no-op that
	// reports success. Caught by the round-trip test rather than by review.
	out, err := Marshal(sc)
	if err != nil {
		return err
	}
	return writeFile(sidecarPath, out)
}

// itoa avoids pulling strconv into this file's surface for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
