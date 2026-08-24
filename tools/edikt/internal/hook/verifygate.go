package hook

// verifygate.go — the Go replacement for templates/hooks/verify-gate.sh
// (PLAN-v080-reliability-core Phase 21). The INV-009 evidence-read gate.
//
// WHAT IT GUARDS. A completion claim is a write that asserts work is finished:
// a sidecar's `passes: false` flipped true, a plan progress row moved to
// `done`, an AC checkbox ticked. The gate refuses one unless a verify report
// for that artifact was READ in the same turn AND that report passes. Its
// whole purpose is to stop a claim being written from confidence rather than
// from evidence.
//
// ITS DENY CONTRACT IS NOT pre-tool-use's, AND THE DIFFERENCE IS DELIBERATE.
// Both hooks deny through `permissionDecision`, but this one routes EVERY
// would-deny through a four-value posture dial (`features.evidence-gate`)
// that pre-tool-use has no equivalent of. A port that unified the two — or
// that "tidied" a deny site to bypass the dial — would take an operator's
// `warn` and silently enforce it as `block`. ADR-038 §5b owns that dial; it is
// not an inconsistency waiting to be cleaned up.
//
// NO python3 GATE, for Phase 20's reason and with Phase 20's evidence. The
// bash original degraded loudly when python3 was missing because its gate WAS
// a python heredoc; the banner it printed ("the evidence-read gate could NOT
// evaluate this write") was true of bash and is FALSE of this handler, which
// has no python3 dependency. Reproducing it for byte-parity would stand a
// security control down on a host where nothing is wrong — ADR-063:d02 exists
// to forbid exactly that. See
// docs/internal/issues/go-handlers-gate-on-a-dependency-they-do-not-have.md.
//
// TWO BASH REGEXES ARE NOT PORTED BECAUSE BASH NEVER USED THEM. PLAN_DONE_RE
// (an arrow-transition matcher) and AC_BOX_OPEN_RE (the unticked-checkbox
// counterpart) are both compiled and never called in verify-gate.sh. Carrying
// them across would have implied a detection path that has never once run.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	// PAYLOAD-TOLERANT ON PURPOSE. The dispatcher's default reply to an
	// undecodable payload is silence, which the host reads as allow — and the
	// payload is the one input an attacker fully controls. Bash denies here
	// (INV-011). Opting in is what keeps that true across the seam instead of
	// losing it to a default nobody chose for this hook.
	RegisterPayloadTolerant("verify-gate", verifyGate)

	// A CRASH IS NOT A CLEAN BILL OF HEALTH. The dispatcher's default reply to
	// a recovered panic is silence, which the host reads as allow — right for
	// an injector, inverted for a gate. A verify-gate that panicked did not
	// find the write safe; it failed to look at it, and INV-013 forbids a
	// control reporting an outcome it did not achieve.
	//
	// So a crash degrades exactly the way an unparseable payload does, through
	// the same posture dial: refused under `block`, announced under `warn`,
	// silent under the two postures that turn the gate off.
	RegisterPanicFallback("verify-gate", verifyGatePanicReply)
}

// verifyGatePanicReply is the reply when the gate itself crashes.
//
// Deliberately minimal: it re-reads the posture and builds a constant message.
// Every input that could have caused the panic is left untouched, because the
// one job of a fallback is to not be the second thing that fails.
func verifyGatePanicReply() adapter.Response {
	posture := resolvePosture(nil)
	if posture == postureEducate || posture == postureDisabled {
		// These two postures mean the gate was never going to decide this
		// write. A crash inside a hook that was already standing down is not
		// news for the model, and announcing it would make `disabled` louder
		// than `warn`.
		return adapter.Response{ContinueTrue: true}
	}
	const msg = "verify-gate: the evidence-read gate CRASHED while checking this write, so " +
		"the completion-claim check did not run. This is not a pass — nothing was verified. " +
		"If this is a completion claim (sidecar passes, plan phase, AC checkbox), re-run the " +
		"verify and Read the report before claiming completion, and report the crash."
	if posture == postureBlock {
		return adapter.Response{Decision: adapter.DecisionDeny, Reason: msg}
	}
	return adapter.Response{Context: msg + " `features.evidence-gate` is `warn`, so the write is " +
		"allowed; under `block` it would be refused."}
}

