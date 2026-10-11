package cmd

// Red-first record (PLAN-v091 phase 1, second review, 2026-10-10). The
// mutation made to see each test in this file go red:
//
//	EmptyAgentFileIsNotTheInstalledAgent: tested `doctorAgentFile != ""`
//	  instead of Flags().Changed("agent-file") -> red (v11 reported, exit 10;
//	  exit 0 path runs full doctor instead of exit 3).
//	NonIntegerVersionIsUnmeasured: decoded prompt_version into an int field
//	  again -> red for 4.9, 4e0, 011, 1_1, 11.5 and the quoted "11"; changed
//	  `ver <= 0` to `ver < 0` in ReportAgentFileContract -> red (no-version
//	  file reports "v0 ... no entry", exit 10).
//	NamedFileSummaryIsConsumerSafe: echoed the parse error (absolute path
//	  included) into the summary for a named file -> red (a "/" segment);
//	  returned reasonRecord instead of reasonNoVersion for a bad shape -> red
//	  (exact-reason table).
//	  (v0.9.1 remedies) drop "/edikt:upgrade again" from the unreadable
//	  reason, or reuse the unreadable reason for a named file with no version
//	  -> red (both lines are pinned in full).
//	  (v0.9.1 remedies) drop "/edikt:upgrade again" from the unreadable
//	  reason, or reuse the unreadable reason for a named file with no version
//	  -> red (both lines are pinned in full).
//	OutsideAnyProject: returned an error instead of the working-directory
//	  fallback when no project is found -> red (exit 1, not 11).
//	StatusJSONFromSubdirectory: made the --status branch resolve "." instead
//	  of reextractProjectRoot -> red (prompt_version UNKNOWN from docs/).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	wantUnreadLine  = "extraction contract state is UNMEASURED: the named agent file could not be read; check the path given to --agent-file, and reinstall edikt if this version's template is missing"
	wantNoVerLine   = "extraction contract state is UNMEASURED: the named agent file declares no usable prompt_version; check the path given to --agent-file, and reinstall edikt if this version's template is missing"
	templateAgentFN = "sidecar-extractor.md"
)

func agentBody(versionLine string) string {
	body := "---\nname: sidecar-extractor\n"
	if versionLine != "" {
		body += versionLine + "\n"
	}
	return body + "---\n\nbody\n"
}

