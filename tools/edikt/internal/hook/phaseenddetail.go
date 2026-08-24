package hook

// phaseenddetail.go — phase-end-detector's detection, gates and artifacts.
//
// Split from phaseenddetector.go so the STEP PROTOCOL (which is new surface,
// and the only part of this migration that is not a translation) reads on its
// own. Everything here is a port of a specific fragment of the bash body, and
// each function names the fragment it replaces.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// ── config reading, exactly as grep/awk/tr read it ──────────────────────────

// phaseEndConfigHas is `grep -qE <pat>` over the config: does ANY line match.
func phaseEndConfigHas(config, pattern string) bool {
	re, err := regexp.Compile("(?m)" + pattern)
	if err != nil {
		return false
	}
	return re.MatchString(config)
}

// phaseEndConfigField is `grep <pat> | awk '{print $2}' | tr -d '"'`.
//
// EVERY matching line contributes, joined by newlines, because that is what
// the command substitution produces. A config with two `base:` lines yields a
// two-line value in bash and does the same here — degenerate, but reproduced
// rather than quietly collapsed to the first, since collapsing would make the
// Go build behave BETTER than the bash on an input the corpus never sees.
func phaseEndConfigField(config, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	var out []string
	for _, line := range strings.Split(config, "\n") {
		if !re.MatchString(line) {
			continue
		}
		fields := strings.Fields(line)
		v := ""
		if len(fields) > 1 {
			v = strings.ReplaceAll(fields[1], `"`, "")
		}
		out = append(out, v)
	}
	joined := strings.Join(out, "\n")
	if strings.TrimRight(joined, "\n") == "" && !strings.Contains(joined, "\n") {
		return ""
	}
	return joined
}

// phaseEndPlanDirs is the four candidate directories, in order.
func phaseEndPlanDirs(base, planDir string) []string {
	return []string{planDir, base + "/product/plans", "docs/plans", "docs/product/plans"}
}

// phaseEndLatestPlan is the plan ladder: the first candidate directory that
// exists AND holds a PLAN-*.md, resolved by `ls -t … | grep -v criteria.yaml |
// head -1`.
//
// Recency is mtime-descending. `ls -t`'s tie-break for equal timestamps is
// unspecified across implementations, so this pins it to name-ascending — a
// choice the bash does not make, stated here rather than left to look like
// parity.
func phaseEndLatestPlan(root string, dirs []string) string {
	for _, dir := range dirs {
		abs := filepath.Join(root, dir)
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			continue
		}
		entries, rerr := os.ReadDir(abs)
		if rerr != nil {
			continue
		}
		type cand struct {
			name string
			mod  time.Time
		}
		var cands []cand
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasPrefix(name, "PLAN-") || !strings.HasSuffix(name, ".md") {
				continue
			}
			if strings.Contains(name, "criteria.yaml") {
				continue
			}
			info, ierr := e.Info()
			if ierr != nil {
				continue
			}
			cands = append(cands, cand{name: name, mod: info.ModTime()})
		}
		if len(cands) == 0 {
			continue
		}
		sort.Slice(cands, func(i, j int) bool {
			if !cands[i].mod.Equal(cands[j].mod) {
				return cands[i].mod.After(cands[j].mod)
			}
			return cands[i].name < cands[j].name
		})
		return dir + "/" + cands[0].name
	}
	return ""
}

// phaseEndFindSidecar is the `<dir>/<stem>-criteria.yaml` scan, same ladder.
func phaseEndFindSidecar(root string, dirs []string, stem string) string {
	for _, dir := range dirs {
		abs := filepath.Join(root, dir)
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			continue
		}
		cand := dir + "/" + stem + "-criteria.yaml"
		if fileExists(filepath.Join(root, cand)) {
			return cand
		}
	}
	return ""
}

// phaseEndFirstMatch is `grep -E <pat> | head -1`.
func phaseEndFirstMatch(body string, re *regexp.Regexp) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if re.MatchString(line) {
			return line
		}
	}
	return ""
}

// phaseEndPhaseNumber is `sed 's/|/\n/g' | sed -n '2p' | tr -d ' ' | grep -oE '[0-9]+'`.
//
// The cell taken is the one between the FIRST and SECOND pipe — a row starting
// with `|` splits into an empty first field, so `sed -n 2p` lands on the phase
// cell. Every digit run in it is emitted on its own line and the command
// substitution joins them, so `3a4` yields "3\n4"; reproduced rather than
// collapsed for the same reason phaseEndConfigField keeps its newlines.
func phaseEndPhaseNumber(line string) string {
	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return ""
	}
	cell := strings.ReplaceAll(parts[1], " ", "")
	return strings.Join(phaseEndDigitsRe.FindAllString(cell, -1), "\n")
}

