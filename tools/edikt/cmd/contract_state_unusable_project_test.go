package cmd

// Red-first record (PLAN-v091 phase 1, AC-1.10, 2026-10-10). Mutation made to
// see this test go red: restore the old fall-through in
// phasea.ResolveExtractorPromptVersionDetail (an unusable project file is
// skipped and the profile's version answers) -> red for both unusable
// sub-cases on every surface (exit 0 validated, "v4" printed, the plan step
// proceeds); make definitionPresent report a present-but-unreadable file as
// absent -> red for the unreadable sub-case in the internal/phasea test; drop
// the project-unusable arm of ReportInstalledContract -> red (the line loses
// its remedy and names the wrong reason).
// Final pass: the present-but-unreadable sub-case (skipped under root) is red
// when definitionPresent treats a file it cannot read as absent (profile v4
// answers: exit 0).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractState_UnusableProjectExtractorIsUnmeasured(t *testing.T) {
	const wantLine = "extraction contract state is UNMEASURED: this project's extractor is present but declares no usable prompt_version, and the profile's copy cannot stand in for it; run /edikt:upgrade to restore it from this version's template, or add the prompt_version line that template carries"

	setup := func(t *testing.T, projectBody string) (root, home string, adr string) {
		t.Helper()
		home = t.TempDir()
		root = t.TempDir()
		bbWriteConfig(t, root, "")
		adr = bbWriteADR(t, root, "ADR-alpha", gateQuoteBB)
		writeExtractorAt(t, filepath.Join(home, ".claude"), 4) // the profile pins a validated v4
		if projectBody != "" {
			p := filepath.Join(root, ".claude", "agents", "sidecar-extractor.md")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(projectBody), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return root, home, adr
	}

	cases := []struct{ name, body string }{
		{"no prompt_version line", "---\nname: sidecar-extractor\nmodel: claude-opus-5\n---\n\nbody\n"},
		{"no frontmatter fence", "no fence here\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, home, adr := setup(t, tc.body)
			cold := gateEnv(home)                                                 // no invoker: the report must not need one
			inv := gateEnv(home, "EDIKT_INVOKER=ci", "EDIKT_SKIP_VERSION_GATE=1") // verbs that want an identity

			out, errOut, code := runGated(t, root, cold, "doctor", "--contract-state")
			line := strings.TrimRight(out, "\n")
			if code != 11 || line != wantLine {
				t.Errorf("doctor --contract-state: exit %d, stdout %q (stderr %s), want exit 11 and %q", code, out, errOut, wantLine)
			}
			for _, bad := range []string{"v4", "validated", "\n"} {
				if strings.Contains(line, bad) {
					t.Errorf("the line names %q: %q", bad, line)
				}
			}

			// Full doctor: the same line directly under each compile remedy.
			dout, _, _ := runGated(t, root, inv, "doctor")
			lines := strings.Split(dout, "\n")
			remedies := 0
			for i, l := range lines {
				if isRemedyLine(l) {
					remedies++
					if i+1 >= len(lines) || lines[i+1] != contractIndent+wantLine {
						t.Errorf("line under %q is not the unmeasured contract line:\n%s", l, dout)
					}
				}
			}
			if remedies == 0 {
				t.Errorf("full doctor printed no compile remedy for an accepted record without a sidecar:\n%s", dout)
			}

			sout, serr, scode := runGated(t, root, inv, "gov", "reextract", "--status", "--json")
			if scode != 0 {
				t.Fatalf("gov reextract --status --json: exit %d\n%s%s", scode, sout, serr)
			}
			for _, want := range []string{`"contract_state": "unmeasured"`, `"contract_validated": false`} {
				if !strings.Contains(strings.ReplaceAll(sout, `":"`, `": "`), want) {
					t.Errorf("status JSON lacks %s:\n%s", want, sout)
				}
			}
			if strings.Contains(sout, `"v4"`) {
				t.Errorf("status JSON names the profile's version:\n%s", sout)
			}

			rel := filepath.ToSlash(filepath.Join("docs", "architecture", "decisions", filepath.Base(adr)))
			pout, perr, pcode := runGated(t, root, inv, "phasea", "plan", "--artifact", rel, "--bootstrap")
			if pcode != 4 {
				t.Errorf("phasea plan: exit %d, want the refusal code 4 (the code phasea plan already uses for an extractor with no version)\nstdout: %s\nstderr: %s", pcode, pout, perr)
			}
			for _, want := range []string{filepath.Join(".claude", "agents", "sidecar-extractor.md"), "/edikt:upgrade", "cannot stand in"} {
				if !strings.Contains(perr, want) {
					t.Errorf("phasea plan stderr lacks %q:\n%s", want, perr)
				}
			}
		})
	}

	t.Run("present but unreadable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a mode-000 file; the unreadable case cannot be built")
		}
		root, home, adr := setup(t, "---\nname: sidecar-extractor\nprompt_version: 11\n---\n\nbody\n")
		path := filepath.Join(root, ".claude", "agents", "sidecar-extractor.md")
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
		out, errOut, code := runGated(t, root, gateEnv(home), "doctor", "--contract-state")
		if code != 11 || strings.TrimRight(out, "\n") != wantLine {
			t.Errorf("doctor --contract-state: exit %d, stdout %q (stderr %s), want exit 11 and %q", code, out, errOut, wantLine)
		}
		rel := filepath.ToSlash(filepath.Join("docs", "architecture", "decisions", filepath.Base(adr)))
		inv := gateEnv(home, "EDIKT_INVOKER=ci", "EDIKT_SKIP_VERSION_GATE=1")
		_, perr, pcode := runGated(t, root, inv, "phasea", "plan", "--artifact", rel, "--bootstrap")
		if pcode != 4 {
			t.Errorf("phasea plan: exit %d, want 4\n%s", pcode, perr)
		}
	})

	t.Run("control: project absent, profile v4", func(t *testing.T) {
		root, home, _ := setup(t, "")
		out, errOut, code := runGated(t, root, gateEnv(home), "doctor", "--contract-state")
		if code != 0 || !strings.Contains(out, "v4 is validated") {
			t.Errorf("exit %d %q (stderr %s), want exit 0 validated", code, out, errOut)
		}
	})
}
