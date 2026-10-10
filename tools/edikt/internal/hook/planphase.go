package hook

// planphase.go — plan-phase-table parsing shared by post-compact.go and
// userpromptsubmit.go. Both bash bodies (post-compact.sh, user-prompt-submit.sh)
// independently carry the identical `grep`/`sed`/`awk` idiom for finding the
// most recent plan file and extracting its in-progress phase's number and
// theme from a pipe-delimited markdown table row; ported once here rather
// than duplicated a third time in Go.

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// planPhaseRe mirrors: grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress'
var planPhaseRe = regexp.MustCompile(`(?i)\| *(Phase )?[0-9]+ *\|.*in[_ -]progress`)

// attemptFormatRe mirrors: echo "$ATTEMPT" | grep -qE '^[0-9]+/[0-9]+$' —
// the "N/N" backward-compat guard post-compact.sh applies to its attempt
// column before trusting it.
var attemptFormatRe = regexp.MustCompile(`^[0-9]+/[0-9]+$`)

// phaseHeadingRe and nextPhaseHeadingRe mirror the sed range
// `/## Phase N[^0-9]/,/## Phase [0-9]/p` post-compact.sh uses to isolate one
// phase's section of a plan file before hunting it for "Context Needed:".
func phaseHeadingRe(phaseNum string) *regexp.Regexp {
	return regexp.MustCompile(`## Phase ` + regexp.QuoteMeta(phaseNum) + `[^0-9]`)
}

var nextPhaseHeadingRe = regexp.MustCompile(`## Phase [0-9]`)

// contextNeededRe mirrors: grep '^ *-' — a bullet line, any amount of
// leading whitespace.
var contextNeededBulletRe = regexp.MustCompile(`^ *-`)

// extractContextNeededBullets mirrors:
//
//	sed -n "/## Phase N[^0-9]/,/## Phase [0-9]/p" "$PLAN" \
//	  | grep -A20 'Context Needed:' | grep '^ *-' | head -5 | sed 's/^ *//'
//
// Best-effort: unreachable from the committed hook-corpus fixtures (no plan
// file in the base tree carries this table shape), documented at each
// caller's own site rather than only here.
func extractContextNeededBullets(planBody []byte, phaseNum string) []string {
	if phaseNum == "" {
		return nil
	}
	lines := strings.Split(string(planBody), "\n")
	startRe := phaseHeadingRe(phaseNum)
	start := -1
	for i, l := range lines {
		if startRe.MatchString(l) {
			start = i
			break
		}
	}
	if start == -1 {
		return nil
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if nextPhaseHeadingRe.MatchString(lines[i]) {
			end = i + 1 // sed's range end is INCLUSIVE of the matching line
			break
		}
	}
	section := lines[start:end]

	ctxIdx := -1
	for i, l := range section {
		if strings.Contains(l, "Context Needed:") {
			ctxIdx = i
			break
		}
	}
	if ctxIdx == -1 {
		return nil
	}
	windowEnd := ctxIdx + 21 // the matched line plus 20 more (`grep -A20`)
	if windowEnd > len(section) {
		windowEnd = len(section)
	}
	var bullets []string
	for _, l := range section[ctxIdx:windowEnd] {
		if contextNeededBulletRe.MatchString(l) {
			bullets = append(bullets, strings.TrimLeft(l, " "))
			if len(bullets) == 5 {
				break
			}
		}
	}
	return bullets
}

// configBase mirrors: grep '^base:' .edikt/config.yaml | awk '{print $2}' | tr -d '"',
// defaulting to "docs".
func configBase(configBytes []byte) string {
	base := grepPrefixFieldTwo(configBytes, "base:")
	base = strings.Trim(base, `"`)
	if base == "" {
		base = "docs"
	}
	return base
}

// configPlanDir mirrors:
//
//	grep -A1 '^plans:' .edikt/config.yaml | grep 'dir:' | awk '{print $2}' | tr -d '"'
//
// defaulting to "$BASE/plans".
func configPlanDir(configBytes []byte, base string) string {
	dir := grepA1SubstringFieldTwo(configBytes, "plans:", "dir:")
	dir = strings.Trim(dir, `"`)
	if dir == "" {
		dir = filepath.Join(base, "plans")
	}
	return dir
}

// grepPrefixFieldTwo returns the whitespace-split 2nd field of the first line
// starting with prefix — `grep '^prefix' file | awk '{print $2}'`.
func grepPrefixFieldTwo(b []byte, prefix string) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, prefix) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
			return ""
		}
	}
	return ""
}

