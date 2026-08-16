// Package pathsproposal validates extraction-time `proposed_paths` entries
// before they are allowed anywhere near a sidecar's enforced `paths:` scope.
//
// SPEC-011 AC-1.8: "Every proposed_paths entry cites evidence, matches at least  edikt-guard:allow
// one existing file, and contains no catch-all glob; an invalid-proposal
// fixture is rejected and a valid one accepted."
//
// The three rules, and why each is here rather than in the JSON schema:
//
//   - EVIDENCE — the schema already requires a non-empty string. That is not
//     enough: a one-character evidence string satisfies the schema and cites
//     nothing, so the proposal can only be rubber-stamped, not reviewed. The
//     minimum here is a length floor, which is a weak instrument and says so
//     in its finding text.
//   - MATCHES AN EXISTING FILE — JSON Schema cannot stat a tree. The validator
//     globs the real repository. Critically it does NOT trust the proposal's
//     own `matched_example` field: the producer of a proposal must not also be
//     the thing that certifies it matched (GL-002 — a verify must not be
//     cheatable by the generator that satisfies it).
//   - NO CATCH-ALL — the schema rejects the four literal spellings (`*`, `**`,
//     `**/*`, `**/**`). It cannot reject the open class: `**/*.go`,
//     `*/**/*.sh`, `*.md` are all anchored nowhere and re-create the ambient
//     "everything is in scope" state this release exists to remove. The rule
//     here is structural — a glob must carry a literal path segment before its
//     first wildcard.
//
// INV-013: Validate refuses to answer at all when it enumerated zero candidate  edikt-guard:allow
// files. Every glob "matching nothing" against an empty tree is a vacuous
// rejection, and the caller must be able to tell that apart from a real one.
package pathsproposal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/globmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// MinEvidenceChars is the floor below which an evidence string is treated as
// absent. It is deliberately low: this catches the empty gesture ("n/a", "-",
// "see prose"), not weak-but-real citations, which are a human review call.
const MinEvidenceChars = 12

// Proposal is one entry of a sidecar's transient `proposed_paths` list.
//
// It is an alias of the decode type, not a second copy of it. A validator that
// carried its own struct would drift from the schema shape the loader accepts,
// and the divergence would be invisible until a field silently stopped being
// validated (GL-002 — two things that must agree are unified).
//
// `matched_example` is carried on the decode type for human review only and is
// NEVER an input to validation: the producer of a proposal must not also be
// what certifies the proposal matched something.
type Proposal = sidecar.ProposedPath

// Finding is one reason one proposal was rejected. A proposal that breaks two
// rules produces two findings — the report names every loss rather than
// stopping at the first.
type Finding struct {
	Index  int    // 0-based position in the proposals slice
	Glob   string // the offending glob, verbatim
	Rule   string // "evidence" | "no-match" | "catch-all" | "shape"
	Reason string // single-line, actionable
}

func (f Finding) String() string {
	return fmt.Sprintf("proposed_paths[%d] %q: %s (%s)", f.Index, f.Glob, f.Reason, f.Rule)
}

// Result is the measured outcome of a validation run.
//
// Checked is the denominator and Files is the size of the tree the match rule
// was evaluated against; a report that omits either cannot be distinguished
// from a run that observed nothing (INV-013).  edikt-guard:allow
type Result struct {
	Checked  int // proposals examined
	Accepted int // proposals with zero findings
	Files    int // candidate files enumerated from the project root
	Findings []Finding
}

// OK reports whether every checked proposal survived.
//
// Zero proposals is OK and is a measured zero — Checked carries the
// denominator so the caller can say "0/0" rather than implying it verified
// something.
func (r Result) OK() bool { return len(r.Findings) == 0 }

// Validate checks every proposal against the file set in files (repo-relative,
// slash-separated paths).
//
// It returns an error — never an all-clear — when files is empty. A glob
// checked against nothing matches nothing, so an empty tree would reject every
// proposal for a reason that says nothing about the proposal. That is an
// unmeasured run, and it must not read as a verdict either way (INV-013).  edikt-guard:allow
func Validate(proposals []Proposal, files []string) (Result, error) {
	if len(files) == 0 {
		return Result{}, fmt.Errorf(
			"UNMEASURED: zero candidate files enumerated — every glob would " +
				"'match nothing' for a reason that is about the file set, not the proposal")
	}

	res := Result{Checked: len(proposals), Files: len(files)}

	for i, p := range proposals {
		before := len(res.Findings)
		glob := strings.TrimSpace(p.Glob)

		switch {
		case glob == "":
			res.Findings = append(res.Findings, Finding{i, p.Glob, "shape",
				"glob is empty"})
		case filepath.IsAbs(glob) || strings.HasPrefix(glob, "~"):
			res.Findings = append(res.Findings, Finding{i, p.Glob, "shape",
				"glob must be repo-relative (no leading / or ~)"})
		case strings.Contains(glob, ".."):
			res.Findings = append(res.Findings, Finding{i, p.Glob, "shape",
				"glob contains a .. traversal segment"})
		case globmatch.LiteralPrefix(glob) == "":
			res.Findings = append(res.Findings, Finding{i, p.Glob, "catch-all",
				"glob is anchored nowhere — it needs a literal path segment before its first wildcard"})
		}

		if len(strings.TrimSpace(p.Evidence)) < MinEvidenceChars {
			res.Findings = append(res.Findings, Finding{i, p.Glob, "evidence",
				fmt.Sprintf("evidence is missing or shorter than %d characters — a proposal without a citation can only be rubber-stamped", MinEvidenceChars)})
		}

		// The match rule runs even for a shape-rejected glob: reporting every
		// reason a proposal failed beats reporting the first one and hiding
		// the rest behind a re-run.
		if glob != "" && !matchesAny(glob, files) {
			res.Findings = append(res.Findings, Finding{i, p.Glob, "no-match",
				fmt.Sprintf("matches none of the %d files under the project root", len(files))})
		}

		if len(res.Findings) == before {
			res.Accepted++
		}
	}

	return res, nil
}

// ValidateAgainstRoot enumerates root's tracked-ish file set and validates
// against it.
func ValidateAgainstRoot(proposals []Proposal, root string) (Result, error) {
	files, err := EnumerateFiles(root)
	if err != nil {
		return Result{}, err
	}
	return Validate(proposals, files)
}

func matchesAny(glob string, files []string) bool {
	for _, f := range files {
		if globmatch.Match(glob, f) {
			return true
		}
	}
	return false
}

// skipDirs are directories excluded from enumeration: build output, VCS
// internals, dependency trees, and edikt's own per-machine state. A glob whose
// only match is a file inside node_modules or .git has not demonstrated it
// scopes any of this project's source.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"__pycache__":  true,
	"dist":         true,
	"build":        true,
}

// EnumerateFiles walks root and returns every regular file as a repo-relative
// slash-separated path, sorted.
//
// Sorted output keeps a Result's findings deterministic across runs on the
// same tree — an unsorted walk would make the "matches none of the N files"
// count stable but the discovery order not, and determinism here is cheap.
func EnumerateFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree is a hole in the instrument, not an
			// absence of files. Surface it rather than quietly enumerating
			// less than the caller thinks.
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("enumerate files under %s: %w", root, err)
	}
	if len(out) == 0 {
		if _, statErr := os.Stat(root); statErr != nil {
			return nil, fmt.Errorf("enumerate files under %s: %w", root, statErr)
		}
	}
	sort.Strings(out)
	return out, nil
}
