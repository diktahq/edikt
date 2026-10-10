package hook

// userpromptsubmit_test.go — direct coverage for user-prompt-submit.sh's
// in-progress-phase detection, which test/fixtures/hook-corpus/
// user-prompt-submit/matrix.yaml deliberately does not exercise: every
// recorded scenario takes the "no plan directory" early exit (see that
// matrix's own header comment — a plan fixture was not added to the shared
// _base tree because it would silently invalidate post-compact's
// already-diff-verified baseline).
//
// This file is the port target for the "UserPromptSubmit: detects
// in-progress phases" assertion retired from test/test-hooks.sh's
// content-grep era (PLAN-v080-reliability-core Phase 12) — it grepped
// templates/hooks/user-prompt-submit.sh for the literal "in.progress" /
// "in_progress" / "in-progress", which is now a 10-line exec shim with none
// of that text in it. The behaviour moved to userpromptsubmit.go /
// planphase.go; this file is where its coverage now lives.

import (
	"io"
	"path/filepath"
	"testing"
)

// TestUserPromptSubmitPhaseRowStrictThenFallback proves the two-tier phase
// detection userPromptSubmitPhaseRow performs: a strict, anchored,
// case-sensitive primary pattern, falling back to the same case-insensitive
// pattern post-compact.sh uses (planPhaseRow) when the strict one finds
// nothing.
func TestUserPromptSubmitPhaseRowStrictThenFallback(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantOK     bool
		wantPhase  string
		wantTheme  string
		viaPrimary bool
	}{
		{
			name: "strict primary pattern matches an anchored in progress row",
			body: "# Plan: Test\n\n## Progress\n\n" +
				"| Phase | Theme | Status |\n" +
				"|---|---|---|\n" +
				"| 2 | Build | in progress |\n",
			wantOK:     true,
			wantPhase:  "2",
			wantTheme:  "Build",
			viaPrimary: true,
		},
		{
			name: "fallback pattern matches Phase-worded, underscored status the strict pattern rejects",
			body: "# Plan: Test\n\n## Progress\n\n" +
				"| Phase 3 | Ship | in_progress |\n",
			wantOK:    true,
			wantPhase: "3",
			wantTheme: "Ship",
		},
		{
			name:   "no in-progress row at all",
			body:   "# Plan: Test\n\n| 1 | Setup | done |\n| 2 | Build | not started |\n",
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fields, ok := userPromptSubmitPhaseRow([]byte(c.body))
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (fields=%v)", ok, c.wantOK, fields)
			}
			if !ok {
				return
			}
			gotPhase := phaseNumDigits(phaseField(fields, 2))
			gotTheme := phaseField(fields, 3)
			if gotPhase != c.wantPhase {
				t.Errorf("phase = %q, want %q", gotPhase, c.wantPhase)
			}
			if gotTheme != c.wantTheme {
				t.Errorf("theme = %q, want %q", gotTheme, c.wantTheme)
			}
		})
	}
}

// TestUserPromptSubmitDetectsInProgressPhaseEndToEnd runs the full handler —
// config, plan directory resolution, most-recent-plan-file selection, and
// message assembly — the way the hook actually fires, proving the detection
// logic is wired all the way through to additionalContext output, not just
// correct in isolation.
func TestUserPromptSubmitDetectsInProgressPhaseEndToEnd(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), "base: docs\n")
	writeTestFile(t, filepath.Join(cwd, "docs", "plans", "PLAN-001-test.md"),
		"# Plan: Test Plan\n\n## Progress\n\n"+
			"| Phase | Theme | Status | Updated |\n"+
			"|-------|-------|--------|---------|\n"+
			"| 1 | Setup | done | 2026-03-20 |\n"+
			"| 2 | Build | in progress | 2026-03-20 |\n"+
			"| 3 | Ship | not started | — |\n")

	inv := &Invocation{Stderr: io.Discard}
	res := userPromptSubmit(inv)

	if res.Response.Context == "" {
		t.Fatal("expected additionalContext to be populated for an in-progress phase, got empty")
	}
	if !containsSubstring(res.Response.Context, "Current phase: 2") {
		t.Errorf("additionalContext = %q, want it to name phase 2 as current", res.Response.Context)
	}
	if !containsSubstring(res.Response.Context, "Build") {
		t.Errorf("additionalContext = %q, want it to carry the phase theme %q", res.Response.Context, "Build")
	}
}

