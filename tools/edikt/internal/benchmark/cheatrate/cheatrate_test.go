package cheatrate

// cheatrate_test.go — unit tests for the deterministic helpers used by
// `bin/edikt gov benchmark cheat-rate` (SPEC-009 Plan C Phase 3 /
// ADR-040). These tests cover:
//
//   - CacheKey determinism + input-separation safety
//   - ReportPath shape (sidecar id appears in the resolved path)
//   - WriteReport + CacheGet round-trip
//   - CreateSandbox hermetic-copy semantics (INV-007): excludes
//     `.git/` and `.edikt/state/`
//
// The package never calls an LLM; all tests run entirely on the local
// filesystem and the no-llm-in-tier-2 CI grep gate covers this path.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheKey(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		a := CacheKey("abc123", "def456")
		b := CacheKey("abc123", "def456")
		if a != b {
			t.Fatalf("CacheKey not deterministic: %s != %s", a, b)
		}
	})

	t.Run("distinct_for_different_inputs", func(t *testing.T) {
		a := CacheKey("aaa", "bbb")
		b := CacheKey("aaa", "ccc")
		if a == b {
			t.Fatalf("CacheKey collided for different verify hashes: %s", a)
		}
	})

	t.Run("input_separation", func(t *testing.T) {
		// (a||b) vs (a'||b') where the concatenation would otherwise collide.
		// CacheKey uses 0x1f as the separator so these MUST hash to
		// distinct values.
		a := CacheKey("ab", "cd")
		b := CacheKey("abc", "d")
		if a == b {
			t.Fatalf("CacheKey concatenation collision not prevented: %s", a)
		}
	})
}

func TestReportPath(t *testing.T) {
	p := ReportPath("/tmp/state", "ADR-001", "2026-05-23T14-30-00Z")
	if !strings.Contains(p, "ADR-001") {
		t.Fatalf("ReportPath should contain sidecar_id, got: %s", p)
	}
	if !strings.HasSuffix(p, ".json") {
		t.Fatalf("ReportPath should end in .json, got: %s", p)
	}
	if !strings.Contains(p, "benchmark") {
		t.Fatalf("ReportPath should live under benchmark/, got: %s", p)
	}
}

func TestWriteAndReadReport(t *testing.T) {
	stateDir := t.TempDir()
	report := &Report{
		SchemaVersion:  1,
		SidecarID:      "ADR-001",
		RanAt:          "2026-05-23T14:30:00Z",
		AdversaryModel: "claude-opus-4-7",
		Verifies: []Verify{
			{
				VerifyID:           "directive[0]",
				Intent:             "test intent",
				VerifyKind:         "structural",
				Verdict:            "not_cheated",
				MajorityRuns:       "3/3",
				ElapsedMs:          0,
				SandboxPath:        "/dev/null",
				AdversaryTracePath: "/dev/null",
			},
		},
		Summary: Summary{
			Total:            1,
			Cheated:          0,
			Inconclusive:     0,
			CheatRate:        0.0,
			InconclusiveRate: 0.0,
		},
	}

	out, err := WriteReport(stateDir, report)
	if err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("report file not present at %s: %v", out, err)
	}
	if !strings.Contains(out, "ADR-001") {
		t.Fatalf("report path should include sidecar id, got: %s", out)
	}

	// Round-trip via the cache helpers (the cache file format is the
	// same JSON shape, so this also exercises the marshal contract).
	cacheDir := filepath.Join(stateDir, "cache")
	key := CacheKey("sidecar-hash", "verify-hash")
	if err := CachePut(cacheDir, key, report); err != nil {
		t.Fatalf("CachePut: %v", err)
	}
	got, err := CacheGet(cacheDir, key)
	if err != nil {
		t.Fatalf("CacheGet: %v", err)
	}
	if got == nil {
		t.Fatalf("CacheGet returned nil for a cached key")
	}
	if got.SidecarID != "ADR-001" {
		t.Fatalf("cached sidecar id mismatch: got %q want %q", got.SidecarID, "ADR-001")
	}
	if len(got.Verifies) != 1 || got.Verifies[0].VerifyID != "directive[0]" {
		t.Fatalf("cached verifies not round-tripped: %+v", got.Verifies)
	}
}

func TestCacheGet_Miss(t *testing.T) {
	cacheDir := t.TempDir()
	r, err := CacheGet(cacheDir, "nonexistent-key")
	if err != nil {
		t.Fatalf("CacheGet on missing key should not error: %v", err)
	}
	if r != nil {
		t.Fatalf("CacheGet on missing key should return nil, got: %+v", r)
	}
}

