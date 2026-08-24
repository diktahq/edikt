package hook

// subagentstop.go — the Go replacement for templates/hooks/subagent-stop.sh
// (PLAN-v080-reliability-core Phase 18). 374 lines of bash around 20 python3
// invocations, and the only hook in the tier whose ordinary outcome is a
// BLOCK: a specialist agent listed under `gates:` whose severity meets its
// configured threshold stops the turn.
//
// The charter is the same as every other phase — reproduce the bash body's
// BYTES, not improve on them — with ONE ruled exception, stated below.
//
// FOUR ORDERING FACTS THAT LOOK ARBITRARY AND ARE NOT
//
//  1. The python3 preflight runs AFTER stdin is read and after the
//     missing-config guard, so a project with no .edikt/config.yaml is silent
//     even with no interpreter, and the no-python3 scenario needs a PATH that
//     still resolves `cat`.
//  2. The duplicate-invocation guard sits between the preflight and identity
//     resolution. A suppressed duplicate therefore leaves a token file behind
//     and writes NOTHING else — no signal-log line, no ledger entry. Two
//     scenarios (repeat-fire, dedup-disabled) pin the difference.
//  3. The session-signals.log append happens BEFORE the `quality-gates:
//     false` opt-out is consulted, so opting out of gates does not opt out of
//     the activity log. quality-gates-off is the scenario that pins it.
//  4. The severity fail-closed rule (ADR-048/INV-011) is applied AFTER the
//     threshold is resolved, because it maps an anomalous severity ONTO that
//     threshold. Resolving the threshold later would leave nothing to map to.
//
// THE ONE DELIBERATE DEVIATION: THE PROJECT ROOT (AC-18.3)
//
// The bash reads its gates block from
// `os.environ.get("EDIKT_PROJECT_ROOT", ".")` while every other path in the
// same script is cwd-relative. The two agree only by accident, and a relative
// "." resolves against whatever directory the host happened to spawn the hook
// in — so the threshold that decides whether a critical finding BLOCKS can be
// read out of a different tree than the opt-out and the is-this-a-gate scan
// were. This handler resolves ONE absolute root and uses it for ALL of them:
// the missing-config guard, the dedup directory, the `quality-gates: false`
// opt-out, the is-this-a-gate scan and the threshold parse.
//
// EDIKT_PROJECT_ROOT is still honoured when SET — dropping it would break
// test/unit/hooks/test_subagent_stop_severity_failclosed.sh's own contract —
// but it now selects that ONE root rather than only the threshold's. Two
// answers to "which project is this" was the defect; keeping the env var
// while leaving the split in place would have fixed the spelling and not the
// problem. In production the variable is never set: shim_test.go asserts the
// shim must not even reference it, because it is a local of stop-hook.sh and
// is exported nowhere.
//
// See subagentStopRoot for what the root anchors on when the variable is
// unset, and why that is not the host-reported cwd.
//
// WHAT THE COMMITTED CORPUS CANNOT REACH is listed in
// test/fixtures/hook-corpus/subagent-stop/matrix.yaml's header together with
// the test that fails when each branch is deleted: the missing-config guard,
// a gate override that MATCHES, and the project-root resolution above (which
// no scenario can discriminate, because the recorder always execs with cwd ==
// the project). All three are implemented in full here and all three are
// covered by subagentstop_test.go with positive controls.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("subagent-stop", subagentStop)
}

// subagentStopNoPython3Message is the literal text subagent-stop.sh's own
// preflight printf carries. The wire shape around it — `{"continue": true,
// "systemMessage": "…"}`, hand-built by printf because python3 is by
// definition unavailable on this branch — is rendered by
// adapter.NoPython3ContinueBanner and never spelled out here (SP-004/SAC-003).
const subagentStopNoPython3Message = "⚠ edikt: python3 is missing — this subagent output could NOT be checked against any configured quality gate. If it contained a critical finding, it was NOT blocked."

// subagentStopDedupTTL and subagentStopDedupPrune mirror the two literals in
// the duplicate-invocation guard: `-lt 15` (seconds) and `find … -mmin +5`,
// which is true for a file strictly older than 300 seconds.
const (
	subagentStopDedupTTL   = 15 * time.Second
	subagentStopDedupPrune = 5 * time.Minute
)

