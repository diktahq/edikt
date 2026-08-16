package hookcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestFloor_EnforcementCannotBeDisabled — the ATTEMPT must fail, not warn. A
// validator that warned would leave the value in force, which is the bypass
// flag under another name.
func TestFloor_EnforcementCannotBeDisabled(t *testing.T) {
	root := write(t, "hooks:\n  enforcement:\n    inject-directives-pre:\n      enabled: false\n")
	_, err := Load(root)
	if err == nil {
		t.Fatal("configuring an enforcement hook to enabled:false was ACCEPTED — 'never enforce' must be unrepresentable")
	}
	if !strings.Contains(err.Error(), "floor") {
		t.Errorf("refusal does not explain the floor: %v", err)
	}
}

// TestFloor_BounceBudgetZeroRefused — zero is not a budget.
func TestFloor_BounceBudgetZeroRefused(t *testing.T) {
	root := write(t, "hooks:\n  injection:\n    bounce_budget: 0\n")
	if _, err := Load(root); err == nil {
		t.Fatal("bounce_budget: 0 was accepted — that is enforcement switched off wearing a number")
	}
}

// TestFloor_SessionDedupScopeRefused — the measured-wrong value cannot be
// configured back in.
func TestFloor_SessionDedupScopeRefused(t *testing.T) {
	root := write(t, "hooks:\n  injection:\n    dedup_scope: session\n")
	_, err := Load(root)
	if err == nil {
		t.Fatal("dedup_scope: session was accepted — it suppresses every subagent injection after a parent bounce")
	}
	if !strings.Contains(err.Error(), "F-020") {
		t.Errorf("refusal does not cite the measurement: %v", err)
	}
}

// TestErgonomicsMayBeDisabled — the distinction is the whole ruling. If
// ergonomics could not be turned off, the floor would be meaningless because
// everything would be floored.
func TestErgonomicsMayBeDisabled(t *testing.T) {
	root := write(t, "hooks:\n  ergonomics:\n    status-line:\n      enabled: false\n")
	c, err := Load(root)
	if err != nil {
		t.Fatalf("disabling an ergonomics hook was refused: %v", err)
	}
	if c.StatusOf("status-line") != StatusDisabled {
		t.Fatalf("status-line reads %q; want disabled", c.StatusOf("status-line"))
	}
}

// TestDisabledIsDistinguishableFromMissing — read BOTH and assert they differ.
// If "off" looked identical to "never configured" we would have relocated the
// invisible fiction rather than removed it.
func TestDisabledIsDistinguishableFromMissing(t *testing.T) {
	root := write(t, "hooks:\n  ergonomics:\n    status-line:\n      enabled: false\n")
	c, _ := Load(root)
	disabled := c.StatusOf("status-line")
	missing := c.StatusOf("a-hook-nobody-configured")
	if disabled == missing {
		t.Fatalf("disabled and unconfigured both read %q — they must be distinguishable", disabled)
	}
	if disabled != StatusDisabled || missing != StatusUnconfigured {
		t.Fatalf("got disabled=%q missing=%q", disabled, missing)
	}
}

// TestMissingConfigIsNotAnError — an unconfigured project is inert, not broken.
func TestMissingConfigIsNotAnError(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("a project with no config was an error: %v", err)
	}
	if c.BounceBudget != DefaultBounceBudget || c.DedupScope != "context" {
		t.Fatalf("defaults not applied: %+v", c)
	}
}

// TestConfiguredBudgetTakesEffect — set a value, observe it change. Asserting
// the parser contains a branch proves nothing about what the caller receives.
func TestConfiguredBudgetTakesEffect(t *testing.T) {
	c, err := Load(write(t, "hooks:\n  injection:\n    bounce_budget: 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.BounceBudget != 3 {
		t.Fatalf("bounce_budget reads %d; want the configured 3", c.BounceBudget)
	}
}

// TestLiveProjectConfigLoads — the repo's own config must satisfy its own
// floor. A config nobody validated is how a floor becomes decorative.
func TestLiveProjectConfigLoads(t *testing.T) {
	c, err := Load("../../../..")
	if err != nil {
		t.Fatalf("this project's own .edikt/config.yaml violates the floor: %v", err)
	}
	if c.StatusOf("inject-directives-pre") != StatusEnabled {
		t.Errorf("inject-directives-pre reads %q in this project; want enabled", c.StatusOf("inject-directives-pre"))
	}
}
