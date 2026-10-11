package cmd

// Every doctor path that tells the reader to run a compile command for a
// missing sidecar (the sidecar check, the delivery-gaps finding, the
// rejected-options note) names the installed extraction contract directly
// under that remedy when the contract is not validated, and prints nothing
// extra when it is.
//
// The tests compare each path's COMPLETE rendered output, not a filtered
// subset, so a line printed twice, printed for one artifact only, printed for
// one kind only, followed by a second line, or left as a blank line all fail.
//
//   - Validated: the output equals the golden captured from the v0.9.0 tag.
//   - Any other state: the expected text is that same golden with exactly one
//     "contract: <summary>" line inserted directly under each remedy line,
//     the summary taken from the reextract package.
//
// A differing profile copy never reaches these lines: the project's extractor
// answers (rejected v11 beside a validated v4 profile is rejected; validated v4
// beside a rejected v11 profile prints the unchanged golden). Mutation: let the
// resolver answer from the profile when the two differ -> red for both.
//
// Isolation: every test points HOME, EDIKT_HOME, EDIKT_ROOT, CLAUDE_HOME and
// CLAUDE_CONFIG_DIR at temp dirs (isolateDoctorEnv) and installs an explicit
// extractor, so nothing reads the developer's own profile. The states run in
// two environments: CLAUDE_HOME set, and CLAUDE_HOME unset with the profile at
// $HOME/.claude.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/reextract"
)

const contractIndent = "           contract: "

type contractState struct {
	name string
	// project is the version installed in the fixture project (-1 none, 0 a
	// file with no prompt_version); profile likewise for the profile.
	project, profile int
	want             string // reextract state it must resolve to
}

var (
	validatedState = contractState{"validated v4", 4, -1, reextract.ContractStateValidated}
	otherStates    = []contractState{
		{"rejected v11", 11, -1, "rejected"},
		{"unrecorded v99", 99, -1, "unrecorded"},
		{"unmeasured, no extractor", -1, -1, reextract.ContractStateUnmeasured},
		{"rejected v11 in the project beside a validated v4 profile", 11, 4, "rejected"},
		{"unmeasured, no prompt_version", 0, -1, reextract.ContractStateUnmeasured},
		{"unmeasured, project extractor unusable beside a pinned profile", 0, 4, reextract.ContractStateUnmeasured},
	}
)

// envModes: the profile is found through CLAUDE_HOME, or through $HOME/.claude.
var envModes = []struct {
	name          string
	claudeHomeSet bool
}{{"CLAUDE_HOME set", true}, {"CLAUDE_HOME unset", false}}

// stateProject isolates the environment, installs the state's extractors and
// returns the fixture root.
func stateProject(t *testing.T, claudeHomeSet bool, s contractState, f contractFixture) string {
	t.Helper()
	profile := isolateDoctorEnv(t, claudeHomeSet)
	root := buildContractFixture(t, f)
	if s.project >= 0 {
		writeExtractorAt(t, filepath.Join(root, ".claude"), s.project)
	}
	if s.profile >= 0 {
		writeExtractorAt(t, profile, s.profile)
	}
	return root
}

func loadGolden(t *testing.T) (string, map[string]string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "doctor-missing-sidecar-validated.golden"))
	if err != nil {
		t.Fatal(err)
	}
	prov, secs := parseGolden(string(raw))
	return prov, secs
}

// isRemedyLine recognises the lines that tell the reader to run a compile
// command for a missing sidecar.
func isRemedyLine(l string) bool {
	return strings.Contains(l, "has no sidecar — run /edikt:") ||
		strings.HasPrefix(l, "           run: /edikt:") ||
		strings.Contains(l, "no sidecar — skipping prohibition check (run /edikt:")
}

// withContractLines returns section with one contract line directly under each
// remedy line, and how many it inserted.
func withContractLines(section, summary string) (string, int) {
	var sb strings.Builder
	n := 0
	for _, l := range strings.SplitAfter(section, "\n") {
		sb.WriteString(l)
		if l != "" && isRemedyLine(strings.TrimRight(l, "\n")) {
			sb.WriteString(contractIndent + summary + "\n")
			n++
		}
	}
	return sb.String(), n
}

