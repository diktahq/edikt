package phasea

// defects.go — classify WHY a sidecar failed, in a form a retry can act on.
//
// THE ROOT CAUSE THIS FILE EXISTS TO REMOVE
//
// verify.go computed a precise per-item diagnostic — which directive is over
// the cap, which anchor does not quote its parent — and dropped it on the floor
// at the point the ReceiptItem was appended, because phasea-receipt.v1 had no
// field to carry it. The retry in commands/gov/compile.md reads the RECEIPT.
// So a re-dispatch sent the identical prompt against the identical artifact
// with no knowledge that it had failed, or why.
//
// That is not a flaky model. It is an UNSEEDED RETRY: the second sample is
// drawn from the same distribution as the first, so "different" is as likely to
// be worse as better. Measured on guideline-event-transport:
//
//	attempt 1   1 over-cap directive   (572)
//	attempt 2   4 over-cap directives  (551 / 664 / 566 / 578)
//	83 anchors  ALL VALID — length was the only defect
//	cost        ~140k tokens and ~1h42m PER dispatch, budget 2, zero output
//
// The system computed exactly the diagnostic that would have fixed the problem
// and discarded it before the retry that needed it.
//
// WHY LENGTH IS MEASURED HERE AND NOT READ OUT OF THE VALIDATOR
//
// The obvious implementation parses maxLength violations out of the schema
// validator's error text. That couples a retry decision to an error-message
// format no contract pins, and it cannot report the ones the validator did not
// reach — jsonschema implementations are free to stop at the first failure.
// Measuring the texts directly against the cap is deterministic, yields the
// line and the actual length the correction prompt needs, and keeps working if
// the validator is swapped.
//
// WHY ANCHORS ARE EVALUATED EVEN WHEN LENGTH ALREADY FAILED
//
// contentGate ran schema validation BEFORE anchors and returned on the first
// error, so a length failure hid anchor state completely. The event-transport
// artifact had 83 valid anchors and needed four sentences split; the receipt
// said "schema-invalid", which is indistinguishable from an extraction that
// grounded nothing. That is the difference between "split four sentences" and
// "re-extract everything" — so both axes are always measured.

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// MaxItemTextLen is the HARD cap -- the length at which the schema refuses the
// document. It is mirrored from internal/sidecar, not owned here: that package
// is the single definition and the schema copies are pinned to it.
//
// CHANGED 2026-08-30, from 500 to the hard cap. The reasoning below still
// stands and is the reason the SOFT cap did not move: a 664-character
// directive stating three obligations does enforce worse than three atomic
// ones, and the extractor prompt still prescribes the split. What changed is
// the CONSEQUENCE of ignoring that advice. Rejecting the whole sidecar for it
// cost a colleague two hours on one extraction and told them nothing about
// which field. So an over-soft directive is now ACCEPTED, WRITTEN, AND FLAGGED
// for a reviewer, and only over-HARD output is a defect that blocks.
//
// EVIDENCE CORRECTED 2026-08-30. This comment previously argued the point from
// "the nine recorded dispatches show re-attempts making outcomes worse". That
// claim is WITHDRAWN: turn count is entirely confounded with c04ad9ee (the
// stdin fix, 2026-08-27T20:03:00Z). All 5 pre-fix dispatches failed and all 4
// post-fix ones succeeded, with ZERO within-era separation, and ADR-002 fails
// at 30/32/69 turns before the fix and succeeds at 5/7 after -- same artifact.
// The nine establish that nothing worked before the fix and everything has
// since; they say nothing about turns and outcomes. See
// docs/internal/reviews/FINDING-2026-08-30-turn-count-confounded.md.
//
// The decision is unchanged and rests on three grounds that need no data:
// length is not a correctness defect, so a retry has no success criterion to
// stop on; retrying discards a valid sidecar, which is the silent loss the
// owner's soft/hard ruling forbids; and a dispatch costs ~$0.20 against a
// review edit costing nothing.
const MaxItemTextLen = sidecar.HardCapProse

