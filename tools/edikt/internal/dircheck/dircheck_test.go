package dircheck

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func TestCheck_CleanDirective(t *testing.T) {
	got := Check(Input{
		ADRID:            "ADR-012",
		DirectiveBody:    "All DB access MUST go through the repository layer.",
		CanonicalPhrases: []string{"repository layer"},
	})
	if len(got) != 0 {
		t.Fatalf("expected no warnings, got: %v", got)
	}
}

func TestCheck_LengthVsCanonical_Triggers(t *testing.T) {
	got := Check(Input{
		ADRID:         "ADR-012",
		DirectiveBody: "All DB access MUST go through the repository layer. NEVER bypass the repository.",
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "directive has 2 sentences but no canonical_phrases") {
		t.Errorf("warning text mismatch: %q", got[0])
	}
}

func TestCheck_LengthCheck_RefTailStripped(t *testing.T) {
	// `(ref: ADR-001)` tail is stripped before sentence counting.
	got := Check(Input{
		ADRID:         "ADR-012",
		DirectiveBody: "Use the repository layer. (ref: ADR-001, §Eviction)",
	})
	for _, w := range got {
		if strings.Contains(w, "sentences") {
			t.Errorf("unexpected length warning when ref tail strips to single sentence: %q", w)
		}
	}
}

func TestCheck_PhraseMissing(t *testing.T) {
	got := Check(Input{
		ADRID:            "ADR-014",
		DirectiveBody:    "Use os.Rename for tmp+rename.",
		CanonicalPhrases: []string{"atomic rename"},
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got: %v", got)
	}
	if !strings.Contains(got[0], `canonical_phrase "atomic rename" not found`) {
		t.Errorf("warning text mismatch: %q", got[0])
	}
}

func TestCheck_PhraseCaseInsensitive(t *testing.T) {
	got := Check(Input{
		ADRID:            "ADR-014",
		DirectiveBody:    "MUST use Atomic Rename when persisting state.",
		CanonicalPhrases: []string{"atomic rename"},
	})
	if len(got) != 0 {
		t.Fatalf("phrase should match case-insensitively, got warnings: %v", got)
	}
}

func TestCheck_NoDirectivesReason_Forbidden(t *testing.T) {
	for _, reason := range []string{"tbd", "TODO", "fix later", "  TBD  "} {
		got := Check(Input{
			ADRID:              "ADR-099",
			DirectiveBody:      "n/a",
			NoDirectivesReason: ptr(reason),
		})
		if len(got) != 1 {
			t.Errorf("reason %q: expected 1 warning, got %d", reason, len(got))
		}
	}
}

func TestCheck_NoDirectivesReason_TooShort(t *testing.T) {
	got := Check(Input{
		ADRID:              "ADR-099",
		DirectiveBody:      "n/a",
		NoDirectivesReason: ptr("nope"),
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got: %v", got)
	}
	if !strings.Contains(got[0], "is not acceptable") {
		t.Errorf("warning text mismatch: %q", got[0])
	}
}

func TestCheck_NoDirectivesReason_Empty(t *testing.T) {
	got := Check(Input{
		ADRID:              "ADR-099",
		DirectiveBody:      "n/a",
		NoDirectivesReason: ptr("   "),
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got: %v", got)
	}
}

func TestCheck_NoDirectivesReason_Acceptable(t *testing.T) {
	got := Check(Input{
		ADRID:              "ADR-099",
		DirectiveBody:      "n/a",
		NoDirectivesReason: ptr("This ADR is purely organisational; it ships no enforceable rule."),
	})
	if len(got) != 0 {
		t.Fatalf("expected no warnings, got: %v", got)
	}
}

func TestCheck_NoDirectivesReason_NilSkipsCheckC(t *testing.T) {
	// Bug-watch: if Check C is reached when reason is nil we'd surface
	// a spurious warning. Guard against that regression.
	got := Check(Input{
		ADRID:         "ADR-099",
		DirectiveBody: "n/a",
	})
	if len(got) != 0 {
		t.Fatalf("expected no warnings when no_directives_reason is null, got: %v", got)
	}
}

func TestCheck_BothBAndCFire(t *testing.T) {
	got := Check(Input{
		ADRID:              "ADR-XYZ",
		DirectiveBody:      "Single clause body.",
		CanonicalPhrases:   []string{"missing phrase"},
		NoDirectivesReason: ptr("tbd"),
	})
	// Check A does not fire: 1 clause. Check B fires: phrase missing.
	// Check C fires: forbidden placeholder.
	if len(got) != 2 {
		t.Fatalf("expected 2 warnings (B,C), got %d: %v", len(got), got)
	}
}

func TestCheck_AAndCFire_NoPhrases(t *testing.T) {
	got := Check(Input{
		ADRID:              "ADR-XYZ",
		DirectiveBody:      "First clause. Second clause; third clause.",
		NoDirectivesReason: ptr("tbd"),
	})
	// Check A fires: 3 clauses, no canonical_phrases.
	// Check C fires: forbidden placeholder.
	if len(got) != 2 {
		t.Fatalf("expected 2 warnings (A,C), got %d: %v", len(got), got)
	}
}

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"single clause", 1},
		{"first. second.", 2},
		{"first; second.", 2},
		{"a! b? c.", 3},
		{"trailing.", 1},
		{"", 0},
		{"   ", 0},
		{"a.b.c", 1}, // no whitespace after period — single clause
	}
	for _, c := range cases {
		got := splitSentences(c.in)
		if len(got) != c.want {
			t.Errorf("splitSentences(%q): want %d clauses, got %d (%v)", c.in, c.want, len(got), got)
		}
	}
}

func TestCheck_DeterministicOrdering(t *testing.T) {
	in := Input{
		ADRID:              "ADR-XYZ",
		DirectiveBody:      "a. b. c.",
		CanonicalPhrases:   []string{"x", "y"},
		NoDirectivesReason: ptr("tbd"),
	}
	first := Check(in)
	second := Check(in)
	if len(first) != len(second) {
		t.Fatalf("non-deterministic length: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("non-deterministic at %d: %q vs %q", i, first[i], second[i])
		}
	}
}
