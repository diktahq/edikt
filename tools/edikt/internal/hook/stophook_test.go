package hook

// stophook_test.go — the four branches
// test/fixtures/hook-corpus/stop-hook/matrix.yaml states it cannot reach, each
// with a POSITIVE CONTROL beside it.
//
// WHY THE CONTROLS ARE NOT OPTIONAL. A test that only asserts a refusal passes
// just as well if the code starts refusing everything: "no signal emitted"
// is the same observation whether the guard fired or the whole detector broke.
// Every case below therefore comes in a pair — the branch under test, and a
// neighbouring input that MUST produce a different, named output. Where three
// outcomes exist (staleness), all three are asserted pairwise distinct on
// literal bytes rather than on "contains" alone.

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// stopHookDecisionMessage trips the adr-candidate detector with a subject
// whose terms appear nowhere in the fixtures below, so G0 never suppresses it.
const stopHookDecisionMessage = "I chose redis over memcached because we need key expiry. A real trade-off."

// newStopHookInvocation builds an invocation carrying only the fields
// stop-hook reads.
func newStopHookInvocation(msg string) *Invocation {
	return &Invocation{
		Name:    "stop-hook",
		Stderr:  io.Discard,
		Payload: adapter.Payload{LastAssistantMessage: msg},
	}
}

// stopHookSandbox chdirs into a fresh project, points HOME at a second temp
// dir, and puts a PATH on the environment that carries python3 and NOTHING
// else — in particular, no `edikt`, so the staleness check's fourth discovery
// rung misses unless a test deliberately plants a binary on an earlier one.
//
// The python3 stub matters: stop-hook's preflight runs before any of this and
// an empty PATH would short-circuit every test here into the missing-
// interpreter banner, which is a different branch entirely.
func stopHookSandbox(t *testing.T) (root, home string) {
	t.Helper()
	root = t.TempDir()
	t.Chdir(root)
	home = t.TempDir()
	t.Setenv("HOME", home)

	stubDir := t.TempDir()
	writeExecutable(t, filepath.Join(stubDir, "python3"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir)
	// Both are consulted, in this order, before $HOME/.edikt — point them at
	// paths that cannot exist so the ladder's third rung misses too.
	t.Setenv("EDIKT_ROOT", filepath.Join(root, "no-such-root"))
	t.Setenv("EDIKT_HOME", filepath.Join(root, "no-such-home"))
	t.Setenv("EDIKT_DISABLE_HOOK_DEDUP", "1")

	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"), "edikt_version: 0.8.0\n")
	return root, home
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil { //nolint:gosec // deliberately executable
		t.Fatalf("write %s: %v", path, err)
	}
}

// ── branch 1: the missing-config guard ──────────────────────────────────────

func TestStopHookNoConfigIsSilent(t *testing.T) {
	root, _ := stopHookSandbox(t)
	// SR-004: stopHookDecisionMessage only names a rejected alternative (one
	// of three Q2 prongs), so the quiet default would not fire it — eager
	// restores the pre-SR-004 trigger this test exists to exercise.
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")

	// Positive control FIRST, so a later "silent" result cannot be explained
	// by the detector being broken in this environment.
	got := stopHook(newStopHookInvocation(stopHookDecisionMessage))
	if !strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("control: with a config present the decision signal must fire; got %+v", got.Response)
	}

	if err := os.Remove(filepath.Join(root, ".edikt", "config.yaml")); err != nil {
		t.Fatalf("remove config: %v", err)
	}
	got = stopHook(newStopHookInvocation(stopHookDecisionMessage))
	if !got.Response.IsZero() {
		t.Fatalf("with no .edikt/config.yaml the hook must write nothing at all; got %+v", got.Response)
	}
	if got.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 — a hook outside an edikt project is not an error", got.ExitCode)
	}
}

// ── branch 2: the upgrade-in-progress marker ────────────────────────────────

