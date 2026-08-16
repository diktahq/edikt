package govrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// scopeRunner writes a sidecar whose `paths:` globs are chosen per artifact,
// standing in for the extractor exactly as bootstrapRunner does. Artifacts
// absent from globs get NO paths key at all — an undeclared sidecar, which
// is the case the scope fallback exists for.
type scopeRunner struct {
	topics map[string]string   // artifact ID -> topic
	globs  map[string][]string // artifact ID -> declared code globs
}

func (r *scopeRunner) Preflight() error { return nil }

func (r *scopeRunner) Resync(_ context.Context, t phasea.Task) error {
	var b strings.Builder
	fmt.Fprintf(&b, "schema_version: 1\ntopic: %q\n", r.topics[t.ArtifactID])
	fmt.Fprintf(&b, "path: \"docs/architecture/decisions/%s\"\n", filepath.Base(t.ParentPath))
	if g := r.globs[t.ArtifactID]; len(g) > 0 {
		b.WriteString("paths:\n")
		for _, p := range g {
			fmt.Fprintf(&b, "  - %q\n", p)
		}
	}
	// Signals are validated against ^[a-z0-9][a-z0-9 _.-]*$ — lower-case.
	fmt.Fprintf(&b, "signals:\n  - %q\n", strings.ToLower(t.ArtifactID)+" signal")
	fmt.Fprintf(&b, "directives:\n  - text: \"Test rule MUST hold. (ref: %s)\"\n", t.ArtifactID)
	b.WriteString("    source_excerpt:\n      line_start: 9\n      line_end: 9\n      quote: \"Test rule.\"\n")
	return os.WriteFile(t.SidecarPath, []byte(b.String()), 0o644)
}

// stageScopeCorpus writes three ADRs across two topics:
//
//	testing  — ADR-001 declares globs, ADR-002 declares none  (partial)
//	security — ADR-003 declares globs                          (complete)
func stageScopeCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, id := range []string{"001", "002", "003"} {
		// Line 9 is "Test rule." — the quote the fake runner anchors to.
		body := fmt.Sprintf("---\nstatus: accepted\n---\n\n# ADR-%s — Test\n\n## Decision\n\nTest rule.\n", id)
		p := filepath.Join(adrDir, fmt.Sprintf("ADR-%s-test.md", id))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write adr: %v", err)
		}
	}
	return root
}

func runScopeCompile(t *testing.T) (root string, out string) {
	root, out, _ = runScopeCompileFull(t)
	return root, out
}

func runScopeCompileFull(t *testing.T) (root string, out string, res *TwoPhaseResult) {
	t.Helper()
	t.Setenv("EDIKT_HEADLESS", "")

	root = stageScopeCorpus(t)
	runner := &scopeRunner{
		topics: map[string]string{"ADR-001": "testing", "ADR-002": "testing", "ADR-003": "security"},
		globs: map[string][]string{
			"ADR-001": {"src/**/*.go"},
			"ADR-003": {"auth/**", "src/session.go"},
		},
	}
	var errBuf, outBuf bytes.Buffer
	r, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      runner,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	if err != nil {
		t.Fatalf("compile failed: %v\nstderr:\n%s", err, errBuf.String())
	}
	return root, outBuf.String() + errBuf.String(), r
}

// readSkill reads the tier-3 surface a retired topic lands on.
func readSkill(t *testing.T, root, topic string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "edikt-"+topic, "SKILL.md"))
	if err != nil {
		t.Fatalf("skill package for %s not written: %v", topic, err)
	}
	return string(b)
}

// noTopicFile asserts a topic has NO tier-2 rules file.
func noTopicFile(t *testing.T, root, topic string) {
	t.Helper()
	p := filepath.Join(root, ".claude", "rules", "governance", topic+".md")
	if _, err := os.Stat(p); err == nil {
		b, _ := os.ReadFile(p)
		t.Fatalf("topic %s still has a tier-2 rules file after retirement:\n%s", topic, frontmatterOf(string(b)))
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", p, err)
	}
}

func readTopic(t *testing.T, root, topic string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", topic+".md"))
	if err != nil {
		t.Fatalf("topic %s not written: %v", topic, err)
	}
	return string(b)
}

