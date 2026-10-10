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
	"compress/gzip"
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

	// PromptID and AgentTranscriptPath make a hook_run record SELF-SUFFICIENT
	// for an exporter: the first joins this record to the user turn that
	// produced it, the second points at the per-turn stream it summarises.
	//
	// Added 2026-08-30 because the subagent hooks were being handed identity
	// and discarding it. Measured before the change: all 1,703 subagent-stop
	// and 22 subagent-start records were kind=hook_run carrying only hook,
	// exit_code and duration_us — no agent_type, no agent_id, nothing that
	// could say WHICH skill produced them. The host sends agent_type on both
	// events (observed: "general-purpose"); edikt simply never wrote it down.
	//
	// This is NOT the same defect as the constant "-" seen on 491 records:
	// those come from the injection path on PreToolUse, where the host sends
	// no agent identity at all and "-" is the honest fallback.
	PromptID            string `json:"prompt_id,omitempty"`
	AgentTranscriptPath string `json:"agent_transcript_path,omitempty"`

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

// ediktHome resolves the user-global edikt home: $EDIKT_ROOT, then
// $EDIKT_HOME, then $HOME/.edikt — the same three-rung priority the hook
// shim's own bash ladder uses to resolve the edikt binary itself
// (${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/bin/edikt, see
// internal/hook/shim/shim.go). Every user-global state path in this package
// MUST resolve through this, not os.UserHomeDir() directly.
//
// Found 2026-08-27: a benchmark sandbox that redirects EDIKT_ROOT to isolate
// the BINARY it runs, without also redirecting HOME (HOME redirection
// itself breaks host keychain auth on macOS — the CLI subprocess cannot
// authenticate once $HOME/Library/Keychains/login.keychain-db leaves the
// search path), needs its STATE to follow EDIKT_ROOT too, or every hook
// journal/dedup write silently lands in the real machine's ~/.edikt/state/
// while the sandbox believes it measured its own isolated journal — a
// verification mechanism (BRAIN-017 §8's hook_events check) reporting a
// false UNMEASURED for a hook that actually fired, or worse, reading nothing
// and never noticing the miss at all.
func ediktHome() (string, error) {
	// A RELATIVE root is refused, not accepted and joined.
	//
	// Traced 2026-08-27, reproduced directly: with EDIKT_ROOT=".",
	// JournalPath's filepath.Join(home, "state") yields the relative path
	// "state", and MkdirAll then creates it in whatever the process's cwd
	// happens to be. A hook firing with cwd at a repo root writes a stray
	// state/hook-journal.jsonl there — untracked, un-ignored, and reachable
	// by a `git add` sweep. For an adopter that means edikt littering a
	// directory into their repository root.
	//
	// The three-rung ladder below is otherwise sound: an unset HOME makes
	// os.UserHomeDir() error, which propagates. The defect was never an empty
	// `home` — it was a NON-EMPTY relative one, which every rung's `!= ""`
	// check accepts. Emptiness was the wrong property to test for.
	//
	// INV-014: assert the published outcome, never let a resolution failure
	// look like a resolution. A path that "works" and lands in the wrong place
	// is worse than an error, because nothing reports it.
	absOrErr := func(v, source string) (string, error) {
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("%s must be an absolute path, got %q — "+
				"a relative root resolves against the process's cwd and writes "+
				"state into whatever directory a hook happened to fire in", source, v)
		}
		return v, nil
	}
	if root := os.Getenv("EDIKT_ROOT"); root != "" {
		return absOrErr(root, "EDIKT_ROOT")
	}
	if home := os.Getenv("EDIKT_HOME"); home != "" {
		return absOrErr(home, "EDIKT_HOME")
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// Defence in depth: os.UserHomeDir() reads $HOME, which a caller can set
	// to a relative value without tripping Go's own emptiness check.
	return absOrErr(filepath.Join(userHome, ".edikt"), "$HOME")
}