// phaseEndCompletionDetected applies the three completion patterns, each with
// the phase number interpolated exactly as the bash interpolates it.
//
// grep is line-oriented and `.` never crosses a newline, so a `.{0,40}` window
// cannot span two lines of a multi-line assistant turn. Go's default (no
// `(?s)`) matches that; the `(?i)` mirrors `grep -i`, which also makes pattern
// 3's `[A-Z ]` case-insensitive — reproduced, not corrected.
func phaseEndCompletionDetected(msg, phaseNum string) bool {
	patterns := []string{
		`(?i)phase[- ]?` + phaseNum + `[^0-9].{0,40}(complete|done|finished|implemented|shipped)`,
		`(?i)(completed|implemented|finished|shipped) phase[- ]?` + phaseNum + `[^0-9]`,
		`(?i)PHASE[- ]?` + phaseNum + `[- ]?[A-Z ]+DONE`,
	}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			// A phase number carrying a regex metacharacter cannot come from
			// the digit extraction above; an uncompilable pattern is treated
			// as no match rather than as a crash.
			continue
		}
		if re.MatchString(msg) {
			return true
		}
	}
	return false
}

// ── the duplicate-invocation guard ──────────────────────────────────────────

// phaseEndDedupSuppresses reports whether this invocation is a duplicate that
// must exit silently, writing and pruning the token where the bash does.
func phaseEndDedupSuppresses(inv *Invocation, root string) bool {
	if os.Getenv("EDIKT_DISABLE_HOOK_DEDUP") == "1" {
		return false
	}
	if !dirExists(filepath.Join(root, ".edikt")) {
		return false
	}
	dedupDir := filepath.Join(root, ".edikt", "state", ".hook-dedup")
	_ = os.MkdirAll(dedupDir, 0o755)

	// Keyed on the RAW payload bytes, which never cross the seam — the digest
	// does. A key derived from the decoded struct, the session id, or a fixed
	// prefix would suppress a DIFFERENT payload; distinct-payloads pins that.
	key := inv.Payload.PayloadDigest(inv.Name + ":")
	if len(key) > 40 {
		key = key[:40]
	}
	if key == "" || !dirExists(dedupDir) {
		return false
	}
	token := filepath.Join(dedupDir, key)
	if fi, err := os.Stat(token); err == nil && fi.Mode().IsRegular() {
		// A negative age (a token stamped in the future) is UNKNOWN, never
		// fresh: suppressing on it would silence a real emission (INV-013),
		// which is why the bash tests `-ge 0` as well as `-lt 15`.
		age := time.Since(fi.ModTime())
		if age >= 0 && age < phaseEndDedupTTL {
			return true
		}
	}
	_ = truncateOrCreate(token)
	phaseEndPruneDedup(dedupDir)
	return false
}

