package cmd

// Red-first record (PLAN-v091 phase 1, AC-1.11, 2026-10-10). Mutation made to
// see TestContractState_ProjectWinsOverADifferentProfile go red: restore the
// refusal for two usable extractors that pin different versions (resolver
// returns an error when the project's and the profile's versions differ) ->
// red on both orderings (doctor exits 11, plan refuses, status unmeasured);
// let the version lookup answer from the profile when the two differ -> red
// (project v11 + profile v4 exits 0, "v4 is validated").

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractState_ProjectWinsOverADifferentProfile(t *testing.T) {
	for _, tc := range []struct {
		name             string
		project, profile int
		wantCode         int
		wantVersion      string
		wantState        string
		wantLine         string
	}{
		{"project v11, profile v4", 11, 4, 10, "v11", "rejected", "extraction contract v11 is REJECTED"},
		{"project v4, profile v11", 4, 11, 0, "v4", "validated", "extraction contract v4 is validated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, root := t.TempDir(), t.TempDir()
			bbWriteConfig(t, root, "")
			adr := bbWriteADR(t, root, "ADR-alpha", gateQuoteBB)
			writeExtractorAt(t, filepath.Join(root, ".claude"), tc.project)
			writeExtractorAt(t, filepath.Join(home, ".claude"), tc.profile)
			cold := gateEnv(home) // no invoker: doctor's report must not need one
			inv := gateEnv(home, "EDIKT_INVOKER=ci", "EDIKT_SKIP_VERSION_GATE=1")
			all := ""

			out, errOut, code := runGated(t, root, cold, "doctor", "--contract-state")
			all += out + errOut
			if code != tc.wantCode || !strings.Contains(out, tc.wantLine) || strings.Count(strings.TrimSpace(out), "\n") != 0 {
				t.Errorf("doctor --contract-state: exit %d %q (stderr %s), want exit %d with %q", code, out, errOut, tc.wantCode, tc.wantLine)
			}

			sout, serr, scode := runGated(t, root, inv, "gov", "reextract", "--status", "--json")
			all += sout + serr
			var m map[string]any
			if scode != 0 || json.Unmarshal([]byte(sout), &m) != nil {
				t.Fatalf("gov reextract --status --json: exit %d\n%s%s", scode, sout, serr)
			}
			if m["contract_state"] != tc.wantState || m["prompt_version"] != tc.wantVersion {
				t.Errorf("status reports state=%v version=%v, want %s / %s\n%s", m["contract_state"], m["prompt_version"], tc.wantState, tc.wantVersion, sout)
			}

			rel := filepath.ToSlash(filepath.Join("docs", "architecture", "decisions", filepath.Base(adr)))
			pout, perr, pcode := runGated(t, root, inv, "phasea", "plan", "--artifact", rel, "--bootstrap")
			all += pout + perr
			if pcode != 0 {
				t.Fatalf("phasea plan: exit %d, want 0 (the project's extractor is usable)\nstdout: %s\nstderr: %s", pcode, pout, perr)
			}
			other := map[string]string{"v4": "v11", "v11": "v4"}[tc.wantVersion]
			for _, want := range []string{
				filepath.Join(".claude", "agents", "sidecar-extractor.md") + " (project); the profile's copy",
				"ignored because the project has its own",
				"pins " + other + ", which differs from the project's " + tc.wantVersion + " (information only; nothing is refused)",
			} {
				if !strings.Contains(pout, want) {
					t.Errorf("phasea plan's selection note lacks %q:\n%s", want, pout)
				}
			}
			plans, _ := filepath.Glob(filepath.Join(root, ".edikt", "state", "phasea", "*.plan.json"))
			if len(plans) != 1 {
				t.Fatalf("want one recorded plan, found %v", plans)
			}
			raw, err := os.ReadFile(plans[0])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"prompt_version": "`+tc.wantVersion+`"`) && !strings.Contains(string(raw), `"prompt_version":"`+tc.wantVersion+`"`) {
				t.Errorf("the recorded plan does not carry the project's prompt version %s:\n%s", tc.wantVersion, raw)
			}
			all += string(raw)
			for _, bad := range []string{"conflict", "different versions"} {
				if strings.Contains(strings.ToLower(all), bad) {
					t.Errorf("output mentions %q:\n%s", bad, all)
				}
			}
		})
	}
}
