package audit

import (
	"strings"
	"testing"
)

// ── D95 (1): a prong verdict quotes the sentence that satisfied it ──────────

// The case that produced this rule: a clean-context auditor passed a draft on
// the real-alternative prong whose two rejected alternatives were "keep
// relying on the tool" and "keep auditing by hand". It read the section's
// presence.
func TestVerdict_ClaimedPassWithNoQuoteFails(t *testing.T) {
	r := GradeProng("Q2 prong 1 — a real rejected alternative existed", "a broker with at-least-once delivery", "", true)
	if r.Verdict != VerdictFail {
		t.Fatalf("a claimed pass with no quote must FAIL, got %q", r.Verdict)
	}
	if !strings.Contains(r.Reason, "NO quote") {
		t.Errorf("the reason must say the quote is missing: %q", r.Reason)
	}
}

func TestVerdict_PassRequiresBothTheClaimAndTheQuote(t *testing.T) {
	quote := "We chose a broker with at-least-once delivery over direct HTTP calls."
	r := GradeProng("Q2 prong 1", "a broker", quote, true)
	if r.Verdict != VerdictPass {
		t.Fatalf("a claim WITH its quote passes, got %q (%s)", r.Verdict, r.Reason)
	}
	if r.Quote != quote {
		t.Errorf("the quote must be carried verbatim, got %q", r.Quote)
	}
	if r := GradeProng("Q2 prong 1", "a broker", quote, false); r.Verdict != VerdictFail {
		t.Errorf("an unsatisfied prong fails even with a quote, got %q", r.Verdict)
	}
}

// D95 (3): an option the auditor cannot describe as a build does not count.
func TestVerdict_DoNothingAlternativesFailTheDecisionProng(t *testing.T) {
	for _, option := range []string{
		"keep relying on the tool",
		"continuing to audit by hand",
		"We considered keeping the status quo",
		"leave it as today",
		"no change",
	} {
		r := GradeDecisionProng(option, "We considered "+option+", and rejected it.")
		if r.Verdict != VerdictFail {
			t.Errorf("%q is a do-nothing variant and must fail the decision prong, got %q", option, r.Verdict)
		}
		if !strings.Contains(r.Reason, "do-nothing") {
			t.Errorf("%q: the reason must name the shape: %q", option, r.Reason)
		}
		if r.Prong == "" || !strings.Contains(r.Prong, "Q2 prong 1") {
			t.Errorf("%q: the failing prong must be named: %q", option, r.Prong)
		}
	}
}

func TestVerdict_ARealAlternativePassesTheDecisionProng(t *testing.T) {
	r := GradeDecisionProng(
		"a layered architecture with a service and a repository per aggregate",
		"We rejected a layered architecture with a service and a repository per aggregate, because ports and adapters isolate the domain.")
	if r.Verdict != VerdictPass {
		t.Fatalf("a real competing design must pass, got %q (%s)", r.Verdict, r.Reason)
	}
}

// The class verdict may not be stronger than its evidence.
func TestVerdict_ReportNamesTheProngThatDecidedIt(t *testing.T) {
	rep := Report{
		Artifact: "ADR-900",
		Class:    "not-an-adr",
		Prongs: []ProngResult{
			GradeProng("Q2 prong 2 — expensive to reverse", "", "Reversing this means re-encoding every stored event.", true),
			GradeDecisionProng("keep relying on the tool", "We considered keep relying on the tool."),
		},
	}
	if !rep.AnyFailed() {
		t.Fatal("a report with a failing prong must say so")
	}
	failing := rep.FailingProngs()
	if len(failing) != 1 || !strings.Contains(failing[0], "Q2 prong 1") {
		t.Fatalf("the failing prong must be named: %v", failing)
	}
}

// ── D95 (2): restatement detection over the corpus ──────────────────────────

var corpus = []CorpusRule{
	{Artifact: "INV-006", Kind: "invariant", Text: "Any value that originates outside edikt's immediate control MUST be validated against an allowlist regex before it is interpolated into a shell argv element."},
	{Artifact: "INV-003", Kind: "invariant", Text: "Every hook that emits JSON MUST construct that JSON via a structured serializer with untrusted values passed as data."},
}

