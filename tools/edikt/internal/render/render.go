// Package render produces the compiled governance output via text/template.
// Deterministic: same input produces byte-equal output.
package render

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"github.com/diktahq/edikt/tools/edikt/internal/compile"
)

// EmptySHA is sha256("") in lower-case hex: the anchor a managed region whose
// body is EMPTY hashes to.
//
// It is a fact about the hash, not a default. Nothing here substitutes it for
// a missing value — every anchor is computed from the bytes this package
// emitted between that region's own markers (see anchorRegions), so a
// directives or manual region with no bullets ARRIVES at this value by
// hashing "", and the prohibitions region never does: topic.md.tmpl emits its
// "## Prohibitions" heading INSIDE the markers, so that body is never empty.
//
// The comment this replaces called it "the anchor for any freshly-bootstrapped
// empty managed region" and RenderTopic defaulted all three region anchors to
// it. That was true of two regions and false of the third, and the false one
// shipped: a bootstrap-written prohibitions region declared sha256("")
// (e3b0c442…) over a body that hashes to d1674c1a…. A doc comment stating a
// semantic the code did not implement, failing silently — the shape this
// package's tests exist to catch (PLAN-test-reach-on-the-delivery-path).
var EmptySHA = func() string {
	h := sha256.Sum256(nil)
	return hex.EncodeToString(h[:])
}()

// RegionSHA computes the sha256 of a region's rendered body — the bytes
// strictly between that region's start- and end-marker lines, heading and
// bullets included.
//
// This is the ONE implementation of the anchor formula. anchorRegions calls it
// on the bytes it just emitted; a reader verifying a compiled topic file calls
// it on the bytes they read back. No caller may compute an anchor from its own
// model of what the renderer WILL emit: predicting another component's bytes
// is how the prohibitions heading came to be counted in one place and not in
// the other (D357).
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
	// MUST NOT bullets. ManualLines is the manual-only faithful copy.
	//
	// There are no SHA fields. Each region's sha256 anchor is computed by
	// this package from the bytes it emits between that region's markers
	// (D357). A caller cannot supply one, correctly or otherwise: a field
	// that is accepted and discarded is the silent-discard shape this repo
	// has already paid for, and a field that is accepted and TRUSTED is one
	// component predicting another's private layout.
	//
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

	// SkillOnly marks a topic whose scoped rules file was RETIRED to a skill
	// package, so no .claude/rules/governance/<topic>.md exists for it.
	//
	// Without this the index told the reader to "read the named file" for four
	// topics that have none — agent-rules, compile, governance-authoring and
	// lifecycle. A model following the ambient core's own instruction found
	// nothing, and nothing pointed it at the skill package that does carry the
	// topic. A broken pointer is worse than an absent one: it spends the
	// reader's attention and returns nothing.
	SkillOnly bool
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

// RenderTopic produces the body of a single topic file, with every managed
// region's sha256 anchor computed from the bytes this call emitted.
//
// It returns ("", err) on any failure. Never a partial body with a nil error:
// a truncated topic file is a plausible-looking surface, which is the failure
// shape this package is least likely to notice and most likely to ship.
func RenderTopic(v TopicView) (string, error) {
	return renderTopic(topicTmplSrc, v)
}

