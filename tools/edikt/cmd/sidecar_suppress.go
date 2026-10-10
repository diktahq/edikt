package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// `suppressed_directives` was READABLE AND UNWRITABLE.
//
// Five places in the binary subtract it at render time, `gov compile`
// documents the merge formula that uses it, and that command's own misuse
// warning says "use suppressed_directives instead" — while no verb could
// write one. The only route was hand-editing the sidecar YAML, which is the
// failure the enrich command's rationale exists to prevent; the first attempt
// at it broke the file on an apostrophe.
//
// The case is ordinary and it has no other remedy: an accepted ADR is refined
// by a later accepted ADR that withdraws one clause. The parent is immutable,
// so the extractor keeps emitting the withdrawn directive, and the corpus
// carries two active directives requiring opposite things.
//
// WHY THE REASON IS NOT IN THE SIDECAR. `suppressed_directives` is a list of
// strings and the schema is closed (`additionalProperties: false`), so a
// reason field is a schema change and a schema change is an ADR. The reason
// goes where this project already records a human's decision about a sidecar
// — beside the approval diffs — so the record exists today rather than after
// a schema round, and the sidecar keeps exactly the shape every reader of it
// already handles.
var (
	suppressPath   string
	suppressText   string
	suppressReason string
)

// SuppressionRecordDir is where the human decision behind a suppression is
// recorded. Same family as the approval diffs, for the same reason: the
// decision is the interesting artifact and it outlives any regeneration.
const SuppressionRecordDir = "docs/internal/suppressions"

type suppressionRecord struct {
	SuppressedAt  string `yaml:"suppressed_at"`
	SidecarPath   string `yaml:"sidecar_path"`
	ArtifactPath  string `yaml:"artifact_path,omitempty"`
	DirectiveText string `yaml:"directive_text"`
	Reason        string `yaml:"reason"`
}

var suppressDirectiveCmd = &cobra.Command{
	Use:   "suppress-directive",
	Short: "Withdraw one extracted directive from the compiled corpus, with the reason",
	Long: `Records <text> in the suppressed_directives[] list of <sidecar-path> so
gov compile subtracts it from the effective rule set, without editing the
parent .md (accepted artifacts are immutable) and without deleting the
directive (the extractor would emit it again on the next extraction).

The match is EXACT against directives[].text. A suppression that matches
nothing is already a compile warning, so writing one is writing a defect:
this refuses instead.

--reason is required and is recorded under ` + SuppressionRecordDir + `,
because a suppression a later reader cannot explain is indistinguishable from
a mistake.

Exit codes:
  0 — success
  1 — validation error
  2 — sidecar file missing
  3 — duplicate suppression, or no directive matches the text`,
	Args: cobra.NoArgs,
	RunE: runSuppressDirective,
}

func runSuppressDirective(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(suppressText) == "" {
		return &exitCodeError{code: 1, msg: "error: --text is empty"}
	}
	if strings.TrimSpace(suppressReason) == "" {
		return &exitCodeError{code: 1, msg: "error: --reason is required — a suppression a later reader cannot explain is indistinguishable from a mistake"}
	}

	sidecarPath, err := resolveSidecarPath(suppressPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: --path: %v", err)}
	}
	if _, statErr := os.Stat(sidecarPath); os.IsNotExist(statErr) {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("error: sidecar not found: %s", sidecarPath)}
	}
	sc, err := sidecar.Load(sidecarPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: loading sidecar: %v", err)}
	}

	// EXACT match against the directive text, because that is what the
	// subtraction at render time compares. A near match here would suppress
	// nothing and report success.
	matched := false
	for _, d := range sc.Directives {
		if d.Text == suppressText {
			matched = true
			break
		}
	}
	if !matched {
		var sample []string
		for i, d := range sc.Directives {
			if i == 3 {
				break
			}
			sample = append(sample, "    "+d.Text)
		}
		return &exitCodeError{code: 3, msg: fmt.Sprintf(
			"no directive in %s matches --text exactly.\n"+
				"  The subtraction at render time is an exact string comparison, so a suppression\n"+
				"  that matches nothing suppresses nothing and shows up as a compile warning.\n"+
				"  The sidecar's directives begin:\n%s",
			filepath.Base(sidecarPath), strings.Join(sample, "\n"))}
	}

	for i, existing := range sc.SuppressedDirectives {
		if existing == suppressText {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("already suppressed at index %d.", i)}
		}
	}

	sc.SuppressedDirectives = append(sc.SuppressedDirectives, suppressText)
	if err := sc.Validate(); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: validate after suppression: %v", err)}
	}
	out, err := sidecar.Marshal(sc)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: marshal: %v", err)}
	}
	if err := migrate.AtomicWriteNoFollow(sidecarPath, out, 0o644); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("error: write: %v", err)}
	}

	recordSuppression(sidecarPath, suppressionRecord{
		SuppressedAt:  time.Now().UTC().Format(time.RFC3339),
		SidecarPath:   sidecarPath,
		ArtifactPath:  sc.Path,
		DirectiveText: suppressText,
		Reason:        suppressReason,
	})

	fmt.Fprintf(os.Stdout, "ok: suppressed one directive in %s\n", sidecarPath)
	fmt.Fprintf(os.Stdout, "    %q\n", suppressText)
	fmt.Fprintf(os.Stdout, "    reason: %s\n", suppressReason)
	fmt.Fprintln(os.Stdout, "    the directive itself is untouched — the parent is immutable and the extractor will emit it again; the subtraction happens at render time")
	return nil
}

