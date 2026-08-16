// Package precondition answers one question about every `verify:` command
// in the corpus: do the paths it references actually exist?
//
// It exists because "precondition absent" and "rule violated" are currently
// the same observation. A verify command asserts that a rule holds; when it
// names a file that is not in the tree, it exits non-zero for a reason that
// has nothing to do with the rule, and internal/verify records `failed` —
// the identical status a genuine violation produces.
//
// That is not hypothetical. Greenfield extraction emitted
// `rg -q '…' internal/rag/chunk.go` for a file absent from the tree; rg
// exited non-zero, the criterion scored FAIL, and COMPILE_EXIT was 1. A
// missing file was reported as a governance violation, and nothing in the
// output could have told the two apart.
//
// This check runs BEFORE execution and needs none: it reads the verify
// string and stats the tree. Read-only, no subprocess, no LLM. It reuses
// sidecar.Discover — the same corpus walker grounding and normative use —
// so all three report against one denominator.
package precondition

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Verdict is the state of ONE verify command's preconditions.
type Verdict string

const (
	// Satisfied — every path operand resolved and exists.
	Satisfied Verdict = "satisfied"

	// Absent — at least one referenced path is not in the tree. The
	// command cannot report on its rule; whatever it exits with is about
	// the missing file. THIS IS THE STATE THE RUNNER CANNOT SEE.
	Absent Verdict = "precondition_absent"

	// NoPathOperands — the command references no path this check can
	// identify (`bin/edikt gov benchmark cheat-rate --help`, a pure
	// pipeline over stdin). Nothing to check, so nothing is claimed.
	NoPathOperands Verdict = "no_path_operands"

	// Unresolvable — every operand carries a shell expansion, a glob, a
	// `~`, or an absolute path. Reported on its own and never folded into
	// Satisfied: an operand nobody resolved is not an operand somebody
	// found (INV-013).
	Unresolvable Verdict = "unresolvable"
)

// Finding names one verify command whose preconditions are not satisfied.
type Finding struct {
	ArtifactID string   `json:"artifact_id"`
	Kind       string   `json:"kind"`
	Index      int      `json:"index"`
	Verdict    Verdict  `json:"verdict"`
	Missing    []string `json:"missing,omitempty"`
	Unresolved []string `json:"unresolved,omitempty"`
	Verify     string   `json:"verify"`
}

// Report is the corpus-wide result.
type Report struct {
	PairsDiscovered int      `json:"pairs_discovered"`
	SkipListed      int      `json:"skip_listed"`
	NoSidecar       int      `json:"no_sidecar"`
	SidecarsScanned int      `json:"sidecars_scanned"`
	Unreadable      []string `json:"unreadable,omitempty"`

	// TotalItems is every directive and prohibition. VerifyCommands is the
	// subset carrying a verify:. Both are reported because "3 absent" means
	// nothing without knowing it is 3 of 46 verifies out of 712 items.
	TotalItems     int `json:"total_items"`
	VerifyCommands int `json:"verify_commands"`

	ByVerdict     map[Verdict]int `json:"by_verdict"`
	PathsChecked  int             `json:"paths_checked"`
	PathsMissing  int             `json:"paths_missing"`
	PathsDeclined int             `json:"paths_declined"`

	Findings []Finding `json:"findings,omitempty"`
}

// Scan walks the artifact directories under projectRoot and checks the
// preconditions of every verify command it finds.
func Scan(projectRoot string, dirs []string) (*Report, error) {
	pairs, err := sidecar.Discover(projectRoot, dirs)
	if err != nil {
		return nil, fmt.Errorf("discover sidecars: %w", err)
	}
	rep := &Report{ByVerdict: map[Verdict]int{}}

	exists := func(rel string) bool {
		_, statErr := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(rel)))
		return statErr == nil
	}

	for _, p := range pairs {
		rep.PairsDiscovered++
		if p.Skip {
			rep.SkipListed++
			continue
		}
		if p.Sidecar == nil {
			if p.LoadErr != nil {
				rep.Unreadable = append(rep.Unreadable, p.ArtifactID)
			} else {
				rep.NoSidecar++
			}
			continue
		}
		rep.SidecarsScanned++

		for i, d := range p.Sidecar.Directives {
			rep.check(p.ArtifactID, "directive", i, d.Verify, exists)
		}
		for i, pr := range p.Sidecar.Prohibitions {
			rep.check(p.ArtifactID, "prohibition", i, pr.Verify, exists)
		}
		rep.TotalItems += len(p.Sidecar.Directives) + len(p.Sidecar.Prohibitions)
	}

	sort.Strings(rep.Unreadable)
	return rep, nil
}

