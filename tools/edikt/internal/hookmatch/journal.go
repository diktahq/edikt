package hookmatch

// journal.go — SUPPRESSION DETECTABILITY (AC-3.8).
//
// THE PROBLEM THIS EXISTS FOR
//
// Once the hook is the only channel for MUST-grade write-touch directives,
// these five situations produce an IDENTICAL observable — a write that
// proceeds with nothing injected:
//
//   1. no directive covers this path            (correct, governance ran)
//   2. the binary is absent                     (governance did not run)
//   3. the index is missing                     (governance did not run)
//   4. the index is corrupt                     (governance did not run)
//   5. the path was crafted to normalise oddly  (governance did not run)
//
// Only the first is a pass. Fail-open is still right — blocking an editor on a
// governance bug is worse than missing an injection — but SILENT fail-open
// means the difference between "governed and clean" and "not governed at all"
// is invisible, indefinitely, to everyone. That is INV-013 at the level of a  edikt-guard:allow
// whole enforcement channel.
//
// TWO SURFACES, DELIBERATELY
//
// A heartbeat alone is insufficient: it proves the binary ran, not that the
// chain ran for the writes that mattered. A stop-surface report alone is
// insufficient: it fires at session end, and a session that never stops never
// reports.
//
// So both:
//
//   HEARTBEAT  every match writes a record carrying its Outcome. `doctor` reads
//              them and reports counts per class. A suppressed class with a
//              non-zero count is a visible, dated fact.
//   STOP       at session end, compare writes that TOUCHED governed globs
//              against hook fires. Writes matched, zero fires => the chain is
//              suppressed, and the report says so by name.
//
// WHY IT IS APPEND-ONLY JSONL
//
// A hook that rewrites a state file can lose the record it just wrote when two
// writes race. Appending a line is atomic enough for this purpose under
// O_APPEND, and the file is readable by `doctor` without a lock.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Record is one hook invocation, journaled.
type Record struct {
	TS        string `json:"ts"`
	Kind      string `json:"kind"` // "hook_match"
	Shim      string `json:"shim"` // "pre" | "post" | "probe"
	Outcome   string `json:"outcome"`
	Detail    string `json:"detail,omitempty"`
	Path      string `json:"path,omitempty"`
	Matched   int    `json:"matched"`
	SessionID string `json:"session_id,omitempty"`

	// ── ONE RECORD SHAPE, THREE CONSUMERS ────────────────────────────────
	//
	// Attribution, staleness, and the outcome view are three readings of the
	// same event, so the field set is designed once rather than three times.
	//
	// Actor answers WHO WAS GOVERNED. Without it a subagent bounce is
	// indistinguishable from a parent one, and in this repo a single corpus
	// pass is ~70 subagent writes to governed paths — the majority of governed
	// activity would be unattributable. "unknown" is a real value and is never
	// guessed at: a resolver that returned "parent" when it could not tell
	// would make every subagent look like the session that spawned it.
	Actor           string `json:"actor,omitempty"`      // parent | subagent | unknown
	AgentType       string `json:"agent_type,omitempty"` // subagent slug when resolvable
	ParentSessionID string `json:"parent_session_id,omitempty"`
	ContextID       string `json:"context_id,omitempty"`

	// DirectiveIDs is what makes the staleness and outcome views possible at
	// all. A count answers "how much fired"; only the ids answer "you keep
	// fighting THIS rule" and "this rule has never fired" — and the second is
	// the higher-yield finding, because silence annoys nobody into noticing.
	DirectiveIDs []string `json:"directive_ids,omitempty"`

	// Bounced separates the two outcomes that matter and look identical in a
	// count: PREVENTED SOMETHING versus DELIVERED WITHOUT FRICTION. A match
	// suppressed by dedup is the latter — the agent was already told.
	Bounced         bool `json:"bounced"`
	DedupSuppressed bool `json:"dedup_suppressed,omitempty"`

	// BudgetExhausted marks a THIRD outcome, distinct from both: the
	// receiving context had not been told before (so it is not
	// DedupSuppressed), but the session's aggregate BounceBudget for this
	// exact directive set was already spent, so the write proceeded as
	// advisory instead of a deny (so it is not Bounced either).
	BudgetExhausted bool `json:"budget_exhausted,omitempty"`

	// ── PER-HOOK INVOCATION (SPEC-014 FR-005/FR-006) ─────────────────────
	//
	// Before these existed, this journal carried only `pre`, `post` and
	// `probe` — three shims out of twenty-two hooks — so "did session-start
	// fire?" had no answer anywhere on the machine. Phases 15 and 19 gate the
	// migration on per-hook evidence, which means every registered hook writes
	// one record per run and the record has to say WHICH hook, HOW it ended,
	// and HOW LONG it took.
	//
	// Hook is the registry name (templates/hooks/registry.yaml `name`).
	Hook string `json:"hook,omitempty"`

	// ExitCode and DurationUS are POINTERS, and that is the whole point.
	// A hook that exited 0 and a record from a non-hook-run source both
	// decode to 0 in a plain int, which is INV-013's decoder failure exactly:
	// the most common real value is also the absent value. A pointer keeps
	// "exited 0" distinguishable from "this record never carried an exit
	// code".
	ExitCode *int `json:"exit_code,omitempty"`

	// Microseconds, not milliseconds: instructions-loaded runs in ~2.5ms
	// today and the trivial handlers will run faster still, so a millisecond
	// field would report most of the tier as "0" — a measurement that reads
	// as an absence.
	DurationUS *int64 `json:"duration_us,omitempty"`
}

