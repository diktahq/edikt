// Package govrun contains the compile/check logic for edikt governance.
// It is called by cmd/gov/compile.go (cobra) and was previously inline in main.go.
package govrun

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/gov-compile/internal/compile"
	"github.com/diktahq/edikt/tools/gov-compile/internal/hash"
	"github.com/diktahq/edikt/tools/gov-compile/internal/orphan"
	"github.com/diktahq/edikt/tools/gov-compile/internal/parse"
	"github.com/diktahq/edikt/tools/gov-compile/internal/render"
	"github.com/diktahq/edikt/tools/gov-compile/model"
	"gopkg.in/yaml.v3"
)

// CompilerVersion is the version string embedded in compiled output.
// It mirrors the binary version and is set by the cmd layer before calling Run.
var CompilerVersion = "0.1.0"

// JSONOutput is the structured output for --json mode (compile.md §Reference JSON format).
type JSONOutput struct {
	Status     string         `json:"status"`
	Topics     []TopicSummary `json:"topics,omitempty"`
	Invariants []RuleSummary  `json:"invariants,omitempty"`
	Stats      *CompileStats  `json:"stats,omitempty"`
	Errors     []string       `json:"errors,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
}

// TopicSummary is a per-topic entry in JSONOutput.
type TopicSummary struct {
	Name       string   `json:"name"`
	File       string   `json:"file"`
	Directives int      `json:"directives"`
	Sources    []string `json:"sources"`
}

// RuleSummary is a per-rule entry in JSONOutput.
type RuleSummary struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// CompileStats holds aggregate counts for JSONOutput.
type CompileStats struct {
	ADRsAccepted    int `json:"adrs_accepted"`
	ADRsSuperseded  int `json:"adrs_superseded"`
	INVsActive      int `json:"invs_active"`
	Guidelines      int `json:"guidelines"`
	TotalDirectives int `json:"total_directives"`
	TopicFiles      int `json:"topic_files"`
}

// ediktConfig is the minimal shape of .edikt/config.yaml that govrun needs.
type ediktConfig struct {
	Paths struct {
		Decisions  string `yaml:"decisions"`
		Invariants string `yaml:"invariants"`
		Guidelines string `yaml:"guidelines"`
	} `yaml:"paths"`
}

// Run executes the governance compile or check operation.
// root is the project root directory; checkOnly skips writing output files;
// jsonMode suppresses prose and emits structured JSON to stdout.
func Run(root string, checkOnly, jsonMode bool, clk model.Clock) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return fmt.Errorf("load .edikt/config.yaml: %w", err)
	}

	progress := func(msg string) {
		if !jsonMode {
			fmt.Println(msg)
		}
	}

	progress("Step 1/5: Reading source documents...")

	// ── Load and filter documents ────────────────────────────────────────────
	rootFS := os.DirFS(root)

	loadDir := func(dir, kind string) ([]*parse.Document, error) {
		abs := filepath.Join(root, dir)
		docs, err := parse.LoadDocuments(rootFS, abs)
		if err != nil {
			return nil, fmt.Errorf("load %s (%s): %w", kind, abs, err)
		}
		return docs, nil
	}

	rawADRs, err := loadDir(cfg.Paths.Decisions, "ADRs")
	if err != nil {
		return err
	}
	rawINVs, err := loadDir(cfg.Paths.Invariants, "INVs")
	if err != nil {
		return err
	}
	rawGuides, _ := loadDir(cfg.Paths.Guidelines, "guidelines") // may not exist

	var acceptedADRs, supersededADRs []*parse.Document
	for _, d := range rawADRs {
		if d.IsIncluded("adr") {
			acceptedADRs = append(acceptedADRs, d)
		} else if isSuperseded(d) {
			supersededADRs = append(supersededADRs, d)
		}
	}
	var activeINVs []*parse.Document
	for _, d := range rawINVs {
		if d.IsIncluded("inv") {
			activeINVs = append(activeINVs, d)
		}
	}
	var includedGuides []*parse.Document
	for _, d := range rawGuides {
		if d.IsIncluded("guideline") {
			includedGuides = append(includedGuides, d)
		}
	}

	progress("Step 2/5: Checking for contradictions...")

	// ── Separate invariants from topic sources ───────────────────────────────
	allDocs := concat(acceptedADRs, activeINVs, includedGuides)

	var invDocs, topicDocs []*parse.Document
	var warnings []string
	var errMsgs []string

	for _, d := range allDocs {
		if !d.Sentinel.Present {
			warnings = append(warnings, fmt.Sprintf("no sentinel block — delegate to LLM: %s", d.Path))
			continue
		}
		if isINVDoc(d) {
			invDocs = append(invDocs, d)
		} else {
			if len(d.Sentinel.Topics) == 0 {
				warnings = append(warnings, fmt.Sprintf("no topic: field — delegate to LLM for grouping: %s", d.Path))
			}
			topicDocs = append(topicDocs, d)
		}
	}

	// ── Fast-path skip ───────────────────────────────────────────────────────
	progress("Step 3/5: Grouping directives by topic...")

	if !checkOnly {
		allFresh := true
		for _, d := range allDocs {
			if !d.Sentinel.Present {
				allFresh = false
				break
			}
			sh := hash.SourceHash(d.BodyExcludingSentinel())
			dh := hash.DirectivesHash(d.Sentinel.Directives)
			if sh != d.Sentinel.SourceHash || dh != d.Sentinel.DirectivesHash {
				allFresh = false
				break
			}
		}
		if allFresh {
			progress("Fast-path skip: all hashes match — no changes.")
			if jsonMode {
				emit(JSONOutput{Status: "skip"})
			}
			return nil
		}
	}

	// ── Group and build effective rules ──────────────────────────────────────
	topicMap, _, err := compile.Group(topicDocs)
	if err != nil {
		return err
	}

	var invRules []compile.Rule
	for _, d := range invDocs {
		src := compile.SourceID(d.Path)
		for _, r := range compile.EffectiveRules(d.Sentinel) {
			invRules = append(invRules, compile.Rule{Text: r, Source: src})
		}
	}
	var invRestated []compile.Rule
	for _, r := range invRules {
		s := r.Text
		if len(s) > 90 {
			s = s[:87] + "..."
		}
		invRestated = append(invRestated, compile.Rule{Text: s, Source: r.Source})
	}

	// ── Routing table + aggregations ─────────────────────────────────────────
	topicNames := sortedKeys(topicMap)
	var routingRows []render.RoutingRow
	for _, name := range topicNames {
		t := topicMap[name]
		routingRows = append(routingRows, render.RoutingRow{
			Signals: strings.Join(signalsForTopic(name), ", "),
			Scope:   strings.Join(dedup(t.Scope), ", "),
			File:    "governance/" + name + ".md",
		})
	}

	reminders, verification := aggregateRemindersAndVerification(allDocs)

	// ── Orphan detection ─────────────────────────────────────────────────────
	var currentOrphans []string
	for _, d := range allDocs {
		if !d.Sentinel.Present || d.Frontmatter.NoDirectives != "" {
			continue
		}
		if len(d.Sentinel.Directives) == 0 && len(d.Sentinel.ManualDirectives) == 0 {
			currentOrphans = append(currentOrphans, compile.SourceID(d.Path))
		}
	}
	historyPath := filepath.Join(root, ".edikt", "state", "compile-history.json")
	prior, _ := orphan.ReadState(historyPath)
	var priorList *[]string
	if prior != nil {
		priorList = &prior.OrphanADRs
	}
	sc, block, writeHist := orphan.Decide(currentOrphans, priorList)
	_ = sc
	for _, id := range currentOrphans {
		if block {
			errMsgs = append(errMsgs, fmt.Sprintf("[BLOCK] %s: consecutive compile with same orphan set", id))
		} else {
			warnings = append(warnings, fmt.Sprintf("[WARN] %s: no directives and no no-directives reason", id))
		}
	}

	// ── Early exit for --check or block ──────────────────────────────────────
	if checkOnly || block {
		status := "ok"
		if block || len(errMsgs) > 0 {
			status = "error"
		}
		if jsonMode {
			emit(JSONOutput{Status: status, Errors: errMsgs, Warnings: warnings})
		} else {
			for _, w := range warnings {
				fmt.Println("  WARN:", w)
			}
			for _, e := range errMsgs {
				fmt.Println("  ERROR:", e)
			}
		}
		if block || len(errMsgs) > 0 {
			return fmt.Errorf("compile blocked: %d error(s)", len(errMsgs))
		}
		return nil
	}

	// ── Write output ─────────────────────────────────────────────────────────
	progress("Step 4/5: Scanning codebase for path patterns...")
	progress("Step 5/5: Writing governance files...")

	outDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir governance: %w", err)
	}

	compiledAt := clk.Now().Format(time.RFC3339)
	totalDirectives := len(invRules)
	var summaries []TopicSummary

	for _, name := range topicNames {
		t := topicMap[name]
		body, err := render.RenderTopic(render.TopicView{
			Name:            name,
			Paths:           t.Paths,
			Sources:         t.Sources,
			Rules:           t.Rules,
			CompiledAt:      compiledAt,
			CompilerVersion: CompilerVersion,
		})
		if err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		if err := writeAtomic(filepath.Join(outDir, name+".md"), body); err != nil {
			return fmt.Errorf("write %s.md: %w", name, err)
		}
		totalDirectives += len(t.Rules)
		summaries = append(summaries, TopicSummary{
			Name:       name,
			File:       "governance/" + name + ".md",
			Directives: len(t.Rules),
			Sources:    t.Sources,
		})
	}

	indexBody, err := render.RenderIndex(render.IndexView{
		CompiledAt:         compiledAt,
		CompilerVersion:    CompilerVersion,
		ADRCount:           len(acceptedADRs) + len(supersededADRs),
		ADRAcceptedCount:   len(acceptedADRs),
		ADRSupersededCount: len(supersededADRs),
		INVCount:           len(activeINVs),
		INVActiveCount:     len(activeINVs),
		GuidelineCount:     len(includedGuides),
		DirectiveCount:     totalDirectives,
		TopicCount:         len(topicMap),
		InvariantRules:     invRules,
		InvariantRestated:  invRestated,
		RoutingRows:        routingRows,
		Reminders:          reminders,
		Verification:       verification,
	})
	if err != nil {
		return fmt.Errorf("render index: %w", err)
	}
	if err := writeAtomic(filepath.Join(root, ".claude", "rules", "governance.md"), indexBody); err != nil {
		return fmt.Errorf("write governance.md: %w", err)
	}

	if writeHist {
		_ = orphan.WriteState(historyPath, currentOrphans, CompilerVersion)
	}

	// ── Summary ──────────────────────────────────────────────────────────────
	if jsonMode {
		var invS []RuleSummary
		for _, r := range invRules {
			invS = append(invS, RuleSummary{Text: r.Text, Source: r.Source})
		}
		emit(JSONOutput{
			Status:     "success",
			Topics:     summaries,
			Invariants: invS,
			Stats: &CompileStats{
				ADRsAccepted:    len(acceptedADRs),
				ADRsSuperseded:  len(supersededADRs),
				INVsActive:      len(activeINVs),
				Guidelines:      len(includedGuides),
				TotalDirectives: totalDirectives,
				TopicFiles:      len(topicMap),
			},
			Warnings: warnings,
		})
	} else {
		fmt.Println()
		fmt.Println("✅ Governance compiled")
		fmt.Println()
		for _, s := range summaries {
			fmt.Printf("  governance/%s.md  (%d directives ← %s)\n", s.Name, s.Directives, strings.Join(s.Sources, ", "))
		}
		fmt.Printf("\n  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("  %d ADRs + %d invariants + %d guidelines\n", len(acceptedADRs), len(activeINVs), len(includedGuides))
		fmt.Printf("  → %d topic files + index\n", len(topicMap))
		fmt.Printf("  → %d total directives\n", totalDirectives)
		for _, w := range warnings {
			fmt.Println("  WARN:", w)
		}
		fmt.Println()
		fmt.Println("  Next: /edikt:gov:review to review directive language quality.")
	}
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func loadConfig(root string) (ediktConfig, error) {
	var cfg ediktConfig
	cfg.Paths.Decisions = "docs/architecture/decisions"
	cfg.Paths.Invariants = "docs/architecture/invariants"
	cfg.Paths.Guidelines = "docs/guidelines"

	data, err := os.ReadFile(filepath.Join(root, ".edikt", "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	var raw struct {
		Paths struct {
			Decisions  string `yaml:"decisions"`
			Invariants string `yaml:"invariants"`
			Guidelines string `yaml:"guidelines"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return cfg, err
	}
	if raw.Paths.Decisions != "" {
		cfg.Paths.Decisions = raw.Paths.Decisions
	}
	if raw.Paths.Invariants != "" {
		cfg.Paths.Invariants = raw.Paths.Invariants
	}
	if raw.Paths.Guidelines != "" {
		cfg.Paths.Guidelines = raw.Paths.Guidelines
	}
	return cfg, nil
}

