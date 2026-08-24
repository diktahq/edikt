// verify_anchors.go — the ADR-056 anchor rung of the content gate.
//
// It moved here from runner.go with the rest of the gate (SPEC-013 Phase 3):
// runner.go's ClaudeRunner is deleted in Phase 4, and this check has to
// outlive it — `phasea verify` grades an item with it and `phasea restore`
// decides a quarantine on it, neither of which involves a dispatch. The
// package is unchanged, so the tests that already pin its behaviour
// (parsegate_test.go, through the dispatch path that still exists today, and
// verify_anchors_test.go directly) run against the new home unmodified.
package phasea

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// verifyAnchorsAgainstParent is the ADR-056 gate: every anchor in a  edikt-guard:allow
// freshly-written sidecar must quote the parent artifact byte-exactly at the
// line range it records.
//
// It runs at the GENERATION BOUNDARY, per dispatch, alongside the parse and
// schema rungs — and it fails the dispatch rather than warning. A warning here
// would be the absence-rendering-as-pass shape INV-013 forbids: the compile  edikt-guard:allow
// continues, the corpus takes on an anchor pointing at prose that is not there,
// and nothing downstream distinguishes it from a verified one.
//
// The failure names every offending anchor with its recorded range AND what
// actually sits at that range. An anchor error is invisible from the quote
// alone — the text is real prose from the artifact — so a message that omits
// the actual lines leaves the reader unable to see what is wrong.
func verifyAnchorsAgainstParent(sidecarPath, parentPath string) error {
	sc, err := sidecar.Load(sidecarPath)
	if err != nil {
		return err
	}

	// No items means nothing to verify — a measured zero, not a skipped
	// check. The parent is not read in this case, because reading it would
	// only manufacture an error for a sidecar that has no anchors to be wrong
	// about (a roadmap-only ADR legitimately compiles to `directives: []`).
	if len(sc.Directives) == 0 && len(sc.Prohibitions) == 0 {
		return nil
	}

	body, err := os.ReadFile(parentPath)
	if err != nil {
		// The parent is the oracle. There ARE anchors here and none of them
		// could be checked — that is unmeasured, and unmeasured must not read
		// as a clean bill of health (INV-011).
		return fmt.Errorf("cannot read parent %s to verify %d item(s) of anchors: %w",
			parentPath, len(sc.Directives)+len(sc.Prohibitions), err)
	}

	v := sidecar.VerifyAnchors(sc, string(body))
	if v.OK() {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "extractor wrote %d invalid anchor(s) of %d across %d item(s) — every anchor must quote %s byte-exactly at the line range it records",
		len(v.Faults), v.Anchors, v.Items, filepath.Base(parentPath))
	for _, f := range v.Faults {
		fmt.Fprintf(&b, "\n    %s", f.String())
	}
	return fmt.Errorf("%s", b.String())
}
