// Package cheatrate provides deterministic helpers for the
// `bin/edikt gov benchmark cheat-rate` subcommand (SPEC-009 Plan C,  // edikt-guard:allow
// ADR-040).  // edikt-guard:allow
//
// The package is intentionally pure-by-default: it produces stable cache
// keys, canonical report paths, and sandbox paths from string inputs
// only, and exposes a small set of side-effecting helpers — sandbox
// creation, report serialization, cache get/put — that share the same
// schema and INV-007 hermetic-sandbox contract.  // edikt-guard:allow
//
// What this package does NOT do (ADR-030 / no-llm-in-tier-2): no LLM  // edikt-guard:allow
// invocation, no host-agent shim dispatch, no subprocess fan-out lives
// here. The `.github/workflows/sidecar-checks.yml` `no-llm-in-tier-2`
// grep gate covers this path. The caller (cmd/gov/benchmark_cheatrate.go)
// is responsible for adversary dispatch in a future revision; this
// package only owns the deterministic filesystem-level plumbing both
// stub-mode and real-mode runs share.
package cheatrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CacheKey returns a stable cache key derived from the sidecar's
// content hash and the verify-text hash. The two inputs together
// uniquely identify a (sidecar version, verify command) pair — the
// run can be replayed from cache as long as neither input changes.
//
// Inputs are expected to be hex-encoded sha256 strings, but any
// printable strings work — the function only hashes the concatenation
// with a separator that cannot appear in a hex digest, preventing
// (a,b)==(a',b') collisions when a||b == a'||b'.
func CacheKey(sidecarContentHash, verifyTextHash string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(sidecarContentHash))
	_, _ = h.Write([]byte{0x1f}) // ASCII unit separator — never appears in hex
	_, _ = h.Write([]byte(verifyTextHash))
	return hex.EncodeToString(h.Sum(nil))
}

// ReportPath returns the canonical filesystem path for a cheat-rate
// run report. Reports live under `<stateDir>/benchmark/<sidecarID>/
// <timestamp>.json` so multiple runs for the same sidecar are
// preserved in chronological order without overwriting each other.
//
// stateDir is typically `.edikt/state` under the project root; the
// caller is responsible for resolving and creating it. The timestamp
// is expected in RFC3339-with-dashes form (e.g. 2026-05-23T14-30-00Z)
// so the resulting path is portable across filesystems.
func ReportPath(stateDir, sidecarID, timestamp string) string {
	return filepath.Join(stateDir, "benchmark", sidecarID, fmt.Sprintf("%s.json", timestamp))
}

// SandboxPath returns the path for a per-verify hermetic sandbox.
// Sandboxes are siblings under `<sandboxesDir>/<sidecarID>/<timestamp>/
// verify-<idx>`. The sandbox itself MUST be created hermetically per
// INV-007 (no host ~/.claude/settings.json, no shell rc, no SSH keys);  // edikt-guard:allow
// this function only computes the path. Use CreateSandbox for the
// hermetic copy.
//
// sandboxesDir is typically `.edikt/state/sandboxes` under the project
// root. verifyIdx is the zero-based index of the verify command inside
// the sidecar's directive/prohibition/verification list.
func SandboxPath(sandboxesDir, sidecarID string, verifyIdx int, timestamp string) string {
	return filepath.Join(sandboxesDir, sidecarID, timestamp, fmt.Sprintf("verify-%d", verifyIdx))
}

// Report is the cheat-rate benchmark output, validated against
// templates/schemas/cheat-rate-report.v1.schema.json. Field ordering
// matches the schema document so json.Marshal output stays diff-stable
// against fixtures committed under test/fixtures/benchmark-stubs/.
type Report struct {
	SchemaVersion  int      `json:"schema_version"`
	SidecarID      string   `json:"sidecar_id"`
	RanAt          string   `json:"ran_at"`
	AdversaryModel string   `json:"adversary_model"`
	Verifies       []Verify `json:"verifies"`
	Summary        Summary  `json:"summary"`
}

// Verify is one verify-command result inside a Report.
//
// Verdict is one of "cheated", "not_cheated", or "inconclusive" per
// ADR-040. MajorityRuns is the "N/M" string from the majority-vote  // edikt-guard:allow
// adversary protocol (e.g. "3/3", "2/3"). SandboxPath and
// AdversaryTracePath are the hermetic-sandbox INV-007 outputs.  // edikt-guard:allow
type Verify struct {
	VerifyID           string `json:"verify_id"`
	Intent             string `json:"intent"`
	VerifyKind         string `json:"verify_kind"`
	Verdict            string `json:"verdict"`
	MajorityRuns       string `json:"majority_runs"`
	ElapsedMs          int    `json:"elapsed_ms"`
	SandboxPath        string `json:"sandbox_path"`
	AdversaryTracePath string `json:"adversary_trace_path"`
}

