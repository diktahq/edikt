package parse

import "testing"

// TestFencePrefix_StrictTypeAndLength pins Phase 3 §3.2: fence detection
// must distinguish marker char (``` vs ~~~) AND require run-length ≥ 3.
func TestFencePrefix_StrictTypeAndLength(t *testing.T) {
	cases := []struct {
		in        string
		wantChar  byte
		wantLen   int
	}{
		{"```", '`', 3},
		{"````", '`', 4},
		{"~~~", '~', 3},
		{"~~~~~~", '~', 6},
		{"```python", '`', 3},
		{"~~~yaml", '~', 3},

		// Below 3 chars or wrong char — no fence.
		{"``", 0, 0},
		{"~~", 0, 0},
		{"#", 0, 0},
		{"", 0, 0},
		{"text", 0, 0},
	}
	for _, tc := range cases {
		c, n := fencePrefix(tc.in)
		if c != tc.wantChar || n != tc.wantLen {
			t.Errorf("fencePrefix(%q) = (%q, %d); want (%q, %d)",
				tc.in, c, n, tc.wantChar, tc.wantLen)
		}
	}
}

// TestFindLineStart_MixedFence_TildeInsideBacktick pins Phase 3 §3.2:
// a `~~~` line inside a ``` block does NOT close the block. A sentinel
// marker that follows must be classified as inside-fence (skipped).
func TestFindLineStart_MixedFence_TildeInsideBacktick(t *testing.T) {
	body := "intro\n" +
		"```markdown\n" +
		"~~~\n" + // mixed-marker line — should NOT close the ``` block
		"[edikt:directives:start]: #\n" +
		"~~~\n" +
		"```\n" +
		"after\n"
	idx := findLineStart(body, "[edikt:directives:start]: #")
	if idx != -1 {
		t.Fatalf("mixed-fence ~~~ inside ``` block should keep marker fenced; got idx=%d (body[idx:]=%q)",
			idx, body[idx:idx+30])
	}
}

// TestFindLineStart_MixedFence_BacktickInsideTilde pins the inverse:
// ``` inside ~~~ must not close the ~~~ block.
func TestFindLineStart_MixedFence_BacktickInsideTilde(t *testing.T) {
	body := "intro\n" +
		"~~~markdown\n" +
		"```\n" + // mixed-marker — does NOT close ~~~ block
		"[edikt:directives:start]: #\n" +
		"```\n" +
		"~~~\n" +
		"after\n"
	idx := findLineStart(body, "[edikt:directives:start]: #")
	if idx != -1 {
		t.Fatalf("mixed-fence ``` inside ~~~ block should keep marker fenced; got idx=%d", idx)
	}
}

// TestFindLineStart_LengthMatch_CommonMark pins the close-length rule:
// a 3-char close cannot terminate a 4-char open.
func TestFindLineStart_LengthMatch_CommonMark(t *testing.T) {
	body := "intro\n" +
		"````markdown\n" +
		"```\n" + // 3 backticks — TOO SHORT to close a 4-backtick fence
		"[edikt:directives:start]: #\n" +
		"````\n" + // proper 4-backtick close
		"after\n"
	idx := findLineStart(body, "[edikt:directives:start]: #")
	if idx != -1 {
		t.Fatalf("3-backtick line cannot close 4-backtick fence; marker should stay inside-fence (got idx=%d)", idx)
	}
}

// TestFindLineStart_FindsUnfencedMarker confirms the happy path: a
// marker outside any fence is found.
func TestFindLineStart_FindsUnfencedMarker(t *testing.T) {
	body := "intro\n" +
		"```markdown\n" +
		"[edikt:directives:start]: #\n" + // INSIDE fence, must skip
		"```\n" +
		"\n" +
		"[edikt:directives:start]: #\n" + // OUTSIDE fence, MUST be found
		"content\n"
	idx := findLineStart(body, "[edikt:directives:start]: #")
	if idx == -1 {
		t.Fatal("unfenced marker should be found; got -1")
	}
	if body[idx-1] != '\n' {
		t.Fatalf("found marker is not column-0; idx=%d byte-before=%q", idx, body[idx-1])
	}
}
