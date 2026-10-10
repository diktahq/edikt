package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Doctor's "unstripped legacy block present" check — the third detection
// layer ADR-082 keeps in the base install (upgrade primary, compile guard
// rail, doctor standing report). It consults the same fence-aware scanner
// compile refuses on, so the two cannot disagree about what a block is.
//
// 🔴 for an artifact compile would include: it is governed on paper and
// reaches nothing — ADR-009 on this corpus, accepted, no sidecar, absent
// from every surface, reported by nothing until this line existed.
// WARN for a skip-listed artifact (superseded, deprecated, or carrying an
// explicit edikt:migration:skip marker — ADR-009 on this corpus): its
// directives would not compile after a lift either, so the block is residue
// or a declared documentary block, not a gap.
func runLegacyBlockCheck(projectRoot string, dirs []string, out io.Writer) (errs, warns int) {
	findings, err := migrate.Scan(projectRoot, dirs)
	if err != nil {
		fmt.Fprintf(out, "  WARN: legacy-block scan UNMEASURED — %v\n", err)
		return 0, 1
	}
	if len(findings) == 0 {
		return 0, 0
	}
	for _, f := range findings {
		if skip, why := sidecar.IsSkipListed(filepath.Join(projectRoot, f.Path)); skip {
			fmt.Fprintf(out, "  WARN: unstripped legacy block present on a skip-listed artifact — %s:%d (%s); nothing compiles from it, lift or leave\n", f.Path, f.Line, why)
			warns++
			continue
		}
		fmt.Fprintf(out, "  🔴 ERROR: unstripped legacy block present — %s:%d is governed on paper and reaches no compiled surface. Run /edikt:migrate\n", f.Path, f.Line)
		errs++
	}
	return errs, warns
}
