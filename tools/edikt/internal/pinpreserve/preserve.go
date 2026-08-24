package pinpreserve

// preserve.go — carrying human-pinned state across a regeneration.
//
// WHY THIS IS NOT THE EXTRACTOR'S JOB
//
// The extractor is a locked prompt with one input (the artifact's prose) and
// one output (a sidecar). It never sees the previous sidecar, deliberately: an
// extractor that read its own prior output would be free to reproduce a past
// mistake forever, and every measurement of extraction quality would be
// measuring a conversation rather than a contract.
//
// The consequence is that a regeneration WIPES anything a human pinned —
// approved `paths:`, an approved `verify:`, the `human_approved_at` stamp that
// records the ceremony happened. On the first full-corpus run this destroyed
// 48 pinned fields across 20 artifacts, and AC-4.4's survival gate is what
// caught it. The preservation therefore belongs HERE, in the tier-2 code that
// owns the before-and-after: capture, dispatch, restore.
//
// WHAT IS RESTORED, AND WHAT IS REPORTED INSTEAD
//
// Top-level `paths:` and `paths_approval:` restore verbatim. They belong to
// the artifact, not to any one directive, so nothing has to be matched up.
// Newly inferred scope still arrives through `proposed_paths` and still goes
// to the ceremony — restoring the approved set does not approve anything new.
//
// Per-directive pins (`verify`, `verify_kind`, `human_approved_at`,
// `positive_fixture_path`, `negative_fixture_path`) need to find their
// directive again, and the v4 contract re-words directives by design — one
// rule split across two sentences becomes one directive with two anchors. So
// matching runs exact-text first, then a normalized noun-phrase match that
// must be UNAMBIGUOUS: exactly one candidate, or the pin is not restored.
//
// AN UNRESTORABLE PIN IS REPORTED, NEVER GUESSED. Attaching an approved verify
// to the wrong directive would be worse than losing it: the sidecar would
// claim a human approved a command for a rule they never saw it against. The
// caller surfaces these for re-approval.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/lossless"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// PinnedField names one human-pinned value that could not be carried forward.
type PinnedField struct {
	ArtifactID string `json:"artifact_id"`
	Field      string `json:"field"`
	// DirectiveText is the pre-regeneration directive the pin belonged to.
	DirectiveText string `json:"directive_text"`
	Value         string `json:"value"`
	Reason        string `json:"reason"`
}

func (p PinnedField) String() string {
	return fmt.Sprintf("%s %s on %q: %s", p.ArtifactID, p.Field, truncate(p.DirectiveText, 80), p.Reason)
}

// PreserveResult reports what a restore carried and what it could not.
type PreserveResult struct {
	PathsRestored     bool          `json:"paths_restored"`
	ApprovalRestored  bool          `json:"approval_restored"`
	DirectivePins     int           `json:"directive_pins_restored"`
	Unrestorable      []PinnedField `json:"unrestorable,omitempty"`
	SidecarRewritten  bool          `json:"sidecar_rewritten"`
	MatchedByNormForm int           `json:"matched_by_normalized_form"`
	// LoadFailed is true when the PRIOR sidecar existed on disk but could not
	// be parsed — a categorically different case from "no prior sidecar",
	// which PreservePinned must never silently treat the same way. See
	// pinnedFieldNamesAtRisk.
	LoadFailed bool `json:"load_failed"`
}

// pinnedFieldNamesAtRisk is every field a human can pin that this package
// restores. Named as one list, not scattered across call sites, so a load
// failure's report and a field addition here can never drift apart: adding a
// new pinnable field to `pin`/`pinOf` without adding it here would silently
// under-report what a load failure put at risk.
var pinnedFieldNamesAtRisk = []string{
	"paths",
	"paths_approval",
	"verify",
	"verify_kind",
	"human_approved_at",
	"positive_fixture_path",
	"negative_fixture_path",
}

// pinnedOf extracts the pinned fields of one directive.
type pin struct {
	verify          string
	verifyKind      string
	humanApprovedAt string
	positiveFixture string
	negativeFixture string
}