// renderTopic is RenderTopic with the template source as a parameter, so a
// test can render through an ALTERED template and observe the anchors follow
// it. The seam is unexported: the template that ships is the embedded one.
func renderTopic(src string, v TopicView) (string, error) {
	// Sort paths and sources for determinism — template's iteration order
	// must not depend on insertion order.
	sort.Strings(v.Paths)
	sort.Strings(v.Sources)

	tmpl, err := template.New("topic").Funcs(tmplFuncs).Parse(src)
	if err != nil {
		return "", fmt.Errorf("parse topic template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		return "", fmt.Errorf("exec topic template: %w", err)
	}
	out, err := anchorRegions(buf.String())
	if err != nil {
		return "", fmt.Errorf("anchor topic %q: %w", v.Name, err)
	}
	return out, nil
}

// RenderIndex produces the body of governance.md. Same contract as
// RenderTopic: ("", err) or a complete body, never a partial one.
func RenderIndex(v IndexView) (string, error) {
	return renderIndex(indexTmplSrc, v)
}

// renderIndex is RenderIndex with the template source as a parameter — see
// renderTopic for why the seam exists.
func renderIndex(src string, v IndexView) (string, error) {
	tmpl, err := template.New("index").Funcs(tmplFuncs).Parse(src)
	if err != nil {
		return "", fmt.Errorf("parse index template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		return "", fmt.Errorf("exec index template: %w", err)
	}
	out, err := anchorRegions(buf.String())
	if err != nil {
		return "", fmt.Errorf("anchor index: %w", err)
	}
	return out, nil
}

// ── managed-region anchoring ────────────────────────────────────────────────
//
// ONE FACT, ONE PLACE (D357). A managed region's anchor is the sha256 of the
// bytes between its own markers, and the only component entitled to state
// those bytes is the one that wrote them. Before this, a caller
// (phaseb.regionSHA) rebuilt the body from the same inputs it had handed the
// renderer and hashed its RECONSTRUCTION — carrying a `withProhibitionsHeading`
// flag because the template puts a heading inside one region's markers. Three
// copies of that one fact (template, caller flag, renderer default), and the
// renderer's copy had drifted.
//
// There is no circularity: the anchor line sits AFTER the region's :end
// marker, outside the range being hashed, so the body can be emitted, hashed,
// and the anchor written into a line the body does not contain.

var (
	regionStartRe = regexp.MustCompile(`(?m)^\[edikt:([A-Za-z0-9_-]+):start\]: #$`)
	// An anchor line still carrying no hex value after the pass ran.
	unfilledAnchorRe = regexp.MustCompile(`(?m)^\[edikt:([A-Za-z0-9_-]+):sha256\]: #[ \t]*$`)
)

// anchorRegions writes each managed region's sha256 anchor into the rendered
// output, computed over that region's own emitted body.
//
// Every failure is loud. A region with markers and no anchor line, a region
// with two, a start marker with no end marker, and an anchor line left unfilled
// all return an error rather than output: an anchor that silently did not get
// written is indistinguishable from one that did, because a wrong hash still
// looks like a hash.
func anchorRegions(out string) (string, error) {
	for _, m := range regionStartRe.FindAllStringSubmatch(out, -1) {
		name := m[1]
		body, err := regionBody(out, name)
		if err != nil {
			return "", err
		}
		prefix := "[edikt:" + name + ":sha256]: #"
		anchorRe := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(prefix) + `.*$`)
		locs := anchorRe.FindAllStringIndex(out, -1)
		if len(locs) != 1 {
			return "", fmt.Errorf("region %q: %d %q lines in the rendered output, want exactly 1 — "+
				"a region whose anchor is missing or duplicated cannot be verified", name, len(locs), prefix)
		}
		loc := locs[0]
		out = out[:loc[0]] + prefix + " " + RegionSHA(body) + out[loc[1]:]
	}
	if m := unfilledAnchorRe.FindStringSubmatch(out); m != nil {
		return "", fmt.Errorf("region %q declares a sha256 anchor but has no [edikt:%s:start]/:end region "+
			"to hash — the anchor line would ship empty", m[1], m[1])
	}
	return out, nil
}

// regionBody returns the bytes a managed region encloses: everything strictly
// between its start-marker line and its end marker. A missing marker is an
// error, not an empty string — "the region is not there" and "the region is
// empty" are different facts and must not both read as "".
func regionBody(out, name string) (string, error) {
	start := "[edikt:" + name + ":start]: #\n"
	i := strings.Index(out, start)
	if i < 0 {
		return "", fmt.Errorf("region %q: no %q line in the rendered output", name, strings.TrimSuffix(start, "\n"))
	}
	rest := out[i+len(start):]
	end := "[edikt:" + name + ":end]: #"
	j := strings.Index(rest, end)
	if j < 0 {
		return "", fmt.Errorf("region %q: start marker with no %q after it", name, end)
	}
	return rest[:j], nil
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
