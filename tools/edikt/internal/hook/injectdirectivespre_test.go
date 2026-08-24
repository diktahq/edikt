package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// WHAT THIS FILE COVERS THAT THE CORPUS DOES NOT.
//
// The corpus pins the deny, the grade filter and the tool filter — it can,
// since the matrix gained a `files:` field that seeds a compiled directive
// index per scenario. What it still cannot pin is CROSS-INVOCATION state:
// dedup suppressing the repeat bounce, and the bounce budget degrading a deny
// into a loud allow. Those need two invocations sharing a dedup store, which is
// a property of the store rather than of this hook, and the matrix says so
// rather than claiming coverage it does not have.

// idpProject builds a project with a compiled directive index and returns its
// root, with the process chdir'd into it and HOME pointed at a scratch dir so
// the dedup store and journal are per-test.
func idpProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(root)

	mustMkdir(t, filepath.Join(root, ".edikt"))
	mustWrite(t, filepath.Join(root, ".edikt", "config.yaml"), "edikt_version: 0.8.0\n")

	idx := filepath.Join(root, ".claude", "rules", "governance")
	mustMkdir(t, idx)
	mustWrite(t, filepath.Join(idx, "directive-index.yaml"), `schema_version: 1
globs:
  "governed/**":
    - id: INV-TEST-001:d01
      grade: must
      class: invariant
      text: The MUST-grade rule under test.
      intent: Give the gate something to deny on.
    - id: ADR-TEST-002:d01
      grade: must
      class: adr
      text: A second MUST-grade rule, ADR class.
  "advisory-only/**":
    - id: GL-TEST-003:d01
      grade: advisory
      class: guideline
      text: An advisory rule that must NOT produce a deny.
`)
	return root
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// idpWrite builds a Write payload through Parse — never by hand, since a
// hand-built Payload reports Absent and Present("AgentID") false, which is
// exactly the identity distinction several of these tests turn on.
func idpWrite(t *testing.T, path, session string, extra map[string]any) adapter.Payload {
	t.Helper()
	obj := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Write",
		"tool_input":      map[string]any{"file_path": path, "content": "x"},
		"session_id":      session,
	}
	for k, v := range extra {
		obj[k] = v
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	p, err := adapter.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func idpRun(t *testing.T, p adapter.Payload) Result {
	t.Helper()
	var stderr bytes.Buffer
	return injectDirectivesPre(&Invocation{Name: "inject-directives-pre", Payload: p, Stderr: &stderr})
}

// The branch the hook exists for: a MUST-grade match denies the write.
func TestInjectDirectivesPreDeniesOnMustMatch(t *testing.T) {
	idpProject(t)
	res := idpRun(t, idpWrite(t, "governed/thing.go", "s-deny", nil))
	if res.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("a MUST-grade match did not deny; got %+v", res.Response)
	}
}

// The deny must NAME every directive it denied on. A deny that says "this is
// governed" without saying by what cannot be complied with, and the whole
// bounce mechanism depends on the agent being able to revise.
func TestInjectDirectivesPreDenyNamesDirectives(t *testing.T) {
	idpProject(t)
	res := idpRun(t, idpWrite(t, "governed/thing.go", "s-names", nil))
	for _, want := range []string{"INV-TEST-001:d01", "ADR-TEST-002:d01", "The MUST-grade rule under test."} {
		if !strings.Contains(res.Response.Reason, want) {
			t.Errorf("deny reason does not name %q:\n%s", want, res.Response.Reason)
		}
	}
	// Structural class ordering: invariant before ADR.
	if i, j := strings.Index(res.Response.Reason, "INV-TEST-001"), strings.Index(res.Response.Reason, "ADR-TEST-002"); i > j {
		t.Errorf("ADR sorted before invariant; a reader should see the highest structural class first")
	}
}

// ADR-060, not ADR-038. Both this hook and verify-gate deny, deliberately
// through different channels: this one denies the TOOL CALL so the model
// revises and proceeds, which is what its own message tells the reader to do.
func TestInjectDirectivesPreUsesADR060DenyShape(t *testing.T) {
	idpProject(t)
	res := idpRun(t, idpWrite(t, "governed/thing.go", "s-shape", nil))

	if res.Response.Block {
		t.Error("the reply stops the turn; ADR-060 specifies a denied tool call, and the message says 'revise the write to comply, then proceed'")
	}
	if res.Response.Message != "" {
		t.Error("the directive text is on the operator-facing channel; the MODEL is the actor that must revise the write")
	}
	if res.Response.Decision != adapter.DecisionDeny || res.Response.Reason == "" {
		t.Errorf("want a permission deny carrying a reason; got %+v", res.Response)
	}
	// And it must render as exactly one JSON object.
	out, err := adapter.Encode("PreToolUse", res.Response)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var one map[string]any
	if err := json.Unmarshal(out, &one); err != nil {
		t.Fatalf("reply is not a single JSON object: %v (%s)", err, out)
	}
}

