package sidecar

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// corpusSidecar is a REAL accepted sidecar from this repository, not a
// hand-written fixture. A hand-written one can drift from what the extractor
// actually emits, and the whole point is what happens to real output.
const corpusSidecar = "../../../../docs/architecture/decisions/" +
	"ADR-057-topic-descriptions-are-proposed-once-per-topic.edikt.yaml"

func loadCorpus(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(corpusSidecar)
	if err != nil {
		t.Fatalf("read corpus sidecar: %v", err)
	}
	return string(raw)
}

// TestBaselineCorpusSidecarValidates is the NEGATIVE CONTROL for every test
// below: if the unmodified file did not validate, a rejection in the mutated
// cases would prove nothing about the mutation.
func TestBaselineCorpusSidecarValidates(t *testing.T) {
	if err := ValidateRawAgainstSchemaV2([]byte(loadCorpus(t))); err != nil {
		t.Fatalf("unmodified corpus sidecar must validate; got: %v", err)
	}
}

// TestRejectionNamesTheField is the point of rejectfmt.go. A colleague spent
// two hours on an extraction that failed because one over-length field
// rejected the entire sidecar and nothing said which field.
func TestRejectionNamesTheField(t *testing.T) {
	base := loadCorpus(t)

	cases := []struct {
		name    string
		mutate  func(string) string
		wantAll []string // every substring must appear
	}{
		{
			name: "length overrun names field, index, actual and limit",
			mutate: func(s string) string {
				return strings.Replace(s, "  - text:",
					"  - text: \""+strings.Repeat("x", HardCapProse+100)+"\"\n    _drop:", 1)
			},
			wantAll: []string{
				"directives[0].text",
				fmt.Sprintf("%d chars", HardCapProse+100),
				fmt.Sprintf("max %d", HardCapProse),
			},
		},
		{
			name: "stray key at the document root",
			mutate: func(s string) string {
				return strings.Replace(s, "\npath:", "\nbogus_key: 1\npath:", 1)
			},
			wantAll: []string{"document root", "unknown key", "bogus_key"},
		},
		{
			name: "stray key NESTED inside a directive entry",
			mutate: func(s string) string {
				return strings.Replace(s, "  - text:", "  - not_a_field: 1\n    text:", 1)
			},
			wantAll: []string{"directives[0]", "unknown key", "not_a_field"},
		},
		{
			name: "missing required key",
			mutate: func(s string) string {
				return strings.Replace(s, "topic:", "topic_x:", 1)
			},
			wantAll: []string{"missing required key", "topic"},
		},
		{
			name: "pattern miss names the value AND the pattern",
			mutate: func(s string) string {
				return strings.Replace(s, "topic:", "topic: \"Bad Topic!!\"\n_drop:", 1)
			},
			wantAll: []string{"topic", "Bad Topic!!", "does not match required pattern"},
		},
		{
			name: "bad enum names the field and the allowed set",
			mutate: func(s string) string {
				return strings.Replace(s, "  - text:",
					"  - enforcement: sometimes\n    text:", 1)
			},
			wantAll: []string{"directives[0].enforcement", "must be one of"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mutated := c.mutate(base)
			if mutated == base {
				t.Fatal("CONTROL FAILED: mutation did not change the document")
			}
			err := ValidateRawAgainstSchemaV2([]byte(mutated))
			if err == nil {
				t.Fatal("CONTROL FAILED: mutation was accepted; it must be rejected")
			}
			got := err.Error()
			for _, want := range c.wantAll {
				if !strings.Contains(got, want) {
					t.Errorf("rejection must contain %q\n  got: %s", want, got)
				}
			}
			assertContract(t, got)
		})
	}
}

// assertContract pins Load's error contract AND the host-path leak that v2's
// %w-wrapped raw error carried until 2026-08-30.
func assertContract(t *testing.T, msg string) {
	t.Helper()
	if strings.Contains(msg, "\n") {
		t.Errorf("rejection must be ONE line; got:\n%s", msg)
	}
	if len(msg) >= 500 {
		t.Errorf("rejection must be under 500 chars; got %d", len(msg))
	}
	if strings.Contains(msg, "file://") || strings.Contains(msg, "/Users/") {
		t.Errorf("rejection must not leak a host path or schema URL; got: %s", msg)
	}
}