// grepA1SubstringFieldTwo returns the whitespace-split 2nd field of the first
// line (within the matched line and the one immediately after it) containing
// substr — `grep -A1 '^prefix' file | grep 'substr' | awk '{print $2}'`.
func grepA1SubstringFieldTwo(b []byte, prefix, substr string) string {
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		window := []string{line}
		if i+1 < len(lines) {
			window = append(window, lines[i+1])
		}
		for _, wl := range window {
			if strings.Contains(wl, substr) {
				fields := strings.Fields(wl)
				if len(fields) >= 2 {
					return fields[1]
				}
				return ""
			}
		}
		return ""
	}
	return ""
}

// mostRecentPlanFile mirrors the `stat`-sorted `ls -t $PLAN_DIR/*.md | head -1`
// (bash's own body branches on BSD vs GNU stat for this; os.Stat needs neither).
// Returns "" if planDir is not a directory or holds no *.md file.
func mostRecentPlanFile(planDir string) string {
	fi, err := os.Stat(planDir)
	if err != nil || !fi.IsDir() {
		return ""
	}
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return ""
	}
	type cand struct {
		path    string
		modTime int64
	}
	var candidates []cand
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		candidates = append(candidates, cand{path: filepath.Join(planDir, e.Name()), modTime: info.ModTime().UnixNano()})
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].modTime > candidates[j].modTime })
	return candidates[0].path
}

// planPhaseRow finds the first in-progress phase row and splits it into
// pipe-delimited fields — mirrors:
//
//	grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' "$PLAN" | head -1
//	echo "$PHASE" | sed 's/|/\n/g'
//
// fields[0] is whatever precedes the first '|' (empty for a row starting
// with '|'); fields[1] is the phase-number column bash calls PHASE_NUM;
// fields[2] is the column bash calls PHASE_THEME (a 4-column
// Phase|Status|Attempt|Updated table's Status column, despite the variable's
// name — ported literally, not "fixed", per this migration's own charter).
func planPhaseRow(planBody []byte) (fields []string, ok bool) {
	sc := bufio.NewScanner(bytes.NewReader(planBody))
	for sc.Scan() {
		line := sc.Text()
		if planPhaseRe.MatchString(line) {
			return strings.Split(line, "|"), true
		}
	}
	return nil, false
}

// phaseField reads a 1-indexed pipe field (bash's `sed -n 'Np'` over the
// newline-split fields is 1-indexed), trimmed of surrounding spaces, or ""
// if the row has fewer fields.
func phaseField(fields []string, n int) string {
	if n < 1 || n > len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[n-1])
}

// phaseNumDigits mirrors: tr -d ' ' | grep -oE '[0-9]+' — the first run of
// digits in the field, spaces removed first.
func phaseNumDigits(field string) string {
	noSpace := strings.ReplaceAll(field, " ", "")
	var digits strings.Builder
	seen := false
	for _, r := range noSpace {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			seen = true
		} else if seen {
			break
		}
	}
	return digits.String()
}

// planNameFromHeading mirrors: head -5 "$PLAN" | grep '^# ' | head -1 | sed 's/^# //',
// counted from the end of a leading YAML frontmatter block. Plans written from
// /edikt:sdlc:plan's template open with frontmatter that pushes the heading
// well past line 5 (issue #13); with no heading, the frontmatter's id: names
// the plan.
func planNameFromHeading(planBody []byte) string {
	body := strings.ReplaceAll(string(planBody), "\r\n", "\n")
	frontmatter := ""
	if strings.HasPrefix(body, "---\n") {
		if end := strings.Index(body[4:], "\n---\n"); end != -1 {
			frontmatter = body[4 : 4+end]
			body = strings.TrimLeft(body[4+end+len("\n---\n"):], "\n")
		}
	}
	sc := bufio.NewScanner(strings.NewReader(body))
	for i := 0; i < 5 && sc.Scan(); i++ {
		line := sc.Text()
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	if m := planFrontmatterIDRe.FindStringSubmatch(frontmatter); m != nil {
		return strings.Trim(strings.TrimSpace(m[1]), `"'`)
	}
	return ""
}

// planFrontmatterIDRe matches a top-level `id:` key in plan frontmatter.
var planFrontmatterIDRe = regexp.MustCompile(`(?m)^id:[ \t]*(\S.*)$`)
