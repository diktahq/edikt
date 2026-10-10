package phasea

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A sidecar that EXISTS but does not parse must verify as schema-invalid, never
// as output-missing.
//
// Field failure this pins, 2026-09-18: a contract emitted 2 of 13 `text:`
// values as bare unquoted scalars. Every directive body ends in a
// `(ref: ADR-NNN)` tail, and that tail carries a colon-space, so a bare scalar
// is unparseable BY CONSTRUCTION rather than by accident of content. 11 of the
// 13 directives were intact; the file was 13KB of usable extraction.
//
// The two statuses carry OPPOSITE remedies, which is why sharing one would be
// a defect rather than an imprecision (SPEC-013 SR-013):
//
//   - output-missing says the extractor wrote NOTHING, and the shipped remedy
//     for it is "a stale agent definition cached at session start — run
//     `edikt doctor` and restart the session". Told about this file, that
//     sends the user to restart a session over two missing quote characters.
//   - schema-invalid says the file is there and failed the content gate, whose
//     remedy is a correction pass over output that already exists.
//
// `contentGate` calls `sidecar.Load` first, so the classification is already
// correct. Nothing pinned it: before this test, StatusSchemaInvalid appeared in
// this package's tests only inside a list of legal status values, so no test
// ever drove an item into it. A behaviour that is right and unpinned is one
// refactor away from being wrong and unnoticed.
func TestVerify_UnparseableSidecarIsSchemaInvalidNotOutputMissing(t *testing.T) {
	// CONTROL FIRST. The same fixture, extracted cleanly, must verify ok — so a
	// schema-invalid verdict below cannot be the fixture failing for some other
	// reason, and this test can distinguish the two.
	good := newFixture(t, "")
	good.extractGood(t)
	if out := good.verify(t); onlyItem(t, out).Status != StatusOK {
		t.Fatalf("control: a clean extraction must verify ok, got %q (%s)",
			onlyItem(t, out).Status, out.Message)
	}
	clean, err := os.ReadFile(good.scPath)
	if err != nil {
		t.Fatalf("reading the control sidecar: %v", err)
	}

	// Now the SAME bytes with exactly one change: the first `text:` value loses
	// its quoting. Deriving the defective file from the passing one is the
	// point — the two differ by the defect and by nothing else, so the verdict
	// provably turns on it.
	// RE2 has no backreferences, so the two quote styles are separate patterns
	// rather than one clever one.
	var loc []int
	var body string
	for _, pat := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)^(\s*-?\s*text:\s*)'(.*)'[ \t]*$`),
		regexp.MustCompile(`(?m)^(\s*-?\s*text:\s*)"(.*)"[ \t]*$`),
	} {
		if loc = pat.FindSubmatchIndex(clean); loc != nil {
			// A YAML single-quoted scalar escapes ' as ''. Undo that, so the
			// bare value is what the extractor would actually have emitted.
			body = strings.ReplaceAll(string(clean[loc[4]:loc[5]]), "''", "'")
			break
		}
	}
	if loc == nil {
		t.Fatalf("no quoted `text:` value in the marshalled sidecar — this test's "+
			"premise is gone, not satisfied:\n%s", clean)
	}
	if !strings.Contains(body, ": ") {
		t.Fatalf("the control body %q carries no colon-space, so unquoting it would "+
			"still parse and this test would pass while measuring nothing", body)
	}
	broken := string(clean[:loc[2]]) + string(clean[loc[2]:loc[3]]) + body + string(clean[loc[1]:])

	f := newFixture(t, "")
	if err := os.WriteFile(f.scPath, []byte(broken), 0o644); err != nil {
		t.Fatalf("planting the unparseable sidecar: %v", err)
	}

	out := f.verify(t)
	it := onlyItem(t, out)

	// The falsifying observation: the file is ON DISK. Any verdict that says
	// the output is missing is contradicted by this stat, not by an opinion.
	if _, statErr := os.Stat(f.scPath); statErr != nil {
		t.Fatalf("the planted sidecar is gone (%v) — this test would then be "+
			"asserting output-missing is wrong about a genuinely missing file", statErr)
	}
	if it.Status == StatusOutputMissing {
		t.Fatalf("an unparseable sidecar that EXISTS was graded output-missing. Its " +
			"shipped remedy is restart-the-session for an extractor that wrote " +
			"nothing; this extractor wrote a file and got two quote marks wrong")
	}
	if it.Status != StatusSchemaInvalid {
		t.Fatalf("status %q, want %q: %s", it.Status, StatusSchemaInvalid, out.Message)
	}
	if out.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("exit %d, want %d — schema-invalid is retryable, so a correction "+
			"pass must still be reachable: %s", out.ExitCode, ExitVerifyIncomplete, out.Message)
	}
	if !f.receiptExists() {
		t.Fatal("no receipt for a schema-invalid item — attempts would reset on every " +
			"call and the correction loop would never terminate")
	}
	// END TO END: the retry must be SEEDED. Until this landed, a parse failure
	// produced an empty correction, so the one remaining attempt re-sent the
	// original prompt against the same artifact with no knowledge that it had
	// failed — the unseeded retry defects.go exists to remove, still in place
	// for the one axis whose remedy the parser hands you for free.
	if it.CorrectionPrompt == "" {
		t.Fatal("the receipt carries no correction_prompt for an unparseable sidecar — " +
			"the retry would re-roll the same prompt blind")
	}
	if !strings.Contains(it.CorrectionPrompt, "line ") {
		t.Errorf("correction_prompt names no line: %q", it.CorrectionPrompt)
	}
}
