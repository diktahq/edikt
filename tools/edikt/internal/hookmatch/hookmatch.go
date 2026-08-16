package hookmatch

// hookmatch — the write-time injection tier's matcher.
//
// Given a path being written, return the compiled directives scoped to it,
// with their grades PINNED at render time (never re-derived here — a consumer
// that re-derived grade could silently downgrade an invariant and nothing
// would report the difference).
//
// FAIL-OPEN, BUT NEVER SILENTLY
//
// This runs inside a PreToolUse hook. A matcher that hard-fails blocks the
// user's editor on a governance bug, which is a worse outcome than missing an
// injection — so every failure class returns "no directives" and lets the
// write through.
//
// That default has a specific danger, and it is the reason for Outcome. Once
// this is the ONLY channel for MUST-grade write-touch directives, a missing
// binary, a corrupt index, and a crafted path are BYTE-IDENTICAL to a clean
// zero-match at the hook's output. Fail-open stays; silent fail-open does not.
// So every result carries WHY it is empty, and the classes are distinct and
// journaled (INV-013: absence never renders as a pass).  edikt-guard:allow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/globmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/surfaces"
	"gopkg.in/yaml.v3"
)

// Outcome names WHY a match returned what it did.
//
// The empty cases are enumerated separately on purpose: "the index says
// nothing applies here" and "the index could not be read" are the same empty
// list and completely different facts.
type Outcome string

const (
	OutcomeMatched      Outcome = "matched"
	OutcomeNoMatch      Outcome = "no_match"      // index read fine; nothing covers this path
	OutcomeIndexMissing Outcome = "index_missing" // not compiled, or rolled back
	OutcomeIndexCorrupt Outcome = "index_corrupt" // present and unparseable
	OutcomeBadPath      Outcome = "path_rejected" // path could not be normalised
	OutcomeIndexEmpty   Outcome = "index_empty"   // compiled, but zero globs
)

// Suppressed reports whether this outcome means governance did not run, as
// opposed to genuinely not applying. This is the predicate the detectability
// surface is built on.
func (o Outcome) Suppressed() bool {
	switch o {
	case OutcomeIndexMissing, OutcomeIndexCorrupt, OutcomeBadPath, OutcomeIndexEmpty:
		return true
	}
	return false
}

// Entry is one graded directive as the hook consumes it.
type Entry struct {
	ID                    string   `yaml:"id" json:"id"`
	Grade                 string   `yaml:"grade" json:"grade"`
	Class                 string   `yaml:"class,omitempty" json:"class,omitempty"`
	Text                  string   `yaml:"text" json:"text"`
	Intent                string   `yaml:"intent,omitempty" json:"intent,omitempty"`
	FalsifyingObservation string   `yaml:"falsifying_observation,omitempty" json:"falsifying_observation,omitempty"`
	Reminders             []string `yaml:"reminders,omitempty" json:"reminders,omitempty"`
	Glob                  string   `yaml:"-" json:"glob"`
}

// Result is one match.
type Result struct {
	Outcome   Outcome `json:"outcome"`
	Detail    string  `json:"detail,omitempty"`
	Path      string  `json:"path"`
	NormPath  string  `json:"normalized_path"`
	IndexHash string  `json:"index_hash,omitempty"`
	Entries   []Entry `json:"entries"`
}

type indexFile struct {
	SchemaVersion int                `yaml:"schema_version" json:"schema_version"`
	Globs         map[string][]Entry `yaml:"globs" json:"globs"`
}

