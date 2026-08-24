package hook

// stophook.go — the Go replacement for templates/hooks/stop-hook.sh
// (PLAN-v080-reliability-core Phase 16).
//
// The largest hook in the migration and the first `class: multi-call` row to
// move: 664 lines of bash around 14 python3 invocations, five signal
// detectors, a duplicate-invocation guard, a four-rung binary discovery
// ladder and a ledger-aware renderer. The charter is the same as every other
// phase — reproduce the bash body's BYTES, not improve on them — so every
// oddity below is ported rather than fixed, and the ones that look like bugs
// carry a comment saying so.
//
// THREE ORDERING FACTS THAT LOOK ARBITRARY AND ARE NOT
//
//  1. The project root is `pwd -P`, never the working directory the host
//     reported. stop-hook.sh captures it at script entry precisely BECAUSE
//     the host's cwd can drift mid-session, and every path this hook touches
//     is relative to that capture. Reading it off the payload instead would
//     move the ledger, the dedup tokens and the staleness log somewhere else
//     the moment the two disagree — which is why the corpus carries a
//     foreign-cwd scenario whose payload names a directory that does not
//     exist.
//  2. The python3 preflight runs AFTER stdin is read and after the two config
//     guards, not first. That ordering is observable: the signal-detection
//     opt-out wins over the missing-interpreter banner, and the no-python3
//     scenario needs a PATH that still resolves `cat`.
//  3. The duplicate-invocation guard sits between the preflight and the
//     re-entrancy check, so a suppressed duplicate leaves a token file behind
//     but never reaches the ledger. Two scenarios (repeat-fire,
//     dedup-disabled) pin the difference.
//
// WHAT THE COMMITTED CORPUS CANNOT REACH is listed in
// test/fixtures/hook-corpus/stop-hook/matrix.yaml's header, together with the
// test that fails when each of those branches is deleted. The four are: the
// missing-config guard, the upgrade-in-progress marker, the completion-claim
// detector (needs a plan file), and sidecar staleness in either outcome
// (needs a sidecar). All four are implemented in full here and all four are
// covered by stophook_test.go with positive controls.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("stop-hook", stopHook)
}

// stopHookNoPython3Message is the literal text stop-hook.sh's own preflight
// printf carries. The wire shape around it — `{"continue": true,
// "systemMessage": "…"}`, hand-built by printf because python3 is by
// definition unavailable on this branch — is rendered by
// adapter.NoPython3ContinueBanner and never spelled out here (SP-004/SAC-003).
const stopHookNoPython3Message = "⚠ edikt: python3 is missing — signal detection (ADR-candidate, doc-gap, completion-claim) was skipped for this turn."

// stopHookDedupTTL and stopHookDedupPrune mirror the two literals in
// stop-hook.sh's duplicate-invocation guard: `-lt 15` (seconds) and
// `find … -mmin +5`. `-mmin +5` is true for a file whose age rounded UP to
// the next whole minute exceeds five, i.e. strictly more than 300 seconds.
const (
	stopHookDedupTTL   = 15 * time.Second
	stopHookDedupPrune = 5 * time.Minute
)

// The five signal regexes, in the order stop-hook.sh applies them.
//
// Longest() is set on every EXTRACTION regex (the ones bash runs through
// `grep -oE`). POSIX ERE is leftmost-LONGEST and Go's default is
// leftmost-first; on `chose [a-z0-9_-]+ over [a-z0-9_-]+( for …)?` the two can
// disagree about whether the optional tail is taken, and the recorded subject
// bytes are what that decides. The boolean `grep -qE` regexes do not need it:
// "does anything match" is the same question under either rule.
var (
	// grep -qiE, stop-hook.sh's ADR trigger.
	stopHookADRTriggerRe = regexp.MustCompile(`(?i)chose .+ over |trade.?off|architectural (decision|constraint|choice)|going forward .*(all|every|must)|hard (constraint|rule|requirement)|ADR|decision record`)
	// grep -ioE, the subject extraction.
	stopHookADRSubjectRe = mustLongest(`(?i)chose [a-z0-9_-]+ over [a-z0-9_-]+( for [a-z0-9 _-]{0,30})?|trade.?off[a-z0-9 :_-]{0,40}`)
	// grep -oiE, new HTTP routes.
	stopHookRouteRe = mustLongest(`(?i)(POST|GET|PUT|DELETE|PATCH) /[a-zA-Z0-9/_:.-]+`)
	// grep -oE (case SENSITIVE — the alternation spells both cases itself).
	stopHookEnvVarRe = mustLongest(`(added|new|required|Added|New|Required).{0,30}[A-Z][A-Z0-9_]{3,}[A-Z0-9]`)
	// grep -qiE / grep -ioE, the security vocabulary. One pattern, used twice.
	stopHookSecurityRe = mustLongest(`(?i)(JWT|OAuth|PKCE|auth[a-z]*|payment|PII|encrypt|decrypt|secret|signing key|private key|bearer token|bcrypt|password hash)`)
	// grep -qiE, the completion-claim trigger. (?m) because grep anchors ^ and
	// $ per LINE, and a multi-line assistant turn is the normal case.
	stopHookCompletionRe = regexp.MustCompile(`(?im)(^|[^a-z])(✓|✅|done!|all (tests pass|green)|build clean|perfect|looks good|should (work|be fine)|phase [0-9]+[a-z]? complete|complete!)([^a-z]|$)`)
	// The ADR grep -v filter over matched env-var candidates. BRE alternation
	// (`\|`) in bash; a plain substring set here.
	stopHookEnvVarExcludes = []string{"ADR", "ARCH", "HTTP", "API", "JSON", "HTML", "CSS"}
	// The in-progress progress-table row, and the plan-id allowlist, both
	// copied from the embedded python.
	stopHookPlanRowRe = regexp.MustCompile(`(?i)^\|\s*([0-9]+[a-zA-Z]?)\s*\|\s*in[\s_-]?progress\b`)
	stopHookPlanIDRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)
)

