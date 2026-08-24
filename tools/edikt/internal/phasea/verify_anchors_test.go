package phasea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVerifyAnchorsAgainstParent exercises the ADR-056 rung directly, at its
// new home in verify_anchors.go.
//
// The function's existing tests — parsegate_test.go's TestResync_* set — go
// through ClaudeRunner.Resync and still pass unmodified, because the move was
// within the package. But ClaudeRunner is deleted in Phase 4 and those tests
// go with it, while this check does not: `phasea verify` grades an item with
// it and `phasea restore` decides a quarantine on it, neither of which
// involves a dispatch. This test covers the same four behaviours without the
// dispatch scaffolding, so the coverage survives that deletion.
func TestVerifyAnchorsAgainstParent(t *testing.T) {
	// parentBody's line 12 is "Both interfaces call the same code." — the
	// line validSidecar and d20InvalidSidecar both anchor at.
	setup := func(t *testing.T, sidecarBody string) (string, string) {
		t.Helper()
		dir := t.TempDir()
		parent := filepath.Join(dir, "GL-003.md")
		if err := os.WriteFile(parent, []byte(parentBody), 0o644); err != nil {
			t.Fatal(err)
		}
		sc := filepath.Join(dir, "GL-003.edikt.yaml")
		if err := os.WriteFile(sc, []byte(sidecarBody), 0o644); err != nil {
			t.Fatal(err)
		}
		return sc, parent
	}

	t.Run("an anchor that quotes the parent byte-exactly passes", func(t *testing.T) {
		sc, parent := setup(t, validSidecar)
		if err := verifyAnchorsAgainstParent(sc, parent); err != nil {
			t.Fatalf("a correctly anchored sidecar was rejected: %v", err)
		}
	})

	t.Run("an anchor pointing at prose that is not there fails and names the fault", func(t *testing.T) {
		bad := strings.Replace(validSidecar,
			`quote: "Both interfaces call the same code."`,
			`quote: "Both interfaces call entirely different code."`, 1)
		if bad == validSidecar {
			t.Fatal("fixture: the quote substitution did not apply, so nothing is under test")
		}
		sc, parent := setup(t, bad)
		err := verifyAnchorsAgainstParent(sc, parent)
		if err == nil {
			t.Fatal("an anchor quoting prose that is not in the parent was accepted — a warning here " +
				"would be the absence-rendering-as-pass shape INV-013 forbids")
		}
		// An anchor error is invisible from the quote alone; the message has
		// to show what actually sits at the recorded range.
		if !strings.Contains(err.Error(), "invalid anchor") {
			t.Errorf("message does not name the anchor fault: %v", err)
		}
		if !strings.Contains(err.Error(), "GL-003.md") {
			t.Errorf("message does not name the parent it was checked against: %v", err)
		}
	})

	t.Run("no items is a measured zero, not a skipped check", func(t *testing.T) {
		// A roadmap-only ADR legitimately compiles to `directives: []`.
		// Reading the parent here would only manufacture an error for a
		// sidecar that has no anchors to be wrong about — so the parent is
		// deliberately absent, proving it is never read on this path.
		dir := t.TempDir()
		sc := filepath.Join(dir, "GL-003.edikt.yaml")
		body := "schema_version: 1\ntopic: interfaces\npath: docs/guidelines/GL-003-dual-mcp-rest-interface.md\nsignals:\n  - mcp server\ndirectives: []\n"
		if err := os.WriteFile(sc, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := verifyAnchorsAgainstParent(sc, filepath.Join(dir, "does-not-exist.md")); err != nil {
			t.Fatalf("a sidecar with no anchors was rejected: %v", err)
		}
	})

	t.Run("anchors present but the parent unreadable is an error, not a pass", func(t *testing.T) {
		dir := t.TempDir()
		sc := filepath.Join(dir, "GL-003.edikt.yaml")
		if err := os.WriteFile(sc, []byte(validSidecar), 0o644); err != nil {
			t.Fatal(err)
		}
		err := verifyAnchorsAgainstParent(sc, filepath.Join(dir, "does-not-exist.md"))
		if err == nil {
			t.Fatal("the parent is the oracle — there ARE anchors here and none could be checked; " +
				"unmeasured must not read as a clean bill of health (INV-011)")
		}
		if !strings.Contains(err.Error(), "cannot read parent") {
			t.Errorf("message does not name the unreadable parent: %v", err)
		}
	})
}