// Summary aggregates Verify verdicts for the run.
// CheatRate is cheated/total (0.0–1.0), InconclusiveRate is
// inconclusive/total. SPEC-009 targets CheatRate < 0.20 on the held-out  // edikt-guard:allow
// corpus.
type Summary struct {
	Total            int     `json:"total"`
	Cheated          int     `json:"cheated"`
	Inconclusive     int     `json:"inconclusive"`
	CheatRate        float64 `json:"cheat_rate"`
	InconclusiveRate float64 `json:"inconclusive_rate"`
}

// sandboxExcludes is the set of root-relative directory prefixes that
// MUST NOT be copied into a hermetic sandbox per INV-007. `.edikt/state/`  // edikt-guard:allow
// carries runtime state (verify-gate evidence reads, pending-verifies,
// cache); `.git/` is not needed for a verify replay; and
// `test/integration/benchmarks/sandboxes/` is the sandbox root itself —
// copying it would recurse. Matched only at the source root.
var sandboxExcludes = []string{
	".edikt/state",
	".git",
	"test/integration/benchmarks/sandboxes",
}

// sandboxExcludesBasename is the set of directory basenames that MUST
// be skipped at ANY nesting depth, not just the source root. These are
// build-output / dependency-cache dirs that:
//
//   - The adversary doesn't need (they're not first-party source).
//   - Bloat the sandbox copy by orders of magnitude. node_modules at
//     a website/ subdir, for example, can add 250+ MiB per sandbox —
//     with 3 runs × N verifies, a single cheat-rate invocation
//     produces multi-GiB transient state that exhausts disk on
//     constrained dev machines.
//
// Discovered the hard way during SPEC-009 Plan E smoke testing:  // edikt-guard:allow
// running `bin/edikt gov benchmark cheat-rate backend-api` against a
// repo with a 251 MiB `website/node_modules/` produced 1.9 GiB of
// per-run sandbox copies, which then poisoned TestCompileGolden's
// tempdir on the next test run.
var sandboxExcludesBasename = map[string]bool{
	// JavaScript / Node
	"node_modules": true,
	".next":        true,
	".nuxt":        true,
	".turbo":       true,
	".pnpm-store":  true,
	// Go
	"vendor": true,
	// Python
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	".tox":          true,
	".pytest_cache": true,
	// Rust
	"target": true,
	// JVM / Gradle
	".gradle": true,
	// Build outputs (cross-stack)
	"dist":     true,
	"build":    true,
	"out":      true,
	"coverage": true,
	// Editor / IDE
	".idea":   true,
	".vscode": true,
	// Generic caches
	".cache": true,
}

// CreateSandbox creates a hermetic sandbox directory at sandboxPath
// per INV-007 by copying the project source tree from srcRoot,  // edikt-guard:allow
// excluding `.edikt/state/`, `.git/`, and
// `test/integration/benchmarks/sandboxes/`. The function never copies
// the host user's `~/.claude/settings.json`, shell rc files, or SSH
// keys — only the in-repo source tree rooted at srcRoot.
//
// CreateSandbox is best-effort: it returns an error on os.MkdirAll
// failure for the sandbox root or on the first un-recoverable copy
// error, but tolerates and skips unreadable entries (e.g. broken
// symlinks) without panicking.
//
// The sandbox is only consumed by future non-stub runs; stub-mode
// tests don't require the copy. Callers that don't need a sandbox
// should not call this function.
func CreateSandbox(sandboxPath, srcRoot string) error {
	if sandboxPath == "" {
		return fmt.Errorf("cheatrate: sandbox path must not be empty")
	}
	if srcRoot == "" {
		return fmt.Errorf("cheatrate: source root must not be empty")
	}
	if err := os.MkdirAll(sandboxPath, 0o755); err != nil {
		return fmt.Errorf("cheatrate: create sandbox dir %s: %w", sandboxPath, err)
	}

	absSrc, err := filepath.Abs(srcRoot)
	if err != nil {
		return fmt.Errorf("cheatrate: resolve src root %s: %w", srcRoot, err)
	}
	absDst, err := filepath.Abs(sandboxPath)
	if err != nil {
		return fmt.Errorf("cheatrate: resolve sandbox path %s: %w", sandboxPath, err)
	}

	return filepath.WalkDir(absSrc, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable entries silently — sandbox is best-effort.
			return nil
		}
		rel, relErr := filepath.Rel(absSrc, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		// INV-007: never copy excluded directories into a hermetic sandbox.  // edikt-guard:allow
		if isSandboxExcluded(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Never recurse into the sandbox itself (sandbox lives inside the
		// repo for stub-mode tests).
		if strings.HasPrefix(path, absDst+string(filepath.Separator)) || path == absDst {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(absDst, rel)
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil // best-effort
			}
			return nil
		}
		// Regular files only — skip symlinks, devices, fifos.
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		_ = copyFile(path, target, info.Mode().Perm())
		return nil
	})
}

