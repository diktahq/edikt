package verify

// requirements.go — the declared-id half of the criteria contract.
//
// A criterion may declare which spec requirements it satisfies. That
// declaration is DATA, and data that nothing validates is exactly the defect
// the declaration exists to close, one level up: a criteria file could name
// SR-999 forever, a coverage report would count it, and the requirement it was
// supposed to stand for would still be unclaimed.
//
// So the ids are checked against the spec the plan implements, at load, and a
// failure NAMES the ids rather than reporting a count — a count tells an
// author that something is wrong without telling them what.
//
// WHAT THIS DOES NOT ESTABLISH. That the criterion asserts anything about the
// requirement it names. A criterion may declare SR-007 and assert nothing
// about SR-007; no check here can tell, and none pretends to. This closes the
// unknown-id case only.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/projectpaths"
)

// planSpecIDRe pulls the spec id out of a plan id such as
// PLAN-SPEC-NNN-some-slug. Case-insensitive because plan ids in the wild carry
// both the upper-case and lower-case spellings of the spec segment.
var planSpecIDRe = regexp.MustCompile(`(?i)\bSPEC-(\d{1,4})\b`)

// declaration is one (criterion, id) pair, kept with its location so a
// rejection can say where to look.
type declaration struct {
	phase     string
	criterion string
	id        string
}

// declaredRequirements returns every id declared anywhere in the file, in file
// order.
func (c *CriteriaFile) declaredRequirements() []declaration {
	var out []declaration
	for _, p := range c.Phases {
		for _, cr := range p.Criteria {
			for _, id := range cr.Requirements {
				out = append(out, declaration{phase: p.ID, criterion: cr.ID, id: id})
			}
		}
	}
	return out
}

// validateDeclaredRequirements rejects a declared id the plan's spec does not
// define.
//
// It is a no-op when nothing is declared. That is deliberate and is the
// upgrade path: a criteria file written before this field existed declares
// nothing, resolves no spec, and loads exactly as it did before. The
// filesystem — and validatePlanMatchesFilename, the oracle check below — is
// only consulted once a file has made a claim.
//
// D391 (2026-09-09): validatePlanMatchesFilename used to run unconditionally
// from LoadCriteria, ahead of this function. That broke the same no-op
// upgrade path one level up: a criteria file that declares NO requirements
// at all, but whose `plan:` field has always disagreed with its own filename
// (a pre-existing, harmless spelling — nothing was ever resolved from it),
// failed to load for the first time on this phase's account, even though the
// field this phase adds was never touched. The filename check exists only to
// protect the spec-resolution this function does, so it now runs from
// inside here, after the same len(declared)==0 no-op guard.
func (c *CriteriaFile) validateDeclaredRequirements(criteriaPath string) error {
	declared := c.declaredRequirements()
	if len(declared) == 0 {
		return nil
	}

	for _, d := range declared {
		if strings.TrimSpace(d.id) == "" {
			return fmt.Errorf("phases[%s].criteria[%s].requirements: an empty id declares nothing",
				d.phase, d.criterion)
		}
	}

	if err := c.validatePlanMatchesFilename(criteriaPath); err != nil {
		return err
	}

	specID, ok := specIDFromPlan(c.Plan)
	if !ok {
		// Fail closed. The alternative is to accept ids against a spec that
		// was never identified, which is the declaration-nothing-validates
		// shape this file exists to prevent.
		return fmt.Errorf("plan %q names no SPEC id, so the %d declared requirement id(s) "+
			"(%s) cannot be checked against anything",
			c.Plan, len(declared), joinUnique(declared))
	}

	root := projectpaths.FindRoot(filepath.Dir(criteriaPath))
	if root == "" {
		return fmt.Errorf("no project root above %s, so the declared requirement id(s) (%s) "+
			"of %s cannot be checked", filepath.Dir(criteriaPath), joinUnique(declared), specID)
	}
	_, specsDir := projectpaths.ProductDirs(root)

	known, specPath, err := loadSpecIDs(specsDir, specID)
	if err != nil {
		return fmt.Errorf("%w — the declared requirement id(s) (%s) cannot be checked",
			err, joinUnique(declared))
	}

	var unknown []string
	seen := map[string]bool{}
	for _, d := range declared {
		if known[d.id] {
			continue
		}
		key := d.phase + "/" + d.criterion + "/" + d.id
		if seen[key] {
			continue
		}
		seen[key] = true
		unknown = append(unknown, fmt.Sprintf("phases[%s].criteria[%s] declares %s",
			d.phase, d.criterion, d.id))
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown requirement id(s) — %s defines no such id (%s):\n  %s",
			specID, specPath, strings.Join(unknown, "\n  "))
	}
	return nil
}

// specIDFromPlan derives the spec id a plan implements from the plan id.
func specIDFromPlan(plan string) (string, bool) {
	m := planSpecIDRe.FindStringSubmatch(plan)
	if m == nil {
		return "", false
	}
	return "SPEC-" + m[1], true
}

