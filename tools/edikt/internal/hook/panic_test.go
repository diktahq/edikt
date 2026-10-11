package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// capture collects journal records instead of writing the user's real journal.
type capture struct{ records []hookmatch.Record }

func (c *capture) append(r hookmatch.Record) error {
	c.records = append(c.records, r)
	return nil
}

// dispatchableNames is the per-hook subtest table.
//
// It is the registry file's non-library rows UNION the names actually
// registered, not just the registered set. Phase 6 registers zero handlers, so
// iterating Lookup's keys alone would produce an empty table and a test that
// passed by doing nothing. Driving from the declared rows means every hook is
// covered from the day it is declared, and a newly migrated hook is covered
// automatically the moment its init() registers it.
func dispatchableNames(t *testing.T) []string {
	t.Helper()
	f := loadRepoRegistry(t)
	seen := map[string]bool{}
	var out []string
	for _, r := range f.Hooks {
		if r.Class == ClassLibrary {
			continue // never registered, never given a payload (ADR-063:d11)
		}
		if !seen[r.Name] {
			seen[r.Name] = true
			out = append(out, r.Name)
		}
	}
	for _, n := range Names() {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		t.Fatal("0 dispatchable hook names — the panic table is empty and would prove nothing")
	}
	return out
}

// TestPanicRecovery — one subtest per hook. A panicking handler MUST exit 0,
// emit a valid empty hook-protocol reply, and journal panic-recovered.
//
// A Go panic exiting non-zero at PreToolUse would BLOCK the user's write. That
// is strictly worse than the missing-interpreter behaviour this migration
// replaces, which is why the assertion is on the exit code first.
func TestPanicRecovery(t *testing.T) {
	names := dispatchableNames(t)
	t.Logf("%d hook name(s) under test", len(names))

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			reg := NewRegistry()
			reg.Register(name, func(inv *Invocation) Result {
				var nilMap map[string]string
				//lint:ignore SA5000 deliberate: the test needs a real runtime panic, not panic("x")
				nilMap["boom"] = "this is a nil map write"
				return Result{}
			})

			var stdout, stderr bytes.Buffer
			cap := &capture{}
			code := Dispatch(Options{
				Name:     name,
				Stdin:    strings.NewReader(`{"session_id":"s1"}`),
				Stdout:   &stdout,
				Stderr:   &stderr,
				Registry: reg,
				Journal:  cap.append,
			})

			if code != 0 {
				t.Fatalf("exit code: got %d, want 0 — a non-zero exit at PreToolUse blocks the write", code)
			}
			var reply map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &reply); err != nil {
				t.Fatalf("stdout is not valid JSON after a panic: %q (%v)", stdout.String(), err)
			}
			if len(reply) != 0 {
				t.Fatalf("stdout after a panic: got %q, want an empty hook-protocol reply", stdout.String())
			}
			if !strings.Contains(stderr.String(), "PANIC recovered") {
				t.Errorf("stderr does not name the panic: %q", stderr.String())
			}

			if len(cap.records) != 1 {
				t.Fatalf("journal records: got %d, want exactly 1", len(cap.records))
			}
			r := cap.records[0]
			if r.Outcome != string(hookmatch.OutcomePanicRecovered) {
				t.Errorf("outcome: got %q, want %q", r.Outcome, hookmatch.OutcomePanicRecovered)
			}
			if r.Hook != name {
				t.Errorf("hook: got %q, want %q", r.Hook, name)
			}
			if !hookmatch.Outcome(r.Outcome).Suppressed() {
				t.Errorf("outcome %q is not Suppressed() — a hook that panicked would be counted as healthy", r.Outcome)
			}
		})
	}
}

// TestPanicRecoveryReachesTheRealJournal proves the default wiring, not just
// the injectable one.
//
// Every other test in this file passes an injected Journal func. Without this,
// all of them could pass while hookmatch.Append was never actually wired up —
// the seam would be tested and the product would not be.
func TestPanicRecoveryReachesTheRealJournal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// State resolves EDIKT_HOME before HOME (7e799074); test/run.sh exports the
	// developer's live EDIKT_HOME, so HOME alone left this test looking for a
	// journal the hook wrote into ~/.edikt (INV-007).
	t.Setenv("EDIKT_HOME", filepath.Join(home, ".edikt"))

	reg := NewRegistry()
	reg.Register("post-compact", func(*Invocation) Result { panic("deliberate") })

	var stdout, stderr bytes.Buffer
	if code := Dispatch(Options{
		Name: "post-compact", Stdin: strings.NewReader("{}"),
		Stdout: &stdout, Stderr: &stderr, Registry: reg,
	}); code != 0 {
		t.Fatalf("exit code: got %d, want 0", code)
	}

	raw, err := os.ReadFile(filepath.Join(home, ".edikt", "state", "hook-journal.jsonl"))
	if err != nil {
		t.Fatalf("the real journal was not written: %v", err)
	}
	if !strings.Contains(string(raw), string(hookmatch.OutcomePanicRecovered)) {
		t.Fatalf("journal does not carry the panic outcome: %s", raw)
	}
	if !strings.Contains(string(raw), `"hook":"post-compact"`) {
		t.Fatalf("journal does not name the hook: %s", raw)
	}
}

// TestSinglePanicRecoverySite — FR-006's structural requirement.
//
// Panic recovery is ONE defer recover() at the dispatch call site, never
// reimplemented per handler. Twenty-two recovery sites is twenty-two chances
// to get the exit code wrong, and getting it wrong blocks a write.
func TestSinglePanicRecoverySite(t *testing.T) {
	root := repoRoot(t)
	pkgDir := filepath.Join(root, "tools", "edikt", "internal", "hook")

	type site struct {
		file string
		line int
	}
	var sites []site
	filesScanned := 0

	err := filepath.WalkDir(pkgDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		filesScanned++
		for i, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue // prose about recovery is not recovery
			}
			if strings.Contains(trimmed, "recover()") {
				rel, _ := filepath.Rel(root, p)
				sites = append(sites, site{file: rel, line: i + 1})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", pkgDir, err)
	}
	if filesScanned == 0 {
		t.Fatalf("scanned 0 non-test Go files under %s — nothing was measured", pkgDir)
	}
	t.Logf("%d non-test Go file(s) scanned, %d recover() site(s) found", filesScanned, len(sites))

	if len(sites) != 1 {
		t.Fatalf("expected exactly 1 recover() site in the hook tier, found %d: %v", len(sites), sites)
	}
	if filepath.Base(sites[0].file) != "run.go" {
		t.Fatalf("the recovery site is in %s; it belongs at the dispatcher in run.go", sites[0].file)
	}
}