func TestStopHookUpgradeInProgressSuppresses(t *testing.T) {
	root, _ := stopHookSandbox(t)
	// SR-004: see the identical note in TestStopHookNoConfigIsSilent — this
	// test exercises the upgrade-marker guard, not the trigger threshold.
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")

	control := stopHook(newStopHookInvocation(stopHookDecisionMessage))
	if !strings.Contains(control.Response.Message, "adr-candidate") {
		t.Fatalf("control: the decision signal must fire without the marker; got %+v", control.Response)
	}

	writeTestFile(t, filepath.Join(root, ".edikt", "state", "upgrade-in-progress"), "")
	got := stopHook(newStopHookInvocation(stopHookDecisionMessage))

	if got.Response.Message != "" {
		t.Errorf("marker present: message = %q, want none", got.Response.Message)
	}
	if got.Response.Text != adapter.ContinueTrueReply {
		t.Errorf("marker present: text = %q, want %q", got.Response.Text, adapter.ContinueTrueReply)
	}
	// The bash writes this one with printf and no trailing newline, unlike the
	// identical-looking reply at the end of the hook. Losing the flag is a
	// one-byte difference no "is it valid JSON" assertion would ever see.
	if !got.Response.NoTrailingNewline {
		t.Error("marker present: reply must carry NoTrailingNewline — the bash uses printf, not echo")
	}
}

// ── branch 3: completion-claim, and its config opt-out ──────────────────────

func TestStopHookCompletionClaim(t *testing.T) {
	const planBody = "# Plan\n\n| Phase | Status | Attempt | Updated |\n" +
		"|---|---|---|---|\n| 1 | done | 1/1 | 2026-08-20 |\n" +
		"| 2 | in-progress | 0/1 | 2026-08-21 |\n"
	const claim = "All tests pass — phase 2 complete!"

	t.Run("fires during an in-progress phase", func(t *testing.T) {
		root, _ := stopHookSandbox(t)
		writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
			"edikt_version: 0.8.0\npaths:\n  plans: docs/internal/plans\n")
		writeTestFile(t, filepath.Join(root, "docs", "internal", "plans", "PLAN-042-demo.md"), planBody)

		got := stopHook(newStopHookInvocation(claim))
		if !strings.Contains(got.Response.Message, "completion-claim: 042-demo phase 2") {
			t.Fatalf("message = %q, want a completion-claim naming plan 042-demo phase 2", got.Response.Message)
		}
		if !strings.Contains(got.Response.Message, "edikt verify 042-demo --phase 2") {
			t.Errorf("message = %q, want the remediation command", got.Response.Message)
		}
	})

	t.Run("suppressed by quality-gates false", func(t *testing.T) {
		root, _ := stopHookSandbox(t)
		writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
			"edikt_version: 0.8.0\nquality-gates: false\npaths:\n  plans: docs/internal/plans\n")
		writeTestFile(t, filepath.Join(root, "docs", "internal", "plans", "PLAN-042-demo.md"), planBody)

		got := stopHook(newStopHookInvocation(claim))
		if strings.Contains(got.Response.Message, "completion-claim") {
			t.Fatalf("quality-gates: false must suppress the claim; got %q", got.Response.Message)
		}
		if got.Response.Text != adapter.ContinueTrueReply {
			t.Errorf("text = %q, want the plain continue reply", got.Response.Text)
		}
	})

	t.Run("silent with no in-progress row", func(t *testing.T) {
		root, _ := stopHookSandbox(t)
		writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
			"edikt_version: 0.8.0\npaths:\n  plans: docs/internal/plans\n")
		writeTestFile(t, filepath.Join(root, "docs", "internal", "plans", "PLAN-042-demo.md"),
			"# Plan\n\n| Phase | Status | Attempt | Updated |\n|---|---|---|---|\n| 1 | done | 1/1 | 2026-08-20 |\n")

		got := stopHook(newStopHookInvocation(claim))
		if strings.Contains(got.Response.Message, "completion-claim") {
			t.Fatalf("no in-progress row must mean no claim; got %q", got.Response.Message)
		}
	})
}

