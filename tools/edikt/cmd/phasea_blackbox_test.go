package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// THE REGRESSION FOR THE EXIT-CODE CLOBBER.
//
// emitPhaseAOutcomeNormal's first line was `if err != nil { return
// &exitCodeError{code: 2, msg: err.Error()} }` — discarding the outcome's
// own ExitCode on every failure path. The only two existing unit tests that
// exercise the function (TestPhaseAOutcome_PropagatesEveryExitCode,
// TestPhaseAOutcome_FailurePrintsOnceOnStderr) call it with err=nil — the
// one shape that never happens in production. Every phasea.Plan / .Restore
// / .Verify return that carries a non-OK ExitCode ALSO returns a non-nil
// error (see verify.go: "if exitCode == ExitVerifyOK { return out, nil };
// return out, fmt.Errorf(...)", repeated at every refusal in plan.go too).
// So the unit tests proved the branch that cannot occur and never touched
// the one that always fires — exit codes 1, 3, 5 and 7 were unreachable
// through the CLI; every failing verify exited 2 regardless of what it
// printed.
//
// This drives the BUILT BINARY, once per documented exit code, and asserts
// the OS process exit status — the only level at which the clobber is
// observable. Same class as INV-014: assert the resolved outcome, not the
// logic that is supposed to produce it.

var bbPlanIDRe = regexp.MustCompile(`\bpap-[0-9a-f]{12}\b`)

func bbWriteExtractorAgent(t *testing.T, root string, promptVersion int) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: sidecar-extractor\nmodel: claude-opus-5\nprompt_version: " +
		strconv.Itoa(promptVersion) + "\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "sidecar-extractor.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func bbWriteADR(t *testing.T, root, id, quote string) string {
	t.Helper()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+"-test.md")
	body := "# Title\n\n**Status:** Accepted\n\n" + quote + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func bbWriteConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bbSidecarPath mirrors the internal package's sidecarPathFor: swap .md for
// .edikt.yaml, same directory, same basename.
func bbSidecarPath(mdPath string) string {
	return mdPath[:len(mdPath)-len(".md")] + ".edikt.yaml"
}

// bbWriteGoodSidecar stands in for a sidecar-extractor dispatch that did its
// job: schema-valid YAML whose anchor quotes the parent byte-exactly at the
// fixed line 5 bbWriteADR places it on.
func bbWriteGoodSidecar(t *testing.T, mdPath, quote string) {
	t.Helper()
	sc := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/" + filepath.Base(mdPath),
		Directives: []sidecar.Directive{{
			Text:           "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: quote}},
		}},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bbSidecarPath(mdPath), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// runPhaseABinary execs the built edikt binary with cmd.Dir = root — the
// actual dispatch shape a caller uses, not a Go function call.
func runPhaseABinary(t *testing.T, root string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	bin := buildBinary(t)
	c := exec.Command(bin, args...)
	c.Dir = root
	c.Env = append(os.Environ(), "CLAUDE_HOME="+t.TempDir())
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	err := c.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("phasea %v did not run: %v", args, err)
		}
	}
	return out.String(), errb.String(), code
}

