package normative

import "testing"

// duplicate_subsumed fires ZERO times on the live corpus. That is a result,
// not coverage: a branch only ever run against non-matching input is
// untested no matter how many items pass through it (GL-002). These cases
// are the only thing standing between "no subsumed duplicates exist" and
// "the subsumed check never worked".
func TestContainsKey(t *testing.T) {
	cases := []struct {
		name         string
		outer, inner string
		want         bool
	}{
		{"whole key inside a longer one",
			"tier 2 go binaries must not spawn invoke or shell out to any llm cli in any circumstance",
			"tier 2 go binaries must not spawn invoke or shell out to any llm cli",
			true},
		{"inner at the start", "the sidecar must be spec only and nothing else", "the sidecar must be spec only", true},
		{"inner at the end", "under no circumstances must the parent md be written", "must the parent md be written", true},

		// Identical keys are duplicate_exact, a different verdict. If
		// containment also claimed them, every exact pair would be reported
		// twice under two names.
		{"identical keys are not containment", "the same words", "the same words", false},
		{"empty inner", "some rule text", "", false},
		{"inner longer than outer", "short", "a much longer key", false},

		// Word-boundary enforcement. Without it "must not" matches inside
		// "must note", and unrelated rules read as restatements.
		{"partial word is not containment", "the runner must note the failure", "must not", false},
		{"prefix of a word is not containment", "compile the sidecars", "compil", false},
		{"disjoint keys", "tier 1 commands are markdown", "tier 2 binaries are go", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := containsKey(c.outer, c.inner); got != c.want {
				t.Errorf("containsKey(%q, %q) = %v, want %v", c.outer, c.inner, got, c.want)
			}
		})
	}
}