func (p pin) empty() bool {
	return p.verify == "" && p.verifyKind == "" && p.humanApprovedAt == "" &&
		p.positiveFixture == "" && p.negativeFixture == ""
}

func pinOf(d sidecar.Directive) pin {
	return pin{
		verify:          d.Verify,
		verifyKind:      d.VerifyKind,
		humanApprovedAt: d.HumanApprovedAt,
		positiveFixture: d.PositiveFixturePath,
		negativeFixture: d.NegativeFixturePath,
	}
}

// applyPin writes the pinned values and reports whether anything CHANGED.
//
// The distinction matters for the count, not just for the write: a restore
// that reported 11 artifacts every time it ran — including runs where all 11
// were already correct — is reporting work it did not do, and a number that
// cannot go to zero is one nobody can use to tell whether the repair is
// finished.
func applyPin(d *sidecar.Directive, p pin) []string {
	var changed []string
	set := func(name string, dst *string, val string) {
		if val != "" && *dst != val {
			*dst = val
			changed = append(changed, name)
		}
	}
	set("verify", &d.Verify, p.verify)
	set("verify_kind", &d.VerifyKind, p.verifyKind)
	set("human_approved_at", &d.HumanApprovedAt, p.humanApprovedAt)
	set("positive_fixture_path", &d.PositiveFixturePath, p.positiveFixture)
	set("negative_fixture_path", &d.NegativeFixturePath, p.negativeFixture)
	return changed
}

// PreservePinned restores human-pinned state from `before` into the sidecar at
// afterPath, writing it back when anything changed.
//
// before is the sidecar as it stood BEFORE the regeneration. A nil before
// means there was nothing to preserve, which is the ordinary bootstrap case —
// UNLESS beforeLoadErr is non-nil, in which case before is nil for a
// completely different reason: a prior sidecar existed on disk but failed to
// parse. Those two are not the same claim. "No prior sidecar" means there was
// genuinely nothing pinned. "Prior sidecar unreadable" means there might have
// been anything — approved paths, a human-approved verify, all of it — and
// this code cannot see it. Silently treating the second as the first is
// exactly the swallow this function exists to prevent for every OTHER shape
// of loss; it must not reintroduce the same shape here.
//
// beforeLoadErr takes priority over before being nil — the caller
// (discover.go via reextract.go) sets Sidecar and LoadErr as mutually
// exclusive, but this function does not trust that invariant blindly: a
// non-nil beforeLoadErr always means "refuse to proceed as if empty",
// regardless of what before itself contains.
func PreservePinned(artifactID string, before *sidecar.Sidecar, beforeLoadErr error, afterPath string) (*PreserveResult, error) {
	if res, done := earlyResult(artifactID, before, beforeLoadErr); done {
		return res, nil
	}
	after, err := sidecar.Load(afterPath)
	if err != nil {
		return &PreserveResult{}, fmt.Errorf("load regenerated sidecar %s: %w", afterPath, err)
	}
	res, _, changed := computePins(artifactID, before, after)
	if !changed {
		return res, nil
	}
	if err := after.Validate(); err != nil {
		return res, fmt.Errorf("%s: restored sidecar fails validation: %w", artifactID, err)
	}
	body, err := sidecar.Marshal(after)
	if err != nil {
		return res, fmt.Errorf("%s: marshal restored sidecar: %w", artifactID, err)
	}
	if err := os.WriteFile(afterPath, body, 0o644); err != nil {
		return res, fmt.Errorf("%s: write restored sidecar: %w", artifactID, err)
	}
	res.SidecarRewritten = true
	return res, nil
}

// earlyResult handles the two cases that never touch the regenerated
// sidecar at all: a prior sidecar that existed but would not parse (every
// pinnable field is at risk and none can be checked), and a genuinely
// absent prior sidecar (nothing was pinned, so nothing can be lost). done
// is false only when there is a real `before` to compare against.
func earlyResult(artifactID string, before *sidecar.Sidecar, beforeLoadErr error) (*PreserveResult, bool) {
	res := &PreserveResult{}
	if beforeLoadErr != nil {
		res.LoadFailed = true
		for _, field := range pinnedFieldNamesAtRisk {
			res.Unrestorable = append(res.Unrestorable, PinnedField{
				ArtifactID: artifactID,
				Field:      field,
				Reason: fmt.Sprintf(
					"the prior sidecar existed but failed to load (%v) — could not check whether this field was pinned before regeneration; review the prior sidecar manually (git history or .edikt/state/reextract-snapshots/) before treating this artifact's re-extraction as final",
					beforeLoadErr,
				),
			})
		}
		return res, true
	}
	if before == nil {
		return res, true
	}
	return res, false
}

