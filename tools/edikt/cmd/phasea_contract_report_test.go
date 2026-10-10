package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestPhaseaPlan_ReportsRejectedContractOnStderr — PLAN-v090-issue-fixes
// Phase 3 (#11 comment item 1, report half). Only `gov reextract` consulted
// the extraction contract's validation record, so `phasea plan` — the path
// every per-artifact compile takes — planned against a contract recorded as
// REJECTED without a word. The plan still goes out (exit 0): this is a
// report, not a refusal, because a refusal here would also block
// `gov compile` and the validation run itself.
func TestPhaseaPlan_ReportsRejectedContractOnStderr(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int
		want    []string // substrings of the stderr report; nil means no report
	}{
		{"rejected", 11, []string{"extraction contract v11", "REJECTED", "reported, not refused"}},
		{"unrecorded", 99, []string{"extraction contract v99", "no validation record", "reported, not refused"}},
		{"validated", 4, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &bbProject{root: t.TempDir(), home: t.TempDir()}
			bbWriteExtractorAgent(t, p.root, tc.version)
			bbWriteConfig(t, p.root, "")
			p.adr = bbWriteADR(t, p.root, "ADR-900", gateQuoteBB)
			rel := filepath.ToSlash(filepath.Join("docs", "architecture", "decisions", filepath.Base(p.adr)))

			stdout, stderr, code := p.run(t, "phasea", "plan", "--artifact", rel, "--bootstrap")
			if code != 0 || bbPlanIDRe.FindString(stdout) == "" {
				t.Fatalf("plan under prompt_version %d: exit %d, want 0 with a plan\nstdout: %s\nstderr: %s",
					tc.version, code, stdout, stderr)
			}
			if tc.want == nil {
				if strings.Contains(stderr, "extraction contract") {
					t.Errorf("a validated contract must not be reported:\n%s", stderr)
				}
				return
			}
			for _, w := range tc.want {
				if !strings.Contains(stderr, w) {
					t.Errorf("stderr must say %q:\n%s", w, stderr)
				}
			}
			if n := strings.Count(strings.TrimSpace(stderr), "\n"); n != 0 {
				t.Errorf("the report must be one stderr line, got %d:\n%s", n+1, stderr)
			}
			if strings.Contains(stdout, "extraction contract") {
				t.Errorf("the contract report belongs on stderr, not stdout:\n%s", stdout)
			}
		})
	}
}