// JournalPath is the per-user hook journal.
func JournalPath() (string, error) {
	home, err := ediktHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "state")
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
//	ROTATION bounds what the LIVE file costs. Past MaxJournalBytes it is moved
//	into journal-archive/ under a UNIQUE, timestamped name and a fresh one
//	starts.
//
//	IT USED TO RENAME TO <name>.1, AND THAT DESTROYED EVIDENCE. A fixed
//	suffix means every rotation overwrites the previous generation, so the
//	entire retention policy was two files: on 2026-08-29 that was 21 hours,
//	and figures measured that morning and reproduced twice could not be
//	re-derived the same evening. The original comment here said the point was
//	"to stop unbounded growth, not to build an archive nothing reads", and
//	that was true when two call sites appended. There are now twenty-two
//	Record construction sites, and this journal is the only real-usage
//	evidence edikt has about its own delivery behaviour. Losing a generation
//	is losing the measurement.
//
//	NOTHING EVER READ THE ROTATED FILE — the only references to <name>.1 were
//	the rename itself and an unrelated rotation in migrate.go. So rotation was
//	pure deletion. Readers still read ONLY the live file and still window its
//	tail; nothing about read cost changes here.
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

	// MaxArchiveBytes caps the TOTAL on-disk size of rotated generations.
	//
	// CHOSEN OVER THE TWO ALTERNATIVES, deliberately. Compress-on-rotate
	// alone does not bound anything — it only makes unbounded growth grow
	// more slowly, and shipping unbounded growth onto a user's disk is not
	// acceptable. Prune-by-age does not bound it either: retention in DAYS
	// says nothing about retention in BYTES, so a heavy week can still fill a
	// disk. A byte cap is the only one of the three that gives a hard
	// guarantee, and it matches the posture MaxJournalBytes already sets.
	//
	// Compression is applied as well, because it buys history for free inside
	// a fixed cap: JSONL of this shape compresses roughly 10:1, so 32 MiB of
	// archive holds on the order of 80 rotations — months of ordinary use,
	// where the old policy held one generation.
	//
	// Worst case on disk goes from 8 MiB (live 4 + one 4 MiB generation) to
	// 36 MiB (live 4 + 32 MiB archive). That is a real increase and it is
	// stated rather than buried.
	MaxArchiveBytes int64 = 32 << 20 // 32 MiB
)

// ArchiveDirName is the subdirectory of the edikt state dir holding rotated
// generations. Readers do not read it; it exists so evidence survives.
const ArchiveDirName = "journal-archive"

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
		rotate(p)
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

// SessionScope is ReadSessionScope's result (SR-005): what one session
// touched, for the Stop hook's end-of-session tally line.
type SessionScope struct {
	SessionID    string
	Paths        []string // distinct, non-empty, sorted
	JournalFound bool

	// MayUndercount is true when the journal's own OVERALL first record
	// already belongs to this session — meaning any history the session
	// produced before that record, if it exists, was rotated or truncated
	// away before this read. The counts derived from Paths are then a lower
	// bound, not a maximum, and the caller must say so rather than imply
	// completeness (INV-013).
	MayUndercount bool
}

// ReadSessionScope reads the WHOLE live journal — never windowed — and
// returns the distinct file paths touched by records carrying sessionID.
//
// Unlike ReadSummaryWindow, this is NOT tail-windowed: a session that
// interleaves with others (subagents, concurrent sessions on the same
// machine) can have its early records pushed outside a 512 KiB tail long
// before the session itself is stale, which would silently undercount rather
// than report a smaller, honest number. A 4 MiB journal parses in
// single-digit milliseconds (TestReadSessionScope_LargeJournalIsFast), so
// there is no latency reason to window this read the way ReadSummaryWindow
// windows the doctor-facing aggregate.
func ReadSessionScope(sessionID string) (SessionScope, error) {
	scope := SessionScope{SessionID: sessionID}
	p, err := JournalPath()
	if err != nil {
		return scope, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return scope, nil
		}
		return scope, err
	}
	defer f.Close()
	scope.JournalFound = true

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)

	seenPaths := map[string]bool{}
	sawFirstRecord := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		if !sawFirstRecord {
			sawFirstRecord = true
			if r.SessionID == sessionID {
				// The file's own first surviving record already belongs to
				// this session — there is no way to tell, from this journal
				// alone, whether earlier records for it ever existed and
				// were rotated away.
				scope.MayUndercount = true
			}
		}
		if r.SessionID != sessionID {
			continue
		}
		if r.Path != "" {
			seenPaths[r.Path] = true
		}
	}
	if err := sc.Err(); err != nil {
		return scope, err
	}

	paths := make([]string, 0, len(seenPaths))
	for p := range seenPaths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	scope.Paths = paths
	return scope, nil
}

