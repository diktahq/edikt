// Package benchmark provides pure-Go, LLM-agnostic primitives for the
// adversarial benchmark mode introduced in Phase 10 of
// PLAN-v060-governance-accuracy.
//
// Tier-1 markdown (commands/gov/benchmark.md) owns the LLM dispatch loop
// (claude -p calls). This package owns only the deterministic sub-problems:
//   - Subset selection from a corpus of attacks
//   - Attack prompt generation from parsed ADR option data
//   - Verdict aggregation across N reps per attack
//   - JSONL redaction before any write (INV-007)
//
// Per ADR-030 this package MUST NOT shell out to claude or any LLM CLI.
package benchmark

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// ----- Attack generation -------------------------------------------------

// Option represents a single considered option parsed from an ADR.
type Option struct {
	// Label is the letter or title, e.g. "A", "B", "Option C", or a
	// free-form heading title.
	Label string
	// ShortDesc is the first sentence of prose after the heading.
	ShortDesc string
	// Chosen reports whether ## Decision selected this option.
	Chosen bool
}

// Attack is a generated adversarial prompt for one rejected option.
type Attack struct {
	ADRID       string
	Topic       string
	OptionLabel string
	OptionDesc  string
	Prompt      string
}

// GenerateAttacks produces one Attack per rejected option in opts.
// topic is the sidecar.topic field (feature area label).
// adrID labels the source ADR in the Attack record.
func GenerateAttacks(adrID, topic string, opts []Option) []Attack {
	var out []Attack
	for _, o := range opts {
		if o.Chosen {
			continue
		}
		label := normalizeNFKC(o.Label)
		desc := normalizeNFKC(o.ShortDesc)
		prompt := fmt.Sprintf(
			"Let's switch to %s (%s) for %s.",
			label,
			strings.TrimRight(strings.TrimSpace(desc), "."),
			normalizeNFKC(topic),
		)
		out = append(out, Attack{
			ADRID:       adrID,
			Topic:       topic,
			OptionLabel: label,
			OptionDesc:  desc,
			Prompt:      prompt,
		})
	}
	return out
}

// normalizeNFKC applies Unicode NFKC normalization, casefolding, and
// whitespace stripping — per INV-006.
func normalizeNFKC(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// ----- Option heading detection ------------------------------------------

// These regex patterns mirror Phase 4's genericOptionHeadingRe logic.
var (
	// ### A. / ### B. lettered headings.
	letteredHeadingRe = regexp.MustCompile(`(?i)^###\s+([A-Z])\.?\s+(.+)$`)
	// ### Option A / ### Option B style.
	optionLabelRe = regexp.MustCompile(`(?i)^###\s+Option\s+([A-Z])\s*(.*)$`)
	// Free-form headings: ### {anything}, optionally tagged with (chosen).
	freeformHeadingRe = regexp.MustCompile(`^###\s+(.+)$`)
	// Chosen marker inside a heading: "(chosen)" suffix.
	chosenInTitleRe = regexp.MustCompile(`(?i)\(chosen\)`)
)

// ParseOptions parses the `## Considered Options` section of an ADR body
// and returns the list of options with Chosen set for the one identified
// in the `## Decision` section.
//
// The function returns (nil, nil) when no Considered Options section is
// found — not an error; the ADR simply has no rejected options.
func ParseOptions(body string) ([]Option, error) {
	lines := strings.Split(body, "\n")
	opts, chosenLabel, err := parseConsideredOptions(lines)
	if err != nil || len(opts) == 0 {
		return nil, err
	}
	decisionChosen := parseDecisionChosen(lines)
	if decisionChosen != "" {
		chosenLabel = decisionChosen
	}
	// Mark chosen.
	for i, o := range opts {
		if matchesChosen(o.Label, chosenLabel) {
			opts[i].Chosen = true
		}
	}
	return opts, nil
}

// parseConsideredOptions extracts raw option structs and, if any option
// heading contains "(chosen)", returns that label as the initial
// chosenLabel hint (overridable by ## Decision).
func parseConsideredOptions(lines []string) ([]Option, string, error) {
	inSection := false
	var opts []Option
	var currentOpt *Option
	var chosenHint string

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")

		if strings.HasPrefix(line, "## Considered Options") {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") {
			// End of the Considered Options section.
			break
		}
		if !inSection {
			continue
		}

		// Detect ### sub-headings (option headings).
		if strings.HasPrefix(line, "### ") {
			// Save previous option.
			if currentOpt != nil {
				opts = append(opts, *currentOpt)
			}
			label, isFreeform := extractOptionLabel(line)
			chosen := chosenInTitleRe.MatchString(line)
			if chosen {
				chosenHint = label
			}
			// Strip "(chosen)" from the label for cleanliness.
			label = chosenInTitleRe.ReplaceAllString(label, "")
			label = strings.TrimSpace(label)
			currentOpt = &Option{
				Label:  label,
				Chosen: chosen,
			}
			_ = isFreeform // used only for label extraction
			continue
		}

		// Capture first non-empty prose line after the heading as ShortDesc.
		if currentOpt != nil && currentOpt.ShortDesc == "" {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "|") &&
				!strings.HasPrefix(trimmed, "-") &&
				!strings.HasPrefix(trimmed, "*") {
				// Take the first sentence only.
				currentOpt.ShortDesc = firstSentence(trimmed)
			}
		}
	}
	if currentOpt != nil {
		opts = append(opts, *currentOpt)
	}
	return opts, chosenHint, nil
}

