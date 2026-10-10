package gov

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
)

// A CITATION EITHER RESOLVES OR IT IS A FINDING.
//
// A governance record cites other records, commits and paths. Those citations
// are what a later reader follows to understand why a rule exists — and they
// are written by whoever drafted the record, which increasingly means a model
// that had the ids in front of it. A citation to a record that does not exist
// looks exactly like a citation to one that does.
//
// This verb resolves every citation in an artifact and reports the ones that
// do not. It is deterministic and it dispatches nothing: the auditor's
// judgement is a separate step, and this half must be true before any of that
// judgement is worth reading.
//
// EVERY VALUE IS VALIDATED BEFORE IT IS LOOKED UP. Artifact ids, commit
// hashes and paths all reach an argv element or the filesystem, and all three
// come out of prose. `../../etc/passwd` is a refusal, never a resolved
// citation.
var (
	// Artifact citations: ADR-900, INV-900, GL-900, SPEC-900, PRD-900.
	citationArtifactRe = regexp.MustCompile(`\b((?:ADR|INV|GL|SPEC|PRD|PLAN|BRAIN|DISCOVERY)-[A-Za-z0-9]*-?\d{3,})\b`)
	// Commit citations: a 7–40 hex run introduced by a word that makes it a
	// commit reference rather than an incidental hex string.
	citationCommitRe = regexp.MustCompile(`(?i)\b(?:commit|sha|revision|at)\s+([0-9a-fA-F]{7,40})\b`)
	// Path citations: backticked repo-relative paths with a slash and an
	// extension, which is what a governance record cites when it names a file.
	citationPathRe = regexp.MustCompile("`([\\w.-]+(?:/[\\w.-]+)+\\.[A-Za-z0-9]+)`")
)

// citation is one reference found in an artifact's prose.
type citation struct {
	kind  string // artifact | commit | path
	value string
	line  int
}

// unresolved is a citation that did not resolve, and WHAT WAS LOOKED FOR — a
// finding that does not say where it looked sends the reader hunting.
type unresolved struct {
	citation
	lookedFor string
	reason    string
}

var auditCiteCmd = &cobra.Command{
	Use:   "audit-cite <artifact>",
	Short: "Resolve every citation in a governance artifact; report the ones that do not resolve",
	Long: `Resolves every artifact id, commit hash and repository path an artifact
cites, and reports each one that does not resolve with what was looked for.

Deterministic and dispatches nothing: the auditor's judgement is a separate
step, and this half has to be true before that judgement is worth reading.

Every value is validated before it is looked up — ids, hashes and paths all
reach an argv element or the filesystem, and all three come out of prose. A
path that resolves outside the repository is a refusal, never a citation.

Exit codes:
  0 — every citation resolved
  1 — at least one did not; each is listed with what was looked for
  3 — usage, or the artifact itself cannot be read`,
	Args: cobra.ExactArgs(1),
	// Bridge *exitErr to os.Exit — without this, every exitErr this
	// command returns collapses to cobra's generic exit 1, and the
	// documented exit-1/exit-3 distinction (unresolved vs. usage) never
	// actually reaches the caller.
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runAuditCite(cmd, args)
		exitFromExitErr(err)
		return err
	},
}

