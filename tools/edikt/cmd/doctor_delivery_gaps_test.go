package cmd

// Every check here gets a POSITIVE CONTROL: a project built with the defect,
// asserting the check FIRES and names the consequence. A test that only proves
// a healthy project passes cannot tell a working detector from one that returns
// nil — two false results on 2026-08-29 were exactly that shape.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gapProject(t *testing.T) string {
	t.Helper()
	isolateDoctorEnv(t, true)
	root := newGapProject(t)
	installValidatedExtractor(t, root)
	return root
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runGaps returns the TOTAL finding count — errors plus warnings — because
// every control below asks the same question of it: did this check fire at
// all. The error/warning split is asserted where it is the property under
// test, in TestAcceptedWithoutSidecarIsRed.
func runGaps(t *testing.T, root string) (string, int, bool) {
	t.Helper()
	var buf bytes.Buffer
	e, w, ran := runDeliveryGapsCheck(root, &buf)
	return buf.String(), e + w, ran
}

func TestDeliveryGaps_HealthyProjectIsQuiet(t *testing.T) {
	out, warns, ran := runGaps(t, gapProject(t))
	if !ran {
		t.Fatal("check did not run on a compiled project")
	}
	if warns != 0 {
		t.Errorf("healthy project warned %d times:\n%s", warns, out)
	}
}

// CONTROL 1 — empty ambient core, and separately an index with no globs.
func TestDeliveryGaps_FiresOnEmptyDeliverySurface(t *testing.T) {
	root := gapProject(t)
	write(t, root, ".claude/rules/governance.md", "# Governance\n\nNo directives here.\n")
	out, warns, _ := runGaps(t, root)
	if warns == 0 || !strings.Contains(out, "ambient core carries no directives") {
		t.Fatalf("empty ambient core not reported:\n%s", out)
	}
	if !strings.Contains(out, "EVERY edit") {
		t.Errorf("finding does not name the consequence:\n%s", out)
	}

	root2 := gapProject(t)
	write(t, root2, ".claude/rules/governance/delivery-map.yaml", "schema_version: 1\nglobs: {}\n")
	out2, warns2, _ := runGaps(t, root2)
	if warns2 == 0 || !strings.Contains(out2, "delivery map has no globs") {
		t.Fatalf("unrouted index not reported:\n%s", out2)
	}
}

// CONTROL 2 — a standing queue, which no later compile mentions.
func TestDeliveryGaps_FiresOnStandingProposalQueue(t *testing.T) {
	root := gapProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".edikt/state/pending-paths"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, ".edikt/state/pending-paths/ADR-057.yaml", "artifact: ADR-057\n")
	out, warns, _ := runGaps(t, root)
	if warns == 0 || !strings.Contains(out, "path-scope proposal(s) awaiting approval") {
		t.Fatalf("standing queue not reported:\n%s", out)
	}
	if !strings.Contains(out, "do not route") {
		t.Errorf("finding does not name the delivery consequence:\n%s", out)
	}
}

// CONTROL 3 — an ACCEPTED artifact with no sidecar fires; a superseded one does
// not. The negative half matters: without it the check reports eleven
// legitimate exclusions alongside the one real finding.
func TestDeliveryGaps_FiresOnAcceptedArtifactWithoutSidecar(t *testing.T) {
	root := gapProject(t)
	write(t, root, "docs/architecture/decisions/ADR-009-terminology.md",
		"# ADR-009: Terminology\n\n**Status:** Accepted\n\n## Decision\n\nSomething.\n")
	out, warns, _ := runGaps(t, root)
	if warns == 0 || !strings.Contains(out, "ADR-009") {
		t.Fatalf("accepted artifact without a sidecar not reported:\n%s", out)
	}
	if !strings.Contains(out, "governs nothing") {
		t.Errorf("finding does not name the consequence:\n%s", out)
	}

	// Negative control: superseded and proposed are legitimate exclusions.
	root2 := gapProject(t)
	write(t, root2, "docs/architecture/decisions/ADR-008-superseded.md",
		"# ADR-008\n\n**Status:** Superseded by ADR-027\n")
	write(t, root2, "docs/architecture/decisions/ADR-080-proposed.md",
		"---\nstatus: proposed\n---\n\n# ADR-080\n")
	if out2, warns2, _ := runGaps(t, root2); warns2 != 0 {
		t.Errorf("superseded/proposed artifacts must not be reported as gaps:\n%s", out2)
	}
}