// ── branch 4: sidecar staleness — INV-013 in miniature ──────────────────────

// TestStopHookStaleSidecars is the acceptance criterion this phase exists for.
//
// "I could not check" and "I checked and found none" MUST NOT produce the same
// output. The three reachable outcomes are asserted on literal bytes and then
// asserted pairwise distinct, so a future change that collapsed any two of
// them fails here even if each one still individually "contains" the word a
// looser assertion would have looked for.
func TestStopHookStaleSidecars(t *testing.T) {
	const adrRelPath = "docs/architecture/decisions/ADR-001-demo.md"

	// seedManifest plants .claude/rules/governance/manifest.yaml — AC-4.4's
	// "compile fingerprint" — and pins its mtime, which is the ONLY thing
	// stopHookSidecarDriftFast reads from it.
	seedManifest := func(t *testing.T, root string, at time.Time) {
		t.Helper()
		p := filepath.Join(root, ".claude", "rules", "governance", "manifest.yaml")
		writeTestFile(t, p, "# manifest\n")
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatalf("chtimes manifest: %v", err)
		}
	}

	// seedSidecarPair writes a parent ADR and a real, loadable sidecar whose
	// one directive's source_excerpt quotes `quote` — verbatim in the prose
	// when inSync, a different sentence otherwise (real drift, not a
	// malformed fixture). Both files' mtimes are set to at.
	seedSidecarPair := func(t *testing.T, root string, at time.Time, inSync bool) {
		t.Helper()
		prose := "The thing MUST happen."
		sidecarQuote := prose
		if !inSync {
			sidecarQuote = "A sentence that no longer appears anywhere in the prose."
		}
		parentBody := "# ADR-001\n\n## Decision\n\n" + prose + "\n"
		parentPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-demo.md")
		writeTestFile(t, parentPath, parentBody)

		sc := &sidecar.Sidecar{
			SchemaVersion: 1,
			Topic:         "demo",
			Path:          adrRelPath,
			Directives: []sidecar.Directive{{
				Text: "The thing MUST happen. (ref: ADR-001)",
				SourceExcerpt: sidecar.SourceExcerpt{
					LineStart: 5,
					LineEnd:   5,
					Quote:     sidecarQuote,
				},
			}},
		}
		body, err := sidecar.Marshal(sc)
		if err != nil {
			t.Fatalf("marshal sidecar: %v", err)
		}
		sidecarPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-demo.edikt.yaml")
		writeTestFile(t, sidecarPath, string(body))

		for _, p := range []string{parentPath, sidecarPath} {
			if err := os.Chtimes(p, at, at); err != nil {
				t.Fatalf("chtimes %s: %v", p, err)
			}
		}
	}

	var unchecked, named, clean string
	past := time.Now().Add(-time.Hour)
	present := time.Now()

	t.Run("no compiled manifest exists -> UNCHECKED", func(t *testing.T) {
		root, _ := stopHookSandbox(t)
		seedSidecarPair(t, root, present, true) // in sync — irrelevant, no manifest to compare against

		got := stopHook(newStopHookInvocation("Updated the README wording."))
		unchecked = got.Response.Message

		if !strings.Contains(unchecked, "sidecar-stale-unchecked") {
			t.Fatalf("message = %q, want the sidecar-stale-unchecked kind", unchecked)
		}
		for _, want := range []string{
			"not checked — no compiled governance manifest exists yet, so no staleness determination was made",
			"this is not a report that sidecars are current",
		} {
			if !strings.Contains(unchecked, want) {
				t.Errorf("message = %q, want it to contain %q", unchecked, want)
			}
		}
		// The bare-bullet icon, not the ⚠ its sibling kind uses: a null result
		// is not a finding and must not dress as one.
		if !strings.HasPrefix(unchecked, "• sidecar-stale-unchecked: ") {
			t.Errorf("message = %q, want it to open with the bullet icon", unchecked)
		}
		// And nothing was invented on disk.
		if _, err := os.Stat(filepath.Join(root, ".edikt", "state", "stale-sidecars.log")); !os.IsNotExist(err) {
			t.Errorf("an UNCHECKED run must not leave a staleness log behind (err=%v)", err)
		}
	})

	t.Run("changed since the last compile and genuinely stale -> named finding", func(t *testing.T) {
		root, _ := stopHookSandbox(t)
		seedManifest(t, root, past)
		seedSidecarPair(t, root, present, false) // out of sync, newer than the manifest

		got := stopHook(newStopHookInvocation("Updated the README wording."))
		named = got.Response.Message

		if !strings.Contains(named, "⚠ sidecar-stale: ADR-001 — sidecar anchors no longer match the prose.") {
			t.Fatalf("message = %q, want a sidecar-stale finding naming the artifact", named)
		}
		if strings.Contains(named, "unchecked") {
			t.Fatalf("a finding must not carry the unchecked wording: %q", named)
		}
		// Derived state carries its provenance or it is regenerated.
		logBody, err := os.ReadFile(filepath.Join(root, ".edikt", "state", "stale-sidecars.log"))
		if err != nil {
			t.Fatalf("read staleness log: %v", err)
		}
		for _, want := range []string{
			"# generated by stop-hook's own fingerprint-based drift check (AC-4.4)",
			"# generated_at: ",
			"# corpus_head: ",
			"ADR-001\n",
		} {
			if !strings.Contains(string(logBody), want) {
				t.Errorf("staleness log = %q, want it to contain %q", logBody, want)
			}
		}
	})

	t.Run("changed since the last compile but still in sync -> silence", func(t *testing.T) {
		root, _ := stopHookSandbox(t)
		seedManifest(t, root, past)
		seedSidecarPair(t, root, present, true) // newer than the manifest, but not actually stale

		got := stopHook(newStopHookInvocation("Updated the README wording."))
		clean = got.Response.Message

		if clean != "" {
			t.Fatalf("a clean check has nothing to report; got %q", clean)
		}
		if got.Response.Text != adapter.ContinueTrueReply {
			t.Fatalf("text = %q, want %q", got.Response.Text, adapter.ContinueTrueReply)
		}
	})

	t.Run("no sidecar at all -> silence, not an UNCHECKED announcement", func(t *testing.T) {
		root, _ := stopHookSandbox(t)
		_ = root // deliberately no sidecar seeded

		got := stopHook(newStopHookInvocation("Updated the README wording."))
		if strings.Contains(got.Response.Message, "unchecked") {
			t.Fatalf("INV-013's Boundary: no subject means no announcement; got %q", got.Response.Message)
		}
		if got.Response.Text != adapter.ContinueTrueReply {
			t.Fatalf("text = %q, want %q", got.Response.Text, adapter.ContinueTrueReply)
		}
	})

	// The whole point, stated as one assertion rather than left implicit in
	// three separate subtests: no two outcomes may share a byte string.
	t.Run("the three outcomes are pairwise distinct", func(t *testing.T) {
		pairs := []struct {
			aName, a, bName, b string
		}{
			{"unchecked", unchecked, "named-finding", named},
			{"unchecked", unchecked, "clean", clean},
			{"named-finding", named, "clean", clean},
		}
		for _, p := range pairs {
			if p.a == p.b {
				t.Errorf("%s and %s produce identical output %q — INV-013 requires them distinguishable",
					p.aName, p.bName, p.a)
			}
		}
		if unchecked == "" || named == "" {
			t.Fatalf("a subtest above did not run: unchecked=%q named=%q", unchecked, named)
		}
	})
}

