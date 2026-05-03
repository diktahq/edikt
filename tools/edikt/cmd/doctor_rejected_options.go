package cmd

// doctor_rejected_options.go — "Rejected Options Coverage" check for
// `edikt doctor`. INV-002: accepted ADRs are immutable. The only valid
// remediation path is bin/edikt sidecar add-manual-directive — the
// failure message MUST NOT suggest editing the ADR body.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// optionHeadingRe matches `### A.`, `### B.` … or `### Option A`, `### Option B` …
var optionHeadingRe = regexp.MustCompile(`^###\s+(?:Option\s+)?([A-Z])[\.\s]`)

// genericOptionHeadingRe matches any non-empty `### {title}` heading. Used as
// a fallback when the ADR uses free-form option titles (e.g. `### Per-concern
// mechanisms (chosen)`) instead of the lettered `### A.` / `### Option A`
// conventions. Empty headings (`### `) are excluded.
var genericOptionHeadingRe = regexp.MustCompile(`^###\s+\S`)

// mustNotRe detects MUST NOT / NEVER in manual_directives entries.
var mustNotRe = regexp.MustCompile(`\b(MUST NOT|NEVER)\b`)

// runRejectedOptionsCheck walks the decisions directory and warns for every
// ADR that has ≥2 Considered Options but no prohibition coverage. Coverage
// = len(prohibitions[]) + count(manual_directives matching mustNotRe).
//
// Returns (warns, ran). ran is false when the decisions dir is absent
// (non-edikt project root or pre-init state).
func runRejectedOptionsCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	dirs := resolveArtifactDirs(projectRoot)
	if dirs.decisions == "" {
		return 0, false
	}
	if _, err := os.Stat(dirs.decisions); err != nil {
		return 0, false
	}

	var mdFiles []string
	_ = filepath.Walk(dirs.decisions, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".md") {
			mdFiles = append(mdFiles, p)
		}
		return nil
	})

	if len(mdFiles) == 0 {
		return 0, false
	}

	headerPrinted := false
	printHeader := func() {
		if !headerPrinted {
			fmt.Fprintln(w, "  ── Rejected Options Coverage ───────────────────")
			headerPrinted = true
		}
	}

	for _, mdPath := range mdFiles {
		optCount, optB := countConsideredOptions(mdPath)
		if optCount < 2 {
			continue
		}

		sidecarPath := sidecarPathFor(mdPath)
		if _, err := os.Stat(sidecarPath); err != nil {
			// Sidecar absent — Phase 1 MISSING check covers this gap.
			// Emit INFO so the user knows we skipped, then continue.
			printHeader()
			fmt.Fprintf(w, "  INFO: %s has %d considered options but no sidecar — skipping prohibition check (run /edikt:adr:compile first)\n",
				rel(projectRoot, mdPath), optCount)
			continue
		}

		sc, err := sidecar.Load(sidecarPath)
		if err != nil {
			// Schema error — SCHEMA INVALID check covers this.
			continue
		}

		coverage := len(sc.Prohibitions)
		for _, md := range sc.ManualDirectives {
			if mustNotRe.MatchString(md) {
				coverage++
			}
		}

		if coverage >= 1 {
			continue
		}

		// Failure: emit WARN with INV-002-compliant remediation.
		printHeader()
		slug := filepath.Base(strings.TrimSuffix(mdPath, ".md"))
		adrID := slug // e.g. "ADR-099-test"

		// Extract the alternative hint: option B title (the first rejected option).
		alternative := optB
		if alternative == "" {
			alternative = "the rejected alternative"
		}

		fmt.Fprintf(w,
			"  WARN: %s has %d considered options but no prohibition coverage. "+
				"The chosen design rejected %d alternatives — without MUST NOT directives, an LLM may re-propose them. "+
				"Run: bin/edikt sidecar add-manual-directive --path %s --text 'MUST NOT use %s — superseded by %s'\n",
			slug, optCount, optCount-1,
			rel(projectRoot, sidecarPath),
			alternative, adrID,
		)
		warns++
	}

	if headerPrinted && warns == 0 {
		fmt.Fprintln(w, "  All ADRs with considered options have prohibition coverage.")
	}

	return warns, true
}

// countConsideredOptions parses a markdown file and returns the number of
// lettered option headings found under ## Considered Options, plus the
// title text of option B (the first non-chosen candidate) for use in the
// remediation hint.
func countConsideredOptions(mdPath string) (count int, optBTitle string) {
	f, err := os.Open(mdPath)
	if err != nil {
		return 0, ""
	}
	defer f.Close()

	inSection := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()

		if strings.HasPrefix(line, "## ") {
			if strings.Contains(strings.ToLower(line), "considered options") {
				inSection = true
				continue
			}
			if inSection {
				// Hit the next ## section — we're done.
				break
			}
			continue
		}

		if !inSection {
			continue
		}

		// Lettered style: `### A. Foo` / `### Option A`. Captures optBTitle
		// from the lettered B heading directly.
		if m := optionHeadingRe.FindStringSubmatch(line); m != nil {
			count++
			if m[1] == "B" && optBTitle == "" {
				after := optionHeadingRe.ReplaceAllString(line, "")
				optBTitle = strings.TrimSpace(after)
			}
			continue
		}
		// Generic free-form title style: `### Free form title`. The second
		// such heading is treated as the first rejected alternative for the
		// remediation hint (when the lettered branch did not capture one).
		if genericOptionHeadingRe.MatchString(line) {
			count++
			if count == 2 && optBTitle == "" {
				optBTitle = strings.TrimSpace(strings.TrimPrefix(line, "###"))
			}
		}
	}
	return count, optBTitle
}