// HookDurations returns every recorded DurationUS for kind_hook_run records
// of the given hook name, oldest first — the raw material for a p50/max
// claim that is MEASURED rather than guessed (AC-4.4). A record with no
// DurationUS (the pointer is nil — an older binary, or a non-hook_run
// record) is skipped, never treated as a recorded zero.
func HookDurations(hookName string) ([]time.Duration, error) {
	p, err := JournalPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)

	var out []time.Duration
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		if r.Hook != hookName || r.DurationUS == nil {
			continue
		}
		out = append(out, time.Duration(*r.DurationUS)*time.Microsecond)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DurationPercentile returns the value at the given percentile (0-100) of a
// SORTED durations slice using nearest-rank. Callers sort once and may query
// several percentiles without re-sorting. An empty slice returns 0 — the
// caller must check len() first rather than reading 0 as "measured zero".
func DurationPercentile(sorted []time.Duration, pct int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (pct*len(sorted) + 99) / 100 // nearest-rank, 1-indexed
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return sorted[idx-1]
}

// Report renders the summary for `doctor`.
//
// It always names the denominator, and it never renders an absent journal as a
// clean one.
func (s Summary) Report() string {
	if !s.JournalFound {
		return "hook chain: NO JOURNAL — the injection hooks have never run on this machine.\n" +
			"  This is not the same as 'no directives fired'. It means the chain is unwired,\n" +
			"  never invoked, or unable to write its journal. Run `edikt hook probe`."
	}
	if s.Total == 0 {
		return "hook chain: journal present but EMPTY — no recorded invocations.\n" +
			"  Run `edikt hook probe` to confirm the chain is live."
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

// rotate moves the live journal into the archive under a unique name, then
// compresses it and prunes the archive back under MaxArchiveBytes.
//
// ORDER MATTERS AND IS THE POINT. The rename happens FIRST and is atomic, so
// the generation is preserved the instant it leaves the live path. Compression
// and pruning are best-effort afterwards: if gzip fails the archive keeps a
// plain .jsonl and nothing is lost, and if pruning fails the cap is exceeded
// until the next rotation rather than a generation being dropped to enforce it.
// Every failure is swallowed for the same reason the caller swallows its own —
// a journal that cannot be rotated is still a journal, and breaking the write
// being observed in order to tidy up would invert this file's priority.
func rotate(p string) {
	dir := filepath.Join(filepath.Dir(p), ArchiveDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	// Two rotations inside the same second must not collide — a collision
	// silently overwrites, which is the whole defect being fixed here.
	//
	// The candidate must be free under BOTH extensions. Checking only .jsonl
	// is not enough and was wrong in the first draft of this function: a
	// rotated generation ends up as .jsonl.gz and its .jsonl is removed, so
	// the plain name always looks free and the NEXT rotation truncates the
	// existing .gz. Caught by TestRotate_NeverOverwritesAPreviousGeneration.
	ts := time.Now().UTC().Format("20060102T150405Z")
	base := filepath.Join(dir, "hook-journal-"+ts)
	dst := base + ".jsonl"
	for i := 1; taken(dst); i++ {
		dst = fmt.Sprintf("%s-%d.jsonl", base, i)
	}
	if err := os.Rename(p, dst); err != nil {
		return
	}
	if compressFile(dst) == nil {
		_ = os.Remove(dst)
	}
	pruneArchive(dir)
}

// compressFile writes <src>.gz. The source is left alone; the caller removes
// it only on success, so a failed compression degrades to a plain archive
// rather than to a lost one.
func compressFile(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(src+".gz", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		zw.Close()
		out.Close()
		_ = os.Remove(src + ".gz")
		return err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		_ = os.Remove(src + ".gz")
		return err
	}
	return out.Close()
}

// pruneArchive deletes OLDEST-FIRST until the archive fits MaxArchiveBytes.
//
// Names are timestamp-prefixed and fixed-width, so lexical order is
// chronological order and no stat-based sort is needed. The newest generation
// is never deleted even if it alone exceeds the cap: a cap that can empty the
// archive would reintroduce exactly the data loss this change removes.
func pruneArchive(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "hook-journal-") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // oldest first
	var total int64
	sizes := make(map[string]int64, len(names))
	for _, n := range names {
		if fi, err := os.Stat(filepath.Join(dir, n)); err == nil {
			sizes[n] = fi.Size()
			total += fi.Size()
		}
	}
	for i := 0; i < len(names)-1 && total > MaxArchiveBytes; i++ {
		if err := os.Remove(filepath.Join(dir, names[i])); err == nil {
			total -= sizes[names[i]]
		}
	}
}

// taken reports whether a rotation candidate is already used, in either the
// plain or the compressed form.
func taken(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	_, err := os.Stat(path + ".gz")
	return err == nil
}
