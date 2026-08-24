package hook

// statusline.go — the Go replacement for templates/hooks/status-line.sh
// (PLAN-v080-reliability-core Phase 13). Emits a plain-text governance
// health summary ("ADRs: N | INVs: M | Drift: K"), cached to
// $HOME/.edikt/state/statusline-cache on a 60s TTL. Zero python3 calls in
// the bash original.
//
// PLAIN TEXT, NOT JSON. Registered under settings.json.tmpl's top-level
// `statusLine` key rather than `hooks:` (templates/hooks/registry.yaml's own
// event: "statusLine" row comment), and Claude Code renders this hook's
// stdout directly — no {"systemMessage": ...} wrapping. The bash original's
// own `printf '%s' "$OUTPUT"` emits ZERO trailing bytes beyond the text
// itself, which is why this is the one handler in the tier that sets
// adapter.Response.NoTrailingNewline — see that field's own doc comment for
// why the dispatcher cannot simply supply the usual single "\n" here.

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("status-line", statusLine)
}

var (
	statusLineADRAcceptedRe = regexp.MustCompile(`(?m)^status:[ \t]*accepted|^\*\*Status:\*\*[ \t]+Accepted`)
	statusLineINVActiveRe   = regexp.MustCompile(`(?m)^status:[ \t]*active|^\*\*Status:\*\*[ \t]+Active`)
	statusLineDriftRe       = regexp.MustCompile(`"findings_open"[ \t]*:[ \t]*([0-9]+)`)
)

func statusLine(inv *Invocation) Result {
	if os.Getenv("EDIKT_STATUSLINE_SKIP") == "1" {
		return Result{}
	}

	pwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	cfgPath := statusLineFindConfig(pwd)
	if cfgPath == "" {
		return Result{}
	}

	home := os.Getenv("HOME")
	cacheDir := filepath.Join(home, ".edikt", "state")
	cacheFile := filepath.Join(cacheDir, "statusline-cache")
	if fi, statErr := os.Stat(cacheFile); statErr == nil && !fi.IsDir() {
		age := time.Now().Unix() - fi.ModTime().Unix()
		if age < 60 {
			if body, rerr := os.ReadFile(cacheFile); rerr == nil { //nolint:gosec // project-local cache file
				return Result{Response: adapter.Response{Text: string(body), NoTrailingNewline: true}}
			}
		}
	}

	cfgBytes, err := os.ReadFile(cfgPath) //nolint:gosec // project-local config
	if err != nil {
		return Result{}
	}
	if statusLineEnabledValue(cfgBytes) != "true" {
		return Result{}
	}

	proot := filepath.Dir(filepath.Dir(cfgPath))
	decisionsDir := statusLineResolvePath(cfgBytes, proot, "decisions", "architecture/decisions")
	invariantsDir := statusLineResolvePath(cfgBytes, proot, "invariants", "architecture/invariants")

	adrCount := 0
	if fi, derr := os.Stat(decisionsDir); derr == nil && fi.IsDir() {
		adrCount = statusLineCountMatchingFiles(decisionsDir, "ADR-", statusLineADRAcceptedRe)
	}
	invCount := 0
	if fi, derr := os.Stat(invariantsDir); derr == nil && fi.IsDir() {
		invCount = statusLineCountMatchingFiles(invariantsDir, "INV-", statusLineINVActiveRe)
	}
	driftCount := statusLineDriftCount(filepath.Join(proot, ".edikt", "state", "drift-report.json"))

	output := fmt.Sprintf("ADRs: %d | INVs: %d | Drift: %s", adrCount, invCount, driftCount)

	_ = os.MkdirAll(cacheDir, 0o755)
	_ = os.WriteFile(cacheFile, []byte(output), 0o644) //nolint:gosec // matches bash's own cache file mode

	return Result{Response: adapter.Response{Text: output, NoTrailingNewline: true}}
}

