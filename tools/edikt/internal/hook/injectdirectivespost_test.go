package hook

// injectdirectivespost_test.go — discriminating coverage for the
// Write/Edit/MultiEdit/NotebookEdit tool-name filter, which
// test/fixtures/hook-corpus/inject-directives-post/matrix.yaml's
// "non-write-tool" scenario NAMES but cannot PROVE (INV-013): the shared
// corpus sandbox carries no .claude/rules/governance/directive-index.yaml
// (see that matrix's own header comment on why the match-found branch is not
// exercised there), so a governed Write ALSO produces the same quiet
// `{"continue": true}` reply as an ungoverned Read — replacing the filter's
// switch with an always-proceed pass-through leaves the recorded corpus at
// 5/5 PASS. Proving the filter requires a real directive index and a path it
// actually matches, which only a Go test with its own sandbox can seed (the
// same gap test/unit/hooks/test_inject_directives_post.sh's own "match
// found" coverage already fills for the bash original — this file is the
// direct Go-side port of that same proof, scoped to the tool-name filter).
//
// PLAN-v080-reliability-core Phase 13.

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// TestInjectDirectivesPostOnlyFiresForWriteTools proves the tool-name filter
// actually changes the reply, not just that a case arm is taken. A single
// advisory directive matches "docs/**/*.md"; a Write payload against a
// matching path must return non-empty additionalContext carrying that
// directive's text, and the SAME payload with tool_name=Read (the negative
// control — same path, same directive index, only the tool differs) must
// return the quiet `{"continue": true}` reply with no context at all.
func TestInjectDirectivesPostOnlyFiresForWriteTools(t *testing.T) {
	if !commandAvailable("python3") {
		t.Skip("python3 not on PATH — injectDirectivesPost's own preflight would short-circuit before the filter runs")
	}

	proj := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(proj)

	idxDir := filepath.Join(proj, ".claude", "rules", "governance")
	if err := os.MkdirAll(idxDir, 0o755); err != nil {
		t.Fatalf("mkdir directive-index dir: %v", err)
	}
	index := `schema_version: 1
globs:
  "docs/**/*.md":
    - id: "ADR-901:d01"
      grade: advisory
      text: "Prefer short, imperative commit-style summaries in doc headers."
      intent: "Keep doc headers scannable."
      reminders:
        - "Check the header fits on one line"
`
	if err := os.WriteFile(filepath.Join(idxDir, "directive-index.yaml"), []byte(index), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write directive-index.yaml: %v", err)
	}

	run := func(t *testing.T, toolName string) adapter.Response {
		t.Helper()
		inv := &Invocation{
			Payload: adapter.Payload{
				ToolName:  toolName,
				FilePath:  "docs/notes.md",
				SessionID: "sess-" + toolName,
			},
			Stderr: io.Discard,
		}
		res := injectDirectivesPost(inv)
		return res.Response
	}

	write := run(t, "Write")
	if write.Context == "" {
		t.Fatal("Write against a governed path produced no additionalContext")
	}
	if !containsSubstring(write.Context, "Prefer short, imperative commit-style summaries") {
		t.Errorf("additionalContext = %q, want it to carry the matched directive's text", write.Context)
	}

	read := run(t, "Read")
	if read.Context != "" {
		t.Errorf("Read against the SAME governed path produced additionalContext (%q), want none — the tool-name filter should have short-circuited before matching", read.Context)
	}

	if write.Context == read.Context {
		t.Fatalf("Write and Read produced identical output (%q) — the tool-name filter has no observable effect", write.Context)
	}
}