func runAuditCite(cmd *cobra.Command, args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	artifactPath, err := resolveArtifactArg(root, args[0])
	if err != nil {
		return &exitErr{code: 3, msg: err.Error()}
	}

	cites, err := scanCitations(artifactPath)
	if err != nil {
		return &exitErr{code: 3, msg: fmt.Sprintf("read %s: %v", artifactPath, err)}
	}

	dirs := migrate.ResolveArtifactDirs(root)
	known := knownArtifactIDs(dirs)

	var bad []unresolved
	for _, c := range cites {
		switch c.kind {
		case "artifact":
			if err := idvalidate.ArtifactID(c.value); err != nil {
				bad = append(bad, unresolved{c, "a valid artifact id", err.Error()})
				continue
			}
			if !known[strings.ToUpper(c.value)] {
				bad = append(bad, unresolved{c,
					fmt.Sprintf("a file matching %s-* under %s, %s or %s",
						c.value, rel(root, dirs.Decisions), rel(root, dirs.Invariants), rel(root, dirs.Guidelines)),
					"no artifact with that id is in the corpus"})
			}
		case "commit":
			if err := idvalidate.CommitHash(c.value); err != nil {
				bad = append(bad, unresolved{c, "a git object id (7–40 hex)", err.Error()})
				continue
			}
			if !commitExists(root, strings.ToLower(c.value)) {
				bad = append(bad, unresolved{c, "git cat-file -e " + strings.ToLower(c.value),
					"no such object in this repository"})
			}
		case "path":
			if _, err := idvalidate.RepoRelPath(root, c.value); err != nil {
				bad = append(bad, unresolved{c, "a repository-relative path that exists", err.Error()})
			}
		}
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "audit-cite %s — %d citation(s) checked\n", rel(root, artifactPath), len(cites))
	if len(cites) == 0 {
		// A record citing nothing is not a finding, and it is not a clean
		// bill either: say which it is, so silence is never read as a pass.
		fmt.Fprintln(out, "  no citations found — nothing was resolved (this is a measurement, not a pass)")
		return nil
	}
	if len(bad) == 0 {
		fmt.Fprintf(out, "  every citation resolved\n")
		return nil
	}
	sort.SliceStable(bad, func(i, j int) bool { return bad[i].line < bad[j].line })
	for _, b := range bad {
		fmt.Fprintf(out, "  UNRESOLVED %s:%d  %s %q\n", rel(root, artifactPath), b.line, b.kind, b.value)
		fmt.Fprintf(out, "      looked for: %s\n", b.lookedFor)
		fmt.Fprintf(out, "      %s\n", b.reason)
		// D118 (finding 9): an unresolved citation was read as "the authority
		// is missing" and fixed forward, when the citation was standing in
		// for an assertion that should not have been made at all. Naming
		// only the additive remedy teaches that reading; the subtractive one
		// is stated every time, alongside it, never instead of it.
		fmt.Fprintf(out, "      remedy: either correct the citation, OR remove the assertion it supports — an unresolved citation is as often a sign the claim doesn't belong here as it is a typo.\n")
	}
	return &exitErr{code: 1, msg: fmt.Sprintf("%d citation(s) did not resolve", len(bad))}
}

// resolveArtifactArg accepts an id or a path and returns the artifact file.
func resolveArtifactArg(root, arg string) (string, error) {
	if strings.HasSuffix(arg, ".md") {
		p, err := idvalidate.RepoRelPath(root, arg)
		if err != nil {
			// An absolute path inside the repo is a legitimate way to name a
			// file on the command line; only refuse what leaves the tree.
			if filepath.IsAbs(arg) {
				if abs, aerr := filepath.EvalSymlinks(arg); aerr == nil && strings.HasPrefix(abs, root) {
					return abs, nil
				}
			}
			return "", err
		}
		return p, nil
	}
	if err := idvalidate.ArtifactID(arg); err != nil {
		return "", err
	}
	dirs := migrate.ResolveArtifactDirs(root)
	for _, dir := range []string{dirs.Decisions, dirs.Invariants, dirs.Guidelines} {
		matches, _ := filepath.Glob(filepath.Join(dir, strings.ToUpper(arg)+"-*.md"))
		if len(matches) > 0 {
			sort.Strings(matches)
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("no artifact matching %q under the corpus directories", arg)
}

// scanCitations reads an artifact line by line — a governance record is prose
// of unbounded length and this walks it once.
func scanCitations(path string) ([]citation, error) {
	f, err := os.Open(path) //nolint:gosec // validated above
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []citation
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	self := selfIDOf(path)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		for _, m := range citationArtifactRe.FindAllStringSubmatch(line, -1) {
			// A record naming ITSELF is not a citation.
			if strings.EqualFold(m[1], self) {
				continue
			}
			out = append(out, citation{"artifact", m[1], n})
		}
		for _, m := range citationCommitRe.FindAllStringSubmatch(line, -1) {
			out = append(out, citation{"commit", m[1], n})
		}
		for _, m := range citationPathRe.FindAllStringSubmatch(line, -1) {
			out = append(out, citation{"path", m[1], n})
		}
	}
	return out, sc.Err()
}

func selfIDOf(path string) string {
	base := filepath.Base(path)
	if m := citationArtifactRe.FindStringSubmatch(base); m != nil {
		return m[1]
	}
	return ""
}

// knownArtifactIDs is every id the corpus carries, from filenames — the same
// resolution a reader performs by looking in the directory.
func knownArtifactIDs(dirs migrate.ArtifactDirs) map[string]bool {
	out := map[string]bool{}
	for _, dir := range []string{dirs.Decisions, dirs.Invariants, dirs.Guidelines, dirs.Prds, dirs.Specs} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if m := citationArtifactRe.FindStringSubmatch(e.Name()); m != nil {
				out[strings.ToUpper(m[1])] = true
			}
		}
	}
	return out
}

// commitExists asks git, with the hash already validated as hex.
func commitExists(root, hash string) bool {
	c := exec.Command("git", "cat-file", "-e", hash+"^{commit}")
	c.Dir = root
	return c.Run() == nil
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

func init() { Cmd.AddCommand(auditCiteCmd) }
