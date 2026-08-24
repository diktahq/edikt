// Package hooksoak implements the SPEC-014 / PLAN-v080-reliability-core
// Phase 15 (group 1) and Phase 19 (group 2) dogfood soak gate: proof that a
// migrated hook GROUP actually ran, over a real window, before the next
// migration group is allowed to start.
//
// # WHAT THIS GATE IS FOR, AND WHAT IT DELIBERATELY DOES NOT PROVE
//
// "Compiles" and "unit tests pass" are already true the moment a hook's Go
// handler lands — they say nothing about whether that handler ever fired in
// real, day-to-day use. This package reads the same journal
// internal/hookmatch/journal.go already writes (Phase 6/12-14's per-hook
// `hook_run` records) and asks one narrow question per hook in the named
// group: did this hook run at all, cleanly, in the window this record
// covers? It does not measure code quality, coverage, or correctness — the
// corpus gate (internal/hookcorpus) and the Go test suite already do that.
// This gate measures USE.
//
// # THE GATE LOGIC IS FIXTURE-TESTABLE ON DAY ZERO
//
// PLAN-v080-reliability-core Phase 15 is explicit: only the gate's PASSING
// waits on wall-clock (14 real days of dogfood use, SPEC-014 spec.md
// "Dogfood soak"). The gate's REFUSAL logic — zero invocations, a
// disqualifying outcome, an absent journal — must be provable against
// synthetic fixtures today, which is why every input this package reads
// (the registry path, the journal path, the git commit lookup, and the
// clock) is an explicit parameter rather than a hardcoded resolution.
//
// INV-013 GOVERNS THIS PACKAGE'S CENTRAL DESIGN DECISION
//
// A missing journal and a journal that recorded zero invocations are
// DIFFERENT FACTS, and this package keeps them different all the way to the
// wire: `journal_status` is "unknown" for the first, "observed" for the
// second, and an "unknown" hook's invocation_count is explicitly a
// zero-valued PLACEHOLDER, never evidence (hook-soak.v1's own field
// description). Collapsing the two would let a hook chain that has never
// run once on a machine report the exact same shape as a hook chain that
// ran and recorded nothing — the one failure mode INV-013 exists to name.
package hooksoak

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook"
)

// SchemaVersion is the hook-soak.v1 contract version this package emits.
const SchemaVersion = 1

// JournalStatus values — hook-soak.v1 `hooks[].journal_status`.
const (
	// JournalObserved means the journal window was read successfully and the
	// counts on this entry reflect what it actually contains, including a
	// genuine zero.
	JournalObserved = "observed"
	// JournalUnknown means the journal could not be read for this hook in
	// this window (absent file, or a file present but unreadable). The
	// counts on this entry are unset placeholders, never evidence.
	JournalUnknown = "unknown"
)

// Status values — hook-soak.v1 `status`.
const (
	StatusPass    = "pass"
	StatusRefused = "refused"
)

// HookEntry is one hook's soak evidence — hook-soak.v1 `hooks[]`.
type HookEntry struct {
	Name                      string `json:"name"`
	JournalStatus             string `json:"journal_status"`
	InvocationCount           int    `json:"invocation_count"`
	PanicRecoveredCount       int    `json:"panic_recovered_count"`
	UnregisteredHookNameCount int    `json:"unregistered_hook_name_count"`
	BinaryAbsentCount         int    `json:"binary_absent_count"`
}

// Record is the full hook-soak.v1 document, written at
// .edikt/state/hook-soak/group-<n>.soak.json by the `hook soak` CLI layer.
// This package only builds the value; writing it to disk is the caller's
// job (deliberately — a pure Check() is what makes every criterion below
// testable without a filesystem round trip beyond the journal itself).
type Record struct {
	SchemaVersion  int         `json:"schema_version"`
	Group          int         `json:"group"`
	SinceCommit    string      `json:"since_commit"`
	ElapsedDays    int         `json:"elapsed_days"`
	Hooks          []HookEntry `json:"hooks"`
	Status         string      `json:"status"`
	RefusalReasons []string    `json:"refusal_reasons,omitempty"`
}

// GroupClass maps a soak group number to the templates/hooks/registry.yaml
// `class` it covers. Group 3 (the gate class — pre-tool-use, verify-gate,
// inject-directives-pre) has no soak gate of its own: FR-007's migration
// order runs group 3 immediately after group 2's soak, on convention
// maturity rather than a third wall-clock window (SPEC-014 spec.md
// "FR-007 — migration order"; hook-soak.v1 `group` field description).
func GroupClass(group int) (string, error) {
	switch group {
	case 1:
		return hook.ClassInjector, nil
	case 2:
		return hook.ClassMultiCall, nil
	default:
		return "", fmt.Errorf("hook soak: group must be 1 or 2 (got %d) — group 3 (the gate class) has no soak gate of its own", group)
	}
}