// TestStopHookStaleUncheckedDoesNotCollapseButAckSilences carries forward the
// F-082a properties test_stop_hook_binary_discovery.sh used to pin against
// the retired binary-discovery ladder (AC-4.4 replaced that ladder with the
// fingerprint/manifest check; the ladder itself is gone, but sidecar-stale-
// unchecked's OWN behavior — a null result never collapses on its own, and
// only an explicit human ack silences it — is unchanged and still needs a
// witness). No compiled manifest is this test's UNCHECKED trigger, in place
// of the retired "binary absent" one.
func TestStopHookStaleUncheckedDoesNotCollapseButAckSilences(t *testing.T) {
	root, _ := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-demo.md"),
		"# ADR-001\n\n## Decision\n\nSomething.\n")
	writeTestFile(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-demo.edikt.yaml"),
		"schema_version: 1\ntopic: demo\npath: docs/architecture/decisions/ADR-001-demo.md\n")
	// No manifest.yaml — UNCHECKED, same precondition TestStopHookStaleSidecars
	// pins for the same kind.

	first := stopHook(newStopHookInvocation("Updated the README wording."))
	if !strings.Contains(first.Response.Message, "sidecar-stale-unchecked") {
		t.Fatalf("precondition: expected UNCHECKED; got %q", first.Response.Message)
	}

	second := stopHook(newStopHookInvocation("Updated the README wording, again."))
	if strings.Contains(second.Response.Message, "known item held") {
		t.Fatalf("a null result (UNCHECKED) must never collapse on its own; got %q", second.Response.Message)
	}
	if !strings.Contains(second.Response.Message, "run /edikt:gov:compile once to establish a baseline") {
		t.Fatalf("an unacked repeat UNCHECKED firing must still carry the remediation in full; got %q", second.Response.Message)
	}

	sum := sha256.Sum256([]byte("sidecar-stale-unchecked|sidecar staleness|not checked — no compiled governance manifest exists yet, so no staleness determination was made"))
	fp := hex.EncodeToString(sum[:])[:16]
	writeTestFile(t, filepath.Join(root, ".edikt", "state", "hook-acks.json"),
		`{"`+fp+`": {"until": "2099-01-01", "why": "deliberately not compiling yet", "at": "test"}}`)

	acked := stopHook(newStopHookInvocation("Updated the README wording, once more."))
	if !strings.Contains(acked.Response.Message, "held: sidecar-stale-unchecked") ||
		!strings.Contains(acked.Response.Message, "ack: deliberately not compiling yet") {
		t.Fatalf("an explicit ack must silence sidecar-stale-unchecked exactly as it does any other kind; got %q",
			acked.Response.Message)
	}
}

