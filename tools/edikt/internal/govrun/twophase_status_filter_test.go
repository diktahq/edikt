package govrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// pairFor builds the minimal sidecar.Pair statusExcluded reads: the parent
// path and the artifact class.
func pairFor(parentPath, kind string) sidecar.Pair {
	return sidecar.Pair{ParentPath: parentPath, Kind: kind}
}

// F-069: ADR-038 carried `status: proposed` and compiled 165 directive-index
// entries anyway, which the write-time tier delivered as MUST-grade denies.
// ADR-020:d03 requires the tier-2 compile helper to filter source documents by
// status; nothing on the sidecar path did. sidecar.Discover carries a DENYLIST
// (superseded / deprecated / migration:skip) and parse.IsIncluded carries the
// ALLOWLIST the ADR describes, and the denylist is the copy on the path that
// runs.
//
// The test asserts BOTH directions, and the second is the load-bearing one.
//
// A status filter written fresh against frontmatter alone passes the negative
// assertion and is still wrong: in this repo's corpus it would take 344
// directive-index entries dark rather than 165, because ADR-001, ADR-007,
// ADR-010 and ADR-060 carry no frontmatter status and declare acceptance in a
// `**Status:** Accepted` body line. parse.IsIncluded already implements that
// fallback; a third copy of the decision would forget it. Only the positive
// assertion below distinguishes delegating from re-implementing.
func TestRunTwoPhase_StatusFilter_ProposedExcluded_BodyAcceptedIncluded(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(adrDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// (1) Frontmatter-accepted ADR. The baseline: proves the pipeline is live,
	// so a green negative assertion cannot come from a compile that produced
	// nothing at all.
	write("ADR-001-frontmatter-accepted.md",
		"---\nstatus: accepted\n---\n\n# ADR-001 — Frontmatter accepted\n\n## Decision\n\nTest rule.\n")

	// (2) BODY-accepted ADR — no `status:` key in frontmatter at all, accepted
	// only via the bolded body line. This is the shape parse.IsIncluded's
	// fallback exists for, and the shape a frontmatter-only allowlist silently
	// drops. "Test rule." must sit on line 9 to match the runner's anchor.
	write("ADR-004-body-accepted.md",
		"---\nid: ADR-004\n---\n\n# ADR-004 — Body accepted\n\n**Status:** Accepted\n\nTest rule.\n")

	// (3) `proposed` ADR — F-069's subject. Given a sidecar ON DISK carrying a
	// marker directive, so the negative assertion tests the Phase B merge
	// filter and not merely the absence of a Phase A dispatch. Anchor-valid on
	// purpose: without the filter this directive compiles cleanly and leaks,
	// which is the failure the test must be able to observe.
	write("ADR-038-proposed.md",
		"---\nstatus: proposed\n---\n\n# ADR-038 — Proposed\n\n## Decision\n\nProposed rule.\n")
	write("ADR-038-proposed.edikt.yaml", `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-038-proposed.md"
signals:
  - "proposed signal"
directives:
  - text: "PROPOSED-MARKER rule MUST hold. (ref: ADR-038)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Proposed rule."
`)

	runner := &skipRunner{}
	var errBuf, outBuf bytes.Buffer
	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      runner,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	if err != nil {
		t.Fatalf("RunTwoPhase: %v\nstderr:\n%s", err, errBuf.String())
	}
	if res.PhaseB == nil {
		t.Fatal("Phase B did not run")
	}

	// No LLM call is spent on an artifact whose directives get discarded.
	for _, id := range runner.dispatched {
		if id == "ADR-038" {
			t.Errorf("proposed artifact dispatched to extractor: %v", runner.dispatched)
		}
	}
	// ...and the two accepted ones WERE dispatched, so "nothing dispatched"
	// cannot be mistaken for "the proposed one was filtered".
	dispatched := strings.Join(runner.dispatched, ",")
	for _, id := range []string{"ADR-001", "ADR-004"} {
		if !strings.Contains(dispatched, id) {
			t.Errorf("accepted %s was not dispatched; got %v", id, runner.dispatched)
		}
	}

	// The exclusion is announced, not silent (INV-015). 165 index entries
	// going dark with no line of output is the same class of event as them
	// arriving with no line of output.
	if !strings.Contains(errBuf.String(), "skip: ADR-038") {
		t.Errorf("expected an announced skip for ADR-038, stderr:\n%s", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "proposed") {
		t.Errorf("skip announcement must name the disqualifying status, stderr:\n%s", errBuf.String())
	}

	// Scan every rendered surface, not just .claude/rules/ — since SPEC-011
	// stage 1 an unscoped topic renders to a skill package instead, and a
	// rules-only scan would stop looking exactly where a leak now lands.
	var compiled strings.Builder
	for _, dir := range []string{
		filepath.Join(root, ".claude", "rules", "governance"),
		filepath.Join(root, ".claude", "skills"),
	} {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil //nolint:nilerr // absent surface is not an error here
			}
			if b, rerr := os.ReadFile(p); rerr == nil {
				compiled.Write(b)
			}
			return nil
		})
	}
	if idx, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md")); err == nil {
		compiled.Write(idx)
	}
	// PIN THE SUBJECT: an empty scan satisfies every negative assertion below
	// while proving nothing (INV-013).
	if compiled.Len() == 0 {
		t.Fatal("no compiled surfaces found to scan — the assertions below had no subject")
	}
	out := compiled.String()

	// NEGATIVE — the proposed ADR contributes zero.
	if strings.Contains(out, "PROPOSED-MARKER") {
		t.Error("proposed ADR-038's directive reached compiled governance")
	}
	if strings.Contains(out, "ref: ADR-038") {
		t.Error("proposed ADR-038 contributed a directive to compiled governance")
	}

	// POSITIVE — the body-fallback ADR still contributes. This is the
	// assertion a frontmatter-only allowlist fails.
	if !strings.Contains(out, "ref: ADR-004") {
		t.Errorf("body-accepted ADR-004 contributed nothing — the status filter "+
			"is not honouring the `**Status:** Accepted` body fallback that "+
			"parse.IsIncluded implements.\ncompiled surfaces:\n%s", out)
	}
	// Control for the positive assertion: frontmatter-accepted still works, so
	// a failure above is specifically about the body fallback and not about
	// the filter rejecting everything.
	if !strings.Contains(out, "ref: ADR-001") {
		t.Error("frontmatter-accepted ADR-001 contributed nothing — the filter is over-broad")
	}
}