// GroupHooks derives the set of hook names a soak group covers DIRECTLY from
// a loaded registry file — never a hand-kept list in this package or its
// caller. This mirrors the established pattern in
// test/test-hook-corpus-replay.sh and test/check-corpus-recording-parity.sh:
// both read templates/hooks/registry.yaml's `status: migrated` rows rather
// than naming hooks in the shell script, specifically so a hook flipped to
// migrated later is covered automatically. This package reuses the same
// discipline through the Go loader (internal/hook.LoadRegistryFile) instead
// of re-parsing YAML a second way.
//
// A row counts toward a group only when BOTH its class matches the group
// AND its status is "migrated" — status alone is not enough once group 2
// exists too, since by Phase 19 both the injector and multi-call rows will
// read "migrated" and class is what tells them apart.
//
// Returns hook names sorted for deterministic output. Errors when the group
// has zero migrated hooks: per hook-soak.v1's own description, a group
// never reaches a soak check before every hook in it has landed, so an empty
// result here is a caller of this gate too early, not a valid empty record
// (the schema itself requires `hooks` minItems: 1).
func GroupHooks(f hook.File, group int) ([]string, error) {
	class, err := GroupClass(group)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, r := range f.Hooks {
		if r.Class == class && r.Status == hook.StatusMigrated {
			names = append(names, r.Name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf(
			"hook soak: registry %s has zero hooks with class %q and status %q (group %d) — this group has not migrated yet, so there is nothing to soak-check",
			f.Path, class, hook.StatusMigrated, group)
	}
	return names, nil
}

// CheckOptions configures one soak-gate evaluation. Every field with a
// caller-visible default names it explicitly in its own doc comment — this
// struct is the gate's entire input surface, and PLAN-v080-reliability-core
// Phase 15 requires that surface to be fully substitutable by a test.
type CheckOptions struct {
	// RegistryPath is a resolved templates/hooks/registry.yaml (or an
	// equivalent override path, per internal/hook.ResolveRegistryPath's
	// precedence). Required.
	RegistryPath string

	// Group selects which FR-007 migration group to check: 1 or 2.
	Group int

	// SinceCommit is the git commit hash this group's soak window counts
	// forward from (hook-soak.v1 `since_commit`) — see CheckOptions.GitLog
	// for how its date is resolved.
	SinceCommit string

	// RepoRoot is where the git lookup runs. Must be inside (or be) the
	// repository SinceCommit belongs to.
	RepoRoot string

	// JournalPath is the hook journal to read. Required — this package
	// never resolves hookmatch.JournalPath() itself; the CLI layer
	// (tools/edikt/cmd/hook_soak.go) is the one place that decides between
	// the real per-user journal and a caller-supplied override, so that
	// decision has exactly one owner instead of being duplicated here.
	JournalPath string

	// Now returns the current time. Defaults to time.Now. Overridable so
	// ElapsedDays is deterministic in a test.
	Now func() time.Time

	// GitLog resolves a commit hash to its commit date. Defaults to
	// GitCommitDate (a real `git log -1 --format=%cI` invocation against
	// RepoRoot). Overridable so the zero-input and disqualifying-outcome
	// criteria can be exercised without depending on a real repository
	// containing SinceCommit — only the "elapsed days come from git, not a
	// state file" criterion needs the real implementation, and that is
	// proven by a dedicated test against this package's own repository.
	GitLog func(repoRoot, commit string) (time.Time, error)

	// ReadJournal reads the hook journal. Defaults to ReadJournal (this
	// package's own reader). Overridable for a test that wants to force a
	// read error distinct from "file absent" without constructing an
	// actually-unreadable file on disk.
	ReadJournal func(path string) (map[string]JournalCounts, bool, error)
}

// Check runs the full soak gate and returns the hook-soak.v1 record.
//
// It returns an error only for a SETUP problem — an unloadable registry, a
// group with nothing migrated yet, or a since-commit git could not resolve
// — none of which produce a valid hook-soak.v1 document (the schema
// requires `hooks` minItems: 1 and every hook entry's fields to be real).
// A REFUSAL (zero invocations, a disqualifying outcome, an absent journal)
// is not an error: it is a complete, valid Record with status "refused" and
// named reasons — the gate did its job and reported what it found.
func Check(o CheckOptions) (Record, error) {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	gitLog := o.GitLog
	if gitLog == nil {
		gitLog = GitCommitDate
	}
	readJournal := o.ReadJournal
	if readJournal == nil {
		readJournal = ReadJournal
	}

	regFile, err := hook.LoadRegistryFile(o.RegistryPath)
	if err != nil {
		return Record{}, err
	}
	names, err := GroupHooks(regFile, o.Group)
	if err != nil {
		return Record{}, err
	}

	commitDate, err := gitLog(o.RepoRoot, o.SinceCommit)
	if err != nil {
		return Record{}, fmt.Errorf("hook soak: resolve --since-commit %s: %w", o.SinceCommit, err)
	}
	elapsed := ElapsedDays(commitDate, now())

	counts, found, jerr := readJournal(o.JournalPath)
	// A read error collapses into "not found" for THIS gate's purposes: an
	// unreadable journal and an absent one are the same fact from a hook's
	// perspective — governance evidence that cannot be produced — and
	// ReadJournal's own contract already documents this. jerr is kept only
	// to enrich the refusal reason text below, never to change control
	// flow: forcing found=false here (rather than trusting the callee)
	// means a future ReadJournal override that gets this wrong cannot
	// silently turn "unreadable" into "clean".
	if jerr != nil {
		found = false
	}

	rec := Record{
		SchemaVersion: SchemaVersion,
		Group:         o.Group,
		SinceCommit:   o.SinceCommit,
		ElapsedDays:   elapsed,
		Hooks:         make([]HookEntry, 0, len(names)),
	}

	var reasons []string
	for _, name := range names {
		entry := HookEntry{Name: name}
		if !found {
			entry.JournalStatus = JournalUnknown
			detail := o.JournalPath
			if jerr != nil {
				detail = fmt.Sprintf("%s (%v)", o.JournalPath, jerr)
			}
			reasons = append(reasons, fmt.Sprintf(
				"%s: journal_status unknown — journal window unreadable at %s", name, detail))
			rec.Hooks = append(rec.Hooks, entry)
			continue
		}

		entry.JournalStatus = JournalObserved
		c := counts[name]
		entry.InvocationCount = c.Invocation
		entry.PanicRecoveredCount = c.PanicRecovered
		entry.UnregisteredHookNameCount = c.Unregistered
		entry.BinaryAbsentCount = c.BinaryAbsent
		rec.Hooks = append(rec.Hooks, entry)

		if c.Invocation == 0 {
			reasons = append(reasons, fmt.Sprintf(
				"%s: zero journal invocation records in window — governance evidence this hook ran is absent", name))
		}
		if c.PanicRecovered > 0 {
			reasons = append(reasons, fmt.Sprintf(
				"%s: %d panic-recovered outcome(s) in window", name, c.PanicRecovered))
		}
		if c.Unregistered > 0 {
			reasons = append(reasons, fmt.Sprintf(
				"%s: %d unregistered-hook-name outcome(s) in window", name, c.Unregistered))
		}
		if c.BinaryAbsent > 0 {
			reasons = append(reasons, fmt.Sprintf(
				"%s: %d binary-absent outcome(s) in window", name, c.BinaryAbsent))
		}
	}

	if len(reasons) > 0 {
		rec.Status = StatusRefused
		rec.RefusalReasons = reasons
	} else {
		rec.Status = StatusPass
	}
	return rec, nil
}

// Report renders a human-readable summary — the CLI's non-JSON output.
func (r Record) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "hook soak — group %d, since %s, %d day(s) elapsed\n", r.Group, r.SinceCommit, r.ElapsedDays)
	for _, h := range r.Hooks {
		if h.JournalStatus == JournalUnknown {
			fmt.Fprintf(&b, "  ? %-28s journal_status=unknown (not zero — unreadable)\n", h.Name)
			continue
		}
		mark := " "
		if h.InvocationCount == 0 || h.PanicRecoveredCount > 0 || h.UnregisteredHookNameCount > 0 || h.BinaryAbsentCount > 0 {
			mark = "!"
		}
		fmt.Fprintf(&b, "  %s %-28s invocations=%-4d panic=%-2d unregistered=%-2d binary-absent=%-2d\n",
			mark, h.Name, h.InvocationCount, h.PanicRecoveredCount, h.UnregisteredHookNameCount, h.BinaryAbsentCount)
	}
	fmt.Fprintf(&b, "status: %s\n", strings.ToUpper(r.Status))
	for _, reason := range r.RefusalReasons {
		fmt.Fprintf(&b, "  - %s\n", reason)
	}
	return b.String()
}