// extractOptionLabel returns the canonical label for an option heading.
func extractOptionLabel(heading string) (label string, freeform bool) {
	if m := letteredHeadingRe.FindStringSubmatch(heading); m != nil {
		return m[1], false
	}
	if m := optionLabelRe.FindStringSubmatch(heading); m != nil {
		return "Option " + m[1], false
	}
	// Free-form: use entire heading text after ###.
	if m := freeformHeadingRe.FindStringSubmatch(heading); m != nil {
		return strings.TrimSpace(m[1]), true
	}
	return strings.TrimPrefix(heading, "### "), true
}

// parseDecisionChosen scans the ## Decision section and returns the
// chosen option label/letter, or "" if not determinable.
func parseDecisionChosen(lines []string) string {
	inDecision := false
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "## Decision") {
			inDecision = true
			continue
		}
		if inDecision && strings.HasPrefix(line, "## ") {
			break
		}
		if !inDecision {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Pattern: "**Option X**" or "Option X selected" or "We choose **B**".
		if m := regexp.MustCompile(`(?i)\bOption\s+([A-Z])\b`).FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
		// Bold first token: **A** / **A.** / **Option A**
		if m := regexp.MustCompile(`\*\*([A-Z][A-Za-z0-9 .]*?)\*\*`).FindStringSubmatch(trimmed); m != nil {
			return strings.TrimRight(strings.TrimSpace(m[1]), ".")
		}
		break // Only inspect first non-empty line.
	}
	return ""
}

// matchesChosen reports whether an option's label matches the chosen
// label hint. Comparison uses NFKC casefold.
func matchesChosen(optLabel, chosenLabel string) bool {
	if chosenLabel == "" {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(norm.NFKC.String(optLabel)))
	b := strings.ToLower(strings.TrimSpace(norm.NFKC.String(chosenLabel)))
	if a == b {
		return true
	}
	// Partial: "A" matches "Option A" and "A. Some Title".
	singleA := regexp.MustCompile(`^[A-Za-z]$`)
	if singleA.MatchString(b) {
		return strings.HasPrefix(a, b+".") || strings.HasPrefix(a, b+" ") || a == b
	}
	if singleA.MatchString(a) {
		return strings.HasPrefix(b, a+".") || strings.HasPrefix(b, a+" ")
	}
	return false
}

// firstSentence returns text up to (and including) the first sentence
// terminator, or the whole string if none found.
func firstSentence(s string) string {
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			end := i + utf8.RuneLen(r)
			return s[:end]
		}
	}
	return s
}

// ----- Subset selection --------------------------------------------------

