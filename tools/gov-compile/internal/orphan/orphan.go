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
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		// Corrupt history — treat as absent (loud warning logged by caller).
		return nil, nil
	}
	return &s, nil
}

// WriteState atomically writes the history file (tmp + rename).
func WriteState(path string, orphans []string, ediktVersion string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir state: %w", err)
	}
	state := State{
		SchemaVersion: 1,
		LastCompileAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
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
