package gov

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// COUNTS, NOT VERDICTS.
//
// This verb counts two things in a governance record's prose: bare artifact
// ids (an id used as the subject of a sentence rather than as a citation) and
// hedges (a rule whose obligation is stated in soft modal force). It decides
// nothing. The review commands display the numbers and a human reads them.
//
// That division is deliberate and it is the tier boundary: a count is
// mechanical and belongs in the binary; whether a particular hedge is a
// problem is judgement and belongs to a reader. A command that BRANCHED on
// these fields would be a tier-1 command deciding from a tier-2 verb's output
// shape, which the tier contract forbids — the exit code is the contract, and
// this verb's exit code says only whether it could read the file.
var (
	// A bare id: an artifact id NOT inside a `(ref: …)` citation and not in a
	// frontmatter references list. Counted per occurrence.
	bareIDRe = regexp.MustCompile(`\b(?:ADR|INV|GL|SPEC|PRD|PLAN|BRAIN|DISCOVERY)-[A-Za-z0-9]*-?\d{3,}\b`)
	refTagRe = regexp.MustCompile(`\(ref:[^)]*\)`)

	// Hedges: soft modal force in a document whose whole purpose is binding
	// obligation. "should probably" binds nothing and compiles to advisory.
	hedgeRe = regexp.MustCompile(`(?i)\b(should probably|should ideally|might want to|could consider|where possible|if possible|as appropriate|generally|typically|usually|preferably|ideally|try to|aim to|where practical)\b`)
)

// proseStats is the JSON this verb prints. Field names are the contract.
type proseStats struct {
	File    string `json:"file"`
	Lines   int    `json:"lines"`
	BareIDs int    `json:"bare_ids"`
	Hedges  int    `json:"hedges"`
	// Samples carry the first few of each so a reader can see what was
	// counted. A count with no example is a number nobody can act on.
	BareIDSamples []string `json:"bare_id_samples,omitempty"`
	HedgeSamples  []string `json:"hedge_samples,omitempty"`
}

var proseStatsCmd = &cobra.Command{
	Use:   "prose-stats <file>",
	Short: "Count bare artifact ids and hedges in a governance record's prose",
	Long: `Prints JSON with bare_ids and hedges counts for one governance record,
plus a few samples of each so the numbers can be acted on.

It DECIDES NOTHING. The review commands display the output; a human reads it.
A caller that branched on these fields would be deciding from this verb's
output shape rather than its exit code, which the tier contract forbids.

Exit codes:
  0 — counted (the numbers may be zero; zero is a measurement)
  3 — the file could not be read`,
	Args: cobra.ExactArgs(1),
	// Bridge *exitErr to os.Exit — see auditcite.go's identical comment.
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runProseStats(cmd, args)
		exitFromExitErr(err)
		return err
	},
}

func runProseStats(cmd *cobra.Command, args []string) error {
	path := args[0]
	f, err := os.Open(path) //nolint:gosec // operator-supplied path, read-only
	if err != nil {
		return &exitErr{code: 3, msg: fmt.Sprintf("cannot read %s: %v", path, err)}
	}
	defer f.Close()

	// A record naming ITSELF — its own title heading, its own filename id —
	// is not a citation and must not count as bare. Every artifact's title
	// carries its own id; counting it would put every file at bare_ids >= 1
	// and drown the signal this verb exists to surface.
	self := selfIDOf(path)

	st := proseStats{File: path}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inFrontmatter := false
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		st.Lines++
		// Frontmatter is where a citation BELONGS, so ids there are not bare.
		if n == 1 && strings.TrimSpace(line) == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
			}
			continue
		}
		// A `(ref: …)` tag is a citation, not a subject: strip it before
		// counting, or every properly-cited directive reads as a violation.
		stripped := refTagRe.ReplaceAllString(line, "")
		for _, m := range bareIDRe.FindAllString(stripped, -1) {
			if self != "" && strings.EqualFold(m, self) {
				continue
			}
			st.BareIDs++
			if len(st.BareIDSamples) < 5 {
				st.BareIDSamples = append(st.BareIDSamples, fmt.Sprintf("%d: %s", n, m))
			}
		}
		for _, m := range hedgeRe.FindAllString(line, -1) {
			st.Hedges++
			if len(st.HedgeSamples) < 5 {
				st.HedgeSamples = append(st.HedgeSamples, fmt.Sprintf("%d: %s", n, m))
			}
		}
	}
	if err := sc.Err(); err != nil {
		return &exitErr{code: 3, msg: fmt.Sprintf("read %s: %v", path, err)}
	}

	body, err := json.MarshalIndent(&st, "", "  ")
	if err != nil {
		return &exitErr{code: 3, msg: fmt.Sprintf("encode: %v", err)}
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(body))
	return nil
}

func init() { Cmd.AddCommand(proseStatsCmd) }
