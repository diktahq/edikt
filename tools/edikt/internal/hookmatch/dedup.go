package hookmatch

// dedup.go — "bounce once per session, not every write".
//
// WHY DEDUP AT ALL
//
// The PreToolUse bounce denies a write so the agent re-reads the directive and
// retries. Bouncing the SAME directive on every subsequent write would turn a
// governance signal into a loop the agent cannot escape, and the rational
// response to an inescapable gate is to route around it.
//
// So: deny once per (session, directive-set), then allow. The agent has been
// told; telling it again on every write is noise, not enforcement.
//
// WHERE THE STATE LIVES, AND WHY NOT /tmp
//
// Under `~/.edikt/state/hook-dedup/`, mode 0700. Not /tmp, for two reasons
// that both matter: /tmp is world-writable, so another local user could plant
// a dedup record and suppress a bounce that should have fired; and /tmp is
// cleared on reboot, which would silently re-bounce every session after a
// restart. A suppression primitive must not be plantable by anyone but its
// owner.
//
// WHY IT KEYS ON THE MATCHED ENTRIES' OWN CONTENT, NOT THE WHOLE INDEX
//
// mtime would be wrong in both directions. A no-op recompile touches mtime
// without changing a single directive, which would re-bounce for nothing; and
// a file restored from backup can carry an OLD mtime with NEW content, which
// would suppress a bounce that should fire. The content is the thing that
// matters, so the content is what is hashed.
//
// F-080: the key used to embed the SHA-256 of the WHOLE directive index file,
// not just the entries this write actually matched. Recompiling ANY other
// artifact changes that whole-file hash, so a write governed by an unrelated,
// byte-identical directive re-bounced anyway — measured directly: a retry on
// nearly every governed write across two days of session work that kept
// recompiling different artifacts. The key now hashes the matched entries'
// own fields (id, grade, text, falsifying_observation, reminders) — content
// that CHANGES exactly when the rule this write is being told about changes,
// and does not move when something else in the corpus does.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DedupDir is the per-user state directory.
func DedupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".edikt", "state", "hook-dedup")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Re-assert the mode: MkdirAll leaves an EXISTING directory's permissions
	// alone, so a dir created loosely by an older version (or by a umask that
	// widened it) would stay world-readable forever without this.
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// entriesContentHash hashes the matched entries' OWN content — not the
// index they were read from. Sorted by ID first so the hash is independent
// of match order; each entry contributes every field that changes what the
// agent would be told, so a genuine edit to the rule still changes the key
// (F-080 dropped only the whole-index component, not content-sensitivity).
func entriesContentHash(entries []Entry) string {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var b strings.Builder
	for _, e := range sorted {
		b.WriteString(e.ID)
		b.WriteByte(0)
		b.WriteString(e.Grade)
		b.WriteByte(0)
		b.WriteString(e.Text)
		b.WriteByte(0)
		b.WriteString(e.FalsifyingObservation)
		b.WriteByte(0)
		b.WriteString(strings.Join(e.Reminders, "\x1f"))
		b.WriteByte(0)
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// dedupKey identifies (session, matched-entry content, directive set).
//
// The ENTRY IDS are in the key, not just the content hash. Two different
// paths in the same session match different directives, and a key that
// ignored which directives fired would bounce the first one and silently
// swallow every other rule for the rest of the session — the exact
// silent-suppression this phase exists to make impossible.
// contextID is the RECEIVING CONTEXT — the agent_id of the dispatch that will
// read the injection, or "parent" for the main session. Keying on the session
// alone was measured wrong: a parent bounce wrote a marker, and every subagent
// in that session inherited the suppression and received nothing while the
// journal showed dedup_suppressed=true. "Once per session" stopped being the
// right unit the moment a session could contain agents.
func dedupKey(sessionID, contextID string, entries []Entry) string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	h := sha256.Sum256([]byte(sessionID + "\x00" + contextID + "\x00" + entriesContentHash(entries) + "\x00" + strings.Join(ids, ",")))
	return hex.EncodeToString(h[:])
}

// budgetKey identifies (session, matched-entry content, directive set) — the
// SAME components as dedupKey, MINUS contextID. Per-context dedup already
// bounds a single receiving context to one bounce; this key is deliberately
// coarser so the budget counter it names can bound the AGGREGATE across
// every distinct context a session dispatches, which per-context keying
// does not bound by itself (see AlreadyBounced).
func budgetKey(sessionID string, entries []Entry) string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	h := sha256.Sum256([]byte(sessionID + "\x00" + entriesContentHash(entries) + "\x00" + strings.Join(ids, ",")))
	return hex.EncodeToString(h[:])
}