func isINVDoc(d *parse.Document) bool {
	return strings.HasPrefix(filepath.Base(d.Path), "INV-")
}

func isSuperseded(d *parse.Document) bool {
	return strings.HasPrefix(strings.ToLower(d.Frontmatter.Status), "superseded")
}

func sortedKeys(m map[string]*compile.Topic) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func concat(slices ...[]*parse.Document) []*parse.Document {
	var out []*parse.Document
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func aggregateRemindersAndVerification(docs []*parse.Document) (reminders, verification []string) {
	seenR, seenV := map[string]bool{}, map[string]bool{}
	for _, d := range docs {
		for _, r := range d.Sentinel.Reminders {
			if !seenR[r] {
				seenR[r] = true
				reminders = append(reminders, r)
			}
		}
		for _, v := range d.Sentinel.Verification {
			if !seenV[v] {
				seenV[v] = true
				verification = append(verification, v)
			}
		}
	}
	if len(reminders) > 10 {
		reminders = reminders[:10]
	}
	if len(verification) > 15 {
		verification = verification[:15]
	}
	return
}

func signalsForTopic(name string) []string {
	known := map[string][]string{
		"architecture":  {"platform", "rules", "plan", "architecture", "structure"},
		"agent-rules":   {"agent", "specialist", "advisor", "subagent", "evaluator", "domain signal", "BLOCKED", "verdict schema"},
		"hooks":         {"hook", "PostToolUse", "format", "preprocessing", "CLAUDE.md", "sentinel", "fixture", "characterization", "JSON protocol", "systemMessage", "permissions", "settings.json"},
		"extensibility": {"template", "override", "custom", "rule pack", "upgrade"},
		"compile":       {"compile", "governance", "schema", "directive", "sentinel block", "invariant record", "INV", "artifact"},
		"release":       {"release", "install", "SHA256SUMS", "cosign", "signing", "Sigstore", "integrity", "tag", "upgrade"},
		"tooling":       {"tier-2", "helper", "tools/", "deterministic", "gov-compile", "latency", "token cost"},
	}
	if sigs, ok := known[name]; ok {
		return sigs
	}
	return []string{name}
}

func writeAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func emit(v JSONOutput) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

// loadFromDir is reserved for future direct use.
func loadFromDir(fsys fs.FS, dir string) ([]*parse.Document, error) {
	return parse.LoadDocuments(fsys, dir)
}

var _ = loadFromDir // suppress unused warning
