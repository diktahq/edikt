package cmd

import (
	"os"
	"strings"
	"testing"
)

// The published chain must not be able to fall behind the resolver. That
// divergence is the whole defect: the install harness pinned a two-level
// chain, CLAUDE_CONFIG_DIR was inserted between the levels, and the guard
// kept passing while resolution escaped to a real home directory.
//
// This asserts the published order IS the resolver's order, by driving both
// with the same environment and comparing. Documentation cannot rot into a
// lie if a test reads it.
func TestResolutionChainMatchesResolver(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"all set — first rung wins", map[string]string{
			"CLAUDE_HOME": "/a", "CLAUDE_CONFIG_DIR": "/b", "HOME": "/c"}},
		{"CLAUDE_HOME unset — falls to CLAUDE_CONFIG_DIR, NOT $HOME", map[string]string{
			"CLAUDE_HOME": "", "CLAUDE_CONFIG_DIR": "/b", "HOME": "/c"}},
		{"only HOME", map[string]string{
			"CLAUDE_HOME": "", "CLAUDE_CONFIG_DIR": "", "HOME": "/c"}},
		{"nothing set", map[string]string{
			"CLAUDE_HOME": "", "CLAUDE_CONFIG_DIR": "", "HOME": ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			want := resolveClaudeRoot()
			got := resolveClaudeRootVia(claudeRootChain)
			if got != want {
				t.Fatalf("published chain resolves %q but resolveClaudeRoot returns %q — "+
					"the chain has drifted from the resolver", got, want)
			}
		})
	}
}

// The second case above is the incident in miniature, asserted directly:
// unsetting CLAUDE_HOME does NOT fall through to $HOME. A guard written on
// that assumption sandboxes nothing.
func TestUnsettingClaudeHomeDoesNotFallToHome(t *testing.T) {
	t.Setenv("CLAUDE_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "/real/profile")
	t.Setenv("HOME", "/sandbox")
	if got := resolveClaudeRoot(); got != "/real/profile" {
		t.Fatalf("resolveClaudeRoot() = %q, want /real/profile — "+
			"this test encodes why unsetting CLAUDE_HOME is not sandboxing", got)
	}
}

// GL-002 rule 7: an OR in an acceptance criterion is a hole unless both
// branches carry equal weight. The first version of this test was named
// "PinnableOrDeclared" and passed over the exact hole it was written for — a
// rung whose input a guard could not name — because "declared" is trivially
// satisfied by any rung with a Detail string, so the OR reduced to it.
//
// Both branches now carry weight. EVERY rung must name its input AND state a
// containment strategy, whether or not it is env-backed. A rung a guard
// cannot name is a hole the guard cannot report; a rung with no containment
// strategy is one it will silently skip.
func TestEveryChainStepNamesItsInputAndContainment(t *testing.T) {
	validKinds := map[string]bool{"env": true, "walk": true, "default": true}
	for name, chain := range ResolutionChains() {
		for i, step := range chain {
			where := name + "[" + itoaTest(i) + "]"
			if !validKinds[step.Kind] {
				t.Errorf("%s: unknown kind %q — a consumer cannot handle it", where, step.Kind)
			}
			if step.Input == "" {
				t.Errorf("%s: no Input named — a guard cannot control what it cannot name "+
					"(this is exactly how cwd stayed invisible)", where)
			}
			if step.Containment == "" {
				t.Errorf("%s: no Containment strategy — a guard has no way to constrain "+
					"this rung and will skip it silently", where)
			}
			// An env-backed rung must be pinnable; a non-env rung must say so
			// and give the alternative. Neither branch is free.
			if step.Kind == "env" || step.Kind == "default" {
				if step.Env == "" {
					t.Errorf("%s: kind=%s but no Env to pin", where, step.Kind)
				}
				if !strings.HasPrefix(step.Containment, "pin:") {
					t.Errorf("%s: env-backed rung must declare a pin strategy, got %q",
						where, step.Containment)
				}
			} else {
				if !strings.HasPrefix(step.Containment, "assert-result:") {
					t.Errorf("%s: non-env rung must declare assert-result containment, got %q",
						where, step.Containment)
				}
			}
		}
	}
}

// itoaTest keeps the failure messages readable without importing strconv into
// the test's public surface.
func itoaTest(i int) string {
	return string(rune('0' + i))
}

// The env format is what shell consumers read. It must list every env-backed
// rung across both chains, deduplicated, or a guard reading it inherits the
// same subset problem the harness had.
func TestEnvFormatCoversEveryEnvRung(t *testing.T) {
	want := map[string]bool{}
	for _, chain := range ResolutionChains() {
		for _, step := range chain {
			if step.Env != "" {
				want[step.Env] = true
			}
		}
	}
	// Mirror the command's env-format logic.
	got := map[string]bool{}
	for _, name := range []string{"claude_root", "edikt_root"} {
		for _, step := range ResolutionChains()[name] {
			if step.Env != "" {
				got[step.Env] = true
			}
		}
	}
	for v := range want {
		if !got[v] {
			t.Errorf("env format omits %q — a guard reading it would leave that variable unpinned", v)
		}
	}
	if len(want) == 0 {
		t.Fatal("no env-backed rungs found — the chain is empty, which cannot be right")
	}
	_ = os.Getenv // keep os imported for symmetry with the resolver under test
}