// TestV1AndV2RejectInTheSameShape. The two schema versions previously reported
// rejections in two different formats -- v1 compacted and truncated at 137
// chars, v2 returned the raw multi-line error. Neither reliably delivered the
// field name.
func TestV1AndV2RejectInTheSameShape(t *testing.T) {
	base := loadCorpus(t)
	over := strings.Replace(base, "  - text:",
		"  - text: \""+strings.Repeat("x", HardCapProse+100)+"\"\n    _drop:", 1)
	v2 := ValidateRawAgainstSchemaV2([]byte(over))
	if v2 == nil {
		t.Fatal("CONTROL FAILED: v2 accepted an over-length field")
	}
	if !strings.Contains(v2.Error(), "directives[0].text") {
		t.Errorf("v2 must name the field; got: %s", v2.Error())
	}
	assertContract(t, v2.Error())
}

// TestManyErrorsDropWholeEntriesNotCharacters. The old compactSchemaError cut
// the joined string at 137 characters, so with several causes the informative
// one was frequently the part removed. A half-written field name reads as
// information and is not.
func TestManyErrorsDropWholeEntriesNotCharacters(t *testing.T) {
	base := loadCorpus(t)
	mutated := base
	for i := 0; i < 40; i++ {
		mutated = strings.Replace(mutated, "\npath:",
			fmt.Sprintf("\nstray_key_with_a_deliberately_long_name_%s_%02d: 1\npath:",
				strings.Repeat("z", 20), i), 1)
	}
	err := ValidateRawAgainstSchemaV2([]byte(mutated))
	if err == nil {
		t.Fatal("CONTROL FAILED: 40 stray keys were accepted")
	}
	msg := err.Error()
	assertContract(t, msg)
	if !strings.Contains(msg, "more)") && !strings.Contains(msg, "list truncated") {
		t.Errorf("dropped content must be reported, not silently lost; got: %s", msg)
	}
	// Whatever survived must be a COMPLETE entry, never a fragment.
	// Every quote must be closed: an odd count means a field name was cut.
	if strings.Count(msg, "'")%2 != 0 {
		t.Errorf("an entry was cut mid-field (unbalanced quotes): %s", msg)
	}
}

func TestPointerToField(t *testing.T) {
	cases := map[string]string{
		"":                       "(document root)",
		"/":                      "(document root)",
		"/topic":                 "topic",
		"/directives/0/text":     "directives[0].text",
		"/directives/12/paths/3": "directives[12].paths[3]",
		"/prohibitions/0/source_excerpts/1/quote": "prohibitions[0].source_excerpts[1].quote",
		"/paths_approval/globs_sha256":            "paths_approval.globs_sha256",
	}
	for ptr, want := range cases {
		if got := pointerToField(ptr); got != want {
			t.Errorf("pointerToField(%q) = %q, want %q", ptr, got, want)
		}
	}
}

// TestThousandsSeparatorsAreParsed is a REGRESSION control for a bug this file
// carried from the moment it was written and that no test could see while every
// cap was three digits: the library renders "got 2,100, want 2,000" once a bound
// passes 999, and the original regexes matched bare digits only. The field name
// still appeared, so the message looked healthy -- it had simply stopped saying
// how long the field was or what the limit is, which is the entire point.
func TestThousandsSeparatorsAreParsed(t *testing.T) {
	got := explain("directives[0].text", "maxLength: got 2,100, want 2,000")
	want := "directives[0].text is 2100 chars, max 2000"
	if got != want {
		t.Errorf("explain() = %q, want %q", got, want)
	}
	// Below the separator threshold, unchanged.
	if got := explain("intent", "maxLength: got 301, want 300"); got != "intent is 301 chars, max 300" {
		t.Errorf("three-digit case regressed: %q", got)
	}
}