// Advisory grade is the PostToolUse tier's channel. Denying for a suggestion
// teaches the agent the gate is noise, which costs the deny its force where it
// actually matters.
func TestInjectDirectivesPreIgnoresAdvisoryGrade(t *testing.T) {
	idpProject(t)
	res := idpRun(t, idpWrite(t, "advisory-only/thing.go", "s-adv", nil))
	if res.Response.Decision == adapter.DecisionDeny {
		t.Fatalf("an advisory-grade directive produced a DENY: %s", res.Response.Reason)
	}
	if !res.Response.ContinueTrue {
		t.Errorf("want a plain allow; got %+v", res.Response)
	}
}

// The tool filter changes the OUTCOME, not just the code path: a Read of the
// very path a Write is denied for must proceed.
func TestInjectDirectivesPreOnlyFiresForWriteTools(t *testing.T) {
	idpProject(t)
	if res := idpRun(t, idpWrite(t, "governed/thing.go", "s-tool", nil)); res.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("control failed: the Write was not denied, so the Read below proves nothing")
	}

	raw, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": "governed/thing.go"},
		"session_id":      "s-tool-read",
	})
	p, err := adapter.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if res := idpRun(t, p); res.Response.Decision == adapter.DecisionDeny {
		t.Error("a Read was denied; only file-writing tools carry a path worth matching")
	}
}

// EXACTLY ONE BOUNCE PER RECEIVING CONTEXT. Denying the same directive set on
// every subsequent write is a loop the agent cannot escape, and the rational
// response to an inescapable gate is to route around it.
func TestInjectDirectivesPreDedupSuppressesRepeat(t *testing.T) {
	idpProject(t)
	first := idpRun(t, idpWrite(t, "governed/thing.go", "s-dedup", map[string]any{"agent_id": "ctx-1"}))
	if first.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("the first write was not denied, so the suppression below proves nothing: %+v", first.Response)
	}
	second := idpRun(t, idpWrite(t, "governed/thing.go", "s-dedup", map[string]any{"agent_id": "ctx-1"}))
	if second.Response.Decision == adapter.DecisionDeny {
		t.Error("the retry was denied again — the agent has no way out of this gate")
	}
}

// A DIFFERENT receiving context has not been told, and must not inherit the
// parent's suppression. Measurement showed the subagent demonstrably had not
// seen the parent's bounce.
func TestInjectDirectivesPreDoesNotSuppressAcrossContexts(t *testing.T) {
	idpProject(t)
	if res := idpRun(t, idpWrite(t, "governed/thing.go", "s-ctx", map[string]any{"agent_id": "ctx-A"})); res.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("control failed: first context was not denied")
	}
	res := idpRun(t, idpWrite(t, "governed/thing.go", "s-ctx", map[string]any{"agent_id": "ctx-B"}))
	if res.Response.Decision != adapter.DecisionDeny {
		t.Error("a second receiving context inherited the first's suppression and was never told the rule")
	}
}

// ABSENT, BLANK AND PRESENT ARE THREE FACTS, NOT TWO. A blank agent_id is
// UNKNOWN and must DELIVER; collapsing it into "parent" let it inherit parent
// suppression once already.
func TestInjectDirectivesPreIdentityIsThreeWay(t *testing.T) {
	cases := []struct {
		name        string
		extra       map[string]any
		wantActor   string
		wantContext string
	}{
		{"no agent_id key at all", nil, "parent", "parent"},
		{"agent_id present and set", map[string]any{"agent_id": "a-1"}, "subagent", "a-1"},
		{"agent_id present but blank", map[string]any{"agent_id": ""}, "unknown", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := idpWrite(t, "governed/thing.go", "s-id", c.extra)
			actor, ctx, _ := resolveInjectionIdentity(p)
			if actor != c.wantActor || ctx != c.wantContext {
				t.Errorf("actor=%q context=%q, want %q/%q", actor, ctx, c.wantActor, c.wantContext)
			}
		})
	}

	// And the consequence: an unknown identity must never suppress. An empty
	// context id is what AlreadyBounced reads as "cannot dedup, so bounce".
	idpProject(t)
	for i := 0; i < 3; i++ {
		res := idpRun(t, idpWrite(t, "governed/thing.go", "s-unknown", map[string]any{"agent_id": ""}))
		if res.Response.Decision != adapter.DecisionDeny {
			t.Fatalf("write %d with an unresolvable identity was suppressed; unknown must over-deliver", i+1)
		}
	}
}

