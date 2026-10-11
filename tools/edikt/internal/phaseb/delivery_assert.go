// delivery_assert.go — every effective rule must reach a rendered surface.
//
// WHY THIS OUTLIVES THE FIX IT SHIPPED WITH.
//
// A manual directive on a topic retired to a skill package reached NO compiled
// surface at all. That was found by hand on a downstream corpus, not by any
// gate — and it is the second time: skillpackage.go:51-55 records the same
// omission one field over, "measured at 65 prohibitions and 145 verification
// items lost across the six retired topics, found by the AC-2.7 coverage
// baseline rather than by any gate".
//
// Two fixes, both correct, both found by someone counting. What was missing is
// the property: a rule that survives extraction, is not suppressed, and is
// attached to a live topic MUST appear somewhere a model will read. This
// asserts exactly that, so the third instance fails a compile instead of
// waiting for a manual count.
//
// NO CARVE-OUTS. The assertion also catches rules that were already missing for
// unrelated reasons. Those are diagnosed separately; a gate with an exemption
// for the cases it already found is not a gate.
package phaseb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"gopkg.in/yaml.v3"
)

var deliveryWS = regexp.MustCompile(`\s+`)

// deliveryNorm collapses whitespace so a rule wrapped differently in a rendered
// surface than in its sidecar still matches. Rendering re-wraps; the rule is
// the same rule.
func deliveryNorm(s string) string {
	return strings.TrimSpace(deliveryWS.ReplaceAllString(s, " "))
}

// deliveryProbeLen is how much of a rule must appear verbatim to count as
// delivered. Long enough that two different rules cannot collide, short enough
// that a trailing annotation — a ref tag, an appended *(manual)* marker —
// does not cause a false miss.
const deliveryProbeLen = 80

// UndeliveredRule is one effective rule that reached no rendered surface.
type UndeliveredRule struct {
	Artifact string
	Topic    string
	Kind     string // "directive" | "manual"
	Text     string
}

// renderedCorpus reads every rendered surface into one normalised haystack.
//
// A YAML surface is PARSED, not read as text. The index renders directive
// text as double-quoted scalars, so a rule containing `\$`, `\[` or `"` is
// stored escaped — `\\$`, `\\[`, `\"` — and its sidecar text, which is what
// this assertion probes with, never appears in the file bytes. Four
// directives on this corpus (INV-006:d05, INV-007:d11, INV-013:d09,
// INV-016:d06) sat under every one of their globs and were reported missing
// by the raw-text probe. GL-002 Part I rule 14: comparing structured data as
// raw text measures its serialisation, not its content.
//
// A YAML surface that fails to parse falls back to its raw text rather than
// being skipped — a corrupt surface must not shrink the haystack silently.
func renderedCorpus(projectRoot string) (string, int, error) {
	var b strings.Builder
	n := 0
	for _, root := range []string{
		filepath.Join(projectRoot, ".claude", "rules"),
		filepath.Join(projectRoot, ".claude", "skills"),
	} {
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil //nolint:nilerr // a missing surface tree is not an error here
			}
			if !strings.HasSuffix(p, ".md") && !strings.HasSuffix(p, ".yaml") {
				return nil
			}
			data, rerr := os.ReadFile(p) // #nosec G122 -- reads compiled surfaces in the project tree edikt itself renders; a concurrent swap of that tree is not in the threat model
			if rerr != nil {
				return nil
			}
			if strings.HasSuffix(p, ".yaml") {
				b.WriteString(yamlScalars(data))
			} else {
				b.WriteString(deliveryNorm(string(data)))
			}
			b.WriteByte(' ')
			n++
			return nil
		})
		if err != nil {
			return "", 0, err
		}
	}
	return b.String(), n, nil
}

// yamlScalars returns every string scalar in a YAML document, normalised and
// space-joined, so a probe matches the value a reader would see rather than
// the bytes the renderer escaped it into. Falls back to the raw text when the
// document does not parse.
func yamlScalars(data []byte) string {
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return deliveryNorm(string(data))
	}
	var b strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteString(deliveryNorm(x))
			b.WriteByte(' ')
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return b.String()
}

// AssertDelivery reports every effective rule that reached no rendered surface.
//
// Effective = directives minus suppressed_directives, UNION manual_directives.
// Returns nil when every one is delivered. An empty rendered corpus returns an
// error rather than a pass: nothing to search is not everything found
// (INV-013).
func AssertDelivery(projectRoot string, pairs []sidecar.Pair) ([]UndeliveredRule, error) {
	hay, files, err := renderedCorpus(projectRoot)
	if err != nil {
		return nil, err
	}
	if files == 0 || strings.TrimSpace(hay) == "" {
		return nil, fmt.Errorf("delivery assertion: no rendered surfaces to search — " +
			"nothing was measured, which is not a pass")
	}

	var missing []UndeliveredRule
	for _, p := range pairs {
		sc := p.Sidecar
		if sc == nil {
			continue
		}
		suppressed := map[string]bool{}
		for _, s := range sc.SuppressedDirectives {
			suppressed[deliveryNorm(s)] = true
		}
		check := func(kind, text string) {
			t := deliveryNorm(text)
			if t == "" || suppressed[t] {
				return
			}
			probe := t
			if len(probe) > deliveryProbeLen {
				probe = probe[:deliveryProbeLen]
			}
			if !strings.Contains(hay, probe) {
				missing = append(missing, UndeliveredRule{
					Artifact: p.ArtifactID, Topic: sc.Topic, Kind: kind, Text: t,
				})
			}
		}
		for _, d := range sc.Directives {
			check("directive", d.Text)
		}
		for _, m := range sc.ManualDirectives {
			check("manual", m)
		}
	}
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].Artifact != missing[j].Artifact {
			return missing[i].Artifact < missing[j].Artifact
		}
		return missing[i].Text < missing[j].Text
	})
	return missing, nil
}

// FormatUndelivered renders the misses for a compile failure — named, grouped,
// and bounded. A list nobody can read is a list nobody acts on.
func FormatUndelivered(missing []UndeliveredRule) string {
	var b strings.Builder
	byTopic := map[string]int{}
	for _, m := range missing {
		byTopic[m.Topic]++
	}
	topics := make([]string, 0, len(byTopic))
	for t := range byTopic {
		topics = append(topics, t)
	}
	sort.Strings(topics)
	fmt.Fprintf(&b, "%d effective rule(s) reach no rendered surface:\n", len(missing))
	for _, t := range topics {
		fmt.Fprintf(&b, "  topic %s: %d\n", t, byTopic[t])
	}
	shown := missing
	if len(shown) > 10 {
		shown = shown[:10]
	}
	for _, m := range shown {
		txt := m.Text
		if len(txt) > 90 {
			txt = txt[:90] + "…"
		}
		fmt.Fprintf(&b, "  - %s [%s] %s\n", m.Artifact, m.Kind, txt)
	}
	if len(missing) > len(shown) {
		fmt.Fprintf(&b, "  ... and %d more\n", len(missing)-len(shown))
	}
	return b.String()
}
