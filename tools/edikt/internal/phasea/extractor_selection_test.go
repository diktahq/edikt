package phasea

// Red-first record (PLAN-v091 phase 1, AC-1.11, 2026-10-10). Mutations made to
// see TestExtractorResolution_ProjectAlwaysWins go red, each reverted after:
//
//	let ResolveExtractorAgentModel walk project-then-profile again (skip an
//	  unreadable or model-less project file and read the profile's) -> red for
//	  every unusable-project case and for project-usable-without-model;
//	let ResolveExtractorContractDigest walk project-then-profile again -> red
//	  for the unreadable-project and directory-project cases;
//	restore the refusal for two usable copies that pin different versions
//	  -> red for the "project v11, profile v4" case (err set, version UNKNOWN);
//	make definitionPresent report an unreadable project file as absent (so the
//	  selection falls to the profile) -> red for the unreadable case on all
//	  three lookups and on the selected path;
//	M6: a zero-byte project file treated as absent (profile digest answers)
//	  -> red for b6 (and in the leak test);
//	M7: a project file with no version and no model yields the profile's model
//	  -> red for b7 (and in the leak test);
//	M8: a project file that is a symlink is skipped for the profile -> red for
//	  b8 and b9.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractorResolution_ProjectAlwaysWins(t *testing.T) {
	const (
		projectModel = "claude-opus-5"
		profileModel = "claude-sonnet-4"
	)
	agent := func(version, model string) string {
		b := "---\nname: sidecar-extractor\n"
		if model != "" {
			b += "model: " + model + "\n"
		}
		if version != "" {
			b += "prompt_version: " + version + "\n"
		}
		return b + "---\n\nbody\n"
	}
	profileV4 := agent("4", profileModel)

	type fileKind int
	const (
		absent fileKind = iota
		regular
		unreadable
		directory
		link     // a symlink to a usable v11 file elsewhere
		dangling // a symlink to nothing
	)
	type file struct {
		kind fileKind
		body string
	}
	type want struct {
		sel       candidateKind // "" = nothing installed
		version   string        // "" = UNKNOWN
		verErr    error
		model     string // "" = UNKNOWN
		digestErr bool   // digest cannot be read from the selected file
	}
	cases := []struct {
		name             string
		project, profile file
		want             want
	}{
		{"a: project v11 model X, profile v4 model Y",
			file{regular, agent("11", projectModel)}, file{regular, profileV4},
			want{sel: candidateProject, version: "v11", model: projectModel}},
		{"b1: project without a version line, profile v4",
			file{regular, agent("", projectModel)}, file{regular, profileV4},
			want{sel: candidateProject, verErr: ErrProjectDefinitionUnusable, model: projectModel}},
		{"b2: project malformed, profile v4",
			file{regular, "just prose\n"}, file{regular, profileV4},
			want{sel: candidateProject, verErr: ErrProjectDefinitionUnusable}},
		{"b3: project unreadable, profile v4",
			file{unreadable, agent("11", projectModel)}, file{regular, profileV4},
			want{sel: candidateProject, verErr: ErrProjectDefinitionUnusable, digestErr: true}},
		{"b4: project is a directory, profile v4",
			file{directory, ""}, file{regular, profileV4},
			want{sel: candidateProject, verErr: ErrProjectDefinitionUnusable, digestErr: true}},
		{"b5: project without a model, profile v4 model Y",
			file{regular, agent("11", "")}, file{regular, profileV4},
			want{sel: candidateProject, version: "v11"}},
		{"b6: project zero bytes, profile v4 model Y",
			file{regular, ""}, file{regular, profileV4},
			want{sel: candidateProject, verErr: ErrProjectDefinitionUnusable}},
		{"b7: project with no version and no model, profile v4 model Y",
			file{regular, agent("", "")}, file{regular, profileV4},
			want{sel: candidateProject, verErr: ErrProjectDefinitionUnusable}},
		{"b8: project is a symlink to a usable v11 file, profile v4 model Y",
			file{link, agent("11", projectModel)}, file{regular, profileV4},
			want{sel: candidateProject, version: "v11", model: projectModel}},
		{"b9: project is a dangling symlink, profile v4 model Y",
			file{dangling, ""}, file{regular, profileV4},
			want{sel: candidateProject, verErr: ErrProjectDefinitionUnusable, digestErr: true}},
		{"c: project absent, profile v4 model Y",
			file{absent, ""}, file{regular, profileV4},
			want{sel: candidateProfile, version: "v4", model: profileModel}},
		{"c2: project absent, profile without a version",
			file{absent, ""}, file{regular, agent("", profileModel)},
			want{sel: candidateProfile, verErr: ErrNoPromptVersion, model: profileModel}},
		{"d: project v11, profile unusable",
			file{regular, agent("11", projectModel)}, file{regular, "just prose\n"},
			want{sel: candidateProject, version: "v11", model: projectModel}},
		{"d2: project v4, profile v4 (same)",
			file{regular, agent("4", projectModel)}, file{regular, profileV4},
			want{sel: candidateProject, version: "v4", model: projectModel}},
		{"e: nothing installed",
			file{absent, ""}, file{absent, ""},
			want{verErr: ErrExtractorNotFound}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.project.kind == unreadable && os.Geteuid() == 0 {
				t.Skip("running as root: chmod 000 does not make a file unreadable")
			}
			root := t.TempDir()
			profileHome := t.TempDir()
			t.Setenv("CLAUDE_HOME", profileHome)
			projectPath := filepath.Join(root, ExtractorAgentRelPath)
			profilePath := filepath.Join(profileHome, "agents", "sidecar-extractor.md")

			place := func(path string, f file) {
				t.Helper()
				switch f.kind {
				case absent:
					return
				case directory:
					if err := os.MkdirAll(path, 0o755); err != nil {
						t.Fatal(err)
					}
					return
				case dangling, link:
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					target := filepath.Join(t.TempDir(), "elsewhere.md")
					if f.kind == link {
						if err := os.WriteFile(target, []byte(f.body), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
					return
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(f.body), 0o644); err != nil {
					t.Fatal(err)
				}
				if f.kind == unreadable {
					if err := os.Chmod(path, 0o000); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
				}
			}
			place(projectPath, tc.project)
			place(profilePath, tc.profile)

			// The selection itself.
			selPath, selKind, present := selectExtractorDefinition(root)
			wantPath := map[candidateKind]string{candidateProject: projectPath, candidateProfile: profilePath}[tc.want.sel]
			if present != (tc.want.sel != "") || selKind != tc.want.sel || selPath != wantPath {
				t.Fatalf("selection = (%q, %q, %v), want (%q, %q, %v)", selPath, selKind, present, wantPath, tc.want.sel, tc.want.sel != "")
			}

			// What the selected file alone says, read independently of the resolvers.
			var wantDigest string
			if !tc.want.digestErr && selPath != "" {
				raw, err := os.ReadFile(selPath)
				if err != nil {
					t.Fatalf("test setup: cannot read the selected file: %v", err)
				}
				sum := sha256.Sum256(raw)
				wantDigest = hex.EncodeToString(sum[:])
			}

			// 1. prompt version
			ver, _, verr := ResolveExtractorPromptVersionDetail(root)
			wantVer := tc.want.version
			if wantVer == "" {
				wantVer = ExtractorPromptVersionUnknown
			}
			if ver != wantVer {
				t.Errorf("version = %q, want %q (err %v)", ver, wantVer, verr)
			}
			if tc.want.verErr == nil && verr != nil {
				t.Errorf("version lookup refused: %v", verr)
			}
			if tc.want.verErr != nil && !errors.Is(verr, tc.want.verErr) {
				t.Errorf("version err = %v, want %v", verr, tc.want.verErr)
			}

			// 2. model
			model, merr := ResolveExtractorAgentModel(root)
			wantModel := tc.want.model
			if wantModel == "" {
				wantModel = ExtractorModelUnknown
			}
			if model != wantModel {
				t.Errorf("model = %q, want %q (err %v)", model, wantModel, merr)
			}
			if (tc.want.model == "") != (merr != nil) {
				t.Errorf("model err = %v, want error exactly when no model is pinned by the selected file", merr)
			}
			if tc.want.sel == "" && !errors.Is(merr, ErrExtractorNotFound) {
				t.Errorf("model err = %v, want ErrExtractorNotFound", merr)
			}

			// 3. contract digest
			digest, derr := ResolveExtractorContractDigest(root)
			switch {
			case tc.want.sel == "" || tc.want.digestErr:
				if digest != ExtractorContractUnrecorded || derr == nil {
					t.Errorf("digest = %q err=%v, want %q with an error", digest, derr, ExtractorContractUnrecorded)
				}
				if tc.want.sel == "" && !errors.Is(derr, ErrExtractorNotFound) {
					t.Errorf("digest err = %v, want ErrExtractorNotFound", derr)
				}
			default:
				if derr != nil || digest != wantDigest {
					t.Errorf("digest = %q err=%v, want the selected file's %q", digest, derr, wantDigest)
				}
			}

			// Explicit: while the project has a file, nothing in the answer is
			// the profile's. The two files carry distinct models and bodies, so
			// a leak cannot coincide with the right answer.
			if tc.want.sel == candidateProject {
				profileSum := sha256.Sum256([]byte(profileV4))
				if model == profileModel {
					t.Errorf("model is the PROFILE's (%q) while a project file exists", model)
				}
				if digest == hex.EncodeToString(profileSum[:]) {
					t.Errorf("digest is the PROFILE's while a project file exists")
				}
			}

			// No lookup, and no note, calls a differing profile a conflict.
			_, note, _ := ResolveExtractorPromptVersionDetail(root)
			for _, text := range []string{note, errText(verr), errText(merr), errText(derr)} {
				if strings.Contains(strings.ToLower(text), "conflict") {
					t.Errorf("mentions a conflict: %q", text)
				}
			}
		})
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