func (c capturedPaths) byPath(p string) string {
	switch p {
	case goldenPathSidecar:
		return c.sidecar
	case goldenPathGaps:
		return c.gaps
	}
	return c.rejected
}

func TestDoctor_MissingSidecarRemedyNamesContractState(t *testing.T) {
	_, golden := loadGolden(t)
	paths := []string{goldenPathSidecar, goldenPathGaps, goldenPathRejected}
	for _, mode := range envModes {
		for _, s := range otherStates {
			for _, f := range contractFixtures {
				t.Run(mode.name+"/"+s.name+"/"+f.name, func(t *testing.T) {
					root := stateProject(t, mode.claudeHomeSet, s, f)
					rep := reextract.ReportInstalledContract(root)
					if rep.State != s.want || rep.Summary == "" {
						t.Fatalf("fixture is not the %q state: %+v", s.want, rep)
					}
					if rep.State == reextract.ContractStateUnmeasured && !strings.Contains(rep.Summary, "UNMEASURED") {
						t.Fatalf("an unmeasured report must say so: %q", rep.Summary)
					}
					got := captureDoctorPaths(root)
					for i, p := range paths {
						sec, ok := golden["=== "+f.name+" | "+p]
						if !ok {
							t.Fatalf("golden has no section for %s | %s", f.name, p)
						}
						want, n := withContractLines(sec, rep.Summary)
						if n != f.wantRemedies[i] {
							t.Fatalf("golden %s | %s holds %d remedy line(s), fixture says %d", f.name, p, n, f.wantRemedies[i])
						}
						if g := got.byPath(p); g != want {
							t.Errorf("%s path output differs from the golden plus one contract line under each remedy.\n--- got ---\n%s--- want ---\n%s", p, g, want)
						}
						if strings.Contains(strings.ToLower(got.byPath(p)), "blocked") {
							t.Errorf("%s path says the remedy is blocked:\n%s", p, got.byPath(p))
						}
					}
				})
			}
		}
	}
}

func TestDoctor_BothMissingSidecarPathsPrintTheSameContractLine(t *testing.T) {
	contractLines := func(out string) []string {
		var ls []string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "contract") {
				ls = append(ls, l)
			}
		}
		return ls
	}
	byName := map[string]contractFixture{}
	for _, f := range contractFixtures {
		byName[f.name] = f
	}
	s := otherStates[0] // rejected v11
	var lines []string
	for _, c := range []struct{ fixture, path string }{
		{"one-adr", goldenPathSidecar}, {"one-adr", goldenPathGaps}, {"proposed-with-options", goldenPathRejected},
	} {
		root := stateProject(t, true, s, byName[c.fixture])
		got := captureDoctorPaths(root).byPath(c.path)
		ls := contractLines(got)
		if len(ls) != 1 {
			t.Fatalf("%s path printed %d contract line(s), want exactly 1:\n%s", c.path, len(ls), got)
		}
		lines = append(lines, ls[0])
	}
	want := contractIndent + reextract.ReportContract("v11").Summary
	for i, l := range lines {
		if l != want {
			t.Errorf("path %d prints %q, want %q", i, l, want)
		}
	}
}

func TestDoctor_ValidatedContractOutputMatches090Golden(t *testing.T) {
	prov, golden := loadGolden(t)
	if !strings.HasPrefix(prov, "# Captured from tag v0.9.0 (commit d511c3fa52ff080a0a86b01eb7676c7cc28eb8e7)") {
		t.Fatalf("golden line 1 does not name the v0.9.0 tag and the tagged commit: %q", prov)
	}
	if r := reextract.ReportContract("v4"); r.State != reextract.ContractStateValidated {
		t.Fatalf("v4 is not validated any more: %+v", r)
	}
	// The project's v4 stays validated, and prints the golden, beside a
	// profile that pins a rejected v11: the profile never reaches the report.
	for _, vs := range []contractState{validatedState, {"validated v4 in the project beside a different v11 profile", 4, 11, reextract.ContractStateValidated}} {
		for _, mode := range envModes {
			for _, f := range contractFixtures {
				t.Run(vs.name+"/"+mode.name+"/"+f.name, func(t *testing.T) {
					root := stateProject(t, mode.claudeHomeSet, vs, f)
					if rep := reextract.ReportInstalledContract(root); rep.State != reextract.ContractStateValidated {
						t.Fatalf("fixture is not validated: %+v", rep)
					}
					got := captureDoctorPaths(root)
					for _, p := range []string{goldenPathSidecar, goldenPathGaps, goldenPathRejected} {
						want, ok := golden["=== "+f.name+" | "+p]
						if !ok {
							t.Fatalf("golden has no section for %s | %s", f.name, p)
						}
						if g := got.byPath(p); g != want {
							t.Errorf("%s path differs from the v0.9.0 golden.\n--- got ---\n%q\n--- want ---\n%q", p, g, want)
						}
					}
				})
			}
		}
	}
}