// SoftItemTextLen is the extractor's TARGET. Text between soft and hard is
// perfectly valid and is reported for human review, never retried: the
// measured correlation between more attempts and worse outcomes makes a retry
// the wrong response to a directive that is merely long.
const SoftItemTextLen = sidecar.SoftCapText

// Defect axes. A retry can only be steered if the axis is named: a length
// defect wants four sentences split, an anchor defect wants a span re-quoted,
// and a parse defect wants the whole extraction redone.
const (
	DefectAxisNone   = "none"
	DefectAxisLength = "length"
	DefectAxisAnchor = "anchor"
	DefectAxisSchema = "schema"
	DefectAxisParse  = "parse"
	// DefectAxisOverclaim: verify_kind behavioral asserted without both
	// fixture files in the tree (D46 (b), D58). Retryable: the correction
	// pass tells the extractor the kind-by-shape rule and the entries.
	DefectAxisOverclaim = "overclaim"
)

// LengthDefect is one over-cap text, with everything the correction prompt
// needs to name it precisely.
type LengthDefect struct {
	Kind  string `json:"kind"`  // "directive" | "prohibition"
	Index int    `json:"index"` // 0-based position within its list
	Line  int    `json:"line"`  // 1-based line in the sidecar, 0 when unknown
	Len   int    `json:"len"`
	Head  string `json:"head"` // leading chars, to locate it by eye

	// ParentLines is the parent line range(s) the text's anchors record,
	// e.g. "12-14" or "12-14, 20-20"; empty when it has none. The correction
	// prompt names the text by these rather than by Line: the extractor never
	// sees the sidecar (restore has quarantined it), so a sidecar line number
	// points at nothing it can read (D-I).
	ParentLines string `json:"parent_lines,omitempty"`
}

// AnchorFaultBrief is one faulting anchor, reduced to what a correction pass
// needs: where it points in the parent, which class of fault it is, and —
// for a whitespace-only quote whose span is unique — the parent's own bytes.
type AnchorFaultBrief struct {
	Kind       string `json:"kind"`   // "directives" | "prohibitions"
	Index      int    `json:"index"`  // 0-based position within its list
	Anchor     int    `json:"anchor"` // 0-based within source_excerpts; -1 for "no anchors"
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
	Class      string `json:"class"`
	Head       string `json:"head,omitempty"` // the entry's leading text, so a fresh extraction can find it
	Canonical  string `json:"canonical,omitempty"`
	FoundStart int    `json:"found_start,omitempty"`
	FoundEnd   int    `json:"found_end,omitempty"`
}

// DefectReport is the full picture of one sidecar's defects, on every axis at
// once. Axis is the PRIMARY axis — what a correction should target — but the
// per-axis counts are all populated, because "83 anchors valid, 4 texts over
// cap" is a different instruction than "83 anchors broken".
type DefectReport struct {
	Axis    string `json:"axis"`
	Summary string `json:"summary"`

	LengthDefects []LengthDefect `json:"length_defects,omitempty"`
	AnchorsValid  int            `json:"anchors_valid"`
	AnchorsBroken int            `json:"anchors_broken"`

	// AnchorsWhitespaceOnly is how many of AnchorsBroken carry the parent's
	// words with reconstructed whitespace — a copy away from valid — kept
	// apart from spliced, misnumbered and invented quotes, which are not.
	AnchorsWhitespaceOnly int `json:"anchors_whitespace_only"`

	// NeedsReview counts the directives and prohibitions the sidecar flags
	// needs_review: true. Measured on the file, so a report can quote it
	// instead of the extractor's own narration of what it flagged.
	NeedsReview int `json:"needs_review"`

	// SchemaError / ParseError are the underlying messages when the sidecar
	// fails for a reason no correction prompt can steer.
	SchemaError string `json:"schema_error,omitempty"`
	ParseError  string `json:"parse_error,omitempty"`

	// Overclaims lists the entries that asserted behavioral without a
	// fixture pair, as the gate reported them.
	Overclaims []string `json:"overclaims,omitempty"`

	// AnchorFaults is every faulting anchor, in sidecar order. The anchor
	// correction prompt is rendered from it.
	AnchorFaults []AnchorFaultBrief `json:"anchor_faults,omitempty"`
}

