package hook

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestPostCompactRendersAttemptOnlyWhenWellFormed is the Go-side successor to
// test-v040-plan-harness.sh's retired "PostCompact hook extracts ATTEMPT
// variable" and "has backward compatibility check (grep -qE)" assertions.
// Both grepped post-compact.sh, which is now a ten-line exec shim.
//
// The corpus cannot stand in for them: deleting the attempt suffix from
// postCompactPlanMessage leaves `dev hook-corpus diff --hook post-compact` at
// 4/4, because no recorded scenario has a plan row carrying a fourth field.
// Confirmed by mutation before this test was written.
//
// The backward-compatibility half is the point of the second case. The bash
// original only used ATTEMPT when it matched N/N, so a plan table whose
// fourth column holds something else (a date, a note) renders without an
// attempt clause rather than printing garbage into the recovery message.
func TestPostCompactRendersAttemptOnlyWhenWellFormed(t *testing.T) {
	row := func(fourth string) string {
		// Column order matches the real progress table:
		// | Phase | Status | Attempt | Updated |  e.g. "| 12 | done | 1/5 | 2026-08-21 |"
		return "# Plan: Demo\n\n" +
			"| Phase | Status | Attempt | Updated |\n" +
			"|-------|--------|---------|---------|\n" +
			"| 2 | in progress | " + fourth + " | 2026-03-20 |\n"
	}

	cases := []struct {
		name   string
		fourth string
		want   string
		absent bool
	}{
		{name: "well-formed N/N renders", fourth: "2/5", want: "(attempt 2/5)"},
		{name: "non-conforming field is dropped", fourth: "2026-03-20", want: "(attempt", absent: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cwd := t.TempDir()
			t.Chdir(cwd)
			writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), "base: docs\n")
			writeTestFile(t, filepath.Join(cwd, "docs", "plans", "PLAN-001-demo.md"), row(tc.fourth))

			got := postCompactPlanMessage(cwd, filepath.Join("docs", "plans"))
			if got == "" {
				t.Fatalf("no plan message rendered at all — the fixture is not being read")
			}
			if tc.absent {
				if strings.Contains(got, tc.want) {
					t.Errorf("fourth field %q is not N/N, want no attempt clause; got %q", tc.fourth, got)
				}
				// Positive control: the message must still name the phase, or
				// this case would pass on an empty-ish render.
				if !strings.Contains(got, "Phase 2") {
					t.Errorf("message lost the phase entirely: %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("want %q in message, got %q", tc.want, got)
			}
		})
	}
}
