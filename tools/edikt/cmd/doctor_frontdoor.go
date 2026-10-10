package cmd

// doctor_frontdoor.go — surfaces the EDIKT_INVOKER front-door signal in
// `bin/edikt doctor` output.
//
// Its own group, not folded into doctor_hook_journal.go: that file's own
// note forbids mixing journals, and `events.jsonl` (written by emitEvent,
// cmd/migrate.go:1044) is a different journal from the hook journal
// hookmatch reads — a different writer, a different shape, a different
// question.
//
// Read-only / soft-signal, matching reportPostFlightScope's own posture:
// doctor's exit code never depends on this section. EDIKT_INVOKER is a
// self-declared signal any session can set — this counts whether callers
// identify themselves, never whether they are trustworthy, and MUST NOT
// be described as enforcement.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// frontDoorWindow is the reporting window doctor's count covers,
// matching SR-009's own "(last 30 days)" text verbatim.
const frontDoorWindow = 30 * 24 * time.Hour

// reportFrontDoor prints the Front Door section of doctor's output.
// Always exits cleanly — never affects the doctor exit code.
func reportFrontDoor(ediktRoot string) {
	fmt.Println()
	fmt.Println("Front Door:")

	path := filepath.Join(ediktRoot, "events.jsonl")
	total, undeclared, err := frontDoorCounts(path, time.Now())
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  undeclared invoker: no walled-verb calls recorded yet (last 30 days)")
			return
		}
		fmt.Printf("  (could not read %s: %v)\n", path, err)
		return
	}
	if total == 0 {
		fmt.Println("  undeclared invoker: no walled-verb calls recorded yet (last 30 days)")
		return
	}
	fmt.Printf("  undeclared invoker: %d of %d walled-verb calls (last 30 days)\n", undeclared, total)
	fmt.Println("  (a self-declared signal — every tier-1 command sets EDIKT_INVOKER; steering, not enforcement)")
}

// frontDoorCounts reads path (events.jsonl) and returns (total,
// undeclared) verb_invoked events within window of now. undeclared
// counts entries whose invoked_by is "direct" (no EDIKT_INVOKER was
// set and the call proceeded because stdin was a real terminal).
//
// A malformed line is skipped, never fatal — one corrupt record must
// not blind doctor to every other line in the journal.
func frontDoorCounts(path string, now time.Time) (total, undeclared int, err error) {
	body, err := os.ReadFile(path) //nolint:gosec // user-global state, doctor-read-only
	if err != nil {
		return 0, 0, err
	}
	cutoff := now.Add(-frontDoorWindow)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Event     string `json:"event"`
			Timestamp string `json:"timestamp"`
			InvokedBy string `json:"invoked_by"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue // one malformed line must not blind the count to the rest
		}
		if rec.Event != "verb_invoked" {
			continue
		}
		ts, perr := time.Parse("2006-01-02T15:04:05Z", rec.Timestamp)
		if perr != nil || ts.Before(cutoff) {
			continue
		}
		total++
		if rec.InvokedBy == "direct" {
			undeclared++
		}
	}
	return total, undeclared, nil
}
