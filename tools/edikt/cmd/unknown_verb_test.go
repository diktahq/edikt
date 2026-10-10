package cmd

import (
	"errors"
	"strings"
	"testing"
)

// An unknown verb exits NON-ZERO at every level. `edikt zzz` and
// `edikt gov zzz` both exited 0 (D83, bench conformance screen), so a caller
// could not tell success from "that verb does not exist" — a CI step calling
// a misspelled or not-yet-released verb goes green forever.
func TestUnknownVerb_ExitsNonZeroAtEveryLevel(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string // fragments the refusal must carry
	}{
		{"root", []string{"zzz"}, []string{"unknown edikt subcommand: zzz", "available:", "gov", "doctor"}},
		{"group", []string{"gov", "zzz"}, []string{"unknown edikt gov subcommand: zzz", "compile"}},
		{"nested group", []string{"util", "yaml", "zzz"}, []string{"unknown edikt util yaml subcommand: zzz", "validate"}},
		{"group with no verb", []string{"gov"}, []string{"edikt gov needs a subcommand", "compile"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := runCmd(t, c.args...)
			if err == nil {
				t.Fatalf("must exit non-zero:\n%s", out)
			}
			combined := out + " " + err.Error()
			for _, w := range c.want {
				if !strings.Contains(combined, w) {
					t.Errorf("refusal does not carry %q:\n%s", w, combined)
				}
			}
		})
	}
}

// The exit code says WHICH kind of failure: 3 is "invalid args" in edikt's
// convention, distinct from 1 ("the command ran and reported a failure"), so
// a caller can tell a wrong invocation from a real negative result.
func TestUnknownVerb_UsesTheInvalidArgsCode(t *testing.T) {
	err := unknownVerb(rootCmd, []string{"zzz"})
	var ece *exitCodeError
	if !errors.As(err, &ece) {
		t.Fatalf("want an exitCodeError, got %v", err)
	}
	if ece.code != 3 {
		t.Fatalf("exit code %d, want 3 (invalid args)", ece.code)
	}
}

// The refusal must not break a real verb, a group's help, or --help.
func TestUnknownVerb_RealVerbsAndHelpStillSucceed(t *testing.T) {
	for _, args := range [][]string{
		{"version"},
		{"gov", "compile", "--help"},
		{"util", "yaml", "--help"},
		{"--help"},
	} {
		if out, err := runCmd(t, args...); err != nil {
			t.Errorf("edikt %s must succeed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}
