package gov

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
)

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	fn()
	os.Stdout = orig
	w.Close()
	return <-done
}

// #9 (c4-b): repairable drift is reported in the JSON a caller parses, as a
// count and as ids. repairable_ids is ALWAYS an array — [] is a measured
// "none repairable", and a missing key or null would be indistinguishable
// from a binary that does not report it at all.
func TestEmitTwoPhaseJSON_RepairableIDsAlwaysArray(t *testing.T) {
	type phaseA struct {
		Stale         int              `json:"stale"`
		StaleIDs      []string         `json:"stale_ids"`
		Repairable    *int             `json:"repairable"`
		RepairableIDs *json.RawMessage `json:"repairable_ids"`
	}
	type doc struct {
		PhaseA phaseA `json:"phase_a"`
	}
	decode := func(b []byte) doc {
		t.Helper()
		var d doc
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, b)
		}
		return d
	}

	for name, res := range map[string]*govrun.TwoPhaseResult{
		"nil result":   nil,
		"nothing open": {},
	} {
		d := decode(captureStdout(t, func() { emitTwoPhaseJSON(res, nil) }))
		if d.PhaseA.RepairableIDs == nil || strings.TrimSpace(string(*d.PhaseA.RepairableIDs)) != "[]" {
			t.Errorf("%s: phase_a.repairable_ids must be [] (got %v)", name, d.PhaseA.RepairableIDs)
		}
		if d.PhaseA.Repairable == nil || *d.PhaseA.Repairable != 0 {
			t.Errorf("%s: phase_a.repairable must be present and 0", name)
		}
	}

	res := &govrun.TwoPhaseResult{
		StaleSidecars:      []string{"ADR-090", "ADR-091", "INV-090"},
		RepairableSidecars: []string{"ADR-090", "INV-090"},
	}
	d := decode(captureStdout(t, func() { emitTwoPhaseJSON(res, nil) }))
	var ids []string
	if d.PhaseA.RepairableIDs == nil {
		t.Fatal("phase_a.repairable_ids missing")
	}
	if err := json.Unmarshal(*d.PhaseA.RepairableIDs, &ids); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "ADR-090,INV-090" || *d.PhaseA.Repairable != 2 {
		t.Errorf("repairable = %d %v, want 2 [ADR-090 INV-090]", *d.PhaseA.Repairable, ids)
	}
	if d.PhaseA.Stale != 3 {
		t.Errorf("repairable ids are a subset of stale, not a replacement: stale=%d", d.PhaseA.Stale)
	}
}

// D-L (#9, c4-e): `--trust` records the project in the USER-GLOBAL trust
// store. `--check` / `--dry-run` promise to write nothing, and the trust
// store is a write — so the combination is refused before anything is
// recorded, exit 2, naming both flags.
func TestGovCompile_TrustWithCheckRefused(t *testing.T) {
	bin := buildBinary(t)
	for _, flag := range []string{"--check", "--dry-run"} {
		t.Run(flag, func(t *testing.T) {
			home := t.TempDir()
			// A project with a sidecar, so a run that ignored the refusal
			// would reach the trust gate rather than stop at "nothing here".
			project := t.TempDir()
			adrDir := filepath.Join(project, "docs", "architecture", "decisions")
			if err := os.MkdirAll(adrDir, 0o755); err != nil {
				t.Fatal(err)
			}
			md := "---\nstatus: accepted\n---\n\n# ADR-080 — fresh\n\n## Decision\n\nFresh rule MUST hold.\n"
			sc := "schema_version: 2\ntopic: \"testing\"\npath: \"docs/architecture/decisions/ADR-080.md\"\nsignals:\n  - \"fresh\"\ndirectives:\n  - text: \"Fresh rule MUST hold. (ref: ADR-080)\"\n    source_excerpts:\n      - line_start: 9\n        line_end: 9\n        quote: \"Fresh rule MUST hold.\"\n"
			if err := os.WriteFile(filepath.Join(adrDir, "ADR-080.md"), []byte(md), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(adrDir, "ADR-080.edikt.yaml"), []byte(sc), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "gov", "compile", flag, "--trust", project)
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"EDIKT_HOME="+filepath.Join(home, ".edikt"),
				"CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"),
				"CLAUDE_HOME="+filepath.Join(home, ".claude"),
				"EDIKT_INVOKER=ci",
				"EDIKT_SKIP_VERSION_GATE=1",
				"EDIKT_VERIFY_TRUST=",
			)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			err := cmd.Run()
			if !isExitCode(err, 2) {
				t.Fatalf("gov compile %s --trust must exit 2; err=%v\n%s", flag, err, buf.String())
			}
			out := buf.String()
			if !strings.Contains(out, "--trust") || !strings.Contains(out, flag) {
				t.Errorf("the refusal must name both flags:\n%s", out)
			}
			// The trust store (ADR-041: $EDIKT_HOME/state/trusted-roots.json).
			// Other user-home writes — the walled-call journal — are not
			// trust and are not what --check promises about.
			store := filepath.Join(home, ".edikt", "state", "trusted-roots.json")
			if _, serr := os.Stat(store); serr == nil {
				b, _ := os.ReadFile(store)
				t.Errorf("a refused --trust must record nothing; the trust store exists:\n%s", b)
			}
		})
	}
}