// TestStopHookStaleLogIsRemovedWhenUnchecked pairs with the log-writing
// assertion above: a staleness log left over from a run that DID measure must
// not survive a later run that could not, or the file becomes a passing claim
// nobody wrote.
func TestStopHookStaleLogIsRemovedWhenUnchecked(t *testing.T) {
	root, _ := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-demo.edikt.yaml"),
		"schema_version: 1\n")
	stale := filepath.Join(root, ".edikt", "state", "stale-sidecars.log")
	writeTestFile(t, stale, "yesterday's answer\n")

	got := stopHook(newStopHookInvocation("Updated the README wording."))
	if !strings.Contains(got.Response.Message, "sidecar-stale-unchecked") {
		t.Fatalf("precondition: expected an UNCHECKED run; got %q", got.Response.Message)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale-sidecars.log survived an UNCHECKED run (err=%v)", err)
	}
}

// TestStopHookPayloadDigestIsOverRawBytes proves the dedup key is derived from
// what the host actually sent rather than from a re-encoding of the decoded
// struct — two payloads that decode identically but arrive differently are
// different invocations to the guard, and a trailing newline is not.
func TestStopHookPayloadDigestIsOverRawBytes(t *testing.T) {
	a, err := adapter.Parse([]byte(`{"last_assistant_message":"x"}`))
	if err != nil {
		t.Fatalf("parse a: %v", err)
	}
	withNewline, err := adapter.Parse([]byte("{\"last_assistant_message\":\"x\"}\n"))
	if err != nil {
		t.Fatalf("parse a+\\n: %v", err)
	}
	reordered, err := adapter.Parse([]byte(`{ "last_assistant_message" : "x" }`))
	if err != nil {
		t.Fatalf("parse reordered: %v", err)
	}

	if a.PayloadDigest("stop-hook:") != withNewline.PayloadDigest("stop-hook:") {
		t.Error("a trailing newline must not change the key — $(cat) strips it")
	}
	if a.PayloadDigest("stop-hook:") == reordered.PayloadDigest("stop-hook:") {
		t.Error("different bytes must produce different keys; the guard hashes the payload, not its meaning")
	}
	if a.PayloadDigest("stop-hook:") == a.PayloadDigest("phase-end-detector:") {
		t.Error("the prefix must participate: two hooks sharing a payload must not share a token")
	}
}

