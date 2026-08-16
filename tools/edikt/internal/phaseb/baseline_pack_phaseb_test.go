package phaseb

// baseline_pack_phaseb_test.go — SPEC-009 Plan C Phase 5.
//
// Pins the contract that every sidecar shipped under
// templates/sidecars/baseline/ satisfies the Phase B compile-time
// constraints from ADR-036 §2 (verify_kind required when verify is set;
// falsifying_observation + human_approved_at required for behavioral
// kind) AND the SPEC-009 Plan C Phase 4 bidirectional-fixture gate
// (positive_fixture_path + negative_fixture_path required for behavioral
// directives).
//
// Strategy: load each baseline sidecar through the strict loader, wrap
// it in a synthetic Pair with a placeholder parent .md inside a fresh
// tempdir, then run Merge. Merge calls validatePhaseBConstraints before
// any rendering happens, so a Phase B violation surfaces as a Merge
// error here without us needing to call the validator directly. The
// indirection keeps the test honest against future constraint additions:
// any new ADR-036-shaped constraint plumbed through Merge gets checked
// against the baseline pack for free.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// phaseBRepoRoot mirrors the sidecar package's repoRoot helper: walks
// up from this test file until it finds a directory containing go.mod
// AND a sibling test/fixtures/sidecars tree. Both anchors are required
// so the walk doesn't latch onto an unrelated nested module.
func phaseBRepoRoot(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(here)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "..", "..", "templates", "sidecars", "baseline")); err == nil {
				return filepath.Clean(filepath.Join(dir, "..", ".."))
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate repo root from phaseb test file")
	return ""
}

// baselineSidecars enumerates the 8 canonical baseline names. Kept in
// sync with the sidecar package's baselineSidecarNames slice — duplicated
// here rather than imported because the constant is package-private to
// sidecar. AC-5.1's shell check is the single external source of truth.
var baselineSidecarNames = []string{
	"backend-api",
	"frontend-component",
	"frontend-page",
	"db-queries",
	"db-transactions",
	"events-bus",
	"test-coverage",
	"owasp-baseline",
}

// TestBaselinePack_PassesPhaseB drives every baseline sidecar through
// Merge and asserts no error. A Phase B constraint regression — for
// example, a future ADR-036 amendment that adds a required field — will
// fail here first, signalling the baseline pack needs an update before
// the constraint can ship.
func TestBaselinePack_PassesPhaseB(t *testing.T) {
	root := phaseBRepoRoot(t)
	baselineDir := filepath.Join(root, "templates", "sidecars", "baseline")

	for _, name := range baselineSidecarNames {
		name := name
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(baselineDir, name+".edikt.yaml")
			sc, err := sidecar.Load(path)
			if err != nil {
				t.Fatalf("Load(%s): %v", name, err)
			}

			// Build a synthetic Pair inside a fresh tempdir. Merge needs a
			// project root with a writable docs/architecture/decisions/
			// directory; we create a placeholder parent .md there so the
			// pair satisfies the structural shape Merge walks.
			projectRoot := t.TempDir()
			parentDir := filepath.Join(projectRoot, "docs", "architecture", "decisions")
			if err := os.MkdirAll(parentDir, 0o755); err != nil {
				t.Fatalf("mkdir parent dir: %v", err)
			}
			// Synthetic ArtifactID — must satisfy the "ADR-NNN" prefix shape
			// that countByPrefix and downstream logic expect. Use a stable
			// placeholder; the parent file itself is not read by Merge.
			artifactBase := "ADR-999-baseline-" + name
			parentPath := filepath.Join(parentDir, artifactBase+".md")
			if err := os.WriteFile(parentPath, []byte("# baseline placeholder\n"), 0o644); err != nil {
				t.Fatalf("write parent .md: %v", err)
			}

			pair := sidecar.Pair{
				ParentPath:  parentPath,
				SidecarPath: path,
				ArtifactID:  "ADR-999",
				Sidecar:     sc,
			}

			res, err := Merge(projectRoot, []sidecar.Pair{pair}, Options{
				CompiledAt:      "2026-05-23T14:00:00Z",
				CompilerVersion: "0.6.0-baseline-test",
			})
			if err != nil {
				t.Fatalf("Merge(%s) failed Phase B: %v", name, err)
			}
			if res == nil {
				t.Fatalf("Merge(%s) returned nil result without error", name)
			}
		})
	}
}
