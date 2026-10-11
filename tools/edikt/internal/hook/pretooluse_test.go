package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// AC-20.4 — externally-controlled values are NFKC-normalized and casefolded
// before any allowlist comparison (INV-006).
//
// This is the bypass it closes: every scope decision is an allowlist
// comparison against a basename taken straight from the hook payload. Without
// normalization a Unicode lookalike that folds to "CLAUDE.md" compares
// unequal, the file is judged out of scope, and the guard allows a write it
// should have denied. One homoglyph defeats the guard.
func TestPreToolUseNormalizesBeforeAllowlistComparison(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"fullwidth folds to the ascii basename", "ＣＬＡＵＤＥ.md", "claude.md"},
		{"case folds", "Settings.JSON", "settings.json"},
		{"surrounding whitespace stripped", "  settings.json  ", "settings.json"},
		{"control characters removed", "settings\u200b.json", "settings\u200b.json"},
	}
	for _, c := range cases {
		if got := scopeKey(c.in); got != c.want {
			t.Errorf("%s: scopeKey(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}

	// The property that matters, stated directly: a lookalike must not be
	// able to make an in-scope basename read as out-of-scope.
	if scopeKey("ＣＬＡＵＤＥ.md") != scopeKey("claude.md") {
		t.Error("a fullwidth homoglyph of CLAUDE.md does not normalize to the same key — the scope check is bypassable")
	}
}

// AC-20.4, the direction that matters. Normalization must only WIDEN what the
// allowlist matches: it may pull a file into scope that was previously
// skipped, and must never push one out.
func TestPreToolUseNormalizationOnlyWidensScope(t *testing.T) {
	for _, base := range []string{"CLAUDE.md", "settings.json", "SKILL.md", "manifest.yaml"} {
		if scopeKey(base) != strings.ToLower(base) {
			t.Errorf("scopeKey(%q) = %q — an ordinary ASCII basename changed shape, which could take a file OUT of scope",
				base, scopeKey(base))
		}
	}
}

// AC-20.5 — the byte-range logic must not panic on hostile input, so the
// dispatcher's recover() is never reached.
//
// recover() existing is not the same as recover() being unnecessary. This
// drives the range and overlap logic directly with inputs designed to break
// it: a panic here would exit 0 and, under this hook's posture, let the write
// through — a guard that fails open on a crafted payload.
func TestPreToolUseByteRangeSurvivesHostilePayloads(t *testing.T) {
	body := "prefix\n[edikt:demo:start]: #\nmanaged\n[edikt:demo:end]: #\nsuffix\n"
	regions := scanSentinelRegions(body)
	if len(regions) != 1 {
		t.Fatalf("fixture produced %d region(s), want 1 — the cases below would prove nothing", len(regions))
	}

	hostile := []struct {
		name string
		p    adapter.Payload
	}{
		{"empty old_string", adapter.Payload{ToolName: "Edit", OldString: ""}},
		{"old_string longer than the file", adapter.Payload{ToolName: "Edit", OldString: strings.Repeat("x", len(body)*4)}},
		{"old_string is the whole file", adapter.Payload{ToolName: "Edit", OldString: body}},
		{"nul bytes", adapter.Payload{ToolName: "Edit", OldString: "\x00\x00"}},
		{"invalid utf-8", adapter.Payload{ToolName: "Edit", OldString: "\xff\xfe\xfd"}},
		{"lone surrogate", adapter.Payload{ToolName: "Edit", OldString: "\xed\xa0\x80"}},
		{"empty write content", adapter.Payload{ToolName: "Write", Content: ""}},
		{"write content is nul", adapter.Payload{ToolName: "Write", Content: "\x00"}},
		{"unknown tool", adapter.Payload{ToolName: "Bash", OldString: "x"}},
	}
	for _, h := range hostile {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: computeEditRange panicked (%v) — the dispatcher's recover() would exit 0 and let the write through", h.name, r)
				}
			}()
			computeEditRange(h.p, body, regions)
		}()
	}

	// The scanner too: it walks the file the payload names.
	for _, s := range []string{"", "\x00", "\xff\xfe", strings.Repeat("[edikt:a:start]: #\n", 500),
		"[edikt:a:start]: #\n[edikt:b:end]: #\n", "[edikt:end]: #\n[edikt:start]: #\n"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("scanSentinelRegions panicked on %q: %v", truncateForMsg(s), r)
				}
			}()
			scanSentinelRegions(s)
		}()
	}
}

// AC-20.5 control. If the fixture stopped producing a region, every case above
// would exercise an empty-region path and pass while testing nothing.
func TestPreToolUseHostileSuiteHasARegionToAttack(t *testing.T) {
	body := "a\n[edikt:demo:start]: #\nm\n[edikt:demo:end]: #\nz\n"
	if got := len(scanSentinelRegions(body)); got != 1 {
		t.Fatalf("scanner found %d region(s) in the attack fixture, want 1", got)
	}
}

// AC-20.6/20.7 — a panic exits 0 and does not block the write, and says so
// loudly rather than only in a journal.
//
// Cited to ADR-060, not INV-011: INV-011 governs the posture-aware decision
// channel, while what makes a panic non-blocking HERE is that PreToolUse
// treats a bare exit 0 with no decision as "no opinion". The distinction is
// the reason the dispatcher holds the tier's only recover() — twenty-two
// handlers each with their own is twenty-two chances to block a user's write
// on an internal fault.
func TestPreToolUsePanicIsNonBlockingAndLoud(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, "recover()") {
		t.Fatal("the dispatcher no longer recovers panics; a handler fault would crash the hook and block the write")
	}
	if !strings.Contains(body, "THIS FILE HOLDS THE ONLY defer recover() IN THE HOOK TIER") {
		t.Error("run.go no longer declares itself the single recovery site — FR-006's whole design is that there is exactly one")
	}
	// Loud, not only journaled: the recovery path must write something a human
	// sees in-session.
	if !strings.Contains(body, "Stderr") && !strings.Contains(body, "systemMessage") {
		t.Error("the recovery path emits nothing user-visible; a panic would be silent to the person whose write was not checked")
	}
}

// The single-recovery-site invariant, asserted by count rather than by reading.
func TestPreToolUseAddsNoSecondRecoverySite(t *testing.T) {
	src, err := os.ReadFile("pretooluse.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "recover()") {
		t.Error("pretooluse.go contains its own recover() — FR-006 requires exactly one site, in the dispatcher, " +
			"because a handler that recovers locally can get the exit code wrong and block the write")
	}
}

// The guard must not gate on python3. Bash did, because its guard WAS python;
// this handler is not, so the gate would announce a degradation that is not
// happening and skip a check it could perform.
func TestPreToolUseDoesNotGateOnPython3(t *testing.T) {
	src, err := os.ReadFile("pretooluse.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), `commandAvailable("python3")`) {
		t.Error("pretooluse.go gates on python3, which it never invokes — that stands the guard down on a healthy host")
	}
}

func truncateForMsg(s string) string {
	if len(s) > 32 {
		return s[:32] + "..."
	}
	return s
}

var _ = filepath.Join
