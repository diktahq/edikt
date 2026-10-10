package cmd

// stops.go — `edikt stops scan`: the stop-point classification scan.
//
// A stop point is a place a command file waits for a human. Each one is
// declared in the command's YAML frontmatter under `stops:` and marked in the
// prose by an anchor comment `<!-- stop: <id> -->`.
//
// The scan enumerates BOTH sides and compares them:
//
//   prose side        every match of the ONE pinned regex in
//                     test/lib/stop-point-patterns.txt, over command BODIES
//                     (frontmatter excluded — `allowed-tools: AskUserQuestion`
//                     is a tool declaration, not a place anything waits).
//   declaration side  the `stops:` entries parsed out of the frontmatter.
//
// Reading only `stops:` would make the scan enumerate its own output: a stop
// point somebody forgot to declare is exactly the one the declaration list
// cannot see. So the prose is enumerated independently and the scan fails in
// BOTH directions — a prose hit with no anchor, an anchor with no
// declaration, a declaration with no anchor.
//
// Two further rules the scan applies:
//
//   - class `duplicate` is a rejection, not a declaration. It names what was
//     REMOVED: the prompts that restated a control the host's permission
//     system and the managed-region guard already apply. A surviving
//     `duplicate` entry fails the scan by name.
//   - the enumerated counts are ratcheted against test/baselines/stop-points.txt.
//     A count of zero is reported UNMEASURED and exits non-zero: an instrument
//     that matched nothing has said nothing about the corpus, and must never
//     render as a clean scan.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	defaultStopsDir      = "commands"
	defaultStopsPatterns = "test/lib/stop-point-patterns.txt"
	defaultStopsBaseline = "test/baselines/stop-points.txt"
)

// stopDecl is one `stops:` entry. The shape is published as
// templates/schemas/command-stops.v1.schema.json; this struct is the reader,
// not the contract — `stops scan --json` emits the blocks so the schema itself
// can be the validator rather than a second copy of its rules living here.
type stopDecl struct {
	ID        string   `yaml:"id"        json:"id"`
	Class     string   `yaml:"class"     json:"class"`
	Default   *string  `yaml:"default"   json:"default"`
	OnSilence string   `yaml:"on_silence" json:"on_silence"`
	Prompt    string   `yaml:"prompt"    json:"prompt"`
	Options   []string `yaml:"options,omitempty" json:"options,omitempty"`

	declLine int // 1-based line of this entry's `- id:` in the file
}

type stopsFile struct {
	Path  string     `json:"path"`
	Stops []stopDecl `json:"stops"`
}

type stopsScanResult struct {
	Files []stopsFile `json:"files"`

	FilesScanned   int
	FilesWithStops int
	Declarations   int
	Anchors        int
	ProseMatches   int

	Findings []string
}