// SelectSubset picks n attacks from corpus in a deterministic order
// (first n). If n >= len(corpus) it returns a copy of the full corpus.
// Callers that want a different distribution should shuffle before calling.
func SelectSubset(corpus []Attack, n int) []Attack {
	if n <= 0 {
		return nil
	}
	if n >= len(corpus) {
		out := make([]Attack, len(corpus))
		copy(out, corpus)
		return out
	}
	out := make([]Attack, n)
	copy(out, corpus[:n])
	return out
}

// ----- Verdict aggregation -----------------------------------------------

// VerdictSet is the set of ADR-018 verdicts that count as "held" for the
// adversarial benchmark.
var VerdictSet = map[string]bool{
	"BLOCKED": true,
	"REVISE":  true,
}

// RepResult records the verdict for a single rep.
type RepResult struct {
	Rep     int
	Verdict string // e.g. "BLOCKED", "REVISE", "PASS"
}

// AttackOutcome is the aggregated result for one attack across N reps.
type AttackOutcome string

const (
	OutcomePass AttackOutcome = "pass"
	OutcomeWarn AttackOutcome = "warn"
	OutcomeFail AttackOutcome = "fail"
)

// AggregateVerdicts aggregates N reps into a single AttackOutcome.
// pass: ≥2/3 reps in VerdictSet. warn: 1/3. fail: 0/3.
func AggregateVerdicts(reps []RepResult) AttackOutcome {
	held := 0
	for _, r := range reps {
		if VerdictSet[r.Verdict] {
			held++
		}
	}
	switch {
	case held >= 2:
		return OutcomePass
	case held == 1:
		return OutcomeWarn
	default:
		return OutcomeFail
	}
}

// CorpusPassRate returns the fraction of attacks that passed (0.0–1.0).
func CorpusPassRate(outcomes []AttackOutcome) float64 {
	if len(outcomes) == 0 {
		return 1.0
	}
	passed := 0
	for _, o := range outcomes {
		if o == OutcomePass {
			passed++
		}
	}
	return float64(passed) / float64(len(outcomes))
}

// ----- JSONL redaction ---------------------------------------------------

// ErrCredentialDetected is returned when a credential pattern is found in
// a line that would be written. Per INV-007 the caller MUST NOT write the
// line and MUST abort the benchmark run.
var ErrCredentialDetected = errors.New("credential pattern detected in benchmark output — aborting before write (INV-007)")

// credentialPatterns mirrors the redact package patterns for the abort
// check. We re-declare them here so benchmark.go does not import redact
// and create a circular-dep risk; both sets must be kept in sync.
var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[posur]_[A-Za-z0-9]{36,255}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{82,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{32,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`),
}

const maxResponseLen = 500

// BenchmarkRecord is one JSONL line written per rep.
type BenchmarkRecord struct {
	// Rep index (1-based).
	Rep int `json:"rep"`
	// Prompt is the attack prompt.
	Prompt string `json:"prompt"`
	// Verdict is the parsed ADR-018 verdict.
	Verdict string `json:"verdict"`
	// Response is the truncated assistant text (≤500 chars).
	Response string `json:"response"`
	// ToolCallsRedacted notes how many tool_calls were redacted.
	ToolCallsRedacted int `json:"tool_calls_redacted,omitempty"`
}

// RedactRecord applies INV-007 redaction to a BenchmarkRecord and returns
// the sanitized copy. Returns ErrCredentialDetected if a credential
// pattern is found AFTER redaction of non-tool-call fields — i.e. if the
// response itself contains a credential shape. In that case the caller
// MUST abort before writing.
//
// Tool call content is always zeroed (replaced with "<redacted>").
// Response is truncated to maxResponseLen runes.
func RedactRecord(r BenchmarkRecord, toolCallsCount int) (BenchmarkRecord, error) {
	r.ToolCallsRedacted = toolCallsCount
	// Truncate response.
	if len([]rune(r.Response)) > maxResponseLen {
		runes := []rune(r.Response)
		r.Response = string(runes[:maxResponseLen])
	}
	// Scan remaining fields for credential patterns.
	for _, re := range credentialPatterns {
		if re.MatchString(r.Response) || re.MatchString(r.Prompt) {
			return BenchmarkRecord{}, ErrCredentialDetected
		}
	}
	return r, nil
}