// Posture values. `warn` is what an absent or unrecognised setting resolves
// to; see resolvePosture for why unrecognised does NOT resolve to block.
const (
	postureBlock    = "block"
	postureWarn     = "warn"
	postureEducate  = "educate"
	postureDisabled = "disabled"
)

// maxVerifyReportBytes caps a verify-report read. A report is a small summary
// object; anything larger is not one, and refusing to buffer it is cheaper
// than trusting the directory it came from.
const maxVerifyReportBytes = 4 << 20

var (
	passesTrueRe = regexp.MustCompile(`(?m)^\s*passes\s*:\s*true\b`)

	// A TABLE CELL, NOT A WORD. This was `\bdone\b` anywhere in the content
	// until F-053, which denied any plan write containing the word at all —
	// including "revisit once the migration is done" and, absurdly, "Nothing
	// is done yet; all phases pending". Survivable while the deny was inert;
	// a blocked write once ADR-061 made it real.
	planDoneCellRe = regexp.MustCompile("(?i)\\|\\s*(?:\\*\\*|`|_)*\\s*done\\s*(?:\\*\\*|`|_)*\\s*\\|")
	planPendingRe  = regexp.MustCompile(`(?i)\b(pending|in[_-]progress)\b`)

	acBoxDoneRe = regexp.MustCompile(`(?i)\[\s*x\s*\][^\n]{0,80}?\b(?:S?AC)-\d+|\b(?:S?AC)-\d+[^\n]{0,80}?\[\s*x\s*\]`)

	// The id must be usable as a state-file key. Anything else is refused
	// rather than sanitised: a mangled id would look up the wrong artifact's
	// evidence, which is worse than looking up none.
	idAllowlistRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,128}$`)

	reportNeedleRe = regexp.MustCompile(`(adr|inv|gl|spec|prd|plan)-\d+`)

	actorAllowlistRe = regexp.MustCompile(`^(migrate|compile|upgrade)$`)

	evidenceGateLineRe = regexp.MustCompile(`^\s+evidence-gate:\s*(.*)$`)
)

func verifyGate(inv *Invocation) Result {
	posture := resolvePosture(inv)

	// `disabled` — short-circuits before any state I/O and writes NO audit
	// record. ADR-038 §5b makes it identical to the env kill-switch.
	if posture == postureDisabled {
		return Result{Response: adapter.Response{ContinueTrue: true}}
	}

	// `educate` — the gate declines to run but RECORDS that it declined. The
	// record is the only thing distinguishing it from `disabled`; without it
	// the two postures are byte-identical and the enum's four-way distinction
	// is unfalsifiable.
	if posture == postureEducate {
		auditGate("posture.educate", posture)
		return Result{Response: adapter.Response{ContinueTrue: true}}
	}

	// Tier B bypass — the ad-hoc operator escape hatch, checked before any
	// state I/O so the gate is fully short-circuited.
	if os.Getenv("EDIKT_DISABLE_VERIFY_GATE") == "1" {
		return Result{Response: adapter.Response{ContinueTrue: true}}
	}

	// Tier A bypass — the allowlisted-actor signal, normalised NFKC +
	// casefold + strip before comparison (INV-006). An unknown value is
	// treated as no actor, so the gate still applies.
	if raw := os.Getenv("EDIKT_HOOK_ACTOR"); raw != "" {
		actor := strings.ToLower(strings.TrimSpace(norm.NFKC.String(raw)))
		if actorAllowlistRe.MatchString(actor) {
			appendJSONL("verify-gate.jsonl", actorRecord{
				TS:    utcNow(),
				Event: "bypass.actor",
				Actor: actor,
			})
			return Result{Response: adapter.Response{ContinueTrue: true}}
		}
	}

	g := gate{posture: posture, stderr: inv.Stderr}

	// UNINSPECTABLE INPUT IS NOT A PASS (INV-011). Two distinct facts reach
	// the same conclusion: bytes that are not JSON (PayloadErr), and no bytes
	// at all (Absent). This hook is registered for write tools only, so
	// either one means a write we cannot look at — and waving it through
	// would make a broken payload the cheapest way past the gate.
	if inv.PayloadErr != nil || inv.Payload.Absent() {
		return g.deny("verify-gate: could not parse the hook payload, so the " +
			"completion-claim check cannot run — denying the write. " +
			"If you are the human operator, the bypass procedure is in " +
			"edikt's hook documentation.")
	}

	p := inv.Payload
	if p.ToolName != "Edit" && p.ToolName != "Write" {
		return allowPlain()
	}

	filePath := p.FilePath
	if filePath == "" {
		filePath = p.Path
	}
	if filePath == "" {
		return allowPlain()
	}

	// NFKC + strip, but NOT casefold: paths are case-sensitive on Linux, and
	// folding here would put two genuinely different files under one name.
	// The derived report id IS casefolded, further down, because it is a
	// state-file key rather than a path.
	normPath := strings.TrimSpace(norm.NFKC.String(filePath))
	if normPath == "" {
		return allowPlain()
	}

	// F-050 — RESOLVE SYMLINKS BEFORE TAKING THE BASENAME. Without it a
	// symlink whose own name matches no completion-claim shape but whose
	// TARGET is a real sidecar, plan or spec slips past every basename
	// comparison below. On a resolution failure the literal path is used
	// rather than denying, consistent with this hook's fail-open contract for
	// conditions it cannot resolve.
	base := filepath.Base(resolvePath(normPath))

	var newText, oldText string
	if p.ToolName == "Edit" {
		newText, oldText = p.NewString, p.OldString
	} else {
		newText = p.Content
	}

	claim := detectClaim(p.ToolName, base, newText, oldText)
	if claim == "" {
		return allowPlain()
	}

	reportID := reportIDFromBasename(base)
	if !idAllowlistRe.MatchString(reportID) {
		// Refuse to look up an id we cannot safely use as a key, so a
		// path-injection attempt does not sail past the gate on a lookup that
		// silently matched nothing.
		reportID = ""
	}

	reads := readEvidenceReads(inv)

	if reportID != "" && truthy(reads[reportID]) {
		reportPath, passing := latestReportVerdict(reportID)
		if !passing {
			detail := "no verify report matching this artifact exists under .edikt/state/verify/"
			if reportPath != "" {
				detail = "the matching verify report (" + filepath.Base(reportPath) +
					") is FAILING or unreadable"
			}
			// INV-011 — A RECORDED READ OF A FAILING REPORT IS NOT EVIDENCE.
			// The read proves someone looked; it does not prove what they
			// saw. Without this branch the gate would be satisfied by the act
			// of reading a report that says the work does not work.
			return g.deny("verify-gate: completion-claim on " + base +
				" — an evidence read was recorded, but " + detail + ". " +
				"Re-run the verify until it passes, Read the fresh " +
				"report, then make the claim.")
		}
		// Consume the read so one verification cannot unlock two claims.
		consumeEvidenceRead(inv, reads, reportID)
		return allowPlain()
	}

	return g.deny("verify-gate: completion-claim detected on " + base +
		" without a fresh verify-report read in the same turn. " + hintFor(claim, reportID))
}

// gate carries the posture through the deny path so no call site can reach a
// deny that skips the dial.
type gate struct {
	posture string
	stderr  interface{ Write([]byte) (int, error) }
}

// deny renders the posture-appropriate refusal.
//
// ROUTED HERE RATHER THAN AT EACH CALL SITE so that every would-deny is
// covered by the dial — including the unparseable-payload deny, which is the
// one most easily written as a bare early return. A posture that silently
// still blocks in some cases is not a dial.
func (g gate) deny(message string) Result {
	if g.posture == postureWarn {
		return g.warn(message)
	}
	auditGate("gate.deny", g.posture)
	// ADR-061 — THE DENY CHANNEL. This hook denied with
	// `{continue: false, decision: "deny", ...}` as ADR-038 originally
	// mandated. None of those keys prevents the tool call: measured 14/14
	// across 118 production transcripts, the write LANDED and the turn was
	// killed afterwards. The gate reported as enforcing the whole time.
	//
	// The reason rides on the model-facing channel, not the operator-facing
	// one: the MODEL is the actor that has to revise the write.
	return Result{Response: adapter.Response{
		Decision: adapter.DecisionDeny,
		Reason:   message,
	}}
}

// warn is the `warn` posture's channel: detect, record, allow.
//
// ADR-062 §4 — ADR-038 §5b said to warn on the operator-facing channel.
// ADR-061 §1 ruled that channel renders to the user's screen only and MUST
// NOT carry text whose intended reader is the model. The actor that must
// revise a completion claim is the model, so a warning delivered there could
// not shape the behaviour this posture exists to shape — and §5b's
// ramp-to-block criterion would have been counting warnings the agent never
// received.
func (g gate) warn(message string) Result {
	// ADR-062 §7: the ledger the doctor-driven ramp to `block` consumes.
	// Without a producer the ramp counts an empty file forever.
	appendJSONL("evidence-gate-warns.jsonl", warnRecord{
		TS:     utcNow(),
		Event:  "gate.warn",
		Reason: message,
	})
	auditGate("gate.warn", g.posture)
	return Result{Response: adapter.Response{
		Context: message + " This is a WARNING: `features.evidence-gate` is " +
			"`warn`, so the write is allowed. Under `block` it would be refused.",
	}}
}

func allowPlain() Result {
	return Result{Response: adapter.Response{ContinueTrue: true}}
}

// resolvePosture reads `features.evidence-gate` from .edikt/config.yaml.
//
// RESOLVED WITHOUT ANY OF THE MACHINERY THE GATE ITSELF NEEDS, which is the
// point ADR-062 §2 makes about the bash original: the two postures whose whole
// purpose is turning the gate OFF must not depend on anything that could be
// missing. The Go rewrite has no such dependency, but the ORDERING property
// still matters — posture is resolved before any state I/O, so `disabled`
// short-circuits before the hook can touch a disk it may not be able to write.
//
// An unrecognised value resolves to `warn` and says so on stderr. It
// deliberately does NOT resolve to `block`: silently upgrading an operator's
// typo to the highest-friction posture is a worse failure than running the
// default ADR-038 already names as safe.
func resolvePosture(inv *Invocation) string {
	raw := readPostureSetting()
	raw = strings.ToLower(strings.Trim(raw, "\"' \t\r"))
	switch raw {
	case postureBlock, postureWarn, postureEducate, postureDisabled:
		return raw
	case "":
		return postureWarn
	default:
		if inv != nil && inv.Stderr != nil {
			fmt.Fprintf(inv.Stderr, "verify-gate: unrecognised features.evidence-gate '%s'; using warn\n", raw)
		}
		return postureWarn
	}
}

// readPostureSetting scans for `evidence-gate:` inside the top-level
// `features:` block, mirroring the bash original's awk: a line with no leading
// whitespace ends the block, so a same-named key under a different parent is
// not read.
func readPostureSetting() string {
	b, err := os.ReadFile(filepath.Join(".edikt", "config.yaml"))
	if err != nil {
		return ""
	}
	inBlock := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "features:") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if line != "" && !strings.ContainsAny(line[:1], " \t#") {
			return ""
		}
		if m := evidenceGateLineRe.FindStringSubmatch(line); m != nil {
			// awk printed $2 — the second whitespace-separated field — so a
			// trailing comment is dropped rather than read as part of the
			// value.
			return strings.Fields(m[1] + " ")[0]
		}
	}
	return ""
}

func detectClaim(tool, base, newText, oldText string) string {
	if isSidecarPassesFlip(tool, base, newText, oldText) {
		return "sidecar"
	}
	if isPlanProgressDone(tool, base, newText, oldText) {
		return "plan"
	}
	if isACCheckboxFlip(base, newText, oldText) {
		return "ac"
	}
	return ""
}

func isSidecarPassesFlip(tool, base, newText, oldText string) bool {
	if !strings.HasSuffix(base, ".edikt.yaml") {
		return false
	}
	if tool == "Write" {
		return passesTrueRe.MatchString(newText)
	}
	// An edit whose old text ALREADY said `passes: true` is idempotent — a
	// comment tweak next to the key, not a claim — and is allowed.
	return passesTrueRe.MatchString(newText) && !passesTrueRe.MatchString(oldText)
}

func isPlanProgressDone(tool, base, newText, oldText string) bool {
	low := strings.ToLower(base)
	if !strings.HasPrefix(low, "plan-") ||
		!(strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".yaml")) {
		return false
	}
	if tool == "Write" {
		return planDoneCellRe.MatchString(newText)
	}
	// A transition, not a state: the row must have carried a pending marker
	// before and must not carry one after.
	return planDoneCellRe.MatchString(newText) &&
		planPendingRe.MatchString(oldText) &&
		!planPendingRe.MatchString(newText)
}

func isACCheckboxFlip(base, newText, oldText string) bool {
	low := strings.ToLower(base)
	if !(strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".yaml")) {
		return false
	}
	return acBoxDoneRe.MatchString(newText) && !acBoxDoneRe.MatchString(oldText)
}

func reportIDFromBasename(name string) string {
	stem := name
	switch {
	case strings.HasSuffix(stem, ".edikt.yaml"):
		stem = stem[:len(stem)-len(".edikt.yaml")]
	case strings.HasSuffix(stem, ".yaml"):
		stem = stem[:len(stem)-len(".yaml")]
	case strings.HasSuffix(stem, ".md"):
		stem = stem[:len(stem)-len(".md")]
	}
	return strings.ToLower(strings.TrimSpace(norm.NFKC.String(stem)))
}

func hintFor(kind, reportID string) string {
	switch kind {
	case "sidecar":
		id := orPlaceholder(reportID, "<id>")
		return "Run `bin/edikt verify gov " + id +
			"` and Read the resulting `.edikt/state/verify/" + id +
			".json` before flipping `passes: true`."
	case "plan":
		return "Run `bin/edikt verify " + orPlaceholder(reportID, "<plan-id>") +
			" --phase <N>` and Read the resulting report before marking the row `done`."
	default:
		return "Run `bin/edikt verify spec " + orPlaceholder(reportID, "<SPEC-id>") +
			"` and Read the resulting report before checking the AC."
	}
}

