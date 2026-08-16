package reextract

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// THE GATE THAT MAKES THE MIRROR SAFE.
//
// fixtureProofYAML is a byte-identical copy of the authoritative,
// human-edited file at FixtureProofRelPath. A copy with no gate is a second
// definition that can silently drift stale; a copy with this gate is a
// mirror. Same pattern as internal/sidecar/schemavalidate_test.go's
// TestSchemaMirrorIsByteIdentical.
func TestFixtureProofMirrorIsByteIdentical(t *testing.T) {
	authoritative := filepath.Join("..", "..", "..", "..", FixtureProofRelPath)
	want, err := os.ReadFile(authoritative)
	if err != nil {
		t.Fatalf("cannot read the authoritative fixture-validation record at %s: %v", authoritative, err)
	}
	if string(want) != string(fixtureProofYAML) {
		t.Fatalf("embedded fixture-validation mirror has DRIFTED from %s.\n"+
			"Re-copy it:\n"+
			"  cp %s tools/edikt/internal/reextract/fixtureproof/",
			authoritative, FixtureProofRelPath)
	}
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func gitCommitAll(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

// fakeRunner counts dispatches and writes a plausible sidecar, so a test can
// assert on WHAT WAS DISPATCHED rather than on what the code said it would do.
type fakeRunner struct {
	mu         sync.Mutex
	dispatched []string
	failIDs    map[string]bool
	// killAfter simulates a mid-run kill: once this many tasks have been
	// dispatched, every further Resync returns an error WITHOUT writing,
	// standing in for a process that died with work outstanding.
	killAfter int
}

func (f *fakeRunner) Preflight() error { return nil }

func (f *fakeRunner) Resync(_ context.Context, t phasea.Task) error {
	f.mu.Lock()
	f.dispatched = append(f.dispatched, t.ArtifactID)
	n := len(f.dispatched)
	f.mu.Unlock()

	if f.killAfter > 0 && n > f.killAfter {
		return errString("simulated kill: process died before extracting " + t.ArtifactID)
	}
	if f.failIDs[t.ArtifactID] {
		return errString("extractor failed on " + t.ArtifactID)
	}
	body := "schema_version: 2\nartifact: " + t.ArtifactID + "\nregenerated: true\n"
	return os.WriteFile(t.SidecarPath, []byte(body), 0o644)
}

func (f *fakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.dispatched)
}

type errString string

func (e errString) Error() string { return string(e) }

// newCorpus builds a scratch project with n ADRs, each with a parent .md and a
// sidecar, plus the config and the installed extractor agent.
func newCorpus(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	dec := filepath.Join(root, "docs/architecture/decisions")
	mustMkdir(t, dec)
	mustMkdir(t, filepath.Join(root, "docs/architecture/invariants"))
	mustMkdir(t, filepath.Join(root, "docs/guidelines"))
	mustMkdir(t, filepath.Join(root, ".claude/agents"))
	mustWrite(t, filepath.Join(root, ".edikt/config.yaml"), "base: docs\n")
	mustWrite(t, filepath.Join(root, ".claude/agents/sidecar-extractor.md"),
		"---\nname: sidecar-extractor\nmodel: claude-opus-5\nprompt_version: 4\n---\nbody\n")

	for i := 1; i <= n; i++ {
		id := adrID(i)
		base := filepath.Join(dec, id+"-fixture")
		mustWrite(t, base+".md", "---\nid: "+id+"\nstatus: accepted\n---\n\n# "+id+"\n\nMUST do the thing.\n")
		mustWrite(t, base+".edikt.yaml", "schema_version: 2\nartifact: "+id+"\nregenerated: false\n")
	}
	return root
}

func adrID(i int) string {
	s := "00" + itoa(i)
	return "ADR-" + s[len(s)-3:]
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func baseOpts(root string, r *fakeRunner) Options {
	return Options{
		ProjectRoot: root,
		Force:       true,
		Concurrency: 2,
		Runner:      r,
		Stderr:      io_Discard{},
		// These tests exercise DISPATCH MECHANICS — force, idempotence,
		// resume, blast radius. The fixture-validation precondition is a
		// different property with its own test below; leaving it armed here
		// would make every one of them a test of that precondition instead.
		SkipFixtureProof: true,
		Now:              func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
}

type io_Discard struct{}

func (io_Discard) Write(p []byte) (int, error) { return len(p), nil }

// TestReextractRefusesWithoutForce is the criterion's first clause: an
// explicit flag, never an implicit staleness side-effect. The assertion is on
// the DISPATCH COUNT, not on the error — a command that errors after
// dispatching has still re-extracted the corpus.
func TestReextractRefusesWithoutForce(t *testing.T) {
	root := newCorpus(t, 3)
	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.Force = false

	if _, err := Run(opts); err == nil {
		t.Fatal("expected a refusal without --force, got nil error")
	}
	if got := r.count(); got != 0 {
		t.Fatalf("refused run dispatched %d extraction(s); want 0", got)
	}
	if _, err := os.Stat(filepath.Join(root, LedgerRelPath)); err == nil {
		t.Fatal("a refused run wrote a ledger; it must leave no trace of a batch that did not happen")
	}
}

// TestReextractDispatchesWholeCorpusOnce covers the forced first run.
func TestReextractDispatchesWholeCorpusOnce(t *testing.T) {
	root := newCorpus(t, 5)
	r := &fakeRunner{}

	res, err := Run(baseOpts(root, r))
	if err != nil {
		t.Fatalf("forced run failed: %v", err)
	}
	if res.Dispatched != 5 || r.count() != 5 {
		t.Fatalf("dispatched %d (runner saw %d); want 5 of 5 eligible", res.Dispatched, r.count())
	}
	if res.Succeeded != 5 {
		t.Fatalf("succeeded %d; want 5", res.Succeeded)
	}

	var l Ledger
	readJSON(t, filepath.Join(root, LedgerRelPath), &l)
	if l.PromptVersion != "v4" {
		t.Fatalf("ledger prompt version %q; want v4 read from the installed agent", l.PromptVersion)
	}
	if len(l.Artifacts) != 5 {
		t.Fatalf("ledger recorded %d artifact(s); want 5", len(l.Artifacts))
	}
	for id, e := range l.Artifacts {
		if e.Status != StatusDone || e.SidecarSHA256 == "" {
			t.Fatalf("%s recorded %q with hash %q; want done with the produced sidecar's hash", id, e.Status, e.SidecarSHA256)
		}
	}
}

// TestReextractIsIdempotent is the "re-invoking on an already-regenerated
// artifact dispatches zero extractions" clause.
func TestReextractIsIdempotent(t *testing.T) {
	root := newCorpus(t, 4)
	first := &fakeRunner{}
	if _, err := Run(baseOpts(root, first)); err != nil {
		t.Fatalf("first run: %v", err)
	}

	second := &fakeRunner{}
	res, err := Run(baseOpts(root, second))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.count() != 0 || res.Dispatched != 0 {
		t.Fatalf("re-invocation dispatched %d (runner saw %d); want 0", res.Dispatched, second.count())
	}
	if res.AlreadyDone != 4 {
		t.Fatalf("already-done %d; want 4", res.AlreadyDone)
	}
}

// TestReextractLedgerIsWrittenPerCompletion is the assertion the resume test
// below CANNOT make.
//
// Found by mutation: moving the ledger write out of the completion callback and
// leaving only the end-of-run save left every resume test green. That is a gate
// measuring the wrong thing — a batch that persists at the end survives a
// clean return, which is precisely the case that never needed resuming. A real
// kill takes the process down with work outstanding and nothing written.
//
// So the probe reads the ledger FROM DISK from inside a later dispatch, at the
// moment a kill would land. If persistence is end-of-run only, the file is
// absent or empty there and this fails.
func TestReextractLedgerIsWrittenPerCompletion(t *testing.T) {
	root := newCorpus(t, 4)
	ledgerPath := filepath.Join(root, LedgerRelPath)

	var seenDuringRun int
	probe := &probingRunner{
		onDispatch: func(n int) {
			// On the third dispatch, two artifacts have already completed.
			if n == 3 {
				var l Ledger
				b, err := os.ReadFile(ledgerPath)
				if err != nil {
					return
				}
				if json.Unmarshal(b, &l) != nil {
					return
				}
				for _, e := range l.Artifacts {
					if e.Status == StatusDone {
						seenDuringRun++
					}
				}
			}
		},
	}
	opts := baseOpts(root, nil)
	opts.Runner = probe
	opts.Concurrency = 1
	if _, err := Run(opts); err != nil {
		t.Fatalf("run: %v", err)
	}
	if seenDuringRun < 2 {
		t.Fatalf("ledger held %d completed entry(ies) mid-run; want at least 2 — completions must be persisted as they happen, not at the end (a killed process never reaches the end)", seenDuringRun)
	}
}

// probingRunner writes sidecars like fakeRunner but calls back with the
// dispatch ordinal so a test can inspect on-disk state mid-batch.
type probingRunner struct {
	mu         sync.Mutex
	n          int
	onDispatch func(int)
}

func (p *probingRunner) Preflight() error { return nil }

func (p *probingRunner) Resync(_ context.Context, t phasea.Task) error {
	p.mu.Lock()
	p.n++
	n := p.n
	p.mu.Unlock()
	if p.onDispatch != nil {
		p.onDispatch(n)
	}
	return os.WriteFile(t.SidecarPath, []byte("schema_version: 2\nartifact: "+t.ArtifactID+"\n"), 0o644)
}

// TestReextractResumesAfterKill is the resumability clause, and it is the one
// that pays for the whole ledger: a kill mid-batch must cost only the
// unfinished work.
func TestReextractResumesAfterKill(t *testing.T) {
	root := newCorpus(t, 6)

	// Serial dispatch so "killed after 2" is deterministic rather than a race
	// against the concurrency cap.
	killed := &fakeRunner{killAfter: 2}
	opts := baseOpts(root, killed)
	opts.Concurrency = 1
	if _, err := Run(opts); err == nil {
		t.Fatal("expected the killed batch to report failure")
	}
	if killed.count() != 6 {
		t.Fatalf("killed batch attempted %d task(s); want all 6 attempted", killed.count())
	}

	resumed := &fakeRunner{}
	res, err := Run(baseOpts(root, resumed))
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if res.AlreadyDone != 2 {
		t.Fatalf("resume treated %d artifact(s) as done; want the 2 that completed before the kill", res.AlreadyDone)
	}
	if res.Dispatched != 4 || resumed.count() != 4 {
		t.Fatalf("resume dispatched %d (runner saw %d); want only the 4 remaining", res.Dispatched, resumed.count())
	}
}

// TestReextractRedispatchesEditedSidecar pins that "already regenerated" is a
// CHECKED claim: the ledger records the hash it wrote, so a sidecar changed
// after its dispatch is not counted as covered by this batch.
func TestReextractRedispatchesEditedSidecar(t *testing.T) {
	root := newCorpus(t, 3)
	if _, err := Run(baseOpts(root, &fakeRunner{})); err != nil {
		t.Fatalf("first run: %v", err)
	}
	edited := filepath.Join(root, "docs/architecture/decisions", adrID(2)+"-fixture.edikt.yaml")
	mustWrite(t, edited, "schema_version: 2\nartifact: ADR-002\nhand_edited: true\n")

	r := &fakeRunner{}
	res, err := Run(baseOpts(root, r))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Dispatched != 1 || r.count() != 1 || r.dispatched[0] != adrID(2) {
		t.Fatalf("dispatched %v; want only the edited %s", r.dispatched, adrID(2))
	}
}

// TestReextractNewPromptVersionStartsFreshBatch pins the batch identity: work
// done under a different extraction contract is not this batch's work.
func TestReextractNewPromptVersionStartsFreshBatch(t *testing.T) {
	root := newCorpus(t, 3)
	if _, err := Run(baseOpts(root, &fakeRunner{})); err != nil {
		t.Fatalf("first run: %v", err)
	}
	mustWrite(t, filepath.Join(root, ".claude/agents/sidecar-extractor.md"),
		"---\nname: sidecar-extractor\nmodel: claude-opus-5\nprompt_version: 5\n---\nbody\n")

	r := &fakeRunner{}
	res, err := Run(baseOpts(root, r))
	if err != nil {
		t.Fatalf("run under the new contract: %v", err)
	}
	if res.Dispatched != 3 || r.count() != 3 {
		t.Fatalf("dispatched %d under a new prompt version; want all 3 re-dispatched", res.Dispatched)
	}
	if res.PromptVersion != "v5" {
		t.Fatalf("batch reported prompt %q; want v5", res.PromptVersion)
	}
}

// TestReextractRefusesUnknownPromptVersion — a batch whose contract cannot be
// named cannot be resumed or attributed, so it must not start. UNKNOWN over a
// guess.
func TestReextractRefusesUnknownPromptVersion(t *testing.T) {
	root := newCorpus(t, 2)
	mustWrite(t, filepath.Join(root, ".claude/agents/sidecar-extractor.md"),
		"---\nname: sidecar-extractor\nmodel: claude-opus-5\n---\nno prompt version here\n")

	r := &fakeRunner{}
	_, err := Run(baseOpts(root, r))
	if err == nil {
		t.Fatal("expected a refusal when the extraction contract version is unreadable")
	}
	if !strings.Contains(err.Error(), "UNKNOWN") {
		t.Fatalf("error %q does not report UNKNOWN; an unnameable contract must be reported, not substituted", err)
	}
	if r.count() != 0 {
		t.Fatalf("dispatched %d with an unknown contract; want 0", r.count())
	}
}

// TestReextractCleanTreePrecondition covers the "one commit from a clean tree"
// clause's precondition half.
func TestReextractCleanTreePrecondition(t *testing.T) {
	root := newCorpus(t, 2)
	gitInit(t, root)

	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.RequireCleanTree = true

	// Dirty: nothing is committed yet, so every file is untracked.
	if _, err := Run(opts); err == nil {
		t.Fatal("expected a refusal on a dirty tree")
	}
	if r.count() != 0 {
		t.Fatalf("dispatched %d on a dirty tree; want 0", r.count())
	}

	gitCommitAll(t, root)
	clean := &fakeRunner{}
	opts.Runner = clean
	if _, err := Run(opts); err != nil {
		t.Fatalf("clean tree still refused: %v", err)
	}
	if clean.count() != 2 {
		t.Fatalf("clean-tree run dispatched %d; want 2", clean.count())
	}
}

// TestReextractTouchesOnlySidecars is the other half of the same clause: the
// batch's blast radius. Asserted against the FILESYSTEM (git status), not
// against the code's intent.
func TestReextractTouchesOnlySidecars(t *testing.T) {
	root := newCorpus(t, 4)
	gitInit(t, root)
	gitCommitAll(t, root)

	opts := baseOpts(root, &fakeRunner{})
	opts.RequireCleanTree = true
	if _, err := Run(opts); err != nil {
		t.Fatalf("run: %v", err)
	}

	dirty, err := gitDirty(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirty) == 0 {
		t.Fatal("the batch changed nothing — a re-extraction that writes no sidecar has not run")
	}
	for _, line := range dirty {
		fields := strings.Fields(line)
		path := fields[len(fields)-1]
		// The ledger is state, not corpus, and lives under .edikt/state/.
		if strings.HasPrefix(path, ".edikt/state/") {
			continue
		}
		if !strings.HasSuffix(path, ".edikt.yaml") {
			t.Fatalf("the batch touched a non-sidecar path %q; it must only write *.edikt.yaml", path)
		}
	}
}

// TestReextractOnlyFilter pins the per-artifact form used to retry one
// artifact without re-running the corpus.
func TestReextractOnlyFilter(t *testing.T) {
	root := newCorpus(t, 5)
	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.Only = []string{adrID(3)}

	res, err := Run(opts)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Dispatched != 1 || r.dispatched[0] != adrID(3) {
		t.Fatalf("dispatched %v; want only %s", r.dispatched, adrID(3))
	}
}

// TestReextractFailureIsResumableNotSticky — a failed artifact stays eligible.
// A ledger that recorded failures as terminal would turn one bad dispatch into
// a permanently unregenerated artifact reported as covered.
func TestReextractFailureIsResumableNotSticky(t *testing.T) {
	root := newCorpus(t, 3)
	bad := &fakeRunner{failIDs: map[string]bool{adrID(2): true}}
	if _, err := Run(baseOpts(root, bad)); err == nil {
		t.Fatal("expected the batch with a failing artifact to report failure")
	}

	// The ledger must SAY it failed, not merely behave as if it had. Found by
	// mutation: recording a failure as `done` left every behavioural
	// assertion green, because re-dispatch is hash-driven — while `--status`
	// reported a complete batch to whoever read it. A status field that can
	// lie about a failure is the absence-rendering-as-pass shape.
	var l Ledger
	readJSON(t, filepath.Join(root, LedgerRelPath), &l)
	if e := l.Artifacts[adrID(2)]; e.Status != StatusFailed || e.Err == "" {
		t.Fatalf("%s recorded status %q err %q; want %q with the failure text", adrID(2), e.Status, e.Err, StatusFailed)
	}
	st, err := Status(root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Failed != 1 || len(st.FailedIDs) != 1 || st.FailedIDs[0] != adrID(2) {
		t.Fatalf("status reported %d failure(s) %v; want exactly %s", st.Failed, st.FailedIDs, adrID(2))
	}

	retry := &fakeRunner{}
	res, err := Run(baseOpts(root, retry))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if res.Dispatched != 1 || retry.dispatched[0] != adrID(2) {
		t.Fatalf("retry dispatched %v; want only the failed %s", retry.dispatched, adrID(2))
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// TestReextractRefusesUnvalidatedContract pins 4-cheap: a prompt bump reaches
// the corpus through this verb and no other, so an unrecorded extraction
// contract is stopped here.
//
// The gate now checks the BINARY's embedded fixture-validation record against
// the resolved prompt version, not a file in the consumer's project — so this
// test forces an unvalidated version explicitly rather than relying on a
// scratch corpus lacking a file it was never expected to have.
//
// Asserted on the DISPATCH COUNT, not the error: a command that refuses after
// dispatching has already re-extracted the corpus.
func TestReextractRefusesUnvalidatedContract(t *testing.T) {
	root := newCorpus(t, 3)
	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.SkipFixtureProof = false
	opts.PromptVersion = "v999999" // not in the embedded validated list

	if _, err := Run(opts); err == nil {
		t.Fatal("dispatched an unvalidated extraction contract — the embedded fixture-validation record should have refused it")
	}
	if got := r.count(); got != 0 {
		t.Fatalf("refused run dispatched %d extraction(s); want 0", got)
	}
}

// TestReextractAcceptsValidatedContract is the isolation half: with the
// resolved contract present in the binary's embedded record, dispatch
// proceeds — no project-side file needed, since this is a release property
// now, not a project one. Without this the refusal above could pass by
// refusing everything. newCorpus's scaffolded sidecar-extractor.md declares
// prompt_version: 4, which the embedded fixtureproof/extractor-validation.yaml
// records as validated.
func TestReextractAcceptsValidatedContract(t *testing.T) {
	root := newCorpus(t, 2)
	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.SkipFixtureProof = false

	if _, err := Run(opts); err != nil {
		t.Fatalf("a validated contract (v4, embedded) was refused: %v", err)
	}
	if r.count() != 2 {
		t.Fatalf("dispatched %d; want 2 with the contract validated", r.count())
	}
}

// TestReextractRefusesEvenInConsumerProjectShape proves the fix this test
// file exists to lock in: unlike the original design (a project-relative
// read of test/fixtures/extractor-validation.yaml), the gate's pass/fail no
// longer depends on the calling project having an edikt-dev-shaped test/
// directory. A scratch corpus with NO test/ directory at all — the real
// shape of every consumer project — still passes for a validated contract
// and still refuses for an unvalidated one, because the check is against the
// binary's embedded record, not the project's filesystem.
func TestReextractRefusesEvenInConsumerProjectShape(t *testing.T) {
	root := newCorpus(t, 1)
	if _, err := os.Stat(filepath.Join(root, "test")); !os.IsNotExist(err) {
		t.Fatalf("test fixture setup changed — newCorpus now creates a test/ directory, "+
			"which defeats the point of this test (stat err: %v)", err)
	}
	r := &fakeRunner{}
	opts := baseOpts(root, r)
	opts.SkipFixtureProof = false
	opts.PromptVersion = "v999999"

	if _, err := Run(opts); err == nil {
		t.Fatal("dispatched with an unvalidated contract in a project with no test/ directory at all — " +
			"this is exactly the shape that shipped broken to every real consumer")
	}
}

// validSidecarRunner writes a genuinely sidecar.Load-able regenerated
// sidecar, unlike fakeRunner's placeholder shorthand. Needed only by
// TestReextractSurfacesLoadFailureThroughTheFullPipeline, where a control
// artifact's real `before` sends PreservePinned into the actual
// sidecar.Load(afterPath) path — fakeRunner's own output would fail there
// for reasons unrelated to what this test is checking.
type validSidecarRunner struct{}

func (validSidecarRunner) Preflight() error { return nil }

func (validSidecarRunner) Resync(_ context.Context, t phasea.Task) error {
	sc := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          t.ParentPath,
		Directives:    []sidecar.Directive{anchored("A regenerated rule. (ref: " + t.ArtifactID + ")")},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		return err
	}
	return os.WriteFile(t.SidecarPath, body, 0o644)
}

// TestReextractSurfacesLoadFailureThroughTheFullPipeline — F-115/A1, wired
// end to end through Run(), not just unit-tested at PreservePinned. A
// corpus's own discoverEligible/Discover loop is what actually produces the
// nil-Sidecar-vs-LoadErr split reextract.go's pinnedBefore/loadFailedBefore
// maps depend on; a unit test on PreservePinned alone would not have caught
// the original bug, since the bug was in reextract.go never CONSULTING
// LoadErr at the wiring layer, not in PreservePinned's own logic.
func TestReextractSurfacesLoadFailureThroughTheFullPipeline(t *testing.T) {
	root := newCorpus(t, 2)

	// newCorpus's own default sidecar shorthand ("artifact: ID\nregenerated:
	// false\n") does NOT round-trip through sidecar.Load itself — those two
	// keys aren't real schema fields, so KnownFields(true) rejects them.
	// Every other test using newCorpus never notices, because none of them
	// call sidecar.Load on these fixtures directly. This test does (via
	// discoverEligible, inside Run()), so the control artifact needs a
	// GENUINELY loadable sidecar, not the shorthand placeholder — otherwise
	// "artifact 2 is a valid-sidecar control" would be false on its face.
	id1, id2 := adrID(1), adrID(2)
	controlPath := filepath.Join(root, "docs/architecture/decisions", id2+"-fixture.edikt.yaml")
	writeSidecar(t, controlPath, &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("A rule. (ref: " + id2 + ")")},
	})

	// Corrupt artifact 1's PRIOR sidecar so sidecar.Load fails on it —
	// deliberately unparseable YAML, not merely "missing" (missing is the
	// ordinary, already-correct bootstrap path; this test is about the OTHER
	// case). A load failure on one artifact must not contaminate reporting
	// for another — that's what the now-genuinely-valid artifact 2 controls
	// for.
	corruptPath := filepath.Join(root, "docs/architecture/decisions", id1+"-fixture.edikt.yaml")
	mustWrite(t, corruptPath, "schema_version: 2\nartifact: ["+id1+"\n  not: [valid yaml at all\n")

	// fakeRunner's own written content uses the same placeholder shorthand as
	// newCorpus's defaults — fine for artifact 1 (beforeLoadErr short-circuits
	// PreservePinned before it ever reaches sidecar.Load(afterPath)), but
	// artifact 2's genuinely-valid `before` sends it into the real
	// restore-path logic, which DOES load the regenerated file — so the
	// regenerated content needs to be genuinely loadable too, or this test
	// would be failing on an unrelated fixture-format mismatch, not proving
	// anything about the load-failure wiring under test.
	// Not baseOpts(root, r): it's typed for *fakeRunner specifically. Built
	// directly here, same fields, for a Runner that isn't *fakeRunner.
	opts := Options{
		ProjectRoot:      root,
		Force:            true,
		Concurrency:      2,
		Runner:           &validSidecarRunner{},
		Stderr:           io_Discard{},
		SkipFixtureProof: true,
		Now:              func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
	res, err := Run(opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The control: artifact 2 must be untouched by artifact 1's load failure.
	for _, id := range res.LoadFailedIDs {
		if id == id2 {
			t.Fatalf("artifact %s (valid prior sidecar) was reported load-failed — contamination across artifacts: %v", id2, res.LoadFailedIDs)
		}
	}

	found := false
	for _, id := range res.LoadFailedIDs {
		if id == id1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("artifact %s had an unparseable prior sidecar and Run() did not report it via LoadFailedIDs — "+
			"the exact swallow this test exists to catch: %+v", id1, res.LoadFailedIDs)
	}

	// The symptom is per-field; the mechanism under test is that AT LEAST
	// the fields F-115 named are represented in the result for this
	// artifact, proving the wiring (not just PreservePinned in isolation)
	// carries the load error through to the report.
	seen := map[string]bool{}
	for _, u := range res.Unrestorable {
		if u.ArtifactID == id1 {
			seen[u.Field] = true
		}
	}
	for _, want := range []string{"paths", "verify", "verify_kind", "human_approved_at"} {
		if !seen[want] {
			t.Errorf("Run()'s Unrestorable report for %s is missing field %q", id1, want)
		}
	}
}
