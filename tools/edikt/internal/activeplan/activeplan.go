// Package activeplan resolves "the active plan" — the same question
// commands/status.md's dashboard and `bin/edikt plan active` answer,
// extracted here so a second caller (gov prune-candidates, SR-006)
// does not reimplement the scoping rule (GL-002 "one rule, one
// implementation").
//
// THE SCOPING IS THE WHOLE ALGORITHM. A plan body contains several
// tables — cost estimates, phase prompts, dependency lists — and rows
// in those look exactly like progress rows. Counting them made
// finished plans look active, which is why the search is bounded to
// the `## Progress` section rather than run over the whole file. A
// plan with no Progress heading contributes nothing rather than being
// treated as fully complete.
package activeplan

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	completeRe = regexp.MustCompile(`(?i)\b(done|pass(ed)?|complete(d)?)\b|✅`)
	rowRe      = regexp.MustCompile(`^\| *\d+[a-z]?\s*\|`)
	progressRe = regexp.MustCompile(`(?mi)^##+ +Progress\b`)
	nextHeadRe = regexp.MustCompile(`(?m)^##+ +`)
)

// Resolve returns the path of the most recently modified PLAN-*.md
// under dir whose Progress table still has at least one row that is
// not marked complete. Returns "" (no error) when every plan is
// complete or dir holds no plans — that is a real answer ("nothing is
// active"), not a failure. Returns an error only when dir cannot be
// read, so a caller can tell "nothing to do" from "could not look".
func Resolve(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type cand struct {
		path string
		mod  int64
	}
	var files []cand
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "PLAN-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		if strings.Contains(name, ".SUPERSEDED") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		files = append(files, cand{filepath.Join(dir, name), info.ModTime().UnixNano()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod > files[j].mod })

	for _, f := range files {
		body, rerr := os.ReadFile(f.path) //nolint:gosec // path built from ReadDir
		if rerr != nil {
			continue
		}
		rows := ProgressRows(string(body))
		if len(rows) == 0 {
			continue
		}
		for _, r := range rows {
			if !completeRe.MatchString(r) {
				return f.path, nil
			}
		}
	}
	return "", nil
}

// ProgressRows returns the table rows inside the `## Progress` section
// only — see the package doc for why the bound matters.
func ProgressRows(body string) []string {
	m := progressRe.FindStringIndex(body)
	if m == nil {
		return nil
	}
	rest := body[m[1]:]
	if nxt := nextHeadRe.FindStringIndex(rest); nxt != nil {
		rest = rest[:nxt[0]]
	}
	var out []string
	for _, ln := range strings.Split(rest, "\n") {
		if rowRe.MatchString(ln) {
			out = append(out, ln)
		}
	}
	return out
}

// Slug strips the PLAN- prefix and .md suffix from a plan file path,
// matching post-flight-report.schema.json's meta.plan convention
// (basename of PLAN-<slug>.md without extension, prefix stripped).
func Slug(planPath string) string {
	base := filepath.Base(planPath)
	base = strings.TrimSuffix(base, ".md")
	return strings.TrimPrefix(base, "PLAN-")
}