// computePins is the one comparison both the mutating and the read-only
// entry points run. It mutates `after` IN MEMORY only — writing it back is
// PreservePinned's job and MissingPins deliberately never does it, which is
// what lets `phasea verify` reuse this logic while performing no writes
// (SPEC-013 D6: the mutation and the gate are separate verbs).
//
// It returns three things: the PreserveResult (carrying Unrestorable — pins
// that could NOT be reattached to any directive), the list of pins that WERE
// reinstated (i.e. fields the live sidecar was missing), and whether
// anything changed at all. A caller checking survival treats both lists as
// "not present on disk": Unrestorable can never be restored, and a non-empty
// reinstated list means the live file did not carry the pin when it was read.
func computePins(artifactID string, before, after *sidecar.Sidecar) (*PreserveResult, []PinnedField, bool) {
	res := &PreserveResult{}
	var reinstated []PinnedField
	changed := false

	// ---- top-level approved scope ------------------------------------------
	if len(before.Paths) > 0 && !sameStrings(before.Paths, after.Paths) {
		after.Paths = append([]string(nil), before.Paths...)
		sort.Strings(after.Paths)
		res.PathsRestored = true
		changed = true
		reinstated = append(reinstated, PinnedField{
			ArtifactID: artifactID,
			Field:      "paths",
			Value:      strings.Join(before.Paths, ", "),
			Reason:     "the regenerated sidecar did not carry the approved paths",
		})
	}
	if before.PathsApproval != nil && after.PathsApproval == nil {
		cp := *before.PathsApproval
		after.PathsApproval = &cp
		res.ApprovalRestored = true
		changed = true
		reinstated = append(reinstated, PinnedField{
			ArtifactID: artifactID,
			Field:      "paths_approval",
			Value:      before.PathsApproval.ApprovedAt,
			Reason:     "the regenerated sidecar did not carry the approval receipt",
		})
	}

	// A restored receipt must still describe the restored globs. If the
	// approval's hash no longer covers them, the receipt is reported as
	// unrestorable rather than re-stamped: re-stamping would be this code
	// approving a scope on a human's behalf.
	if after.PathsApproval != nil && sidecar.HashGlobs(after.Paths) != after.PathsApproval.GlobsSHA256 {
		res.Unrestorable = append(res.Unrestorable, PinnedField{
			ArtifactID: artifactID,
			Field:      "paths_approval",
			Value:      after.PathsApproval.ApprovedAt,
			Reason:     "the approval receipt's hash does not cover the restored globs — re-approve with `bin/edikt sidecar approve --kind paths`",
		})
	}

	// ---- per-directive pins ------------------------------------------------
	byText := map[string]int{}
	byNorm := map[string][]int{}
	for i, d := range after.Directives {
		byText[d.Text] = i
		n := lossless.NormalizeNounPhrase(d.Text)
		if n != "" {
			byNorm[n] = append(byNorm[n], i)
		}
	}

	for _, bd := range before.Directives {
		p := pinOf(bd)
		if p.empty() {
			continue
		}
		if i, ok := byText[bd.Text]; ok {
			if fields := applyPin(&after.Directives[i], p); len(fields) > 0 {
				res.DirectivePins++
				changed = true
				reinstated = append(reinstated, reinstatedFields(artifactID, bd.Text, p, fields)...)
			}
			continue
		}
		n := lossless.NormalizeNounPhrase(bd.Text)
		if cands, ok := byNorm[n]; ok && len(cands) == 1 {
			if fields := applyPin(&after.Directives[cands[0]], p); len(fields) > 0 {
				res.DirectivePins++
				res.MatchedByNormForm++
				changed = true
				reinstated = append(reinstated, reinstatedFields(artifactID, bd.Text, p, fields)...)
			}
			continue
		}
		// AMBIGUOUS OR ABSENT. Reported for re-approval, never attached to a
		// best guess: a sidecar claiming a human approved a command against a
		// rule they never saw it on is worse than one missing the command.
		reason := "the directive it was pinned to has no counterpart after regeneration"
		if cands, ok := byNorm[n]; ok && len(cands) > 1 {
			reason = fmt.Sprintf("%d directives now match its subject — attaching it to one would invent an approval", len(cands))
		}
		for field, val := range map[string]string{
			"verify":                p.verify,
			"verify_kind":           p.verifyKind,
			"human_approved_at":     p.humanApprovedAt,
			"positive_fixture_path": p.positiveFixture,
			"negative_fixture_path": p.negativeFixture,
		} {
			if val == "" {
				continue
			}
			res.Unrestorable = append(res.Unrestorable, PinnedField{
				ArtifactID:    artifactID,
				Field:         field,
				DirectiveText: bd.Text,
				Value:         val,
				Reason:        reason,
			})
		}
	}
	sort.Slice(res.Unrestorable, func(i, j int) bool {
		if res.Unrestorable[i].DirectiveText != res.Unrestorable[j].DirectiveText {
			return res.Unrestorable[i].DirectiveText < res.Unrestorable[j].DirectiveText
		}
		return res.Unrestorable[i].Field < res.Unrestorable[j].Field
	})

	sortPinnedFields(reinstated)
	return res, reinstated, changed
}

