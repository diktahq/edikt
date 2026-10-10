package cmd

// doctor_failed_terminal.go — an unconsumed plan stuck at failed-terminal is
// a doctor RED (D57). A host-rejected explicit
// extractor.model pin is one cause; retry exhaustion is another; both leave
// the plan permanently unable to reach Phase B without a human decision
// (a fresh plan from a changed input), and doctor is where a user looks
// first for exactly that kind of stuck state.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// failedTerminalNote names one unconsumed plan with at least one
// failed-terminal item.
type failedTerminalNote struct {
	PlanID string
	ItemID string
	Reason string
}

// checkFailedTerminalReceipts scans every receipt under
// .edikt/state/phasea/*.receipt.json and reports each failed-terminal item
// belonging to a still-unconsumed plan. A malformed receipt is skipped, not
// fatal — this is an advisory scan over runtime state, not a load-bearing
// parse.
func checkFailedTerminalReceipts(projectRoot string) []failedTerminalNote {
	dir := filepath.Join(projectRoot, ".edikt", "state", "phasea")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var notes []failedTerminalNote
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".receipt.json") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			continue
		}
		var r phasea.Receipt
		if jerr := json.Unmarshal(data, &r); jerr != nil {
			continue
		}
		if r.Consumed {
			continue
		}
		for _, it := range r.Items {
			if it.Status != phasea.StatusFailedTerminal {
				continue
			}
			notes = append(notes, failedTerminalNote{PlanID: r.PlanID, ItemID: it.ItemID, Reason: it.Diagnostic})
		}
	}
	return notes
}

// runFailedTerminalCheck prints one ERROR line per failed-terminal item and
// returns the count for doctor's error tally.
func runFailedTerminalCheck(projectRoot string, out io.Writer) int {
	notes := checkFailedTerminalReceipts(projectRoot)
	for _, n := range notes {
		fmt.Fprintf(out, "  ERROR: plan %s, item %s is failed-terminal: %s\n"+
			"         Not retryable — a fresh plan requires a changed input.\n",
			n.PlanID, n.ItemID, n.Reason)
	}
	return len(notes)
}
