package hook

import (
	"os"
	"testing"
)

// TestMain clears the two environment variables that outrank HOME in the
// state ladder (EDIKT_ROOT > EDIKT_HOME > $HOME/.edikt, 7e799074) before any
// test in this package runs. Every test here isolates by pointing HOME at a
// temp dir, and test/run.sh exports the developer's live EDIKT_HOME — so under
// the suite the tests were reading and writing ~/.edikt/state, and one of
// them rotated the live journal (INV-007). Tests that exercise the ladder set
// the variables themselves with t.Setenv, which still works after this.
func TestMain(m *testing.M) {
	os.Unsetenv("EDIKT_HOME")
	os.Unsetenv("EDIKT_ROOT")
	os.Exit(m.Run())
}
