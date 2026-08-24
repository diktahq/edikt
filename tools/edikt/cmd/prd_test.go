package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// These verbs mutate a governance artifact and append the audit record that
// says who changed it. The properties below are the ones a caller cannot check
// for itself: idempotence, the roll-up guard, and the fact that no path here
// writes without a history entry.

func runPRD(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := &cobra.Command{Use: "prd"}
	c.AddCommand(newPRDShipCmd(), newPRDCloseCmd(), newPRDLinkSpecCmd())
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func writePRD(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "prd.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readPRD(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := yaml.Unmarshal(b, &d); err != nil {
		t.Fatalf("result is not valid YAML: %v\n%s", err, b)
	}
	return d
}

// An EMPTY requirements list is not "all shipped". The python guarded this and
// the guard is the load-bearing half — without it a PRD with no requirements
// rolls up to shipped the first time anyone runs this.
func TestPRDShipDoesNotRollUpAnEmptyRequirementsList(t *testing.T) {
	p := writePRD(t, "id: PRD-1\nstatus: accepted\nrequirements: []\n")
	if _, err := runPRD(t, "ship", "--file", p, "--frs", "FR-001",
		"--author", "me", "--at", "2026-08-23T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if got := readPRD(t, p)["status"]; got != "accepted" {
		t.Errorf("status rolled up to %v on an empty requirements list", got)
	}
}

func TestPRDShipRollsUpOnlyWhenEveryRequirementShipped(t *testing.T) {
	p := writePRD(t, "id: PRD-1\nstatus: accepted\nrequirements:\n  - id: FR-001\n    status: draft\n  - id: FR-002\n    status: draft\n")
	out, err := runPRD(t, "ship", "--file", p, "--frs", "FR-001", "--author", "me", "--at", "2026-08-23T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "FR-001" {
		t.Errorf("affected = %q, want FR-001", strings.TrimSpace(out))
	}
	if got := readPRD(t, p)["status"]; got != "accepted" {
		t.Errorf("status rolled up to %v with one requirement still draft", got)
	}
	if _, err := runPRD(t, "ship", "--file", p, "--frs", "FR-002", "--author", "me", "--at", "2026-08-23T00:01:00Z"); err != nil {
		t.Fatal(err)
	}
	if got := readPRD(t, p)["status"]; got != "shipped" {
		t.Errorf("status = %v after every requirement shipped, want shipped", got)
	}
}

// Re-shipping an already-shipped requirement must not claim work again.
func TestPRDShipIsIdempotent(t *testing.T) {
	p := writePRD(t, "id: PRD-1\nstatus: accepted\nrequirements:\n  - id: FR-001\n    status: shipped\n")
	out, err := runPRD(t, "ship", "--file", p, "--frs", "FR-001", "--author", "me", "--at", "2026-08-23T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("affected = %q, want empty — the requirement was already shipped", strings.TrimSpace(out))
	}
}

// A repeat link must not fabricate a second linking event.
func TestPRDLinkSpecIsIdempotent(t *testing.T) {
	p := writePRD(t, "id: PRD-1\nstatus: accepted\n")
	for i := 0; i < 3; i++ {
		if _, err := runPRD(t, "link-spec", "--file", p, "--spec", "SPEC-015",
			"--author", "me", "--at", "2026-08-23T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	d := readPRD(t, p)
	specs, _ := d["source_specs"].([]any)
	if len(specs) != 1 {
		t.Errorf("source_specs = %v after three identical links", specs)
	}
	hist, _ := d["revision_history"].([]any)
	if len(hist) != 1 {
		t.Errorf("revision_history has %d entries after three identical links", len(hist))
	}
}

// close is one verb for two words, and both must produce the action the
// predecessor produced. A suffix-trimming implementation got "cancelled" right
// and "deprecated" wrong — the near-miss that survives review because the
// common case works.
func TestPRDCloseRecordsTheRightActionForBothStatuses(t *testing.T) {
	for _, c := range []struct{ status, action string }{
		{"cancelled", "cancel"},
		{"deprecated", "deprecate"},
	} {
		p := writePRD(t, "id: PRD-1\nstatus: accepted\n")
		if _, err := runPRD(t, "close", "--file", p, "--status", c.status, "--reason", "r",
			"--author", "me", "--at", "2026-08-23T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
		d := readPRD(t, p)
		if d["status"] != c.status {
			t.Errorf("status = %v, want %s", d["status"], c.status)
		}
		if d[c.status+"_reason"] != "r" {
			t.Errorf("%s_reason = %v", c.status, d[c.status+"_reason"])
		}
		hist, _ := d["revision_history"].([]any)
		last, _ := hist[len(hist)-1].(map[string]any)
		if last["action"] != c.action {
			t.Errorf("%s produced action %v, want %s", c.status, last["action"], c.action)
		}
	}
}

func TestPRDCloseRefusesABlankReasonAndAnUnknownStatus(t *testing.T) {
	p := writePRD(t, "id: PRD-1\nstatus: accepted\n")
	if _, err := runPRD(t, "close", "--file", p, "--status", "bogus", "--reason", "r",
		"--author", "me", "--at", "2026-08-23T00:00:00Z"); err == nil {
		t.Error("an unknown status was accepted")
	}
	if _, err := runPRD(t, "close", "--file", p, "--status", "cancelled", "--reason", "   ",
		"--author", "me", "--at", "2026-08-23T00:00:00Z"); err == nil {
		t.Error("a blank reason was accepted — a PRD closed without one cannot be understood later")
	}
}

// EVERY mutating path writes a history entry and clears _sync. That is the
// property a generic "set this YAML path" verb would have destroyed.
func TestPRDEveryMutationIsAuditedAndDesyncs(t *testing.T) {
	cases := [][]string{
		{"ship", "--frs", "FR-001"},
		{"close", "--status", "cancelled", "--reason", "r"},
		{"link-spec", "--spec", "SPEC-1"},
	}
	for _, extra := range cases {
		p := writePRD(t, "id: PRD-1\nstatus: accepted\nrequirements:\n  - id: FR-001\n    status: draft\n_sync:\n  md_hash: \"abc\"\n  yaml_hash: \"def\"\n  synced_at: \"t\"\n")
		args := append([]string{extra[0], "--file", p}, extra[1:]...)
		args = append(args, "--author", "me", "--at", "2026-08-23T00:00:00Z")
		if _, err := runPRD(t, args...); err != nil {
			t.Fatalf("%s: %v", extra[0], err)
		}
		d := readPRD(t, p)
		hist, _ := d["revision_history"].([]any)
		if len(hist) == 0 {
			t.Errorf("%s mutated the sidecar without recording who did it", extra[0])
		}
		sync, _ := d["_sync"].(map[string]any)
		for _, k := range []string{"md_hash", "yaml_hash", "synced_at"} {
			if sync[k] != "" {
				t.Errorf("%s left _sync.%s = %v — it asserts an agreement this command cannot see",
					extra[0], k, sync[k])
			}
		}
	}
}

// Comments survive. The python used safe_dump and destroyed every one.
func TestPRDPreservesComments(t *testing.T) {
	p := writePRD(t, "# load-bearing comment\nid: PRD-1\nstatus: accepted\n")
	if _, err := runPRD(t, "link-spec", "--file", p, "--spec", "SPEC-1",
		"--author", "me", "--at", "2026-08-23T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "# load-bearing comment") {
		t.Errorf("the comment was destroyed:\n%s", b)
	}
}
