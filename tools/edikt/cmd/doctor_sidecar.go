package cmd

// doctor_sidecar.go — Phase 7 of the sidecar architecture plan (PLAN-sidecar-architecture).
//
// Adds a "Sidecar Health" check group to `edikt doctor`, walking the project
// (cwd) for sidecar/parent pairing issues. Five checks per the plan prompt:
//
//   1. ORPHAN          — .edikt.yaml without a sibling .md             (errN++)
//   2. MISSING         — .md without a sibling .edikt.yaml             (errN++)
//   3. PATH MISMATCH   — sidecar.path does not resolve to the sibling   (errN++)
//   4. SCHEMA INVALID  — sidecar.Load() returns a validation error      (errN++)
//   5. EMPTY DIRECTIVES— soft warning only, never a failure             (warnN++)
//
// Doctor's existing exit semantics remain intact (0 healthy, 1 warnings,
// 2 errors). Checks 1-4 increment errN; check 5 increments warnN. The check
// group is silent when run outside a project (no decisions/invariants/
// guidelines dirs visible).

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// runSidecarChecks performs the five sidecar checks rooted at projectRoot,
// writing diagnostics to w. Returns counters that the doctor caller folds
// into its existing errN/warnN aggregation.
//
// Returns (errs, warns, ran). ran == false when the project has no artifact
// dirs (fresh sandbox, non-edikt repo) — in that case the caller skips the
// whole group header so the doctor output stays clean.
func runSidecarChecks(projectRoot string, w io.Writer) (errs int, warns int, ran bool) {
	dirs := resolveArtifactDirs(projectRoot)
	dirList := []string{dirs.decisions, dirs.invariants, dirs.guidelines}

	// If none of the artifact dirs exist, this is not an edikt project root.
	// Skip the group entirely rather than emit noise.
	any := false
	for _, d := range dirList {
		if _, err := os.Stat(d); err == nil {
			any = true
			break
		}
	}
	if !any {
		return 0, 0, false
	}

	fmt.Fprintln(w, "  ── Sidecar Health ─────────────────────────────")

	mds, sidecars := collectArtifacts(dirList)

	// Check 1 (orphan): every .edikt.yaml has a sibling .md.
	for _, sp := range sidecars {
		mdPath := strings.TrimSuffix(sp, ".edikt.yaml") + ".md"
		if _, err := os.Stat(mdPath); err != nil {
			fmt.Fprintf(w, "  ERROR: ORPHAN: %s has no corresponding .md\n",
				rel(projectRoot, sp))
			errs++
		}
	}

	// Check 2 (missing): every .md in artifact dirs has a sidebar .edikt.yaml.
	// Skip-list mirrors migrate_sidecars.go: an artifact opts out via
	// `migration: skip` / `documents_legacy_format: true` frontmatter or an
	// `<!-- edikt:migration:skip reason="…" -->` marker (Phase 6 of
	// PLAN-sidecar-review-fixes #16). Those files contain documentation-mention
	// sentinels, not real ones.
	for _, md := range mds {
		if skip, _ := isSkipListed(md); skip {
			continue
		}
		sp := sidecarPathFor(md)
		if _, err := os.Stat(sp); err != nil {
			kind := commandKindForPath(md, dirs)
			fmt.Fprintf(w, "  ERROR: MISSING: %s has no sidecar — run /edikt:%s:compile\n",
				rel(projectRoot, md), kind)
			errs++
		}
	}

	// Checks 3 + 4 + 5 require loading each sidecar.
	for _, sp := range sidecars {
		sc, err := sidecar.Load(sp)
		if err != nil {
			// Check 4 (schema validation): structural / decode / validate failure.
			fmt.Fprintf(w, "  ERROR: SCHEMA INVALID: %s: %v\n", rel(projectRoot, sp), err)
			errs++
			continue
		}

		// Check 3 (path mismatch): sidecar.path, resolved relative to
		// projectRoot, must point at the sibling .md.
		expectedSibling := strings.TrimSuffix(sp, ".edikt.yaml") + ".md"
		resolved := sc.Path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(projectRoot, sc.Path)
		}
		if !samePath(resolved, expectedSibling) {
			fmt.Fprintf(w,
				"  ERROR: PATH MISMATCH: %s points at %q, expected %q\n",
				rel(projectRoot, sp), sc.Path, rel(projectRoot, expectedSibling))
			errs++
		}

		// Check 5 (empty directives): soft warning only.
		if len(sc.Directives) == 0 {
			fmt.Fprintf(w,
				"  WARN: NEEDS REVIEW: %s has no directives in its sidecar — confirm the prose has no rules to extract, or regenerate the sidecar\n",
				rel(projectRoot, sp))
			warns++
		}
	}

	if errs == 0 && warns == 0 {
		fmt.Fprintln(w, "  All sidecar checks passed.")
	}

	return errs, warns, true
}

// collectArtifacts walks artifact dirs once and returns the parent .md set
// and the sidecar .edikt.yaml set, both sorted for deterministic output.
func collectArtifacts(dirs []string) (mds, sidecars []string) {
	for _, d := range dirs {
		if d == "" {
			continue
		}
		_ = filepath.Walk(d, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			name := info.Name()
			switch {
			case strings.HasSuffix(name, ".edikt.yaml"):
				sidecars = append(sidecars, p)
			case strings.HasSuffix(name, ".md"):
				mds = append(mds, p)
			}
			return nil
		})
	}
	sort.Strings(mds)
	sort.Strings(sidecars)
	return mds, sidecars
}

// commandKindForPath maps an .md path to its :compile command namespace.
// "adr" / "invariant" / "guideline" — used in the missing-sidecar hint line
// so the user gets a copy-pasteable command.
func commandKindForPath(mdPath string, dirs artifactDirs) string {
	switch {
	case dirs.decisions != "" && strings.HasPrefix(mdPath, dirs.decisions+string(filepath.Separator)):
		return "adr"
	case dirs.invariants != "" && strings.HasPrefix(mdPath, dirs.invariants+string(filepath.Separator)):
		return "invariant"
	case dirs.guidelines != "" && strings.HasPrefix(mdPath, dirs.guidelines+string(filepath.Separator)):
		return "guideline"
	}
	return "adr"
}

// samePath compares two paths after Clean+EvalSymlinks (best-effort), so
// `./docs/.../X.md` and `docs/.../X.md` compare equal regardless of the
// caller's tree shape.
func samePath(a, b string) bool {
	ca := filepath.Clean(a)
	cb := filepath.Clean(b)
	if ca == cb {
		return true
	}
	if ra, err := filepath.EvalSymlinks(ca); err == nil {
		ca = ra
	}
	if rb, err := filepath.EvalSymlinks(cb); err == nil {
		cb = rb
	}
	return ca == cb
}

// rel returns p relative to root when possible, otherwise p as-is.
func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}
