package phasea

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The exact bytes GL-003's extraction produced: a backtick escaped inside a
// double-quoted YAML scalar. Backtick is not a YAML escape — only \" and \\
// are — so this is a hard parse error and the sidecar cannot be loaded.
// Copied from the failing artifact rather than invented, because a fixture
// that merely "looks broken" would not prove the gate catches the shape
// production actually emits.
const d20InvalidSidecar = "schema_version: 1\n" +
	"topic: interfaces\n" +
	"path: docs/guidelines/GL-003-dual-mcp-rest-interface.md\n" +
	"signals:\n" +
	"  - mcp server\n" +
	"directives:\n" +
	// ONE backslash before each backtick. Two would be a legal escaped
	// backslash and the fixture would parse, silently testing nothing —
	// which is what the first version of this file did.
	"  - text: \"Both interfaces MUST call the same \\`internal/rag\\` code. (ref: GL-003)\"\n" +
	"    source_excerpt:\n" +
	"      line_start: 12\n" +
	"      line_end: 12\n" +
	"      quote: \"Both interfaces call the same code.\"\n"

const validSidecar = `schema_version: 1
topic: interfaces
path: docs/guidelines/GL-003-dual-mcp-rest-interface.md
signals:
  - mcp server
directives:
  - text: "Both interfaces MUST call the same internal/rag code. (ref: GL-003)"
    source_excerpt:
      line_start: 12
      line_end: 12
      quote: "Both interfaces call the same code."
`

// writerRunner stands in for the claude CLI by writing fixed bytes to the
// sidecar path, which is exactly what the real dispatch's agent does. The
// stub exits 0, so the ONLY thing standing between bad output and a landed
// file is the parse gate.
func writerRunner(t *testing.T, payload string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncat > \"$EDIKT_TEST_SIDECAR\" <<'SIDECAR_EOF'\n" + payload + "SIDECAR_EOF\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return bin
}

// parentBody is the prose the fixtures' anchors quote. Line 12 carries the
// quoted sentence, matching the fixtures' recorded line_start/line_end.
//
// It exists because the ADR-056 anchor gate resolves anchors against the real
// parent: a fixture with an anchor and no parent file is a dispatch nothing
// could verify, which the gate correctly refuses. Writing the parent makes
// these tests exercise the gate rather than trip over its unmeasured path.
const parentBody = `# GL-003 — Dual interface

## Rule

Line four.
Line five.
Line six.
Line seven.
Line eight.
Line nine.
Line ten.
Both interfaces call the same code.
`

func runResyncGate(t *testing.T, bin, sidecarPath string) error {
	t.Helper()
	parent := filepath.Join(filepath.Dir(sidecarPath), "GL-003.md")
	if err := os.WriteFile(parent, []byte(parentBody), 0o644); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}
	t.Setenv("EDIKT_TEST_SIDECAR", sidecarPath)
	r := &ClaudeRunner{Binary: bin}
	return r.Resync(context.Background(), Task{
		ArtifactType: "guideline",
		ArtifactID:   "GL-003",
		ParentPath:   filepath.Join(filepath.Dir(sidecarPath), "GL-003.md"),
		SidecarPath:  sidecarPath,
	})
}

// TestResync_UnloadableSidecarIsRejectedAndRolledBack is D20 as an
// executable claim. Before the gate, a dispatch that wrote unparseable YAML
// returned nil — the file existed and had changed, which is all the
// stat-based checks could see — and the corrupt sidecar then failed compile
// for the whole project.
func TestResync_UnloadableSidecarIsRejectedAndRolledBack(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "GL-003.edikt.yaml")
	if err := os.WriteFile(sc, []byte(validSidecar), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := runResyncGate(t, writerRunner(t, d20InvalidSidecar), sc)
	if err == nil {
		t.Fatal("Resync returned nil for a sidecar that cannot be parsed — " +
			"a file that exists and changed is not a sidecar that loads")
	}
	if !strings.Contains(err.Error(), "unloadable sidecar") {
		t.Errorf("error should name the parse failure; got: %v", err)
	}

	// The tree must be byte-identical to before the dispatch: the agent
	// owns the Write, so the gate's job is to leave nothing behind.
	got, readErr := os.ReadFile(sc)
	if readErr != nil {
		t.Fatalf("sidecar missing after rollback: %v", readErr)
	}
	if string(got) != validSidecar {
		t.Errorf("prior sidecar was not restored.\n got: %q", string(got))
	}
}

