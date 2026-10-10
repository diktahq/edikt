package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// frontDoorTree builds a minimal root->top->leaf cobra tree, matching
// TestVersionGateApplies' own pattern — a unit test on the predicate,
// not a full binary invocation.
func frontDoorTree(topName, leafName string) (root, top, leaf *cobra.Command) {
	root = &cobra.Command{Use: "edikt"}
	top = &cobra.Command{Use: topName}
	leaf = &cobra.Command{Use: leafName}
	top.AddCommand(leaf)
	root.AddCommand(top)
	return root, top, leaf
}

func TestFrontDoor_WalledVerbRefusesFromNonTerminalWithNoInvoker(t *testing.T) {
	_, _, leaf := frontDoorTree("gov", "compile")
	t.Setenv("EDIKT_INVOKER", "")
	old := isTerminalStdin
	isTerminalStdin = func() bool { return false }
	t.Cleanup(func() { isTerminalStdin = old })

	err := checkFrontDoor(leaf, t.TempDir())
	if err == nil {
		t.Fatal("expected a refusal for a walled verb with no invoker from a non-terminal")
	}
	ece, ok := err.(*exitCodeError)
	if !ok {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ece.code != 4 {
		t.Errorf("expected exit code 4, got %d", ece.code)
	}
	if !strings.Contains(ece.msg, "/edikt:gov:compile") {
		t.Errorf("expected the message to name /edikt:gov:compile, got %q", ece.msg)
	}
}

func TestFrontDoor_WalledVerbPassesFromRealTerminal(t *testing.T) {
	_, _, leaf := frontDoorTree("gov", "compile")
	t.Setenv("EDIKT_INVOKER", "")
	old := isTerminalStdin
	isTerminalStdin = func() bool { return true }
	t.Cleanup(func() { isTerminalStdin = old })

	root := t.TempDir()
	if err := checkFrontDoor(leaf, root); err != nil {
		t.Fatalf("expected a real terminal to always pass, got %v", err)
	}
	// SR-009's own text: invoked_by is "the value or `direct`" — a
	// terminal call with no invoker still gets journaled, as "direct",
	// so doctor's undeclared-invoker count has a subject to measure.
	body, err := os.ReadFile(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatalf("expected events.jsonl for a terminal call with no invoker: %v", err)
	}
	if !strings.Contains(string(body), `"invoked_by":"direct"`) {
		t.Errorf("expected invoked_by=direct for an undeclared terminal call, got %q", string(body))
	}
}

func TestFrontDoor_WalledVerbPassesWithValidInvoker(t *testing.T) {
	_, _, leaf := frontDoorTree("gov", "compile")
	old := isTerminalStdin
	isTerminalStdin = func() bool { return false }
	t.Cleanup(func() { isTerminalStdin = old })

	for _, v := range []string{"/edikt:gov:compile", "ci", "bench"} {
		t.Setenv("EDIKT_INVOKER", v)
		if err := checkFrontDoor(leaf, t.TempDir()); err != nil {
			t.Errorf("EDIKT_INVOKER=%q: expected pass, got %v", v, err)
		}
	}
}

func TestFrontDoor_InvalidInvokerStillRefused(t *testing.T) {
	_, _, leaf := frontDoorTree("gov", "compile")
	old := isTerminalStdin
	isTerminalStdin = func() bool { return false }
	t.Cleanup(func() { isTerminalStdin = old })

	for _, v := range []string{"garbage", "; rm -rf /", "edikt:gov:compile", "/edikt:", "/EDIKT:GOV:COMPILE"} {
		t.Setenv("EDIKT_INVOKER", v)
		if err := checkFrontDoor(leaf, t.TempDir()); err == nil {
			t.Errorf("EDIKT_INVOKER=%q: expected refusal, got nil error", v)
		}
	}
}

func TestFrontDoor_JournalsWalledOnly(t *testing.T) {
	root := t.TempDir()
	_, _, walledLeaf := frontDoorTree("gov", "compile")
	old := isTerminalStdin
	isTerminalStdin = func() bool { return false }
	t.Cleanup(func() { isTerminalStdin = old })
	t.Setenv("EDIKT_INVOKER", "/edikt:gov:compile")

	if err := checkFrontDoor(walledLeaf, root); err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatalf("expected events.jsonl to exist after a walled-verb pass: %v", err)
	}
	if !strings.Contains(string(body), "verb_invoked") || !strings.Contains(string(body), "/edikt:gov:compile") {
		t.Errorf("expected events.jsonl to record the walled invocation, got %q", string(body))
	}

	// An exempt verb passing through the SAME root must NOT add another
	// journal entry — invoked_by is journaled for walled verbs only.
	before := len(strings.Split(strings.TrimSpace(string(body)), "\n"))
	_, _, doctorLeaf := frontDoorTree("doctor", "noop")
	if err := checkFrontDoor(doctorLeaf, root); err != nil {
		t.Fatalf("exempt verb must never refuse: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}
	afterCount := len(strings.Split(strings.TrimSpace(string(after)), "\n"))
	if afterCount != before {
		t.Errorf("expected no new journal entry for an exempt verb, went from %d to %d lines", before, afterCount)
	}
}

// TestFrontDoor_Exempt runs every named exempt top-level verb from a
// non-terminal with EDIKT_INVOKER unset and asserts none refuses.
func TestFrontDoor_Exempt(t *testing.T) {
	old := isTerminalStdin
	isTerminalStdin = func() bool { return false }
	t.Cleanup(func() { isTerminalStdin = old })
	t.Setenv("EDIKT_INVOKER", "")

	for top := range frontDoorExemptTop {
		_, _, leaf := frontDoorTree(top, "leaf")
		if err := checkFrontDoor(leaf, t.TempDir()); err != nil {
			t.Errorf("exempt top-level verb %q must never refuse, got %v", top, err)
		}
	}
}

// TestFrontDoor_BenchRunnerVerbsPass covers D50(a)'s "every verb the
// benchmark runner invokes from a non-terminal with EDIKT_INVOKER
// unset [must not print] run /edikt:" — the documented invoker for
// machinery is "bench".
func TestFrontDoor_BenchRunnerVerbsPass(t *testing.T) {
	old := isTerminalStdin
	isTerminalStdin = func() bool { return false }
	t.Cleanup(func() { isTerminalStdin = old })
	t.Setenv("EDIKT_INVOKER", "bench")

	for _, top := range []string{"gov", "migrate", "sidecar", "upgrade", "upgrade-pin", "prd", "plan"} {
		_, _, leaf := frontDoorTree(top, "leaf")
		if err := checkFrontDoor(leaf, t.TempDir()); err != nil {
			t.Errorf("EDIKT_INVOKER=bench must clear verb %q, got %v", top, err)
		}
	}
}

func TestFrontDoor_WalledDerivesFromPermitList(t *testing.T) {
	_, _, leaf := frontDoorTree("gov", "compile")
	if !frontDoorWalled(leaf) {
		t.Error("expected gov (a user-audience, non-exempt permit-list verb) to be walled")
	}
	_, _, doctorLeaf := frontDoorTree("doctor", "leaf")
	if frontDoorWalled(doctorLeaf) {
		t.Error("expected doctor (named exempt) to never be walled")
	}
	_, _, unknownLeaf := frontDoorTree("not-a-real-verb", "leaf")
	if frontDoorWalled(unknownLeaf) {
		t.Error("expected a verb absent from the permit list entirely to never be walled")
	}
}

func TestFrontDoorCommandName(t *testing.T) {
	_, _, leaf := frontDoorTree("gov", "compile")
	if got, want := frontDoorCommandName(leaf), "/edikt:gov:compile"; got != want {
		t.Errorf("frontDoorCommandName = %q, want %q", got, want)
	}
}