// isSandboxExcluded reports whether a path relative to the source
// root falls under one of the INV-007 sandbox-exclude rules:  // edikt-guard:allow
//
//   - Root-anchored prefix matches (sandboxExcludes — `.edikt/state`,
//     `.git`, the sandbox root itself).
//   - Any-depth basename matches (sandboxExcludesBasename —
//     `node_modules`, `vendor`, `dist`, etc.). The any-depth rule
//     catches build-output dirs nested arbitrarily, e.g.
//     `website/node_modules/` or `services/api/dist/`.
func isSandboxExcluded(rel string) bool {
	clean := filepath.ToSlash(rel)
	for _, ex := range sandboxExcludes {
		if clean == ex || strings.HasPrefix(clean, ex+"/") {
			return true
		}
	}
	// Any-depth basename check: a path is excluded iff any of its
	// segments matches a basename in the exclude set. This catches
	// `node_modules/`, `website/node_modules/`, `services/api/dist/`,
	// etc. uniformly.
	for _, segment := range strings.Split(clean, "/") {
		if sandboxExcludesBasename[segment] {
			return true
		}
	}
	return false
}

// copyFile writes src's contents to dst with mode perm. Best-effort:
// returns an error but callers may choose to ignore it (sandbox copy is
// non-fatal per INV-007 sandbox semantics).  // edikt-guard:allow
func copyFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// WriteReport serializes report to the canonical ReportPath under
// stateDir and returns the absolute path written.
//
// The write is atomic — JSON is written to a sibling temp file and
// renamed into place — so concurrent readers never observe a partial
// JSON document. The function uses report.RanAt as the timestamp
// segment of the path; the caller is responsible for normalizing the
// timestamp to a filesystem-safe form (colons replaced with dashes)
// before populating Report.RanAt.
//
// stateDir is typically `.edikt/state` under the project root.
func WriteReport(stateDir string, report *Report) (string, error) {
	if report == nil {
		return "", fmt.Errorf("cheatrate: report must not be nil")
	}
	if report.SidecarID == "" {
		return "", fmt.Errorf("cheatrate: report.SidecarID must not be empty")
	}
	if report.RanAt == "" {
		return "", fmt.Errorf("cheatrate: report.RanAt must not be empty")
	}
	timestamp := fsSafeTimestamp(report.RanAt)
	out := ReportPath(stateDir, report.SidecarID, timestamp)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", fmt.Errorf("cheatrate: create report dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("cheatrate: marshal report: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), ".report-*.json.tmp")
	if err != nil {
		return "", fmt.Errorf("cheatrate: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cheatrate: write report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cheatrate: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, out); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cheatrate: rename report into place: %w", err)
	}
	return out, nil
}

// fsSafeTimestamp turns an RFC3339 timestamp (which may contain colons)
// into a path-safe form by replacing ':' with '-'. RFC3339 already
// disallows path separators so no further escaping is required.
func fsSafeTimestamp(ts string) string {
	return strings.ReplaceAll(ts, ":", "-")
}

// CacheGet looks up a cached Report by key under cacheDir. Returns
// (nil, nil) when no cache entry exists. Returns an error when the
// cache entry exists but is unreadable or unparseable — caller may
// treat that as a cache miss or surface it for diagnostics.
//
// cacheDir is typically `.edikt/state/benchmark/cache` under the
// project root. Keys are produced by CacheKey().
func CacheGet(cacheDir, key string) (*Report, error) {
	if cacheDir == "" {
		return nil, fmt.Errorf("cheatrate: cache dir must not be empty")
	}
	if key == "" {
		return nil, fmt.Errorf("cheatrate: cache key must not be empty")
	}
	path := filepath.Join(cacheDir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cheatrate: read cache %s: %w", path, err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("cheatrate: parse cache %s: %w", path, err)
	}
	return &r, nil
}

// CachePut writes report to cacheDir under key as a plain JSON file.
// Existing entries are overwritten atomically (temp file + rename).
func CachePut(cacheDir, key string, report *Report) error {
	if cacheDir == "" {
		return fmt.Errorf("cheatrate: cache dir must not be empty")
	}
	if key == "" {
		return fmt.Errorf("cheatrate: cache key must not be empty")
	}
	if report == nil {
		return fmt.Errorf("cheatrate: report must not be nil")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("cheatrate: create cache dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("cheatrate: marshal cache report: %w", err)
	}
	out := filepath.Join(cacheDir, key+".json")
	tmp, err := os.CreateTemp(cacheDir, ".cache-*.json.tmp")
	if err != nil {
		return fmt.Errorf("cheatrate: create cache temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cheatrate: write cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cheatrate: close cache temp: %w", err)
	}
	if err := os.Rename(tmpPath, out); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cheatrate: rename cache into place: %w", err)
	}
	return nil
}