// subagentStopAgents is the identity allowlist, in the bash's own order.
//
// It is the WHOLE of ADR-026's identity contract: a subagent whose name is not
// on it is not an agent, so it is never logged, never scored and never gated.
// A slice rather than a map because membership is the only question asked of
// it and eighteen string comparisons cost less than a map allocation on a
// hook that runs after every subagent.
var subagentStopAgents = []string{
	"architect", "dba", "security", "api", "backend", "frontend", "qa", "sre",
	"platform", "docs", "pm", "ux", "data", "performance", "compliance",
	"mobile", "seo", "gtm",
}

// The three identity-source labels the ledger records. "structured" is the
// initial value and survives only if neither resolution path fired — which
// cannot happen, because an unresolved identity exits before the ledger is
// reached. Ported as-is rather than simplified: it is a recorded field, and
// the value that never appears today is the one a future edit would surface.
const (
	subagentStopSourceStructured = "structured"
	subagentStopSourceEvaluator  = "evaluator_output"
	subagentStopSourcePayload    = "payload"
)

// subagentStopSeverityLevels is the comparison table. A severity absent from
// it scores 0, which is BELOW every threshold — that is how "ok" allows.
var subagentStopSeverityLevels = map[string]int{"critical": 3, "warning": 2, "info": 1}

// subagentStopGatesHeadRe mirrors awk's `/^gates:/` — anchored at the start of
// a line, and NOT at a word boundary after it (`gates:x` matches too).
var subagentStopGatesHeadRe = regexp.MustCompile(`^gates:`)

// subagentStopSanitizeRe mirrors `tr -cd 'a-z0-9_-'`: DELETE every byte
// outside the allowlist rather than escaping or replacing it.
var subagentStopSanitizeRe = regexp.MustCompile(`[^a-z0-9_-]`)

// subagentStopEvent is the hook's own `gate_fired` ledger record, in the
// python dict's insertion order (which is what json.dumps writes).
//
// Lives here rather than in adapter/events.go because not one of these field
// names is host wire vocabulary — "agent", "severity", "threshold",
// "finding", "identity_source" are edikt's own ledger format, and none is on
// no-claude-leak-in-hook-core.sh's denylist. adapter.WorktreeEventJSON's own
// doc comment records the same judgement from the other side: it COULD have
// lived in a handler and was kept beside its two siblings for cohesion.
type subagentStopEvent struct {
	TS             string `json:"ts"`
	Event          string `json:"event"`
	Agent          string `json:"agent"`
	Severity       string `json:"severity"`
	Threshold      string `json:"threshold"`
	Finding        string `json:"finding"`
	FindingPrefix  string `json:"finding_prefix"`
	User           string `json:"user"`
	Email          string `json:"email"`
	IdentitySource string `json:"identity_source"`
}

// subagentStopLegacyEvent is the one-line record the legacy branch appends
// when a payload carries no evaluator_output.severity at all.
type subagentStopLegacyEvent struct {
	TS    string `json:"ts"`
	Event string `json:"event"`
	Hook  string `json:"hook"`
}

// subagentStopTelemetryEvent is the SECOND, differently shaped line the same
// gate fire writes — through templates/hooks/event-log.sh's edikt_log_event,
// which json.loads the four-field object the hook builds and folds
// type/at/by in afterwards, in that order.
type subagentStopTelemetryEvent struct {
	Agent         string `json:"agent"`
	Severity      string `json:"severity"`
	Threshold     string `json:"threshold"`
	FindingPrefix string `json:"finding_prefix"`
	Type          string `json:"type"`
	At            string `json:"at"`
	By            string `json:"by"`
}