// bbPlan runs `phasea plan` for real and returns the plan_id parsed from its
// stdout — the same string a caller reads, not a struct field this test has
// no business seeing.
func bbPlan(t *testing.T, root string) string {
	t.Helper()
	out, errOut, code := runPhaseABinary(t, root, "phasea", "plan")
	if code != 0 {
		t.Fatalf("phasea plan exited %d, want 0\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	m := bbPlanIDRe.FindString(out)
	if m == "" {
		t.Fatalf("no plan_id found in phasea plan stdout: %q", out)
	}
	return m
}

// bbRestore runs `phasea restore --plan <id>` for real, so verify's restore
// coupling is satisfied the same way a real caller satisfies it.
func bbRestore(t *testing.T, root, planID string) {
	t.Helper()
	out, errOut, code := runPhaseABinary(t, root, "phasea", "restore", "--plan", planID)
	if code != 0 {
		t.Fatalf("phasea restore exited %d, want 0\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestPhaseA_BinaryExitCodes(t *testing.T) {
	t.Run("0 — every item ok", func(t *testing.T) {
		root := t.TempDir()
		bbWriteExtractorAgent(t, root, 3)
		md := bbWriteADR(t, root, "ADR-900", gateQuoteBB)
		bbWriteConfig(t, root, "")
		planID := bbPlan(t, root)
		bbWriteGoodSidecar(t, md, gateQuoteBB)
		bbRestore(t, root, planID)

		out, errOut, code := runPhaseABinary(t, root, "phasea", "verify", "--plan", planID)
		if code != phasea.ExitVerifyOK {
			t.Fatalf("phasea verify exited %d, want %d (ExitVerifyOK)\nstdout: %s\nstderr: %s",
				code, phasea.ExitVerifyOK, out, errOut)
		}
	})

	t.Run("1 — a retryable item, no dispatch happened", func(t *testing.T) {
		root := t.TempDir()
		bbWriteExtractorAgent(t, root, 3)
		bbWriteADR(t, root, "ADR-900", gateQuoteBB)
		bbWriteConfig(t, root, "")
		planID := bbPlan(t, root)
		bbRestore(t, root, planID)
		// No sidecar written: the item is output-missing, retryable.

		out, errOut, code := runPhaseABinary(t, root, "phasea", "verify", "--plan", planID)
		if code != phasea.ExitVerifyIncomplete {
			t.Fatalf("phasea verify exited %d, want %d (ExitVerifyIncomplete)\nstdout: %s\nstderr: %s",
				code, phasea.ExitVerifyIncomplete, out, errOut)
		}
	})

	t.Run("2 — unknown plan_id", func(t *testing.T) {
		root := t.TempDir()
		bbWriteExtractorAgent(t, root, 3)
		bbWriteADR(t, root, "ADR-900", gateQuoteBB)
		bbWriteConfig(t, root, "")

		out, errOut, code := runPhaseABinary(t, root, "phasea", "verify", "--plan", "pap-abcdef012345")
		if code != phasea.ExitVerifyPlanRejected {
			t.Fatalf("phasea verify exited %d, want %d (ExitVerifyPlanRejected)\nstdout: %s\nstderr: %s",
				code, phasea.ExitVerifyPlanRejected, out, errOut)
		}
	})

	t.Run("3 — malformed plan_id", func(t *testing.T) {
		root := t.TempDir()
		bbWriteConfig(t, root, "")

		out, errOut, code := runPhaseABinary(t, root, "phasea", "verify", "--plan", "not-a-plan-id")
		if code != phasea.ExitVerifyMalformed {
			t.Fatalf("phasea verify exited %d, want %d (ExitVerifyMalformed)\nstdout: %s\nstderr: %s",
				code, phasea.ExitVerifyMalformed, out, errOut)
		}
	})

	t.Run("4 — plan refuses with no extractor definition", func(t *testing.T) {
		root := t.TempDir()
		bbWriteADR(t, root, "ADR-900", gateQuoteBB)
		bbWriteConfig(t, root, "")
		// No .claude/agents/sidecar-extractor.md: prompt_version resolves to
		// unknown, and plan.go refuses before any corpus work (ExitRefused).

		out, errOut, code := runPhaseABinary(t, root, "phasea", "plan")
		if code != phasea.ExitRefused {
			t.Fatalf("phasea plan exited %d, want %d (ExitRefused)\nstdout: %s\nstderr: %s",
				code, phasea.ExitRefused, out, errOut)
		}
	})

	t.Run("5 — FAILED terminal when no retryable item remains", func(t *testing.T) {
		root := t.TempDir()
		bbWriteExtractorAgent(t, root, 3)
		bbWriteADR(t, root, "ADR-900", gateQuoteBB)
		// max_attempts_per_item: 1 makes the first verify call the last one —
		// an output-missing item at attempt 1 is already exhausted.
		bbWriteConfig(t, root, "phasea:\n  max_attempts_per_item: 1\n")
		planID := bbPlan(t, root)
		bbRestore(t, root, planID)

		out, errOut, code := runPhaseABinary(t, root, "phasea", "verify", "--plan", planID)
		if code != phasea.ExitVerifyFailed {
			t.Fatalf("phasea verify exited %d, want %d (ExitVerifyFailed)\nstdout: %s\nstderr: %s",
				code, phasea.ExitVerifyFailed, out, errOut)
		}
	})

	t.Run("7 — restore receipt missing", func(t *testing.T) {
		root := t.TempDir()
		bbWriteExtractorAgent(t, root, 3)
		bbWriteADR(t, root, "ADR-900", gateQuoteBB)
		bbWriteConfig(t, root, "")
		planID := bbPlan(t, root)
		// No restore call: verify must refuse rather than judge a corpus
		// that never had pinned state reinstated onto it.

		out, errOut, code := runPhaseABinary(t, root, "phasea", "verify", "--plan", planID)
		if code != phasea.ExitVerifyRestoreMissing {
			t.Fatalf("phasea verify exited %d, want %d (ExitVerifyRestoreMissing)\nstdout: %s\nstderr: %s",
				code, phasea.ExitVerifyRestoreMissing, out, errOut)
		}
	})
}

// D119a: a parent-hash mismatch is terminal IN FACT — the plan pins
// ArtifactSHA256 at plan time and evaluateItem re-hashes the LIVE artifact
// on every verify call, so the pin never moves and the artifact never
// reverts on its own. No re-dispatch against the SAME plan can ever clear
// it. Before the fix, StatusParentHashMismatch was absent from
// IsTerminalStatus, so it counted as retryable and exited 1 (INCOMPLETE)
// with a "re-dispatch and verify again" instruction that could never
// succeed — a dead end dressed as an answer. This drives the real binary
// through exactly that scenario and asserts exit 5, not 1.
func TestPhaseA_BinaryExitCode_ParentHashMismatchIsTerminal(t *testing.T) {
	root := t.TempDir()
	bbWriteExtractorAgent(t, root, 3)
	md := bbWriteADR(t, root, "ADR-900", gateQuoteBB)
	bbWriteConfig(t, root, "")
	planID := bbPlan(t, root)
	bbWriteGoodSidecar(t, md, gateQuoteBB)
	bbRestore(t, root, planID)

	// Mutate the PARENT ADR after planning — the plan's pinned
	// ArtifactSHA256 now disagrees with what verify re-hashes live.
	if err := os.WriteFile(md, []byte("# Title\n\n**Status:** Accepted\n\n"+gateQuoteBB+"\n\nAn extra sentence added after planning.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errOut, code := runPhaseABinary(t, root, "phasea", "verify", "--plan", planID)
	if code != phasea.ExitVerifyFailed {
		t.Fatalf("phasea verify exited %d, want %d (ExitVerifyFailed) — a parent-hash mismatch "+
			"must be terminal, not retryable\nstdout: %s\nstderr: %s",
			code, phasea.ExitVerifyFailed, out, errOut)
	}
}

// gateQuoteBB sits at line 5 of every ADR bbWriteADR writes, matching the
// internal package's own gateQuote fixture line placement so a sidecar
// anchored at 5..5 quoting it clears the content gate.
const gateQuoteBB = "The extractor MUST quote the artifact byte-exactly."