// suppressionProjectRoot is the project a sidecar belongs to, derived from
// the sidecar's OWN path and nothing else.
//
// A governance artifact lives under the project's docs tree, so the segment
// before `docs/` is the root; with no such segment, the record goes beside the
// sidecar. Deliberately NOT a climb-until-you-find-a-marker walk: an unbounded
// climb leaves the project entirely when the markers are absent — measured
// here, it walked out of a temp directory into the shared temp parent and
// wrote the record there, which is the same class as the fixed four-level
// climb it replaced. A rule that reads the input rather than the surrounding
// filesystem cannot wander.
func suppressionProjectRoot(sidecarPath string) string {
	sep := string(os.PathSeparator)
	marker := sep + "docs" + sep
	if i := strings.LastIndex(sidecarPath, marker); i > 0 {
		return sidecarPath[:i]
	}
	return filepath.Dir(sidecarPath)
}

// recordSuppression writes the human decision beside the approval diffs.
// Best-effort, and LOUD when it fails: the suppression is already on disk, and
// a suppression whose reason went unrecorded is the thing this verb exists to
// prevent.
func recordSuppression(sidecarPath string, r suppressionRecord) {
	root := suppressionProjectRoot(sidecarPath)
	dir := filepath.Join(root, SuppressionRecordDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warn: suppression reason NOT recorded (mkdir %s): %v — the suppression is written; record the reason by hand\n", dir, err)
		return
	}
	name := fmt.Sprintf("%s-%s.yaml",
		strings.ReplaceAll(r.SuppressedAt, ":", ""),
		sanitizeDiffID(strings.TrimSuffix(filepath.Base(sidecarPath), ".edikt.yaml")))
	out, err := yaml.Marshal(&r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: suppression reason NOT recorded (marshal): %v\n", err)
		return
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, out, 0o644); err != nil { //nolint:gosec // decision record, world-readable by design
		fmt.Fprintf(os.Stderr, "warn: suppression reason NOT recorded (write %s): %v\n", path, err)
		return
	}
	// The ABSOLUTE path: a relative one is ambiguous the moment the caller's
	// working directory is not the project root, and this line exists so a
	// reader can open the record.
	fmt.Fprintf(os.Stdout, "    reason recorded: %s\n", path)
}

func init() {
	suppressDirectiveCmd.Flags().StringVar(&suppressPath, "path", "", "path to the sidecar (or its parent .md)")
	suppressDirectiveCmd.Flags().StringVar(&suppressText, "text", "", "the directive text to withdraw, matched EXACTLY")
	suppressDirectiveCmd.Flags().StringVar(&suppressReason, "reason", "", "which record withdrew it, and why (required)")
	_ = suppressDirectiveCmd.MarkFlagRequired("path")
	_ = suppressDirectiveCmd.MarkFlagRequired("text")
	sidecarCmd.AddCommand(suppressDirectiveCmd)
}