func mustLongest(pat string) *regexp.Regexp {
	re := regexp.MustCompile(pat)
	re.Longest()
	return re
}

// stopHookStopWords is the `grep -vE '^(chose|over|…)$'` filter the G0 gate
// applies to the subject's own words before searching for them.
var stopHookStopWords = map[string]bool{
	"chose": true, "over": true, "for": true, "the": true, "a": true, "an": true,
	"to": true, "is": true, "was": true, "of": true, "and": true,
	"trade": true, "tradeoff": true, "trade-off": true,
}

// stopHookIcons is the renderer's ICONS dict. A kind absent from it renders
// "•" — which is what sidecar-stale-unchecked does, deliberately: it is a
// null result, not a finding, and it does not borrow the ⚠ its sibling uses.
var stopHookIcons = map[string]string{
	"adr-candidate": "💡", "doc-gap": "📄", "security-audit": "🔒",
	"sidecar-stale": "⚠", "completion-claim": "⚠",
}

// stopHookNoCollapseKinds is F-082a's rule: a kind that reports a NULL RESULT
// (a check that could not run) never collapses to the terse known-held line,
// even on a second firing, because the correct response to it is mechanical
// (install the binary) rather than a judgment call to be justified with an ack.
var stopHookNoCollapseKinds = map[string]bool{"sidecar-stale-unchecked": true}

// typedSignal is one entry of stop-hook.sh's four parallel TYPED_* arrays.
type typedSignal struct{ kind, subject, evidence, ask string }

func stopHook(inv *Invocation) Result {
	// `EDIKT_PROJECT_ROOT="$(pwd -P)"` — the physical working directory at
	// script entry, NOT inv.Root() (which prefers the host-reported cwd).
	// See this file's header, fact 1.
	root, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	if resolved, rerr := filepath.EvalSymlinks(root); rerr == nil {
		root = resolved
	}
	configPath := filepath.Join(root, ".edikt", "config.yaml")

	// ── guard 1: only run in edikt projects ─────────────────────────────
	if !fileExists(configPath) {
		return Result{}
	}
	configBytes, _ := os.ReadFile(configPath) //nolint:gosec // project-local config

	// ── guard 2: the documented opt-out ─────────────────────────────────
	// `grep -q 'signal-detection: false'` is a plain substring search over
	// the whole file, not a YAML lookup: a commented-out line matches too.
	// Ported as-is.
	if strings.Contains(string(configBytes), "signal-detection: false") {
		return Result{}
	}

	// ── guard 3: /edikt:upgrade is mid-flight ───────────────────────────
	// printf, not echo: this branch emits the literal with NO trailing
	// newline, unlike the identical-looking reply at the end of the hook.
	if fileExists(filepath.Join(root, ".edikt", "state", "upgrade-in-progress")) {
		return Result{Response: adapter.Response{
			Text:              adapter.ContinueTrueReply,
			NoTrailingNewline: true,
		}}
	}

	// ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────
	// The Go handler needs no interpreter, but the branch is BEHAVIOUR: a
	// project whose python3 is gone gets a banner rather than silence, and
	// the corpus pins its bytes. Removing it here would silently change what
	// such a project sees.
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: stop-hook — python3 not found; signal detection skipped for this turn")
		return Result{Response: adapter.Response{Text: adapter.NoPython3ContinueBanner(stopHookNoPython3Message)}}
	}

	// ── duplicate-invocation guard (bok-services field bug #4) ──────────
	if stopHookDedupSuppresses(inv, root) {
		return Result{}
	}

	// ── re-entrancy guard ───────────────────────────────────────────────
	if inv.Payload.StopActive {
		return Result{}
	}

	lastMsg := strings.TrimSpace(inv.Payload.LastAssistantMessage)
	if lastMsg == "" {
		return Result{}
	}

	var typed []typedSignal
	typed = append(typed, stopHookADRCandidate(root, configBytes, lastMsg)...)
	typed = append(typed, stopHookDocGap(lastMsg)...)
	typed = append(typed, stopHookSecurity(lastMsg)...)
	typed = append(typed, stopHookSidecarDrift(root)...)
	typed = append(typed, stopHookCompletionClaim(root, configBytes, lastMsg)...)

	// stop-hook.sh's legacy free-text SIGNALS array has had no producer since
	// ADR-050 and is permanently empty; the `${#SIGNALS[@]} -eq 0` half of
	// this test is therefore always true. Kept in the comment rather than in
	// the code, because an always-empty slice in Go would read as a variable
	// somebody forgot to fill.
	if len(typed) == 0 {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	stopHookAppendSessionLog(typed)
	return stopHookRender(root, typed)
}

