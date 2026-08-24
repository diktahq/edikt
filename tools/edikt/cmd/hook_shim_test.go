package cmd

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook"
	"github.com/diktahq/edikt/tools/edikt/internal/hook/shim"
)

// A CANONICAL SHIM MUST FAIL THE DISPATCH-ORCHESTRATOR CHECK.
//
// This is the regression that prompted the check. A bulk re-render walked every
// migrated registry row, regenerated phase-end-detector.sh as a canonical exec
// shim — 135 lines to 10, three dispatch sites replaced by one `hook run` line
// with no --step — and `shim check` reported "0 divergences", because the file
// now matched the render it was never meant to be generated from.
//
// The three properties the check asserted were all TRUE of that file: non-empty,
// execs `hook run <name>`, no python3. Necessary and not sufficient. What makes
// this shape different is that it is NOT a canonical render and that it carries
// --step, so those are what the check now asserts.
func TestDispatchOrchestratorRejectsACanonicalShim(t *testing.T) {
	stopEvent := "Stop"
	row := hook.Row{Name: "phase-end-detector", File: "phase-end-detector.sh", Event: &stopEvent}
	canonical, err := shim.Render("phase-end-detector", "Stop")
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	problems := checkDispatchOrchestrator(row, []byte(canonical))
	if len(problems) == 0 {
		t.Fatal("a canonical exec shim passed the dispatch-orchestrator check — " +
			"this is exactly the flattening that shipped once and reported 0 divergences")
	}
	var sawIdentity bool
	for _, p := range problems {
		if strings.Contains(p, "IS the canonical exec shim") {
			sawIdentity = true
		}
	}
	if !sawIdentity {
		t.Errorf("the check complained, but not that the file IS a canonical render:\n%v", problems)
	}

	// The control: the REAL caller must still pass, or the check above proves
	// only that it rejects everything.
	real := "#!/usr/bin/env bash\n" +
		"_TOOL=claude\n" +
		"_argv=(hook run phase-end-detector --event Stop --step \"$_STEP\")\n" +
		"exec \"$EDIKT_BIN\" \"${_argv[@]}\"\n"
	if got := checkDispatchOrchestrator(row, []byte(real)); len(got) != 0 {
		t.Errorf("the real dispatch-orchestrating caller was rejected: %v", got)
	}
}