// A first extraction has no prior content, so rollback means REMOVAL.
// Leaving a corrupt file behind would be worse than the bootstrap gap it
// was meant to fill: compile would fail on it every run afterwards.
func TestResync_UnloadableBootstrapSidecarIsRemoved(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "GL-003.edikt.yaml")

	err := runResyncGate(t, writerRunner(t, d20InvalidSidecar), sc)
	if err == nil {
		t.Fatal("Resync returned nil for an unparseable bootstrap sidecar")
	}
	if _, statErr := os.Stat(sc); !os.IsNotExist(statErr) {
		t.Errorf("corrupt bootstrap sidecar was left on disk at %s", sc)
	}
}

// Isolation: the gate must not reject good output. Without this, "rejects
// everything" would pass the two tests above and break every extraction.
func TestResync_ValidSidecarIsAccepted(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "GL-003.edikt.yaml")

	if err := runResyncGate(t, writerRunner(t, validSidecar), sc); err != nil {
		t.Fatalf("Resync rejected a valid sidecar: %v", err)
	}
	got, readErr := os.ReadFile(sc)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(got) != validSidecar {
		t.Errorf("valid sidecar was not left in place")
	}
}

// TestResync_RejectsBadAnchorSidecar is the ADR-056 gate's SENSITIVITY case at
// the dispatch boundary.
//
// The sidecar below parses as YAML and conforms to the schema. Every rung the
// gate had before this one passes it. Its single anchor quotes real prose from
// the parent — only the recorded line number is wrong, which is precisely the
// failure three extractor-prompt revisions could not drive to zero
// (1/203 → 5/200 → 1/184 anchors).
//
// Without this case, extending the acceptance chain could be a no-op and every
// other test in this file would still pass.
func TestResync_RejectsBadAnchorSidecar(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "GL-003.edikt.yaml")

	// Same content as validSidecar, with the anchor moved one line off.
	badAnchor := strings.Replace(validSidecar, "line_start: 12", "line_start: 11", 1)
	badAnchor = strings.Replace(badAnchor, "line_end: 12", "line_end: 11", 1)

	err := runResyncGate(t, writerRunner(t, badAnchor), sc)
	if err == nil {
		t.Fatal("dispatch producing a schema-valid sidecar with a mis-anchored directive was accepted — " +
			"the generation boundary is not verifying anchors against the parent")
	}
	// The message has to name the anchor AND what is actually at those lines:
	// the quote is correct prose, so a reader cannot diagnose this from the
	// quote alone.
	for _, want := range []string{"source_excerpts[0]", "lines 11-11", "actual at those lines"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("failure does not name %q:\n%v", want, err)
		}
	}
	if _, statErr := os.Stat(sc); statErr == nil {
		t.Fatal("mis-anchored sidecar survived on disk — it was not rolled back")
	}
}

// TestResync_AcceptsSidecarWithNoItems — a roadmap-only artifact legitimately
// compiles to `directives: []`. It has no anchors to be wrong about, so the
// gate must accept it WITHOUT reading the parent.
//
// This is the zero-input path (INV-013), and getting it wrong in the other
// direction would be worse than the bug being fixed: every such artifact would
// fail its dispatch for a missing oracle it never needed.
func TestResync_AcceptsSidecarWithNoItems(t *testing.T) {
	dir := t.TempDir()
	sc := filepath.Join(dir, "GL-003.edikt.yaml")
	empty := "schema_version: 1\ntopic: interfaces\npath: docs/guidelines/GL-003.md\nsignals:\n  - mcp server\ndirectives: []\n"

	if err := runResyncGate(t, writerRunner(t, empty), sc); err != nil {
		t.Fatalf("a sidecar with zero items was rejected: %v", err)
	}
}