// reinstatedFields renders the field names applyPin actually wrote back into
// PinnedField records, so a read-only caller can NAME what the live sidecar
// was missing rather than merely count it (AC-3.5: verify "names the field").
func reinstatedFields(artifactID, directiveText string, p pin, fields []string) []PinnedField {
	values := map[string]string{
		"verify":                p.verify,
		"verify_kind":           p.verifyKind,
		"human_approved_at":     p.humanApprovedAt,
		"positive_fixture_path": p.positiveFixture,
		"negative_fixture_path": p.negativeFixture,
	}
	out := make([]PinnedField, 0, len(fields))
	for _, f := range fields {
		out = append(out, PinnedField{
			ArtifactID:    artifactID,
			Field:         f,
			DirectiveText: directiveText,
			Value:         values[f],
			Reason:        "the regenerated sidecar did not carry this pinned value",
		})
	}
	return out
}

func sortPinnedFields(fs []PinnedField) {
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].DirectiveText != fs[j].DirectiveText {
			return fs[i].DirectiveText < fs[j].DirectiveText
		}
		return fs[i].Field < fs[j].Field
	})
}

// MissingPins is PreservePinned's READ-ONLY sibling: it reports every pinned
// field from `before` that the sidecar at afterPath does not currently carry,
// and writes nothing anywhere.
//
// It exists because SPEC-013's D6 seam splits the restore from the gate.
// `bin/edikt phasea restore` performs the mutation; `bin/edikt phasea verify`
// only observes, and an observer that could not tell a surviving pin from a
// lost one would have to either re-run the mutation (collapsing the seam) or
// trust the mutation's own report of itself (the exact "did the write happen"
// fusion the seam exists to undo). Both returned classes mean the same thing
// to a gate — the pin is not on disk — but they are kept distinguishable by
// Reason: an Unrestorable pin can never be reattached, while a reinstated one
// simply had not been restored yet when verify looked.
func MissingPins(artifactID string, before *sidecar.Sidecar, beforeLoadErr error, afterPath string) ([]PinnedField, error) {
	if res, done := earlyResult(artifactID, before, beforeLoadErr); done {
		return res.Unrestorable, nil
	}
	after, err := sidecar.Load(afterPath)
	if err != nil {
		return nil, fmt.Errorf("load regenerated sidecar %s: %w", afterPath, err)
	}
	res, reinstated, _ := computePins(artifactID, before, after)
	out := append(append([]PinnedField(nil), res.Unrestorable...), reinstated...)
	sortPinnedFields(out)
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, "\x00") == strings.Join(y, "\x00")
}