func subagentStop(inv *Invocation) Result {
	root, err := subagentStopRoot(inv)
	if err != nil {
		return Result{}
	}
	configPath := filepath.Join(root, ".edikt", "config.yaml")

	// ── guard 1: only run in edikt projects ─────────────────────────────
	if !fileExists(configPath) {
		return Result{}
	}

	// ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────
	// The Go handler needs no interpreter, but the branch is BEHAVIOUR, and
	// on this hook it is the most consequential one in the tier: with
	// python3, a critical finding replies `{"decision": "block", …}`;
	// without it, `{"continue": true}` — not silence, the OPPOSITE verdict,
	// in bytes indistinguishable from "nothing was wrong". ADR-058:d05 ruled
	// that it must keep degrading to allow; what the banner adds is that the
	// allow now says it could not evaluate anything.
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: subagent-stop — python3 not found; this subagent output could NOT be evaluated against any quality gate")
		return Result{Response: adapter.Response{Text: adapter.NoPython3ContinueBanner(subagentStopNoPython3Message)}}
	}

	// ── duplicate-invocation guard (bok-services field bug #4) ──────────
	if subagentStopDedupSuppresses(inv, root) {
		return Result{}
	}

	// ── identity, and the exit that ADR-026 requires when there is none ──
	agent, identitySource := subagentStopIdentity(inv.Payload)
	if agent == "" {
		// `printf`, not `echo`: no trailing newline, unlike the no-python3
		// banner above.
		return Result{Response: adapter.Response{
			Text:              adapter.ContinueTrueReply,
			NoTrailingNewline: true,
		}}
	}

	home := os.Getenv("HOME")

	// ── severity ────────────────────────────────────────────────────────
	severity := "info"
	anomaly := ""
	finding := ""
	if evalSeverity := subagentStopEvalSeverity(inv.Payload); evalSeverity != "" {
		switch evalSeverity {
		case "critical", "warning", "info", "ok":
			severity = evalSeverity
		default:
			// The nonconforming value is attacker-influenceable, so it is
			// shown only after `tr -cd`/`cut -c1-32`. Note this DELETES the
			// disallowed bytes rather than escaping them, so "<script>"
			// becomes "script" and the length cap is applied to what
			// survives.
			san := subagentStopSanitizeRe.ReplaceAllString(evalSeverity, "")
			san = subagentStopCutBytes(san, 32)
			if san == "" {
				san = "<empty>"
			}
			anomaly = "nonconforming severity value: " + san
		}
		// `str(desc)[:120]` — sliced by CODE POINT, then the trailing
		// newlines command substitution strips off python's print().
		finding = strings.TrimRight(truncateRunes(inv.Payload.EvaluatorFinding, 120), "\n")
	} else {
		// Legacy unstructured payload. Severity stays "info" and the fact is
		// recorded in three places: the ledger, stderr, and the anomaly that
		// fails a GATE-LISTED agent closed below.
		_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
		subagentStopAppendEvent(home, subagentStopLegacyEvent{
			TS: subagentStopNow(), Event: "legacy_payload", Hook: inv.Name,
		})
		fmt.Fprintln(inv.Stderr, "warn: legacy evaluator payload (no evaluator_output.severity); severity defaulted to info")
		anomaly = "missing severity (no evaluator_output.severity in the payload)"
	}

	// ── session signals ─────────────────────────────────────────────────
	// ADR-050's lesson: a channel that writes nothing anywhere is one nobody
	// can tell apart from a channel that had nothing to say. This line is
	// what makes "a subagent ran and was scored" observable at all.
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	subagentStopAppendLine(filepath.Join(home, ".edikt", "session-signals.log"),
		subagentStopNow()+" AGENT "+agent+" severity="+severity)

	// ── quality gate logic ──────────────────────────────────────────────
	configBytes, _ := os.ReadFile(configPath) //nolint:gosec // project-local config
	// `grep -q 'quality-gates: false'` is a plain substring search over the
	// whole file, not a YAML lookup: a commented-out line matches too.
	if strings.Contains(string(configBytes), "quality-gates: false") {
		return Result{}
	}

	isGate := subagentStopIsGate(configBytes, agent)
	threshold := subagentStopThreshold(root, agent)

	// ADR-048 (amends ADR-026) / INV-011: for a GATE-LISTED agent, a missing
	// or nonconforming severity maps to the agent's own threshold. An
	// anomalous verdict field must not DISARM the gate, which is what
	// mapping it to "info" used to do. Non-gate agents are untouched — that
	// boundary is the non-gate-nonconforming scenario.
	if isGate && anomaly != "" {
		severity = threshold
		if finding == "" {
			finding = "severity fail-closed: " + anomaly
		}
	}

	if !isGate || !subagentStopShouldBlock(severity, threshold) {
		return Result{Response: adapter.Response{
			Text:              adapter.ContinueTrueReply,
			NoTrailingNewline: true,
		}}
	}

	// ── an existing override for this exact finding suppresses the block ─
	prefix := subagentStopCutLines(finding, 80)
	overrides := filepath.Join(home, ".edikt", "gate-overrides.jsonl")
	if fileExists(overrides) && subagentStopOverrideMatches(overrides, agent, prefix) {
		return Result{Response: adapter.Response{
			Text:              adapter.ContinueTrueReply,
			NoTrailingNewline: true,
		}}
	}

	// ── the gate fires ──────────────────────────────────────────────────
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	subagentStopAppendEvent(home, subagentStopEvent{
		TS:             subagentStopNow(),
		Event:          "gate_fired",
		Agent:          agent,
		Severity:       severity,
		Threshold:      threshold,
		Finding:        finding,
		FindingPrefix:  prefix,
		User:           gitConfigValue("user.name"),
		Email:          gitConfigValue("user.email"),
		IdentitySource: identitySource,
	})

	// Telemetry, UNCONDITIONAL since Phase 23.
	//
	// This was gated on `fileExists($HOME/.edikt/hooks/event-log.sh)`, which
	// faithfully reproduced bash's `if type edikt_log_event` — the function
	// existed only because the hook had sourced that file at entry. Phase 23
	// deletes the file, and leaving the gate would have degraded the condition
	// to "does a file we no longer ship happen to still exist": a fresh
	// install would write one ledger line where an upgraded one wrote two,
	// silently and forever, with the difference visible nowhere.
	//
	// Removing the gate rather than the record, because the record cannot be
	// removed: nine of subagent-stop's seventeen corpus recordings pin the
	// two-line shape, and a MIGRATED row can never be re-recorded against
	// bash. Corpus-neutral by construction — `_base` carries the helper, so
	// the gate was true in every one of those recordings and unconditional
	// produces the same bytes.
	logEventTightenMode(home)
	subagentStopAppendEvent(home, subagentStopTelemetryEvent{
		Agent: agent, Severity: severity, Threshold: threshold,
		FindingPrefix: prefix, Type: "gate_fired",
		At: subagentStopNow(), By: gitConfigValue("user.email"),
	})

	msg := "🔴 BLOCKED — " + agent + " gate fired (severity: " + severity +
		" ≥ threshold: " + threshold + ")\n   To change threshold: .edikt/config.yaml  gates." +
		agent + ": " + threshold
	if anomaly != "" {
		// (ref: ADR-048 / INV-011 — fail-closed to the threshold on an anomaly)
		msg += "\n   severity was fail-closed to the threshold: " + anomaly
	}
	return Result{Response: adapter.Response{Block: true, Message: msg}}
}

