package phasea

// Red-first record (PLAN-v091 phase 1, AC-1.10, 2026-10-10). Mutation made to
// see this test go red: restore the old fall-through in
// ResolveExtractorPromptVersionDetail (drop the `first.kind == candidateProject
// && first.problem != ""` refusal) -> red for sub-cases (a) to (d), each of
// which then answers v4 from the profile. Make definitionPresent treat any
// Lstat error as absent -> red for (c) and (d). The controls stay green under
// both mutations, which is what makes them controls.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveExtractorPromptVersion_UnusableProjectDefinitionIsNotMaskedByProfile(t *testing.T) {
	const pinned4 = "---\nname: sidecar-extractor\nprompt_version: 4\n---\n"
	projectPath := func(root string) string { return filepath.Join(root, ExtractorAgentRelPath) }
	put := func(t *testing.T, root, body string) {
		t.Helper()
		p := projectPath(root)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	unusable := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{"a: no prompt_version line", func(t *testing.T, root string) {
			put(t, root, "---\nname: sidecar-extractor\nmodel: sonnet\n---\n")
		}},
		{"b: no frontmatter fence", func(t *testing.T, root string) { put(t, root, "just prose\n") }},
		{"c: present but unreadable", func(t *testing.T, root string) {
			if os.Geteuid() == 0 {
				t.Skip("running as root: chmod 000 does not make a file unreadable")
			}
			put(t, root, pinned4)
			if err := os.Chmod(projectPath(root), 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(projectPath(root), 0o644) })
		}},
		{"d: a directory at the path", func(t *testing.T, root string) {
			if err := os.MkdirAll(projectPath(root), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"e: a dangling symlink", func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Dir(projectPath(root)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "nowhere.md"), projectPath(root)); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range unusable {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeProfileAgent(t, pinned4)
			tc.setup(t, root)
			got, _, err := ResolveExtractorPromptVersionDetail(root)
			if got != ExtractorPromptVersionUnknown {
				t.Errorf("answered %q for an unusable project definition; the profile's v4 must not stand in", got)
			}
			if !errors.Is(err, ErrProjectDefinitionUnusable) {
				t.Fatalf("err = %v, want ErrProjectDefinitionUnusable", err)
			}
			if !strings.Contains(err.Error(), projectPath(root)) {
				t.Errorf("error %q does not name the project path %s", err, projectPath(root))
			}
		})
	}

	t.Run("control: project absent, profile v4", func(t *testing.T) {
		root := t.TempDir()
		writeProfileAgent(t, pinned4)
		if got, _, err := ResolveExtractorPromptVersionDetail(root); err != nil || got != "v4" {
			t.Errorf("got %q err=%v, want v4", got, err)
		}
	})
	t.Run("control: project v11, profile unusable", func(t *testing.T) {
		root := t.TempDir()
		put(t, root, "---\nname: sidecar-extractor\nprompt_version: 11\n---\n")
		writeProfileAgent(t, "---\nname: sidecar-extractor\n---\n")
		got, note, err := ResolveExtractorPromptVersionDetail(root)
		if err != nil || got != "v11" || !strings.Contains(note, "carries no prompt_version") {
			t.Errorf("got %q note=%q err=%v, want v11 with a note", got, note, err)
		}
	})
	t.Run("control: project v4, profile v11, the project wins", func(t *testing.T) {
		root := t.TempDir()
		put(t, root, pinned4)
		writeProfileAgent(t, "---\nname: sidecar-extractor\nprompt_version: 11\n---\n")
		if got, _, err := ResolveExtractorPromptVersionDetail(root); err != nil || got != "v4" {
			t.Errorf("got %q err=%v, want v4 from the project", got, err)
		}
	})
	t.Run("control: sole project definition without a version keeps its own error", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("CLAUDE_HOME", t.TempDir())
		put(t, root, "---\nname: sidecar-extractor\n---\n")
		if _, _, err := ResolveExtractorPromptVersionDetail(root); !errors.Is(err, ErrNoPromptVersion) {
			t.Errorf("err = %v, want ErrNoPromptVersion", err)
		}
	})
}