func writeNamedAgent(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), templateAgentFN)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestContractState_EmptyAgentFileIsNotTheInstalledAgent(t *testing.T) {
	p := csProject(t, 11)
	out, errOut, code := p.run(t, "doctor", "--contract-state", "--agent-file", "")
	if code != 11 {
		t.Fatalf("--agent-file \"\": exit %d, want 11 (unmeasured)\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	if strings.Contains(out, "v11") || !strings.Contains(out, "UNMEASURED") {
		t.Errorf("an empty --agent-file reported the installed agent: %q", out)
	}
	_, errOut, code = p.run(t, "doctor", "--agent-file", "")
	if code != 3 {
		t.Errorf("--agent-file \"\" without --contract-state: exit %d, want 3\nstderr: %s", code, errOut)
	}
}

func TestContractState_NonIntegerVersionIsUnmeasured(t *testing.T) {
	p := csProject(t, 11)
	for _, tc := range []struct{ name, line string }{
		{"float 4.9", "prompt_version: 4.9"},
		{"exponent 4e0", "prompt_version: 4e0"},
		{"octal 011", "prompt_version: 011"},
		{"underscore 1_1", "prompt_version: 1_1"},
		{"float 11.5", "prompt_version: 11.5"},
		{"quoted string", `prompt_version: "11"`},
		{"zero", "prompt_version: 0"},
		{"negative", "prompt_version: -4"},
		{"no prompt_version", ""},
	} {
		f := writeNamedAgent(t, agentBody(tc.line))
		out, errOut, code := p.run(t, "doctor", "--contract-state", "--agent-file", f)
		if code != 11 || strings.TrimSpace(out) != wantNoVerLine {
			t.Errorf("%s: exit %d, want 11 with %q\nstdout: %s\nstderr: %s", tc.name, code, wantNoVerLine, out, errOut)
		}
		if strings.Contains(out, "validated") || strings.Contains(out, "v4") || strings.Contains(out, "v11") || strings.Contains(out, "v0") {
			t.Errorf("%s: a version leaked into the line: %q", tc.name, out)
		}
	}
	// A plain integer still resolves.
	f := writeNamedAgent(t, agentBody("prompt_version: 4"))
	if out, _, code := p.run(t, "doctor", "--contract-state", "--agent-file", f); code != 0 || !strings.Contains(out, "v4 is validated") {
		t.Errorf("plain 4: exit %d %q, want validated", code, out)
	}
	// The installed-agent path reads through the same reader.
	for _, line := range []string{"prompt_version: 4.9", "prompt_version: 011"} {
		ip := &bbProject{root: t.TempDir(), home: t.TempDir()}
		bbWriteConfig(t, ip.root, "")
		dir := filepath.Join(ip.root, ".claude", "agents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, templateAgentFN), []byte(agentBody(line)), 0o644); err != nil {
			t.Fatal(err)
		}
		out, errOut, code := ip.run(t, "doctor", "--contract-state")
		if code != 11 || strings.Contains(out, "validated") || strings.Contains(out, "v4") || strings.Contains(out, "v9") {
			t.Errorf("installed %s: exit %d %q (stderr %s), want unmeasured 11", line, code, out, errOut)
		}
	}
}

func TestContractState_NamedFileSummaryIsConsumerSafe(t *testing.T) {
	p := csProject(t, 11)
	dirAsFile := filepath.Join(t.TempDir(), templateAgentFN)
	if err := os.Mkdir(dirAsFile, 0o755); err != nil {
		t.Fatal(err)
	}
	wrong := filepath.Join(t.TempDir(), "other.md")
	if err := os.WriteFile(wrong, []byte(agentBody("prompt_version: 4")), 0o644); err != nil {
		t.Fatal(err)
	}
	unreadable := writeNamedAgent(t, agentBody("prompt_version: 4"))
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	for _, tc := range []struct{ name, path, want string }{
		{"missing", filepath.Join(t.TempDir(), templateAgentFN), wantUnreadLine},
		{"directory", dirAsFile, wantUnreadLine},
		{"wrong basename", wrong, wantUnreadLine},
		{"unreadable", unreadable, wantUnreadLine},
		{"no frontmatter fence", writeNamedAgent(t, "no fence here\n"), wantNoVerLine},
		{"unterminated fence", writeNamedAgent(t, "---\nprompt_version: 4\n"), wantNoVerLine},
		{"malformed yaml", writeNamedAgent(t, "---\nprompt_version: [unterminated\n---\n"), wantNoVerLine},
	} {
		out, errOut, code := p.run(t, "doctor", "--contract-state", "--agent-file", tc.path)
		if code != 11 {
			t.Errorf("%s: exit %d, want 11\nstdout: %s\nstderr: %s", tc.name, code, out, errOut)
		}
		line := strings.TrimRight(out, "\n")
		if line != tc.want {
			t.Errorf("%s: summary %q, want exactly %q", tc.name, line, tc.want)
		}
		if strings.Contains(line, "\n") || strings.Contains(strings.ReplaceAll(line, "/edikt:upgrade", ""), "/") || strings.Contains(line, "UNKNOWN") {
			t.Errorf("%s: summary is not one consumer-safe line: %q", tc.name, line)
		}
	}
}

func TestContractState_OutsideAnyProject(t *testing.T) {
	home := t.TempDir()
	out, errOut, code := csRun(t, t.TempDir(), home, "doctor", "--contract-state")
	if code != 11 || !strings.Contains(out, "UNMEASURED") {
		t.Fatalf("outside any project: exit %d, want 11 UNMEASURED\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestReextractStatusJSON_FromSubdirectoryReportsTheProjectVersion(t *testing.T) {
	p := csProject(t, 11)
	sub := filepath.Join(p.root, "docs", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := csRun(t, sub, p.home, "gov", "reextract", "--status", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if m["prompt_version"] != "v11" || m["contract_state"] != "rejected" {
		t.Errorf("from a subdirectory: prompt_version %v, contract_state %v; want v11 / rejected", m["prompt_version"], m["contract_state"])
	}
}
