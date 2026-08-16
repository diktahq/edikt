package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// "Which artifacts need a sidecar" must have exactly one answer. cmd used to
// carry its own copy of the rule, which drifted: it never read frontmatter
// `status: superseded|deprecated`, and its body regex required a `**Status:**`
// prefix so it missed `**Superseded by ADR-NNN**` and every `Deprecated` line.
//
// The result was tools disagreeing in the field: `gov compile` and `verify`
// skipped retired artifacts while `doctor` reported them as MISSING sidecars —
// five in edikt's own repo, four in a consumer repo where acting on doctor's
// advice would have recreated sidecars an owner ruling deliberately deleted.
//
// These cases are the shapes that actually occur in the corpus. Each must be
// skipped, and cmd's answer must equal internal/sidecar's.
func TestSkipList_ParityWithSharedPredicate(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantSkip bool
	}{
		{
			name:     "frontmatter superseded, body without Status prefix (ADR-031/033 shape)",
			body:     "---\nid: ADR-031\nstatus: superseded\nsuperseded_by: ADR-052\n---\n\n# ADR-031\n\n**Superseded by ADR-052**\n",
			wantSkip: true,
		},
		{
			name:     "frontmatter deprecated + body Deprecated (ADR-014/019 shape)",
			body:     "---\nid: ADR-014\nstatus: deprecated\n---\n\n# ADR-014\n\n**Status:** Deprecated\n",
			wantSkip: true,
		},
		{
			name:     "no frontmatter, body Deprecated only (ADR-003 shape)",
			body:     "# ADR-003\n\n**Status:** Deprecated\n",
			wantSkip: true,
		},
		{
			name:     "canonical superseded status line (ADR-046 shape)",
			body:     "---\nid: ADR-046\nstatus: superseded\n---\n\n# ADR-046\n\n**Status:** Superseded by ADR-052\n",
			wantSkip: true,
		},
		{
			name:     "migration:skip frontmatter",
			body:     "---\nid: ADR-009\nmigration: skip\nreason: \"legacy block preserved\"\n---\n\n# ADR-009\n",
			wantSkip: true,
		},
		{
			name:     "live accepted ADR is NOT skipped",
			body:     "---\nid: ADR-052\nstatus: accepted\n---\n\n# ADR-052\n\n**Status:** Accepted\n",
			wantSkip: false,
		},
	}

	dir := t.TempDir()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(dir, "artifact.md")
			if err := os.WriteFile(p, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			gotCmd, _ := isSkipListed(p)
			gotShared, _ := sidecar.IsSkipListed(p)

			if gotCmd != c.wantSkip {
				t.Errorf("cmd.isSkipListed = %v, want %v", gotCmd, c.wantSkip)
			}
			if gotCmd != gotShared {
				t.Errorf("PARITY BROKEN: cmd=%v shared=%v — the rule has two answers again",
					gotCmd, gotShared)
			}
		})
	}
}

// README files in artifact dirs are documentation, not governance. Both
// predicates must agree on that too.
func TestSkipList_ReadmeParity(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "README.md")
	if err := os.WriteFile(p, []byte("# Decisions\n\nThis directory holds ADRs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gotCmd, _ := isSkipListed(p)
	gotShared, _ := sidecar.IsSkipListed(p)
	if !gotCmd || !gotShared {
		t.Fatalf("README not skipped: cmd=%v shared=%v", gotCmd, gotShared)
	}
}