// subagentStopRoot resolves the ONE project root this handler uses — AC-18.3.
//
// WHAT THE CRITERION ASKS FOR: no relative `"."` default. The bash resolves
// its gates block against `os.environ.get("EDIKT_PROJECT_ROOT", ".")`, which
// means a different directory depending on where the host happened to spawn
// the hook, and which can therefore disagree with the cwd-relative reads the
// opt-out and the is-a-gate scan use. This returns ONE absolute root, used by
// all three, with the env var still honoured when explicitly set.
//
// WHAT IT ANCHORS ON, AND WHY NOT inv.Root(). The seam's own Root() prefers
// the working directory the HOST reported. Not used here, for the same reason
// stophook.go states in its header (fact 1) and pins with a foreign-cwd
// scenario: the payload's cwd is not authoritative — adapter.Payload.Cwd's own
// doc comment says so — and on this hook, trusting it is a security downgrade
// rather than a neutral choice. A reported cwd that no longer exists (every
// one of the four committed subagent-stop payload fixtures carries one:
// "/tmp/edikt-test-project", "${HOME}/projects/greenfield" unexpanded) makes
// the handler find no .edikt/config.yaml and exit SILENTLY — which on this
// hook means a critical finding from a gate-listed agent stops blocking, with
// output byte-identical to a project that has no gates at all. That is the
// same class of defect this criterion exists to remove, arriving from the
// other direction, so the anchor is the process's own working directory:
// exactly what the bash's relative paths resolve against, and what the
// foreign-cwd corpus scenario now pins.
//
// No host wire vocabulary is named here — the leak gate's subject is field
// spellings and vendor names, and this function reads neither.
func subagentStopRoot(inv *Invocation) (string, error) {
	root := os.Getenv("EDIKT_PROJECT_ROOT")
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = cwd
	}
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("subagent-stop: no project root resolved")
	}
	if !filepath.IsAbs(root) {
		abs, aerr := filepath.Abs(root)
		if aerr != nil {
			return "", aerr
		}
		root = abs
	}
	_ = inv
	return root, nil
}