var stopsCmd = &cobra.Command{
	Use:   "stops",
	Short: "Stop-point declarations in command files",
	Long: `Read and check the stop-point declarations command files carry.

A stop point is a place a command waits for a human. Each is declared in the
command's frontmatter under ` + "`stops:`" + ` and anchored in the prose by
` + "`<!-- stop: <id> -->`" + `.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

func newStopsScanCmd() *cobra.Command {
	var (
		dir           string
		patternsPath  string
		baselinePath  string
		emitJSON      bool
		classesReport bool
		noBaseline    bool
	)
	c := &cobra.Command{
		Use:   "scan",
		Short: "Enumerate prose stop points and their declarations, and compare them",
		Long: `Enumerate every stop point in the command files from two independent
sides — the prose, via the one pinned regex, and the frontmatter declarations —
and fail when either side is missing its counterpart, naming file and line.

Exit codes:
  0  every prose stop point is declared, every declaration is anchored
  1  findings (named on stdout), or the enumerated count is UNMEASURED
  2  the scan could not run (unreadable pattern file, unparseable frontmatter)`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			re, err := loadStopPatterns(patternsPath)
			if err != nil {
				return exitCode2(err)
			}
			res, err := scanStopPoints(dir, re)
			if err != nil {
				return exitCode2(err)
			}
			if !noBaseline {
				res.applyBaseline(baselinePath)
			}
			out := cmd.OutOrStdout()
			if emitJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(struct {
					Files []stopsFile `json:"files"`
				}{res.Files}); err != nil {
					return exitCode2(err)
				}
				return nil
			}
			ok := res.report(out, classesReport)
			if !ok {
				return errStopsFindings
			}
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", defaultStopsDir, "directory of command files to scan")
	c.Flags().StringVar(&patternsPath, "patterns", defaultStopsPatterns, "file holding the one pinned prose regex")
	c.Flags().StringVar(&baselinePath, "baseline", defaultStopsBaseline, "ratchet file for the enumerated counts")
	c.Flags().BoolVar(&emitJSON, "json", false, "emit the declaration blocks as JSON (for schema validation) and exit 0")
	c.Flags().BoolVar(&classesReport, "classes-report", false, "print the per-class report")
	c.Flags().BoolVar(&noBaseline, "no-baseline", false, "skip the count ratchet (the UNMEASURED rule still applies)")
	return c
}

// errStopsFindings carries exit 1 with an empty message — the findings are
// already on stdout, named file:line, and a second generic error line would
// only push them off a reader's screen.
var errStopsFindings = &exitCodeError{code: 1, msg: ""}

// exitCode2 is "the scan could not run", kept distinct from "the scan ran and
// found something". A caller that cannot tell those apart reads an unreadable
// pattern file as a clean corpus.
func exitCode2(err error) error { return &exitCodeError{code: 2, msg: err.Error()} }

// ── the pinned regex ────────────────────────────────────────────────────────

// loadStopPatterns reads the ONE regex. Everything before the first
// non-comment non-blank line is commentary; a SECOND regex line is an error
// rather than a silently ignored one — two regexes would mean the file no
// longer says which one the scan used.
func loadStopPatterns(path string) (*regexp.Regexp, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
	if err != nil {
		return nil, fmt.Errorf("pinned pattern file unreadable: %w", err)
	}
	var found string
	var extra int
	for _, ln := range strings.Split(string(raw), "\n") {
		t := strings.TrimRight(ln, "\r")
		if strings.TrimSpace(t) == "" || strings.HasPrefix(strings.TrimSpace(t), "#") {
			continue
		}
		if found == "" {
			found = t
			continue
		}
		extra++
	}
	if found == "" {
		return nil, fmt.Errorf("%s holds no regex line — the scan has nothing to enumerate with", path)
	}
	if extra > 0 {
		return nil, fmt.Errorf("%s holds %d regex line(s) beyond the first; the scan reads ONE pinned regex", path, extra+1)
	}
	re, err := regexp.Compile(found)
	if err != nil {
		return nil, fmt.Errorf("%s: regex does not compile: %w", path, err)
	}
	if re.SubexpIndex("id") < 0 {
		return nil, fmt.Errorf("%s: the pinned regex has no named group `id` — the anchor side cannot be read", path)
	}
	return re, nil
}

// ── the scan ────────────────────────────────────────────────────────────────

var stopsHeadingRe = regexp.MustCompile(`^#{1,6}\s`)

func scanStopPoints(dir string, re *regexp.Regexp) (*stopsScanResult, error) {
	var paths []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot walk %s: %w", dir, err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .md files under %s — there was no subject to scan", dir)
	}

	res := &stopsScanResult{FilesScanned: len(paths)}
	for _, p := range paths {
		if err := res.scanOne(p, re); err != nil {
			return nil, err
		}
	}
	sort.Strings(res.Findings)
	return res, nil
}

func (r *stopsScanResult) scanOne(path string, re *regexp.Regexp) error {
	raw, err := os.ReadFile(path) //nolint:gosec // walked path
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")

	decls, bodyStart, err := parseStopDecls(path, lines)
	if err != nil {
		return err
	}
	if len(decls) > 0 {
		r.FilesWithStops++
		r.Files = append(r.Files, stopsFile{Path: path, Stops: decls})
		r.Declarations += len(decls)
	}

	declByID := map[string]*stopDecl{}
	for i := range decls {
		d := &decls[i]
		if _, dup := declByID[d.ID]; dup {
			r.findf("%s:%d: two `stops:` entries share id %q — an id is what an anchor names, so it has to be unique in the file", path, d.declLine, d.ID)
			continue
		}
		declByID[d.ID] = d
		if d.Class == "duplicate" {
			r.findf("%s:%d: stop %q is class `duplicate` — that class names what was REMOVED. Delete the prompt, state what the command writes, and drop the entry", path, d.declLine, d.ID)
		}
	}

	// Prose side. Body only: the frontmatter's `allowed-tools` list is a tool
	// declaration, not a place anything waits.
	anchorSeen := map[string]int{}   // id → line
	anchorHasProse := map[string]bool{}
	curAnchor, curAnchorLine := "", 0
	for i := bodyStart; i < len(lines); i++ {
		lineNo := i + 1
		ln := lines[i]
		if stopsHeadingRe.MatchString(ln) {
			curAnchor, curAnchorLine = "", 0
		}
		for _, m := range re.FindAllStringSubmatchIndex(ln, -1) {
			idIdx := re.SubexpIndex("id")
			var id string
			if 2*idIdx+1 < len(m) && m[2*idIdx] >= 0 {
				id = ln[m[2*idIdx]:m[2*idIdx+1]]
			}
			if id != "" {
				r.Anchors++
				if prev, dup := anchorSeen[id]; dup {
					r.findf("%s:%d: anchor %q repeats (first at line %d) — one anchor per stop point, or the count double-reads it", path, lineNo, id, prev)
				} else {
					anchorSeen[id] = lineNo
				}
				if _, declared := declByID[id]; !declared {
					r.findf("%s:%d: anchor %q has no `stops:` declaration in this file's frontmatter", path, lineNo, id)
				}
				curAnchor, curAnchorLine = id, lineNo
				continue
			}
			// A prose hit.
			r.ProseMatches++
			if curAnchor == "" {
				r.findf("%s:%d: stop point in the prose (%q) with no `<!-- stop: ... -->` anchor above it in this section", path, lineNo, strings.TrimSpace(truncate(ln, 70)))
				continue
			}
			anchorHasProse[curAnchor] = true
			_ = curAnchorLine
		}
	}

	for i := range decls {
		d := &decls[i]
		if _, ok := anchorSeen[d.ID]; !ok {
			r.findf("%s:%d: `stops:` declares %q but no `<!-- stop: %s -->` anchor appears in the body", path, d.declLine, d.ID, d.ID)
		}
	}
	return nil
}