func orPlaceholder(s, placeholder string) string {
	if s == "" {
		return placeholder
	}
	return s
}

func evidenceStatePath() string {
	return filepath.Join(".edikt", "state", ".evidence-reads")
}

// readEvidenceReads loads the recorded evidence reads. A missing file is {},
// which means default-deny on a claim; a MALFORMED file is also {}, reported
// on stderr rather than through the protocol, because a corrupt state file is
// an operator problem and not a message for the model.
func readEvidenceReads(inv *Invocation) map[string]any {
	b, err := os.ReadFile(evidenceStatePath())
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		if inv != nil && inv.Stderr != nil {
			fmt.Fprintln(inv.Stderr, "verify-gate: .edikt/state/.evidence-reads malformed; treating as empty")
		}
		return map[string]any{}
	}
	return m
}

// consumeEvidenceRead clears the read that unlocked this claim and rewrites
// the state file atomically, so a concurrent session never observes a
// half-written map.
//
// FAILS OPEN, LOUDLY. A state-file write failure must not brick the developer
// workflow, so the allow still stands — but it is reported on stderr rather
// than swallowed, because the consequence (the same read can unlock a second
// claim) is invisible otherwise.
func consumeEvidenceRead(inv *Invocation, reads map[string]any, id string) {
	reads[id] = nil
	dir := filepath.Dir(evidenceStatePath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		reportStateWriteFailure(inv, err)
		return
	}
	body, err := marshalEvidenceReads(reads)
	if err != nil {
		reportStateWriteFailure(inv, err)
		return
	}
	tmp, err := os.CreateTemp(dir, ".evidence-reads.")
	if err != nil {
		reportStateWriteFailure(inv, err)
		return
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		reportStateWriteFailure(inv, err)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		reportStateWriteFailure(inv, err)
		return
	}
	if err := os.Rename(tmp.Name(), evidenceStatePath()); err != nil {
		os.Remove(tmp.Name())
		reportStateWriteFailure(inv, err)
	}
}