// TestDoctor_BuiltBinaryNamesContractUnderEachRemedy runs the built binary's
// full `edikt doctor`, so the wiring (which stream, which cwd, which root) is
// covered and not only the functions.
func TestDoctor_BuiltBinaryNamesContractUnderEachRemedy(t *testing.T) {
	run := func(t *testing.T, version int) (stdout, stderr string) {
		t.Helper()
		isolateDoctorEnv(t, true)
		root := buildContractFixture(t, contractFixtures[0]) // one missing ADR
		writeExtractorAt(t, filepath.Join(root, ".claude"), version)
		c := exec.Command(buildBinary(t), "doctor")
		c.Dir = root
		c.Env = os.Environ() // isolateDoctorEnv already replaced the five variables
		var out, errb bytes.Buffer
		c.Stdout, c.Stderr = &out, &errb
		_ = c.Run() // doctor exits non-zero on the missing sidecar
		return out.String(), errb.String()
	}

	t.Run("v11 names the contract under both remedies, on stdout", func(t *testing.T) {
		out, errOut := run(t, 11)
		want := contractIndent + reextract.ReportContract("v11").Summary
		lines := strings.Split(out, "\n")
		seen := 0
		for i, l := range lines {
			if strings.Contains(l, "has no sidecar — run /edikt:adr:compile") ||
				strings.HasPrefix(l, "           run: /edikt:adr:compile") {
				seen++
				if i+1 >= len(lines) || lines[i+1] != want {
					t.Errorf("line under %q is not the contract line:\n%s", l, out)
				}
			}
		}
		if seen != 2 {
			t.Errorf("saw %d remedy lines, want 2 (sidecar check, delivery gaps):\n%s", seen, out)
		}
		if n := strings.Count(out, want); n != 2 {
			t.Errorf("contract line appears %d times on stdout, want 2:\n%s", n, out)
		}
		if strings.Contains(errOut, "contract:") {
			t.Errorf("a contract line leaked to stderr:\n%s", errOut)
		}
	})

	t.Run("v4 prints no contract line anywhere", func(t *testing.T) {
		out, errOut := run(t, 4)
		if !strings.Contains(out, "has no sidecar — run /edikt:adr:compile") {
			t.Fatalf("fixture did not reach the missing-sidecar finding:\n%s", out)
		}
		for name, s := range map[string]string{"stdout": out, "stderr": errOut} {
			if strings.Contains(s, "contract:") {
				t.Errorf("%s has a contract line under a validated contract:\n%s", name, s)
			}
		}
	})
}

// The fixtures that older doctor tests build must not see the machine's own
// profile. The profile here holds a rejected extractor (v11) and is reachable
// through HOME; each builder has to hide it and install its own.
func TestDoctorFixtureBuildersIgnoreTheDevelopersProfile(t *testing.T) {
	poison := func(t *testing.T) {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		os.Unsetenv("CLAUDE_HOME")
		os.Unsetenv("CLAUDE_CONFIG_DIR")
		writeExtractorAt(t, filepath.Join(home, ".claude"), 11)
	}
	for name, build := range map[string]func(*testing.T) string{
		"gapProject":                     gapProject,
		"scaffoldProject":                scaffoldProject,
		"scaffoldRejectedOptionsProject": scaffoldRejectedOptionsProject,
	} {
		t.Run(name, func(t *testing.T) {
			poison(t)
			root := build(t)
			rep := reextract.ReportInstalledContract(root)
			if rep.State != reextract.ContractStateValidated || rep.PromptVersion != "v4" {
				t.Errorf("the fixture resolved %+v; it must resolve its own v4 and not the profile's v11", rep)
			}
		})
	}
}