func (r *stopsScanResult) findf(format string, a ...any) {
	r.Findings = append(r.Findings, fmt.Sprintf(format, a...))
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// parseStopDecls returns the `stops:` entries and the 0-based index of the
// first BODY line. A file with no frontmatter has bodyStart 0 and no
// declarations.
func parseStopDecls(path string, lines []string) ([]stopDecl, int, error) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, 0, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, 0, nil // unterminated frontmatter: treat the whole file as body
	}
	fmLines := lines[1:end]
	var doc struct {
		Stops []stopDecl `yaml:"stops"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &doc); err != nil {
		return nil, 0, fmt.Errorf("%s: frontmatter is not parseable YAML: %w", path, err)
	}
	// Attach the source line of each entry so a finding can name file:line.
	// Matched by `- id: <value>` in frontmatter order, which is the order
	// yaml.Unmarshal preserved.
	idLineRe := regexp.MustCompile(`^\s*-\s+id:\s*"?([A-Za-z0-9][A-Za-z0-9._:-]*)"?\s*$`)
	seq := map[string]int{}
	for i, ln := range fmLines {
		if m := idLineRe.FindStringSubmatch(ln); m != nil {
			if _, ok := seq[m[1]]; !ok {
				seq[m[1]] = i + 2 // +1 for the opening `---`, +1 for 1-based
			}
		}
	}
	for i := range doc.Stops {
		if l, ok := seq[doc.Stops[i].ID]; ok {
			doc.Stops[i].declLine = l
		} else {
			doc.Stops[i].declLine = 1
		}
	}
	return doc.Stops, end + 1, nil
}

// ── the ratchet ─────────────────────────────────────────────────────────────

func (r *stopsScanResult) applyBaseline(path string) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
	if err != nil {
		r.findf("baseline %s unreadable (%v) — the enumerated count is not ratcheted against anything", path, err)
		return
	}
	want := map[string]int{}
	for _, ln := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		k, v, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		n, convErr := strconv.Atoi(strings.TrimSpace(v))
		if convErr != nil {
			r.findf("baseline %s: %q is not a number", path, t)
			continue
		}
		want[strings.TrimSpace(k)] = n
	}
	got := map[string]int{
		"files_with_stops": r.FilesWithStops,
		"declarations":     r.Declarations,
		"anchors":          r.Anchors,
		"prose_matches":    r.ProseMatches,
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w, ok := want[k]
		if !ok {
			r.findf("baseline %s: no `%s:` line — that count is not ratcheted", path, k)
			continue
		}
		if w != got[k] {
			verb := "rose to"
			if got[k] < w {
				verb = "FELL to"
			}
			r.findf("baseline %s: %s %s %d, baseline says %d. A fall means the scan stopped seeing stop points it used to see; a rise means one landed without being ratcheted. Re-read the diff, then update the baseline in the same change", path, k, verb, got[k], w)
		}
	}
}

// ── reporting ───────────────────────────────────────────────────────────────

func (r *stopsScanResult) report(w io.Writer, classes bool) bool {
	fmt.Fprintf(w, "stop-point scan: %d command file(s) read, %d carrying `stops:`\n", r.FilesScanned, r.FilesWithStops)
	fmt.Fprintf(w, "  prose side:        %d pattern match(es), %d anchor(s)\n", r.ProseMatches, r.Anchors)
	fmt.Fprintf(w, "  declaration side:  %d declaration(s)\n", r.Declarations)

	unmeasured := r.ProseMatches+r.Anchors == 0
	if unmeasured {
		fmt.Fprintf(w, "\nUNMEASURED: the pinned regex matched nothing across %d file(s). A scan that\n"+
			"enumerated zero stop points has said nothing about whether they are declared —\n"+
			"this is not a pass. Check the pinned regex in the patterns file.\n", r.FilesScanned)
	}

	if classes {
		r.reportClasses(w)
	}

	if len(r.Findings) > 0 {
		fmt.Fprintf(w, "\n%d finding(s):\n", len(r.Findings))
		for _, f := range r.Findings {
			fmt.Fprintf(w, "  - %s\n", f)
		}
	}
	if unmeasured || len(r.Findings) > 0 {
		return false
	}
	fmt.Fprintf(w, "\nOK: every prose stop point is anchored and declared; every declaration is anchored.\n")
	return true
}

func (r *stopsScanResult) reportClasses(w io.Writer) {
	byClass := map[string][]stopDecl{}
	for _, f := range r.Files {
		for _, s := range f.Stops {
			byClass[s.Class] = append(byClass[s.Class], s)
		}
	}
	fmt.Fprintf(w, "\nclass report (%d declaration(s)):\n", r.Declarations)

	// duplicate — the class that must not survive.
	fmt.Fprintf(w, "  duplicate: %d surviving\n", len(byClass["duplicate"]))

	// risk — every one needs a default and the safe side on silence.
	//
	// THE CLEAN LINE IS A WHOLE-CLASS VERDICT, not a row of independent
	// counters. A caller greps this report for `risk: 0 undeclared-default`;
	// printing that phrase beside a NON-zero not-safe-side count would hand
	// that caller a match while the class is defective — the substring is the
	// interface, so a defect anywhere in the class has to break it. Hence the
	// DEFECTIVE prefix: it moves the numbers out from under the clean line's
	// shape rather than reporting them in it.
	risk := byClass["risk"]
	if len(risk) == 0 {
		fmt.Fprintf(w, "  risk: UNMEASURED — 0 declared\n")
	} else {
		noDefault, notSafe := 0, 0
		for _, s := range risk {
			if s.Default == nil || strings.TrimSpace(*s.Default) == "" {
				noDefault++
			}
			if s.OnSilence != "safe-side" {
				notSafe++
			}
		}
		if noDefault > 0 || notSafe > 0 {
			fmt.Fprintf(w, "  risk: DEFECTIVE — %d undeclared-default, %d not-safe-side (%d declared)\n", noDefault, notSafe, len(risk))
		} else {
			fmt.Fprintf(w, "  risk: 0 undeclared-default, 0 not-safe-side (%d declared)\n", len(risk))
		}
	}

	sel := byClass["selection"]
	if len(sel) == 0 {
		fmt.Fprintf(w, "  selection: UNMEASURED — 0 declared\n")
	} else {
		noDefault, notTake, outOfSet := 0, 0, 0
		for _, s := range sel {
			if s.Default == nil || strings.TrimSpace(*s.Default) == "" {
				noDefault++
			}
			if s.OnSilence != "take-default" {
				notTake++
			}
			if len(s.Options) > 0 && s.Default != nil {
				in := false
				for _, o := range s.Options {
					if o == *s.Default {
						in = true
					}
				}
				if !in {
					outOfSet++
				}
			}
		}
		if noDefault > 0 || notTake > 0 || outOfSet > 0 {
			fmt.Fprintf(w, "  selection: DEFECTIVE — %d undeclared-default, %d not-take-default, %d default-outside-options (%d declared)\n",
				noDefault, notTake, outOfSet, len(sel))
		} else {
			fmt.Fprintf(w, "  selection: 0 undeclared-default, 0 not-take-default, 0 default-outside-options (%d declared)\n", len(sel))
		}
	}

	kn := byClass["knowledge"]
	if len(kn) == 0 {
		fmt.Fprintf(w, "  knowledge: UNMEASURED — 0 declared\n")
	} else {
		bad := 0
		for _, s := range kn {
			if s.OnSilence != "refuse" && s.OnSilence != "unanswered" {
				bad++
			}
		}
		fmt.Fprintf(w, "  knowledge: %d without refuse-or-unanswered (%d declared)\n", bad, len(kn))
	}
}

func init() {
	stopsCmd.AddCommand(newStopsScanCmd())
	rootCmd.AddCommand(stopsCmd)
}
