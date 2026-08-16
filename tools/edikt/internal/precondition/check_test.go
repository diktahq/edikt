package precondition

import (
	"reflect"
	"sort"
	"testing"
)

// present is a path oracle over a fixed set, so these cases never depend on
// the working tree. The corpus check does the filesystem half; this does the
// classification half, and the two must not be entangled.
func present(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) bool { return set[p] }
}

func TestCheckCommand(t *testing.T) {
	cases := []struct {
		name        string
		verify      string
		exists      func(string) bool
		want        Verdict
		wantMissing []string
	}{
		// THE CASE THIS CHECK EXISTS FOR. Greenfield extraction emitted a
		// verify naming a file absent from the tree; rg exited non-zero and
		// the runner scored FAIL, so a missing file read as a governance
		// violation. It must read as precondition_absent instead.
		{"greenfield: rg over a file not in the tree",
			"rg -q 'func Chunk' internal/rag/chunk.go",
			present(),
			Absent, []string{"internal/rag/chunk.go"}},

		{"satisfied: file present",
			"test -f templates/settings.json.tmpl",
			present("templates/settings.json.tmpl"),
			Satisfied, nil},

		{"one of several operands missing",
			"test -f a/present.sh && grep -q 'x' b/absent.md",
			present("a/present.sh"),
			Absent, []string{"b/absent.md"}},

		// ── Corpus correction 1: a quoted operand is a PATTERN ──
		// ADR-038's verify names .edikt/state/.evidence-reads twice: once as
		// a git operand, once inside a grep pattern. Counting the pattern as
		// a path reports a file the command never opens.
		{"corpus: slash inside a quoted pattern is not a path", // ADR-038[49]
			"grep -q '.edikt/state/.evidence-reads\\|.edikt/state/' .gitignore",
			present(".gitignore"),
			Satisfied, nil},

		// ── Corpus correction 2: absence IS the assertion ──
		// ADR-044 verifies that a file ADR-044 deleted stays deleted.
		// Flagging it inverts the finding.
		{"corpus: test ! -f asserts the path is GONE", // ADR-044[4]
			"test ! -f tools/edikt/internal/gradecompile/dispatch.go",
			present(),
			Unresolvable, nil},
		{"corpus: negated file test alongside a real operand", // ADR-044[4] full
			"bash tools/edikt/check/no-llm-in-tier-2.sh && test ! -f tools/edikt/internal/gradecompile/dispatch.go",
			present("tools/edikt/check/no-llm-in-tier-2.sh"),
			Satisfied, nil},
		// But a negated MATCH is not a negated existence claim. The file
		// still has to be there, and this must stay detectable.
		{"negated grep still requires its file",
			"! grep -q 'DispatchAdversary' tools/edikt/internal/benchmark/cheatrate/adversary.go",
			present(),
			Absent, []string{"tools/edikt/internal/benchmark/cheatrate/adversary.go"}},

		// ── Corpus correction 3: cd changes the resolution base ──
		{"corpus: cd before the operand", // INV-011[2]
			"cd tools/edikt && go test ./internal/phasea/ -run TestX",
			present("tools/edikt/internal/phasea"),
			Satisfied, nil},

		// ── Corpus correction 4: name-only predicates ──
		// git check-ignore exits 0 for a path that does not exist, so its
		// operand is a NAME being reasoned about, not a file being opened.
		{"corpus: git check-ignore operand need not exist", // ADR-050[4]
			"git check-ignore -q .edikt/state/hook-acks.json",
			present(),
			Unresolvable, nil},

		// ── Declines, none of which may read as satisfied ──
		{"shell expansion is declined", "test -f $HOME/x.md", present(), Unresolvable, nil},
		{"glob is declined", "test -f docs/**/*.md", present(), Unresolvable, nil},
		{"absolute path is declined", "test -f /etc/hosts", present(), Unresolvable, nil},

		// ── No path operands at all ──
		{"help flag has no path operands",
			"bin/edikt gov benchmark cheat-rate --help >/dev/null 2>&1",
			present("bin/edikt"),
			Satisfied, nil},
		{"pure pipeline over stdin", "echo hi | grep -q hi", present(), NoPathOperands, nil},
		{"empty verify", "", present(), NoPathOperands, nil},

		// Redirections are not preconditions.
		{"redirect targets are not operands",
			"bash test/x.sh >/dev/null 2>&1",
			present("test/x.sh"),
			Satisfied, nil},

		// Command substitution must be entered, or a real path hides inside
		// a double-quoted token.
		{"corpus: path inside $( ) is found",
			`test "$(grep -cv '^$' tools/edikt/check/no-llm-in-tier-2.exempt)" -eq 1`,
			present(),
			Absent, []string{"tools/edikt/check/no-llm-in-tier-2.exempt"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, missing, _, _, _ := CheckCommand(c.verify, c.exists)
			if got != c.want {
				t.Errorf("verdict = %q, want %q\n  verify: %s\n  missing: %v",
					got, c.want, c.verify, missing)
			}
			sort.Strings(missing)
			want := append([]string(nil), c.wantMissing...)
			sort.Strings(want)
			if len(missing) != 0 || len(want) != 0 {
				if !reflect.DeepEqual(missing, want) {
					t.Errorf("missing = %v, want %v\n  verify: %s", missing, want, c.verify)
				}
			}
		})
	}
}
