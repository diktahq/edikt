package govrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// #9 (c4-a/b/c/e), #7 (c3-e). `gov compile --check` / `--dry-run` rewrote
// sidecars: autorepair ran under CheckOnly and phasea wrote the result itself,
// re-marshalled (header dropped, signals re-sorted), and the run then printed
// `0 stale` — the drift it had just repaired on disk was invisible to the
// check that was asked to measure it. The rules these tests hold:
//
//   - under CheckOnly, nothing on disk moves (bytes, header, mtime);
//   - repairable drift is STALE: counted once, printed `stale:`, exit non-zero,
//     and listed separately as mechanically repairable;
//   - a full compile persists the repair through the guarded writer, keeping
//     the header, and the repaired sidecar is what Phase B compiles.

const autorepairHeader = "# yaml-language-server: $schema=../../../.edikt/schemas/gov-sidecar.v2.schema.json"

// driftedParent puts the rule at line 11; the sidecars below anchor it at 9.
func driftedParent(id, status string) string {
	return "---\nstatus: " + status + "\n---\n\n# " + id + " — drift\n\n## Decision\n\nContext line one.\nContext line two.\nDrifted rule MUST hold.\n"
}

func driftedSidecar(relPath, id string) string {
	return autorepairHeader + `
schema_version: 2
topic: "testing"
path: "` + relPath + `"
signals:
  - "zebra"
  - "alpha"
directives:
  - text: "Drifted rule MUST hold. (ref: ` + id + `)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Drifted rule MUST hold."
`
}

// partialSidecar: directive[0] is drifted but findable; directive[1]'s quote
// is gone from the prose, so the repair can only ever be partial.
func partialSidecar(relPath, id string) string {
	return autorepairHeader + `
schema_version: 2
topic: "testing"
path: "` + relPath + `"
signals:
  - "partial"
directives:
  - text: "Drifted rule MUST hold. (ref: ` + id + `)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Drifted rule MUST hold."
  - text: "Vanished rule MUST hold. (ref: ` + id + `)"
    source_excerpts:
      - line_start: 10
        line_end: 10
        quote: "Vanished rule MUST hold."
`
}

type driftFixture struct {
	root     string
	sidecars map[string]string // id -> absolute sidecar path
}

func writeDriftFixture(t *testing.T, partial ...string) driftFixture {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	invDir := filepath.Join(root, "docs", "architecture", "invariants")
	for _, d := range []string{adrDir, invDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	isPartial := map[string]bool{}
	for _, id := range partial {
		isPartial[id] = true
	}
	fx := driftFixture{root: root, sidecars: map[string]string{}}
	write := func(dir, rel, id, status string) {
		md := filepath.Join(dir, id+".md")
		if err := os.WriteFile(md, []byte(driftedParent(id, status)), 0o644); err != nil {
			t.Fatal(err)
		}
		body := driftedSidecar(rel+"/"+id+".md", id)
		if isPartial[id] {
			body = partialSidecar(rel+"/"+id+".md", id)
		}
		sc := filepath.Join(dir, id+".edikt.yaml")
		if err := os.WriteFile(sc, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		fx.sidecars[id] = sc
	}
	write(adrDir, "docs/architecture/decisions", "ADR-090", "accepted")
	write(invDir, "docs/architecture/invariants", "INV-090", "active")
	for _, id := range partial {
		if strings.HasPrefix(id, "ADR-") && id != "ADR-090" {
			write(adrDir, "docs/architecture/decisions", id, "accepted")
		}
	}
	stageExtractorAgent(t, root, "claude-sonnet-5")
	return fx
}

type fileState struct {
	bytes []byte
	mtime time.Time
}

func snapshot(t *testing.T, paths map[string]string) map[string]fileState {
	t.Helper()
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	out := map[string]fileState{}
	for id, p := range paths {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[id] = fileState{bytes: b, mtime: old}
	}
	return out
}

func assertUntouched(t *testing.T, paths map[string]string, before map[string]fileState) {
	t.Helper()
	for id, p := range paths {
		b, _ := os.ReadFile(p)
		if !bytes.Equal(b, before[id].bytes) {
			t.Errorf("%s: --check modified the sidecar:\n--- before\n%s--- after\n%s", id, before[id].bytes, b)
		}
		if !strings.HasPrefix(string(b), autorepairHeader+"\n") {
			t.Errorf("%s: the schema header is gone", id)
		}
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if !st.ModTime().Equal(before[id].mtime) {
			t.Errorf("%s: --check touched the mtime (%v -> %v)", id, before[id].mtime, st.ModTime())
		}
	}
}

func countOf(xs []string, x string) int {
	n := 0
	for _, v := range xs {
		if v == x {
			n++
		}
	}
	return n
}

func TestRunTwoPhase_CheckOnlyNeverPersistsAutorepair(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "1")
	fx := writeDriftFixture(t)
	before := snapshot(t, fx.sidecars)

	var errBuf, outBuf bytes.Buffer
	res, err := RunTwoPhase(TwoPhaseOptions{ProjectRoot: fx.root, CheckOnly: true, NoWait: true, Stderr: &errBuf, Stdout: &outBuf}, model.RealClock{})
	combined := outBuf.String() + errBuf.String()

	assertUntouched(t, fx.sidecars, before)
	if err == nil {
		t.Fatalf("--check over repairable drift must fail; it passed:\n%s", combined)
	}
	if res == nil {
		t.Fatalf("no result: %v", err)
	}
	for _, id := range []string{"ADR-090", "INV-090"} {
		if n := countOf(res.StaleSidecars, id); n != 1 {
			t.Errorf("%s must appear exactly once in StaleSidecars, got %d: %v", id, n, res.StaleSidecars)
		}
		if n := countOf(res.RepairableSidecars, id); n != 1 {
			t.Errorf("%s must appear exactly once in RepairableSidecars, got %d: %v", id, n, res.RepairableSidecars)
		}
		if !regexpStaleLine(combined, id) {
			t.Errorf("no `stale: %s` line for the review commands' grep:\n%s", id, combined)
		}
	}
	if !strings.Contains(combined, "error: 2 sidecar(s) stale — 2 mechanically repairable (run edikt gov compile), 0 need extraction (run /edikt:gov:compile in Claude Code)") {
		t.Errorf("the refusal must split repairable from extraction-needing drift:\n%s", combined)
	}
	if strings.Contains(combined, "0 stale") {
		t.Errorf("a --check that found drift must never report 0 stale:\n%s", combined)
	}
	// Nothing on disk changed, and nothing in memory leaked to the caller's
	// pairs either: a fresh load still sees the drift.
	sc, lerr := sidecar.Load(fx.sidecars["ADR-090"])
	if lerr != nil {
		t.Fatal(lerr)
	}
	if stale, _, _ := sc.IsStale(fx.root); !stale {
		t.Error("the on-disk sidecar must still be stale after --check")
	}
}

func regexpStaleLine(out, id string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "stale: "+id+" ") {
			return true
		}
	}
	return false
}

