package govrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// skipRunner stands in for the claude CLI: it writes a minimal valid
// sidecar for whatever artifact it is handed — the same contract as the
// per-artifact `:compile` command in production. Self-contained so this
// file has no dependency on other test helpers.
type skipRunner struct {
	dispatched []string
}

func (r *skipRunner) Preflight() error { return nil }

func (r *skipRunner) Resync(_ context.Context, t phasea.Task) error {
	r.dispatched = append(r.dispatched, t.ArtifactID)
	body := fmt.Sprintf(`schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/%s"
signals:
  - "bootstrap signal"
directives:
  - text: "Test rule MUST hold. (ref: %s)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Test rule."
`, filepath.Base(t.ParentPath), t.ArtifactID)
	return os.WriteFile(t.SidecarPath, []byte(body), 0o644)
}

// Field bug (bok-services 2026-08-07): after retiring ADRs via frontmatter
// `status: superseded`, Phase A re-bootstrapped their sidecars from prose and
// Phase B compiled their directives — duplicating rules reclassified into
// guidelines. The whole pipeline must treat skip-listed artifacts as inert:
// no dispatch, no merge, and an existing (even hand-written or legacy)
// sidecar next to them must be tolerated, not compiled and not fatal.
func TestRunTwoPhase_SupersededArtifactIsFullyExcluded(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Live ADR with a stale sidecar — will be re-extracted by the fake runner.
	live := "---\nstatus: accepted\n---\n\n# ADR-001 — Test\n\n## Decision\n\nTest rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(live), 0o644); err != nil {
		t.Fatal(err)
	}
	staleLive := `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-001-test.md"
signals: []
directives:
  - text: "Old anchor. (ref: ADR-001)"
    source_excerpts:
      - line_start: 2
        line_end: 2
        quote: "This text is long gone from the parent body."
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.edikt.yaml"), []byte(staleLive), 0o644); err != nil {
		t.Fatal(err)
	}

	// Retired ADR (frontmatter-only supersession) WITH a leftover sidecar
	// carrying a directive that must NOT reach compiled governance.
	retired := "---\nstatus: superseded\nsuperseded_by: GL-004\n---\n\n# ADR-002 — Retired\n\n## Decision\n\nRetired rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-002-retired.md"), []byte(retired), 0o644); err != nil {
		t.Fatal(err)
	}
	leftover := `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-002-retired.md"
signals:
  - "retired signal"
directives:
  - text: "RETIRED-MARKER rule MUST hold. (ref: ADR-002)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Retired rule."
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-002-retired.edikt.yaml"), []byte(leftover), 0o644); err != nil {
		t.Fatal(err)
	}

	// A second retired ADR with a hand-written EMPTY sidecar (the field
	// workaround) — must be tolerated silently.
	retired2 := "---\nstatus: superseded\nsuperseded_by: GL-005\n---\n\n# ADR-003 — Retired too\n\n## Decision\n\nAlso retired.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-003-retired.md"), []byte(retired2), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-003-retired.md"
signals: []
directives: []
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-003-retired.edikt.yaml"), []byte(empty), 0o644); err != nil {
		t.Fatal(err)
	}

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

	// Only the live ADR is dispatched.
	for _, id := range runner.dispatched {
		if id != "ADR-001" {
			t.Errorf("skip-listed artifact dispatched to extractor: %s", id)
		}
	}
	if len(runner.dispatched) != 1 {
		t.Errorf("expected exactly 1 dispatch (ADR-001), got %v", runner.dispatched)
	}
	if res.PhaseB == nil {
		t.Fatal("Phase B did not run")
	}

	// The skip must be announced (existing sidecar ignored).
	if !strings.Contains(errBuf.String(), "skip: ADR-002") {
		t.Errorf("expected skip announcement for ADR-002, stderr:\n%s", errBuf.String())
	}

	// Compiled governance must not carry the retired directive.
	//
	// The scan covers EVERY rendered surface, not just `.claude/rules/`. Since
	// SPEC-011 stage 1 an unscoped topic renders to a skill package instead of
	// a rules file, so a rules-only scan would miss it — and that cuts both
	// ways: the positive assertion would fail on a live directive that merely
	// moved tier, and the NEGATIVE assertion would silently stop looking where
	// a leaked retired directive is now most likely to land. A scan of a path
	// you did not verify is traversable is not a scan (GL-002).
	var compiled strings.Builder
	for _, dir := range []string{
		filepath.Join(root, ".claude", "rules", "governance"),
		filepath.Join(root, ".claude", "skills"),
	} {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil //nolint:nilerr // absent surface is not an error here
			}
			b, rerr := os.ReadFile(p)
			if rerr == nil {
				compiled.Write(b)
			}
			return nil
		})
	}
	if idx, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md")); err == nil {
		compiled.Write(idx)
	}
	// PIN THE SUBJECT: an empty scan would satisfy the negative assertion
	// while proving nothing.
	if compiled.Len() == 0 {
		t.Fatal("no compiled surfaces found to scan — the assertions below had no subject")
	}
	if strings.Contains(compiled.String(), "RETIRED-MARKER") {
		t.Error("retired ADR-002 directive leaked into compiled governance")
	}
	if !strings.Contains(compiled.String(), "Test rule MUST hold") {
		t.Error("live ADR-001 directive missing from compiled governance")
	}
}

// Lock contention must be announced, not silently blocked on — a blocking
// wait with no output was indistinguishable from a hung compile.
func TestAcquireCompileLock_AnnouncesContention(t *testing.T) {
	root := t.TempDir()

	release1, err := acquireCompileLock(root, false, os.Stderr)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// --no-wait contention fails fast with an actionable error.
	if _, err := acquireCompileLock(root, true, os.Stderr); err == nil {
		t.Fatal("second acquire with --no-wait must fail while lock held")
	}

	// Blocking acquire announces the wait, then succeeds once released.
	var msg bytes.Buffer
	acquired := make(chan func(), 1)
	go func() {
		r2, err := acquireCompileLock(root, false, &msg)
		if err != nil {
			t.Errorf("blocking acquire: %v", err)
			acquired <- func() {}
			return
		}
		acquired <- r2
	}()

	// Wait until the goroutine has printed the contention notice.
	deadline := time.Now().Add(5 * time.Second)
	for msg.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(msg.String(), "compile.lock is held") {
		t.Errorf("blocking acquire must announce contention, got %q", msg.String())
	}

	release1()
	release2 := <-acquired
	release2()
}
