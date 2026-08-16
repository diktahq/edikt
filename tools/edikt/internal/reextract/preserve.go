package reextract

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
func applyPin(d *sidecar.Directive, p pin) bool {
	changed := false
	set := func(dst *string, val string) {
		if val != "" && *dst != val {
			*dst = val
			changed = true
		}
	}
	set(&d.Verify, p.verify)
	set(&d.VerifyKind, p.verifyKind)
	set(&d.HumanApprovedAt, p.humanApprovedAt)
	set(&d.PositiveFixturePath, p.positiveFixture)
	set(&d.NegativeFixturePath, p.negativeFixture)
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
		return res, nil
	}
	if before == nil {
		return res, nil
	}
	after, err := sidecar.Load(afterPath)
	if err != nil {
		return res, fmt.Errorf("load regenerated sidecar %s: %w", afterPath, err)
	}

	changed := false

	// ---- top-level approved scope ------------------------------------------
	if len(before.Paths) > 0 && !sameStrings(before.Paths, after.Paths) {
		after.Paths = append([]string(nil), before.Paths...)
		sort.Strings(after.Paths)
		res.PathsRestored = true
		changed = true
	}
	if before.PathsApproval != nil && after.PathsApproval == nil {
		cp := *before.PathsApproval
		after.PathsApproval = &cp
		res.ApprovalRestored = true
		changed = true
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
			if applyPin(&after.Directives[i], p) {
				res.DirectivePins++
				changed = true
			}
			continue
		}
		n := lossless.NormalizeNounPhrase(bd.Text)
		if cands, ok := byNorm[n]; ok && len(cands) == 1 {
			if applyPin(&after.Directives[cands[0]], p) {
				res.DirectivePins++
				res.MatchedByNormForm++
				changed = true
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