// NormalizePath resolves a path the way the matcher must see it (AC-3.4).
//
// TWO steps, and both are load-bearing:
//
//	Clean       collapses "a/../b" and "./x", so a literal path cannot be
//	            dressed up to miss a glob it should match.
//	EvalSymlinks resolves the link to its target, so a symlink whose LITERAL
//	            path escapes a governed glob but whose TARGET is inside it
//	            still matches.
//
// The disagreement case is the whole point. `docs/link.go -> tools/edikt/x.go`
// matches nothing as a literal and matches `tools/**/*.go` as a target; the
// inverse is a link inside a governed directory pointing outside it. Matching
// on the literal alone lets an agent write governed code through a link, which
// is a governance bypass with no error anywhere.
//
// A path that does not exist yet is NOT an error: Write creates new files, and
// that is the most common case at PreToolUse. It is Cleaned and returned.
func NormalizePath(root, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("empty path")
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)

	// Resolve through the DEEPEST EXISTING ANCESTOR, then re-append the part
	// that does not exist yet.
	//
	// EvalSymlinks on the full path fails for a file Write is about to create —
	// the common PreToolUse case — leaving the path unresolved while the ROOT
	// below is resolved. On macOS that alone breaks matching for every new
	// file: /var is a symlink to /private/var, so root becomes /private/var/…
	// and the path stays /var/…, filepath.Rel returns a "../.." escape, and the
	// matcher silently falls back to an absolute path that matches no glob.
	//
	// Silently. That is the shape of the bug that matters: not an error, just
	// governance that stops applying to newly created files.
	abs = resolveExisting(abs)

	// Report relative to the project root, because that is what the compiled
	// globs are written against. A path outside the root is returned absolute
	// and will simply match nothing — which is correct, not an error: editing
	// a file outside the project is not a governance event.
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return abs, nil
	}
	if rootResolved, rerr := filepath.EvalSymlinks(rootAbs); rerr == nil {
		rootAbs = rootResolved
	}
	if rel, rerr := filepath.Rel(rootAbs, abs); rerr == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel), nil
	}
	return filepath.ToSlash(abs), nil
}

// resolveExisting resolves symlinks as deeply as the filesystem allows,
// re-appending any trailing components that do not exist yet.
func resolveExisting(abs string) string {
	rest := ""
	cur := abs
	for i := 0; i < 64; i++ { // bounded: a pathological path must not spin
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs // reached the root without resolving anything
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
	return abs
}

// IndexPath is where the compiled directive index lives.
//
// RESOLVED THROUGH THE RENDER MANIFEST, not from the directory layout. The
// manifest is the contract a consumer reads INSTEAD of assuming where a
// surface sits; hardcoding the path here would mean a renamed surface silently
// stops matching, and a hook that matches nothing reports "no directives
// apply" — a suppression indistinguishable from a clean corpus.
//
// The conventional path remains the answer when no manifest exists at all (a
// project that has not compiled since the manifest landed) and when the
// manifest cannot be read — in the latter case the caller's own
// index-missing / index-corrupt taxonomy takes over from the returned path,
// which is where that failure is already reported with a type.
func IndexPath(root string) string {
	if p, err := surfaces.ResolveKind(root, surfaces.KindDirectiveIdx, ""); err == nil {
		return p
	}
	return filepath.Join(root, ".claude", "rules", "governance", "directive-index.yaml")
}

// indexCache is the on-disk JSON fast-path for loadIndex (F-058). YAMLSHA256
// is the same hash Match already computes over the raw YAML bytes — it is
// the cache's ONLY validity check, so a cache that predates the last compile
// is detected and bypassed rather than served stale.
type indexCache struct {
	YAMLSHA256 string    `json:"yaml_sha256"`
	Index      indexFile `json:"index"`
}

// indexCachePath sits under project-local .edikt/state/, matching where
// every other per-project runtime cache in this codebase lives (pending
// proposals, evidence-reads) — never under /tmp (world-writable, wiped on
// reboot) and never under the user-global ~/.edikt/state/ (this cache is
// keyed to one project's compiled index, not shared across projects).
func indexCachePath(root string) string {
	return filepath.Join(root, ".edikt", "state", "hook-index-cache.json")
}

// loadIndex is Match's ONLY source for the parsed index. `hook match` is a
// fresh OS process per invocation (spawned by the shell hook), so there is
// no in-process cache to keep warm across calls — F-058 measured 33ms of the
// 39.6ms per-call cost as this exact YAML parse. encoding/json is ~4x faster
// than yaml.v3 on the same bytes (benchmarked on the real 2.6MB directive
// index), so a persistent JSON mirror turns most calls into a JSON decode
// instead of a YAML one.
//
// SAFETY, NOT SPEED, IS THE INVARIANT. The cache is validated against the
// YAML content hash on every read (not mtime — a no-op recompile that
// rewrites identical bytes must not falsely invalidate, and a restored
// backup must not falsely validate). Any way this can go wrong — missing
// file, corrupt JSON, stale hash, unwritable directory — falls straight
// through to the YAML parse, which remains authoritative. A failed or
// skipped cache write NEVER affects this call's result, only the next
// call's speed.
func loadIndex(root string, raw []byte, yamlHash string) (indexFile, error) {
	if cb, err := os.ReadFile(indexCachePath(root)); err == nil {
		var c indexCache
		if json.Unmarshal(cb, &c) == nil && c.YAMLSHA256 == yamlHash {
			return c.Index, nil
		}
	}

	var idx indexFile
	if err := yaml.Unmarshal(raw, &idx); err != nil {
		return indexFile{}, err
	}

	writeIndexCacheBestEffort(root, yamlHash, idx)
	return idx, nil
}

// writeIndexCacheBestEffort refreshes the JSON cache after a YAML parse.
// Every failure is swallowed: a read-only .edikt/state/, a concurrent
// writer, a full disk — none of these are this call's problem, since this
// call already has its answer from the YAML it just parsed. Atomic via
// write-temp-then-rename so a concurrent reader (another `hook match`
// process racing this one) never observes a half-written cache file.
func writeIndexCacheBestEffort(root, yamlHash string, idx indexFile) {
	dir := filepath.Dir(indexCachePath(root))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	body, err := json.Marshal(indexCache{YAMLSHA256: yamlHash, Index: idx})
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "hook-index-cache-*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, indexCachePath(root)); err != nil {
		os.Remove(tmpPath)
	}
}

