package trust

import (
	"os"
	"path/filepath"
	"testing"
)

// withEnv sets env vars for the duration of a test (t.Setenv auto-restores).
// HOME is pointed at a fresh tmp dir so the test never touches the developer's
// real ~/.edikt/state/trusted-roots.json.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EDIKT_HOME", "") // force the $HOME/.edikt resolution path
	t.Setenv(EnvBypass, "")    // start untrusted unless a test opts in
	t.Setenv(EnvMode, "")      // default posture (warn) unless a test overrides
	return home
}

func TestMode_resolution(t *testing.T) {
	sandbox(t)
	if Mode() != ModeWarn {
		t.Fatalf("unset EDIKT_VERIFY_TRUST_MODE should default to warn, got %q", Mode())
	}
	t.Setenv(EnvMode, "block")
	if Mode() != ModeBlock {
		t.Fatalf("block not resolved")
	}
	t.Setenv(EnvMode, "DISABLED") // case-insensitive
	if Mode() != ModeDisabled {
		t.Fatalf("disabled (uppercase) not resolved")
	}
	t.Setenv(EnvMode, "bogus")
	if Mode() != ModeWarn {
		t.Fatalf("unrecognized mode should fall back to warn, got %q", Mode())
	}
}

func TestEvaluate_warnDefault_runsRecordsWarns(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	d, msg := Evaluate(root, false)
	if d != ProceedWithWarning {
		t.Fatalf("warn posture on untrusted root: got %v, want ProceedWithWarning", d)
	}
	if msg == "" {
		t.Fatalf("warn posture must return a notice message")
	}
	// Trust-on-first-use: the root is now recorded, so a second evaluation is silent.
	if !IsTrusted(root) {
		t.Fatalf("warn posture must record the root (TOFU)")
	}
	if d2, _ := Evaluate(root, false); d2 != Proceed {
		t.Fatalf("second evaluation after TOFU record: got %v, want Proceed (silent)", d2)
	}
}

func TestEvaluate_block_refuses(t *testing.T) {
	sandbox(t)
	t.Setenv(EnvMode, "block")
	root := t.TempDir()
	if d, _ := Evaluate(root, false); d != Refuse {
		t.Fatalf("block posture on untrusted root: got %v, want Refuse", d)
	}
	if IsTrusted(root) {
		t.Fatalf("block posture must NOT record the root")
	}
}

func TestEvaluate_disabled_proceedsSilentlyNoRecord(t *testing.T) {
	sandbox(t)
	t.Setenv(EnvMode, "disabled")
	root := t.TempDir()
	if d, msg := Evaluate(root, false); d != Proceed || msg != "" {
		t.Fatalf("disabled posture: got (%v,%q), want (Proceed,\"\")", d, msg)
	}
	if IsTrusted(root) {
		t.Fatalf("disabled posture must NOT record the root")
	}
}

func TestEvaluate_explicitTrust_recordsAndProceeds(t *testing.T) {
	sandbox(t)
	t.Setenv(EnvMode, "block") // even in block mode, --trust proceeds
	root := t.TempDir()
	if d, _ := Evaluate(root, true); d != Proceed {
		t.Fatalf("explicit --trust: got %v, want Proceed", d)
	}
	if !IsTrusted(root) {
		t.Fatalf("explicit --trust must record the root")
	}
}

func TestIsTrusted_defaultFailClosed(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	if IsTrusted(root) {
		t.Fatalf("unknown project should be untrusted by default")
	}
}

func TestIsTrusted_envBypass(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	t.Setenv(EnvBypass, "1")
	if !IsTrusted(root) {
		t.Fatalf("EDIKT_VERIFY_TRUST=1 must grant trust without a store entry")
	}
}

func TestRecord_thenTrusted(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	if err := Record(root); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !IsTrusted(root) {
		t.Fatalf("recorded project must be trusted")
	}
	// A different root must remain untrusted.
	other := t.TempDir()
	if IsTrusted(other) {
		t.Fatalf("unrelated project must not be trusted by recording a different one")
	}
}

func TestRecord_persistsAndIsIdempotent(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	if err := Record(root); err != nil {
		t.Fatalf("Record #1: %v", err)
	}
	if err := Record(root); err != nil {
		t.Fatalf("Record #2 (idempotent): %v", err)
	}
	m := load()
	if len(m) != 1 {
		t.Fatalf("expected exactly one entry after double-record, got %d", len(m))
	}
	if _, ok := m[Realpath(root)]; !ok {
		t.Fatalf("store missing the recorded realpath key")
	}
}

func TestStore_neverUnderProjectRoot(t *testing.T) {
	home := sandbox(t)
	root := t.TempDir()
	if err := Record(root); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// The store must live under $HOME/.edikt/state, NOT inside the project.
	want := filepath.Join(home, ".edikt", "state", "trusted-roots.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("trust store not at user-global path %s: %v", want, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".edikt", "state", "trusted-roots.json")); err == nil {
		t.Fatalf("trust store must NOT be written under the project root")
	}
}

func TestCorruptStore_failsClosed(t *testing.T) {
	home := sandbox(t)
	root := t.TempDir()
	storeDir := filepath.Join(home, ".edikt", "state")
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "trusted-roots.json"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}
	if IsTrusted(root) {
		t.Fatalf("corrupt store must fail closed (untrusted), not grant trust")
	}
}

func TestEnvBypassDoesNotPersist(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	t.Setenv(EnvBypass, "1")
	if !IsTrusted(root) {
		t.Fatalf("env bypass should grant trust")
	}
	// Drop the bypass — without a recorded entry the project is untrusted
	// again (the bypass is ephemeral, it must not write to the store).
	t.Setenv(EnvBypass, "")
	if IsTrusted(root) {
		t.Fatalf("env bypass must not persist trust to the store")
	}
}
