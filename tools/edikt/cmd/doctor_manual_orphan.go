package cmd

// doctor_manual_orphan.go — Phase 8 "Orphan Manual Ref" check for
// `edikt doctor`. INV-002: accepted ADRs are immutable. Manual directives
// that cite a non-existent ADR file are surfaced as ORPHAN findings. The
// remediation is to fix the ref tag in the sidecar's manual_directives
// list — never to backfill a missing ADR body.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// manualRefRe captures the artifact ID inside a `(ref: …)` clause. The
// allowlist is intentionally narrow — INV-006 normalisation runs in
// idvalidate.ArtifactID before any filesystem lookup, so a Unicode-lookalike
// ID is rejected before it can map to a real-world path.
var manualRefRe = regexp.MustCompile(`\(ref:\s*([A-Za-z0-9_-]+)`)

// runOrphanManualRefCheck walks every sidecar in the artifact directories
// and flags manual_directives whose `(ref: ADR-NNN)` cites a non-existent
// ADR file under paths.decisions. Returns (warns, ran). ran is false when
// the artifact dirs are absent (non-edikt project).
func runOrphanManualRefCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	dirs := resolveArtifactDirs(projectRoot)
	if dirs.decisions == "" && dirs.invariants == "" && dirs.guidelines == "" {
		return 0, false
	}

	// Collect every sidecar under the artifact dirs.
	type sidecarRef struct {
		path string
		sc   *sidecar.Sidecar
	}
	var sidecars []sidecarRef
	for _, d := range []string{dirs.decisions, dirs.invariants, dirs.guidelines} {
		if d == "" {
			continue
		}
		_ = filepath.Walk(d, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(p, ".edikt.yaml") {
				return nil
			}
			sc, lerr := sidecar.Load(p)
			if lerr != nil {
				return nil
			}
			sidecars = append(sidecars, sidecarRef{path: p, sc: sc})
			return nil
		})
	}
	if len(sidecars) == 0 {
		return 0, false
	}

	// Index ADR file basenames present on disk so we can detect orphans
	// without filesystem-walking on every reference.
	adrFiles := map[string]bool{}
	if dirs.decisions != "" {
		entries, _ := os.ReadDir(dirs.decisions)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			// "ADR-NNN-foo.md" → "ADR-NNN" (matches what gov:compile uses).
			base := strings.TrimSuffix(name, ".md")
			if strings.HasPrefix(base, "ADR-") || strings.HasPrefix(base, "INV-") {
				end := 4
				for end < len(base) && base[end] >= '0' && base[end] <= '9' {
					end++
				}
				adrFiles[base[:end]] = true
			}
		}
	}

	headerPrinted := false
	printHeader := func() {
		if !headerPrinted {
			fmt.Fprintln(w, "  ── Orphan Manual Refs ──────────────────────────")
			headerPrinted = true
		}
	}

	for _, sr := range sidecars {
		for _, md := range sr.sc.ManualDirectives {
			m := manualRefRe.FindStringSubmatch(md)
			if m == nil {
				continue
			}
			id := m[1]
			// INV-006: validate before any filesystem lookup so a
			// pathological ID can't bypass the allowlist via NFKC
			// trickery.
			if err := idvalidate.ArtifactID(id); err != nil {
				continue
			}
			if !strings.HasPrefix(id, "ADR-") {
				// Phase 8 scope is ADR refs only — INV/guideline IDs in
				// manual_directives may legitimately appear without a
				// matching .md file in dirs.decisions.
				continue
			}
			if adrFiles[id] {
				continue
			}
			printHeader()
			fmt.Fprintf(w, "  ORPHAN: manual directive in %s cites %s which does not exist.\n",
				rel(projectRoot, sr.path), id)
			warns++
		}
	}

	return warns, true
}
