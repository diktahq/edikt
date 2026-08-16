package phasea

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// F-029 — the dispatch must write into the tree it was TOLD to write into.
//
// The prompt carries an artifact ID and nothing else, so the subagent resolves
// that ID against its own working directory. With cmd.Dir unset the child
// inherited the parent's cwd, so `gov reextract <scratch-root>` rewrote the
// LIVE corpus and then reported "wrote no sidecar" — the writer and the
// checker were looking at different trees.
//
// WHY BOTH HALVES. Asserting only "the sidecar appeared in the target tree"
// passes the bug: the live tree being clobbered is invisible to that check.
// The bug IS the second tree, so the second tree is what has to be asserted.

// minimalSidecar is the smallest shape that survives post-dispatch schema
// validation. The first draft of this test omitted `path:` and the runner
// rolled the write back — which is the anchor-verification guard working, and
// is why the test asserts on file CONTENT rather than on the dispatch merely
// returning nil.
const minimalSidecar = `schema_version: 2
topic: tooling
path: ADR-001-x.md
directives: []
`

// twoTrees builds a target root and a decoy root, each with a sidecar, and
// returns their paths. The decoy stands in for the live corpus.
func twoTrees(t *testing.T) (target, decoy string) {
	t.Helper()
	base := t.TempDir()
	for _, name := range []string{"target", "decoy"} {
		d := filepath.Join(base, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "ADR-001-x.edikt.yaml"),
			[]byte(minimalSidecar), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(base, "target"), filepath.Join(base, "decoy")
}

// TestResync_WritesIntoProjectRoot_NotTheInheritedCwd is the regression.
//
// The stub writes to a RELATIVE path, exactly as the real subagent does when
// it resolves an artifact ID against its own config. Where that relative path
// lands is decided entirely by the working directory, which is the thing
// under test.
func TestResync_WritesIntoProjectRoot_NotTheInheritedCwd(t *testing.T) {
	target, decoy := twoTrees(t)

	// Run the parent process from the DECOY, so an unset cmd.Dir sends the
	// write there — reproducing the field conditions rather than simulating
	// them.
	restore, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(decoy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(restore) })

	decoyBefore, err := os.ReadFile(filepath.Join(decoy, "ADR-001-x.edikt.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	task := Task{
		ArtifactType: "adr",
		ArtifactID:   "ADR-001",
		ParentPath:   filepath.Join(target, "ADR-001-x.md"),
		SidecarPath:  filepath.Join(target, "ADR-001-x.edikt.yaml"),
		ProjectRoot:  target,
	}
	r := &ClaudeRunner{Binary: stubClaude(t,
		`printf 'schema_version: 2\ntopic: tooling\npath: ADR-001-x.md\ndirectives: []\nsignals:\n  - stub marker\n' > ADR-001-x.edikt.yaml`)}

	if err := r.Resync(context.Background(), task); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// Half one — it landed where it was told to.
	got, err := os.ReadFile(task.SidecarPath)
	if err != nil {
		t.Fatalf("no sidecar in the TARGET tree: %v", err)
	}
	if !strings.Contains(string(got), "stub marker") {
		t.Errorf("target sidecar was not rewritten by the dispatch")
	}

	// Half two — and nowhere else. This is the assertion the bug would fail.
	decoyAfter, err := os.ReadFile(filepath.Join(decoy, "ADR-001-x.edikt.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoyAfter) != string(decoyBefore) {
		t.Fatalf("the dispatch wrote into a tree it was not targeting — "+
			"ProjectRoot was %q but the inherited cwd %q was modified", target, decoy)
	}
}

// TestResync_SensitivityOfTheDecoyCheck proves half two can actually fail.
// A control that has never gone red is not known to be able to.
func TestResync_SensitivityOfTheDecoyCheck(t *testing.T) {
	target, decoy := twoTrees(t)

	restore, _ := os.Getwd()
	if err := os.Chdir(decoy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(restore) })

	decoyBefore, _ := os.ReadFile(filepath.Join(decoy, "ADR-001-x.edikt.yaml"))

	// ProjectRoot deliberately EMPTY — the pre-fix behaviour.
	task := Task{
		ArtifactType: "adr",
		ArtifactID:   "ADR-001",
		ParentPath:   filepath.Join(target, "ADR-001-x.md"),
		SidecarPath:  filepath.Join(target, "ADR-001-x.edikt.yaml"),
	}
	r := &ClaudeRunner{Binary: stubClaude(t,
		`printf 'clobbered\n' > ADR-001-x.edikt.yaml`)}

	// The dispatch is EXPECTED to fail here: the target never receives a file.
	// That failure is the zero-file check doing its job — the same signal that
	// made the real defect visible instead of silent.
	if err := r.Resync(context.Background(), task); err == nil {
		t.Fatal("a dispatch that wrote to the wrong tree was reported as success")
	}

	decoyAfter, _ := os.ReadFile(filepath.Join(decoy, "ADR-001-x.edikt.yaml"))
	if string(decoyAfter) == string(decoyBefore) {
		t.Fatal("decoy unchanged with ProjectRoot empty — the test cannot " +
			"observe a cross-tree write, so its green in the sibling test proves nothing")
	}
}
