package cmd

// doctor_binary_rung.go — SPEC-017 SR-001/SR-002/SR-003, PRD-009 FR-029.
//
// WHICH BINARY ANSWERS THIS REPO'S HOOKS. Observed 2026-08-27: edikt-dev is a
// trusted root with a local bin/edikt, so it answers via rung 2 — including
// ADR-074's unreleased exemption. edikt-langfuse, a WORKTREE OF THE SAME
// REPOSITORY, is absent from trusted-roots.json and has no local bin/, so it
// falls to rung 3: the released launcher, which predates ADR-074 and denies at
// 9,075 characters. Same repo, same machine, same `current` — two checkouts
// enforcing different governance, and nothing surfaced it.
//
// That is not a release oversight; ADR-074 is unreleased, so its absence from
// the released binary is correct. The defect is the SILENCE.
//
// REPORTS WHAT IT MEASURED, NEVER WHAT THAT IMPLIES (SPEC-017 SSP-001). It says
// "rung 2, <path>", not "the exemption is active". Probing a binary for a symbol
// and reporting a governance decision as present is the error this check exists
// to prevent — it measures the binary someone chose, not the one the ladder
// resolves.
//
// The ladder is REPRODUCED from the shim, not re-derived:
//
//	1. ${PWD}/.edikt/bin/edikt   ] trust-gated on $(pwd -P) ∈ trusted-roots.json
//	2. ${PWD}/bin/edikt          ]
//	3. ${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/bin/edikt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// binaryRung is the resolved answer, with its own denominator: every candidate
// considered, so a reader can see what was swept rather than only the winner.
type binaryRung struct {
	Rung       int
	Path       string
	Candidates []rungCandidate
	Trusted    bool
	TrustFile  string
}

type rungCandidate struct {
	Rung       int
	Path       string
	Exists     bool
	TrustGated bool
	Skipped    string // why this candidate did not win, empty if it did
}

// resolveBinaryRung walks the shim's ladder for projectRoot.
//
// trustedRoots is read from EDIKT_HOME's state; a root is trusted when it
// appears as a key. An unreadable file means NOT trusted — an unreadable trust
// store must not grant trust (INV-011).
func resolveBinaryRung(projectRoot, ediktRoot string) binaryRung {
	res := binaryRung{Rung: 0}
	res.TrustFile = filepath.Join(ediktRoot, "state", "trusted-roots.json")
	res.Trusted = rootIsTrusted(res.TrustFile, projectRoot)

	cands := []struct {
		rung  int
		path  string
		gated bool
	}{
		{1, filepath.Join(projectRoot, ".edikt", "bin", "edikt"), true},
		{2, filepath.Join(projectRoot, "bin", "edikt"), true},
		{3, filepath.Join(ediktRoot, "bin", "edikt"), false},
	}

	for _, c := range cands {
		rc := rungCandidate{Rung: c.rung, Path: c.path, TrustGated: c.gated}
		info, err := os.Stat(c.path)
		rc.Exists = err == nil && info.Mode().IsRegular()
		switch {
		case !rc.Exists:
			rc.Skipped = "absent"
		case c.gated && !res.Trusted:
			rc.Skipped = "root not in trusted-roots.json"
		case res.Rung == 0:
			res.Rung, res.Path = c.rung, c.path
		default:
			rc.Skipped = "a lower rung already answered"
		}
		res.Candidates = append(res.Candidates, rc)
	}
	return res
}

func rootIsTrusted(trustFile, projectRoot string) bool {
	raw, err := os.ReadFile(trustFile) //nolint:gosec // fixed state path
	if err != nil {
		return false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		resolved = projectRoot
	}
	for k := range m {
		if k == projectRoot || k == resolved {
			return true
		}
	}
	return false
}

// newestGoSourceMTime is the STALENESS PROXY for SR-003. It reports that the
// binary predates its source — never that the two are inequivalent. A rebuild
// that changed nothing resets it, and that is stated wherever it is printed.
func newestGoSourceMTime(dir string) (time.Time, int, bool) {
	var newest time.Time
	n := 0
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		n++
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if err != nil || n == 0 {
		return time.Time{}, 0, false
	}
	return newest, n, true
}

// runBinaryRungCheck reports the resolved rung and trust state. Returns a warning
// count. deep enables the SR-003 staleness proxy.
func runBinaryRungCheck(projectRoot, ediktRoot string, deep bool, out io.Writer) int {
	r := resolveBinaryRung(projectRoot, ediktRoot)

	if r.Rung == 0 {
		fmt.Fprintf(out, "  ⚠ no edikt binary on any rung — hooks cannot run\n")
		for _, c := range r.Candidates {
			fmt.Fprintf(out, "      rung %d %s — %s\n", c.Rung, c.Path, c.Skipped)
		}
		return 1
	}

	warns := 0
	fmt.Fprintf(out, "  hook binary: rung %d — %s\n", r.Rung, r.Path)
	fmt.Fprintf(out, "      swept %d candidate(s); root %s in trusted-roots.json\n",
		len(r.Candidates), map[bool]string{true: "IS", false: "is NOT"}[r.Trusted])

	// SR-002: a local build that is silently ignored.
	for _, c := range r.Candidates {
		if c.Exists && c.TrustGated && c.Skipped == "root not in trusted-roots.json" {
			fmt.Fprintf(out, "  ⚠ %s exists but this root is untrusted — your own build is "+
				"being ignored; hooks answer from rung %d instead\n", c.Path, r.Rung)
			warns++
		}
	}

	// A worktree answering from the installed binary is the observed instance.
	if r.Rung == 3 {
		fmt.Fprintf(out, "  ⚠ hooks answer from the installed launcher, not this "+
			"checkout — governance here may differ from a trusted checkout of the "+
			"same repository\n")
		warns++
	}

	if !deep {
		return warns
	}

	// SR-003: staleness PROXY, and it says so.
	if r.Rung == 3 {
		fmt.Fprintf(out, "      staleness: not checked — rung 3 is a released "+
			"launcher, not built from this source\n")
		return warns
	}
	srcDir := filepath.Join(projectRoot, "tools", "edikt")
	newest, n, ok := newestGoSourceMTime(srcDir)
	if !ok {
		fmt.Fprintf(out, "      staleness: UNMEASURED — no .go sources under %s\n", srcDir)
		return warns
	}
	info, err := os.Stat(r.Path)
	if err != nil {
		fmt.Fprintf(out, "      staleness: UNMEASURED — cannot stat %s\n", r.Path)
		return warns
	}
	if info.ModTime().Before(newest) {
		fmt.Fprintf(out, "  ⚠ binary predates its source (%d .go file(s) newer) — "+
			"STALENESS PROXY, not an equivalence claim; a no-op rebuild resets it\n", n)
		warns++
	} else {
		fmt.Fprintf(out, "      staleness: binary is newer than all %d .go source(s) "+
			"(proxy only)\n", n)
	}
	return warns
}