// TestTopicPaths_CarryDeclaredGlobsNotTheADRPath is the core A2 regression.
//
// merge.go built the compiled topic file's `paths:` frontmatter from
// Sidecar.Path — the ADR document's own location — so governance/*.md
// declared `docs/architecture/decisions/ADR-….md` where code globs belong.
// The legacy path never had this bug (compile.go:137 unions
// doc.Sentinel.Paths); the sidecar migration regressed it.
func TestTopicPaths_CarryDeclaredGlobsNotTheADRPath(t *testing.T) {
	root, _ := runScopeCompile(t)

	sec := readTopic(t, root, "security")
	for _, want := range []string{`- "auth/**"`, `- "src/session.go"`} {
		if !strings.Contains(sec, want) {
			t.Errorf("security.md missing declared glob %s:\n%s", want, frontmatterOf(sec))
		}
	}
	if strings.Contains(sec, "docs/architecture/decisions/") {
		t.Errorf("security.md scopes itself to the ADR document path:\n%s", frontmatterOf(sec))
	}

	// The partially-declared topic has no tier-2 file to carry a wrong path
	// at all now — it retires to tier 3 (SPEC-011 stage 1). The ADR-path
	// regression this test exists for is asserted on the surface that still
	// declares globs; asserting it on a file that cannot exist would be a
	// check that passes because its subject is absent (INV-013).
	noTopicFile(t, root, "testing")
}

// TestTopicPaths_UndeclaredSidecarRetiresTopicToTier3 pins the union rule
// and the tier consequence SPEC-011 stage 1 attached to it.
//
// An absent `paths:` still declares NO RESTRICTION, not NO PATHS, and the
// topic's scope is still the union of its sidecars' scopes — scoping
// `testing` to ADR-001's globs would silently narrow ADR-002, which never
// declared a scope, to another ADR's territory. The union is unchanged.
//
// What changed is what an unrestricted union COSTS. It used to render a
// tier-2 rules file globbed at `**/*`, which the host loads on every edit:
// measured at ~44k of the ~46.6k ambient tokens on the dogfood corpus. Such a
// topic now retires to tier 3 and is reached by skill invocation instead.
//
// Asserted from the published TopicsRetiredToSkill as well as from disk: a
// file being absent is also what a compile that never ran looks like, and the
// two must not be indistinguishable (INV-013/INV-014).
func TestTopicPaths_UndeclaredSidecarRetiresTopicToTier3(t *testing.T) {
	root, _, res := runScopeCompileFull(t)

	noTopicFile(t, root, "testing")

	var retired []string
	if res.PhaseB != nil {
		retired = res.PhaseB.TopicsRetiredToSkill
	}
	found := false
	for _, n := range retired {
		if n == "testing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unscoped topic not published as retired to tier 3; got %v", retired)
	}

	// The scoped topic must NOT be retired — otherwise "retire everything"
	// would pass this test while destroying tier 2 entirely.
	for _, n := range retired {
		if n == "security" {
			t.Fatalf("scoped topic wrongly retired to tier 3; got %v", retired)
		}
	}
	if sec := readTopic(t, root, "security"); !strings.Contains(sec, `- "auth/**"`) {
		t.Fatalf("scoped topic lost its tier-2 file:\n%s", frontmatterOf(sec))
	}

	// The guidance must still REACH the reader on its new surface. A
	// retirement that dropped the directives would show up here and nowhere
	// else — the token count would look even better.
	if sk := readSkill(t, root, "testing"); !strings.Contains(sk, "Test rule MUST hold.") {
		t.Fatalf("retired topic's directives absent from its skill package:\n%s", sk)
	}
}

// TestTopicRetirement_NamesWhyAndWhich applies the A1 lesson: "retired
// because undeclared" must not be indistinguishable from "lost", the same way
// unmeasured must not look like zero.
//
// It used to be asserted inside the unscoped topic FILE. There is no such
// file now, so the naming moves to the compile report — which is the better
// place for it anyway: a reader learns a topic left tier 2 at the moment it
// happens, rather than by opening a file to find a warning in it.
//
// The naming IS the clearing condition: declare globs on the named sidecar
// and the topic returns to tier 2 automatically.
func TestTopicRetirement_NamesWhyAndWhich(t *testing.T) {
	root, out, _ := runScopeCompileFull(t)
	_ = root

	if !strings.Contains(out, "Tier 3") {
		t.Errorf("compile does not report the tier-3 retirement:\n%s", out)
	}
	if !strings.Contains(out, "testing") {
		t.Errorf("retirement report does not name WHICH topic retired:\n%s", out)
	}
	if !strings.Contains(out, "ADR-002") {
		t.Errorf("report does not name the sidecar that failed to declare — "+
			"without it the retirement has no stated clearing condition:\n%s", out)
	}
	// A fully-declared topic must not be reported as retired.
	if strings.Contains(out, "security") && strings.Contains(out, "retired from .claude/rules/ to skill packages: security") {
		t.Errorf("fully-declared topic wrongly reported as retired:\n%s", out)
	}
}

