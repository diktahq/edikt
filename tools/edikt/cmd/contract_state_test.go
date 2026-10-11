package cmd

// Red-first record (PLAN-v091 phase 1, corrected 2026-10-10 after review).
// One-line mutation made to see each test in this file go red. (The gate,
// write and subdirectory-vs-phasea tests live in contract_state_gates_test.go.)
//
//	SurfacesAgree: made unmeasured() return an empty Summary -> red
//	  (non-empty assertion); made phasea.go print v99 as validated -> red; made
//	  Status() derive ContractState from the boolean (rejected/unrecorded
//	  collapse) -> red for v11 against the report.
//	ResolvesFromProjectRootInSubdirectory: made runContractState resolve from
//	  the working directory instead of projectroot.FindFromCwd -> red.
//	AgentFileReportsTheTemplate: made runContractState ignore agentFile -> red;
//	  made the usage check drop "--agent-file without --contract-state" -> red.
//	UnresolvableVersionExits11: made ReportInstalledContract return validated
//	  on a resolver error -> red.
//	TestReextractStatusJSON_CarriesContractState: dropped ContractState from
//	  Status() -> red.
//	Final pass (whole-plan review):
//	SurfacesAgree/unmeasured: made Status() return the generic "could not be
//	  resolved" reason for every unmeasured state -> red (status != doctor);
//	  made phasea.Plan drop the ContractLead prefix -> red (plan first line).
//	ResolvesFromProjectRootInSubdirectory (project v11 + profile v11): made the
//	  resolver treat any profile file beside a project file as a conflict -> red.
//	Project always wins (v0.9.1 ruling): the "conflict" unmeasured kind is gone
//	  (it is now a validated/rejected answer from the project, tested in
//	  contract_state_project_wins_test.go); in the subdirectory test, project
//	  v11 + profile v4 now answers v11 rejected (10) from root and subdirectory
//	  alike. Made the resolver answer from the profile when the two differ
//	  -> red (exit 0, v4).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func csRun(t *testing.T, dir, home string, args ...string) (string, string, int) {
	t.Helper()
	return runSandboxed(t, dir, home, args...)
}

func csProject(t *testing.T, version int) *bbProject {
	t.Helper()
	p := &bbProject{root: t.TempDir(), home: t.TempDir()}
	bbWriteExtractorAgent(t, p.root, version)
	bbWriteConfig(t, p.root, "")
	p.adr = bbWriteADR(t, p.root, "ADR-900", gateQuoteBB)
	return p
}