// ── the duplicate-invocation guard ──────────────────────────────────────────

// stopHookDedupSuppresses reports whether this invocation is a duplicate that
// must exit silently. It also writes (and prunes) the token as a side effect,
// exactly where the bash does.
func stopHookDedupSuppresses(inv *Invocation, root string) bool {
	if os.Getenv("EDIKT_DISABLE_HOOK_DEDUP") == "1" {
		return false
	}
	if !dirExists(filepath.Join(root, ".edikt")) {
		return false
	}
	dedupDir := filepath.Join(root, ".edikt", "state", ".hook-dedup")
	_ = os.MkdirAll(dedupDir, 0o755)

	// The key is over the RAW payload bytes, which never cross the seam —
	// see adapter.Payload.PayloadDigest for why a digest does and the bytes
	// do not.
	key := inv.Payload.PayloadDigest(inv.Name + ":")
	if len(key) > 40 {
		key = key[:40]
	}
	if key == "" || !dirExists(dedupDir) {
		return false
	}
	token := filepath.Join(dedupDir, key)

	if fi, err := os.Stat(token); err == nil && fi.Mode().IsRegular() {
		// Age in whole seconds, truncated toward zero — python's own
		// `int(time.time() - os.path.getmtime(f))`. A negative age (a token
		// stamped in the future) is UNKNOWN, never fresh: suppressing on it
		// would silence a real emission (INV-013), which is why the bash
		// tests `-ge 0` as well as `-lt 15`.
		age := time.Since(fi.ModTime())
		if age >= 0 && age < stopHookDedupTTL {
			return true
		}
	}

	_ = truncateOrCreate(token)
	stopHookPruneDedup(dedupDir)
	return false
}

