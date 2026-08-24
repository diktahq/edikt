package hook

// posttooluse_test.go — discriminating coverage for EDIKT_FORMAT_SKIP, which
// test/fixtures/hook-corpus/post-tool-use/matrix.yaml's "format-skip"
// scenario NAMES but cannot PROVE (INV-013): the corpus sandbox has no
// per-scenario mechanism to seed an arbitrary extra file into the project
// tree (see that matrix's own header comment on the formatter-dispatch
// limit), so "format-skip" and "baseline" both take the "FILE does not
// exist" early exit and produce byte-identical zero-length output — deleting
// the guard from postToolUse entirely leaves the recorded corpus at 5/5
// PASS. Proving the guard requires a REAL file on disk and a REAL formatter
// on PATH, which only a Go test with its own sandbox can seed.
//
// PLAN-v080-reliability-core Phase 13.

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestPostToolUseFormatSkipSuppressesFormatterInvocation proves
// EDIKT_FORMAT_SKIP=1 actually stops the formatter from running — not just
// that the early-return branch is taken. A fake `gofmt` earlier on PATH
// appends to a marker file when invoked; the skip run must leave the marker
// absent, and the un-skipped run (the negative control) MUST leave it
// present, or the first assertion would pass for the wrong reason — the
// formatter never running at all, unrelated to the guard under test.
func TestPostToolUseFormatSkipSuppressesFormatterInvocation(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), "base: docs\n")

	target := filepath.Join(cwd, "handler.go")
	writeTestFile(t, target, "package main\n")

	marker := filepath.Join(cwd, "gofmt-called.marker")
	binDir := filepath.Join(cwd, "fakebin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir fakebin: %v", err)
	}
	// A fake `gofmt` that does nothing but record that it ran. Its own exit
	// code is irrelevant to this test; runFormatter swallows failures the
	// same way the bash original did.
	script := "#!/bin/sh\necho called >> \"" + marker + "\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "gofmt"), []byte(script), 0o755); err != nil { //nolint:gosec // test fixture script
		t.Fatalf("write fake gofmt: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("CLAUDE_TOOL_INPUT_FILE_PATH", target)

	formatterRan := func(t *testing.T, skip bool) bool {
		t.Helper()
		_ = os.Remove(marker)
		if skip {
			t.Setenv("EDIKT_FORMAT_SKIP", "1")
		} else {
			t.Setenv("EDIKT_FORMAT_SKIP", "")
		}
		inv := &Invocation{Stderr: io.Discard}
		postToolUse(inv)
		_, err := os.Stat(marker)
		return err == nil
	}

	if formatterRan(t, true) {
		t.Fatal("EDIKT_FORMAT_SKIP=1: fake gofmt still ran — the skip guard did not suppress the formatter")
	}
	// Negative control: without the env var, the SAME file and PATH must
	// actually reach the formatter. Without this half, the assertion above
	// would pass just as well if the formatter dispatch were broken for any
	// unrelated reason.
	if !formatterRan(t, false) {
		t.Fatal("without EDIKT_FORMAT_SKIP: fake gofmt never ran — negative control failed, so the skip assertion above proves nothing")
	}
}

// TestPostToolUseAutoFormatFalseSuppressesFormatterInvocation is
// auto-format's counterpart to the EDIKT_FORMAT_SKIP test above, for the
// SAME reason the corpus's own matrix.yaml documents: the "auto-format-off"
// scenario NAMES `auto-format: false` in .edikt/config.yaml but cannot
// PROVE it discriminates — that scenario's target file never exists in the
// corpus sandbox, so the config check and the pre-existing "file does not
// exist" early exit produce byte-identical output either way. Confirmed by
// mutation: removing the `strings.Contains(configBytes, "auto-format:
// false")` guard from postToolUse and re-running `dev hook-corpus diff
// --hook post-tool-use --bin <mutated>` still passed 5/5. This test seeds a
// REAL file and a fake `gofmt` on PATH — the config-flag mirror of the
// EDIKT_FORMAT_SKIP test above — so the guard's effect is actually
// observable.
func TestPostToolUseAutoFormatFalseSuppressesFormatterInvocation(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	target := filepath.Join(cwd, "handler.go")
	writeTestFile(t, target, "package main\n")

	marker := filepath.Join(cwd, "gofmt-called.marker")
	binDir := filepath.Join(cwd, "fakebin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir fakebin: %v", err)
	}
	script := "#!/bin/sh\necho called >> \"" + marker + "\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "gofmt"), []byte(script), 0o755); err != nil { //nolint:gosec // test fixture script
		t.Fatalf("write fake gofmt: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("CLAUDE_TOOL_INPUT_FILE_PATH", target)

	formatterRan := func(t *testing.T, autoFormatFalse bool) bool {
		t.Helper()
		_ = os.Remove(marker)
		cfg := "base: docs\n"
		if autoFormatFalse {
			cfg += "auto-format: false\n"
		}
		writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), cfg)
		inv := &Invocation{Stderr: io.Discard}
		postToolUse(inv)
		_, err := os.Stat(marker)
		return err == nil
	}

	if formatterRan(t, true) {
		t.Fatal("auto-format: false: fake gofmt still ran — the config guard did not suppress the formatter")
	}
	// Negative control: with the SAME file and PATH but no `auto-format:
	// false`, the formatter must actually run — otherwise the assertion
	// above proves nothing about the guard specifically.
	if !formatterRan(t, false) {
		t.Fatal("without auto-format: false: fake gofmt never ran — negative control failed, so the guard assertion above proves nothing")
	}
}