func TestContractState_UnresolvableVersionExits11(t *testing.T) {
	p := &bbProject{root: t.TempDir(), home: t.TempDir()}
	bbWriteConfig(t, p.root, "")
	out, errOut, code := p.run(t, "doctor", "--contract-state")
	if code != 11 {
		t.Fatalf("no installed extractor: exit %d, want 11\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	if !strings.Contains(out, "UNMEASURED") || strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Errorf("want one UNMEASURED line on stdout, got %q", out)
	}
}

func TestContractState_AgentFileReportsTheTemplate(t *testing.T) {
	write := func(dir string, v int) string {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(dir, "sidecar-extractor.md")
		body := "---\nname: sidecar-extractor\nprompt_version: " + strconv.Itoa(v) + "\n---\n\nbody\n"
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	// installed v4, template v11 -> rejected, 10
	p := csProject(t, 4)
	tpl := write(filepath.Join(t.TempDir(), "tpl"), 11)
	out, errOut, code := p.run(t, "doctor", "--contract-state", "--agent-file", tpl)
	if code != 10 || !strings.Contains(out, "v11") || !strings.Contains(out, "REJECTED") {
		t.Fatalf("installed v4 + template v11: exit %d, want 10 rejected v11\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	// installed v11, template v4 -> validated, 0
	p = csProject(t, 11)
	tpl = write(filepath.Join(t.TempDir(), "tpl"), 4)
	out, errOut, code = p.run(t, "doctor", "--contract-state", "--agent-file", tpl)
	if code != 0 || !strings.Contains(out, "v4") || !strings.Contains(out, "is validated") {
		t.Fatalf("installed v11 + template v4: exit %d, want 0 validated v4\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	// wrong basename, missing file -> unmeasured, 11
	wrong := filepath.Join(t.TempDir(), "other.md")
	if err := os.WriteFile(wrong, []byte("---\nprompt_version: 4\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{wrong, filepath.Join(t.TempDir(), "sidecar-extractor.md")} {
		out, errOut, code = p.run(t, "doctor", "--contract-state", "--agent-file", f)
		if code != 11 || !strings.Contains(out, "UNMEASURED") {
			t.Errorf("--agent-file %s: exit %d, want 11 UNMEASURED\nstdout: %s\nstderr: %s", f, code, out, errOut)
		}
	}
	// --agent-file without --contract-state is a usage error
	_, errOut, code = p.run(t, "doctor", "--agent-file", tpl)
	if code != 3 {
		t.Errorf("--agent-file alone: exit %d, want 3\nstderr: %s", code, errOut)
	}
	// the preview wrote nothing
	if _, err := os.Stat(filepath.Join(p.root, ".edikt", "state", "reextract-ledger.json")); err == nil {
		t.Errorf("--agent-file preview wrote a ledger")
	}
}

// stateOfSummary reads the state a Summary line names: a second, independent
// reader, so the test compares two surfaces to each other, not each to a
// constant.
func stateOfSummary(t *testing.T, summary string) string {
	t.Helper()
	switch {
	case strings.Contains(summary, "UNMEASURED"):
		return "unmeasured"
	case strings.Contains(summary, "REJECTED"):
		return "rejected"
	case strings.Contains(summary, "has no entry"):
		return "unrecorded"
	case strings.Contains(summary, "is validated"):
		return "validated"
	}
	t.Fatalf("summary names no state: %q", summary)
	return ""
}

func exitForState(state string) int {
	switch state {
	case "validated":
		return 0
	case "unmeasured":
		return 11
	}
	return 10
}

func TestContractState_SurfacesAgree(t *testing.T) {
	for _, tc := range []struct {
		version   int
		wantState string
	}{{4, "validated"}, {11, "rejected"}, {99, "unrecorded"}, {0, "unmeasured"}} {
		var p *bbProject
		if tc.version == 0 {
			p = &bbProject{root: t.TempDir(), home: t.TempDir()}
			bbWriteConfig(t, p.root, "")
			p.adr = bbWriteADR(t, p.root, "ADR-900", gateQuoteBB)
		} else {
			p = csProject(t, tc.version)
		}
		out, errOut, code := p.run(t, "doctor", "--contract-state")
		summary := strings.TrimSpace(out)
		if summary == "" || strings.Contains(summary, "\n") {
			t.Fatalf("v%d: summary must be one non-empty line, got %q (stderr %q)", tc.version, out, errOut)
		}
		reportState := stateOfSummary(t, summary)
		if code != exitForState(reportState) {
			t.Fatalf("v%d: exit %d does not match the state its own summary names (%s)\n%s", tc.version, code, reportState, out)
		}
		if reportState != tc.wantState {
			t.Errorf("v%d: report state %s, want %s", tc.version, reportState, tc.wantState)
		}

		jout, jerr, jcode := p.run(t, "gov", "reextract", "--status", "--json")
		if jcode != 0 {
			t.Fatalf("v%d --status --json exit %d\n%s%s", tc.version, jcode, jout, jerr)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(jout), &m); err != nil {
			t.Fatalf("v%d status json: %v\n%s", tc.version, err, jout)
		}
		if m["contract_state"] != reportState {
			t.Errorf("v%d: --status --json says %v, the report says %s", tc.version, m["contract_state"], reportState)
		}

		if reportState == "unmeasured" {
			continue // phasea plan refuses outright with no resolvable version
		}
		rel := filepath.ToSlash(filepath.Join("docs", "architecture", "decisions", filepath.Base(p.adr)))
		_, planErr, pcode := p.run(t, "phasea", "plan", "--artifact", rel, "--bootstrap")
		if pcode != 0 {
			t.Fatalf("v%d phasea plan exit %d\n%s", tc.version, pcode, planErr)
		}
		switch reportState {
		case "validated":
			if strings.Contains(planErr, "extraction contract") {
				t.Errorf("v%d: validated must be silent on phasea plan:\n%s", tc.version, planErr)
			}
		case "rejected", "unrecorded":
			if !strings.Contains(planErr, summary) {
				t.Errorf("v%d: phasea plan stderr does not carry the report's summary\nsummary: %s\nstderr: %s", tc.version, summary, planErr)
			}
		}
	}
}

// unmeasuredProject builds a project in one unmeasured state.
func unmeasuredProject(t *testing.T, kind string) *bbProject {
	t.Helper()
	p := &bbProject{root: t.TempDir(), home: t.TempDir()}
	bbWriteConfig(t, p.root, "")
	p.adr = bbWriteADR(t, p.root, "ADR-alpha", gateQuoteBB)
	profile := filepath.Join(p.home, ".claude")
	switch kind {
	case "no extractor":
	case "project unusable beside a pinned profile":
		writeExtractorAt(t, filepath.Join(p.root, ".claude"), 0)
		writeExtractorAt(t, profile, 4)
	case "sole definition without a version":
		writeExtractorAt(t, filepath.Join(p.root, ".claude"), 0)
	default:
		t.Fatalf("unknown kind %q", kind)
	}
	return p
}

var unmeasuredKinds = []string{"no extractor", "project unusable beside a pinned profile", "sole definition without a version"}

// planRefusal runs the plan step the way compile does and returns its stderr.
func planRefusal(t *testing.T, p *bbProject) (string, int) {
	t.Helper()
	rel := filepath.ToSlash(filepath.Join("docs", "architecture", "decisions", filepath.Base(p.adr)))
	_, errOut, code := p.run(t, "phasea", "plan", "--artifact", rel, "--bootstrap")
	return errOut, code
}

// Every unmeasured state: the line doctor prints, the reason gov reextract
// --status reports, the line the plan step's refusal leads with, and the
// contract line under doctor's compile remedy are the SAME string.
func TestContractState_SurfacesAgreeWhenUnmeasured(t *testing.T) {
	for _, kind := range unmeasuredKinds {
		t.Run(kind, func(t *testing.T) {
			p := unmeasuredProject(t, kind)
			out, errOut, code := p.run(t, "doctor", "--contract-state")
			summary := strings.TrimSpace(out)
			if code != 11 || summary == "" || strings.Contains(summary, "\n") || !strings.Contains(summary, "UNMEASURED") {
				t.Fatalf("doctor --contract-state: exit %d %q (%s), want one UNMEASURED line, exit 11", code, out, errOut)
			}
			jout, jerr, jcode := p.run(t, "gov", "reextract", "--status", "--json")
			var m map[string]any
			if jcode != 0 || json.Unmarshal([]byte(jout), &m) != nil {
				t.Fatalf("status json: exit %d\n%s%s", jcode, jout, jerr)
			}
			if reason, _ := m["dispatch_blocked_reason"].(string); reason != summary {
				t.Errorf("gov reextract --status reason differs from doctor's line\nstatus: %q\ndoctor: %q", reason, summary)
			}
			perr, pcode := planRefusal(t, p)
			if pcode != 4 {
				t.Errorf("phasea plan: exit %d, want 4", pcode)
			}
			assertSummaryThenDetail(t, perr, summary, "refusing to emit a plan: ")
			dout, _, _ := p.run(t, "doctor")
			if !strings.Contains(dout, "\n"+contractIndent+summary+"\n") {
				t.Errorf("doctor's contract: line differs from --contract-state's:\n%s", dout)
			}
		})
	}
}

func TestContractState_ResolvesFromProjectRootInSubdirectory(t *testing.T) {
	// (a) the project's own agent is found from a subdirectory.
	p := csProject(t, 11)
	sub := filepath.Join(p.root, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := csRun(t, sub, p.home, "doctor", "--contract-state")
	if code != 10 || !strings.Contains(out, "v11") {
		t.Fatalf("from docs/: exit %d, want the project agent's v11 rejected (10)\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	// (a2) project v11 + profile v11: the same version, so the profile copy
	// agrees and the answer is v11 rejected from the subdirectory too.
	same := csProject(t, 11)
	writeExtractorAt(t, filepath.Join(same.home, ".claude"), 11)
	sameSub := filepath.Join(same.root, "docs")
	if err := os.MkdirAll(sameSub, 0o755); err != nil {
		t.Fatal(err)
	}
	sOut, sErr, sCode := csRun(t, sameSub, same.home, "doctor", "--contract-state")
	if sCode != 10 || !strings.Contains(sOut, "v11") {
		t.Fatalf("project v11 + profile v11 from docs/: exit %d, want v11 rejected (10)\nstdout: %s\nstderr: %s", sCode, sOut, sErr)
	}
	// (b) a DIFFERENT profile agent. The project's wins: resolved from the
	// project root the answer is the project's v11, rejected (10). Resolved
	// from the working directory, only the profile agent would be seen: v4,
	// validated, exit 0 - the wrong contract, reported clean.
	profile := filepath.Join(p.home, ".claude", "agents")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "sidecar-extractor.md"), []byte("---\nprompt_version: 4\n---\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rootOut, _, rootCode := csRun(t, p.root, p.home, "doctor", "--contract-state")
	subOut, subErr, subCode := csRun(t, sub, p.home, "doctor", "--contract-state")
	if rootCode != 10 || !strings.Contains(rootOut, "v11") {
		t.Fatalf("precondition: from the root, project v11 + profile v4 should be the project's v11 rejected (10), got %d %q", rootCode, rootOut)
	}
	if subOut != rootOut || subCode != rootCode {
		t.Fatalf("from docs/ the answer differs from the root's (the profile agent leaked in)\nroot: %d %q\nsub:  %d %q (%s)",
			rootCode, rootOut, subCode, subOut, subErr)
	}
}

func TestReextractStatusJSON_CarriesContractState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int
		want    string
	}{{"v4", 4, "validated"}, {"v11", 11, "rejected"}, {"v99", 99, "unrecorded"}, {"unknown", 0, "unmeasured"}} {
		p := &bbProject{root: t.TempDir(), home: t.TempDir()}
		bbWriteConfig(t, p.root, "")
		if tc.version > 0 {
			bbWriteExtractorAgent(t, p.root, tc.version)
		}
		out, errOut, code := p.run(t, "gov", "reextract", "--status", "--json")
		if code != 0 {
			t.Fatalf("%s: exit %d\n%s%s", tc.name, code, out, errOut)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("%s: %v\n%s", tc.name, err, out)
		}
		st, ok := m["contract_state"].(string)
		if !ok || st != tc.want {
			t.Errorf("%s: contract_state = %v, want %s", tc.name, m["contract_state"], tc.want)
		}
		if m["contract_validated"] != (st == "validated") {
			t.Errorf("%s: contract_validated = %v disagrees with contract_state %q", tc.name, m["contract_validated"], st)
		}
		if st == "unmeasured" {
			reason, _ := m["dispatch_blocked_reason"].(string)
			if m["contract_validated"] != false {
				t.Errorf("%s: unmeasured but contract_validated = %v", tc.name, m["contract_validated"])
			}
			if strings.Contains(reason, "has no entry") || strings.Contains(reason, "--skip-fixture-proof") || !strings.Contains(reason, "UNMEASURED") {
				t.Errorf("%s: unmeasured state beside a contradicting reason: %q", tc.name, reason)
			}
		}
	}
}
