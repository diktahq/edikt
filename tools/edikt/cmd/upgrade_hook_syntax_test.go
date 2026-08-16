package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// F-036 / F-051 — the hook syntax gate, and the correction that motivates it.
//
// F-051 recorded that an unparseable shim "makes bash exit 2 before line 1
// runs", so the in-script `_allow` fail-open branches are "unreachable by
// construction". That claim was agent-reported and UNVERIFIED. Measured, it is
// wrong, and TestFailOpenReachabilityIsPositional is the measurement.
//
// The correction does not remove the need for an external check — it changes
// what the check is for. The in-script guard works for errors below it and not
// above it, which is a guarantee that silently evaporates whenever an edit
// lands higher in the file, with nothing reporting the lost coverage.

// TestFailOpenReachabilityIsPositional is the experiment, kept as a test so
// the claim stays falsifiable rather than becoming folklore in a comment.
func TestFailOpenReachabilityIsPositional(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	dir := t.TempDir()

	// The fail-open branch fires unconditionally when reached.
	failOpen := "_allow() { printf '{\"continue\": true}\\n'; exit 0; }\ntrue || true\n_allow\n"
	broken := "if [ -z \"$X\"\n" // unterminated `if` — syntax error at EOF

	cases := []struct {
		name        string
		body        string
		wantExit    int
		wantAllowed bool
	}{
		{
			name:        "syntax error BELOW the fail-open branch: _allow is reached",
			body:        "#!/usr/bin/env bash\n" + failOpen + broken,
			wantExit:    0,
			wantAllowed: true,
		},
		{
			name:        "syntax error ABOVE the fail-open branch: nothing runs, exit 2",
			body:        "#!/usr/bin/env bash\n" + broken + failOpen,
			wantExit:    2,
			wantAllowed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+".sh")
			if err := os.WriteFile(p, []byte(tc.body), 0o755); err != nil {
				t.Fatal(err)
			}
			out, _ := exec.Command("bash", p).Output()
			code := 0
			if err := exec.Command("bash", p).Run(); err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					code = ee.ExitCode()
				}
			}
			allowed := strings.Contains(string(out), `"continue": true`)

			if code != tc.wantExit {
				t.Errorf("exit code = %d, want %d", code, tc.wantExit)
			}
			if allowed != tc.wantAllowed {
				t.Errorf("fail-open reached = %v, want %v (output %q)", allowed, tc.wantAllowed, string(out))
			}
		})
	}
	// Both subtests together are the finding: neither alone shows it. One says
	// fail-open works, the other says it does not, and the difference is only
	// the position of the error.
}

func TestCheckHookSyntax(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}

	newPayload := func(t *testing.T, hooks map[string]string) string {
		t.Helper()
		root := t.TempDir()
		hdir := filepath.Join(root, "templates", "hooks")
		if err := os.MkdirAll(hdir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range hooks {
			if err := os.WriteFile(filepath.Join(hdir, name), []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}

	t.Run("a payload of parseable hooks passes", func(t *testing.T) {
		// ISOLATION. Without this, a gate that refused everything would pass
		// every other case here while making upgrade impossible.
		root := newPayload(t, map[string]string{
			"a.sh": "#!/usr/bin/env bash\necho ok\n",
			"b.sh": "#!/usr/bin/env bash\nif [ -z \"$X\" ]; then echo y; fi\n",
		})
		if err := checkHookSyntax(root); err != nil {
			t.Fatalf("a valid payload was refused: %v", err)
		}
	})

	t.Run("a hook that does not parse is refused, and named", func(t *testing.T) {
		root := newPayload(t, map[string]string{
			"good.sh": "#!/usr/bin/env bash\necho ok\n",
			"bad.sh":  "#!/usr/bin/env bash\nif [ -z \"$X\"\n",
		})
		err := checkHookSyntax(root)
		if err == nil {
			t.Fatal("a payload with an unparseable hook was accepted")
		}
		// Naming the file is the difference between an actionable refusal and
		// a mystery: the operator has to know WHICH hook to look at.
		if !strings.Contains(err.Error(), "bad.sh") {
			t.Fatalf("refusal does not name the offending hook: %v", err)
		}
	})

	t.Run("a payload with NO hooks is refused, not reported clean", func(t *testing.T) {
		// INV-013. Zero scripts checked is UNMEASURED. Passing here would make
		// a payload that silently stopped shipping hooks indistinguishable
		// from one whose hooks are all fine.
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := checkHookSyntax(root); err == nil {
			t.Fatal("an empty payload was reported as checked; absence is not a pass")
		}
	})

	t.Run("non-hook .sh files are out of scope", func(t *testing.T) {
		// A broken test fixture elsewhere in the payload must not block an
		// upgrade — the gate's claim is about hooks, and its reach should
		// match its claim.
		root := newPayload(t, map[string]string{"a.sh": "#!/usr/bin/env bash\necho ok\n"})
		other := filepath.Join(root, "test", "fixtures")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(other, "deliberately-broken.sh"),
			[]byte("#!/usr/bin/env bash\nif [ -z \"$X\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkHookSyntax(root); err != nil {
			t.Fatalf("a broken non-hook script blocked the upgrade: %v", err)
		}
	})
}

// TestShippedHooksParse runs the gate against the hooks this repo actually
// ships, so the release cannot regress the property the gate protects.
func TestShippedHooksParse(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	root := "../../.." // repo root from tools/edikt/cmd
	if _, err := os.Stat(filepath.Join(root, "templates", "hooks")); err != nil {
		t.Skipf("templates/hooks not reachable from test cwd: %v", err)
	}
	if err := checkHookSyntax(root); err != nil {
		t.Fatalf("a shipped hook does not parse: %v", err)
	}
}
