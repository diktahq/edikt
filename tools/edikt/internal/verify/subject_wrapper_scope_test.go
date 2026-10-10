package verify

import (
	"os"
	"path/filepath"
	"testing"
)

// The gotest.sh wrapper root must not outlive the command it belongs to.
//
// Found in the field, not by inspection: AC-13.1 of PLAN-v090-release runs
// `test/lib/gotest.sh` and then, after an `&&`, `./test/run.sh`. The wrapper
// root set for gotest.sh's package arguments was still in force when the walk
// reached `./test/run.sh`, so the pre-flight looked for it under
// tools/edikt/test/run.sh — absent — and reported the release gate's own
// suite criterion as UNMEASURED while test/run.sh sat readable at the root.
//
// The leak survived because clearing wrapperRoot required a repo-path token
// that does NOT begin with "./", and every token between the two (`'Test'`,
// `&&`, `(`) returns earlier in the loop.
func TestUnreadableSubjects_WrapperRootStopsAtCommandSeparator(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "test", "run.sh"))
	mustWrite(t, filepath.Join(root, "tools", "edikt", "internal", "hookmatch", "m.go"))

	// The shape from AC-13.1: a gotest.sh call, then `&&`, then a repo-root
	// script that shares the "./" prefix the wrapper rule keys on.
	cmd := "bash test/lib/gotest.sh './internal/hook/... ./internal/hookmatch/' 'Test'" +
		" && ( ./test/run.sh > /tmp/run.out 2>&1 )"

	if got := unreadableSubjects(root, cmd); got != "" {
		t.Fatalf("every subject is readable, but the pre-flight reported %q as unreadable;\n"+
			"the gotest.sh wrapper root leaked past its own arguments and resolved it under tools/edikt/", got)
	}
}

// The wrapper root must still apply to gotest.sh's own arguments — the fix
// must not simply delete the behaviour the leak was a side effect of.
func TestUnreadableSubjects_WrapperRootStillAppliesToItsOwnArguments(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "tools", "edikt", "internal", "hookmatch", "m.go"))

	// ./internal/hookmatch/ exists ONLY under tools/edikt. If the wrapper root
	// stopped applying to gotest.sh's arguments, this would resolve against
	// the repo root and be reported unreadable.
	cmd := "bash test/lib/gotest.sh './internal/hookmatch/' 'Test'"

	if got := unreadableSubjects(root, cmd); got != "" {
		t.Fatalf("gotest.sh's own argument should resolve under tools/edikt, got %q unreadable", got)
	}
}

// A genuinely absent subject after the separator must still be caught — the
// fix must not turn the pre-flight off for everything past the first `&&`.
func TestUnreadableSubjects_AbsentSubjectAfterSeparatorStillReported(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "tools", "edikt", "internal", "hookmatch", "m.go"))

	cmd := "bash test/lib/gotest.sh './internal/hookmatch/' 'Test' && bash test/gone.sh"

	if got := unreadableSubjects(root, cmd); got != "test/gone.sh" {
		t.Fatalf("want test/gone.sh reported unreadable, got %q", got)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