// TestUserPromptSubmitAllPhasesDoneStaysSilent is the negative control: no
// in-progress row anywhere in the table means no additionalContext at all,
// proving the detector does not fire on every plan, only an active one.
func TestUserPromptSubmitAllPhasesDoneStaysSilent(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), "base: docs\n")
	writeTestFile(t, filepath.Join(cwd, "docs", "plans", "PLAN-001-test.md"),
		"# Plan: Test Plan\n\n## Progress\n\n"+
			"| Phase | Status | Updated |\n"+
			"|-------|--------|---------|\n"+
			"| 1 | done | 2026-03-20 |\n"+
			"| 2 | done | 2026-03-20 |\n")

	inv := &Invocation{Stderr: io.Discard}
	res := userPromptSubmit(inv)

	if res.Response.Context != "" {
		t.Errorf("expected silence when every phase is done, got %q", res.Response.Context)
	}
}

// TestUserPromptSubmitPlanInjectionFalseSuppressesContext is the Go-side
// successor to test-quality.sh's retired "UserPromptSubmit checks
// plan-injection config" grep (which matched the literal string
// "plan-injection: false" in the now-10-line user-prompt-submit.sh exec
// shim). It is also the fixture the corpus's own matrix.yaml documents it
// could not build: that matrix's "plan-injection-off" scenario shares the
// hook-wide _base tree with no docs/plans/ directory, so its guard and
// baseline's own "no plan directory" early exit both produce byte-identical
// empty output — confirmed by mutation (removing the `plan-injection:
// false` guard from userPromptSubmit and re-running `dev hook-corpus diff
// --hook user-prompt-submit --bin <mutated>` still passed 4/4). This test
// seeds a REAL in-progress plan, the fixture TestUserPromptSubmitDetects
// InProgressPhaseEndToEnd above proves renders non-empty context, so the
// flag's effect is actually observable.
func TestUserPromptSubmitPlanInjectionFalseSuppressesContext(t *testing.T) {
	planBody := "# Plan: Test Plan\n\n## Progress\n\n" +
		"| Phase | Theme | Status | Updated |\n" +
		"|-------|-------|--------|---------|\n" +
		"| 2 | Build | in progress | 2026-03-20 |\n"

	run := func(t *testing.T, injectionOff bool) string {
		t.Helper()
		cwd := t.TempDir()
		t.Chdir(cwd)

		cfg := "base: docs\n"
		if injectionOff {
			cfg += "plan-injection: false\n"
		}
		writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), cfg)
		writeTestFile(t, filepath.Join(cwd, "docs", "plans", "PLAN-001-test.md"), planBody)

		inv := &Invocation{Stderr: io.Discard}
		return userPromptSubmit(inv).Response.Context
	}

	off := run(t, true)
	if off != "" {
		t.Fatalf("plan-injection: false: got additionalContext %q, want empty — the config guard did not suppress plan injection", off)
	}
	// Negative control: with the SAME in-progress plan but no `plan-injection:
	// false`, context must actually be injected — otherwise the assertion
	// above proves nothing about the guard specifically.
	on := run(t, false)
	if on == "" {
		t.Fatal("without plan-injection: false: additionalContext was empty — negative control failed, so the guard assertion above proves nothing")
	}
}

// TestPlanNameFromHeading_Frontmatter pins issue #13: a plan written from
// /edikt:sdlc:plan's template opens with a YAML frontmatter block, which
// pushes the `# Plan:` heading past the first five lines. The name must
// still resolve — from the heading after the fence, else the frontmatter id.
func TestPlanNameFromHeading_Frontmatter(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "no frontmatter, heading on line 1",
			body: "# Plan: Legacy\n\n## Progress\n",
			want: "Plan: Legacy",
		},
		{
			name: "template-shaped frontmatter pushes the heading past line 5",
			body: "---\ntype: plan\nid: PLAN-harness-targets\nmodel: opus\nphases:\n  - id: 1\n  - id: 2\n  - id: 3\n---\n\n# Plan: Harness targets\n\n## Overview\n",
			want: "Plan: Harness targets",
		},
		{
			name: "frontmatter with no heading falls back to the frontmatter id",
			body: "---\ntype: plan\nid: PLAN-harness-targets\n---\n\n## Overview\n",
			want: "PLAN-harness-targets",
		},
		{
			name: "CRLF frontmatter",
			body: "---\r\ntype: plan\r\nid: PLAN-x\r\n---\r\n\r\n# Plan: CRLF\r\n",
			want: "Plan: CRLF",
		},
		{
			name: "nothing to name it by",
			body: "## Overview\n",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := planNameFromHeading([]byte(c.body)); got != c.want {
				t.Errorf("planNameFromHeading = %q, want %q", got, c.want)
			}
		})
	}
}
