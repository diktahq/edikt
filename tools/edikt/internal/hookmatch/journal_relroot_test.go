package hookmatch

import (
	"path/filepath"
	"strings"
	"testing"
)

// A relative EDIKT_ROOT once produced the relative dir "state", which MkdirAll
// created in whatever cwd the process had. Reproduced against a repo root:
// a stray state/hook-journal.jsonl, untracked and un-ignored.
//
// The pair is what makes this a real control: an absolute root MUST still
// resolve, or a test asserting only the refusal would pass against a resolver
// that refuses everything (PRD-009 FR-017).
func TestEdiktHomeRefusesRelativeRoots(t *testing.T) {
	abs := t.TempDir() // always absolute

	for _, tc := range []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"relative EDIKT_ROOT is refused", map[string]string{"EDIKT_ROOT": "."}, "EDIKT_ROOT"},
		{"relative EDIKT_ROOT subpath is refused", map[string]string{"EDIKT_ROOT": "some/dir"}, "EDIKT_ROOT"},
		{"relative EDIKT_HOME is refused", map[string]string{"EDIKT_ROOT": "", "EDIKT_HOME": "./x"}, "EDIKT_HOME"},
		{"relative HOME is refused", map[string]string{"EDIKT_ROOT": "", "EDIKT_HOME": "", "HOME": "rel"}, "$HOME"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, err := ediktHome()
			if err == nil {
				t.Fatalf("ediktHome() = %q, want an error — a relative root writes state into the process cwd", got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name %s", err, tc.wantErr)
			}
		})
	}

	t.Run("an absolute root still resolves", func(t *testing.T) {
		t.Setenv("EDIKT_ROOT", abs)
		got, err := ediktHome()
		if err != nil {
			t.Fatalf("absolute EDIKT_ROOT rejected: %v", err)
		}
		if got != abs {
			t.Errorf("ediktHome() = %q, want %q", got, abs)
		}
	})

	t.Run("JournalPath under an absolute root stays absolute", func(t *testing.T) {
		t.Setenv("EDIKT_ROOT", abs)
		p, err := JournalPath()
		if err != nil {
			t.Fatalf("JournalPath: %v", err)
		}
		if !filepath.IsAbs(p) {
			t.Errorf("JournalPath() = %q, want an absolute path", p)
		}
	})
}
