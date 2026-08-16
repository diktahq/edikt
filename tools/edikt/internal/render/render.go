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
	Name string
	// Description is the topic's APPROVED registry line from
	// .edikt/topics.yaml, rendered VERBATIM. Empty means no human has
	// approved a description for this topic yet; the template says so
	// explicitly rather than omitting the line, because a missing
	// description and an approved-but-blank one must not look the same
	// (INV-013). Nothing in this package may rewrite, wrap, truncate, or
	// re-derive it (SSP-002).
	Description string
	// Paths is the compiled scope: either the union of the contributing
	// sidecars' declared code globs, or the single unrestricted glob when
	// any contributor declared none.
	Paths []string
	// ScopeNote explains the Paths above in one line — how many globs came
	// from how many sources, or which sources left the topic unscoped.
	// "Unscoped because undeclared" must not read the same as "unscoped by
	// choice", so the reason travels with the result.
	ScopeNote       string
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
	// Reminders and Verification moved here from governance.md's always-on
	// body. They are ARTIFACT-SCOPED guidance, so they belong beside the
	// directives they qualify, behind the topic file's `paths:` glob —
	// governance.md declares `paths: "**/*"` and therefore loaded all 335 of
	// them on every edit regardless of what was being touched (D37).
	Reminders    []string
	Verification []string

	DirectiveLines   []string
	ProhibitionLines []string
	ManualLines      []string
	DirectivesSHA    string
	ProhibitionsSHA  string
	ManualSHA        string

	// DeliveryNote explains an empty Directives region that is empty on
	// purpose — every contributing directive was path-scoped and therefore
	// delivered from directive-index.yaml at write time (ADR-066).  edikt-guard:allow
	//
	// It renders OUTSIDE the managed region, above the start anchor. Inside
	// would fold prose into the bytes DirectivesSHA covers, making a
	// generated explanation indistinguishable from a hand-edit of a managed
	// region; and a reader who has already scrolled past two bare anchors has
	// already drawn the wrong conclusion, so the explanation has to arrive
	// first.
	//
	// Empty for every topic that renders its own directives, which keeps the
	// common case byte-identical.
	DeliveryNote string
}

// IndexView is the data passed to the governance.md index template.
type IndexView struct {
	CompiledAt      string
	CompilerVersion string

	// Compiled counts are the artifacts whose directives actually reached
	// the compiled output. Retired artifacts (superseded / deprecated /
	// migration:skip) are filtered out upstream of the merge and are NOT
	// counted here.
	//
	// These replace the former ADRCount/ADRAcceptedCount/ADRSupersededCount
	// and INVCount/INVActiveCount pairs. Those were two names for one
	// number wherever the caller had only the active set, which is how the
	// index came to report "41 ADRs (41 accepted, 0 superseded)" for a
	// corpus of 53 ADRs with 8 retired: the split was a claim, not a
	// measurement. One name per number removes the place the claim hid.
	ADRCompiled       int
	INVCompiled       int
	GuidelineCompiled int

	// Excluded counts retired artifacts by kind ("adr", "invariant",
	// "guideline"). A nil map means the caller did not measure exclusions
	// and the header says so; a non-nil map — including an empty one — is
	// a real measurement whose total may legitimately be zero. Collapsing
	// those two into a bare 0 is the defect this field exists to prevent
	// (INV-013: an absent field decoded to a zero value is UNMEASURED).
	Excluded map[string]int

	DirectiveCount int
	TopicCount     int

	InvariantRules    []compile.Rule
	InvariantRestated []compile.Rule

	// TopicIndex is the registry-sourced topic list: one row per rendered
	// topic, carrying the approved description verbatim. It is the surface
	// SPEC-011 grows the ambient core toward — a reader picks a topic from a  edikt-guard:allow
	// task-language line instead of scanning a keyword table.
	//
	// Every rendered topic gets a row, including topics whose description is
	// still pending approval. Dropping the pending ones would make the index
	// silently under-report the corpus.
	TopicIndex []TopicIndexRow

	Reminders    []string
	Verification []string
}

// TopicIndexRow is one topic and its pinned description.
type TopicIndexRow struct {
	Topic string
	// Description is verbatim registry content, or empty when no approved
	// entry exists for this topic.
	Description string
}

var tmplFuncs = template.FuncMap{
	"title":        titleCase,
	"joinSources":  func(srcs []string) string { return strings.Join(srcs, ", ") },
	"excludedNote": excludedNote,
}

// excludedNote renders the retired-artifact clause of the index source
// header from IndexView.Excluded.
//
// A nil map is reported as UNMEASURED, not as zero. The two are different
// claims: "I counted the retired artifacts and there were none" versus "I
// never counted". A `{{ if .Excluded }}` guard in the template could not
// draw that line, because Go templates treat a nil map and an empty map
// alike — which is exactly how a measured zero would have been mislabelled
// as unmeasured, the same defect in the other direction.
//
// Keys are rendered in sorted order rather than from a fixed list, so a
// kind this function has never heard of still appears in the total instead
// of being silently dropped.
func excludedNote(m map[string]int) string {
	if m == nil {
		return "retired-artifact exclusions UNMEASURED"
	}
	kinds := make([]string, 0, len(m))
	total := 0
	for k, n := range m {
		if n <= 0 {
			continue
		}
		total += n
		kinds = append(kinds, k)
	}
	if total == 0 {
		return "0 retired artifacts excluded"
	}
	sort.Strings(kinds)
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		label := k
		if label == "" {
			// Pair.Kind is empty when the artifact came from a configured
			// directory the class contract has no entry for. Naming it
			// keeps the total honest without inventing a class.
			label = "unclassified"
		}
		parts = append(parts, fmt.Sprintf("%s %d", label, m[k]))
	}
	noun := "artifacts"
	if total == 1 {
		noun = "artifact"
	}
	return fmt.Sprintf("%d retired %s excluded (%s)", total, noun, strings.Join(parts, ", "))
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

// RendererFingerprint is a hash over the render TEMPLATES themselves.
//
// It exists because the topic-file cache was keyed on its INPUTS (the
// contributing sidecars, and later the approved topic description) but never on
// the RENDERER. Editing a template therefore changed what render would produce
// while every cached file kept its old bytes, and compile reported "10
// unchanged" — a true statement about the sidecars and a false one about the
// output. Measured, not theorised: removing compiled_at from both templates
// propagated to zero of ten topic files until this landed.
//
// That is the same class of defect as a timestamp in hashed bytes — "hash of
// what would render" ceasing to equal "hash on disk" — reached from the other
// direction, and it is why a render manifest cannot be trusted without it.
func RendererFingerprint() string {
	h := sha256.New()
	// Domain-separated and order-fixed, so two templates cannot swap content
	// and produce the same digest.
	h.Write([]byte("topic.md.tmpl\x00"))
	h.Write([]byte(topicTmplSrc))
	h.Write([]byte("\x00index.md.tmpl\x00"))
	h.Write([]byte(indexTmplSrc))
	return hex.EncodeToString(h.Sum(nil))
}