// JournalPath is the per-user hook journal.
func JournalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".edikt", "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "hook-journal.jsonl"), nil
}

// KindHookRun marks a record written by `bin/edikt hook run <name>`'s
// dispatcher, as opposed to the write-time matcher's own records.
const KindHookRun = "hook_run"

// ── ROTATION AND WINDOWING ──────────────────────────────────────────────────
//
// This journal was written when exactly two call sites appended to it. After
// the hook migration there are twenty-two, and the arithmetic changes: a
// working day of a busy session goes from hundreds of records to thousands.
// ReadSummary used to os.ReadFile the whole thing and parse every line, which
// is a cost `doctor` and every `hook report` pays in full, forever, over
// history nobody is asking about.
//
// TWO MECHANISMS, BECAUSE THEY SOLVE DIFFERENT HALVES.
//
//	ROTATION bounds what is on disk. Past MaxJournalBytes the file is renamed
//	to <name>.1 and a fresh one starts. Exactly ONE generation is kept: the
//	point is to stop unbounded growth, not to build an archive nothing reads.
//
//	WINDOWING bounds what is PARSED. A reader seeks to the tail and parses the
//	last WindowBytes only. Rotation alone would not give this — a 4 MB file
//	just under the rotation threshold is still 4 MB to parse.
//
// A windowed read is a partial observation, so it says so. Summary carries
// Windowed and the byte counts, and Report names them: a count over the tail
// of a journal MUST NOT read as a count over all of it (INV-013).
const (
	// MaxJournalBytes is the rotation threshold.
	MaxJournalBytes int64 = 4 << 20 // 4 MiB

	// DefaultWindowBytes is how much of the tail an ordinary read parses.
	DefaultWindowBytes int64 = 512 << 10 // 512 KiB
)