// Count is the number of defects on the report's primary axis. It is the
// quantity convergence is measured on: attempt N is BETTER than N-1 when this
// falls, WORSE when it rises. Comparing across different axes is meaningless,
// which is why Axis travels with it.
func (r DefectReport) Count() int {
	switch r.Axis {
	case DefectAxisOverclaim:
		return len(r.Overclaims)
	case DefectAxisLength:
		return len(r.LengthDefects)
	case DefectAxisAnchor:
		return r.AnchorsBroken
	case DefectAxisNone:
		return 0
	default:
		// A parse or schema failure is one indivisible defect: the document
		// could not be evaluated item by item, so there is no count to take.
		return 1
	}
}

// ClassifyDefects measures a sidecar on every axis and names the primary one.
//
// The order of the axes is not arbitrary. Parse failure hides everything, so it
// wins. Length is checked before other schema violations because it is the one
// with a known mechanical remedy the extractor already carries. Anchors are
// measured last but ALWAYS measured, so a length failure never hides them.
func ClassifyDefects(sidecarPath, parentPath string) DefectReport {
	return ClassifyDefectsIn("", sidecarPath, parentPath)
}

// ClassifyDefectsIn is ClassifyDefects with the project root, which the
// over-claim check needs to stat fixture paths; an empty root skips that
// axis (the caller had no tree to check against).
func ClassifyDefectsIn(projectRoot, sidecarPath, parentPath string) DefectReport {
	var rep DefectReport

	// LOOSE DECODE, NOT sidecar.Load.
	//
	// sidecar.Load VALIDATES as it loads, and the 500-character cap is one of
	// the things it enforces — so an over-cap sidecar does not load at all.
	// Classifying through Load would therefore report every length failure as
	// "does not parse", and the anchors would be unmeasurable precisely when
	// the artifact most needs them measured. That is the reported case exactly:
	// 83 anchors that were valid the whole time, invisible because the document
	// carrying them could not be loaded.
	//
	// The loose decode asks only the two questions this file needs — how long
	// is each text, and where does each anchor point — and answers them for
	// documents the strict loader rejects.
	sc, err := looseParse(sidecarPath)
	if err != nil {
		rep.Axis = DefectAxisParse
		rep.ParseError = err.Error()
		rep.Summary = fmt.Sprintf("sidecar does not parse: %v", err)
		return rep
	}

	rep.LengthDefects = lengthDefects(sidecarPath, sc)
	for _, d := range sc.Directives {
		if d.NeedsReview {
			rep.NeedsReview++
		}
	}
	for _, p := range sc.Prohibitions {
		if p.NeedsReview {
			rep.NeedsReview++
		}
	}

	// Anchors are measured unconditionally — see the file doc.
	valid, faults := anchorFaultsOn(sc, parentPath)
	rep.AnchorsValid, rep.AnchorsBroken = valid, len(faults)
	for _, f := range faults {
		if f.Class == sidecar.FaultClassWhitespaceOnly {
			rep.AnchorsWhitespaceOnly++
		}
		rep.AnchorFaults = append(rep.AnchorFaults, AnchorFaultBrief{
			Kind: f.Kind, Index: f.Index, Anchor: f.Anchor,
			LineStart: f.LineStart, LineEnd: f.LineEnd,
			Class: f.Class, Head: entryHead(sc, f.Kind, f.Index), Canonical: f.Canonical,
			FoundStart: f.FoundStart, FoundEnd: f.FoundEnd,
		})
	}

	raw, rerr := os.ReadFile(sidecarPath)
	var schemaErr error
	if rerr == nil {
		schemaErr = sidecar.ValidateRawAgainstDeclaredSchema(raw)
	}

	if projectRoot != "" {
		if oerr := verifyNoBehavioralOverclaim(projectRoot, sidecarPath); oerr != nil {
			rep.Overclaims = strings.Split(strings.TrimPrefix(oerr.Error(), "behavioral over-claim: "), "; ")
		}
	}

	switch {
	case len(rep.LengthDefects) > 0:
		rep.Axis = DefectAxisLength
		rep.Summary = fmt.Sprintf(
			"%d text(s) over the %d-character cap; %d anchor(s) valid, %d broken",
			len(rep.LengthDefects), MaxItemTextLen, rep.AnchorsValid, rep.AnchorsBroken)
	case rep.AnchorsBroken > 0:
		rep.Axis = DefectAxisAnchor
		rep.Summary = fmt.Sprintf("%d anchor(s) do not quote the parent byte-exactly: %d whitespace-only, "+
			"%d not verbatim/misnumbered; %d valid",
			rep.AnchorsBroken, rep.AnchorsWhitespaceOnly, rep.AnchorsBroken-rep.AnchorsWhitespaceOnly, rep.AnchorsValid)
	case len(rep.Overclaims) > 0:
		rep.Axis = DefectAxisOverclaim
		rep.Summary = fmt.Sprintf("%d entr(y/ies) assert verify_kind behavioral without both fixture files; %d anchor(s) valid",
			len(rep.Overclaims), rep.AnchorsValid)
	case schemaErr != nil:
		// Schema-invalid for a reason that is neither length nor anchor. No
		// mechanical correction is known, so the axis stays generic rather
		// than pretending a targeted retry would help.
		rep.Axis = DefectAxisSchema
		rep.SchemaError = schemaErr.Error()
		rep.Summary = fmt.Sprintf("fails gov-sidecar validation: %v", schemaErr)
	default:
		rep.Axis = DefectAxisNone
		rep.Summary = fmt.Sprintf("no defects; %d anchor(s) valid", rep.AnchorsValid)
	}
	return rep
}