// stopHookPruneDedup mirrors `find "$dir" -type f -mmin +5 -delete`.
func stopHookPruneDedup(dir string) {
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
		if time.Since(info.ModTime()) > stopHookDedupPrune {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// ── signal 1: adr-candidate, with the G0 "already captured" gate ────────────

func stopHookADRCandidate(root string, configBytes []byte, lastMsg string) []typedSignal {
	if !stopHookADRTriggerRe.MatchString(lastMsg) {
		return nil
	}
	subject := stopHookSanitizeSubject(stopHookADRSubjectRe.FindString(lastMsg))
	if subject == "" {
		// ADR-050: the subject is MANDATORY. An adr-candidate that cannot name
		// the decision it detected does not emit at all.
		return nil
	}
	if stopHookG0Hit(root, configBytes, subject) {
		return nil
	}
	return []typedSignal{{
		kind:     "adr-candidate",
		subject:  subject,
		evidence: "decision language detected in the last response",
		ask:      "run /edikt:adr:new — the intake gates (GL-001) will classify it",
	}}
}

// stopHookSanitizeSubject mirrors:
//
//	printf '%s' "$1" | tr -cd 'A-Za-z0-9 ._/-' | cut -c1-80
//
// Byte-wise on purpose: the recorder runs the hook under LC_ALL=C, where
// `cut -c` counts bytes and `tr -cd` deletes every byte outside the set —
// including every byte of a multi-byte rune. The subject is attacker-
// influenceable (it comes out of the model's own last turn), and this is the
// only thing standing between it and the renderer.
func stopHookSanitizeSubject(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out = append(out, c)
		case c == ' ', c == '.', c == '_', c == '/', c == '-':
			out = append(out, c)
		}
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return string(out)
}

// stopHookG0Hit is the "already captured" gate: three independent lookups
// (governance titles, compiled sidecar signals, spec provenance) against each
// significant word of the detected subject. A hit on ANY word suppresses.
func stopHookG0Hit(root string, configBytes []byte, subject string) bool {
	base := stopHookConfigBase(configBytes)
	dirs := []string{
		filepath.Join(root, base, "decisions"),
		filepath.Join(root, base, "architecture", "decisions"),
		filepath.Join(root, base, "architecture", "invariants"),
	}
	specDir := filepath.Join(root, base, "product", "specs")

	for _, term := range stopHookSubjectTerms(subject) {
		// G0a: the term appears in a governance file AND on the first line of
		// one of the first three files that carry it — a title match, not a
		// body mention. `head -1 $(…| head -3)` prints a `==> path <==` header
		// per file when there is more than one, and the bash greps THAT whole
		// stream, so a term appearing only in a path still counts. Ported.
		if matches := grepRecursiveList(dirs, term, ""); len(matches) > 0 {
			if strings.Contains(strings.ToLower(headFirstLines(matches, 3)), strings.ToLower(term)) {
				return true
			}
		}
		// G0b: a compiled sidecar lists the term as a signal (`- <term>`).
		if grepRecursiveQuiet([]string{filepath.Join(root, base)}, "- "+term, "*.edikt.yaml") {
			return true
		}
		// G0c: a spec yaml mentions the term AND at least one such file
		// carries provenance.
		specHits := grepRecursiveList([]string{specDir}, term, "*.yaml")
		if len(specHits) > 0 && anyFileContainsAny(specHits, []string{"provenance", "source:"}) {
			return true
		}
	}
	return false
}

// stopHookSubjectTerms mirrors:
//
//	printf '%s' "$SUBJ" | tr '[:upper:]' '[:lower:]' | tr -s ' ' '\n' \
//	  | grep -vE '^(chose|over|…)$'
//
// `tr -s` squeezes runs of spaces into ONE newline, and the unquoted command
// substitution that consumes it drops empty fields, so this is "lowercase
// words, minus the stop list".
func stopHookSubjectTerms(subject string) []string {
	var out []string
	for _, w := range strings.Split(strings.ToLower(subject), " ") {
		if w == "" || stopHookStopWords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// stopHookConfigBase mirrors:
//
//	grep '^base:' .edikt/config.yaml | awk '{print $2}' | tr -d '"'
//
// with "docs" as the default. Every matching line contributes its own second
// field (the bash pipeline does not stop at the first), and `tr -d` removes
// quotes anywhere rather than only at the ends.
func stopHookConfigBase(configBytes []byte) string {
	var fields []string
	for _, line := range strings.Split(string(configBytes), "\n") {
		if !strings.HasPrefix(line, "base:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 {
			fields = append(fields, f[1])
		} else {
			fields = append(fields, "")
		}
	}
	joined := strings.ReplaceAll(strings.Join(fields, "\n"), `"`, "")
	joined = strings.TrimRight(joined, "\n")
	if joined == "" {
		return "docs"
	}
	return joined
}

// ── signal 2: doc-gap ───────────────────────────────────────────────────────

func stopHookDocGap(lastMsg string) []typedSignal {
	// Audit HI-5: presence-of-signal only. The matched substring becomes the
	// SUBJECT (sanitized) but never the wording of the ask, so an attacker-
	// controlled "POST /admin/delete-everything" cannot phrase the suggestion.
	newRoute := stopHookRouteRe.FindString(lastMsg)
	if newRoute != "" {
		return []typedSignal{{
			kind:     "doc-gap",
			subject:  stopHookSanitizeSubject(newRoute),
			evidence: "new HTTP route referenced",
			ask:      "consider /edikt:docs:review to check documentation",
		}}
	}
	newEnv := ""
	for _, m := range stopHookEnvVarRe.FindAllString(lastMsg, -1) {
		if stopHookContainsAny(m, stopHookEnvVarExcludes) {
			continue // `grep -v 'ADR\|ARCH\|HTTP\|API\|JSON\|HTML\|CSS'`
		}
		newEnv = m
		break // `head -1`
	}
	if newEnv == "" {
		return nil
	}
	return []typedSignal{{
		kind:     "doc-gap",
		subject:  stopHookSanitizeSubject(newEnv),
		evidence: "new environment variable referenced",
		ask:      "consider /edikt:docs:review to check documentation",
	}}
}

func stopHookContainsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// ── signal 3: security-audit ────────────────────────────────────────────────

func stopHookSecurity(lastMsg string) []typedSignal {
	matches := stopHookSecurityRe.FindAllString(lastMsg, -1)
	if len(matches) == 0 {
		return nil
	}
	// `SEC_COUNT` is `grep -o … | wc -l`: the number of non-overlapping
	// matches, not the number of distinct terms. Two mentions of the same
	// word clear the threshold, and that is the bash's own rule.
	if len(matches) < 2 {
		return nil
	}
	return []typedSignal{{
		kind:     "security-audit",
		subject:  fmt.Sprintf("security-sensitive change (%d terms)", len(matches)),
		evidence: "auth/payments/PII/crypto central to the last response",
		ask:      "run /edikt:sdlc:audit before shipping",
	}}
}

// ── signal 4: sidecar drift ─────────────────────────────────────────────────

// stopHookSidecarDrift is INV-013 in miniature and the reason this hook exists
// in its current shape.
//
// Three outcomes, and all three MUST be distinguishable at the hook's own
// output:
//
//	the binary could not be resolved, or answered unusably → UNCHECKED
//	the check ran and named stale artifacts                → sidecar-stale
//	the check ran and found none                           → nothing at all
//
// The first two emit different kinds with different wording; the third is
// silent because there is nothing to report. What must never happen is the
// first collapsing into the third — "I could not look" rendered as "I looked
// and everything is fine". That is the defect INV-013 is named after, and it
// is asserted on literal bytes in TestStopHookStaleSidecars.
func stopHookSidecarDrift(root string) []typedSignal {
	staleCount := 0
	staleIDs := ""
	unchecked := false

	bin := stopHookResolveBinary(root)
	if bin == "" {
		unchecked = true
	} else {
		out := stopHookGovCompileCheck(root, bin)
		if out == "" {
			unchecked = true
		} else if count, ids, ok := stopHookParseStaleJSON(out); !ok {
			unchecked = true
		} else {
			staleCount, staleIDs = count, ids
		}
	}

	// Derived state carries its provenance, or it is regenerated: a reader
	// must be able to tell "N stale sidecars" from "an N-entry file nobody
	// updated". Both non-writing branches REMOVE the log rather than leaving
	// yesterday's answer behind.
	staleLog := filepath.Join(root, ".edikt", "state", "stale-sidecars.log")
	switch {
	case unchecked:
		_ = os.Remove(staleLog)
	case staleCount > 0:
		stopHookWriteStaleLog(root, staleLog, staleIDs)
	default:
		_ = os.Remove(staleLog)
	}

	// Shape guards, applied AFTER the log write exactly as the bash does.
	if !stopHookAllDigits(strconv.Itoa(staleCount)) {
		staleCount = 0
	}
	if strings.ContainsFunc(staleIDs, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' ||
			r == ',' || r == '.' || r == '_' || r == '-')
	}) {
		staleIDs = ""
	}

	// INV-013's own Boundary clause, and the one place this hook deliberately
	// says NOTHING about a check that did not run: a project with no sidecars
	// has no staleness to determine, so announcing non-coverage on every Stop
	// would be noise. "I could not check" is worth saying; "there was nothing
	// to check" is not. The subject test is a literal `docs` walk in the bash
	// — NOT the configured base — and is ported that way.
	hasSidecars := false
	if dirExists(filepath.Join(root, "docs")) {
		hasSidecars = findFirstMatch(filepath.Join(root, "docs"), "*.edikt.yaml")
	}

	switch {
	case unchecked && hasSidecars:
		return []typedSignal{{
			kind:     "sidecar-stale-unchecked",
			subject:  "sidecar staleness",
			evidence: "not checked — the edikt binary is unavailable, so no staleness determination was made",
			ask:      "install the binary (see README → Install) to restore the check; this is not a report that sidecars are current",
		}}
	case staleCount > 0:
		subject := staleIDs
		if subject == "" {
			subject = fmt.Sprintf("%d artifacts", staleCount)
		}
		return []typedSignal{{
			kind:     "sidecar-stale",
			subject:  subject,
			evidence: "sidecar anchors no longer match the prose",
			ask:      "run /edikt:gov:compile to resync, or hold with: edikt hook ack <fingerprint> --until <event> --why <reason>",
		}}
	}
	return nil
}

// stopHookResolveBinary is the four-rung discovery ladder, most specific
// first. Rung 2 (`bin/edikt`) is deliberately BELOW rung 1
// (`.edikt/bin/edikt`): only the installer ever creates the latter, while
// `bin/edikt` is an ordinary relative path any Go project might occupy, and a
// downstream repo's unrelated binary must not shadow their real install.
// Returning "" is rung 5 — UNCHECKED, never a silent "0 stale".
func stopHookResolveBinary(root string) string {
	ediktRoot := os.Getenv("EDIKT_ROOT")
	if ediktRoot == "" {
		ediktRoot = os.Getenv("EDIKT_HOME")
	}
	if ediktRoot == "" {
		ediktRoot = filepath.Join(os.Getenv("HOME"), ".edikt")
	}
	for _, cand := range []string{
		filepath.Join(root, ".edikt", "bin", "edikt"),
		filepath.Join(root, "bin", "edikt"),
		filepath.Join(ediktRoot, "bin", "edikt"),
	} {
		if isExecutable(cand) {
			return cand
		}
	}
	if commandAvailable("edikt") {
		// The bare name, resolved through PATH at exec time — bash's own
		// `_edikt_bin="edikt"`, not the absolute path `command -v` printed.
		return "edikt"
	}
	return ""
}

// stopHookGovCompileCheck runs the ONE canonical staleness predicate.
//
// stop-hook.sh used to carry a ~90-line python reimplementation of the drift
// check. It diverged from internal/sidecar/drift.go — which tolerates
// whitespace and line shift — and reported two artifacts stale while
// `gov compile --check` said zero. Both false positives were then ACKed, and
// the held ack swallowed a genuine finding into the same bundle. Exit code
// plus one JSON field, per the tier-2 consumption contract; never a second
// implementation.
func stopHookGovCompileCheck(root, bin string) string {
	cmd := exec.Command(bin, "gov", "compile", "--check", "--json") //nolint:gosec // discovery-ladder path
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "EDIKT_VERIFY_TRUST=1", "EDIKT_SKIP_VERSION_GATE=1")
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	// `$( )` strips trailing newlines; an all-whitespace answer is "" to the
	// `[ -z … ]` that follows.
	return strings.TrimRight(string(out), "\n")
}

