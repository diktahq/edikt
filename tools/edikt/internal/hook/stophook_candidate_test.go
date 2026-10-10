package hook

// stophook_candidate_test.go — SR-004 (AC-4.1): `signals.adr_candidate`
// defaults to quiet, firing only when the session's evidence answers GL-001
// Q2's three prongs — a real rejected alternative, a reversal cost, a
// re-litigation risk — and names them in the emitted signal. `eager` restores
// stopHookADRCandidateEager's pre-SR-004 behavior unchanged (see
// TestStopHookNoConfigIsSilent et al., which exercise that path directly via
// an explicit `signals.adr_candidate: eager` in their own sandbox config).

import (
	"path/filepath"
	"strings"
	"testing"
)

func stopHookQuietConfig(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"), "edikt_version: 0.8.0\n")
}

func TestStopHookCandidate_NoChoiceIsSilent(t *testing.T) {
	root, _ := stopHookSandbox(t)
	stopHookQuietConfig(t, root)

	const msg = "Fixed a typo in the parser's error message and reran the test suite."
	got := stopHook(newStopHookInvocation(msg))
	if strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("no choice in the evidence: adr-candidate must not fire; got %q", got.Response.Message)
	}
}

func TestStopHookCandidate_PartialProngsIsSilent(t *testing.T) {
	root, _ := stopHookSandbox(t)
	stopHookQuietConfig(t, root)

	// Names a rejected alternative and a reversal cost, but never a
	// re-litigation risk — two of three prongs is still silent.
	const msg = "I chose Kafka over RabbitMQ for this. It would be expensive to reverse " +
		"once producers are wired up."
	got := stopHook(newStopHookInvocation(msg))
	if strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("only two of three Q2 prongs evidenced: adr-candidate must not fire; got %q", got.Response.Message)
	}
}

func TestStopHookCandidate_AllThreeProngsFiresNamingThem(t *testing.T) {
	root, _ := stopHookSandbox(t)
	stopHookQuietConfig(t, root)

	const msg = "I chose Kafka over RabbitMQ for this pipeline. It would be expensive to reverse " +
		"once producers are wired up, and without the record this would be re-litigated by the " +
		"next engineer who reaches for RabbitMQ."
	got := stopHook(newStopHookInvocation(msg))
	if !strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("all three Q2 prongs evidenced: adr-candidate must fire; got %q", got.Response.Message)
	}
	// Naming the prongs: each matched fragment's surviving (sanitized) text
	// must appear in the rendered line, not a generic "decision detected".
	for _, want := range []string{"chose Kafka over RabbitMQ", "expensive to reverse", "re-litigated"} {
		if !strings.Contains(got.Response.Message, want) {
			t.Errorf("message = %q, want it to name the matched prong text %q", got.Response.Message, want)
		}
	}
	if strings.Contains(got.Response.Message, "decision language detected in the last response") {
		t.Errorf("message = %q, must not carry the eager path's generic evidence line", got.Response.Message)
	}
}

func TestStopHookCandidate_EagerRestoresOldSignal(t *testing.T) {
	root, _ := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")

	// Names only a rejected alternative — no reversal cost, no re-litigation
	// risk — which the quiet default (proven above) would suppress. Eager
	// fires on decision language alone, exactly as it did before SR-004.
	got := stopHook(newStopHookInvocation(stopHookDecisionMessage))
	if !strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("eager: decision language alone must fire; got %q", got.Response.Message)
	}
	if !strings.Contains(got.Response.Message, "decision language detected in the last response") {
		t.Errorf("message = %q, want the eager path's generic evidence line", got.Response.Message)
	}
}

func TestStopHookCandidate_UnknownModeFallsBackToQuiet(t *testing.T) {
	root, _ := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: nonsense\n")

	got := stopHook(newStopHookInvocation(stopHookDecisionMessage))
	if strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("an unrecognised signals.adr_candidate value must fall back to quiet, not eager; got %q",
			got.Response.Message)
	}
}