// check classifies one item's verify and folds it into the report. Items
// with no verify: are not verify commands and are not counted as any
// verdict — they are simply outside this check's subject.
func (r *Report) check(id, kind string, idx int, verify string, exists func(string) bool) {
	if strings.TrimSpace(verify) == "" {
		return
	}
	r.VerifyCommands++
	v, missing, unresolved, checked, declined := CheckCommand(verify, exists)
	r.ByVerdict[v]++
	r.PathsChecked += checked
	r.PathsDeclined += declined
	r.PathsMissing += len(missing)

	if v == Satisfied || v == NoPathOperands {
		return
	}
	r.Findings = append(r.Findings, Finding{
		ArtifactID: id, Kind: kind, Index: idx, Verdict: v,
		Missing: missing, Unresolved: unresolved, Verify: verify,
	})
}

// CheckCommand classifies one verify command against a path-existence
// oracle. Exported so a caller can check a single command without a corpus
// — and so the classification is testable without touching the filesystem.
func CheckCommand(verify string, exists func(string) bool) (Verdict, []string, []string, int, int) {
	if strings.TrimSpace(verify) == "" {
		return NoPathOperands, nil, nil, 0, 0
	}
	ops := sidecar.ExtractVerifyPaths(verify)
	if len(ops) == 0 {
		return NoPathOperands, nil, nil, 0, 0
	}

	var missing, unresolved []string
	checked, declined := 0, 0
	for _, op := range ops {
		if op.Unresolvable {
			declined++
			unresolved = append(unresolved, op.Raw+" ("+op.Reason+")")
			continue
		}
		if op.AssertedAbsent {
			// The command asserts this path is GONE. Its absence is the
			// rule holding, so it is not a precondition at all — and
			// flagging it would invert the finding. ADR-044 verifies that
			// a file it deleted stays deleted.
			declined++
			unresolved = append(unresolved, op.Resolved()+" (absence is the assertion)")
			continue
		}
		checked++
		if !exists(op.Resolved()) {
			missing = append(missing, op.Resolved())
		}
	}
	switch {
	case len(missing) > 0:
		return Absent, missing, unresolved, checked, declined
	case checked == 0:
		return Unresolvable, nil, unresolved, checked, declined
	default:
		return Satisfied, nil, unresolved, checked, declined
	}
}

// Summary renders the result with its denominators and its ceiling.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Verify preconditions: %d of %d verify command(s) have an ABSENT precondition\n",
		r.ByVerdict[Absent], r.VerifyCommands)
	fmt.Fprintf(&b, "  corpus: %d pair(s) discovered = %d scanned + %d skip-listed + "+
		"%d without a sidecar + %d unreadable\n",
		r.PairsDiscovered, r.SidecarsScanned, r.SkipListed, r.NoSidecar, len(r.Unreadable))
	fmt.Fprintf(&b, "  %d verify command(s) across %d item(s)\n", r.VerifyCommands, r.TotalItems)
	fmt.Fprintf(&b, "  path operands: %d resolved and checked, %d MISSING, %d declined\n",
		r.PathsChecked, r.PathsMissing, r.PathsDeclined)

	kinds := make([]Verdict, 0, len(r.ByVerdict))
	for k := range r.ByVerdict {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	for _, k := range kinds {
		fmt.Fprintf(&b, "    %-24s %d\n", k, r.ByVerdict[k])
	}
	if n := len(r.Unreadable); n > 0 {
		fmt.Fprintf(&b, "  UNMEASURED: %d sidecar(s) would not load: %s\n",
			n, strings.Join(r.Unreadable, ", "))
	}
	b.WriteString("  A DECLINED operand is not a satisfied one — a glob, a shell expansion, an\n" +
		"  absolute path, a name-only predicate operand, or a path whose ABSENCE is the\n" +
		"  assertion. Nothing was checked about any of them.\n")
	b.WriteString("  ceiling: this proves a referenced path EXISTS IN THE CURRENT WORKING TREE. " +
		"A gitignored build artifact reads as present here and is absent in a fresh clone, " +
		"so this number is conditional on what has been built. It cannot tell whether the " +
		"command's assertion is correct, whether the path is the RIGHT one, or whether a " +
		"command with no path operands has preconditions of another kind (a binary on PATH, " +
		"a network service, an env var).\n")
	return b.String()
}
