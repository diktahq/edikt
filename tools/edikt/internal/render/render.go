// Package render produces the compiled governance output via text/template.
// Deterministic: same input produces byte-equal output.
package render

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"github.com/diktahq/edikt/tools/edikt/internal/compile"
)

// EmptySHA is sha256("") in lower-case hex; the anchor for any
// freshly-bootstrapped empty managed region defaults to this so the
// bootstrap-write step emits a deterministic value rather than a literal
// empty string.
var EmptySHA = func() string {
	h := sha256.Sum256(nil)
	return hex.EncodeToString(h[:])
}()

// RegionSHA computes the sha256 of a region's rendered body. The body is
// the concatenation of each bullet line followed by '\n'. For an empty
// region the result is sha256("") (==EmptySHA). Used by phase B to anchor
// each managed region inside a topic file per Phase 8 of
// PLAN-v060-governance-accuracy.
func RegionSHA(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

//go:embed templates/topic.md.tmpl
var topicTmplSrc string

//go:embed templates/index.md.tmpl
var indexTmplSrc string

// TopicView is the data passed to the topic template.
type TopicView struct {
	Name            string
	Paths           []string
	Sources         []string
	Rules           []compile.Rule
	CompiledAt      string // ISO 8601, may be a fixed sentinel when deterministic output is required
	CompilerVersion string
	Fingerprint     string // Phase 8: SHA-256 over (sidecar_path, sidecar_content_hash) tuples for the topic

	// Phase 8 (PLAN-v060-governance-accuracy) — three managed regions per
	// topic file. DirectiveLines is the interleaved auto+manual list with
	// the *(manual)* marker on author-overrides. ProhibitionLines is the
	// MUST NOT bullets. ManualLines is the manual-only faithful copy. The
	// SHA fields are sha256 over the rendered content of each region
	// (excluding marker lines and the anchor line itself).
	DirectiveLines   []string
	ProhibitionLines []string
	ManualLines      []string
	DirectivesSHA    string
	ProhibitionsSHA  string
	ManualSHA        string
}

// IndexView is the data passed to the governance.md index template.
type IndexView struct {
	CompiledAt      string
	CompilerVersion string
	ADRCount        int
	ADRAcceptedCount int
	ADRSupersededCount int
	INVCount        int
	INVActiveCount  int
	GuidelineCount  int
	DirectiveCount  int
	TopicCount      int

	InvariantRules    []compile.Rule
	InvariantRestated []compile.Rule

	RoutingRows []RoutingRow
	Reminders   []string
	Verification []string
}

// RoutingRow is one line of the governance.md routing table.
type RoutingRow struct {
	Signals string
	Scope   string
	File    string
}

var tmplFuncs = template.FuncMap{
	"title":       titleCase,
	"joinSources": func(srcs []string) string { return strings.Join(srcs, ", ") },
}

// RenderTopic produces the body of a single topic file.
func RenderTopic(v TopicView) (string, error) {
	// Sort paths and sources for determinism — template's iteration order
	// must not depend on insertion order.
	sort.Strings(v.Paths)
	sort.Strings(v.Sources)
	// Default SHA fields to the empty-region hash when caller did not set
	// them. Empty-region anchor is sha256("") — keeps the bootstrap path
	// from emitting a literal empty hex.
	if v.DirectivesSHA == "" {
		v.DirectivesSHA = EmptySHA
	}
	if v.ProhibitionsSHA == "" {
		v.ProhibitionsSHA = EmptySHA
	}
	if v.ManualSHA == "" {
		v.ManualSHA = EmptySHA
	}

	tmpl, err := template.New("topic").Funcs(tmplFuncs).Parse(topicTmplSrc)
	if err != nil {
		return "", fmt.Errorf("parse topic template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		return "", fmt.Errorf("exec topic template: %w", err)
	}
	return buf.String(), nil
}

// RenderIndex produces the body of governance.md.
func RenderIndex(v IndexView) (string, error) {
	tmpl, err := template.New("index").Funcs(tmplFuncs).Parse(indexTmplSrc)
	if err != nil {
		return "", fmt.Errorf("parse index template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		return "", fmt.Errorf("exec index template: %w", err)
	}
	return buf.String(), nil
}

// titleCase turns "agent-rules" into "Agent-Rules" for headings.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	capNext := true
	for i, r := range runes {
		if unicode.IsLetter(r) && capNext {
			runes[i] = unicode.ToUpper(r)
			capNext = false
		} else if r == '-' || r == '_' || r == ' ' {
			capNext = true
		} else {
			capNext = false
		}
	}
	return string(runes)
}
