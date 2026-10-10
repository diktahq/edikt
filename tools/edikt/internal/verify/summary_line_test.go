package verify

import (
	"strings"
	"testing"
)

// D74 (a): the console summary names every class, unmeasured included. The
// regression was a line that read "4 passed, 0 failed, 0 timeout, 0 skipped"
// over a report holding seven unmeasured criteria.
func TestSummaryLine_NamesUnmeasured(t *testing.T) {
	s := Summary{Passed: 4, Unmeasured: 7, Total: 11}
	line := s.Line()
	if !strings.Contains(line, "7 unmeasured") {
		t.Fatalf("unmeasured class missing from the summary: %q", line)
	}
	for _, class := range []string{"passed", "failed", "unmeasured", "timeout", "skipped", "total"} {
		if !strings.Contains(line, class) {
			t.Errorf("class %q not named in %q", class, line)
		}
	}
}