// phaseEndPruneDedup mirrors `find "$dir" -type f -mmin +5 -delete`.
func phaseEndPruneDedup(dir string) {
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
		if time.Since(info.ModTime()) > phaseEndDedupPrune {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// ── the ownership guard ─────────────────────────────────────────────────────

// phaseEndOwnedByUs is `[ -O path ]`: is the file owned by the effective uid.
//
// The second return is whether the question could be ANSWERED at all. A
// platform whose stat carries no uid reports "unknown", and the caller then
// proceeds — the same direction the bash takes when the file is absent, and
// the only direction that does not turn an unanswerable question into a
// governance stop (INV-013).
func phaseEndOwnedByUs(path string) (owned, known bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false, false
	}
	return int(st.Uid) == os.Geteuid(), true
}

// ── the events ledger ───────────────────────────────────────────────────────

type phaseEndDetectEvent struct {
	TS    string `json:"ts"`
	Event string `json:"event"`
	Plan  string `json:"plan"`
	Phase any    `json:"phase"`
}

type phaseEndEvalEvent struct {
	TS    string `json:"ts"`
	Event string `json:"event"`
	Plan  string `json:"plan"`
	Phase any    `json:"phase"`
	Exit  int    `json:"exit"`
}

// phaseEndLogEvent appends one record to $HOME/.edikt/events.jsonl.
//
// Best-effort, exactly as the bash's `|| true` makes it: a ledger that cannot
// be written must not break the turn it is observing.
func phaseEndLogEvent(rec any) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	dir := filepath.Join(home, ".edikt")
	_ = os.MkdirAll(dir, 0o755)
	line, merr := adapter.MarshalPythonCompact(rec)
	if merr != nil {
		return
	}
	f, oerr := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if oerr != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

// phaseEndNumberOrString is python's `int(x)` with a fallback to the raw
// string, so a non-numeric phase id lands in the ledger as text rather than
// making the record unwritable.
func phaseEndNumberOrString(s string) any {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return s
}

func phaseEndTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// ── the messages ────────────────────────────────────────────────────────────

// phaseEndMissingSidecarWarning renders the evaluation-history warning, whose
// middle clause depends on which of the three generation statuses was reached.
func phaseEndMissingSidecarWarning(phaseNum, planName, genStatus string) string {
	stem := strings.ReplaceAll(planName, ".md", "")
	extra := ""
	switch genStatus {
	case "failed":
		extra = "edikt tried to rebuild it automatically but couldn't."
	case "attempted":
		extra = "edikt tried to rebuild it automatically but the file wasn't created."
	}
	msg := "⚠️  Phase " + phaseNum + " — evaluation history not found for " + planName + ".\n"
	if extra != "" {
		msg += "    " + extra + "\n"
	}
	msg += "\n" +
		"    Your work is still being evaluated. But without the history file,\n" +
		"    edikt can't track repeated failures, block reasons, or when each\n" +
		"    check was last run — so the evaluator starts fresh every time.\n" +
		"\n" +
		"    To rebuild it:\n" +
		"      /edikt:sdlc:plan --sidecar-only " + stem
	b, err := adapter.SystemMessageReply(msg)
	if err != nil {
		return ""
	}
	return string(b)
}

// phaseEndReinstallHint is what the missing-template message tells the user to
// do, and it is the ONE place this migration deliberately does not reproduce
// the bash byte-for-byte.
//
// The bash printed `curl -fsSL …/diktahq/edikt/main/install.sh | bash` — an
// install command tracking a MOVING REF, which INV-008 forbids for anything a
// user copies. It survived only because the gate's scope is README, CLAUDE.md,
// .github/workflows, website, docs/guides, install.sh and bin/edikt, and
// templates/hooks/ is not in it. Carrying the same string into the binary
// would have put it inside bin/edikt, where the gate DOES look — and the right
// response to a rule finally reaching a violation is to fix the violation, not
// to obfuscate the literal until the grep stops seeing it.
//
// No fixture pins the old bytes (the corpus cannot reach this branch and no
// test greps the message), so the wording changes to an instruction that
// cannot go stale or point at an unpinned artifact.
func phaseEndReinstallHint() string {
	return "/edikt:upgrade — or reinstall edikt from a pinned release tag"
}

// phaseEndEvaluatorTemplate is the three-rung template ladder.
func phaseEndEvaluatorTemplate(root string) string {
	cands := []string{
		filepath.Join(root, ".edikt", "current", "templates", "agents", "evaluator-headless.md"),
		filepath.Join(root, "templates", "agents", "evaluator-headless.md"),
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cands = append(cands, filepath.Join(home, ".edikt", "templates", "agents", "evaluator-headless.md"))
	}
	for _, c := range cands {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

// phaseEndEvaluatorPrompt builds the evaluator's -p argument.
func phaseEndEvaluatorPrompt(phaseNum, plan, sidecar string) string {
	parts := []string{
		"Evaluate Phase " + phaseNum + " of " + filepath.Base(plan) + ".",
		"",
		"Plan file: " + plan,
	}
	if sidecar != "" {
		parts = append(parts,
			"Criteria sidecar: "+sidecar,
			"",
			"Read the criteria for phase "+phaseNum+" from the sidecar, run each `verify` command if present, and return per-criterion PASS/FAIL verdicts.")
	} else {
		parts = append(parts,
			"",
			"Read the acceptance criteria for phase "+phaseNum+" from the plan, and return per-criterion PASS/FAIL verdicts.")
	}
	parts = append(parts,
		"",
		"Also run `git diff --name-only HEAD~1 HEAD 2>/dev/null || git diff --name-only` to see what was changed.",
		"",
		"Return your verdict in the format specified by the evaluator system prompt.")
	return strings.Join(parts, "\n")
}

// phaseEndConfiguredModel is `grep -A10 '^evaluator:' | grep -E '^\s*model:' |
// awk '{print $2}' | tr -d '"' | head -1`.
func phaseEndConfiguredModel(root string) string {
	body, err := os.ReadFile(filepath.Join(root, ".edikt", "config.yaml")) //nolint:gosec // project-local config
	if err != nil {
		return ""
	}
	lines := strings.Split(string(body), "\n")
	within := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "evaluator:") {
			within = i
		}
		if within < 0 || i-within > 10 {
			continue
		}
		if !phaseEndModelRe.MatchString(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return ""
		}
		return strings.ReplaceAll(fields[1], `"`, "")
	}
	return ""
}

// phaseEndEvalTimeout is `${EDIKT_EVAL_TIMEOUT:-180}` with the INV-006 numeric
// guard: a non-numeric value is 180, never passed through to argv.
func phaseEndEvalTimeout() string {
	v := os.Getenv("EDIKT_EVAL_TIMEOUT")
	if v == "" {
		return "180"
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return "180"
		}
	}
	return v
}

// phaseEndPostFlightTimeout is `${POST_FLIGHT_TIMEOUT:-120}`. Deliberately NOT
// numeric-guarded: the bash does not guard it either, and the value reaches
// the reason string as well as the timeout, so a guard here would change the
// rendered message for a config the corpus does not cover.
func phaseEndPostFlightTimeout() string {
	if v := os.Getenv("POST_FLIGHT_TIMEOUT"); v != "" {
		return v
	}
	return "120"
}