// BounceBudget caps how many times one directive set may bounce within a
// session across ALL contexts, after which the write proceeds and the caller
// is told to surface the directives as advisory instead.
//
// Keying per context bounds bounces by the number of dispatches rather than by
// writes, so a loop is already unlikely — but the guard is explicit rather than
// relying on the key's shape to stay right. No silent allow and no infinite
// deny: past the budget the write proceeds LOUDLY.
//
// BounceBudget is the DEFAULT and the fallback when a caller passes a
// non-positive value (e.g. hookcfg.Load could not be consulted). The
// effective value normally comes from .edikt/config.yaml
// (hooks.injection.bounce_budget) with a floor of 1 — a strength decision
// compiled into a binary is a strength decision the team cannot review, which
// is the same defect as enforcement living in a user's own settings.
const BounceBudget = 8

// BounceResult is the dedup+budget decision for one directive-set match.
type BounceResult struct {
	// Bounce is true when this write should be DENIED: either the receiving
	// context has never been told about this exact directive set, and the
	// session's aggregate budget for that set has not been spent, or the
	// receiving context is unresolvable (UNKNOWN MEANS DELIVER).
	Bounce bool
	// BudgetExhausted is true when this receiving context has NOT been told
	// before (so per-context dedup alone would bounce), but the session's
	// aggregate budget for this exact directive set — summed across every
	// distinct context that has bounced on it — has already been spent. The
	// write proceeds (Bounce is false), but the caller MUST still surface the
	// directive set as advisory rather than silence: "proceeds LOUDLY", not
	// "proceeds unheard".
	BudgetExhausted bool
}

// AlreadyBounced reports the dedup+budget decision for
// (session, context, directive-set), recording state when it decides
// to deliver (as either a deny or an advisory). The directive set's own
// content — not the index file it came from — is what the key is sensitive
// to (F-080).
//
// TWO KEYS, ON PURPOSE. Per-context dedup (dedupKey) answers "has THIS
// receiving context already been told" and is unconditional: exactly one
// bounce per distinct context, forever, regardless of budget — this is what
// keeps a single subagent from being re-bounced on every write. The budget
// counter (budgetKey) answers a different question: "how many DISTINCT
// contexts has this exact directive set already bounced, this session" — and
// bounds that aggregate, because per-context keying by itself does not: a
// session that dispatches many subagents against the same governed path would
// otherwise earn one fresh bounce per dispatch with no ceiling.
//
// An empty contextID means identity could not be resolved, and the answer is
// then always "not yet bounced" — DELIVER (as a bounce) rather than suppress,
// unconditionally, ignoring the budget. An extra bounce costs one regenerated
// write and is visible; a silent suppression costs the enforcement claim and
// is not.
func AlreadyBounced(sessionID, contextID string, entries []Entry, budget int) (BounceResult, error) {
	if contextID == "" {
		return BounceResult{Bounce: true}, nil
	}
	if sessionID == "" || len(entries) == 0 {
		return BounceResult{Bounce: true}, nil
	}
	if budget < 1 {
		budget = BounceBudget
	}
	dir, err := DedupDir()
	if err != nil {
		return BounceResult{Bounce: true}, err
	}

	perContext := filepath.Join(dir, dedupKey(sessionID, contextID, entries))
	if _, err := os.Stat(perContext); err == nil {
		// This exact receiving context has already been told, by whichever
		// channel — deny or advisory. Silent from here on, unconditionally;
		// the budget does not apply to a repeat visit from the same context.
		return BounceResult{}, nil
	} else if !os.IsNotExist(err) {
		return BounceResult{Bounce: true}, err
	}

	// A fresh context for this directive set: it earns a bounce under
	// per-context rules. Whether that bounce is a deny or an advisory depends
	// on the session's aggregate spend for this exact set, tracked
	// separately from any one context.
	budgetFile := filepath.Join(dir, "budget-"+budgetKey(sessionID, entries))
	count := 0
	if b, err := os.ReadFile(budgetFile); err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	} else if !os.IsNotExist(err) {
		return BounceResult{Bounce: true}, err
	}
	count++
	if err := os.WriteFile(budgetFile, []byte(strconv.Itoa(count)+"\n"), 0o600); err != nil {
		return BounceResult{Bounce: true}, err
	}
	if err := os.WriteFile(perContext, []byte("bounced\n"), 0o600); err != nil {
		return BounceResult{Bounce: true}, err
	}

	if count > budget {
		return BounceResult{BudgetExhausted: true}, nil
	}
	return BounceResult{Bounce: true}, nil
}