// statusLineFindConfig mirrors: walk up from $PWD looking for
// .edikt/config.yaml, stopping at "/". Unlike every other hook in this
// tier, status-line.sh does NOT restrict itself to the process's own cwd —
// it climbs ancestors, per the preprocessor-contract's cwd-independence
// note in the bash original's own header comment.
func statusLineFindConfig(pwd string) string {
	d := pwd
	for d != "/" {
		cand := filepath.Join(d, ".edikt", "config.yaml")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

// statusLineEnabledValue mirrors the awk state machine reading
// `features:\n  statusline: <value>` — a top-level `features:` line, then
// the first INDENTED `statusline:` line beneath it, stopping the moment a
// non-indented line ends the block.
func statusLineEnabledValue(cfg []byte) string {
	inFeatures := false
	sc := bufio.NewScanner(bytes.NewReader(cfg))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "features:") {
			inFeatures = true
			continue
		}
		if inFeatures {
			if indent, ok := statusLineIndentedKey(line, "statusline:"); ok {
				return statusLineAwkValue(indent)
			}
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				inFeatures = false
			}
		}
	}
	return ""
}

// statusLineIndentedKey reports whether line is `^[[:space:]]+<key>` and
// returns the remainder of the line after the leading whitespace (still
// including the key itself, for statusLineAwkValue to strip).
func statusLineIndentedKey(line, key string) (string, bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i == 0 || !strings.HasPrefix(line[i:], key) {
		return "", false
	}
	return line[i:], true
}

// statusLineAwkValue mirrors:
//
//	sub(/.*statusline:[[:space:]]*/, "")
//	sub(/[[:space:]]*#.*$/, "")
func statusLineAwkValue(afterIndent string) string {
	rest := strings.TrimPrefix(afterIndent, "statusline:")
	j := 0
	for j < len(rest) && (rest[j] == ' ' || rest[j] == '\t') {
		j++
	}
	rest = rest[j:]
	if idx := strings.Index(rest, "#"); idx >= 0 {
		rest = strings.TrimRight(rest[:idx], " \t")
	}
	return rest
}

// statusLineResolvePath mirrors resolve_path(): a `  <key>: <value>` line in
// CFG wins; otherwise `<base:>/<defaultSub>` (base defaulting to "docs");
// an absolute result is returned as-is, a relative one is joined onto proot.
func statusLineResolvePath(cfg []byte, proot, key, defaultSub string) string {
	rel := statusLineGrepFieldTwo(cfg, "  "+key+":")
	if rel == "" {
		base := statusLineGrepFieldTwo(cfg, "base:")
		if base == "" {
			base = "docs"
		}
		rel = base + "/" + defaultSub
	}
	if strings.HasPrefix(rel, "/") {
		return rel
	}
	return filepath.Join(proot, rel)
}

// statusLineGrepFieldTwo mirrors `grep "^<prefix>" CFG | awk '{print $2}' |
// tr -d '"'`: the first line starting with prefix, its second
// whitespace-split field, quotes stripped.
func statusLineGrepFieldTwo(cfg []byte, prefix string) string {
	sc := bufio.NewScanner(bytes.NewReader(cfg))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, prefix) {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return ""
			}
			return strings.ReplaceAll(fields[1], `"`, "")
		}
	}
	return ""
}

// statusLineCountMatchingFiles mirrors `grep -l -E '<re>' DIR/<prefix>*.md
// | wc -l`: grep -l counts MATCHING FILES, not matching lines — a file with
// several matching lines still counts once.
func statusLineCountMatchingFiles(dir, prefix string, re *regexp.Regexp) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".md") {
			continue
		}
		body, rerr := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // project-local ADR/INV file
		if rerr != nil {
			continue
		}
		if re.Match(body) {
			count++
		}
	}
	return count
}

// statusLineDriftCount mirrors reading `"findings_open"` out of
// DRIFT_REPORT with a raw regex extraction (not JSON-parsed) — kept as a
// STRING, not re-formatted as an int: the bash original's `%s` printf
// re-emits exactly what grep captured, digits and all.
func statusLineDriftCount(path string) string {
	body, err := os.ReadFile(path) //nolint:gosec // project-local state file
	if err != nil {
		return "0"
	}
	m := statusLineDriftRe.FindSubmatch(body)
	if m == nil {
		return "0"
	}
	return string(m[1])
}