// ── the dispatch output ─────────────────────────────────────────────────────

// phaseEndReadDispatch reads the file the caller redirected a dispatch into.
// An unreadable or absent file is the empty string, which is what a command
// substitution over a failed redirect produces.
func phaseEndReadDispatch(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path) //nolint:gosec // caller-supplied step-protocol path
	if err != nil {
		return ""
	}
	return string(b)
}

// phaseEndHeadLines is `| head -N` followed by a command substitution's
// trailing-newline strip.
func phaseEndHeadLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// phaseEndToolName is the CLI the caller dispatches, as data. Empty means the
// caller did not name one, and the messages then read "the evaluator CLI"
// rather than inventing a vendor.
func phaseEndToolName(inv *Invocation) string {
	if inv.Step != nil && inv.Step.DispatchTool != "" {
		return inv.Step.DispatchTool
	}
	return "the evaluator"
}

func phaseEndToolAvailable(inv *Invocation) bool {
	return inv.Step != nil && inv.Step.DispatchAvailable
}

// ── the L1 verdict ──────────────────────────────────────────────────────────

// phaseEndFindFirstJSONObject is `_find_first_json_object`: scan for the first
// balanced `{`…`}` run that PARSES, tolerating prose either side.
//
// Brace counting is deliberately NOT string-aware, because the python is not
// either — a `{` inside a JSON string value shifts the balance point in both
// implementations identically. On a parse failure the inner walk breaks and
// the scan continues from the NEXT `{`, which is the python's own `break`.
func phaseEndFindFirstJSONObject(text string) (jsonObject, bool) {
	for start := 0; start < len(text); start++ {
		if text[start] != '{' {
			continue
		}
		depth := 0
		for i := start; i < len(text); i++ {
			switch text[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					var obj jsonObject
					if err := json.Unmarshal([]byte(text[start:i+1]), &obj); err == nil {
						return obj, true
					}
					i = len(text) // break the inner walk, as python's `break` does
				}
			}
		}
	}
	return jsonObject{}, false
}

// phaseEndL1Output computes the verdict, runs the evidence gate, writes both
// durable artifacts, and returns the L1 reply bytes.
//
// Every WRITE happens before the reply is composed, which is the ordering the
// bash comment calls out: an early exit that skipped persistence used to leave
// a BLOCKED outcome with zero durable trace.
func phaseEndL1Output(r *phaseEndRun, raw string) string {
	phase := r.state.PhaseNum
	if phase == "" {
		phase = "?"
	}
	evalTS := r.state.EvalTS
	if evalTS == "" {
		evalTS = phaseEndTimestamp()
	}

	verdictObj, found := phaseEndFindFirstJSONObject(raw)
	missingMsg := ""
	if _, isStr := verdictObj.str("verdict"); !found || !isStr {
		lowered := strings.ToLower(raw)
		authMarker := ""
		for _, m := range phaseEndAuthMarkers {
			if strings.Contains(lowered, m) {
				authMarker = m
				break
			}
		}
		if authMarker != "" {
			// Fail-closed on a missing/nonconforming verdict (ref: INV-011).
			// Never advise re-authenticating this session: the child resolves
			// its OWN profile from the hook environment, so the session being
			// logged in does not fix it.
			missingMsg = "⚠️  Phase " + phase + " evaluator subprocess FAILED to authenticate. " +
				"This is an evaluator FAILURE — verdict treated as BLOCKED, " +
				"not a prompt to authenticate this session. The evaluator child " +
				"resolves its own " + adapter.HostConfigDirNoun() + " from the hook environment " +
				"(" + adapter.HostConfigDirNote() + "). If this project runs under a non-default profile, " +
				"ensure " + adapter.HostConfigDirEnv + " is exported where hooks run; otherwise " +
				"the child uses the default profile, which is unauthenticated here."
		} else {
			missingMsg = "⚠️  Phase " + phase + " evaluator did not emit a structured JSON verdict. " +
				"Verdict treated as BLOCKED until the evaluator emits a " +
				"schema-conforming object. Output head:\n" + runeHead(raw, 800)
		}
		blockedReason := "no schema-conforming evaluator verdict"
		if authMarker != "" {
			blockedReason += " (auth failure: " + authMarker + ")"
		}
		verdictObj = jsonObject{}
		_ = verdictObj.setValue("verdict", "BLOCKED")
		_ = verdictObj.setValue("criteria", []any{})
		meta := jsonObject{}
		_ = meta.setValue("blocked_reason", blockedReason)
		_ = verdictObj.setValue("meta", meta)
	}

	verdict, _ := verdictObj.str("verdict")
	if verdict == "" {
		verdict = "BLOCKED"
	}
	criteria := verdictObj.objectList("criteria")
	meta := verdictObj.object("meta")

	// ── the INV-011 evidence gate's spec IS the criteria sidecar ────────
	testRunIDs, gateErr := phaseEndTestRunIDs(r.state.Root, r.state.Sidecar)

	grandfathered := meta.truthy("grandfathered")

	var gateViolations []string
	sidecarGateBlocked := false
	if !grandfathered && verdict == "PASS" {
		if gateErr != "" {
			sidecarGateBlocked = true
		}
		for _, c := range criteria {
			cid, _ := c.str("id")
			if !testRunIDs[cid] {
				continue
			}
			et, ok := c.str("evidence_type")
			if ok && et == "test_run" {
				continue
			}
			shown := "missing"
			if raw, present := c.raw("evidence_type"); present {
				shown = pythonRepr(strings.Trim(string(raw), `"`))
				if !ok {
					shown = string(raw)
				}
			}
			gateViolations = append(gateViolations,
				cid+": criterion names a shell command but evidence_type is "+shown)
		}
	}
	if len(gateViolations) > 0 || sidecarGateBlocked {
		verdict = "BLOCKED"
	}

	outputMsg := phaseEndOutputMessage(phase, verdict, missingMsg, gateErr,
		sidecarGateBlocked, gateViolations, grandfathered, criteria)

	// ── persist the verdict JSON ────────────────────────────────────────
	if r.state.PlanFile != "" && r.state.PlanStem != "" {
		final := verdictObj.clone()
		_ = final.setValue("verdict", verdict)
		phaseEndPersistVerdict(r.state.Root, r.state.PlanFile, r.state.PlanStem, phase, final, evalTS)
	}

	// ── update the evaluation state ─────────────────────────────────────
	// The criteria sidecar is NEVER written here — it is spec-only, and
	// `bin/edikt verify` strict-parses it.
	if r.state.PlanStem != "" && len(criteria) > 0 {
		day := evalTS
		if len(day) > 10 {
			day = day[:10]
		}
		phaseEndUpdateEvalState(r.state.Root, r.state.PlanStem, criteria, phase, day)
	}

	b, err := adapter.SystemMessageReply(outputMsg)
	if err != nil {
		return ""
	}
	// The bash writes this through `print()` into a tmpfile and reads it back
	// with `$(cat …)`, which strips the newline again — so the L1 reply
	// carries none of its own.
	return string(b)
}