// Match returns the directives covering path.
//
// Never returns an error: every failure is an Outcome. A hook that has to
// handle both an error and an outcome will eventually handle one of them by
// ignoring it.
func Match(root, path string) Result {
	res := Result{Path: path, Outcome: OutcomeNoMatch}

	norm, err := NormalizePath(root, path)
	if err != nil {
		res.Outcome = OutcomeBadPath
		res.Detail = err.Error()
		return res
	}
	res.NormPath = norm

	raw, err := os.ReadFile(IndexPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			res.Outcome = OutcomeIndexMissing
			res.Detail = "no compiled directive index; run `bin/edikt gov compile`"
		} else {
			res.Outcome = OutcomeIndexCorrupt
			res.Detail = err.Error()
		}
		return res
	}
	sum := sha256.Sum256(raw)
	res.IndexHash = hex.EncodeToString(sum[:])

	idx, err := loadIndex(root, raw, res.IndexHash)
	if err != nil {
		res.Outcome = OutcomeIndexCorrupt
		res.Detail = err.Error()
		return res
	}
	if len(idx.Globs) == 0 {
		// Compiled but covering nothing. Distinct from no_match: no_match means
		// the index has coverage and this path is outside it; index_empty means
		// there is no coverage at all, and every path would "pass".
		res.Outcome = OutcomeIndexEmpty
		res.Detail = "directive index has zero globs"
		return res
	}

	globs := make([]string, 0, len(idx.Globs))
	for g := range idx.Globs {
		globs = append(globs, g)
	}
	sort.Strings(globs) // deterministic entry order

	seen := map[string]struct{}{}
	for _, g := range globs {
		if !globmatch.Match(g, norm) {
			continue
		}
		for _, e := range idx.Globs[g] {
			// One directive may be reachable through two globs. Dedup by id so
			// the reader is not told the same rule twice.
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

// FilterGrade narrows to one grade. Used to keep the two shims disjoint: the
// PreToolUse bounce carries MUST-grade only, PostToolUse advisory only.
func FilterGrade(in []Entry, grade string) []Entry {
	var out []Entry
	for _, e := range in {
		if e.Grade == grade {
			out = append(out, e)
		}
	}
	return out
}
