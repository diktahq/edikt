package reextract

// Red-first record (PLAN-v091 phase 1, corrected 2026-10-10). One-line
// mutation made to see each test go red:
//
//	SummaryIsOneConsumerSafeLine: echo the resolver error into the unmeasured
//	  summary -> red (path segment); collapse ReportInstalledContract's switch
//	  to one reason -> red (per-kind reason comparison); make unmeasured()
//	  return "" -> red (non-empty assertion).
//	  (v0.9.1 resolver fix) drop the remedy clause from one reason constant
//	  -> red (every unmeasured line is pinned in full below, as a literal, not
//	  through the constant); make a project-unusable error fall into the
//	  default arm -> red (its own line is pinned); restore the old
//	  fall-through in the resolver -> red (project-unusable case).
//	UnreadableIsUnmeasured: return validated from reportFrom's default arm
//	  -> red.
//	EmptyRecordIsUnmeasured: delete the "no entries -> unreadable" guard in
//	  contractStateFrom -> red for nil, empty, comment-only, wrong-keyed and
//	  HTML records (each came back unrecorded).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_HOME", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	return home
}

func writeAgent(t *testing.T, path, frontmatter string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\n"+frontmatter+"---\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

var commandNameRe = regexp.MustCompile(`/edikt:[a-z:-]+`)

var internalIDRe = regexp.MustCompile(`(ADR|INV|SPEC|PRD|PLAN|BRAIN|DISCOVERY)-[0-9]`)

func assertConsumerSafe(t *testing.T, label, summary string) {
	t.Helper()
	if summary == "" || strings.Contains(summary, "\n") {
		t.Errorf("%s: summary must be exactly one non-empty line, got %q", label, summary)
	}
	// "/edikt:upgrade" is a command name, not a path; nothing else may carry a "/".
	if strings.Contains(commandNameRe.ReplaceAllString(summary, ""), "/") {
		t.Errorf("%s: summary carries a path segment: %q", label, summary)
	}
	if internalIDRe.MatchString(summary) {
		t.Errorf("%s: summary names an internal artifact id: %q", label, summary)
	}
}

func TestContractReport_SummaryIsOneConsumerSafeLine(t *testing.T) {
	home := isolateHome(t)
	for _, v := range []string{"v4", "v11", "v99", "UNKNOWN"} {
		assertConsumerSafe(t, v, ReportContract(v).Summary)
	}
	assertConsumerSafe(t, "unreadable record", reportFrom([]byte("validated: [unterminated"), "v4").Summary)

	// Resolver errors from REAL unresolvable projects, which embed absolute paths.
	empty := t.TempDir()
	unpinned := t.TempDir()
	writeAgent(t, filepath.Join(unpinned, ".claude", "agents", "sidecar-extractor.md"), "name: sidecar-extractor\n")
	projectUnusable := t.TempDir()
	writeAgent(t, filepath.Join(projectUnusable, ".claude", "agents", "sidecar-extractor.md"), "name: sidecar-extractor\n")
	profileAgent := filepath.Join(home, ".claude", "agents", "sidecar-extractor.md")

	// Every line is a LITERAL: each unmeasured reason ends with its remedy, and
	// a reason that lost its remedy (or two that collapsed into one) fails here.
	const head = "extraction contract state is UNMEASURED: "
	seen := map[string]string{}
	for _, c := range []struct{ name, root, want string }{
		{"none", empty, head + "no extractor is installed; run /edikt:upgrade to install it"},
		{"unpinned", unpinned, head + "the extractor declares no usable prompt_version; run /edikt:upgrade to restore it from this version's template, or add the prompt_version line that template carries"},
		{"project-unusable", projectUnusable, head + "this project's extractor is present but declares no usable prompt_version, and the profile's copy cannot stand in for it; run /edikt:upgrade to restore it from this version's template, or add the prompt_version line that template carries"},
	} {
		name, root := c.name, c.root
		if name == "project-unusable" {
			writeAgent(t, profileAgent, "prompt_version: 4\n")
		}
		r := ReportInstalledContract(root)
		if r.State != ContractStateUnmeasured {
			t.Errorf("%s: state %q, want unmeasured", name, r.State)
		}
		assertConsumerSafe(t, name, r.Summary)
		if strings.Contains(r.Summary, root) {
			t.Errorf("%s: summary leaks the project root: %q", name, r.Summary)
		}
		if r.Summary != c.want {
			t.Errorf("%s: summary %q, want %q", name, r.Summary, c.want)
		}
		seen[name] = r.Summary
	}
	for _, p := range []string{"/nonexistent/sidecar-extractor.md", filepath.Join(t.TempDir(), "other.md")} {
		r := ReportAgentFileContract(p)
		if r.State != ContractStateUnmeasured {
			t.Errorf("%s: state %q, want unmeasured", p, r.State)
		}
		assertConsumerSafe(t, p, r.Summary)
		if want := head + "the named agent file could not be read; check the path given to --agent-file, and reinstall edikt if this version's template is missing"; r.Summary != want {
			t.Errorf("%s: summary %q, want %q", p, r.Summary, want)
		}
	}
	noVer := filepath.Join(t.TempDir(), "sidecar-extractor.md")
	writeAgent(t, noVer, "name: sidecar-extractor\n")
	seen["file-no-version"] = ReportAgentFileContract(noVer).Summary
	assertConsumerSafe(t, "file-no-version", seen["file-no-version"])
	if want := head + "the named agent file declares no usable prompt_version; check the path given to --agent-file, and reinstall edikt if this version's template is missing"; seen["file-no-version"] != want {
		t.Errorf("named file without version: %q, want %q", seen["file-no-version"], want)
	}
	seen["file"] = ReportAgentFileContract("/nonexistent/sidecar-extractor.md").Summary
	seen["record"] = reportFrom([]byte("validated: [unterminated"), "v4").Summary
	seen["unresolvable"] = ReportContract("UNKNOWN").Summary
	for k, v := range seen {
		assertConsumerSafe(t, k, v)
	}
	if want := head + "this build's validation record could not be read; reinstall edikt"; seen["record"] != want {
		t.Errorf("unreadable record summary %q, want %q", seen["record"], want)
	}
	if want := head + "the extractor's prompt version could not be resolved; run edikt doctor for details"; seen["unresolvable"] != want {
		t.Errorf("unresolvable summary %q, want %q", seen["unresolvable"], want)
	}
	byText := map[string]string{}
	for k, v := range seen {
		if k2, dup := byText[v]; dup {
			t.Errorf("kinds %q and %q share one summary %q: reasons must be distinct", k, k2, v)
		}
		byText[v] = k
	}
	// The unrecorded state also says what to do.
	if got, want := reportFrom([]byte("validated:\n  - prompt_version: v4\n"), "v99").Summary,
		"extraction contract v99 has no entry in this build's validation record: neither validated nor rejected, so review each sidecar it writes against its prose"; got != want {
		t.Errorf("unrecorded summary %q, want %q", got, want)
	}
}

func TestContractReport_EmptyRecordIsUnmeasured(t *testing.T) {
	for name, rec := range map[string][]byte{
		"nil":          nil,
		"empty":        []byte(""),
		"comment-only": []byte("# validated:\n#   - prompt_version: v4\n"),
		"wrong-keyed":  []byte("approved:\n  - prompt_version: v4\n"),
		"html":         []byte("<!doctype html><html><body>404</body></html>"),
		"empty-lists":  []byte("validated: []\nrejected: []\n"),
	} {
		r := reportFrom(rec, "v4")
		if r.State != ContractStateUnmeasured {
			t.Errorf("%s: reported %q, want unmeasured", name, r.State)
		}
		if r.Summary == "" {
			t.Errorf("%s: empty summary", name)
		}
		if state, _ := contractStateFrom(rec, "v4"); state != contractStateUnreadable {
			t.Errorf("%s: state %q, want unreadable", name, state)
		}
	}
	// A record WITH entries that merely lacks this version is still unrecorded.
	rec := []byte("validated:\n  - prompt_version: v4\n")
	if got := reportFrom(rec, "v99").State; got != ContractStateUnrecorded {
		t.Errorf("entries without v99: %q, want unrecorded", got)
	}
}

func TestContractReport_UnreadableIsUnmeasured(t *testing.T) {
	for _, garbage := range []string{"validated: [unterminated", "\x00\x01:::"} {
		r := reportFrom([]byte(garbage), "v4")
		if r.State != ContractStateUnmeasured {
			t.Errorf("garbage record %q reported %q, want unmeasured", garbage, r.State)
		}
		if state, _ := contractStateFrom([]byte(garbage), "v4"); state != contractStateUnreadable {
			t.Errorf("garbage record state %q, want unreadable", state)
		}
	}
	// And the real record still answers: v4 validated, v11 rejected.
	if got := ReportContract("v4").State; got != ContractStateValidated {
		t.Errorf("v4 = %q, want validated", got)
	}
	if got := ReportContract("v11").State; got != ContractStateRejected {
		t.Errorf("v11 = %q, want rejected", got)
	}
	if got := ReportContract("v99").State; got != ContractStateUnrecorded {
		t.Errorf("v99 = %q, want unrecorded", got)
	}
	if got := ReportContract("UNKNOWN").State; got != ContractStateUnmeasured {
		t.Errorf("UNKNOWN = %q, want unmeasured", got)
	}
}

// The project's extractor answers whatever the profile holds: a differing
// profile copy neither refuses nor changes the reported version.
func TestContractReport_ProjectWinsOverADifferentProfile(t *testing.T) {
	home := isolateHome(t)
	profileAgent := filepath.Join(home, ".claude", "agents", "sidecar-extractor.md")
	for _, tc := range []struct {
		name             string
		project, profile string
		wantVersion      string
		wantState        string
	}{
		{"project v4, profile v11", "prompt_version: 4\n", "prompt_version: 11\n", "v4", ContractStateValidated},
		{"project v11, profile v4", "prompt_version: 11\n", "prompt_version: 4\n", "v11", ContractStateRejected},
	} {
		root := t.TempDir()
		writeAgent(t, filepath.Join(root, ".claude", "agents", "sidecar-extractor.md"), tc.project)
		writeAgent(t, profileAgent, tc.profile)
		r := ReportInstalledContract(root)
		if r.PromptVersion != tc.wantVersion || r.State != tc.wantState {
			t.Errorf("%s: reported %s/%s, want %s/%s (%s)", tc.name, r.PromptVersion, r.State, tc.wantVersion, tc.wantState, r.Summary)
		}
		if strings.Contains(strings.ToLower(r.Summary), "different versions") || strings.Contains(strings.ToLower(r.Summary), "conflict") {
			t.Errorf("%s: summary still speaks of a conflict: %q", tc.name, r.Summary)
		}
	}
}