// F-052 — PAST THE BUDGET THE WRITE PROCEEDS, BUT NOT QUIETLY. Going silent
// past the budget would be the "proceeds unheard" outcome the budget mechanism
// was written to rule out.
func TestInjectDirectivesPreBudgetDegradesLoudly(t *testing.T) {
	root := idpProject(t)
	mustWrite(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nhooks:\n  injection:\n    bounce_budget: 2\n")

	var last Result
	denies := 0
	for i := 0; i < 5; i++ {
		last = idpRun(t, idpWrite(t, "governed/thing.go", "s-budget",
			map[string]any{"agent_id": "ctx-" + string(rune('a'+i))}))
		if last.Response.Decision == adapter.DecisionDeny {
			denies++
		}
	}
	if denies == 0 {
		t.Fatal("no context was ever denied, so this measures nothing")
	}
	if denies == 5 {
		t.Fatal("every context was denied; the bounce budget never engaged")
	}
	if last.Response.Decision == adapter.DecisionDeny {
		t.Fatalf("still denying past the budget: %s", last.Response.Reason)
	}
	// The loud half: the directives still reach the model.
	if last.Response.Context == "" {
		t.Fatal("past the budget the write proceeds SILENTLY — the exact outcome the budget exists to rule out")
	}
	for _, want := range []string{"INV-TEST-001:d01", "bounce_budget"} {
		if !strings.Contains(last.Response.Context, want) {
			t.Errorf("the degraded message does not mention %q:\n%s", want, last.Response.Context)
		}
	}
}

// ADR-063:d02, third application. The bash shim degraded when python3 was
// missing because its JSON construction WAS python. This handler has no
// interpreter dependency, so reproducing the banner would announce a failure
// that is not occurring — and then cause it.
func TestInjectDirectivesPreRunsWithoutPython3(t *testing.T) {
	idpProject(t)
	t.Setenv("PATH", "")
	res := idpRun(t, idpWrite(t, "governed/thing.go", "s-nopy", nil))
	if res.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("the gate stopped enforcing with python3 unreachable; got %+v", res.Response)
	}
	if strings.Contains(res.Response.Reason, "python3") || strings.Contains(res.Response.Context, "python3") {
		t.Error("the reply claims a python3 degradation this handler cannot suffer")
	}
}