// phaseEndTestRunIDs is the lightweight YAML scan that identifies which
// criteria name a shell command, and the gate ERROR when the spec cannot be
// read at all.
//
// An absent or unreadable sidecar is a gate FAILURE, not a free pass: it used
// to leave the id set empty so the gate silently never fired (INV-011).
func phaseEndTestRunIDs(root, sidecar string) (map[string]bool, string) {
	ids := map[string]bool{}
	if sidecar == "" {
		return ids, "no criteria sidecar resolved for this plan"
	}
	abs := filepath.Join(root, sidecar)
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return ids, "criteria sidecar missing"
	}
	body, rerr := os.ReadFile(abs) //nolint:gosec // config-declared sidecar path
	if rerr != nil {
		return ids, "criteria sidecar unreadable"
	}
	for _, block := range phaseEndBlockSplitRe.Split(string(body), -1) {
		idm := phaseEndSidecarIDRe.FindStringSubmatch(block)
		vm := phaseEndSidecarVerRe.FindStringSubmatch(block)
		if idm == nil || vm == nil {
			continue
		}
		if phaseEndTestCommandRe.MatchString(vm[1]) {
			ids[idm[1]] = true
		}
	}
	return ids, ""
}

// phaseEndOutputMessage is the user-visible message, on every path. Composed
// separately from the writes above so no branch can exit before them.
func phaseEndOutputMessage(phase, verdict, missingMsg, gateErr string,
	sidecarGateBlocked bool, gateViolations []string, grandfathered bool,
	criteria []jsonObject) string {
	switch {
	case missingMsg != "":
		return missingMsg
	case sidecarGateBlocked:
		extra := ""
		if len(gateViolations) > 0 {
			extra = "\nAdditionally:\n  - " + strings.Join(gateViolations, "\n  - ")
		}
		// (ref: INV-011 — the gate cannot certify what it cannot read)
		return "⚠️  Phase " + phase + " PASS was forced to BLOCKED — the evidence gate " +
			"could not run: " + gateErr + ". Regenerate the criteria sidecar with " +
			"/edikt:sdlc:plan --sidecar-only <plan-slug> before marking the " +
			"phase done." + extra
	case len(gateViolations) > 0:
		return "⚠️  Phase " + phase + " PASS was forced to BLOCKED by the " +
			"evidence gate. test_run evidence is required for:\n  - " +
			strings.Join(gateViolations, "\n  - ") + "\n\n" +
			"Re-run the evaluator with Bash available, or explicitly mark the " +
			"phase blocked in the plan."
	case verdict == "PASS":
		msg := "✓ Phase " + phase + " evaluation: PASS"
		if grandfathered {
			msg += " (grandfathered from pre-v0.5.0 verdict)"
		}
		return msg
	}
	lines := []string{"⚠️  Phase " + phase + " evaluation: " + verdict}
	for _, c := range criteria {
		status := phaseEndField(c, "status", "?")
		if status == "met" {
			continue
		}
		lines = append(lines, "  - "+phaseEndField(c, "id", "?")+": "+status+
			" — "+phaseEndField(c, "evidence", "(no evidence)"))
		if notes := phaseEndField(c, "notes", ""); notes != "" {
			lines = append(lines, "      note: "+notes)
		}
	}
	return strings.Join(lines, "\n") + "\n\nReview the findings and fix before marking the phase done."
}

