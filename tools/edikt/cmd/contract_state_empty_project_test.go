package cmd

// Red-first record (PLAN-v091 phase 1, AC-1.11, 2026-10-10). Mutations made to
// see TestContractState_EmptyProjectExtractorIsNotAbsent go red, each reverted:
//
//	treat a zero-byte project extractor as absent in
//	  phasea.selectExtractorDefinition (the built binary then printed
//	  "extraction contract v4 is validated", exit 0) -> red for "zero bytes";
//	skip a project file that is a symlink (definitionPresent via os.Stat
//	  for a dangling link, or selection ignoring links) -> red for "dangling
//	  symlink" (profile v4 answers) and "symlink to a usable v11 file" (exit 0,
//	  v4 instead of v11 REJECTED exit 10).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractState_EmptyProjectExtractorIsNotAbsent(t *testing.T) {
	const unusableLine = "extraction contract state is UNMEASURED: this project's extractor is present but declares no usable prompt_version, and the profile's copy cannot stand in for it; run /edikt:upgrade to restore it from this version's template, or add the prompt_version line that template carries"

	setup := func(t *testing.T, place func(path string)) (root, home string) {
		t.Helper()
		home, root = t.TempDir(), t.TempDir()
		bbWriteConfig(t, root, "")
		writeExtractorAt(t, filepath.Join(home, ".claude"), 4) // the profile pins a validated v4
		p := filepath.Join(root, ".claude", "agents", "sidecar-extractor.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		place(p)
		return root, home
	}

	unusable := []struct {
		name  string
		place func(t *testing.T, path string)
	}{
		{"zero bytes", func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"dangling symlink", func(t *testing.T, path string) {
			if err := os.Symlink(filepath.Join(t.TempDir(), "nowhere.md"), path); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range unusable {
		t.Run(tc.name, func(t *testing.T) {
			root, home := setup(t, func(path string) { tc.place(t, path) })
			out, errOut, code := runGated(t, root, gateEnv(home), "doctor", "--contract-state")
			line := strings.TrimRight(out, "\n")
			if code != 11 || line != unusableLine {
				t.Errorf("exit %d, stdout %q (stderr %s), want exit 11 and %q", code, out, errOut, unusableLine)
			}
			for _, bad := range []string{"v4", "validated"} {
				if strings.Contains(line, bad) {
					t.Errorf("the line names %q: %q", bad, line)
				}
			}
		})
	}

	t.Run("symlink to a usable v11 file", func(t *testing.T) {
		root, home := setup(t, func(path string) {
			target := filepath.Join(t.TempDir(), "elsewhere", "v11.md")
			writeExtractorAt(t, filepath.Dir(filepath.Dir(target)), 11) // writes <dir>/agents/sidecar-extractor.md
			real := filepath.Join(filepath.Dir(filepath.Dir(target)), "agents", "sidecar-extractor.md")
			if err := os.Symlink(real, path); err != nil {
				t.Fatal(err)
			}
		})
		out, errOut, code := runGated(t, root, gateEnv(home), "doctor", "--contract-state")
		if code != 10 || !strings.Contains(out, "extraction contract v11 is REJECTED") || strings.Contains(out, "v4") {
			t.Errorf("exit %d, stdout %q (stderr %s), want exit 10 with v11 REJECTED and no v4", code, out, errOut)
		}
	})
}