// planIDFromFilename derives the plan id a criteria file's own name asserts:
// PLAN-<id>-criteria.yaml → PLAN-<id>. It does not touch the "PLAN-" prefix —
// callers that want prefix-insensitive comparison strip it themselves — this
// only strips the fixed "-criteria.yaml" suffix every sidecar carries.
func planIDFromFilename(path string) string {
	return strings.TrimSuffix(filepath.Base(path), "-criteria.yaml")
}

// validatePlanMatchesFilename rejects a criteria file whose `plan:` field
// disagrees with the plan id its own filename asserts.
//
// Called only from validateDeclaredRequirements, and only once that function
// has already established something is declared — see its own doc comment
// (D391) for why running this unconditionally at load broke the upgrade path
// for files that never touch requirements: at all.
//
// This is the oracle validateDeclaredRequirements trusts: specIDFromPlan
// derives the spec id to check declared requirement ids against from c.Plan
// alone — a string read out of the very document being guarded — while the
// file itself is located by FILENAME (cmd/verify.go's locateCriteriaSidecar)
// and, before this check existed, the two were never compared. Same file,
// same filename, only `plan:` edited: declaring a requirement id under
// plan: PLAN-<spec-a>-x used to validate against spec A, but editing that
// one line to plan: PLAN-<spec-b>-y validated the identical declaration
// against spec B instead — a stale or copy-pasted `plan:` field, the
// ordinary failure mode across this corpus and not an adversarial one,
// silently redirects every declaration into another spec's id namespace and
// the loader reports success.
//
// Comparison strips an optional "PLAN-" prefix from BOTH sides before
// comparing: several live criteria files write the bare slug
// (`plan: v060-governance-accuracy`) under a filename that carries the
// prefix (PLAN-v060-governance-accuracy-criteria.yaml), and that is a
// legitimate spelling already in the corpus, not a mismatch to reject.
func (c *CriteriaFile) validatePlanMatchesFilename(path string) error {
	want := planIDFromFilename(path)
	if strings.TrimPrefix(c.Plan, "PLAN-") == strings.TrimPrefix(want, "PLAN-") {
		return nil
	}
	return fmt.Errorf("plan: %q does not match the file it was found in (%s implies plan id %q) — "+
		"any declared requirement id would be checked against whatever spec %q's plan: field names, "+
		"not the plan this file actually is",
		c.Plan, filepath.Base(path), want, c.Plan)
}

// specSidecar is the read-only slice of the spec sidecar this file needs.
// Decoded NON-strictly on purpose: the spec schema is owned elsewhere and
// grows fields on its own schedule, and rejecting a spec here because it
// carries a field this file does not read would turn an unrelated schema
// addition into a criteria-loading failure.
type specSidecar struct {
	ID           string `yaml:"id"`
	Requirements []struct {
		ID string `yaml:"id"`
	} `yaml:"requirements"`
	Protections []struct {
		ID string `yaml:"id"`
	} `yaml:"protections"`
}

// loadSpecIDs returns every id a spec defines — requirement ids AND protection
// ids, because a criterion may legitimately be the thing that holds a
// protection — plus the path they were read from, for the error message.
func loadSpecIDs(specsDir, specID string) (map[string]bool, string, error) {
	dir, err := findSpecDir(specsDir, specID)
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, "spec.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s sidecar: %w", specID, err)
	}
	var s specSidecar
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return nil, path, fmt.Errorf("parse %s: %w", path, err)
	}
	known := make(map[string]bool, len(s.Requirements)+len(s.Protections))
	for _, r := range s.Requirements {
		if r.ID != "" {
			known[r.ID] = true
		}
	}
	for _, p := range s.Protections {
		if p.ID != "" {
			known[p.ID] = true
		}
	}
	if len(known) == 0 {
		// INV-011: an empty read is not an empty spec until we say so out  // edikt-guard:allow
		// loud. Reporting "every id is unknown" from a sidecar that simply
		// failed to yield anything would be a confident wrong answer.
		return nil, path, fmt.Errorf("%s defines no requirement or protection ids", path)
	}
	return known, path, nil
}

// findSpecDir resolves <specsDir>/<specID>-<slug>/ (or <specsDir>/<specID>/).
//
// The prefix match is anchored on a separator, so an id cannot resolve to a
// directory whose id merely begins with the same digits — a two-digit id
// silently matching a three-digit neighbour would check declarations against a
// spec nobody named. An ambiguous match is an error rather than a first-wins pick:
// two directories claiming the same id is a corpus problem, and silently
// choosing one of them would validate against a spec the author did not mean.
func findSpecDir(specsDir, specID string) (string, error) {
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return "", fmt.Errorf("read specs dir %s: %w", specsDir, err)
	}
	var matches []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == specID || strings.HasPrefix(name, specID+"-") {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no %s directory under %s", specID, specsDir)
	case 1:
		return filepath.Join(specsDir, matches[0]), nil
	default:
		return "", fmt.Errorf("%d directories under %s claim %s (%s)",
			len(matches), specsDir, specID, strings.Join(matches, ", "))
	}
}

// joinUnique renders the declared ids for an error message, deduplicated and
// ordered so the text is stable between runs.
func joinUnique(ds []declaration) string {
	seen := map[string]bool{}
	var ids []string
	for _, d := range ds {
		if seen[d.id] {
			continue
		}
		seen[d.id] = true
		ids = append(ids, d.id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}
