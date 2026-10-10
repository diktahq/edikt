package gov

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// `gov reextract restore-pinned` reads each sidecar as it stood at a git ref
// and carries the pinned fields back. These pin what it says about
// artifact-level `paths:` (#11 comment item 4): a glob set no receipt covers
// is carried, never called approved; and unlike `phasea restore` it never
// drops a `paths:` — it is a repair from history, not a gate on an extractor.

const restorePinnedRel = "docs/architecture/decisions/ADR-900-test.edikt.yaml"

var restorePinnedGlobs = []string{"tools/edikt/**/*.go"}

func restorePinnedSidecar(t *testing.T, path string, mutate func(*sidecar.Sidecar)) {
	t.Helper()
	sc := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives: []sidecar.Directive{{
			Text:           "The gate MUST refuse an unsigned payload. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: "The gate MUST refuse an unsigned payload."}},
		}},
	}
	if mutate != nil {
		mutate(sc)
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// restorePinnedRepo commits `atRef` as the sidecar's history and leaves
// `live` in the working tree, then runs restore-pinned --from HEAD.
func restorePinnedRepo(t *testing.T, atRef, live func(*sidecar.Sidecar)) (root, out string) {
	t.Helper()
	root = t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(restorePinnedRel))
	restorePinnedSidecar(t, path, atRef)
	git := []string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}
	run(t, root, "git", "init", "-q")
	run(t, root, "git", append(git, "add", ".")...)
	run(t, root, "git", append(git, "commit", "-q", "-m", "seed")...)
	restorePinnedSidecar(t, path, live)

	home := t.TempDir()
	cmd := exec.Command(buildBinary(t), "gov", "reextract", "restore-pinned", "--from", "HEAD", root)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"EDIKT_HOME="+filepath.Join(home, ".edikt"),
		"CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"),
		"CLAUDE_HOME="+filepath.Join(home, ".claude"),
		"EDIKT_INVOKER=ci",
	)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restore-pinned: %v\n%s", err, b)
	}
	return root, string(b)
}

func TestReextractRestorePinned_PathsLabel(t *testing.T) {
	t.Run("unreceipted paths are restored and labelled unreceipted", func(t *testing.T) {
		root, out := restorePinnedRepo(t,
			func(sc *sidecar.Sidecar) { sc.Paths = restorePinnedGlobs },
			nil)
		if !strings.Contains(out, "restored ADR-900") {
			t.Fatalf("fixture: nothing was restored, so no label is under test:\n%s", out)
		}
		if strings.Contains(out, "approved paths") {
			t.Errorf("restore-pinned called paths no receipt covers \"approved paths\":\n%s", out)
		}
		if !strings.Contains(out, "unreceipted") {
			t.Errorf("restore-pinned must say the carried paths are unreceipted:\n%s", out)
		}
		sc, err := sidecar.Load(filepath.Join(root, filepath.FromSlash(restorePinnedRel)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(sc.Paths, ",") != strings.Join(restorePinnedGlobs, ",") {
			t.Errorf("unreceipted paths were not carried forward: %v", sc.Paths)
		}
	})

	t.Run("receipted paths are labelled approved", func(t *testing.T) {
		_, out := restorePinnedRepo(t,
			func(sc *sidecar.Sidecar) {
				sc.Paths = restorePinnedGlobs
				sc.PathsApproval = &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: sidecar.HashGlobs(restorePinnedGlobs)}
			},
			nil)
		if !strings.Contains(out, "approved paths") {
			t.Errorf("restore-pinned must call receipted paths approved:\n%s", out)
		}
		if strings.Contains(out, "unreceipted") {
			t.Errorf("receipted paths labelled unreceipted:\n%s", out)
		}
	})
}

// TestReextractRestorePinned_DoesNotStripPaths: the history had no `paths:`,
// the live sidecar has one, and a pinned field forces a rewrite. The rewrite
// must keep the live paths — dropping extractor-introduced scope is phasea
// restore's rule, made where the pre-dispatch state is known; restore-pinned
// only knows a ref, and a ref without paths is not evidence an extractor
// wrote the live ones.
func TestReextractRestorePinned_DoesNotStripPaths(t *testing.T) {
	root, out := restorePinnedRepo(t,
		func(sc *sidecar.Sidecar) { sc.Directives[0].HumanApprovedAt = "2026-08-03T00:00:00Z" },
		func(sc *sidecar.Sidecar) { sc.Paths = restorePinnedGlobs })
	if !strings.Contains(out, "restored ADR-900") {
		t.Fatalf("fixture: nothing was rewritten, so a strip had no chance to happen:\n%s", out)
	}
	sc, err := sidecar.Load(filepath.Join(root, filepath.FromSlash(restorePinnedRel)))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Directives[0].HumanApprovedAt == "" {
		t.Fatal("fixture: the pin was not restored")
	}
	if strings.Join(sc.Paths, ",") != strings.Join(restorePinnedGlobs, ",") {
		t.Errorf("restore-pinned stripped the live paths: got %v\n%s", sc.Paths, out)
	}
	if strings.Contains(out, "dropped") {
		t.Errorf("restore-pinned reported a drop:\n%s", out)
	}
}