func reportStateWriteFailure(inv *Invocation, err error) {
	if inv != nil && inv.Stderr != nil {
		fmt.Fprintf(inv.Stderr,
			"verify-gate: state-file write failed; gate temporarily disabled until disk frees (%v)\n", err)
	}
}

// marshalEvidenceReads renders the map the way the bash original's json.dump
// did, so a state file written by either implementation reads identically.
func marshalEvidenceReads(reads map[string]any) ([]byte, error) {
	return adapter.MarshalPythonCompact(reads)
}

// latestReportVerdict finds the newest verify report matching id and reports
// whether it PASSES.
//
// A recorded read is only evidence when the report it points at passes:
// missing, unparseable, or showing failures all return false. Fail-closed
// here is the whole value of the check — an open answer would let the gate be
// satisfied by a report nobody can read.
func latestReportVerdict(id string) (string, bool) {
	dir := filepath.Join(".edikt", "state", "verify")
	needle := id
	if m := reportNeedleRe.FindString(id); m != "" {
		needle = m
	}
	// BOUNDARY-ANCHORED, not a bare substring (audit 2026-08-07 #1): without
	// the anchors a passing report for gov-ADR-0010 satisfied the gate for
	// ADR-001.
	needleRe, err := regexp.Compile(`(^|[^0-9a-z])` + regexp.QuoteMeta(needle) + `($|[^0-9])`)
	if err != nil {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if needleRe.MatchString(strings.ToLower(e.Name())) {
			candidates = append(candidates, e.Name())
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Strings(candidates)
	latest := ""
	var latestMod time.Time
	for _, name := range candidates {
		full := filepath.Join(dir, name)
		st, err := os.Stat(full)
		if err != nil {
			continue
		}
		if latest == "" || st.ModTime().After(latestMod) {
			latest, latestMod = full, st.ModTime()
		}
	}
	if latest == "" {
		return "", false
	}
	b, err := readCappedN(latest, maxVerifyReportBytes)
	if err != nil {
		return latest, false
	}
	var rep struct {
		Summary struct {
			Failed  *float64 `json:"failed"`
			Timeout *float64 `json:"timeout"`
			Passed  *float64 `json:"passed"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(b, &rep); err != nil {
		return latest, false
	}
	s := rep.Summary
	// AN ALL-SKIPPED RUN IS NOT COMPLETION EVIDENCE (audit 2026-08-07 #2).
	// failed==0 alone is satisfied by a run that checked nothing, so at least
	// one criterion must have actually passed. Absent fields are absent, not
	// zero — INV-013: a report that does not say it passed did not.
	if s.Failed == nil || *s.Failed != 0 {
		return latest, false
	}
	if s.Timeout == nil || *s.Timeout != 0 {
		return latest, false
	}
	if s.Passed == nil || *s.Passed < 1 {
		return latest, false
	}
	return latest, true
}

func readCappedN(path string, max int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, max)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

// truthy mirrors the bash original's `evidence_reads.get(id)` test: a
// consumed read is stored as null and must not re-unlock a claim, and an
// empty string is not a timestamp.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	default:
		return true
	}
}

type auditRecord struct {
	TS      string `json:"ts"`
	Event   string `json:"event"`
	Posture string `json:"posture"`
}

type actorRecord struct {
	TS    string `json:"ts"`
	Event string `json:"event"`
	Actor string `json:"actor"`
}

type warnRecord struct {
	TS     string `json:"ts"`
	Event  string `json:"event"`
	Reason string `json:"reason"`
}

func auditGate(event, posture string) {
	appendJSONL("verify-gate.jsonl", auditRecord{TS: utcNow(), Event: event, Posture: posture})
}

func utcNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// appendJSONL writes one record to a state ledger. NEVER FATAL: these are
// telemetry, and telemetry must not decide whether a write proceeds. A
// serializer, never string concatenation — INV-003 applies to a log file as
// absolutely as to stdout.
func appendJSONL(name string, rec any) {
	dir := filepath.Join(".edikt", "state")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	body, err := adapter.MarshalPythonCompact(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(body, '\n'))
}