// stopHookParseStaleJSON reproduces the embedded python's deliberately picky
// decode. ok=false means UNCHECKED, and each refusal below is one of INV-013's
// own listed shapes:
//
//	no phase_a               the binary answered something else entirely
//	stale_ids ABSENT         an older binary predates the field — `.get(k, [])`
//	                         here would turn "this binary cannot tell you" into
//	                         "nothing is stale"
//	stale_ids not a list     the field exists and means something else
func stopHookParseStaleJSON(body string) (count int, ids string, ok bool) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return 0, "", false
	}
	rawPhase, present := doc["phase_a"]
	if !present {
		return 0, "", false
	}
	var phase map[string]json.RawMessage
	if err := json.Unmarshal(rawPhase, &phase); err != nil {
		return 0, "", false
	}
	rawIDs, present := phase["stale_ids"]
	if !present {
		return 0, "", false
	}
	var list []json.RawMessage
	if err := json.Unmarshal(rawIDs, &list); err != nil {
		return 0, "", false
	}
	head := list
	if len(head) > 6 {
		head = head[:6] // `ids[:6]`
	}
	rendered := make([]string, 0, len(head))
	for _, item := range head {
		var s string
		if json.Unmarshal(item, &s) == nil {
			rendered = append(rendered, s)
			continue
		}
		// `str(i)` over a non-string id. Real ids are strings; the raw JSON
		// text is the closest faithful rendering for anything else, and the
		// allowlist guard downstream discards whatever it does not like.
		rendered = append(rendered, string(item))
	}
	return len(list), strings.Join(rendered, ","), true
}

