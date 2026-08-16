package hookmatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const cacheTestIndexYAML = `
schema_version: 1
globs:
  "tools/**/*.go":
    - id: "INV-902:d01"
      grade: must
      text: "cache-covered directive"
      reminders: []
`

// TestLoadIndex_CacheMissFallsBackAndPopulates is F-058's baseline: no cache
// on disk yet, so the first call must parse the YAML directly (same result
// as before this change existed) and leave a usable JSON cache behind for
// the next call.
func TestLoadIndex_CacheMissFallsBackAndPopulates(t *testing.T) {
	root := t.TempDir()
	writeIndex(t, root, cacheTestIndexYAML)

	res := Match(root, "tools/edikt/x.go")
	if res.Outcome != OutcomeMatched || len(res.Entries) != 1 {
		t.Fatalf("first call (cold cache) = %+v, want one matched entry", res)
	}

	cb, err := os.ReadFile(indexCachePath(root))
	if err != nil {
		t.Fatalf("cache was not written after a cold-cache call: %v", err)
	}
	var c indexCache
	if err := json.Unmarshal(cb, &c); err != nil {
		t.Fatalf("written cache is not valid JSON: %v", err)
	}
	if c.YAMLSHA256 != res.IndexHash {
		t.Errorf("cache yaml_sha256 = %s, want %s (must match the hash Match reported)", c.YAMLSHA256, res.IndexHash)
	}
	if len(c.Index.Globs["tools/**/*.go"]) != 1 {
		t.Errorf("cached index missing the glob entry: %+v", c.Index)
	}
}

// TestLoadIndex_WarmCacheServesIdenticalResult is the cache's whole reason
// to exist: once populated, a second call against the SAME index bytes must
// return byte-for-byte the same match result as the cold-cache call did —
// the fast path must not silently diverge from the source of truth it
// mirrors.
func TestLoadIndex_WarmCacheServesIdenticalResult(t *testing.T) {
	root := t.TempDir()
	writeIndex(t, root, cacheTestIndexYAML)

	cold := Match(root, "tools/edikt/x.go")
	if cold.Outcome != OutcomeMatched {
		t.Fatalf("cold call did not match: %+v", cold)
	}
	if _, err := os.Stat(indexCachePath(root)); err != nil {
		t.Fatalf("cache not present before warm call: %v", err)
	}

	warm := Match(root, "tools/edikt/x.go")
	if warm.Outcome != cold.Outcome {
		t.Errorf("warm outcome = %s, want %s", warm.Outcome, cold.Outcome)
	}
	if warm.IndexHash != cold.IndexHash {
		t.Errorf("warm index hash = %s, want %s (hash must come from YAML bytes regardless of cache use)", warm.IndexHash, cold.IndexHash)
	}
	if len(warm.Entries) != len(cold.Entries) || warm.Entries[0].ID != cold.Entries[0].ID {
		t.Errorf("warm entries = %+v, want %+v", warm.Entries, cold.Entries)
	}
}

// TestLoadIndex_StaleCacheIsBypassed is the safety property the whole
// design rests on: a cache written for an OLDER index must never be served
// for a NEWER one. Detected by content hash, not mtime, so a same-content
// recompile does not falsely invalidate and a backup restore does not
// falsely validate.
func TestLoadIndex_StaleCacheIsBypassed(t *testing.T) {
	root := t.TempDir()
	writeIndex(t, root, cacheTestIndexYAML)

	first := Match(root, "tools/edikt/x.go")
	if first.Outcome != OutcomeMatched {
		t.Fatalf("first call did not match: %+v", first)
	}

	// Recompile: the index now covers a DIFFERENT path, with the old path
	// dropped. If the stale cache were served, this would still report
	// OutcomeMatched for the old path — the exact bug this test rules out.
	writeIndex(t, root, `
schema_version: 1
globs:
  "docs/**/*.md":
    - id: "INV-903:d01"
      grade: must
      text: "a completely different directive"
      reminders: []
`)

	after := Match(root, "tools/edikt/x.go")
	if after.Outcome == OutcomeMatched {
		t.Fatalf("stale cache served after recompile: got Matched for a path the new index no longer covers: %+v", after)
	}
	afterNew := Match(root, "docs/readme.md")
	if afterNew.Outcome != OutcomeMatched {
		t.Fatalf("new index's own path did not match after recompile: %+v", afterNew)
	}
}

// TestLoadIndex_CorruptCacheFallsBackToYAML proves the cache can never make
// things WORSE than not having one: any way the cache file can be broken —
// truncated, not JSON, missing the field entirely — must fall straight
// through to the YAML parse rather than erroring the whole match.
func TestLoadIndex_CorruptCacheFallsBackToYAML(t *testing.T) {
	root := t.TempDir()
	writeIndex(t, root, cacheTestIndexYAML)

	if err := os.MkdirAll(filepath.Dir(indexCachePath(root)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexCachePath(root), []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := Match(root, "tools/edikt/x.go")
	if res.Outcome != OutcomeMatched || len(res.Entries) != 1 {
		t.Fatalf("match with a corrupt cache present = %+v, want the YAML fallback to still succeed", res)
	}
}