// Append writes one record.
//
// Its error is returned but callers in the hook path deliberately ignore it:
// a journal that cannot be written must not break the write it is observing.
// That is a real gap and it is named rather than hidden — an unwritable
// journal is itself undetectable, which is why `doctor` also reports journal
// STALENESS and not only its contents.
func Append(r Record) error {
	if r.TS == "" {
		r.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if r.Kind == "" {
		r.Kind = "hook_match"
	}
	p, err := JournalPath()
	if err != nil {
		return err
	}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	// Rotate AFTER the write, from the same handle's own view of the file, so
	// this record is never the one lost to the rename. Every rotation failure
	// is swallowed: a journal that cannot be rotated is still a journal, and
	// breaking the write being observed to tidy up would invert the priority
	// this file is built around.
	if st, serr := f.Stat(); serr == nil && st.Size() > MaxJournalBytes {
		f.Close()
		_ = os.Rename(p, p+".1")
		return nil
	}
	return f.Close()
}

// Summary is the detectability report.
type Summary struct {
	Total        int            `json:"total"`
	ByOutcome    map[string]int `json:"by_outcome"`
	Suppressed   int            `json:"suppressed"`
	LastTS       string         `json:"last_ts,omitempty"`
	JournalFound bool           `json:"journal_found"`

	// BudgetExhausted counts invocations where governance DID run and DID
	// deliver, just not as a deny — the session's BounceBudget for that exact
	// directive set was already spent. Counted separately from Suppressed:
	// folding it in would misreport "governance did not run" for an
	// invocation where it did.
	BudgetExhausted int `json:"budget_exhausted"`

	// ByHook counts per-hook invocation records. Empty for a journal written
	// entirely by the write-time matcher, which is the honest answer before
	// any hook has migrated — not a claim that no hook ran.
	ByHook map[string]int `json:"by_hook,omitempty"`

	// ── WHAT THIS SUMMARY ACTUALLY LOOKED AT ─────────────────────────────
	//
	// Windowed says the counts cover only the tail. BytesScanned and
	// BytesTotal give the denominator. Malformed counts lines that would not
	// decode — previously skipped in silence, which meant a corrupted journal
	// and a quiet one produced the same report.
	Windowed     bool  `json:"windowed"`
	BytesScanned int64 `json:"bytes_scanned"`
	BytesTotal   int64 `json:"bytes_total"`
	Malformed    int   `json:"malformed"`
}

// ReadSummary aggregates the journal.
//
// A MISSING journal is reported as JournalFound=false with zero counts, and
// the caller must not render that as "zero suppression". No journal means the
// hook chain has never run on this machine — which is either a fresh install
// or a chain that is not wired up at all, and those are the two cases this
// whole mechanism exists to tell apart.
func ReadSummary() (Summary, error) { return ReadSummaryWindow(DefaultWindowBytes) }

// ReadSummaryWindow aggregates at most the last window bytes of the journal.
//
// A window <= 0 means "parse everything" and is what a test or an explicit
// full-history audit asks for. Anything else seeks to size-window and DISCARDS
// the first partial line — a JSONL tail that begins mid-record would otherwise
// contribute one guaranteed malformed line to every windowed read, and a
// permanent off-by-one in a corruption counter is worse than no counter.
func ReadSummaryWindow(window int64) (Summary, error) {
	s := Summary{ByOutcome: map[string]int{}, ByHook: map[string]int{}}
	p, err := JournalPath()
	if err != nil {
		return s, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return s, err
	}
	s.JournalFound = true
	s.BytesTotal = st.Size()

	if window > 0 && st.Size() > window {
		if _, err := f.Seek(st.Size()-window, io.SeekStart); err != nil {
			return s, err
		}
		s.Windowed = true
	}

	sc := bufio.NewScanner(f)
	// A single journal line carries directive ids and detail text; the default
	// 64 KiB token limit is generous but not unbounded, and a line that
	// exceeded it would end the scan EARLY AND SILENTLY — every later record
	// vanishing from the count with nothing reporting it.
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)

	first := true
	for sc.Scan() {
		lineBytes := sc.Bytes()
		s.BytesScanned += int64(len(lineBytes)) + 1
		if first && s.Windowed {
			// The partial record the seek landed inside. Discarded, not
			// counted as malformed: it is an artifact of windowing, not of
			// corruption.
			first = false
			continue
		}
		first = false
		line := strings.TrimSpace(string(lineBytes))
		if line == "" {
			continue
		}
		var r Record
		if json.Unmarshal([]byte(line), &r) != nil {
			s.Malformed++
			continue
		}
		s.Total++
		s.ByOutcome[r.Outcome]++
		if r.Hook != "" {
			s.ByHook[r.Hook]++
		}
		if Outcome(r.Outcome).Suppressed() {
			s.Suppressed++
		}
		if r.BudgetExhausted {
			s.BudgetExhausted++
		}
		if r.TS > s.LastTS {
			s.LastTS = r.TS
		}
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	return s, nil
}

// Report renders the summary for `doctor`.
//
// It always names the denominator, and it never renders an absent journal as a
// clean one.
func (s Summary) Report() string {
	if !s.JournalFound {
		return "hook chain: NO JOURNAL — the injection hooks have never run on this machine.\n" +
			"  This is not the same as 'no directives fired'. It means the chain is unwired,\n" +
			"  never invoked, or unable to write its journal. Run `bin/edikt hook probe`."
	}
	if s.Total == 0 {
		return "hook chain: journal present but EMPTY — no recorded invocations.\n" +
			"  Run `bin/edikt hook probe` to confirm the chain is live."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "hook chain: %d invocation(s) recorded, last %s\n", s.Total, s.LastTS)
	if s.Windowed {
		fmt.Fprintf(&b, "  WINDOWED: counts cover the most recent %d of %d journal byte(s) — earlier\n",
			s.BytesScanned, s.BytesTotal)
		b.WriteString("  history was NOT parsed, so these are tail counts, not totals.\n")
	}
	if s.Malformed > 0 {
		fmt.Fprintf(&b, "  MALFORMED: %d line(s) in the scanned range could not be decoded and are\n", s.Malformed)
		b.WriteString("  counted nowhere else — a corrupt journal must not read as a quiet one.\n")
	}
	keys := make([]string, 0, len(s.ByOutcome))
	for k := range s.ByOutcome {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		mark := " "
		if Outcome(k).Suppressed() {
			mark = "!"
		}
		fmt.Fprintf(&b, "  %s %-14s %d\n", mark, k, s.ByOutcome[k])
	}
	if len(s.ByHook) > 0 {
		hooks := make([]string, 0, len(s.ByHook))
		for h := range s.ByHook {
			hooks = append(hooks, h)
		}
		sort.Strings(hooks)
		fmt.Fprintf(&b, "  per-hook invocations (%d hook(s) seen in the scanned range):\n", len(hooks))
		for _, h := range hooks {
			fmt.Fprintf(&b, "    %-24s %d\n", h, s.ByHook[h])
		}
	}
	if s.BudgetExhausted > 0 {
		fmt.Fprintf(&b, "  BOUNCE BUDGET SPENT: %d invocation(s) delivered as advisory instead of a deny —\n", s.BudgetExhausted)
		b.WriteString("  governance ran and told the agent, but this session's hooks.injection.bounce_budget\n")
		b.WriteString("  for that directive set was already spent. Not a suppression: the agent was informed.\n")
	}
	if s.Suppressed > 0 {
		fmt.Fprintf(&b, "  SUPPRESSED: %d invocation(s) where governance DID NOT RUN.\n", s.Suppressed)
		b.WriteString("  These are indistinguishable from a clean no-match at the hook's output,\n")
		b.WriteString("  which is why they are counted here. Each one is a write that proceeded\n")
		b.WriteString("  ungoverned.\n")
	} else {
		b.WriteString("  no suppressed invocations recorded\n")
	}
	return b.String()
}
