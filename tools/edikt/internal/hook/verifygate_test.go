package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// WHY THIS FILE CARRIES MORE THAN THE USUAL SHARE OF THE VERIFICATION.
//
// The hook corpus pins twelve verify-gate scenarios and every one of them
// passes, but a scenario can only vary three things: the payload, the config
// file, and the environment. It cannot seed .edikt/state/.evidence-reads or
// .edikt/state/verify/, and those two files are what the gate is ABOUT — the
// entire allow-on-evidence path and every reason a recorded read fails to
// count are structurally unreachable from the matrix.
//
// So the corpus proves the gate's refusals and its bypasses; these tests prove
// the part that decides when a claim is actually allowed. Neither is
// sufficient alone, and saying which covers what is the point — a criterion
// that claimed corpus coverage of these branches would be claiming coverage
// its own verify cannot check.

// vgProject builds a project tree with a chosen posture and returns its root,
// with the process chdir'd into it.
func vgProject(t *testing.T, posture string) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join(root, ".edikt", "state", "verify"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "edikt_version: 0.8.0\n"
	if posture != "" {
		cfg += "features:\n  evidence-gate: " + posture + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func vgWriteEvidence(t *testing.T, reads map[string]any) {
	t.Helper()
	b, err := json.Marshal(reads)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".edikt", "state", ".evidence-reads"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func vgWriteReport(t *testing.T, name string, passed, failed, timeout any) {
	t.Helper()
	summary := map[string]any{}
	if passed != nil {
		summary["passed"] = passed
	}
	if failed != nil {
		summary["failed"] = failed
	}
	if timeout != nil {
		summary["timeout"] = timeout
	}
	b, err := json.Marshal(map[string]any{"summary": summary})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(".edikt", "state", "verify", name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// vgClaim builds an AC-checkbox completion claim against the named plan file.
func vgClaim(t *testing.T, planFile string) adapter.Payload {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Edit",
		"tool_input": map[string]any{
			"file_path":  planFile,
			"old_string": "- [ ] AC-1.1 the thing works",
			"new_string": "- [x] AC-1.1 the thing works",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Built through Parse, never by hand: a hand-built Payload reports
	// Absent() and would silently exercise the no-payload branch instead of
	// the one each test names.
	p, err := adapter.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func vgRun(t *testing.T, p adapter.Payload) (Result, string) {
	t.Helper()
	var stderr bytes.Buffer
	res := verifyGate(&Invocation{Name: "verify-gate", Payload: p, Stderr: &stderr})
	return res, stderr.String()
}

func vgIsDeny(r Result) bool { return r.Response.Decision == adapter.DecisionDeny }
func vgIsAllow(r Result) bool {
	return r.Response.ContinueTrue && r.Response.Decision == adapter.DecisionUnset
}

// THE CORE ALLOW PATH, which the corpus cannot reach at all: a recorded
// evidence read plus a PASSING report lets the claim through, and the read is
// consumed so it cannot unlock a second claim.
func TestVerifyGateAllowsOnRecordedReadOfPassingReport(t *testing.T) {
	vgProject(t, postureBlock)
	vgWriteReport(t, "plan-demo.json", 3, 0, 0)
	vgWriteEvidence(t, map[string]any{"plan-demo": "2026-08-23T10:00:00Z"})

	res, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
	if !vgIsAllow(res) {
		t.Fatalf("a recorded read of a passing report must allow the claim; got %+v", res.Response)
	}

	// The read must be spent. Without this, one verification unlocks every
	// subsequent claim for the same artifact — the gate would check once and
	// then wave everything through.
	b, err := os.ReadFile(filepath.Join(".edikt", "state", ".evidence-reads"))
	if err != nil {
		t.Fatalf("state file not rewritten: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("state file is not valid JSON after the rewrite: %v (%s)", err, b)
	}
	if v, ok := got["plan-demo"]; !ok || v != nil {
		t.Errorf("evidence read was not consumed: %v", got)
	}

	// And the spent read must not satisfy the next claim.
	res2, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
	if !vgIsDeny(res2) {
		t.Errorf("a consumed read still unlocked a second claim; got %+v", res2.Response)
	}
}

// INV-011 — a recorded read is only evidence when the report it points at
// PASSES. The read proves someone looked; it does not prove what they saw.
func TestVerifyGateDeniesWhenTheReadReportDoesNotPass(t *testing.T) {
	cases := []struct {
		name                    string
		passed, failed, timeout any
		writeReport             bool
		wantDetail              string
	}{
		{"failing report", 3, 1, 0, true, "is FAILING or unreadable"},
		{"timed-out report", 3, 0, 1, true, "is FAILING or unreadable"},
		{"all-skipped run passes nothing", 0, 0, 0, true, "is FAILING or unreadable"},
		{"summary omits failed entirely", 3, nil, 0, true, "is FAILING or unreadable"},
		{"summary omits passed entirely", nil, 0, 0, true, "is FAILING or unreadable"},
		{"no report at all", 0, 0, 0, false, "no verify report matching this artifact exists"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vgProject(t, postureBlock)
			if c.writeReport {
				vgWriteReport(t, "plan-demo.json", c.passed, c.failed, c.timeout)
			}
			vgWriteEvidence(t, map[string]any{"plan-demo": "2026-08-23T10:00:00Z"})

			res, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
			if !vgIsDeny(res) {
				t.Fatalf("want deny, got %+v", res.Response)
			}
			if !strings.Contains(res.Response.Reason, c.wantDetail) {
				t.Errorf("reason %q does not name the cause %q", res.Response.Reason, c.wantDetail)
			}
		})
	}
}

// Audit 2026-08-07 #1 — the report needle is boundary-anchored. A bare
// substring let a PASSING report for a longer id satisfy the gate for a
// shorter one, so verifying ADR-0010 unlocked a claim about ADR-001.
func TestVerifyGateReportMatchIsBoundaryAnchored(t *testing.T) {
	vgProject(t, postureBlock)
	// A passing report for a DIFFERENT artifact whose id contains this one's.
	vgWriteReport(t, "gov-adr-0010.json", 5, 0, 0)
	vgWriteEvidence(t, map[string]any{"adr-001": "2026-08-23T10:00:00Z"})

	if _, passing := latestReportVerdict("adr-001"); passing {
		t.Error("a passing report for adr-0010 satisfied the gate for adr-001 — the needle is not anchored")
	}
	// The control: the same lookup DOES find its own report, so the negative
	// above is anchoring rather than a lookup that matches nothing.
	vgWriteReport(t, "gov-adr-001.json", 5, 0, 0)
	if _, passing := latestReportVerdict("adr-001"); !passing {
		t.Error("adr-001's own passing report was not found — the anchored needle matches nothing at all")
	}
}

// F-053 — the bare word "done" is not a completion claim. The loose form
// denied any plan write containing the word, including prose. Harmless while
// the deny was inert; a blocked write once ADR-061 made it real.
func TestVerifyGateProseDoneIsNotAClaim(t *testing.T) {
	notClaims := []struct{ name, old, new string }{
		{"prose mentioning done", "phase notes", "revisit once the migration is done"},
		{"a sentence denying completion", "x", "Nothing is done yet; all phases pending"},
	}
	for _, c := range notClaims {
		if isPlanProgressDone("Edit", "PLAN-demo.md", c.new, c.old) {
			t.Errorf("%s: prose was read as a completion claim", c.name)
		}
	}
	// The control: a real table-cell transition IS a claim, so the negatives
	// above are the bound working rather than detection being switched off.
	if !isPlanProgressDone("Edit", "PLAN-demo.md", "| 3 | Port | done |", "| 3 | Port | pending |") {
		t.Error("a table row flipping pending -> done was not detected as a claim")
	}
}

// The three claim shapes, each with the near-miss that must NOT trip it.
func TestVerifyGateClaimShapes(t *testing.T) {
	cases := []struct {
		name, tool, base, newText, oldText, want string
	}{
		{"sidecar false->true", "Edit", "ADR-001.edikt.yaml", "passes: true", "passes: false", "sidecar"},
		{"sidecar already true is idempotent", "Edit", "ADR-001.edikt.yaml", "passes: true\n# note", "passes: true", ""},
		{"sidecar write", "Write", "ADR-001.edikt.yaml", "passes: true", "", "sidecar"},
		{"plan row flip", "Edit", "PLAN-x.md", "| done |", "| pending |", "plan"},
		{"plan row still pending", "Edit", "PLAN-x.md", "| done | pending |", "| pending |", ""},
		{"ac checkbox flip", "Edit", "SPEC-1.md", "- [x] AC-1.1", "- [ ] AC-1.1", "ac"},
		{"ac already checked", "Edit", "SPEC-1.md", "- [x] AC-1.1", "- [x] AC-1.1", ""},
		{"ac shape in an unrelated extension", "Edit", "notes.txt", "- [x] AC-1.1", "- [ ] AC-1.1", ""},
	}
	for _, c := range cases {
		if got := detectClaim(c.tool, c.base, c.newText, c.oldText); got != c.want {
			t.Errorf("%s: detectClaim = %q, want %q", c.name, got, c.want)
		}
	}
}

// Tier A bypass — the actor allowlist is normalised before comparison
// (INV-006), so a homoglyph cannot buy a pass, and an unknown actor leaves the
// gate applying rather than opening it.
func TestVerifyGateActorAllowlistIsNormalizedAndClosed(t *testing.T) {
	cases := []struct {
		name, actor string
		wantBypass  bool
	}{
		{"plain allowlisted actor", "migrate", true},
		{"uppercase folds in", "MIGRATE", true},
		{"surrounding whitespace stripped", "  compile  ", true},
		{"fullwidth homoglyph folds to the same actor", "ｍｉｇｒａｔｅ", true},
		{"unknown actor does not bypass", "attacker", false},
		{"substring of an allowlisted actor does not bypass", "migrateX", false},
		{"empty actor does not bypass", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vgProject(t, postureBlock)
			t.Setenv("EDIKT_HOOK_ACTOR", c.actor)
			res, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
			if c.wantBypass && !vgIsAllow(res) {
				t.Fatalf("actor %q should bypass the gate; got %+v", c.actor, res.Response)
			}
			if !c.wantBypass && !vgIsDeny(res) {
				t.Fatalf("actor %q must NOT bypass the gate; got %+v", c.actor, res.Response)
			}
		})
	}
}

// The posture dial covers EVERY would-deny, including the unparseable-payload
// one. A posture that silently still blocks in some cases is not a dial.
func TestVerifyGatePostureCoversEveryDeny(t *testing.T) {
	for _, c := range []struct {
		posture  string
		wantDeny bool
		wantWarn bool
	}{
		{postureBlock, true, false},
		{postureWarn, false, true},
	} {
		t.Run(c.posture+"/claim", func(t *testing.T) {
			vgProject(t, c.posture)
			res, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
			if vgIsDeny(res) != c.wantDeny {
				t.Errorf("posture %s: deny=%v, want %v", c.posture, vgIsDeny(res), c.wantDeny)
			}
			if (res.Response.Context != "") != c.wantWarn {
				t.Errorf("posture %s: warn-channel=%v, want %v", c.posture, res.Response.Context != "", c.wantWarn)
			}
		})
		t.Run(c.posture+"/unparseable", func(t *testing.T) {
			vgProject(t, c.posture)
			var stderr bytes.Buffer
			res := verifyGate(&Invocation{Name: "verify-gate", Stderr: &stderr,
				PayloadErr: os.ErrInvalid})
			if vgIsDeny(res) != c.wantDeny {
				t.Errorf("posture %s: unparseable-payload deny=%v, want %v — this deny is the one most easily written as a bare early return that skips the dial",
					c.posture, vgIsDeny(res), c.wantDeny)
			}
		})
	}
}

// An unrecognised posture resolves to warn and SAYS SO. It must not resolve to
// block: silently upgrading an operator's typo to the highest-friction posture
// is a worse failure than running the documented-safe default.
func TestVerifyGateUnrecognisedPostureFallsBackToWarnLoudly(t *testing.T) {
	vgProject(t, "blcok")
	var stderr bytes.Buffer
	got := resolvePosture(&Invocation{Stderr: &stderr})
	if got != postureWarn {
		t.Errorf("unrecognised posture resolved to %q, want %q", got, postureWarn)
	}
	if !strings.Contains(stderr.String(), "unrecognised features.evidence-gate") {
		t.Errorf("the fallback was silent; stderr = %q", stderr.String())
	}
}

// The posture key is read from the top-level `features:` block only. A
// same-named key under a different parent must not be picked up — otherwise an
// unrelated section could turn the gate off.
func TestVerifyGatePostureIsScopedToTheFeaturesBlock(t *testing.T) {
	root := vgProject(t, "")
	cfg := "features:\n  evidence-gate: block\nother:\n  evidence-gate: disabled\n"
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolvePosture(nil); got != postureBlock {
		t.Errorf("posture = %q, want %q — a key outside features: was read", got, postureBlock)
	}
}

// F-050 — the basename comparison resolves symlinks first. Without it a link
// whose own name matches no claim shape but whose TARGET is a real plan file
// slips past every basename check.
func TestVerifyGateResolvesSymlinksBeforeClassifying(t *testing.T) {
	root := vgProject(t, postureBlock)
	target := filepath.Join(root, "PLAN-demo.md")
	if err := os.WriteFile(target, []byte("- [ ] AC-1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "innocent.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res, _ := vgRun(t, vgClaim(t, link))
	if !vgIsDeny(res) {
		t.Errorf("a claim reached through a symlink was not classified; got %+v", res.Response)
	}
}

// ADR-063:d02 — this handler has NO python3 dependency, so it must not
// reproduce bash's python3-absence banner. Bash degraded truthfully (its gate
// WAS a python heredoc); repeating that here would stand a working security
// control down on a healthy host.
func TestVerifyGateRunsWithoutPython3(t *testing.T) {
	vgProject(t, postureBlock)
	// An empty PATH removes python3 along with everything else.
	t.Setenv("PATH", "")
	res, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
	if !vgIsDeny(res) {
		t.Fatalf("the gate stopped enforcing when python3 was unreachable; got %+v", res.Response)
	}
	if strings.Contains(res.Response.Reason, "python3") {
		t.Errorf("the reply claims a python3 degradation this handler cannot have: %q", res.Response.Reason)
	}
}

// The two ledgers are distinct and both are written on a warn. ADR-062 §7:
// evidence-gate-warns.jsonl is what the doctor-driven ramp to `block`
// consumes, and without a producer the ramp counts an empty file forever.
func TestVerifyGateWarnWritesBothLedgers(t *testing.T) {
	vgProject(t, postureWarn)
	res, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
	if res.Response.Context == "" {
		t.Fatal("warn posture produced no model-facing warning")
	}
	for _, name := range []string{"evidence-gate-warns.jsonl", "verify-gate.jsonl"} {
		b, err := os.ReadFile(filepath.Join(".edikt", "state", name))
		if err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
		var rec map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(b), &rec); err != nil {
			t.Fatalf("%s is not one JSON object per line: %v (%s)", name, err, b)
		}
	}
}

// `educate` and `disabled` both allow, and the ONLY thing separating them is
// the audit record. Without it the enum's four-way distinction is
// unfalsifiable — so the difference is asserted, not assumed.
func TestVerifyGateEducateAndDisabledDifferOnlyByTheAuditRecord(t *testing.T) {
	vgProject(t, postureEducate)
	resE, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
	if _, err := os.Stat(filepath.Join(".edikt", "state", "verify-gate.jsonl")); err != nil {
		t.Errorf("educate wrote no audit record, making it indistinguishable from disabled: %v", err)
	}

	vgProject(t, postureDisabled)
	resD, _ := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
	if _, err := os.Stat(filepath.Join(".edikt", "state", "verify-gate.jsonl")); !os.IsNotExist(err) {
		t.Errorf("disabled wrote an audit record; it must short-circuit before any state I/O (err=%v)", err)
	}
	if !vgIsAllow(resE) || !vgIsAllow(resD) {
		t.Errorf("both postures must allow: educate=%+v disabled=%+v", resE.Response, resD.Response)
	}
}

// A malformed state file is treated as empty and reported on stderr, NOT
// through the protocol: a corrupt state file is an operator problem, and the
// gate must still default-deny rather than crash or wave the claim through.
func TestVerifyGateMalformedStateFileDefaultDenies(t *testing.T) {
	vgProject(t, postureBlock)
	if err := os.WriteFile(filepath.Join(".edikt", "state", ".evidence-reads"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, stderr := vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))
	if !vgIsDeny(res) {
		t.Errorf("a corrupt state file let the claim through; got %+v", res.Response)
	}
	if !strings.Contains(stderr, "malformed") {
		t.Errorf("the corruption was not reported on stderr: %q", stderr)
	}
}

// The newest report wins, so a stale PASS cannot outlive a fresh FAIL.
func TestVerifyGateUsesTheNewestMatchingReport(t *testing.T) {
	vgProject(t, postureBlock)
	vgWriteReport(t, "plan-demo-old.json", 5, 0, 0)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(".edikt", "state", "verify", "plan-demo-old.json"), old, old); err != nil {
		t.Fatal(err)
	}
	vgWriteReport(t, "plan-demo-new.json", 5, 2, 0)

	path, passing := latestReportVerdict("plan-demo")
	if passing {
		t.Error("a stale passing report outranked a fresh failing one")
	}
	if !strings.Contains(path, "new") {
		t.Errorf("selected %q, want the newest report", path)
	}
}

// AC-21.4 — A CRASH IS NOT AN ALLOW UNDER `block`.
//
// The dispatcher recovers every panic and, by default, replies with nothing —
// which the host reads as permission to proceed. For a gate that inverts the
// control: a hook that crashed did not find the write safe, it failed to look
// at it. INV-013 forbids a control reporting an outcome it did not achieve.
func TestVerifyGatePanicDegradesThroughThePostureDial(t *testing.T) {
	cases := []struct {
		posture      string
		wantDeny     bool
		wantAnnounce bool
	}{
		{postureBlock, true, false},
		{postureWarn, false, true},
		{postureEducate, false, false},
		{postureDisabled, false, false},
	}
	for _, c := range cases {
		t.Run(c.posture, func(t *testing.T) {
			vgProject(t, c.posture)
			got := verifyGatePanicReply()
			if (got.Decision == adapter.DecisionDeny) != c.wantDeny {
				t.Errorf("posture %s: deny=%v, want %v — a crashed gate must not read as a pass",
					c.posture, got.Decision == adapter.DecisionDeny, c.wantDeny)
			}
			if (got.Context != "") != c.wantAnnounce {
				t.Errorf("posture %s: announced=%v, want %v", c.posture, got.Context != "", c.wantAnnounce)
			}
			if c.wantDeny && !strings.Contains(got.Reason, "not a pass") {
				t.Errorf("posture %s: the deny does not say a crash is not a pass: %q", c.posture, got.Reason)
			}
		})
	}
}

// The fallback must survive the conditions that produced the panic. It is the
// second thing to run after a crash, and a fallback that also crashes leaves
// the dispatcher with nothing but its silent default.
func TestVerifyGatePanicReplySurvivesABrokenProject(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	// No .edikt at all, then an unreadable config, then a directory where the
	// config should be — three ways the tree can be wrong underneath it.
	if got := verifyGatePanicReply(); got.Context == "" && got.Decision == adapter.DecisionUnset {
		t.Error("no project: fallback produced no reply at all")
	}
	if err := os.MkdirAll(filepath.Join(root, ".edikt", "config.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := verifyGatePanicReply(); got.Context == "" && got.Decision == adapter.DecisionUnset {
		t.Error("config.yaml is a directory: fallback produced no reply at all")
	}
}

// AC-21.6 — the migrated hook's audit records match the PRE-MIGRATION golden's
// field-set and types.
//
// The golden was captured from the BASH hook while the registry row still read
// `pending`, for the reason the corpus records oracle=bash before a swap: a
// golden taken after the migration would be the Go rewrite measured against
// itself and would match by construction.
//
// The contract is field-set and types, NOT values — `ts` is wall-clock and
// `event`/`posture` vary with what produced the record.
func TestVerifyGateAuditRecordMatchesPreMigrationGolden(t *testing.T) {
	goldenPath := filepath.Join("..", "..", "..", "..", "test", "fixtures", "verify-gate-record.golden.json")
	gb, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden not readable at %s: %v", goldenPath, err)
	}
	var golden struct {
		Contract struct {
			RequiredKeys  []string          `json:"required_keys"`
			Types         map[string]string `json:"types"`
			PostureValues []string          `json:"posture_values"`
		} `json:"contract"`
	}
	if err := json.Unmarshal(gb, &golden); err != nil {
		t.Fatalf("golden is not valid JSON: %v", err)
	}
	if len(golden.Contract.RequiredKeys) == 0 {
		t.Fatal("golden declares no required keys — this test would assert nothing")
	}

	// `educate` is the posture that emits: it is the one _edikt_audit call
	// site the bash hook had, and the one the golden was captured from.
	vgProject(t, postureEducate)
	vgRun(t, vgClaim(t, "docs/internal/plans/PLAN-demo.md"))

	b, err := os.ReadFile(filepath.Join(".edikt", "state", "verify-gate.jsonl"))
	if err != nil {
		t.Fatalf("no audit record was written: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 audit record, got %d", len(lines))
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("audit record is not one JSON object: %v (%s)", err, lines[0])
	}

	if len(rec) != len(golden.Contract.RequiredKeys) {
		t.Errorf("record has %d key(s) %v, golden contract requires exactly %d %v",
			len(rec), keysOf(rec), len(golden.Contract.RequiredKeys), golden.Contract.RequiredKeys)
	}
	for _, k := range golden.Contract.RequiredKeys {
		v, ok := rec[k]
		if !ok {
			t.Errorf("record is missing required key %q", k)
			continue
		}
		if want := golden.Contract.Types[k]; want == "string" {
			if _, isStr := v.(string); !isStr {
				t.Errorf("key %q: got %T, want string", k, v)
			}
		}
	}
	// ts must be an RFC3339 UTC instant in the golden's exact shape.
	if ts, ok := rec["ts"].(string); ok {
		if _, err := time.Parse("2006-01-02T15:04:05Z", ts); err != nil {
			t.Errorf("ts %q is not RFC3339 UTC in the golden's format: %v", ts, err)
		}
	}
	// posture must be one of the four the enum declares.
	if p, ok := rec["posture"].(string); ok {
		found := false
		for _, allowed := range golden.Contract.PostureValues {
			if p == allowed {
				found = true
			}
		}
		if !found {
			t.Errorf("posture %q is not one of %v", p, golden.Contract.PostureValues)
		}
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// THE TWO DECLARATIONS ARE THE GATE'S FAIL-CLOSED PROPERTY, and neither is
// visible in the handler body — both live in init(). A refactor that dropped
// either would leave every test above passing while the seam quietly answered
// for the gate: an undecodable payload and a panic would each become a silent
// allow, which is what this hook exists to prevent.
func TestVerifyGateDeclaresItsFailClosedOptIns(t *testing.T) {
	if !PayloadTolerant("verify-gate") {
		t.Error("verify-gate is not payload-tolerant — an undecodable payload would be answered by the " +
			"dispatcher's silent fail-open instead of by INV-011's deny")
	}
	if _, ok := LookupPanicFallback("verify-gate"); !ok {
		t.Error("verify-gate declares no panic reply — a crash would read to the host as permission to proceed")
	}
}

// The seam itself, end to end: a payload-tolerant handler RUNS on undecodable
// input and its reply reaches stdout, while a plain handler does not run at
// all. Asserted through Dispatch rather than by calling the handler, because
// the behaviour under test belongs to the dispatcher.
func TestDispatchRunsOnlyPayloadTolerantHandlersOnBadInput(t *testing.T) {
	const bad = `{"tool_name": "Edit", "tool_input": {broken`

	for _, c := range []struct {
		name     string
		tolerant bool
		wantRan  bool
	}{
		{"tolerant-hook", true, true},
		{"plain-hook", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			reg := NewRegistry()
			ran := false
			h := func(inv *Invocation) Result {
				ran = true
				if inv.PayloadErr == nil {
					t.Error("handler ran without PayloadErr set — it cannot tell uninspectable input from empty input")
				}
				return Result{Response: adapter.Response{Decision: adapter.DecisionDeny, Reason: "refused"}}
			}
			if c.tolerant {
				reg.RegisterPayloadTolerant(c.name, h)
			} else {
				reg.Register(c.name, h)
			}

			var stdout, stderr bytes.Buffer
			code := Dispatch(Options{
				Name: c.name, Event: "PreToolUse",
				Stdin: strings.NewReader(bad), Stdout: &stdout, Stderr: &stderr,
				Registry: reg, Journal: func(hookmatch.Record) error { return nil },
			})
			if code != 0 {
				t.Fatalf("exit code %d, want 0", code)
			}
			if ran != c.wantRan {
				t.Fatalf("handler ran=%v, want %v", ran, c.wantRan)
			}
			if c.wantRan && !strings.Contains(stdout.String(), "refused") {
				t.Errorf("the tolerant handler's reply did not reach stdout: %q", stdout.String())
			}
			if !c.wantRan {
				// The dispatcher's own parse-failure reply is the empty
				// object `{}` — measured, not assumed; section 4's
				// "zero Response writes nothing" rule governs the normal
				// return path, not this one. What matters here is that it
				// carries NO VERDICT: the host reads `{}` as "no opinion".
				var reply map[string]any
				if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &reply); err != nil {
					t.Fatalf("stdout is not valid JSON: %q (%v)", stdout.String(), err)
				}
				if len(reply) != 0 {
					t.Errorf("a non-tolerant handler produced a verdict on a bad payload: %q", stdout.String())
				}
			}
		})
	}
}