// The draft that produced this rule: an existing invariant's sentence with one
// noun substituted.
func TestRestatement_OneNounSubstitutedIsFound(t *testing.T) {
	draft := []string{
		"Any value that originates outside edikt's immediate control MUST be validated against an allowlist pattern before it is interpolated into a shell argv element.",
	}
	got := FindRestatements(draft, corpus)
	if len(got) == 0 {
		t.Fatal("a sentence differing by one noun must be reported as a restatement")
	}
	if got[0].Artifact != "INV-006" {
		t.Errorf("it must name the record that already says it, got %q", got[0].Artifact)
	}
	if got[0].Verdict != "amendment to INV-006" {
		t.Errorf("the verdict must be an amendment, not a new record: %q", got[0].Verdict)
	}
	if got[0].Exact {
		t.Error("a one-noun substitution is a NEAR match, not an exact one")
	}
	if got[0].Similarity < NearMatchThreshold {
		t.Errorf("the reported similarity must justify the match, got %.2f", got[0].Similarity)
	}
}

// Cosmetics only: the same rule, repunctuated and recased.
func TestRestatement_CosmeticEditsAreTheSameRule(t *testing.T) {
	draft := []string{
		"every hook that emits json MUST construct that json via a structured serializer, with untrusted values passed as data!!",
	}
	got := FindRestatements(draft, corpus)
	if len(got) == 0 || !got[0].Exact {
		t.Fatalf("a repunctuated, recased copy is the SAME rule: %+v", got)
	}
}

// A genuinely new rule is not a restatement — a detector that flags everything
// trains its reader to dismiss it.
func TestRestatement_ANewRuleIsNotReported(t *testing.T) {
	draft := []string{"A release asset MUST carry a cosign signature verified before installation."}
	if got := FindRestatements(draft, corpus); len(got) != 0 {
		t.Fatalf("a new rule must not be reported: %+v", got)
	}
}

// The identity function keeps artifact ids intact, so the same sentence naming
// a different invariant is a different rule.
func TestRestatement_ADifferentArtifactIdIsADifferentRule(t *testing.T) {
	a := Normalise("A hook MUST emit JSON. (ref: INV-003)")
	b := Normalise("A hook MUST emit JSON. (ref: INV-004)")
	if a == b {
		t.Fatal("artifact ids must survive normalisation — two rules citing different records are two rules")
	}
	if RuleDigest("invariant", "A hook MUST emit JSON. (ref: INV-003)") ==
		RuleDigest("invariant", "A hook MUST emit JSON. (ref: INV-004)") {
		t.Error("their digests must differ too")
	}
}

// The pair-overlap detector, which comes free: two records requiring one thing.
func TestRestatement_OverlapAcrossTheCorpusIsReported(t *testing.T) {
	c := append([]CorpusRule(nil), corpus...)
	c = append(c, CorpusRule{
		Artifact: "ADR-901", Kind: "adr",
		Text: "Every hook that emits JSON MUST construct that JSON via a structured serializer with untrusted values passed as data.",
	})
	got := FindOverlaps(c)
	if len(got) != 1 {
		t.Fatalf("one overlapping pair expected, got %d: %+v", len(got), got)
	}
	if !got[0].Exact {
		t.Error("the same sentence in two records is an exact overlap")
	}
	pair := got[0].A.Artifact + "+" + got[0].B.Artifact
	if pair != "INV-003+ADR-901" && pair != "ADR-901+INV-003" {
		t.Errorf("the overlap must name both records, got %s", pair)
	}
}

// ── the identity function's parity cases ────────────────────────────────────

// These are the cases the brownfield lane's own test pins. Reproduced here
// because this package reproduces the function: if the two implementations
// ever diverge, this is where it shows up, rather than in two subtly
// different answers to "is this the same rule".
func TestNormalise_ParityWithTheLanesCases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Hooks   MUST emit JSON.  ", "hooks must emit json"},
		{"Hooks MUST emit JSON!!!", "hooks must emit json"},
		{"A rule naming INV-006.", "a rule naming inv-006"},
		{"A path commands/config.md stays whole.", "a path commands/config.md stays whole"},
		{"MiXeD CaSe", "mixed case"},
	}
	for _, c := range cases {
		if got := Normalise(c.in); got != c.want {
			t.Errorf("Normalise(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