// TestScopeCoverage_IsReported pins the fraction. Without it, topics sit
// unscoped permanently and nobody learns it is fixable. The count has to
// move with the corpus rather than being dropped once it looks bad
// (INV-013).
func TestScopeCoverage_IsReported(t *testing.T) {
	_, out := runScopeCompile(t)

	const want = "1 of 2 topic(s) scoped to tier 2"
	if !strings.Contains(out, want) {
		t.Fatalf("compile did not report scope coverage.\nwant substring: %s\ngot:\n%s", want, out)
	}
	if !strings.Contains(out, "ADR-002") {
		t.Errorf("scope coverage line does not name the undeclared sidecar:\n%s", out)
	}
}

// TestRenderedTopics_CarryNoCatchAllGlob is the re-pointed form of the old
// TestUnscopedGlob_MatchesRepoRootFilesUnderTier1Matcher.
//
// That test pinned the SPELLING of the unrestricted glob — `**` rather than
// `**/*`, because commands/gov/verify-diff.md matches with Python's fnmatch
// where `**/*` requires a slash and would miss every repo-root file. The glob
// it pinned no longer exists: an unrestricted topic retires to tier 3 instead
// of rendering with a catch-all.
//
// Deleting the test would drop a real guard, because the underlying hazard is
// the same one in reverse. A catch-all glob on a tier-2 file is exactly what
// puts scoped governance into the ambient budget — the ~44k-token leak stage 1
// exists to close — and it would reappear silently the first time some future
// fallback re-emits one. So the assertion inverts: NO rendered topic file may
// carry a glob that matches everything.
//
// It still runs the REAL matcher rather than reimplementing fnmatch (INV-014):
// a Go copy of fnmatch's semantics would agree with itself while diverging
// from the consumer that actually reads these globs.
func TestRenderedTopics_CarryNoCatchAllGlob(t *testing.T) {
	root, _, _ := runScopeCompileFull(t)

	dir := filepath.Join(root, ".claude", "rules", "governance")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read governance dir: %v", err)
	}

	// PIN THE SUBJECT. A zero-file directory would satisfy every assertion
	// below while proving nothing — the empty-result class. This test is only
	// meaningful if at least one topic file was rendered.
	checked := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		body, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		glob := extractFirstGlob(t, string(body))
		checked++

		// Probe with a file no legitimate scoped topic should claim. If the
		// glob matches it, the glob is a catch-all under the real matcher
		// whatever its spelling.
		const unrelated = "CHANGELOG.md"
		out, xerr := exec.Command("python3", "-c",
			"import fnmatch,sys; print('yes' if fnmatch.fnmatch(sys.argv[2], sys.argv[1]) else 'no')",
			glob, unrelated).Output()
		if xerr != nil {
			t.Skipf("python3 unavailable for tier-1 matcher check: %v", xerr)
		}
		if strings.TrimSpace(string(out)) == "yes" {
			t.Errorf("rendered topic %s carries catch-all glob %q — it matches %q "+
				"under the tier-1 matcher and would load on every edit, which is "+
				"the ambient leak retirement to tier 3 exists to close",
				e.Name(), glob, unrelated)
		}
	}
	if checked == 0 {
		t.Fatal("no rendered topic files to check — the assertion had no subject, " +
			"which is not the same as passing")
	}
}
func extractFirstGlob(t *testing.T, body string) string {
	t.Helper()
	inPaths := false
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimRight(line, " ") == "paths:" {
			inPaths = true
			continue
		}
		if inPaths {
			s := strings.TrimSpace(line)
			if strings.HasPrefix(s, "- ") {
				return strings.Trim(strings.TrimPrefix(s, "- "), `"`)
			}
			break
		}
	}
	t.Fatalf("no paths: entry found in compiled topic:\n%s", frontmatterOf(body))
	return ""
}

// frontmatterOf returns the leading YAML frontmatter plus the comment block
// of a compiled topic file, so failures show the scope decision rather than
// the whole rules file.
func frontmatterOf(body string) string {
	lines := strings.Split(body, "\n")
	end := len(lines)
	if end > 24 {
		end = 24
	}
	return strings.Join(lines[:end], "\n")
}
