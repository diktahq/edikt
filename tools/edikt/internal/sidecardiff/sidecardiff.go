// Package sidecardiff implements the three-tier structural-equivalence
// comparator for golden fixture testing (Phase 6, PLAN-v060-governance-accuracy).
// No LLM invocation, no exec.LookPath, no network — pure Go only (ADR-030).
package sidecardiff

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"gopkg.in/yaml.v3"
)

// FixtureConfig is the schema for <fixture-dir>/fixture.yaml.
type FixtureConfig struct {
	Model       string     `yaml:"model"`
	Temperature float64    `yaml:"temperature"`
	Seed        *int       `yaml:"seed,omitempty"`
	Thresholds  Thresholds `yaml:"thresholds,omitempty"`
	HashBaseline string    `yaml:"hash_baseline"`
}

// Thresholds holds the per-tier pass/fail thresholds.
type Thresholds struct {
	LevenshteinMax float64 `yaml:"levenshtein_max"`
	JaccardMin     float64 `yaml:"jaccard_min"`
}

// defaultThresholds returns the spec-mandated defaults.
func defaultThresholds() Thresholds {
	return Thresholds{
		LevenshteinMax: 0.05,
		JaccardMin:     0.7,
	}
}

// LoadFixtureConfig loads and strictly decodes <fixture-dir>/fixture.yaml.
// Unknown top-level fields are rejected (KnownFields strict decode).
func LoadFixtureConfig(fixtureDir string) (*FixtureConfig, error) {
	path := filepath.Join(fixtureDir, "fixture.yaml")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open fixture.yaml: %w", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(bufio.NewReader(f))
	dec.KnownFields(true)

	var cfg FixtureConfig
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse fixture.yaml: %w", err)
	}

	// Apply defaults when the thresholds block is missing/zero.
	defaults := defaultThresholds()
	if cfg.Thresholds.LevenshteinMax == 0 {
		cfg.Thresholds.LevenshteinMax = defaults.LevenshteinMax
	}
	if cfg.Thresholds.JaccardMin == 0 {
		cfg.Thresholds.JaccardMin = defaults.JaccardMin
	}
	return &cfg, nil
}

// Result is the overall result of a Diff run.
type Result struct {
	Pass bool
	// Tier results in order: [0]=Tier1, [1]=Tier2, [2]=Tier3
	Tiers []TierResult
}

// TierResult holds the outcome and diagnostics for one tier.
type TierResult struct {
	Name        string
	Pass        bool
	Diagnostics []string
}

// refIDRe matches "(ref: <id>)" in directive text.
var refIDRe = regexp.MustCompile(`\(ref:\s*([\w-]+)\)`)

// greppableTokenRe matches tokens worth comparing in verification entries.
var greppableTokenRe = regexp.MustCompile(`[\w/]+\.(go|md|sh|py|yaml|json)|GET|POST|PUT|DELETE|PATCH|grep|test_\w+|^[A-Z][A-Z_]+$`)

// Diff loads expected.edikt.yaml and actual.edikt.yaml from fixtureDir,
// applies the three-tier comparator with cfg's thresholds, and returns
// the Result. All output is written to w (suitable for CI logs).
func Diff(fixtureDir string, cfg *FixtureConfig) (*Result, error) {
	expectedPath := filepath.Join(fixtureDir, "expected.edikt.yaml")
	actualPath := filepath.Join(fixtureDir, "actual.edikt.yaml")

	exp, err := sidecar.Load(expectedPath)
	if err != nil {
		return nil, fmt.Errorf("load expected: %w", err)
	}
	act, err := sidecar.Load(actualPath)
	if err != nil {
		return nil, fmt.Errorf("load actual: %w", err)
	}

	res := &Result{}

	t1 := tier1HardFields(exp, act)
	res.Tiers = append(res.Tiers, t1)

	var t2 TierResult
	if t1.Pass {
		t2 = tier2DirectiveBodies(exp, act, cfg.Thresholds.LevenshteinMax)
	} else {
		t2 = TierResult{
			Name:        "directive-bodies",
			Pass:        false,
			Diagnostics: []string{"skipped: Tier 1 failed"},
		}
	}
	res.Tiers = append(res.Tiers, t2)

	t3 := tier3VerificationJaccard(exp, act, cfg.Thresholds.JaccardMin)
	res.Tiers = append(res.Tiers, t3)

	res.Pass = t1.Pass && t2.Pass && t3.Pass
	return res, nil
}