// AC-3.4's OTHER half. TestDeliveryGaps_FiresOnAcceptedArtifactWithoutSidecar
// proves the check FIRES and that a proposed artifact is not counted. It says
// nothing about the two words the criterion actually turns on: the finding is
// an ERROR, and it carries the REMEDY. Both were absent — the whole group
// printed `warn` and no gap type had a remedy field at all — and the phase
// closed on an alternation where only the sibling test's name had to match.
//
// SEVERITY is the property, not the glyph. A warning is a line doctor already
// prints forty-three of on this repo; an accepted record that governs nothing
// is a false belief about what is being enforced, and doctor's exit code has
// to carry it (INV-013).
//
// Falsification, run before this test was trusted: with Fatal removed from
// artifactsWithoutSidecar the errs assertion fails (`got 0 error(s), 1
// warning(s)`); with Remedy removed the remedy assertion fails on output that
// otherwise still contains the id, the consequence and the RED glyph. Neither
// is reachable from the sibling test above, which passes under both mutations.
func TestAcceptedWithoutSidecarIsRed(t *testing.T) {
	root := gapProject(t)
	write(t, root, "docs/architecture/decisions/ADR-009-terminology.md",
		"# ADR-009: Terminology\n\n**Status:** Accepted\n\n## Decision\n\nSomething.\n")

	var buf bytes.Buffer
	errs, warns, ran := runDeliveryGapsCheck(root, &buf)
	out := buf.String()
	if !ran {
		t.Fatal("check did not run on a compiled project")
	}
	if errs != 1 || warns != 0 {
		t.Fatalf("an accepted artifact with no sidecar must count as 1 error, 0 warnings; got %d error(s), %d warning(s):\n%s", errs, warns, out)
	}
	if !strings.Contains(out, "🔴 ERROR:") {
		t.Errorf("the finding must print as a RED error, not a warning:\n%s", out)
	}
	// The remedy is the command that closes it, and it is the per-kind
	// compile command: `gov compile` merges sidecars that exist and would
	// exit 0 having written none of the missing ones.
	if !strings.Contains(out, "run: /edikt:adr:compile") {
		t.Errorf("the finding does not name the remedy:\n%s", out)
	}
	// The remedy has to be the right one for the KIND that is missing —
	// telling an invariant's author to run adr:compile is a remedy that
	// cannot work.
	root2 := gapProject(t)
	write(t, root2, "docs/architecture/invariants/INV-042-thing.md",
		"---\nstatus: active\n---\n\n# INV-042\n\n## Statement\n\nSomething MUST hold.\n")
	buf.Reset()
	errs2, _, _ := runDeliveryGapsCheck(root2, &buf)
	if errs2 != 1 || !strings.Contains(buf.String(), "run: /edikt:invariant:compile") {
		t.Errorf("a missing invariant sidecar must name invariant:compile as the remedy; got %d error(s):\n%s", errs2, buf.String())
	}

	// Negative control, restated at THIS severity: a proposed artifact must
	// not be an error either. Without it, "is RED" could be satisfied by a
	// check that reddens on every artifact it sees.
	root3 := gapProject(t)
	write(t, root3, "docs/architecture/decisions/ADR-080-proposed.md",
		"---\nstatus: proposed\n---\n\n# ADR-080\n")
	buf.Reset()
	errs3, warns3, _ := runDeliveryGapsCheck(root3, &buf)
	if errs3 != 0 || warns3 != 0 {
		t.Errorf("a proposed artifact must be counted as neither error nor warning; got %d/%d:\n%s", errs3, warns3, buf.String())
	}
}

// CONTROL 4 — `accepted` on an invariant is the trap: it is the word every ADR
// uses, it matches neither branch of the filter, and the invariant is dropped.
func TestDeliveryGaps_FiresOnInvariantStatusThatIsNotActive(t *testing.T) {
	root := gapProject(t)
	write(t, root, "docs/architecture/invariants/INV-042-thing.md",
		"---\nstatus: accepted\n---\n\n# INV-042\n\n## Statement\n\nSomething MUST hold.\n")
	write(t, root, "docs/architecture/invariants/INV-042-thing.edikt.yaml", "schema_version: 2\n")
	out, warns, _ := runGaps(t, root)
	if warns == 0 || !strings.Contains(out, "INV-042") {
		t.Fatalf("invariant excluded by status not reported:\n%s", out)
	}
	if !strings.Contains(out, "accepted") {
		t.Errorf("finding does not name the offending status word:\n%s", out)
	}

	// Negative control: `active` and no-status are both included.
	root2 := gapProject(t)
	write(t, root2, "docs/architecture/invariants/INV-001-a.md", "---\nstatus: active\n---\n\n# INV-001\n")
	write(t, root2, "docs/architecture/invariants/INV-001-a.edikt.yaml", "schema_version: 2\n")
	write(t, root2, "docs/architecture/invariants/INV-002-b.md", "# INV-002\n\nLegacy, no status.\n")
	write(t, root2, "docs/architecture/invariants/INV-002-b.edikt.yaml", "schema_version: 2\n")
	if out2, warns2, _ := runGaps(t, root2); warns2 != 0 {
		t.Errorf("active/no-status invariants must not be reported:\n%s", out2)
	}
}

// INV-013: an uncompiled project is UNMEASURED, not healthy.
func TestDeliveryGaps_DoesNotRunWithoutConfig(t *testing.T) {
	if _, _, ran := runGaps(t, t.TempDir()); ran {
		t.Error("check claimed to run on a project with no edikt config")
	}
}

// D47 (3): pending behavioral-verify proposals are reported as a count with
// the remedy, so a compile that routed a pairless behavioral (rather than
// refusing) leaves a standing signal in doctor.
func TestDeliveryGaps_FiresOnPendingVerifies(t *testing.T) {
	root := gapProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".edikt/state/pending-verifies"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, ".edikt/state/pending-verifies/ADR-061-p00.yaml", "id: ADR-061-p00\nentry_kind: prohibition\nproposed_verify: bash test/x.sh\nproposed_at: \"2026-08-27T09:00:00Z\"\n")
	write(t, root, ".edikt/state/pending-verifies/ADR-061-d01.yaml", "id: ADR-061-d01\nproposed_verify: bash test/y.sh\nproposed_at: \"2026-09-02T09:00:00Z\"\n")
	out, warns, _ := runGaps(t, root)
	if warns == 0 || !strings.Contains(out, "2 behavioral verify proposal(s) awaiting approval, oldest ") {
		t.Fatalf("pending verifies not reported as a count with the oldest age (D51 b):\n%s", out)
	}
	if !strings.Contains(out, "ADR-061-d01, ADR-061-p00") {
		t.Fatalf("the ids must be listed:\n%s", out)
	}
	if strings.Contains(out, "oldest 0 d") {
		t.Fatalf("the oldest is the 2026-08-27 one, days old, never 0:\n%s", out)
	}
	if !strings.Contains(out, "compiles without its verify") || !strings.Contains(out, "sidecar approve") {
		t.Errorf("finding must name the consequence and the remedy:\n%s", out)
	}
}
