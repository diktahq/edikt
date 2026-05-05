// Package orphan implements the orphan-ADR detection + state transition
// logic ported from commands/gov/compile.md §12d.
//
// Rules (SPEC-005 Phase 7):
//
//	1. First detection   — no prior history. Warn, exit 0, write history.
//	2. Consecutive same  — prior set exists and matches current. BLOCK.
//	3. Subset / different — some orphans resolved. Reset to first-detection.
//	4. Superset          — new orphans added. First-detection for new set.
//	5. Fallthrough       — sets differ but neither sub/superset. First-detection.
package orphan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// State is the payload written to .edikt/state/compile-history.json.
type State struct {
	SchemaVersion int      `json:"schema_version"`
	LastCompileAt string   `json:"last_compile_at"`
	OrphanADRs    []string `json:"orphan_adrs"`
	EdiktVersion  string   `json:"edikt_version,omitempty"`
}

// Scenario names the transition case.
type Scenario string

const (
	FirstDetection       Scenario = "first_detection"
	Consecutive          Scenario = "consecutive"
	SubsetResolved       Scenario = "subset_resolved"
	SupersetAdded        Scenario = "superset_added"
	DifferentReset       Scenario = "different_reset"
	NoOrphans            Scenario = "no_orphans"
)

// Decide computes the transition scenario.
// Returns the scenario, whether to block, and whether to write history.
func Decide(current []string, prior *[]string) (sc Scenario, block bool, write bool) {
	curr := uniqSort(current)

	if len(curr) == 0 {
		return NoOrphans, false, true
	}
	if prior == nil {
		return FirstDetection, false, true
	}

	priorSet := make(map[string]struct{}, len(*prior))
	for _, p := range *prior {
		priorSet[p] = struct{}{}
	}
	currSet := make(map[string]struct{}, len(curr))
	for _, c := range curr {
		currSet[c] = struct{}{}
	}

	if setEqual(priorSet, currSet) {
		// Consecutive — BLOCK, do not overwrite history.
		return Consecutive, true, false
	}
	if isSubset(currSet, priorSet) {
		return SubsetResolved, false, true
	}
	if isSubset(priorSet, currSet) {
		return SupersetAdded, false, true
	}
	return DifferentReset, false, true
}

// ReadState loads the history file. Returns (nil, nil) if absent, which the
// caller should treat as "no prior history".
func ReadState(path string) (*State, error) {
	s, _, err := ReadStateDetailed(path)
	return s, err
}

// ReadStateDetailed is the verbose form of ReadState used by callers that
// need to know whether the history was missing, corrupt, or healthy. The
// boolean reports whether the file was loadable; when false the caller
// should treat the history as absent (matches the python heredoc's
// `history_loadable` flag and the `[WARN] compile-history.json is
// unparseable` log line).
func ReadStateDetailed(path string) (*State, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		// Corrupt history — treat as absent (loud warning logged by
		// caller). Loadable=false signals the corruption.
		return nil, false, nil
	}
	// Schema sanity: orphan_adrs must be a JSON array (decoded slice).
	// A nil slice from a missing key is fine; a non-list would have
	// failed Unmarshal already because OrphanADRs is []string.
	return &s, true, nil
}

// WriteState atomically writes the history file (tmp + rename) using the
// current UTC clock for last_compile_at.
func WriteState(path string, orphans []string, ediktVersion string) error {
	return WriteStateAt(path, orphans, ediktVersion, time.Now().UTC())
}

// WriteStateAt is the deterministic form of WriteState. The timestamp is
// injected by the caller so byte-equal output tests have a stable input.
// Used by `bin/edikt gov compile-history` when the operator pins a clock.
func WriteStateAt(path string, orphans []string, ediktVersion string, ts time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir state: %w", err)
	}
	state := State{
		SchemaVersion: 1,
		LastCompileAt: ts.UTC().Format("2006-01-02T15:04:05Z"),
		OrphanADRs:    uniqSort(orphans),
		EdiktVersion:  ediktVersion,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func uniqSort(in []string) []string {
	m := make(map[string]struct{}, len(in))
	for _, s := range in {
		m[s] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func setEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func isSubset(small, large map[string]struct{}) bool {
	for k := range small {
		if _, ok := large[k]; !ok {
			return false
		}
	}
	return true
}