// stopHookWriteStaleLog writes the out-of-band artifact /edikt:gov:compile
// consumes. Best-effort at every step, like the bash's `|| true` chain.
func stopHookWriteStaleLog(root, path, staleIDs string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	head := "unknown"
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		if trimmed := strings.TrimRight(string(out), "\n"); trimmed != "" {
			head = trimmed
		}
	}
	var b strings.Builder
	b.WriteString("# generated by stop-hook.sh from `edikt gov compile --check --json`\n")
	b.WriteString("# generated_at: " + time.Now().UTC().Format("2006-01-02T15:04:05Z") + "\n")
	b.WriteString("# corpus_head: " + head + "\n")
	b.WriteString(strings.ReplaceAll(staleIDs, ",", "\n") + "\n")
	_ = os.WriteFile(path, []byte(b.String()), 0o644) //nolint:gosec // matches the bash redirect's default mode
}

// ── signal 5: completion-claim without evidence ─────────────────────────────

func stopHookCompletionClaim(root string, configBytes []byte, lastMsg string) []typedSignal {
	if strings.Contains(string(configBytes), "quality-gates: false") {
		return nil
	}
	if !stopHookCompletionRe.MatchString(lastMsg) {
		return nil
	}
	count, plan, phase := stopHookScanPlans(root, configBytes)

	// Defence in depth: the same allowlists the embedded python already
	// applied, re-applied to its output. A hand-tampered config cannot smuggle
	// text into a user-facing message through this path.
	if !stopHookAllDigits(strconv.Itoa(count)) {
		count = 0
	}
	if plan == "" || strings.ContainsFunc(plan, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-')
	}) {
		plan = ""
	}
	if phase == "" || strings.ContainsFunc(phase, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		phase = ""
	}
	if count <= 0 || plan == "" || phase == "" {
		return nil
	}
	return []typedSignal{{
		kind:     "completion-claim",
		subject:  plan + " phase " + phase,
		evidence: "completion phrase during an in-progress plan phase",
		ask:      "run: bin/edikt verify " + plan + " --phase " + phase + " before declaring done",
	}}
}

// stopHookPlansDir mirrors the embedded python's hardened `paths:` walk:
// scan for a top-level `paths:` key, stop at the first non-indented line
// after it, and take `plans:`. A traversal or absolute value is REFUSED back
// to the default rather than honoured.
func stopHookPlansDir(configBytes []byte) string {
	const fallback = "docs/internal/plans"
	inPaths := false
	for _, raw := range strings.Split(string(configBytes), "\n") {
		line := strings.TrimRight(raw, "\r")
		if !inPaths {
			if strings.HasPrefix(line, "paths:") {
				inPaths = true
			}
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if !strings.HasPrefix(s, "plans:") {
			continue
		}
		v := strings.TrimSpace(strings.SplitN(s, ":", 2)[1])
		if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
			v = v[1 : len(v)-1]
		} else if len(v) >= 2 && strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") {
			v = v[1 : len(v)-1]
		}
		for _, seg := range strings.Split(v, "/") {
			if seg == ".." {
				return fallback
			}
		}
		if strings.HasPrefix(v, "/") {
			return fallback
		}
		return v
	}
	return fallback
}

// stopHookScanPlans counts plan files carrying an in-progress phase row and
// names the first one. A plans directory that cannot be listed yields
// (0, "", "") — the python's own `print("0||")` on OSError.
func stopHookScanPlans(root string, configBytes []byte) (count int, firstPlan, firstPhase string) {
	dir := stopHookPlansDir(configBytes)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, "", ""
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names) // `sorted(os.listdir(plans_dir))`
	for _, name := range names {
		if !strings.HasPrefix(name, "PLAN-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		planID := strings.TrimSuffix(strings.TrimPrefix(name, "PLAN-"), ".md")
		if !stopHookPlanIDRe.MatchString(planID) {
			continue
		}
		body, rerr := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // config-declared plans dir
		if rerr != nil {
			continue
		}
		for _, line := range strings.Split(string(body), "\n") {
			m := stopHookPlanRowRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if firstPlan == "" {
				firstPlan, firstPhase = planID, m[1]
			}
			count++
			break
		}
	}
	return count, firstPlan, firstPhase
}

// ── output: the session log and the ledger-aware renderer (ADR-050) ─────────

