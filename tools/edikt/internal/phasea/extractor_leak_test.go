package phasea

// Red-first record (PLAN-v091 phase 1, AC-1.11, 2026-10-10). Mutations made to
// see these tests go red, each reverted after:
//
//	M6: a zero-byte project extractor yields the PROFILE's digest (the
//	  selection treats an empty project file as absent)
//	  -> red: TestExtractorResolution_ProfileNeverLeaksIntoModelOrDigest/zero_bytes;
//	M7: a project extractor with no prompt_version and no model line yields the
//	  PROFILE's model (ResolveExtractorAgentModel falls back to the profile when
//	  the project's model is empty) -> red: .../no_version_no_model;
//	M8: a project file that is a symlink is skipped in favour of the profile
//	  (definitionPresent uses os.Stat instead of os.Lstat, or selection skips
//	  links) -> red: .../symlink_to_usable;
//	M3: an unusable project file with NO profile copy returns
//	  ErrProjectDefinitionUnusable instead of ErrNoPromptVersion
//	  -> red: TestResolveExtractorPromptVersion_SoleUnusableProjectIsNoPromptVersion;
//	the link note: drop the sameFile branch -> red:
//	  TestSelectionNote_ProjectLinkedToProfileIsOneDefinition.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractorResolution_ProfileNeverLeaksIntoModelOrDigest(t *testing.T) {
	const profileModel = "profile-sentinel-model"
	profileBody := "---\nname: sidecar-extractor\nmodel: " + profileModel + "\nprompt_version: 4\n---\n\nprofile body\n"
	profileSum := sha256.Sum256([]byte(profileBody))
	profileDigest := hex.EncodeToString(profileSum[:])

	cases := []struct {
		name  string
		setup func(t *testing.T, project string)
	}{
		{"zero bytes", func(t *testing.T, p string) { writeFile(t, p, "") }},
		{"no version no model", func(t *testing.T, p string) { writeFile(t, p, "---\nname: sidecar-extractor\n---\n\nproject body\n") }},
		{"symlink to usable", func(t *testing.T, p string) {
			target := filepath.Join(t.TempDir(), "usable.md")
			writeFile(t, target, "---\nname: sidecar-extractor\nmodel: project-sentinel-model\nprompt_version: 11\n---\n\nproject body\n")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, p); err != nil {
				t.Fatal(err)
			}
		}},
		{"dangling symlink", func(t *testing.T, p string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "nowhere.md"), p); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			t.Setenv("CLAUDE_HOME", home)
			writeFile(t, filepath.Join(home, "agents", "sidecar-extractor.md"), profileBody)
			tc.setup(t, filepath.Join(root, ExtractorAgentRelPath))

			model, _ := ResolveExtractorAgentModel(root)
			if model == profileModel {
				t.Errorf("model is the profile's %q while a project file exists", model)
			}
			digest, _ := ResolveExtractorContractDigest(root)
			if digest == profileDigest {
				t.Errorf("digest is the profile's while a project file exists")
			}
			ver, _, _ := ResolveExtractorPromptVersionDetail(root)
			if ver == "v4" {
				t.Errorf("version is the profile's while a project file exists")
			}
		})
	}
}

func TestResolveExtractorPromptVersion_SoleUnusableProjectIsNoPromptVersion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_HOME", t.TempDir()) // no profile copy
	writeFile(t, filepath.Join(root, ExtractorAgentRelPath), "---\nname: sidecar-extractor\n---\n")
	ver, _, err := ResolveExtractorPromptVersionDetail(root)
	if ver != ExtractorPromptVersionUnknown {
		t.Errorf("version = %q, want UNKNOWN", ver)
	}
	if !errors.Is(err, ErrNoPromptVersion) {
		t.Errorf("err = %v, want ErrNoPromptVersion (the project file is the only definition)", err)
	}
	if errors.Is(err, ErrProjectDefinitionUnusable) {
		t.Errorf("err = %v, ErrProjectDefinitionUnusable is for an unusable project file BESIDE a profile copy", err)
	}
}

func TestSelectionNote_ProjectLinkedToProfileIsOneDefinition(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("CLAUDE_HOME", home)
	profile := filepath.Join(home, "agents", "sidecar-extractor.md")
	writeFile(t, profile, "---\nname: sidecar-extractor\nmodel: sonnet\nprompt_version: 4\n---\n")
	project := filepath.Join(root, ExtractorAgentRelPath)
	if err := os.MkdirAll(filepath.Dir(project), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(profile, project); err != nil {
		t.Fatal(err)
	}
	ver, note, err := ResolveExtractorPromptVersionDetail(root)
	if err != nil || ver != "v4" {
		t.Fatalf("version = %q err=%v, want v4", ver, err)
	}
	if !strings.Contains(note, "the project's extractor is a link to the profile's copy; one definition") {
		t.Errorf("note does not say it is one definition: %q", note)
	}
	for _, bad := range []string{"found 2 definitions", "ignored"} {
		if strings.Contains(note, bad) {
			t.Errorf("note says %q about a single file: %q", bad, note)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