// subagentStopDedupSuppresses reports whether this exact payload has already
// been handled inside the TTL, and writes (and prunes) the token where the
// bash does.
func subagentStopDedupSuppresses(inv *Invocation, root string) bool {
	if os.Getenv("EDIKT_DISABLE_HOOK_DEDUP") == "1" {
		return false
	}
	if !dirExists(filepath.Join(root, ".edikt")) {
		return false
	}
	dedupDir := filepath.Join(root, ".edikt", "state", ".hook-dedup")
	_ = os.MkdirAll(dedupDir, 0o755)

	// Keyed on the RAW payload bytes, which never cross the seam — see
	// adapter.Payload.PayloadDigest for why only a digest does.
	key := inv.Payload.PayloadDigest(inv.Name + ":")
	if len(key) > 40 {
		key = key[:40]
	}
	if key == "" || !dirExists(dedupDir) {
		return false
	}
	token := filepath.Join(dedupDir, key)
	if fi, err := os.Stat(token); err == nil && fi.Mode().IsRegular() {
		// A NEGATIVE age (a token stamped in the future) is UNKNOWN, never
		// fresh: suppressing on it would silence a real emission (INV-013),
		// which is why the bash tests `-ge 0` as well as `-lt 15`.
		age := time.Since(fi.ModTime())
		if age >= 0 && age < subagentStopDedupTTL {
			return true
		}
	}
	_ = truncateOrCreate(token)
	subagentStopPruneDedup(dedupDir)
	return false
}

