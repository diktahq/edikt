// removal.go — coverage preview for a proposed paths[] narrowing (F-033).
//
// This is a Go port of the SAME algorithm test/lib/paths-calibration.py
// already implements (matched-file computation, path-token "named" scan) —
// not a second design. It is ported rather than shelled out to because
// `bin/edikt sidecar approve` is a shipped, tier-2 binary every edikt-using
// project runs (ADR-021: tier-2 MUST be Go, no runtime dependency), while  edikt-guard:allow
// paths-calibration.py is dev-only tooling under this repo's own test/lib/
// that does not ship and does not exist in a consumer project's tree. The
// PATH_TOKEN pattern and covered_by_named logic below are deliberately kept
// byte-for-byte equivalent to their Python counterparts so the two never
// silently drift on what counts as "named" (GL-002: one rule, one
// implementation, or the weaker one is the one that gets used).
package pathsproposal

import (
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/globmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// pathTokenRE matches a path-shaped token inside directive prose:
// test/security/sandbox/, templates/hooks/*.sh, tools/edikt/internal/
// hookmatch/. Mirrors paths-calibration.py's PATH_TOKEN exactly.
var pathTokenRE = regexp.MustCompile(`[A-Za-z0-9_.\-]+(?:/[A-Za-z0-9_.\-*]+)+/?`)

// NamedPaths extracts path-shaped tokens from a set of prose strings — the
// directive/prohibition bodies a coverage preview checks a removal against.
// Mirrors paths-calibration.py's named_paths (scanning every string field
// except the glob itself, since the glob is not evidence for itself).
func NamedPaths(texts []string) map[string]bool {
	tokens := map[string]bool{}
	for _, t := range texts {
		for _, m := range pathTokenRE.FindAllString(t, -1) {
			tokens[strings.TrimSuffix(m, "/")] = true
		}
	}
	return tokens
}

// CoveredByNamed reports whether some prose token names this exact file, or
// a directory containing it. Mirrors paths-calibration.py's covered_by_named.
func CoveredByNamed(path string, tokens map[string]bool) bool {
	for t := range tokens {
		if t == path {
			return true
		}
		if strings.ContainsAny(t, "*?") {
			if globmatch.Match(t, path) || globmatch.Match(t+"/*", path) {
				return true
			}
			continue
		}
		if strings.HasPrefix(path, t+"/") {
			return true
		}
	}
	return false
}

// RemovalPreview is the measured cost of removing one glob from a sidecar's
// paths[]: the files that would lose this artifact's write-time delivery,
// and — the case the ceremony refuses outright — which of those are
// literally named in the sidecar's own directive/prohibition text.
type RemovalPreview struct {
	RemovedGlob    string
	RemainingGlobs []string
	Lost           []string // matched by RemovedGlob, NOT matched by any remaining glob
	NamedLost      []string // subset of Lost named in directive/prohibition prose
	FilesEnumerated int
}

// PreviewRemoval computes what removing removedGlob from sc.Paths would
// cost, against the real tree at root. It does not mutate sc or decide
// whether the removal is safe — deciding is the human ceremony's job; this
// only measures, per the same discipline PATH-033's own measurement
// instrument (test/lib/paths-calibration.py) already established: report,
// never gate silently.
func PreviewRemoval(removedGlob string, remainingGlobs []string, sc *sidecar.Sidecar, root string) (RemovalPreview, error) {
	files, err := EnumerateFiles(root)
	if err != nil {
		return RemovalPreview{}, err
	}

	prev := RemovalPreview{
		RemovedGlob:     removedGlob,
		RemainingGlobs:  append([]string(nil), remainingGlobs...),
		FilesEnumerated: len(files),
	}

	for _, f := range files {
		if !globmatch.Match(removedGlob, f) {
			continue
		}
		stillCovered := false
		for _, g := range remainingGlobs {
			if globmatch.Match(g, f) {
				stillCovered = true
				break
			}
		}
		if !stillCovered {
			prev.Lost = append(prev.Lost, f)
		}
	}

	var texts []string
	for _, d := range sc.Directives {
		texts = append(texts, d.Text)
	}
	for _, p := range sc.Prohibitions {
		texts = append(texts, p.Text)
	}
	tokens := NamedPaths(texts)

	for _, f := range prev.Lost {
		if CoveredByNamed(f, tokens) {
			prev.NamedLost = append(prev.NamedLost, f)
		}
	}

	return prev, nil
}