// statusExcluded's verdicts, unit-level. RunTwoPhase exercises the wiring; this
// pins the decision table itself, including the two cases that must NOT be
// treated as a status verdict: an unclassified parent and an unreadable one.
func TestStatusExcluded_DecisionTable(t *testing.T) {
	root := t.TempDir()

	writeArtifact := func(name, content string) string {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	cases := []struct {
		name     string
		kind     string
		content  string
		file     string
		excluded bool
	}{
		{"adr frontmatter accepted", "adr", "---\nstatus: accepted\n---\n\n# A\n", "a1.md", false},
		{"adr body accepted, no fm status", "adr", "---\nid: X\n---\n\n**Status:** Accepted\n", "a2.md", false},
		{"adr proposed", "adr", "---\nstatus: proposed\n---\n\n# A\n", "a3.md", true},
		{"adr draft", "adr", "---\nstatus: draft\n---\n\n# A\n", "a4.md", true},
		{"adr no status anywhere", "adr", "---\nid: X\n---\n\n# A\n", "a5.md", true},
		{"inv active", "invariant", "---\nstatus: active\n---\n\n# I\n", "i1.md", false},
		{"inv legacy no status", "invariant", "---\nid: X\n---\n\n# I\n", "i2.md", false},
		{"inv revoked", "invariant", "---\nstatus: revoked\n---\n\n# I\n", "i3.md", true},
		{"guideline any status", "guideline", "---\nstatus: whatever\n---\n\n# G\n", "g1.md", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeArtifact(tc.file, tc.content)
			got, reason := statusExcluded(pairFor(p, tc.kind))
			if got != tc.excluded {
				t.Errorf("statusExcluded = %v (%q), want %v", got, reason, tc.excluded)
			}
			if got && reason == "" {
				t.Error("an exclusion must carry a reason a user can act on")
			}
		})
	}

	// An unclassified parent is not a status verdict — this filter drops
	// artifacts whose status is known and disqualifying, never artifacts whose
	// class could not be read.
	p := writeArtifact("unclassified.md", "---\nstatus: proposed\n---\n\n# U\n")
	if got, _ := statusExcluded(pairFor(p, "")); got {
		t.Error("an unclassified parent must not be excluded on status")
	}

	// Neither is an unreadable one — the LoadErr and bootstrap paths own that.
	if got, _ := statusExcluded(pairFor(filepath.Join(root, "does-not-exist.md"), "adr")); got {
		t.Error("an unreadable parent must not be excluded on status")
	}
}