// FormatResult writes a human-readable result to the returned string.
// No ANSI colors.
func FormatResult(r *Result) string {
	tierNames := []string{"hard-fields", "directive-bodies", "verification-jaccard"}
	var sb strings.Builder
	for i, tr := range r.Tiers {
		name := tierNames[i]
		if i < len(r.Tiers) && r.Tiers[i].Name != "" {
			name = r.Tiers[i].Name
		}
		status := "PASS"
		if !tr.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(&sb, "[TIER %d: %s] %s\n", i+1, name, status)
		for _, d := range tr.Diagnostics {
			fmt.Fprintf(&sb, "  %s\n", d)
		}
	}
	if r.Pass {
		fmt.Fprintf(&sb, "RESULT: PASS\n")
	} else {
		fmt.Fprintf(&sb, "RESULT: FAIL\n")
	}
	return sb.String()
}

// ── Tier 1: hard fields, strict equal ────────────────────────────────────────

func sortedCopy(s []string) []string {
	cp := append([]string(nil), s...)
	sort.Strings(cp)
	return cp
}

func extractRefIDs(directives []sidecar.Directive) []string {
	seen := map[string]bool{}
	var ids []string
	for _, d := range directives {
		for _, m := range refIDRe.FindAllStringSubmatch(d.Text, -1) {
			id := m[1]
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

func tier1HardFields(exp, act *sidecar.Sidecar) TierResult {
	tr := TierResult{Name: "hard-fields", Pass: true}

	if exp.Topic != act.Topic {
		tr.Pass = false
		tr.Diagnostics = append(tr.Diagnostics,
			fmt.Sprintf("topic: expected=%q actual=%q", exp.Topic, act.Topic))
	}

	expSignals := sortedCopy(exp.Signals)
	actSignals := sortedCopy(act.Signals)
	if !strSliceEq(expSignals, actSignals) {
		tr.Pass = false
		tr.Diagnostics = append(tr.Diagnostics,
			fmt.Sprintf("signals: expected=%v actual=%v", expSignals, actSignals))
	}

	expPaths := sortedCopy(exp.Paths)
	actPaths := sortedCopy(act.Paths)
	if !strSliceEq(expPaths, actPaths) {
		tr.Pass = false
		tr.Diagnostics = append(tr.Diagnostics,
			fmt.Sprintf("paths: expected=%v actual=%v", expPaths, actPaths))
	}

	expScope := sortedCopy(exp.Scope)
	actScope := sortedCopy(act.Scope)
	if !strSliceEq(expScope, actScope) {
		tr.Pass = false
		tr.Diagnostics = append(tr.Diagnostics,
			fmt.Sprintf("scope: expected=%v actual=%v", expScope, actScope))
	}

	if len(exp.Prohibitions) != len(act.Prohibitions) {
		tr.Pass = false
		tr.Diagnostics = append(tr.Diagnostics,
			fmt.Sprintf("prohibitions count: expected=%d actual=%d",
				len(exp.Prohibitions), len(act.Prohibitions)))
	}

	if len(exp.Directives) != len(act.Directives) {
		tr.Pass = false
		tr.Diagnostics = append(tr.Diagnostics,
			fmt.Sprintf("directives count: expected=%d actual=%d",
				len(exp.Directives), len(act.Directives)))
	} else {
		expRefs := extractRefIDs(exp.Directives)
		actRefs := extractRefIDs(act.Directives)
		if !strSliceEq(expRefs, actRefs) {
			tr.Pass = false
			tr.Diagnostics = append(tr.Diagnostics,
				fmt.Sprintf("directive ref IDs: expected=%v actual=%v", expRefs, actRefs))
		}
	}

	return tr
}

func strSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── Tier 2: directive bodies, normalized Levenshtein ─────────────────────────

var modalityTokenRe = regexp.MustCompile(`\b(MUST NOT|MUST|SHOULD NOT|SHOULD|MAY|NEVER|ALWAYS|DO NOT)\b`)

func normalizeDirective(s string) string {
	s = strings.ToLower(s)
	// collapse whitespace
	fields := strings.FieldsFunc(s, unicode.IsSpace)
	s = strings.Join(fields, " ")
	return s
}

// extractModalityPrefix pulls out sorted modality tokens as a prefix and
// returns "PREFIX remaining" normalized form.
func normalizeWithModality(s string) string {
	// Find modality tokens in original (case-sensitive) form first
	modTokens := modalityTokenRe.FindAllString(s, -1)
	sort.Strings(modTokens)
	// Normalize the full string
	norm := normalizeDirective(s)
	if len(modTokens) == 0 {
		return norm
	}
	prefix := strings.ToLower(strings.Join(modTokens, " "))
	// Remove modality tokens from the normalized text to get remainder
	for _, tok := range modTokens {
		norm = strings.ReplaceAll(norm, strings.ToLower(tok), "")
	}
	// Collapse again
	remainder := strings.Join(strings.Fields(norm), " ")
	return prefix + " " + remainder
}

func tier2DirectiveBodies(exp, act *sidecar.Sidecar, maxRatio float64) TierResult {
	tr := TierResult{Name: "directive-bodies", Pass: true}
	// Tier 1 guarantees counts match; zip by index.
	for i := range exp.Directives {
		expNorm := normalizeWithModality(exp.Directives[i].Text)
		actNorm := normalizeWithModality(act.Directives[i].Text)
		dist := Levenshtein(expNorm, actNorm)
		maxLen := max2(len(expNorm), len(actNorm))
		var ratio float64
		if maxLen > 0 {
			ratio = float64(dist) / float64(maxLen)
		}
		if ratio > maxRatio {
			tr.Pass = false
			tr.Diagnostics = append(tr.Diagnostics,
				fmt.Sprintf("directive[%d]: levenshtein_ratio=%.4f (max=%.2f)", i, ratio, maxRatio))
			tr.Diagnostics = append(tr.Diagnostics,
				fmt.Sprintf("  expected: %q", exp.Directives[i].Text))
			tr.Diagnostics = append(tr.Diagnostics,
				fmt.Sprintf("  actual:   %q", act.Directives[i].Text))
		}
	}
	return tr
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Tier 3: verification items, keyword Jaccard ───────────────────────────────

// tokenizeSplitRe splits on whitespace and punctuation (not word chars or /).
var tokenizeSplitRe = regexp.MustCompile(`[^\w/]+`)

func greppableTokens(items []string) map[string]bool {
	set := map[string]bool{}
	for _, item := range items {
		parts := tokenizeSplitRe.Split(item, -1)
		for _, tok := range parts {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			if greppableTokenRe.MatchString(tok) {
				set[tok] = true
			}
		}
	}
	return set
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	union := len(a)
	for k := range b {
		if !a[k] {
			union++
		}
	}
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func tier3VerificationJaccard(exp, act *sidecar.Sidecar, minJaccard float64) TierResult {
	tr := TierResult{Name: "verification-jaccard", Pass: true}
	expTokens := greppableTokens(exp.Verification)
	actTokens := greppableTokens(act.Verification)
	score := jaccard(expTokens, actTokens)
	if score < minJaccard {
		tr.Pass = false
		var missing, extra []string
		for k := range expTokens {
			if !actTokens[k] {
				missing = append(missing, k)
			}
		}
		for k := range actTokens {
			if !expTokens[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		tr.Diagnostics = append(tr.Diagnostics,
			fmt.Sprintf("jaccard=%.4f (min=%.2f)", score, minJaccard))
		if len(missing) > 0 {
			tr.Diagnostics = append(tr.Diagnostics,
				fmt.Sprintf("tokens in expected but not actual: %v", missing))
		}
		if len(extra) > 0 {
			tr.Diagnostics = append(tr.Diagnostics,
				fmt.Sprintf("tokens in actual but not expected: %v", extra))
		}
	}
	return tr
}

// ── Levenshtein — rolled own, no third-party dep ─────────────────────────────

// Levenshtein returns the edit distance between a and b using the standard
// iterative two-row O(min(m,n)) implementation.
func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	// Ensure ra is the shorter string for the space optimization.
	if len(ra) > len(rb) {
		ra, rb = rb, ra
	}
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}

	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := 0; i <= la; i++ {
		prev[i] = i
	}

	for j := 1; j <= lb; j++ {
		curr[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[i] + 1
			ins := curr[i-1] + 1
			sub := prev[i-1] + cost
			curr[i] = minOf3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[la]
}

func minOf3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}