func TestCreateSandbox(t *testing.T) {
	// Build a fake source tree containing all the directories CreateSandbox
	// must exclude per INV-007.
	src := t.TempDir()
	mustMkdir(t, filepath.Join(src, "commands"))
	mustWrite(t, filepath.Join(src, "commands", "ok.md"), "ok\n")
	mustMkdir(t, filepath.Join(src, ".git"))
	mustWrite(t, filepath.Join(src, ".git", "HEAD"), "ref: refs/heads/main\n")
	mustMkdir(t, filepath.Join(src, ".edikt", "state"))
	mustWrite(t, filepath.Join(src, ".edikt", "state", "secret.json"), `{"token":"deadbeef"}`)
	mustMkdir(t, filepath.Join(src, "test", "integration", "benchmarks", "sandboxes"))
	mustWrite(t, filepath.Join(src, "test", "integration", "benchmarks", "sandboxes", "old.txt"), "old\n")

	sandbox := filepath.Join(t.TempDir(), "verify-0")
	if err := CreateSandbox(sandbox, src); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}

	// Sandbox root must exist.
	if _, err := os.Stat(sandbox); err != nil {
		t.Fatalf("sandbox dir missing: %v", err)
	}

	// Included content must be present.
	if _, err := os.Stat(filepath.Join(sandbox, "commands", "ok.md")); err != nil {
		t.Fatalf("expected commands/ok.md in sandbox, got: %v", err)
	}

	// INV-007: .git/ MUST NOT be copied.
	if _, err := os.Stat(filepath.Join(sandbox, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git should be excluded from sandbox (INV-007), got: %v", err)
	}

	// INV-007: .edikt/state/ MUST NOT be copied (runtime state, may carry secrets).
	if _, err := os.Stat(filepath.Join(sandbox, ".edikt", "state")); !os.IsNotExist(err) {
		t.Fatalf(".edikt/state should be excluded from sandbox (INV-007), got: %v", err)
	}

	// Recursive-sandbox guard: test/integration/benchmarks/sandboxes MUST NOT be copied.
	if _, err := os.Stat(filepath.Join(sandbox, "test", "integration", "benchmarks", "sandboxes")); !os.IsNotExist(err) {
		t.Fatalf("benchmarks/sandboxes should be excluded, got: %v", err)
	}
}

func TestCreateSandbox_EmptyArgs(t *testing.T) {
	if err := CreateSandbox("", "/tmp"); err == nil {
		t.Fatal("expected error for empty sandboxPath")
	}
	if err := CreateSandbox("/tmp/out", ""); err == nil {
		t.Fatal("expected error for empty srcRoot")
	}
}

// TestCreateSandbox_ExcludesBuildArtifacts confirms that build-output
// and dependency-cache directories — node_modules, vendor, dist,
// target, etc. — are skipped at any nesting depth, not just at the
// source root. Discovered the hard way during SPEC-009 Plan E smoke
// testing: a 251 MiB `website/node_modules/` produced 1.9 GiB of
// transient sandbox copies per cheat-rate run.
func TestCreateSandbox_ExcludesBuildArtifacts(t *testing.T) {
	src := t.TempDir()
	// Legitimate first-party files (must be copied).
	mustMkdir(t, filepath.Join(src, "commands"))
	mustWrite(t, filepath.Join(src, "commands", "ok.md"), "ok\n")
	mustMkdir(t, filepath.Join(src, "website"))
	mustWrite(t, filepath.Join(src, "website", "index.html"), "<html/>\n")

	// Top-level build-output dirs.
	mustMkdir(t, filepath.Join(src, "node_modules", "react"))
	mustWrite(t, filepath.Join(src, "node_modules", "react", "package.json"), "{}\n")
	mustMkdir(t, filepath.Join(src, "vendor", "github.com"))
	mustWrite(t, filepath.Join(src, "vendor", "github.com", "foo.go"), "package foo\n")
	mustMkdir(t, filepath.Join(src, "dist"))
	mustWrite(t, filepath.Join(src, "dist", "bundle.js"), "// built\n")

	// Nested build-output dirs (the case that caused the regression).
	mustMkdir(t, filepath.Join(src, "website", "node_modules", "@esbuild"))
	mustWrite(t, filepath.Join(src, "website", "node_modules", "@esbuild", "bin"), "binary\n")
	mustMkdir(t, filepath.Join(src, "services", "api", "target", "release"))
	mustWrite(t, filepath.Join(src, "services", "api", "target", "release", "out"), "rust\n")
	mustMkdir(t, filepath.Join(src, "pkg", "__pycache__"))
	mustWrite(t, filepath.Join(src, "pkg", "__pycache__", "mod.pyc"), "bytecode\n")

	sandbox := filepath.Join(t.TempDir(), "sb")
	if err := CreateSandbox(sandbox, src); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}

	// Legitimate first-party files survived.
	for _, want := range []string{
		filepath.Join(sandbox, "commands", "ok.md"),
		filepath.Join(sandbox, "website", "index.html"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected %s to be copied, got: %v", want, err)
		}
	}

	// Build-output dirs MUST NOT be present in the sandbox — at any depth.
	excludes := []string{
		filepath.Join(sandbox, "node_modules"),
		filepath.Join(sandbox, "vendor"),
		filepath.Join(sandbox, "dist"),
		filepath.Join(sandbox, "website", "node_modules"),
		filepath.Join(sandbox, "services", "api", "target"),
		filepath.Join(sandbox, "pkg", "__pycache__"),
	}
	for _, p := range excludes {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be excluded; stat err = %v", p, err)
		}
	}
}

// TestIsSandboxExcluded_NestedBasename covers the bare predicate to
// make the rule visible and grep-friendly when adding new excludes.
func TestIsSandboxExcluded_NestedBasename(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		// Root-anchored prefixes (existing).
		{".edikt/state", true},
		{".edikt/state/cache", true},
		{".git", true},
		{".git/objects/pack", true},
		{"test/integration/benchmarks/sandboxes", true},

		// Basename matches at any depth.
		{"node_modules", true},
		{"website/node_modules", true},
		{"website/node_modules/@esbuild/bin", true},
		{"services/api/vendor/foo", true},
		{"pkg/__pycache__/x.pyc", true},
		{"target", true},
		{"build/x", true},

		// Legitimate paths that MUST NOT match.
		{"commands/ok.md", false},
		{"website/index.html", false},
		{"docs/architecture/decisions/ADR-040.md", false},
		// "node_modules" as a filename component but not a directory
		// segment we'd skip would still match; that's acceptable
		// false-positive territory — file names matching build-dir
		// conventions are vanishingly rare.
	}
	for _, c := range cases {
		got := isSandboxExcluded(c.rel)
		if got != c.want {
			t.Errorf("isSandboxExcluded(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

// --- helpers ---

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir parent of %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}
