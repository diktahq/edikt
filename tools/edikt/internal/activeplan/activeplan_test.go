package activeplan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePlan(t *testing.T, dir, name, body string, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
	return path
}

const incompletePlan = `# PLAN-x

## Progress

| Phase | Status |
|---|---|
| 1 | done |
| 2 | in-progress |
`

const completePlan = `# PLAN-y

## Progress

| Phase | Status |
|---|---|
| 1 | done |
| 2 | complete |
`

const noProgressPlan = `# PLAN-z

## Cost Estimate

| Phase | Cost |
|---|---|
| 1 | 5 |
`

func TestResolve_PicksNewestIncompletePlan(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	writePlan(t, dir, "PLAN-old-incomplete.md", incompletePlan, base.Add(-2*time.Hour))
	newest := writePlan(t, dir, "PLAN-new-incomplete.md", incompletePlan, base)

	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != newest {
		t.Errorf("got %q, want %q", got, newest)
	}
}

func TestResolve_SkipsCompletePlanAndFallsThroughToOlderIncomplete(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	older := writePlan(t, dir, "PLAN-older-incomplete.md", incompletePlan, base.Add(-2*time.Hour))
	writePlan(t, dir, "PLAN-newest-complete.md", completePlan, base)

	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != older {
		t.Errorf("expected the newest INCOMPLETE plan (%q), got %q", older, got)
	}
}

func TestResolve_SkipsSupersededFile(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	writePlan(t, dir, "PLAN-x.SUPERSEDED.md", incompletePlan, base)
	older := writePlan(t, dir, "PLAN-older.md", incompletePlan, base.Add(-time.Hour))

	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != older {
		t.Errorf("expected the non-superseded plan (%q), got %q", older, got)
	}
}

func TestResolve_NoProgressHeadingContributesNothing(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, dir, "PLAN-z.md", noProgressPlan, time.Now())

	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("a plan with no Progress heading must not be treated as active, got %q", got)
	}
}

func TestResolve_EveryPlanCompleteReturnsEmptyNotError(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, dir, "PLAN-y.md", completePlan, time.Now())

	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty (no active plan), got %q", got)
	}
}

func TestResolve_UnreadableDirReturnsError(t *testing.T) {
	_, err := Resolve(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for an unreadable directory")
	}
}

func TestSlug_StripsPrefixAndExtension(t *testing.T) {
	cases := map[string]string{
		"docs/internal/plans/PLAN-v090-release.md": "v090-release",
		"PLAN-spec-008-post-flight-pipeline.md":    "spec-008-post-flight-pipeline",
		"/abs/path/PLAN-foo.md":                    "foo",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
