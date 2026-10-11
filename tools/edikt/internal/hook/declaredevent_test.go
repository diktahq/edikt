package hook

import (
	"testing"
)

// The regression: a payload without hook_event_name used to reach the
// encoder as "" and the deny was dropped (fail-open). The registry row is the
// fallback, and it must give the FIRING event, not a generic one.
func TestDeclaredEvent_FromRegistryRow(t *testing.T) {
	root := repoRoot(t)
	rootFn := func() (string, error) { return root, nil }
	for name, want := range map[string]string{
		"pre-tool-use":    "PreToolUse",
		"post-compact":    "PostCompact",
		"session-compact": "SessionStart",
		"post-tool-use":   "PostToolUse",
	} {
		if got := declaredEvent(name, rootFn); got != want {
			t.Errorf("declaredEvent(%q) = %q, want %q", name, got, want)
		}
	}
}

// An unknown hook, or one whose row declares no event, resolves to "" — the
// encoder's refusal stays in place rather than a guessed name going out.
func TestDeclaredEvent_UnknownOrEventless(t *testing.T) {
	root := repoRoot(t)
	rootFn := func() (string, error) { return root, nil }
	if got := declaredEvent("no-such-hook", rootFn); got != "" {
		t.Errorf("unknown hook resolved to %q, want \"\"", got)
	}
	if got := declaredEvent("inject-directives-pre", func() (string, error) { return t.TempDir(), nil }); got != "" {
		// A root with no registry at all and no EDIKT_HOME payload: nothing to
		// consult. (EDIKT_HOME may point at a live install with a registry, in
		// which case this legitimately resolves; guard the assertion on that.)
		if _, _, err := ResolveRegistryPath(t.TempDir()); err != nil {
			t.Errorf("no registry anywhere, yet resolved %q", got)
		}
	}
}