// phaseEndField is python's `c.get(k, default)` rendered into a message —
// including the str() of a non-string value, which is what an f-string does.
func phaseEndField(o jsonObject, key, def string) string {
	raw, ok := o.raw(key)
	if !ok {
		return def
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// phaseEndPersistVerdict writes verdicts/<stem>/phase-<N>.json beside the plan.
func phaseEndPersistVerdict(root, planFile, stem, phase string, verdict jsonObject, ts string) {
	abs := planFile
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, planFile)
	}
	dir := filepath.Join(filepath.Dir(abs), "verdicts", stem)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return // best-effort; non-fatal
	}
	meta := verdict.object("meta")
	_ = meta.setValue("evaluated_at", ts)
	_ = verdict.setValue("meta", meta)
	body, err := adapter.MarshalPythonIndent2(verdict)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "phase-"+phase+".json"), append(body, '\n'), 0o644) //nolint:gosec // artifact read by humans and by verify
}

// ── the plan-eval state file ────────────────────────────────────────────────

type phaseEndEvalState struct {
	SchemaVersion int                 `json:"schema_version"`
	Plan          string              `json:"plan"`
	LastEvaluated *string             `json:"last_evaluated"`
	Phases        []phaseEndEvalPhase `json:"phases"`
}

type phaseEndEvalPhase struct {
	ID       string             `json:"id"`
	Status   string             `json:"status"`
	Criteria []phaseEndEvalCrit `json:"criteria"`
}

type phaseEndEvalCrit struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	FailCount     int     `json:"fail_count"`
	FailReason    *string `json:"fail_reason"`
	BlockReason   *string `json:"block_reason"`
	LastEvaluated *string `json:"last_evaluated"`
}

// phaseEndUpdateEvalState is `_update_eval_state`: read/modify/write
// .edikt/state/plan-eval/<stem>-eval.json, touching only the target phase.
func phaseEndUpdateEvalState(root, stem string, criteria []jsonObject, phaseID, day string) {
	dir := filepath.Join(root, ".edikt", "state", "plan-eval")
	path := filepath.Join(dir, stem+"-eval.json")

	state := phaseEndEvalState{SchemaVersion: 1, Plan: stem}
	if body, err := os.ReadFile(path); err == nil { //nolint:gosec // project-local state
		var loaded phaseEndEvalState
		if json.Unmarshal(body, &loaded) == nil {
			state = loaded
		}
	}
	d := day
	state.LastEvaluated = &d

	idx := -1
	for i := range state.Phases {
		if state.Phases[i].ID == phaseID {
			idx = i
			break
		}
	}
	if idx < 0 {
		state.Phases = append(state.Phases, phaseEndEvalPhase{ID: phaseID, Status: "pending"})
		idx = len(state.Phases) - 1
	}
	entry := &state.Phases[idx]

	var statuses []string
	for _, c := range criteria {
		cid := phaseEndField(c, "id", "")
		if cid == "" {
			continue
		}
		ev := phaseEndField(c, "status", "")
		scStatus := "fail"
		switch ev {
		case "met":
			scStatus = "pass"
		case "blocked":
			scStatus = "blocked"
		}

		ci := -1
		for i := range entry.Criteria {
			if entry.Criteria[i].ID == cid {
				ci = i
				break
			}
		}
		if ci < 0 {
			entry.Criteria = append(entry.Criteria, phaseEndEvalCrit{ID: cid, Status: "pending"})
			ci = len(entry.Criteria) - 1
		}
		crit := &entry.Criteria[ci]
		crit.Status = scStatus
		when := day
		crit.LastEvaluated = &when
		switch scStatus {
		case "fail":
			reason := phaseEndReasonText(c)
			crit.FailReason = phaseEndOptional(reason)
			crit.FailCount++
		case "pass":
			crit.FailReason = nil
			crit.FailCount = 0
		default:
			// blocked: fail_count unchanged, the block reason recorded.
			crit.BlockReason = phaseEndOptional(phaseEndReasonText(c))
		}
		statuses = append(statuses, scStatus)
	}

	if len(statuses) > 0 {
		entry.Status = "pass"
		for _, s := range statuses {
			if s == "blocked" {
				entry.Status = "blocked"
			}
		}
		for _, s := range statuses {
			if s == "fail" {
				entry.Status = "fail"
			}
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return // best-effort
	}
	body, err := adapter.MarshalPythonIndent2(state)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(body, '\n'), 0o644) //nolint:gosec // project-local state
}