// lengthDefects measures every directive and prohibition text against the cap.
// Line numbers come from locating the text in the raw file; a text that cannot
// be located still reports Line 0 rather than being dropped, because an
// unlocatable over-cap directive is still over cap.
func lengthDefects(path string, sc *sidecar.Sidecar) []LengthDefect {
	var out []LengthDefect
	raw, _ := os.ReadFile(path)
	lines := strings.Split(string(raw), "\n")

	add := func(kind string, i int, text string, anchors []sidecar.SourceExcerpt) {
		if len(text) <= MaxItemTextLen {
			return
		}
		var ranges []string
		for _, a := range anchors {
			ranges = append(ranges, fmt.Sprintf("%d-%d", a.LineStart, a.LineEnd))
		}
		out = append(out, LengthDefect{
			Kind:        kind,
			Index:       i,
			Line:        locateLine(lines, text),
			Len:         len(text),
			Head:        head(text, 60),
			ParentLines: strings.Join(ranges, ", "),
		})
	}
	for i, d := range sc.Directives {
		add("directive", i, d.Text, d.Anchors())
	}
	for i, p := range sc.Prohibitions {
		add("prohibition", i, p.Text, p.Anchors())
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Line < out[b].Line })
	return out
}

// locateLine finds the 1-based line whose content contains the start of text.
// YAML folds long scalars across lines, so the whole string will not appear on
// one line; the first sentence-worth is enough to anchor a human or a
// correction prompt to the right place.
func locateLine(lines []string, text string) int {
	probe := head(text, 40)
	if probe == "" {
		return 0
	}
	for i, l := range lines {
		if strings.Contains(l, probe) {
			return i + 1
		}
	}
	return 0
}