// ADR-063:d02, fourth application, and the one that removes a bug class rather
// than a banner. The bash shim resolved $EDIKT_BIN through ADR-041's ladder and
// degraded when it found nothing — its own comment calls that the third
// instance of a resolution bug already fixed twice elsewhere. This handler IS
// the binary, so there is no resolution left to fail.
func TestInjectDirectivesPreHasNoBinaryResolution(t *testing.T) {
	src, err := os.ReadFile("injectdirectivespre.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, forbidden := range []string{"exec.Command", "EDIKT_BIN", "LookPath"} {
		if strings.Contains(stripComments(body), forbidden) {
			t.Errorf("handler still reaches for a binary via %q — the migration was supposed to remove that class, not relocate it", forbidden)
		}
	}

	// And the behavioural half: with every discovery rung poisoned, the gate
	// still denies. A handler that shelled out would degrade here.
	idpProject(t)
	t.Setenv("EDIKT_ROOT", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("EDIKT_HOME", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("PATH", "")
	if res := idpRun(t, idpWrite(t, "governed/thing.go", "s-nobin", nil)); res.Response.Decision != adapter.DecisionDeny {
		t.Errorf("the gate degraded when no edikt binary was discoverable; got %+v", res.Response)
	}
}

// stripComments removes // line comments so a doc comment naming a forbidden
// token does not read as the handler using it.
func stripComments(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// The fail-open contract, which is the OPPOSITE of verify-gate's ruling on the
// same input and must stay that way. ADR-060:d07 governs this tier; INV-011
// governs verify-gate, where the payload IS the claim being checked.
func TestInjectDirectivesPreFailsOpenOnUnparseablePayload(t *testing.T) {
	idpProject(t)
	var stderr bytes.Buffer
	res := injectDirectivesPre(&Invocation{
		Name: "inject-directives-pre", Stderr: &stderr, PayloadErr: os.ErrInvalid,
	})
	if res.Response.Decision == adapter.DecisionDeny {
		t.Fatal("an unparseable payload DENIED; blocking an editor because governance is broken is worse than missing an injection (ADR-060:d07)")
	}
	if !res.Response.ContinueTrue {
		t.Errorf("want bash's explicit allow so the fail-open is visible in this hook's own bytes; got %+v", res.Response)
	}
}

// AC-22.3 — the migrated hook's bounce/dedup behaviour matches the
// PRE-MIGRATION golden captured while the registry row still read `pending`.
//
// The golden is asserted as a CONTRACT, not as bytes. Its own header explains
// why the previous capture was discarded: it recorded a byte count without
// recording the input that produced it, which is unreproducible, and ADR-071
// then moved the MUST population from 98.2% to 30.2% of the corpus so the
// number described a different world. What survives re-grading is the shape —
// which keys appear, which channel each branch uses, and the invariant that
// past the budget the write proceeds without going silent.
func TestInjectDirectivesPreMatchesBounceBudgetGolden(t *testing.T) {
	gb, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "test", "fixtures", "bounce-budget-dedup.golden.json"))
	if err != nil {
		t.Fatalf("golden not readable: %v", err)
	}
	var golden struct {
		Contract struct {
			ConfigKey     string `json:"config_key"`
			DefaultBudget int    `json:"default_budget"`
			DedupScope    string `json:"dedup_scope"`
		} `json:"contract"`
		Captured struct {
			UnderBudgetBounce struct {
				TopLevelKeys       []string `json:"top_level_keys"`
				HookSpecificKeys   []string `json:"hookSpecificOutput_keys"`
				PermissionDecision string   `json:"permissionDecision"`
			} `json:"under_budget_bounce"`
		} `json:"captured"`
	}
	if err := json.Unmarshal(gb, &golden); err != nil {
		t.Fatalf("golden is not valid JSON: %v", err)
	}
	if golden.Contract.DefaultBudget == 0 || len(golden.Captured.UnderBudgetBounce.HookSpecificKeys) == 0 {
		t.Fatal("golden carries no contract to assert against — this test would check nothing")
	}

	root := idpProject(t)
	mustWrite(t, filepath.Join(root, ".edikt", "config.yaml"), "edikt_version: 0.8.0\n")

	res := idpRun(t, idpWrite(t, "governed/thing.go", "s-golden", nil))
	out, err := adapter.Encode("PreToolUse", res.Response)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("reply is not one JSON object: %v (%s)", err, out)
	}

	// Top-level key set, exactly.
	if len(got) != len(golden.Captured.UnderBudgetBounce.TopLevelKeys) {
		t.Errorf("top-level keys %v, golden %v", keysOf2(got), golden.Captured.UnderBudgetBounce.TopLevelKeys)
	}
	for _, k := range golden.Captured.UnderBudgetBounce.TopLevelKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("reply is missing top-level key %q the golden recorded", k)
		}
	}
	var hso map[string]json.RawMessage
	if raw, ok := got["hookSpecificOutput"]; ok {
		if err := json.Unmarshal(raw, &hso); err != nil {
			t.Fatalf("hookSpecificOutput is not an object: %v", err)
		}
	}
	for _, k := range golden.Captured.UnderBudgetBounce.HookSpecificKeys {
		if _, ok := hso[k]; !ok {
			t.Errorf("hookSpecificOutput is missing %q the golden recorded", k)
		}
	}
	var decision string
	if raw, ok := hso["permissionDecision"]; ok {
		_ = json.Unmarshal(raw, &decision)
	}
	if decision != golden.Captured.UnderBudgetBounce.PermissionDecision {
		t.Errorf("permissionDecision %q, golden %q", decision, golden.Captured.UnderBudgetBounce.PermissionDecision)
	}

	// The contract's own default, read from the config key it names.
	if golden.Contract.ConfigKey != "hooks.injection.bounce_budget" {
		t.Errorf("golden names config key %q; the handler reads hooks.injection.bounce_budget", golden.Contract.ConfigKey)
	}
	if golden.Contract.DedupScope != "context" {
		t.Errorf("golden records dedup_scope %q; this handler keys dedup on the RECEIVING context", golden.Contract.DedupScope)
	}
}

func keysOf2(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