// subagentStopPruneDedup mirrors `find "$dir" -type f -mmin +5 -delete`.
func subagentStopPruneDedup(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil || !info.Mode().IsRegular() {
			continue
		}
		if time.Since(info.ModTime()) > subagentStopDedupPrune {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// subagentStopIdentity resolves WHO this subagent was, and from where.
//
// Two sources, in priority order, each filtered through the same allowlist:
// the structured evaluator verdict, then the host's own payload fields. The
// content-keyword fallback that used to sit under both was removed in v0.6.0
// — a subagent that mentioned "security" in its prose could fire the security
// gate — so an unresolvable identity is now a clean exit rather than a guess.
func subagentStopIdentity(p adapter.Payload) (agent, source string) {
	if name := subagentStopCanonical(subagentStopEvalAgent(p)); name != "" {
		return name, subagentStopSourceEvaluator
	}
	// `for k in (...): v = d.get(k); if isinstance(v, str) and v: print(...);
	// break` — the FIRST non-empty value wins and the loop stops there, even
	// when that value turns out not to be allowlisted. A present-but-empty
	// field is skipped rather than winning.
	for _, v := range []string{p.AgentNameField, p.SubagentTypeField, p.ToolName, p.AgentField} {
		if v == "" {
			continue
		}
		if name := subagentStopCanonical(v); name != "" {
			return name, subagentStopSourcePayload
		}
		break
	}
	return "", subagentStopSourceStructured
}

// subagentStopCanonical applies `.strip().lower()` and the allowlist.
func subagentStopCanonical(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return ""
	}
	for _, allowed := range subagentStopAgents {
		if v == allowed {
			return allowed
		}
	}
	return ""
}

// subagentStopEvalAgent and subagentStopEvalSeverity reproduce the bash's own
// round trip through ONE string:
//
//	EVAL_OUT=$(… print(agent + "|" + severity))
//	EVAL_AGENT="${EVAL_OUT%%|*}"      # up to the FIRST '|'
//	EVAL_SEVERITY="${EVAL_OUT##*|}"   # after the LAST '|'
//
// A pipe inside either value therefore moves the split, and command
// substitution has already stripped the trailing newlines print() added. Both
// are ported rather than "cleaned up": a severity containing a pipe resolves
// to its own tail today, and that decides whether a gate fires.
func subagentStopEvalAgent(p adapter.Payload) string {
	joined := subagentStopEvalJoined(p)
	if i := strings.Index(joined, "|"); i >= 0 {
		return joined[:i]
	}
	return joined
}

func subagentStopEvalSeverity(p adapter.Payload) string {
	joined := subagentStopEvalJoined(p)
	if i := strings.LastIndex(joined, "|"); i >= 0 {
		return joined[i+1:]
	}
	return joined
}

func subagentStopEvalJoined(p adapter.Payload) string {
	agent := strings.ToLower(strings.TrimSpace(p.EvaluatorAgent))
	sev := strings.ToLower(strings.TrimSpace(p.EvaluatorSeverity))
	return strings.TrimRight(agent+"|"+sev, "\n")
}

// subagentStopIsGate mirrors:
//
//	awk '/^gates:/{found=1} found && /<agent>/{print "yes"; exit}' .edikt/config.yaml
//
// Two awk rules over the SAME record, which is why the `gates:` line itself is
// also searched for the agent name. Deliberately sloppy in two ways this port
// keeps: the search is unanchored (any line containing the name, including a
// COMMENT, and including lines after the gates block has ended), and it never
// stops at the end of the block. The threshold parser below is indentation-
// and comment-aware and does NOT agree with it — see the
// threshold-default-comment scenario, which exists because that disagreement
// decides whether a gate fires.
func subagentStopIsGate(configBytes []byte, agent string) bool {
	found := false
	for _, line := range splitLines(configBytes) {
		if subagentStopGatesHeadRe.MatchString(line) {
			found = true
		}
		if found && strings.Contains(line, agent) {
			return true
		}
	}
	return false
}

// subagentStopThreshold resolves the severity bar for this agent:
//
//	EDIKT_GATE_SEVERITY_THRESHOLD > gates.<agent> > gates.default > "critical"
//
// The config scan is the bash's own stdlib-only parser (INV-001 — no YAML
// library in a hook), reproduced including its quirks: blank and comment
// lines are skipped, indent is counted in SPACES only, a zero-indent line
// opens or closes the gates block by prefix, and quotes are stripped from
// both key and value. Anything unreadable keeps the fail-closed "critical".
func subagentStopThreshold(root, agent string) string {
	if env := os.Getenv("EDIKT_GATE_SEVERITY_THRESHOLD"); env != "" {
		return subagentStopValidThreshold(env)
	}
	// AC-18.3: the config read is the ONE root this handler resolved, never
	// a second, independently-defaulted path. EDIKT_PROJECT_ROOT still
	// selects that root (see subagentStopRoot); what is gone is its "."
	// default and the split it created.
	body, err := os.ReadFile(filepath.Join(root, ".edikt", "config.yaml")) //nolint:gosec // project-local config
	if err != nil {
		return "critical"
	}
	gates := map[string]string{}
	inGates := false
	for _, line := range splitLines(body) {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimLeft(line, " \t\n\v\f\r"), "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		stripped := strings.TrimSpace(line)
		if indent == 0 {
			inGates = strings.HasPrefix(stripped, "gates:")
			continue
		}
		if inGates && strings.Contains(stripped, ":") {
			k, v, _ := strings.Cut(stripped, ":")
			gates[strings.Trim(strings.TrimSpace(k), "\"'")] = strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	if v, ok := gates[agent]; ok {
		return subagentStopValidThreshold(v)
	}
	if v, ok := gates["default"]; ok {
		return subagentStopValidThreshold(v)
	}
	return "critical"
}

// subagentStopValidThreshold mirrors the `case` that follows every resolution
// path: anything outside the three known bars is "critical", the strictest.
func subagentStopValidThreshold(v string) string {
	switch v {
	case "critical", "warning", "info":
		return v
	}
	return "critical"
}

// subagentStopShouldBlock mirrors:
//
//	levels.get(sev, 0) >= levels.get(thresh, 3) and levels.get(sev, 0) > 0
//
// What makes "ok" — and any severity outside the table — allow is the FIRST
// half: it scores 0, and every threshold that reaches here has already been
// validated to critical/warning/info, so the bar is never lower than 1.
//
// The `> 0` clause is therefore unreachable as written, in the bash exactly
// as here, and it is ported rather than dropped BECAUSE it is unreachable:
// its only effect would be under a threshold outside the table, which the
// preceding validation currently makes impossible. Removing it would silently
// couple this comparison to that validation staying where it is. Said plainly
// so nobody mistakes it for a guard the corpus pins — no scenario can
// discriminate it, and none claims to.
func subagentStopShouldBlock(severity, threshold string) bool {
	sev := subagentStopSeverityLevels[severity]
	thresh, ok := subagentStopSeverityLevels[threshold]
	if !ok {
		thresh = 3
	}
	return sev >= thresh && sev > 0
}

// subagentStopOverrideMatches reports whether $HOME/.edikt/gate-overrides.jsonl
// already carries a human's decision to accept THIS agent's THIS finding.
//
// Same-record matching, never same-file: agent and finding_prefix must appear
// on one line together, so one agent's override cannot silence another's
// finding.
//
// DOCUMENTED DIVERGENCE, and it is a decision rather than an oversight. The
// bash catches a malformed JSON line and skips it, but a line that is VALID
// JSON and not an object (`5`, `"x"`, `[1]`) raises AttributeError on
// `rec.get`, which escapes the loop, prints a traceback to the hook's stderr
// and leaves MATCHED empty — so the gate fires. This returns false on that
// shape WITHOUT scanning further, which reaches the same verdict (the gate
// fires) by the same reasoning ADR-063 gives for failing open in Go: the
// traceback is not reproduced, and no later record is allowed to suppress a
// block the bash would have emitted.
func subagentStopOverrideMatches(path, agent, prefix string) bool {
	body, err := os.ReadFile(path) //nolint:gosec // per-user edikt state
	if err != nil {
		return false
	}
	for _, line := range splitLines(body) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &rec) != nil {
			var any any
			if json.Unmarshal([]byte(line), &any) != nil {
				continue // malformed — `except json.JSONDecodeError: continue`
			}
			return false // valid JSON, wrong shape: see this function's own note
		}
		// A non-string value compares unequal rather than raising, exactly as
		// python's `rec.get('agent') == agent` does against an int or a list.
		if subagentStopJSONString(rec["agent"]) == agent &&
			subagentStopJSONString(rec["finding_prefix"]) == prefix {
			return true
		}
	}
	return false
}

// subagentStopJSONString decodes raw as a string, or returns a value that
// cannot equal any real one when it is absent or of another type.
func subagentStopJSONString(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return "\x00<absent>\x00"
	}
	return s
}