func TestRunTwoPhase_CheckOnlyStillStaleNotDoubleCounted(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "1")
	fx := writeDriftFixture(t, "ADR-091")
	before := snapshot(t, fx.sidecars)

	var errBuf bytes.Buffer
	res, err := RunTwoPhase(TwoPhaseOptions{ProjectRoot: fx.root, CheckOnly: true, NoWait: true, Stderr: &errBuf, Stdout: &errBuf}, model.RealClock{})
	assertUntouched(t, fx.sidecars, before)
	if err == nil || res == nil {
		t.Fatalf("--check over stale sidecars must fail with a result; err=%v\n%s", err, errBuf.String())
	}
	for _, id := range []string{"ADR-090", "ADR-091", "INV-090"} {
		if n := countOf(res.StaleSidecars, id); n != 1 {
			t.Errorf("%s must be counted exactly once as stale, got %d: %v", id, n, res.StaleSidecars)
		}
	}
	if n := countOf(res.RepairableSidecars, "ADR-091"); n != 0 {
		t.Errorf("a partially repairable sidecar is not mechanically repairable: %v", res.RepairableSidecars)
	}
	if len(res.RepairableSidecars) != 2 {
		t.Errorf("RepairableSidecars = %v, want ADR-090 and INV-090", res.RepairableSidecars)
	}
	if !strings.Contains(errBuf.String(), "error: 3 sidecar(s) stale — 2 mechanically repairable (run edikt gov compile), 1 need extraction (run /edikt:gov:compile in Claude Code)") {
		t.Errorf("the refusal must count the split exactly:\n%s", errBuf.String())
	}
}

func TestRunTwoPhase_PartialRepairReportsResidualAnchor(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "1")
	fx := writeDriftFixture(t, "ADR-091")
	var errBuf bytes.Buffer
	_, _ = RunTwoPhase(TwoPhaseOptions{ProjectRoot: fx.root, CheckOnly: true, NoWait: true, Stderr: &errBuf, Stdout: &errBuf}, model.RealClock{})
	out := errBuf.String()
	// directive[0] was repairable; what is still stale after the repair is
	// directive[1]. Naming directive[0] sends the reader to prose that the
	// repair has already accounted for.
	if !strings.Contains(out, "stale: ADR-091 — directive[1].anchor[0]: quote not found at lines 10-10") {
		t.Errorf("a partial repair must report the RESIDUAL anchor:\n%s", out)
	}
	if strings.Contains(out, "stale: ADR-091 — directive[0]") {
		t.Errorf("the repaired anchor was reported as the reason:\n%s", out)
	}
}

func TestRunTwoPhase_FullCompileAppliesAutorepairKeepingHeader(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	fx := writeDriftFixture(t)

	var errBuf, outBuf bytes.Buffer
	if _, err := RunTwoPhase(TwoPhaseOptions{ProjectRoot: fx.root, NoWait: true, Stderr: &errBuf, Stdout: &outBuf, OnLoss: "accept"}, model.RealClock{}); err != nil {
		t.Fatalf("full compile over repairable drift must succeed: %v\n%s%s", err, outBuf.String(), errBuf.String())
	}
	for id, p := range fx.sidecars {
		b, _ := os.ReadFile(p)
		if !strings.HasPrefix(string(b), autorepairHeader+"\n") {
			t.Errorf("%s: the persisted repair dropped the schema header:\n%s", id, b)
		}
		sc, err := sidecar.Load(p)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if a := sc.Directives[0].SourceExcerpts[0]; a.LineStart != 11 || a.LineEnd != 11 {
			t.Errorf("%s: repair not persisted, anchor at %d-%d", id, a.LineStart, a.LineEnd)
		}
	}
	// THE VALUE-COPY TRAP. A repair applied to a copy of the pair would be
	// written to disk while Phase B compiled the drifted sidecar. A --check
	// right after the compile compares the rendered tree against a fresh
	// render of the on-disk sidecars, so it passes only if Phase B compiled
	// what was persisted.
	errBuf.Reset()
	outBuf.Reset()
	if _, err := RunTwoPhase(TwoPhaseOptions{ProjectRoot: fx.root, CheckOnly: true, NoWait: true, Stderr: &errBuf, Stdout: &outBuf}, model.RealClock{}); err != nil {
		t.Fatalf("--check after the repairing compile must be clean: %v\n%s%s", err, outBuf.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "anchor drift: 0 stale (2 sidecar(s))") {
		t.Errorf("the clean check must report the computed count:\n%s", errBuf.String())
	}
}