// TestStopHookG0TermLookupIsFixedString replaces the grep that used to sit in
// test-security.sh ("stop-hook.sh: uses grep -F for fixed-string matching on
// LLM content").
//
// That assertion could only ever see whether a `-F` appeared somewhere in the
// file, and after the swap it still "passed" — matching the `grep -qF` inside
// the shim's own trust check, which has nothing to do with governance lookup.
// The property it was reaching for is real: the G0 gate searches for terms
// lifted out of the model's own last turn, and the sanitizer permits '.' to
// survive (via `trade.?off`, whose `.?` can match a literal dot). A regex
// lookup would then let an attacker-influenceable subject match governance
// text it does not actually name, silently suppressing a real signal.
//
// Two sandboxes, differing only in ONE character of the governance file's
// first line: "tradeXoff" (which a regex "trade.off" matches and a fixed
// string does not) and "trade.off" (which both match). Fixed-string lookup
// fires on the first and suppresses on the second.
func TestStopHookG0TermLookupIsFixedString(t *testing.T) {
	const msg = "This is a trade.off zzz worth stating."

	run := func(t *testing.T, body string) Result {
		t.Helper()
		root, _ := stopHookSandbox(t)
		// SR-004: the trade-off-only message below names no reversal cost or
		// re-litigation risk, so eager is what this test exercises the G0
		// gate against, matching its pre-SR-004 behavior.
		writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
			"edikt_version: 0.8.0\nbase: docs\nsignals:\n  adr_candidate: eager\n")
		writeTestFile(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-900-demo.md"), body)
		return stopHook(newStopHookInvocation(msg))
	}

	// Positive control first: with the term present LITERALLY on line 1, G0
	// must fire and suppress. Without this, "no suppression" below would be
	// equally consistent with a G0 gate that never matches anything at all.
	suppressed := run(t, "# trade.off demo record\n\n## Decision\n\nSomething was decided.\n")
	if strings.Contains(suppressed.Response.Message, "adr-candidate") {
		t.Fatalf("control: a literal term match must suppress the candidate; got %q",
			suppressed.Response.Message)
	}

	// Neither layer of the lookup may match: the term appears nowhere in the
	// file except as something a regex would accept.
	fired := run(t, "# tradeXoff demo record\n\n## Decision\n\nSomething was decided.\n")
	if !strings.Contains(fired.Response.Message, "adr-candidate: trade.off zzz worth stating") {
		t.Fatalf("term %q must be matched as a FIXED string, so %q is not a hit and the "+
			"candidate fires; got %q", "trade.off", "tradeXoff", fired.Response.Message)
	}

	// The TITLE layer alone. G0a is two lookups in series — a recursive
	// content scan, then a first-line check over the files it returned — and
	// the case above only flips when BOTH are widened, because either one
	// still blocks on its own. Here the content scan legitimately matches (the
	// term is in the body, literally) and the first-line check is the only
	// thing standing between the subject and a suppressed signal, so a regex
	// there is observable by itself.
	firedTitleOnly := run(t, "# tradeXoff demo record\n\n## Decision\n\nThe trade.off was accepted.\n")
	if !strings.Contains(firedTitleOnly.Response.Message, "adr-candidate: trade.off zzz worth stating") {
		t.Fatalf("the first-line check must be a FIXED-string match: %q is not %q and the "+
			"candidate fires; got %q", "tradeXoff", "trade.off", firedTitleOnly.Response.Message)
	}
}
