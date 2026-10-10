package cmd

// doctor_sidecar.go — Phase 7 of the sidecar architecture plan (PLAN-sidecar-architecture).
//
// Adds a "Sidecar Health" check group to `edikt doctor`, walking the project
// (cwd) for sidecar/parent pairing issues. Five checks per the plan prompt:
//
// 1. ORPHAN — .edikt.yaml without a sibling .md (errN++)
// 2. MISSING — .md without a sibling .edikt.yaml (errN++)
// 3. PATH MISMATCH — sidecar.path does not resolve to the sibling (errN++)
// 4. SCHEMA INVALID — sidecar.Load() returns a validation error (errN++)
// 5. EMPTY DIRECTIVES— soft warning only, never a failure (warnN++)
// 6. ANCHOR DRIFT   — a recorded anchor no longer resolves against the
//                     parent prose (errN++)
//
// Doctor's existing exit semantics remain intact (0 healthy, 1 warnings,
// 2 errors). Checks 1-4 and 6 increment errN; check 5 increments warnN. The
// check group is silent when run outside a project (no decisions/invariants/
// guidelines dirs visible).

import (
	"fmt"
	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
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
	dirs := migrate.ResolveArtifactDirs(projectRoot)
	dirList := []string{dirs.Decisions, dirs.Invariants, dirs.Guidelines}

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

	// Check 1b (stale back-reference, INV-016): a sidecar's `path:` must point at  edikt-guard:allow
	// the artifact it sits beside.
	//
	// `path:` is carried on every sidecar and NOTHING READ IT. Data that exists
	// and is never read cannot detect anything — it is decoration that looks
	// like a link. Validating it costs one comparison and turns the field into a
	// detector for the case the orphan check above cannot see: an artifact and
	// its sidecar renamed TOGETHER, where the pairing still resolves and the
	// recorded path silently describes a file that no longer exists.
	for _, sp := range sidecars {
		mdPath := strings.TrimSuffix(sp, ".edikt.yaml") + ".md"
		if _, err := os.Stat(mdPath); err != nil {
			continue // already reported as an orphan above
		}
		sc, lerr := sidecar.Load(sp)
		if lerr != nil || sc == nil || strings.TrimSpace(sc.Path) == "" {
			continue // unreadable or unset is a different finding
		}
		want := rel(projectRoot, mdPath)
		if filepath.Clean(sc.Path) != filepath.Clean(want) {
			fmt.Fprintf(w, "  ERROR: STALE BACK-REFERENCE: %s records path: %q but sits beside %s\n",
				rel(projectRoot, sp), sc.Path, want)
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
		if skip, _ := migrate.IsSkipListed(md); skip {
			continue
		}
		sp := migrate.SidecarPathFor(md)
		if _, err := os.Stat(sp); err != nil {
			kind := commandKindForPath(md, dirs)
			// The same status filter compile applies (sidecar.StatusExcluded,
			// ADR-020:d03): a proposed or rejected ADR has no sidecar to
			// demand, and telling the user to compile one was wrong twice —
			// compile --check would not have produced it (D46 (a)).
			if excluded, why := sidecar.StatusExcluded(sidecarKindForCommand(kind), md); excluded {
				fmt.Fprintf(w, "  skipped: %s has no sidecar and needs none (%s)\n", rel(projectRoot, md), why)
				continue
			}
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

		// Check 6 (anchor drift, INV-014): every recorded anchor must still  edikt-guard:allow
		// resolve against the parent prose.
		//
		// Checks 1-5 ask whether a sidecar is structurally ATTACHED to an
		// artifact. None of them reads the artifact's words. A sidecar can
		// pass all five while quoting prose that was rewritten underneath
		// it, and the compiled governance then enforces a rule its authority
		// no longer states — which is the condition INV-014 exists to
		// forbid.
		//
		// This reuses VerifyAnchors rather than reimplementing the comparison.
		//
		// It is deliberately STRICTER than the anchor drift `gov compile
		// --check` reports, and the two are not interchangeable. Compile's
		// check tolerates whitespace normalisation; VerifyAnchors requires
		// the quote to be a byte slice of the parent. Measured on a real
		// project: compile reported `anchor drift: 0 stale (86 sidecar(s))`
		// while this check found a reconstructed quote in a sidecar written
		// minutes earlier. Both are correct about their own question —
		// compile asks "will the merge still resolve this", doctor asks "was
		// this quote copied or retyped" — and a retyped quote is stale from
		// the moment it is written even though it still resolves today.
		//
		// Do NOT relax this to match compile. The looser test is what let a
		// reconstructed anchor reach a sealed sidecar unnoticed.
		//
		// Timestamps are deliberately not consulted. A sidecar older than
		// its parent may be perfectly current (the edit was a typo fix) and
		// a sidecar newer than its parent may be stale (the body was
		// reverted). mtime answers a question nobody asked; the anchors
		// answer the one that matters.
		mdBody, rerr := os.ReadFile(expectedSibling)
		if rerr != nil {
			// A missing or unreadable parent is checks 1 and 2's finding.
			// Reporting it again here would double-count one defect.
			continue
		}
		// Compile skips a superseded or migration-skip artifact before its
		// status filter ("existing sidecar ignored; safe to delete"), and that
		// covers shapes the status filter does not: a superseded guideline, an
		// ADR whose body status line reads "Superseded by". Doctor skips the
		// same set, or it tells the user to regenerate a sidecar compile
		// ignores (bok round 6).
		if skip, why := sidecar.IsSkipListed(expectedSibling); skip {
			fmt.Fprintf(w, "  skipped: %s anchors not checked (%s)\n", rel(projectRoot, sp), why)
			continue
		}
		// Status-filtered like compile (D46 (b)): a proposed or rejected
		// artifact's sidecar is not governed, so its anchors are not a
		// finding — but the skip is said, never silent (INV-013).
		if excluded, why := sidecar.StatusExcluded(sidecarKindForCommand(commandKindForPath(expectedSibling, dirs)), expectedSibling); excluded {
			fmt.Fprintf(w, "  skipped: %s anchors not checked (%s)\n", rel(projectRoot, sp), why)
			continue
		}
		if v := sidecar.VerifyAnchors(sc, string(mdBody)); !v.OK() {
			// The denominator travels with the verdict (INV-013): "3 faults"  edikt-guard:allow
			// means nothing without the anchor count it is drawn from.
			fmt.Fprintf(w,
				"  ERROR: ANCHOR DRIFT: %s — %d of %d anchor(s) no longer resolve against %s — run /edikt:%s:compile to regenerate\n",
				rel(projectRoot, sp), len(v.Faults), v.Anchors,
				rel(projectRoot, expectedSibling),
				commandKindForPath(expectedSibling, dirs))
			for _, f := range v.Faults {
				fmt.Fprintf(w, "    %s\n", f)
			}
			errs++
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
func commandKindForPath(mdPath string, dirs migrate.ArtifactDirs) string {
	switch {
	case dirs.Decisions != "" && strings.HasPrefix(mdPath, dirs.Decisions+string(filepath.Separator)):
		return "adr"
	case dirs.Invariants != "" && strings.HasPrefix(mdPath, dirs.Invariants+string(filepath.Separator)):
		return "invariant"
	case dirs.Guidelines != "" && strings.HasPrefix(mdPath, dirs.Guidelines+string(filepath.Separator)):
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

// sidecarKindForCommand maps the command-name kind commandKindForPath
// returns ("adr" | "invariant" | "guideline") onto sidecar's pair kinds.
func sidecarKindForCommand(kind string) string {
	switch kind {
	case "invariant":
		return sidecar.KindInvariant
	case "guideline":
		return sidecar.KindGuideline
	default:
		return sidecar.KindADR
	}
}
