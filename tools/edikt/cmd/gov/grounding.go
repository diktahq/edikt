package gov

// grounding.go — wires internal/grounding to a surface that can FAIL.
//
// WHY THIS EXISTS
//
// The package computed a corpus grounding report and had NO production caller:
// no CLI verb, no gate, no compile step imported it, verified with `go list`
// across the module. Its "1218 of 1218 grounded" was produced only by its own
// TestGroundingCorpus — which logs findings and asserts nothing beyond
// TotalItems != 0, so it could not fail on an ungrounded excerpt either. A
// metric nothing runs is a control that cannot fail (PRD-009; SPEC-017 SR-010
// exists to catch exactly this shape), and SPEC-010 line 228 cites
// "Grounding sits at 100%" as an established fact about the corpus.
//
// WHAT IT ASKS, AND HOW THAT DIFFERS FROM THE EXTRACTION GATE
//
// Deliberately NOT the same question as `phasea verify`, and the difference is
// not an oversight — it is documented at sidecar/anchorverify.go:25-37 and is
// the reason both exist:
//
//	phasea verify   "did the extractor COPY what it claims to have copied?"
//	                sidecar.VerifyAnchors, byte-exact. A whitespace difference
//	                at extraction time means the quote was reconstructed.
//
//	this command    "does the recorded quote STILL APPEAR in the prose?"
//	                sidecar.ClassifyExcerpt, which tolerates re-wrapping,
//	                because prose gets re-wrapped over a document's life and an
//	                anchor should survive that.
//
// Two questions, two strictnesses, kept apart on purpose (GL-002). This
// command does not re-litigate that split; it makes the looser one reportable
// and failable on the verdicts that are failures under EITHER reading.
//
// WHAT FAILS, AND WHAT DELIBERATELY DOES NOT
//
// Fails on verdicts that are ungrounded however the predicate is read:
// not_found, out_of_range, no_quote, self_quoting. A self-quoting excerpt
// quotes the directive's own text rather than the artifact, so it is grounded
// in nothing.
//
// grounded_whitespace_normalized is REPORTED but does NOT fail. Whether it
// should is an open question — it is grounded to this command and a stale
// anchor to `phasea verify`, and resolving that is a decision about which
// predicate is authoritative, not a threshold for this command to pick.
// Printing it separately is what stops the two instruments disagreeing
// SILENTLY, which is the part that does not need a ruling.

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/diktahq/edikt/tools/edikt/internal/grounding"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// groundingDirs are the artifact roots scanned, matching the corpus test.
var groundingDirs = []string{
	"docs/architecture/decisions",
	"docs/architecture/invariants",
	"docs/guidelines",
}

// hardFailVerdicts are ungrounded under both the strict and the lenient
// reading, so failing on them pre-empts no decision.
var hardFailVerdicts = map[sidecar.ExcerptVerdict]bool{
	sidecar.ExcerptNotFound:    true,
	sidecar.ExcerptOutOfRange:  true,
	sidecar.ExcerptNoQuote:     true,
	sidecar.ExcerptSelfQuoting: true,
}

var groundingCmd = &cobra.Command{
	Use:   "grounding [root]",
	Short: "Report whether every sidecar excerpt still appears in the artifact it quotes",
	Long: `Scan the corpus and report each excerpt's grounding verdict.

Asks whether a recorded quote STILL APPEARS in its parent's prose, tolerating
re-wrapping. That is a different and looser question than ` + "`phasea verify`" + `,
which asks whether the extractor copied byte-exactly at extraction time. Both
exist on purpose (sidecar/anchorverify.go).

Exits 1 when any excerpt is ungrounded under either reading — not_found,
out_of_range, no_quote or self_quoting. Whitespace-normalised matches are
reported separately and do not fail.

Exits 2 when the scan measured nothing, which is not a pass.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		rep, err := grounding.Scan(root, groundingDirs)
		if err != nil {
			return &exitErr{code: 2, msg: fmt.Sprintf("grounding scan: %v", err)}
		}

		out := cmd.OutOrStdout()
		fmt.Fprint(out, rep.Summary())

		// ZERO-INPUT PATH. A scan that found no items has not established a
		// clean corpus; it has established nothing (INV-013).
		if rep.TotalItems == 0 {
			return &exitErr{code: 2, msg: "grounding scanned zero items — the corpus was not found, so this reports nothing. This is NOT a pass."}
		}

		var hard []grounding.ItemFinding
		for _, f := range rep.Findings {
			if hardFailVerdicts[f.Verdict] {
				hard = append(hard, f)
			}
		}
		// Reported whether or not it fails, so the looser predicate's
		// disagreement with the extraction gate is never silent.
		ws := rep.ByVerdict[sidecar.ExcerptGroundedWhitespace]
		if ws > 0 {
			fmt.Fprintf(out, "\n  note: %d excerpt(s) match only after whitespace normalisation.\n", ws)
			fmt.Fprintf(out, "        Grounded here, STALE to `edikt phasea verify`, which requires byte-exact.\n")
			fmt.Fprintf(out, "        Reported, not failed — which predicate governs is an open decision.\n")
		}

		if len(hard) > 0 {
			fmt.Fprintf(os.Stderr, "\n  FAIL: %d excerpt(s) grounded in nothing:\n", len(hard))
			for _, f := range hard {
				fmt.Fprintf(os.Stderr, "      %-28s %s %s[%d]\n", f.Verdict, f.ArtifactID, f.Kind, f.Index)
			}
			return &exitErr{code: 1, msg: fmt.Sprintf("%d ungrounded excerpt(s)", len(hard))}
		}
		return nil
	},
}

func init() { Cmd.AddCommand(groundingCmd) }