// phaseEndReasonText is `str(c.get('evidence') or c.get('notes') or
// ”).replace('\n',' ')[:200]`.
func phaseEndReasonText(c jsonObject) string {
	text := phaseEndField(c, "evidence", "")
	if text == "" || text == "null" {
		text = phaseEndField(c, "notes", "")
	}
	if text == "null" {
		text = ""
	}
	text = strings.ReplaceAll(text, "\n", " ")
	return runeHead(text, 200)
}

// phaseEndOptional is python's `x or None` for the two reason fields.
func phaseEndOptional(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

// ── the post-flight kill switches ───────────────────────────────────────────

// phaseEndL1Verdict reads the L1 reply's own systemMessage the way the bash
// does — by its marker text, not by a field.
//
// Fragile on purpose, and pinned: the marker IS the contract between the
// verdict renderer and the post-flight gate, and
// test/unit/hooks/test_phase_end_detector_l1.sh catches a byte-shift in it.
func phaseEndL1Verdict(l1 string) string {
	msg, ok := adapter.DecodeSystemMessage([]byte(l1))
	if !ok {
		return "UNKNOWN"
	}
	switch {
	case strings.HasPrefix(msg, "✓ Phase") && strings.Contains(msg, "PASS"):
		return "PASS"
	case strings.Contains(msg, "BLOCKED"):
		return "BLOCKED"
	}
	return "FAIL"
}

// phaseEndScopeRecord is one line of the scope journal.
type phaseEndScopeRecord struct {
	TS         string `json:"ts"`
	Event      string `json:"event"`
	Resolution string `json:"resolution"`
	Enabled    bool   `json:"enabled"`
}

// phaseEndPostFlightScopeEnabled asks the tier-2 scope query and JOURNALS how
// the answer was obtained.
//
// THREE STATES, AND A FAILURE IS NOT AN ANSWER. This used to end
// `--json 2>/dev/null || echo '{"enabled":true}'`, so a binary that exited
// non-zero produced bytes identical to a genuine `{"enabled": true}`. A third
// path was quieter still: with no binary resolved the query never happened and
// post-flight stayed on having never been asked. All three remain PERMISSIVE —
// dispatching on an unanswered question is the safe direction — but they are
// now distinguishable after the fact, which is the whole of INV-013 here.
func phaseEndPostFlightScopeEnabled(root string) bool {
	bin := phaseEndResolveScopeBinary(root)
	if bin == "" {
		phaseEndJournalScope(root, "never-queried", true)
		return true
	}
	cmd := exec.Command(bin, "gov", "post-flight-scope", "--json") //nolint:gosec // resolved from a fixed ladder
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		phaseEndJournalScope(root, "resolution-failed", true)
		return true
	}
	var scope struct {
		Enabled bool `json:"enabled"`
	}
	if json.Unmarshal(out, &scope) != nil || !scope.Enabled {
		phaseEndJournalScope(root, "resolved", false)
		return false
	}
	phaseEndJournalScope(root, "resolved", true)
	return true
}