// subagentStopCutBytes mirrors `cut -c1-N` on a single line: under LC_ALL=C —
// which the corpus recorder sets and which GNU cut applies unconditionally —
// `-c` counts BYTES, not characters.
func subagentStopCutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// subagentStopCutLines mirrors `echo "$FINDING" | cut -c1-N`, which truncates
// EACH LINE rather than the string as a whole, and the `$(…)` around it,
// which strips the trailing newlines back off.
func subagentStopCutLines(s string, n int) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = subagentStopCutBytes(l, n)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// subagentStopNow mirrors `date -u +"%Y-%m-%dT%H:%M:%SZ"` and python's
// `datetime.utcnow().strftime(...)` — the same format from both sides.
func subagentStopNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// subagentStopAppendEvent appends one JSON line to $HOME/.edikt/events.jsonl.
//
// Built from a struct through encoding/json, never concatenated: INV-003
// exists because the values here — a finding description seeded by a file the
// agent read — are attacker-influenceable, and a struct cannot express the
// unterminated string a printf format can.
func subagentStopAppendEvent(home string, v any) {
	body, err := adapter.MarshalPythonCompact(v)
	if err != nil {
		return
	}
	subagentStopAppendLine(filepath.Join(home, ".edikt", "events.jsonl"), string(body))
}

// subagentStopAppendLine appends one line plus its newline, creating the file
// with bash's own `>>` mode when it does not exist.
func subagentStopAppendLine(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // matches bash `>>` semantics
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(line + "\n")
}

// logEventTightenMode reproduces the ONE side effect of edikt_log_event that
// is not the line itself: create-with-0600 if absent, re-chmod to 0600 if
// present (MED-8 — the ledger carries git email, findings and session
// metadata, and the default umask would leave it world-readable).
func logEventTightenMode(home string) {
	logFile := filepath.Join(home, ".edikt", "events.jsonl")
	if _, err := os.Stat(logFile); err != nil {
		f, cerr := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // matches event-log.sh's own MED-8 tightened mode
		if cerr != nil {
			return
		}
		_ = f.Close()
		return
	}
	_ = os.Chmod(logFile, 0o600) //nolint:gosec // matches event-log.sh's own MED-8 tightened mode
}

// gitConfigValue mirrors `git config <key> 2>/dev/null || echo "unknown"`.
// The fallback fires on a NON-ZERO exit only; a successful lookup that
// returns empty stays empty rather than being upgraded to "unknown".
func gitConfigValue(key string) string {
	out, err := exec.Command("git", "config", key).Output() //nolint:gosec // fixed argv, key is a package constant
	if err != nil {
		return "unknown"
	}
	return strings.TrimRight(string(out), "\r\n")
}
