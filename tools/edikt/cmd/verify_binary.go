package cmd

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

// binaryProvenance is what the running binary knows about its own source.
type binaryProvenance struct {
	Revision string // vcs.revision, "" when the build was not stamped
	Modified bool   // vcs.modified
}

// readBinaryProvenance reads the build-info settings Go stamps at build time.
func readBinaryProvenance() binaryProvenance {
	var p binaryProvenance
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return p
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			p.Revision = s.Value
		case "vcs.modified":
			p.Modified = s.Value == "true"
		}
	}
	return p
}

// binaryFreshness decides whether the verifier may run against a plan at
// head. A verify runner built from an older commit measures the plan with
// yesterday's checks — the D74 finding: a checkout binary at 5734d41c reported
// seven criteria unmeasured that a HEAD build measured. So:
//
//   - revision known and not head: REFUSE, unless --allow-stale-binary
//   - revision known, modified tree: warn loudly, run
//   - revision unknown (unstamped build, e.g. a linked worktree): warn loudly, run
//
// The head comparison is by prefix so a short SHA on either side still
// matches its long form.
func binaryFreshness(p binaryProvenance, head string, allowStale bool, w io.Writer) error {
	head = strings.TrimSuffix(strings.TrimSpace(head), "-dirty")
	switch {
	case p.Revision == "":
		fmt.Fprintln(w, "warn: this binary carries no vcs.revision (unstamped build) — it may not match the checkout it is verifying. Rebuild from the checkout root: go build -C tools/edikt -o ../../bin/edikt ./")
		return nil
	case !isHexSHA(head):
		// Not a git checkout, or HEAD unreadable: there is nothing to compare
		// against, and "unknown" is not a commit this binary could be stale
		// relative to. Say so; do not refuse.
		fmt.Fprintf(w, "warn: the checkout's HEAD is not readable (%q) — this binary (built at %s) cannot be checked against it.\n", head, short(p.Revision))
		return nil
	case !strings.HasPrefix(p.Revision, head) && !strings.HasPrefix(head, p.Revision):
		msg := fmt.Sprintf("this binary was built at %s but the checkout is at %s — a stale runner verifies the plan with old checks (D74). Rebuild: go build -C tools/edikt -o ../../bin/edikt ./  (or pass --allow-stale-binary to run anyway)", short(p.Revision), short(head))
		if allowStale {
			fmt.Fprintln(w, "warn: "+msg)
			return nil
		}
		return fmt.Errorf("stale binary: %s", msg)
	case p.Modified:
		fmt.Fprintf(w, "warn: this binary was built at %s from a MODIFIED tree — uncommitted changes may or may not be in it. Rebuild before trusting a phase verdict.\n", short(p.Revision))
		return nil
	}
	return nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func isHexSHA(s string) bool {
	if len(s) < 7 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