// phaseEndResolveScopeBinary is the four-rung ladder, in the ONLY order that
// is safe: the two project-supplied candidates, then the GLOBAL INSTALL ROOT,
// then bare PATH.
//
// The global root outranks PATH deliberately. A bare `edikt` on PATH is
// whatever the shell happens to find — on a developer machine often a stale
// global release, and in a test sandbox a deliberately planted fake. Every
// other ladder in the tier already orders it this way; this loop was the lone
// exception and resolved the fake in a live reproduction (AC-17.3).
func phaseEndResolveScopeBinary(root string) string {
	globalRoot := os.Getenv("EDIKT_ROOT")
	if globalRoot == "" {
		globalRoot = os.Getenv("EDIKT_HOME")
	}
	if globalRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			globalRoot = filepath.Join(home, ".edikt")
		}
	}
	cands := []string{
		filepath.Join(root, ".edikt", "bin", "edikt"),
		filepath.Join(root, "bin", "edikt"),
	}
	if globalRoot != "" {
		cands = append(cands, filepath.Join(globalRoot, "bin", "edikt"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Mode().Perm()&0o111 != 0 {
			return c
		}
	}
	if p, err := exec.LookPath("edikt"); err == nil {
		return p
	}
	return ""
}

func phaseEndJournalScope(root, resolution string, enabled bool) {
	dir := filepath.Join(root, ".edikt", "state", "post-flight")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	line, err := adapter.MarshalPythonCompact(phaseEndScopeRecord{
		TS:         phaseEndTimestamp(),
		Event:      "post-flight-scope",
		Resolution: resolution,
		Enabled:    enabled,
	})
	if err != nil {
		return
	}
	f, oerr := os.OpenFile(filepath.Join(dir, ".scope.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if oerr != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

// ── post-flight telemetry and the combined reply ────────────────────────────

type phaseEndMetricRecord struct {
	TS           string `json:"ts"`
	Plan         string `json:"plan"`
	Phase        int    `json:"phase"`
	DispatchMode string `json:"dispatch_mode"`
	L1Status     string `json:"l1_status"`
	Reason       string `json:"reason"`
	ElapsedMS    int64  `json:"elapsed_ms"`
}

// phaseEndAppendMetrics appends one line to .edikt/state/post-flight/.metrics.jsonl.
//
// A non-integer phase writes NOTHING, which is what the bash's `int(argv[3])`
// raising inside a `|| true` already did. Reproduced rather than widened: a
// record whose phase field silently changed type is worse than an absent one.
func phaseEndAppendMetrics(st phaseEndState, reason string, elapsedMS int64) {
	phase, err := strconv.Atoi(st.PhaseNum)
	if err != nil {
		return
	}
	if reason == "" {
		reason = "ok"
	}
	dir := filepath.Join(st.Root, ".edikt", "state", "post-flight")
	if mkerr := os.MkdirAll(dir, 0o755); mkerr != nil {
		return
	}
	line, merr := adapter.MarshalPythonCompact(phaseEndMetricRecord{
		TS:           phaseEndTimestamp(),
		Plan:         st.PlanStem,
		Phase:        phase,
		DispatchMode: "auto-hook",
		L1Status:     "passed",
		Reason:       reason,
		ElapsedMS:    elapsedMS,
	})
	if merr != nil {
		return
	}
	f, oerr := os.OpenFile(filepath.Join(dir, ".metrics.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if oerr != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

// phaseEndCombined renders the final systemMessage.
//
// The post-flight result is summarized through STRUCTURED FIELDS ONLY.
// Agent-derived free text is never interpolated (INV-004): the report itself
// lives on disk and the user opens it.
func phaseEndCombined(l1, reason, out string) string {
	l1Msg := ""
	if l1 != "" {
		l1Msg, _ = adapter.DecodeSystemMessage([]byte(l1))
	}

	combined := ""
	switch {
	case reason != "":
		// Degraded path — the L1 signal is preserved and the reason appended.
		combined = l1Msg + "\n\n⚠ post-flight: " + reason
	default:
		var pf jsonObject
		if err := json.Unmarshal([]byte(out), &pf); err != nil {
			combined = l1Msg + "\n\n📋 post-flight: completed"
			break
		}
		report := phaseEndSafeReportPath(phaseEndFirstNonEmpty(pf, "report_md", "report_json"))
		var degraded []string
		for _, pair := range [][2]string{
			{"l2_status", "L2"}, {"l3_status", "L3"}, {"synthesis_status", "synthesis"},
		} {
			tok := phaseEndStatusToken(phaseEndField(pf, pair[0], ""))
			if tok != "" && !phaseEndCleanStatuses[strings.ToLower(tok)] {
				degraded = append(degraded, pair[1]+" "+tok)
			}
		}
		switch {
		case len(degraded) > 0:
			tail := ""
			if report != "" {
				tail = " — see " + report
			}
			combined = l1Msg + "\n\n⚠ post-flight: " + strings.Join(degraded, ", ") + tail
		case report != "":
			combined = l1Msg + "\n\n📋 post-flight: " + report
		default:
			combined = l1Msg + "\n\n📋 post-flight: completed"
		}
	}
	b, err := adapter.SystemMessageReply(combined)
	if err != nil {
		return ""
	}
	return string(b) + "\n"
}

func phaseEndFirstNonEmpty(o jsonObject, keys ...string) string {
	for _, k := range keys {
		if v, ok := o.str(k); ok && v != "" {
			return v
		}
	}
	return ""
}

// phaseEndSafeReportPath validates report_md/report_json before it reaches the
// systemMessage. These are PATHS by the post-flight contract, but the value
// arrives from a forked dispatch whose pipeline reads repo-controlled content
// — a prompt-injected run could return arbitrary text. Surface it ONLY when it
// is a single-line relative path.
func phaseEndSafeReportPath(v string) string {
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "/") {
		return ""
	}
	for _, seg := range strings.Split(v, "/") {
		if seg == ".." {
			return ""
		}
	}
	// python's `$` also matches before ONE trailing newline; Go's does not.
	if !phaseEndReportPathRe.MatchString(strings.TrimSuffix(v, "\n")) {
		return ""
	}
	return v
}

// phaseEndStatusToken validates an agent-derived status field: a single short
// alphanumeric token, or nothing.
func phaseEndStatusToken(v string) string {
	if phaseEndStatusTokenRe.MatchString(strings.TrimSuffix(v, "\n")) {
		return v
	}
	return ""
}
