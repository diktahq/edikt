package hookmatch

// governing.go — "what GOVERNS this path?", as distinct from "what does the
// write-time hook INJECT for this path?".
//
// Those were one question while delivery had one surface. ADR-079 gave it two —
// MUST injected from directive-index.yaml, advisory rendered into topic files —
// and every instrument that had been asking Match() the first question while
// meaning the second silently started under-reporting. Two gates caught it:
// check-review-coverage-equivalence (790 -> 238) and check-directive-scope-
// coverage (28 -> 37 gaps). They were right; the instrument was wrong.
//
// So the two questions now have two functions with two names, and the name is
// the fix: a future reader choosing between them has to state which one they
// mean, which is the step that was skipped.
//
// SAME MATCHER, DELIBERATELY. Resolution runs through globmatch.Match over the
// map's glob keys — the identical call Match makes over the index's glob keys,
// against the identical NormalizePath output. check-review-coverage-equivalence
// records what a second matcher costs: an earlier hand-rolled fnmatch loop in
// that gate disagreed with the real matcher by 67 directives (723 vs 790). A
// coverage instrument that measures with a different matcher than the thing it
// certifies reports on a corpus nobody reviews.
//
// NO TEXT. delivery-map.yaml carries identity, grade, topic and destination —
// never directive text, which lives only on the surface that delivers it. Both
// consumers were checked to need no text before this was written.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/diktahq/edikt/tools/edikt/internal/surfaces"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/globmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
)

// GoverningEntry is one directive's routing record for a path.
type GoverningEntry struct {
	ID      string `yaml:"id" json:"id"`
	Grade   string `yaml:"grade" json:"grade"`
	Class   string `yaml:"class,omitempty" json:"class,omitempty"`
	Topic   string `yaml:"topic,omitempty" json:"topic,omitempty"`
	Surface string `yaml:"surface" json:"surface"`
	Glob    string `yaml:"-" json:"glob"`
}

// GoverningResult mirrors Result's shape, including its outcome vocabulary, so
// a caller can treat "no map" and "no index" the same way rather than learning
// a second failure taxonomy.
type GoverningResult struct {
	Outcome  Outcome          `json:"outcome"`
	Detail   string           `json:"detail,omitempty"`
	Path     string           `json:"path"`
	NormPath string           `json:"normalized_path"`
	MapHash  string           `json:"map_hash,omitempty"`
	Entries  []GoverningEntry `json:"entries"`
}

type deliveryMapFile struct {
	SchemaVersion int                         `yaml:"schema_version"`
	Globs         map[string][]GoverningEntry `yaml:"globs"`
}

// DeliveryMapPath resolves the delivery map through the render manifest,
// falling back to its conventional location when no manifest resolves —
// the same indirection the retired index reader used, so a consumer follows
// a renamed surface instead of finding nothing (AC-4.8).
func DeliveryMapPath(root string) string {
	if p, err := surfaces.ResolveKind(root, surfaces.KindDeliveryMap, ""); err == nil {
		return p
	}
	return filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml")
}

// ManifestPath resolves the render manifest's own conventional location.
// Deliberately a hookmatch function rather than a literal in internal/hook/:
// SP-004/SAC-003 forbid internal/hook/ (outside adapter/) from naming a host
// vendor directly, and ".claude" is exactly that name (AC-4.4).
func ManifestPath(root string) string {
	return filepath.Join(root, ".claude", "rules", "governance", phaseb.ManifestName)
}

// MatchGoverning reports every directive that governs path, on any surface.
//
// It FAILS LOUDLY where Match fails open. Match is on the write-time path,
// where blocking an editor because governance is broken is worse than missing
// an injection. This function is on the measurement path, where a silent zero
// is the defect: a coverage gate that reads "no map" as "nothing governs this"
// reports a clean corpus forever. Callers get an outcome they must inspect.
func MatchGoverning(root, path string) GoverningResult {
	res := GoverningResult{Path: path, Outcome: OutcomeNoMatch}

	norm, err := NormalizePath(root, path)
	if err != nil {
		res.Outcome = OutcomeBadPath
		res.Detail = err.Error()
		return res
	}
	res.NormPath = norm

	raw, err := os.ReadFile(DeliveryMapPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			res.Outcome = OutcomeIndexMissing
			res.Detail = "no compiled delivery map; run `edikt gov compile`"
		} else {
			res.Outcome = OutcomeIndexCorrupt
			res.Detail = err.Error()
		}
		return res
	}
	sum := sha256.Sum256(raw)
	res.MapHash = hex.EncodeToString(sum[:])

	var dm deliveryMapFile
	if err := yaml.Unmarshal(raw, &dm); err != nil {
		res.Outcome = OutcomeIndexCorrupt
		res.Detail = fmt.Sprintf("delivery map does not parse: %v", err)
		return res
	}
	if len(dm.Globs) == 0 {
		res.Outcome = OutcomeIndexEmpty
		res.Detail = "delivery map has zero globs"
		return res
	}

	globs := make([]string, 0, len(dm.Globs))
	for g := range dm.Globs {
		globs = append(globs, g)
	}
	sort.Strings(globs) // deterministic entry order, as in Match

	seen := map[string]struct{}{}
	for _, g := range globs {
		if !globmatch.Match(g, norm) {
			continue
		}
		for _, e := range dm.Globs[g] {
			// One directive may be reachable through two globs. Dedup by id so
			// a coverage count never double-counts a rule — the same reason
			// Match dedups, for a different consequence.
			if _, dup := seen[e.ID]; dup {
				continue
			}
			seen[e.ID] = struct{}{}
			e.Glob = g
			res.Entries = append(res.Entries, e)
		}
	}
	if len(res.Entries) > 0 {
		res.Outcome = OutcomeMatched
	}
	return res
}

// FilterSurface narrows to one delivery surface.
func FilterSurface(in []GoverningEntry, surface string) []GoverningEntry {
	var out []GoverningEntry
	for _, e := range in {
		if e.Surface == surface {
			out = append(out, e)
		}
	}
	return out
}
