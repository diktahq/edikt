package gov

import (
	"strings"
	"testing"
)

// The gate is a security control: intent and falsifying_observation come from
// artifact prose and end up in a prompt sent to a verifier.
func TestExtractDirectivesGateDropsTheFieldNotTheDirective(t *testing.T) {
	ctrl := "has" + string(rune(1)) + "a control char"
	body := "[edikt:directives:start]: #\n" +
		"- text: keeps its intent\n  intent: fine\n" +
		"- text: control char\n  intent: " + ctrl + "\n" +
		"- text: overlong\n  intent: " + strings.Repeat("x", 400) + "\n" +
		"[edikt:directives:end]: #\n"
	got := extractDirectives(body, "ADR-1")
	if len(got) != 3 {
		t.Fatalf("got %d directives, want 3 — a bad FIELD must not remove the whole rule from the run", len(got))
	}
	if got[0].Intent != "fine" {
		t.Errorf("clean intent was dropped: %q", got[0].Intent)
	}
	for _, i := range []int{1, 2} {
		if got[i].Intent != "" {
			t.Errorf("directive %d kept an inadmissible intent: %q", i, got[i].Intent)
		}
		if got[i].Text == "" {
			t.Errorf("directive %d lost its text — the gate rejected the entry, not the field", i)
		}
	}
}

// The gate normalises to DECIDE and returns the ORIGINAL. Emitting the
// casefolded form would put a mangled sentence in front of the verifier and
// call it the directive's intent.
func TestExtractDirectivesReturnsTheAuthorsText(t *testing.T) {
	body := "[edikt:directives:start]: #\n- text: T\n  intent: Mixed Case Intent With CAPS\n[edikt:directives:end]: #\n"
	got := extractDirectives(body, "R")
	if len(got) != 1 {
		t.Fatal("fixture produced no directive")
	}
	if got[0].Intent != "Mixed Case Intent With CAPS" {
		t.Errorf("intent = %q — normalisation decides admissibility, it is not what gets delivered", got[0].Intent)
	}
}

func TestExtractDirectivesHandlesBothFormatsAndBlankLines(t *testing.T) {
	body := "[edikt:directives:start]: #\n" +
		"- a bare bullet\n" +
		"- text: multi-line\n\n  intent: a blank line must not end the entry\n" +
		"[edikt:directives:end]: #\n"
	got := extractDirectives(body, "R")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Text != "a bare bullet" {
		t.Errorf("simple format: %q", got[0].Text)
	}
	if got[1].Intent != "a blank line must not end the entry" {
		t.Errorf("a blank line ended the entry: %+v", got[1])
	}
	for i, d := range got {
		want := "R.directive[" + string(rune('0'+i)) + "]"
		if d.DirectiveID != want {
			t.Errorf("directive_id = %q, want %q", d.DirectiveID, want)
		}
	}
}

// An ABSENT region is an empty array, never nil: a nil slice marshals to
// `null`, and a caller measuring its length gets an error rather than zero.
func TestExtractDirectivesAbsentRegionIsEmptyNotNil(t *testing.T) {
	got := extractDirectives("# nothing here\n", "R")
	if got == nil {
		t.Fatal("nil slice — marshals to null, which a caller cannot take len() of")
	}
	if len(got) != 0 {
		t.Errorf("got %d directives from a file with no region", len(got))
	}
}
