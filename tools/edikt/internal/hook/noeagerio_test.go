//go:build unix

package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestNoEagerIO — AC-6.10. The dispatcher performs NO index or config IO
// before the resolved handler asks for it.
//
// # WHY THIS IS MEASURED AND NOT ASSERTED BY INSPECTION
//
// `hook match` costs 23.3ms against the binary's 9.4ms floor; 14ms of that is
// per-call index work — a 2.6 MB YAML read plus a sha256, paid before any
// cache can be consulted. instructions-loaded runs in 2.5ms today. If that
// index work leaked into the common dispatch path, every trivial hook would
// get ~10x slower and a code review would never notice, because the leak would
// arrive as one innocuous helper call inside something else.
//
// # HOW THE PROBE WORKS
//
// The directive index and .edikt/config.yaml are replaced by NAMED PIPES with
// no writer. open(2) on such a FIFO BLOCKS. So any attempt to read either file
// hangs the dispatcher, and "Dispatch returned within the deadline" is direct
// evidence that neither was touched — not an inference from imports, not a
// grep, an observation of the actual syscall path.
//
// The second subtest is the sensitivity control: it proves the FIFO really
// does block a read, so a passing first subtest cannot be explained by the
// probe being inert (INV-013 — a detector that cannot fail measures nothing).
func TestNoEagerIO(t *testing.T) {
	root := t.TempDir()
	idx := plantBlockingFIFO(t, filepath.Join(root, ".claude", "rules", "governance", "directive-index.yaml"))
	cfg := plantBlockingFIFO(t, filepath.Join(root, ".edikt", "config.yaml"))

	// Journal writes go to a scratch HOME; the journal is neither index nor
	// config, and AC-6.7 requires it on every run.
	t.Setenv("HOME", t.TempDir())

	t.Run("dispatch does not touch the index or config", func(t *testing.T) {
		reg := NewRegistry()
		handlerRan := make(chan struct{})
		reg.Register("instructions-loaded", func(inv *Invocation) Result {
			close(handlerRan)
			return Result{}
		})

		done := make(chan int, 1)
		go func() {
			done <- Dispatch(Options{
				Name:     "instructions-loaded",
				Stdin:    strings.NewReader(`{"hook_event_name":"InstructionsLoaded","cwd":"` + root + `"}`),
				Stdout:   &bytes.Buffer{},
				Stderr:   &bytes.Buffer{},
				Registry: reg,
				RootFn:   func() (string, error) { return root, nil },
			})
		}()

		select {
		case code := <-done:
			if code != 0 {
				t.Fatalf("exit code: got %d, want 0", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Dispatch blocked for 5s — it read %s or %s before the handler asked "+
				"for either. Eager index IO costs ~14ms on every hook fire.", idx, cfg)
		}
		select {
		case <-handlerRan:
		default:
			t.Fatal("the handler never ran")
		}
	})

	t.Run("the probe can actually fail", func(t *testing.T) {
		read := make(chan struct{})
		go func() {
			_, _ = os.ReadFile(idx)
			close(read)
		}()
		select {
		case <-read:
			t.Fatal("reading the FIFO returned immediately — the probe is inert and the " +
				"subtest above proves nothing")
		case <-time.After(300 * time.Millisecond):
		}
		// Release the blocked reader: opening the write end succeeds
		// immediately when a reader is already waiting, and closing it
		// delivers EOF. Left blocked, the goroutine would outlive the test.
		w, err := os.OpenFile(idx, os.O_WRONLY, 0)
		if err != nil {
			t.Fatalf("open FIFO for write: %v", err)
		}
		_ = w.Close()
		<-read
	})
}

// TestDispatcherSourceDoesNotReachTheIndex is the static half.
//
// The FIFO probe proves the CURRENT common path is clean. This catches the
// next change: a helper added to run.go that calls the matcher or the config
// loader would still pass the probe if the handler under test happened to be
// the one asking. Two different failure modes, two checks.
func TestDispatcherSourceDoesNotReachTheIndex(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{"hookmatch.Match(", "hookmatch.IndexPath(", "hookcfg."}
	scanned := 0
	for _, name := range []string{"run.go", "registry.go"} {
		raw, err := os.ReadFile(filepath.Join(root, "tools", "edikt", "internal", "hook", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		for _, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			for _, f := range forbidden {
				if strings.Contains(trimmed, f) {
					t.Errorf("%s calls %s on the dispatch path: %q", name, f, trimmed)
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("scanned 0 files — nothing was measured")
	}
	t.Logf("%d dispatcher source file(s) scanned for %d forbidden call(s)", scanned, len(forbidden))
}

// plantBlockingFIFO creates a named pipe at p (making parents as needed) and
// returns its path. Reading it blocks until something opens the write end.
func plantBlockingFIFO(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", p, err)
	}
	return p
}