// stopHookAppendSessionLog writes the record /edikt:status reads.
//
// This loop used to iterate the free-text SIGNALS array only. ADR-050 moved
// every producer to the typed channel and removed the appends, leaving the
// loop reading a permanently empty array: the hook kept emitting messages
// while writing nothing to the log, and /edikt:status reported no signal
// activity for any session. Nothing failed — the loop ran zero times and the
// `|| true` swallowed the rest.
func stopHookAppendSessionLog(typed []typedSignal) {
	home := os.Getenv("HOME")
	if home == "" {
		return
	}
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	f, err := os.OpenFile(filepath.Join(home, ".edikt", "session-signals.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // matches the bash `>>` default mode
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	for _, t := range typed {
		// The kind is written verbatim and lowercase so consumers grep a
		// stable machine token rather than prose. The subject is already
		// sanitized upstream and must not re-enter unsanitized.
		_, _ = fmt.Fprintf(f, "%s %s %s\n", ts, t.kind, t.subject)
	}
}

// stopHookLedgerRecord is one appended ledger line. The two shapes differ by
// exactly one field and the field ORDER is part of the bytes the corpus pins,
// so `evidence` is omitempty rather than a second struct: a held record
// carries no evidence and a surfaced one always does.
type stopHookLedgerRecord struct {
	TS          string `json:"ts"`
	Event       string `json:"event"`
	Kind        string `json:"kind"`
	Subject     string `json:"subject"`
	Evidence    string `json:"evidence,omitempty"`
	Fingerprint string `json:"fingerprint"`
}

func stopHookRender(root string, typed []typedSignal) Result {
	state := filepath.Join(root, ".edikt", "state")
	ledgerPath := filepath.Join(state, "hook-ledger.jsonl")
	acksPath := filepath.Join(state, "hook-acks.json")

	known := stopHookKnownFingerprints(ledgerPath)
	acks := stopHookLoadAcks(acksPath)

	var fresh, held []string
	var appends []stopHookLedgerRecord
	now := func() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

	for _, t := range typed {
		sum := sha256.Sum256([]byte(t.kind + "|" + t.subject + "|" + t.evidence))
		fp := hex.EncodeToString(sum[:])[:16]
		icon, hasIcon := stopHookIcons[t.kind]
		if !hasIcon {
			icon = "•"
		}
		ack, acked := acks[fp]
		switch {
		// `if ack and ack_active(ack)` — an explicit human hold wins over
		// every other branch, including the no-collapse rule below.
		case acked && stopHookAckActive(root, ack):
			why := ack.Why
			if len(why) > 60 {
				why = why[:60]
			}
			held = append(held, fmt.Sprintf("⏸ held: %s %s [%s] (ack: %s)", t.kind, t.subject, fp, why))
			appends = append(appends, stopHookLedgerRecord{
				TS: now(), Event: "held", Kind: t.kind, Subject: t.subject, Fingerprint: fp,
			})
		case known[fp] && !stopHookNoCollapseKinds[t.kind]:
			// F-082: this branch used to drop `ask` on the floor, so a signal
			// that recurs across Stop firings surfaced its remediation exactly
			// once and every firing after that offered only suppression — the
			// reader's sole reachable action for an unresolved condition was
			// to silence it, not fix it. No ledger append here, deliberately:
			// the fingerprint is already recorded.
			held = append(held, fmt.Sprintf("⏸ 1 known item held: %s %s [%s] — %s (ack with --why to silence details)",
				t.kind, t.subject, fp, t.ask))
		default:
			fresh = append(fresh, fmt.Sprintf("%s %s: %s — %s. %s [%s]", icon, t.kind, t.subject, t.evidence, t.ask, fp))
			appends = append(appends, stopHookLedgerRecord{
				TS: now(), Event: "surfaced", Kind: t.kind, Subject: t.subject,
				Evidence: t.evidence, Fingerprint: fp,
			})
		}
	}

	stopHookAppendLedger(state, ledgerPath, appends)

	lines := append(append([]string{}, fresh...), held...)
	if len(lines) == 0 {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}
	return Result{Response: adapter.Response{Message: strings.Join(lines, "\n")}}
}

// stopHookAppendLedger opens the ledger in append mode even when there is
// nothing to write — python's `with open(path, "a")` CREATES the file
// regardless, and an empty hook-ledger.jsonl is part of the recorded
// filesystem delta for a firing whose every record collapsed.
func stopHookAppendLedger(state, path string, records []stopHookLedgerRecord) {
	if err := os.MkdirAll(state, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // project-local state
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	for _, rec := range records {
		body, merr := adapter.MarshalPythonCompact(rec)
		if merr != nil {
			continue
		}
		_, _ = f.Write(append(body, '\n'))
	}
}

func stopHookKnownFingerprints(path string) map[string]bool {
	known := map[string]bool{}
	body, err := os.ReadFile(path) //nolint:gosec // project-local state
	if err != nil {
		return known
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Fingerprint string `json:"fingerprint"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue // a malformed line is skipped, never fatal
		}
		if rec.Fingerprint != "" {
			known[rec.Fingerprint] = true
		}
	}
	return known
}

// stopHookAck is one entry of .edikt/state/hook-acks.json.
type stopHookAck struct {
	Until string `json:"until"`
	Why   string `json:"why"`
	Head  string `json:"head"`
}

// stopHookLoadAcks reads the ack file, keyed by kind+"\x00"+fingerprint.
//
// DIVERGENCE, STATED. The bash renderer crashes with an AttributeError on an
// ack whose value is not an object, and because that python block is the
// script's LAST command the hook then exits non-zero — at Stop, the worst
// available outcome for a malformed local state file. ADR-063's own
// Consequences require the Go tier to fail open, so a non-object entry is
// treated as no ack at all. Nothing else about the ack path changes.
func stopHookLoadAcks(path string) map[string]stopHookAck {
	out := map[string]stopHookAck{}
	body, err := os.ReadFile(path) //nolint:gosec // project-local state
	if err != nil {
		return out
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return out
	}
	for fp, item := range raw {
		var a stopHookAck
		if json.Unmarshal(item, &a) != nil {
			continue // see the DIVERGENCE note above
		}
		out[fp] = a
	}
	return out
}

// stopHookAckActive evaluates an ack's expiry. UNKNOWN SHAPES COUNT AS ACTIVE:
// a visible hold is safer than a silent re-fire storm while an operator is
// still working out what the entry meant.
func stopHookAckActive(root string, a stopHookAck) bool {
	const commitPrefix = "commit-touching:"
	if strings.HasPrefix(a.Until, commitPrefix) {
		path := strings.TrimPrefix(a.Until, commitPrefix)
		cmd := exec.Command("git", "log", "-1", "--format=%H", "--", path)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return true
		}
		current := strings.TrimSpace(string(out))
		return current == a.Head || current == ""
	}
	if a.Until == "compile-clean" {
		return true // cleared by unack or a clean compile flow
	}
	head := a.Until
	if len(head) > 10 {
		head = head[:10]
	}
	d, err := time.ParseInLocation("2006-01-02", head, time.Local)
	if err != nil {
		return true
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return !d.Before(today)
}

// ── small shell equivalents ─────────────────────────────────────────────────

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// isExecutable mirrors `[ -x path ]`, including its acceptance of a directory
// with the execute bit — the bash then fails to exec it and degrades to
// UNCHECKED, which is the correct answer either way.
func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().Perm()&0o111 != 0
}

func stopHookAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// findFirstMatch mirrors `find <dir> -name '<glob>' -print -quit | grep -q .`:
// does at least one entry anywhere under dir match the basename glob.
func findFirstMatch(dir, glob string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ok, merr := filepath.Match(glob, d.Name()); merr == nil && ok {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// grepRecursiveList mirrors `grep -rilF -- <needle> <dirs…>` (optionally with
// `--include=<glob>`): every regular file under any of dirs whose CONTENT
// contains needle, case-insensitively, as a fixed string.
//
// Symlinks are not followed — `-r` does not follow them; `-R` would. The
// result is sorted, which the bash's own ordering is not: grep -r walks in
// directory order, so `| head -3` picks an unspecified three when more than
// three files match. Sorting here makes the Go side reproducible; it can only
// differ from bash where the bash was already unspecified.
func grepRecursiveList(dirs []string, needle, includeGlob string) []string {
	lowerNeedle := strings.ToLower(needle)
	var out []string
	for _, dir := range dirs {
		if !dirExists(dir) {
			continue // grep prints an error to the suppressed stderr and moves on
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if includeGlob != "" {
				if ok, merr := filepath.Match(includeGlob, d.Name()); merr != nil || !ok {
					return nil
				}
			}
			body, rerr := os.ReadFile(p) //nolint:gosec // inside a project-declared governance dir
			if rerr != nil {
				return nil
			}
			if strings.Contains(strings.ToLower(string(body)), lowerNeedle) {
				out = append(out, p)
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func grepRecursiveQuiet(dirs []string, needle, includeGlob string) bool {
	return len(grepRecursiveList(dirs, needle, includeGlob)) > 0
}

func anyFileContainsAny(paths []string, needles []string) bool {
	for _, p := range paths {
		body, err := os.ReadFile(p) //nolint:gosec // caller-supplied match list
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(body))
		for _, n := range needles {
			if strings.Contains(lower, strings.ToLower(n)) {
				return true
			}
		}
	}
	return false
}

// headFirstLines mirrors `head -1 <file…>` over at most max files: the bare
// first line for a single file, and the `==> path <==` banner form (blank
// line between files) for more than one. The banner carries the PATHS, which
// is why the caller's fixed-string search can hit on a filename.
func headFirstLines(paths []string, max int) string {
	if len(paths) > max {
		paths = paths[:max]
	}
	var b strings.Builder
	for i, p := range paths {
		body, err := os.ReadFile(p) //nolint:gosec // caller-supplied match list
		if err != nil {
			continue
		}
		first := body
		if idx := strings.IndexByte(string(body), '\n'); idx >= 0 {
			first = body[:idx]
		}
		if len(paths) > 1 {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("==> " + p + " <==\n")
		}
		b.Write(first)
		b.WriteString("\n")
	}
	return b.String()
}
