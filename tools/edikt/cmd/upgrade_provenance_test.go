package cmd

// upgrade_provenance_test.go — SPEC-024 phase 3, AC-3.3.
//
// The decision `edikt upgrade` makes before it replaces a payload, asserted as
// a TABLE at the function boundary: (current, latest, recorded source) ->
// {advance, refuse, refuse-with-reason}.
//
// It is asserted HERE, at upgradeProvenanceGate, and not end to end, because
// observing a real advance needs a signed download, an activation and a
// symlink flip. INV-008 forbids a floating URL and releases are signed through
// Fulcio and GitHub OIDC, so CI has nothing to fetch, and a stubbed downloader
// standing in for the real one would be testing the mock. What is NOT claimed
// by this file is written down in docs/internal/plans/verdicts/
// SPEC-024-provenance-release-gate.md, together with the operator-run
// observation of both source states.

import (
	"strings"
	"testing"
)

// TestUpgradeProvenance_DecisionTable — AC-3.3.
func TestUpgradeProvenance_DecisionTable(t *testing.T) {
	const (
		advance          = "advance"
		refuseWithReason = "refuse_with_reason"
	)
	// outcome collapses the gate's answer to the vocabulary PC-04 records: an
	// advance, a refusal that names provenance, or some other refusal named by
	// its own mechanism.
	outcome := func(r upgradeRefusal) string {
		switch r {
		case refusalNone:
			return advance
		case refusalLocalPayload:
			return refuseWithReason
		default:
			return string(r)
		}
	}

	cases := []struct {
		name                    string
		current, latest, source string
		want                    string
		why                     string
	}{
		// ── PC-04, the ruled row: the three source states, same versions ──
		{"PC-04 published-sourced pre-release advances to its release",
			"0.8.0-rc1", "0.8.0", lockSourcePublished, advance,
			"the payload came from a published release and can be fetched again, so being stranded on an rc is the only harm and it is the recoverable one"},
		{"PC-04 local-sourced pre-release refuses, saying why",
			"0.8.0-rc1", "0.8.0", lockSourceLocal, refuseWithReason,
			"a locally built payload cannot be re-fetched; replacing it silently is the loss SSP-004 refuses"},
		{"PC-04 absent source reads as published and advances",
			"0.8.0-rc1", "0.8.0", lockSourceUnknown, advance,
			"every install written before the field existed carries no source; refusing them all would strand every one"},

		// ── provenance is not limited to pre-releases ─────────────────────
		{"a local payload on a release version is protected too",
			"0.8.0", "0.9.0", lockSourceLocal, refuseWithReason,
			"what is destroyed is a payload, not a version string — nothing about the harm depends on the suffix"},
		{"a published payload on a release version advances",
			"0.8.0", "0.9.0", lockSourcePublished, advance,
			"the ordinary upgrade, which must keep working"},

		// ── the version question is answered FIRST, and its answer wins ───
		{"an unrecognised source reads as published",
			"0.8.0", "0.9.0", "tarball-from-a-colleague", advance,
			"the ruling names published, local and absent; inventing a fourth refusal for a string this binary does not know would be deciding something no owner has, and it is recorded as a known limit rather than left to chance"},
		{"already up to date wins over provenance",
			"0.9.0", "0.8.0", lockSourceLocal, string(refusalUpToDate),
			"no advance was going to happen, so naming provenance would name a mechanism that decided nothing"},
		{"not comparable wins over provenance",
			"dev", "0.9.0", lockSourceLocal, string(refusalNotComparable),
			"a channel has no position in a release ordering; that is the truer answer and it comes first"},
		{"cross-major wins over provenance",
			"0.9.0", "1.0.0", lockSourceLocal, string(refusalCrossMajor),
			"the cross-major rejection is not a provenance question"},
		{"a skipped minor line wins over provenance",
			"0.7.0", "0.9.0", lockSourceLocal, string(refusalMinorSkip),
			"the adjacent-line gate refuses before provenance is consulted"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason, msg := upgradeProvenanceGate(c.current, c.latest, c.source)
			if got := outcome(reason); got != c.want {
				t.Fatalf("upgradeProvenanceGate(current=%q, latest=%q, source=%q) = %q (reason %q), want %q — %s\n%s",
					c.current, c.latest, c.source, got, reason, c.want, c.why, msg)
			}
			if c.want == advance && msg != "" {
				t.Errorf("an advance carries the message %q; nothing refused, so there is nothing to say", msg)
			}
			if c.want != advance && strings.TrimSpace(msg) == "" {
				t.Errorf("%q refused with no message — a refusal a user cannot read is a refusal they cannot act on", reason)
			}
		})
	}

	// THE REASON IS PART OF THE OUTCOME. `refuse_with_reason` is not "a
	// non-zero exit": a test asserting only that an upgrade did not happen
	// passes when the minor-line gate refuses for arithmetic reasons and the
	// provenance check has been deleted.
	t.Run("the local-payload refusal says what it protected and how to proceed", func(t *testing.T) {
		reason, msg := upgradeProvenanceGate("0.8.0-rc1", "0.8.0", lockSourceLocal)
		if reason != refusalLocalPayload {
			t.Fatalf("reason = %q, want %q", reason, refusalLocalPayload)
		}
		for _, want := range []string{"LOCAL source", "0.8.0-rc1", "0.8.0", "edikt install"} {
			if !strings.Contains(msg, want) {
				t.Errorf("the refusal does not mention %q, so it cannot be acted on:\n%s", want, msg)
			}
		}
	})

	t.Run("the provenance refusal is distinct from every other mechanism", func(t *testing.T) {
		for _, other := range []upgradeRefusal{
			refusalNone, refusalDevLinked, refusalNotComparable,
			refusalUpToDate, refusalCrossMajor, refusalMinorSkip,
		} {
			if refusalLocalPayload == other {
				t.Errorf("refusalLocalPayload is indistinguishable from %q, so two mechanisms cannot be told apart", other)
			}
		}
	})

	// The gate must not become a second version comparison. Every refusal it
	// produces that is NOT a provenance one has to be the one upgradeVersionGate
	// produces for the same pair, or there are two answers to one question —
	// the defect this whole spec is about.
	t.Run("the version-derived answers are upgradeVersionGate's, unchanged", func(t *testing.T) {
		for _, pair := range [][2]string{
			{"0.8.0-rc1", "0.8.0"}, {"0.9.0", "0.8.0"}, {"dev", "0.9.0"},
			{"0.9.0", "1.0.0"}, {"0.7.0", "0.9.0"}, {"0.8.0", "0.9.0"},
		} {
			want, wantMsg := upgradeVersionGate(pair[0], pair[1])
			got, gotMsg := upgradeProvenanceGate(pair[0], pair[1], lockSourcePublished)
			if got != want || gotMsg != wantMsg {
				t.Errorf("upgradeProvenanceGate(%q, %q, published) = %q, but upgradeVersionGate says %q — the provenance gate is answering the version question a second time",
					pair[0], pair[1], got, want)
			}
		}
	})
}
