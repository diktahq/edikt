package hooksoak

// journal.go — reads the hookmatch hook journal at an EXPLICIT, overridable
// path, rather than through hookmatch.JournalPath()'s fixed
// ~/.edikt/state/hook-journal.jsonl resolution.
//
// WHY THIS DOES NOT REUSE hookmatch.ReadSummary / ReadSummaryWindow DIRECTLY
//
// Those functions are correct for their own callers (`hook report`,
// `doctor`) but hardcode the real per-user journal path, which is exactly
// the thing PLAN-v080-reliability-core Phase 15 requires this gate to be
// able to override: criteria 2, 3, and 4 (a hook with zero records, a
// disqualifying outcome, an absent journal) cannot be exercised against
// fixtures today if the only journal this package can ever read is the
// live, machine-local one. The CLI layer's `--journal` flag is this
// package's sole testability seam for those three criteria — see
// tools/edikt/cmd/hook_soak.go.
//
// This reader is deliberately narrower than hookmatch's own: it only tallies
// per-hook (`hook_run`) records, because hook-soak.v1 has nothing to say
// about the write-time matcher's own `hook_match` records (those carry no
// `hook` field at all — see hookmatch.Record's doc comment).

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// JournalCounts is one hook's tally from a single journal read.
type JournalCounts struct {
	// Invocation is every journaled record naming this hook, regardless of
	// outcome — hook-soak.v1 `invocation_count`.
	Invocation int
	// PanicRecovered, Unregistered, and BinaryAbsent are the three
	// disqualifying outcome sub-counts (hook-soak.v1
	// panic_recovered_count / unregistered_hook_name_count /
	// binary_absent_count). Each is ALSO counted in Invocation above — an
	// invocation that panicked is still an invocation, just an unhealthy
	// one; splitting it out here would make "the hook ran" and "the hook
	// ran badly" mutually exclusive counts when they are not.
	PanicRecovered int
	Unregistered   int
	BinaryAbsent   int
}

// ReadJournal reads every hook_run-shaped record at path and tallies counts
// per hook name.
//
// found=false means the journal could not be treated as evidence AT ALL —
// either the file does not exist, or it exists but could not be opened. Both
// collapse to the same return value on purpose: from a hook's perspective,
// "the journal was never written" and "the journal exists but this process
// cannot read it" are the same fact — governance evidence that cannot be
// produced right now — and hook-soak.v1's `journal_status: unknown` does not
// distinguish between them either. The caller (Check, in hooksoak.go) is
// what turns found=false into "unknown", never into a rendered zero
// (INV-013).
//
// A LINE that fails to decode (a truncated write, a corrupted byte) is
// skipped rather than failing the whole read — consistent with
// hookmatch.ReadSummaryWindow's own tolerance for malformed lines. This
// gate's subject is per-hook invocation evidence, not journal-file
// integrity; a corrupt journal is already surfaced separately by `doctor`
// via hookmatch.Summary.Malformed.
func ReadJournal(path string) (map[string]JournalCounts, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("open hook journal %s: %w", path, err)
	}
	defer f.Close()

	counts := map[string]JournalCounts{}
	sc := bufio.NewScanner(f)
	// Same headroom as hookmatch.ReadSummaryWindow: one journal line carries
	// directive ids and detail text, and a line that exceeded a smaller
	// buffer would end the scan early and silently.
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r hookmatch.Record
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		if r.Hook == "" {
			continue
		}
		c := counts[r.Hook]
		c.Invocation++
		switch hookmatch.Outcome(r.Outcome) {
		case hookmatch.OutcomePanicRecovered:
			c.PanicRecovered++
		case hookmatch.OutcomeUnregisteredHookName:
			c.Unregistered++
		case hookmatch.OutcomeBinaryAbsent:
			c.BinaryAbsent++
		}
		counts[r.Hook] = c
	}
	if err := sc.Err(); err != nil {
		return nil, false, fmt.Errorf("scan hook journal %s: %w", path, err)
	}
	return counts, true, nil
}