// entryHead is the leading text of the directive or prohibition a fault
// belongs to ("" when the index is out of range).
func entryHead(sc *sidecar.Sidecar, kind string, i int) string {
	switch {
	case kind == "directives" && i >= 0 && i < len(sc.Directives):
		return head(sc.Directives[i].Text, 60)
	case kind == "prohibitions" && i >= 0 && i < len(sc.Prohibitions):
		return head(sc.Prohibitions[i].Text, 60)
	}
	return ""
}

func head(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// looseParse decodes only the fields defect classification needs, with no
// validation of any kind. It deliberately accepts documents sidecar.Load
// rejects — that is its entire reason for existing.
//
// Both anchor shapes are read: v1's single `source_excerpt` and v2's
// `source_excerpts` list. Reading only one would under-count anchors on half
// the corpus and report a clean tally for a document whose anchors were never
// looked at.
func looseParse(path string) (*sidecar.Sidecar, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Directives []struct {
			Text           string                  `yaml:"text"`
			SourceExcerpt  *sidecar.SourceExcerpt  `yaml:"source_excerpt"`
			SourceExcerpts []sidecar.SourceExcerpt `yaml:"source_excerpts"`
			NeedsReview    bool                    `yaml:"needs_review"`
		} `yaml:"directives"`
		Prohibitions []struct {
			Text           string                  `yaml:"text"`
			SourceExcerpt  *sidecar.SourceExcerpt  `yaml:"source_excerpt"`
			SourceExcerpts []sidecar.SourceExcerpt `yaml:"source_excerpts"`
			NeedsReview    bool                    `yaml:"needs_review"`
		} `yaml:"prohibitions"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	sc := &sidecar.Sidecar{}
	for _, d := range doc.Directives {
		nd := sidecar.Directive{Text: d.Text, SourceExcerpts: d.SourceExcerpts, NeedsReview: d.NeedsReview}
		if d.SourceExcerpt != nil {
			nd.SourceExcerpt = *d.SourceExcerpt
		}
		sc.Directives = append(sc.Directives, nd)
	}
	for _, p := range doc.Prohibitions {
		np := sidecar.Prohibition{Text: p.Text, SourceExcerpts: p.SourceExcerpts, NeedsReview: p.NeedsReview}
		if p.SourceExcerpt != nil {
			np.SourceExcerpt = *p.SourceExcerpt
		}
		sc.Prohibitions = append(sc.Prohibitions, np)
	}
	return sc, nil
}

// CorrectionPrompt renders the diagnostic as an INSTRUCTION for a re-dispatch,
// not a description of a failure.
//
// This is the difference the whole change turns on. The re-dispatch used to
// resend the original prompt verbatim; it now carries what went wrong and what
// to do about it, which is exactly what a human did by hand — in minutes — on
// the artifact that had already consumed ~280k tokens and ~3.4 hours.
//
// It returns "" when the axis admits no mechanical correction, so a caller
// cannot accidentally send an empty instruction and count it as a seeded retry.
func (r DefectReport) CorrectionPrompt() string {
	switch r.Axis {
	case DefectAxisLength:
		var entries, lens []string
		for _, d := range r.LengthDefects {
			where := fmt.Sprintf("%s #%d", d.Kind, d.Index)
			if d.ParentLines != "" {
				where = fmt.Sprintf("the %s grounded at parent lines %s", d.Kind, d.ParentLines)
			}
			entries = append(entries, fmt.Sprintf("%s, starting %q, is %d characters", where, d.Head, d.Len))
			lens = append(lens, fmt.Sprintf("%d", d.Len))
		}
		return capDiagnostic(fmt.Sprintf(
			"CORRECTION PASS — do not re-extract from scratch. The previous attempt "+
				"wrote %d text(s) over the %d-character ceiling (%s; lengths %s): %s. Split each "+
				"at its natural seam per sidecar-extractor.md:742 into atomic statements, "+
				"each anchored to the parent lines it states. The rest of that attempt "+
				"validated — %d anchor(s) quoted the parent byte-exactly — so extract "+
				"everything else the same way.",
			len(r.LengthDefects), MaxItemTextLen, pluralKind(r.LengthDefects),
			strings.Join(lens, "/"), strings.Join(entries, "; "), r.AnchorsValid), correctionPromptCap)
	case DefectAxisAnchor:
		return r.anchorCorrectionPrompt()
	case DefectAxisOverclaim:
		return fmt.Sprintf(
			"CORRECTION PASS — do not re-extract from scratch. %d entr(y/ies) assert "+
				"`verify_kind: behavioral` without both fixture files in the tree: %s. "+
				"The kind follows the verify's SHAPE and only that: a named test command "+
				"that exists in the tree → tooling; a grep, file or path check → structural; "+
				"nothing you can name → no verify and no verify_kind, falsifying_observation "+
				"only; behavioral ONLY when both positive_fixture_path and "+
				"negative_fixture_path exist and are named. A named test that proves the "+
				"whole rule is still tooling. Change only those entries' verify fields; "+
				"every directive text and every valid anchor stays unchanged (%d valid).",
			len(r.Overclaims), strings.Join(r.Overclaims, "; "), r.AnchorsValid)
	case DefectAxisParse:
		// A parse failure was originally classified non-correctable, on the
		// reasoning that it has no known mechanical remedy the way length
		// ("split at the seam") and anchor ("re-quote that span") do. That
		// reasoning held until the remedy was measured: a contract emitted
		// `text:` values as bare scalars, and every directive body ends in a
		// "(ref: ...)" tail carrying a colon-space, so a bare scalar cannot
		// parse by construction. The parser names the line. That is a remedy.
		//
		// The original guard is KEPT, not overridden: a correction prompt is
		// returned ONLY when the error localises. An unlocalised parse error
		// still yields "", because a retry that looks seeded and carries
		// nothing actionable is worse than an unseeded one — the metric built
		// to catch unseeded retries would report it fixed.
		line, ok := parseErrorLine(r.ParseError)
		if !ok {
			return ""
		}
		return fmt.Sprintf(
			"CORRECTION PASS — do not re-extract from scratch. The previous attempt "+
				"wrote a sidecar that is not valid YAML: %s. Fix the SYNTAX at line %d "+
				"and re-emit the same content: this is a serialization fault, not an "+
				"extraction fault, so do not re-decide any directive, any grade, or any "+
				"anchor. The parser's message is the authority on what is wrong. One "+
				"cause measured on this corpus: a `text:` value written as a bare "+
				"scalar, which cannot parse when the body ends in a \"(ref: ...)\" tail, "+
				"because that tail carries a colon-space — single-quote every `text:` "+
				"value, doubling any quote inside it. Unlike the other correction axes "+
				"this one cannot tell you how much of the document survived: the file "+
				"did not load, so no anchor in it has been checked.",
			strings.TrimSpace(r.ParseError), line)
	default:
		return ""
	}
}

// correctionPromptCap bounds a rendered correction prompt so it fits the
// receipt's 4000-character correction_prompt with capDiagnostic's suffix.
const correctionPromptCap = 3900

// anchorCorrectionPrompt is the anchor axis's instruction, self-contained
// (D-I): the extractor never sees the rejected sidecar — restore has
// quarantined it — so every faulting anchor is named by the PARENT line range
// it records, with what to do about it for its class. A whitespace-only quote
// gets the parent's exact bytes for the span, so the remedy is a copy, not a
// second reconstruction.
//
// Faults other than whitespace-only are listed first. They are the rarer and
// less mechanical ones, and when the prompt is capped the tail is what goes.
func (r DefectReport) anchorCorrectionPrompt() string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"CORRECTION PASS — do not re-extract from scratch. %d of %d anchors do not "+
			"quote the parent byte-exactly. Every anchor's quote must be ONE contiguous "+
			"slice of the parent's bytes at the lines it records, line breaks and "+
			"indentation included. Re-quote only these:",
		r.AnchorsBroken, r.AnchorsValid+r.AnchorsBroken)
	ordered := make([]AnchorFaultBrief, 0, len(r.AnchorFaults))
	for _, f := range r.AnchorFaults {
		if f.Class != sidecar.FaultClassWhitespaceOnly {
			ordered = append(ordered, f)
		}
	}
	for _, f := range r.AnchorFaults {
		if f.Class == sidecar.FaultClassWhitespaceOnly {
			ordered = append(ordered, f)
		}
	}
	for _, f := range ordered {
		b.WriteString("\n- ")
		b.WriteString(anchorFaultInstruction(f))
	}
	fmt.Fprintf(&b, "\nThe other %d anchor(s) quoted the parent correctly; extract everything "+
		"else the same way and leave every directive text unchanged.", r.AnchorsValid)
	return capDiagnostic(b.String(), correctionPromptCap)
}

func anchorFaultInstruction(f AnchorFaultBrief) string {
	lines := fmt.Sprintf("lines %d-%d", f.LineStart, f.LineEnd)
	entry := fmt.Sprintf("%s[%d]", f.Kind, f.Index)
	if f.Head != "" {
		entry += fmt.Sprintf(" (%q)", f.Head)
	}
	where := fmt.Sprintf("%s anchor %d, %s", entry, f.Anchor, lines)
	switch f.Class {
	case sidecar.FaultClassWhitespaceOnly:
		if f.Canonical != "" {
			return fmt.Sprintf("%s (whitespace-only — right words, reconstructed bytes): the parent's "+
				"bytes at %s are: %s", where, lines, yamlDoubleQuoted(f.Canonical))
		}
		return fmt.Sprintf("%s (whitespace-only — right words, reconstructed bytes; the words occur "+
			"more than once there): copy one occurrence from %s exactly as the file has it", where, lines)
	case sidecar.FaultClassOffByOne:
		return fmt.Sprintf("%s (off-by-one): the quote is right but sits at lines %d-%d — record that "+
			"range", where, f.FoundStart, f.FoundEnd)
	case sidecar.FaultClassRange:
		return fmt.Sprintf("%s (range): the parent has no such range — anchor to the lines the "+
			"directive actually states", where)
	case sidecar.FaultClassEmpty:
		if f.Anchor < 0 {
			return fmt.Sprintf("%s (no anchors): give it at least one anchor quoting the parent "+
				"lines it states", entry)
		}
		return fmt.Sprintf("%s (empty quote): quote the parent lines it states", where)
	default: // not-verbatim
		return fmt.Sprintf("%s (not verbatim): the quote is not at those lines — quote one contiguous "+
			"slice of %s as the file has it; never splice separate pieces together", where, lines)
	}
}

// yamlDoubleQuoted renders s as a YAML double-quoted scalar: escapes are
// explicit, so line breaks and indentation survive being read off a prompt
// and written back into a sidecar.
func yamlDoubleQuoted(s string) string {
	out, err := yaml.Marshal(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: yaml.DoubleQuotedStyle})
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimRight(string(out), "\n")
}

// parseErrorLine pulls the line number out of a YAML parse error.
//
// It reports ok=false rather than guessing, and the caller turns that into an
// empty correction prompt. A correction that cannot say WHERE is the thing the
// non-correctable guard exists to prevent.
func parseErrorLine(msg string) (int, bool) {
	m := parseLineRx.FindStringSubmatch(msg)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

var parseLineRx = regexp.MustCompile(`(?i)\bline (\d+)`)

func pluralKind(ds []LengthDefect) string {
	kinds := map[string]bool{}
	for _, d := range ds {
		kinds[d.Kind] = true
	}
	if len(kinds) == 1 && kinds["prohibition"] {
		return "prohibitions"
	}
	if len(kinds) > 1 {
		return "directives/prohibitions"
	}
	return "directives"
}
