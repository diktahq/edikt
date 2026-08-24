package hook

// statusline_test.go — discriminating coverage for EDIKT_STATUSLINE_SKIP,
// which test/fixtures/hook-corpus/status-line/matrix.yaml's "skip-env"
// scenario NAMES but cannot PROVE (INV-013): the sandbox's default config
// carries no `features:` block at all, so BOTH "baseline" and "skip-env"
// take the same "statusline disabled" early exit and produce byte-identical
// zero-byte output — deleting the guard from statusLine entirely leaves the
// recorded corpus at 4/4 PASS. Proving the guard requires a project where
// status-line actually renders non-empty text, which the corpus's shared
// `_base` config does not provide (see that matrix's own header comment on
// why a non-zero-output config was never added there).
//
// PLAN-v080-reliability-core Phase 13.

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStatusLineSkipEnvChangesObservableOutput proves EDIKT_STATUSLINE_SKIP=1
// suppresses the hook's actual rendered output, not merely that its early
// branch is taken. The un-skipped run doubles as Phase 13's own required
// proof that status-line still renders non-empty text at all — registration
// in settings.json.tmpl is not evidence the render path works.
func TestStatusLineSkipEnvChangesObservableOutput(t *testing.T) {
	render := func(t *testing.T, skip bool) string {
		t.Helper()
		home := t.TempDir()
		proj := t.TempDir()
		t.Setenv("HOME", home)
		t.Chdir(proj)
		writeTestFile(t, filepath.Join(proj, ".edikt", "config.yaml"),
			"base: docs\nfeatures:\n  statusline: true\n")
		if skip {
			t.Setenv("EDIKT_STATUSLINE_SKIP", "1")
		} else {
			t.Setenv("EDIKT_STATUSLINE_SKIP", "")
		}
		inv := &Invocation{Stderr: io.Discard}
		res := statusLine(inv)
		return res.Response.Text
	}

	withSkip := render(t, true)
	if withSkip != "" {
		t.Errorf("EDIKT_STATUSLINE_SKIP=1 → %q, want empty output", withSkip)
	}

	// Negative control, and Phase 13's own render-still-works proof: without
	// the skip flag, against a config with `features: statusline: true`, the
	// hook MUST produce non-empty text. If it did not, the assertion above
	// would pass for the wrong reason — the render path broken, not the flag
	// suppressing it.
	withoutSkip := render(t, false)
	if withoutSkip == "" {
		t.Fatal("status-line produced empty output without the skip flag — the render path itself never ran, so the flag's effect cannot be observed")
	}

	if withSkip == withoutSkip {
		t.Fatalf("EDIKT_STATUSLINE_SKIP produced no observable difference — both runs said %q", withSkip)
	}
}

// TestStatusLineCacheAgeReadsPortableModTime is the Go-side successor to
// test/unit/hooks/test_d23_stat_fallback.sh's status-line.sh case (F-059 /
// D23). The bash original computed cache age via
// `date -r "$CACHE_FILE" || stat -f %m "$CACHE_FILE" || stat -c %Y ... ||
// echo 0` because GNU's `-f` flag means "filesystem status" (not BSD's
// FORMAT flag): `stat -f %m` prints "?" and exits 0 on GNU, so the `||`
// chain never reaches the correct `-c` branch and the cache silently never
// expires. The retired suite proved this two ways: a fake GNU-shaped `stat`
// on PATH, and a grep for the literal `date -r "$CACHE_FILE"` string in the
// shipped bash file.
//
// Neither proof transfers as-is: this handler (statusLine, PLAN-v080
// Phase 13) reads mtime via os.Stat(cacheFile).ModTime() — a single
// portable syscall wrapper with no shell-out and no GNU/BSD flag ambiguity,
// so there is no fallback chain left to grep for or fake a broken `stat`
// against. What DOES still need its own proof is the property the bash fix
// existed to guarantee: age is computed from the cache file's REAL mtime,
// not a sentinel that always reads as fresh (or always as stale). This
// test pins that property directly by manipulating mtime with os.Chtimes —
// the Go-native way to reproduce "the clock the age check reads from" — and
// would fail if a future edit stopped reading ModTime correctly (e.g.
// hardcoded a fixed age, compared against the wrong file, or inverted the
// TTL comparison).
func TestStatusLineCacheAgeReadsPortableModTime(t *testing.T) {
	newProject := func(t *testing.T) (cacheFile string) {
		t.Helper()
		home := t.TempDir()
		proj := t.TempDir()
		t.Setenv("HOME", home)
		t.Chdir(proj)
		writeTestFile(t, filepath.Join(proj, ".edikt", "config.yaml"),
			"base: docs\nfeatures:\n  statusline: true\n")
		return filepath.Join(home, ".edikt", "state", "statusline-cache")
	}

	const marker = "CACHE-MARKER-DO-NOT-RENDER"

	// Fresh cache (mtime "now", age ~0s < the 60s TTL) MUST be served
	// verbatim from the cache-hit path.
	cacheFile := newProject(t)
	writeTestFile(t, cacheFile, marker)
	got := statusLine(&Invocation{Stderr: io.Discard}).Response.Text
	if got != marker {
		t.Fatalf("fresh cache (age ~0s): got %q, want cached marker %q — the cache-hit path did not read a fresh mtime as fresh", got, marker)
	}

	// Stale cache (mtime pushed back 120s, past the 60s TTL) MUST NOT be
	// served — this is the failure mode D23 named: a broken age read that
	// treats every cache file as fresh (GNU stat's "?" sentinel, formatted
	// into an arithmetic expression, historically behaved this way) would
	// return the marker here too, undetectably.
	cacheFile2 := newProject(t)
	writeTestFile(t, cacheFile2, marker)
	old := time.Now().Add(-120 * time.Second)
	if err := os.Chtimes(cacheFile2, old, old); err != nil {
		t.Fatalf("os.Chtimes: %v", err)
	}
	got2 := statusLine(&Invocation{Stderr: io.Discard}).Response.Text
	if got2 == marker {
		t.Fatalf("stale cache (age 120s > 60s TTL): got the planted marker %q back — the TTL comparison did not treat an old mtime as stale", marker)
	}
	if got2 == "" {
		t.Fatal("stale cache: render path produced empty output — expected a real (non-cached) status line")
	}
}
