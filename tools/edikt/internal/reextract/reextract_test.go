package reextract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// extractAllExcept stages the extractor's output for every plan item but one,
// standing in for a fan-out where a single artifact's subagent failed.
func extractAllExcept(t *testing.T, root, planID, skipID string) {
	t.Helper()
	var plan phasea.DispatchPlan
	raw, rerr := os.ReadFile(phasea.PlanFilePath(root, planID))
	if rerr != nil {
		t.Fatalf("read plan: %v", rerr)
	}
	if jerr := json.Unmarshal(raw, &plan); jerr != nil {
		t.Fatalf("parse plan: %v", jerr)
	}
	for _, it := range plan.Items {
		if it.ItemID == skipID {
			continue
		}
		p := filepath.Join(root, it.ExpectedOutput.Path)
		body := regeneratedSidecar(it.ArtifactPath, it.ItemID)
		if werr := os.WriteFile(p, []byte(body), 0o644); werr != nil {
			t.Fatalf("stage extraction for %s: %v", it.ItemID, werr)
		}
	}
}

// regeneratedSidecar is what a successful extraction would have written: a
// loadable, schema-valid sidecar whose anchor quotes the parent byte-exactly.
//
// All three properties are load-bearing — phasea.Restore's content gate loads
// it, validates it against its own declared schema, and resolves its anchors
// against the real parent. A placeholder that satisfied only the first would
// be quarantined, and the test would report a failed extraction that never
// actually failed.
//
// Line 8 is "MUST do the thing." in newCorpus's parent bodies.
func regeneratedSidecar(artifactPath, itemID string) string {
	return fmt.Sprintf("schema_version: 2\n"+
		"topic: testing\n"+
		"path: %s\n"+
		"signals:\n  - testing\n"+
		"directives:\n"+
		"  - text: \"A regenerated rule MUST hold. (ref: %s)\"\n"+
		"    source_excerpts:\n"+
		"      - line_start: 8\n"+
		"        line_end: 8\n"+
		"        quote: \"MUST do the thing.\"\n", artifactPath, itemID)
}

// plannedCount reports how many artifacts a run handed off to tier-1.
//
// These tests asserted on a fakeRunner's dispatch tally. There is no dispatch
// from tier-2 to tally now (SPEC-013) — Run() emits a plan and returns — so
// the same question is answered by the plan's item count. It is the stronger
// observable: the plan is what tier-1 actually fans out from, whereas the
// stub only ever existed inside the test process.
func plannedCount(res *Result) int {
	if res == nil {
		return 0
	}
	return res.Dispatched
}

// extractInto writes the sidecar the extractor would have produced for every
// item in a plan, standing in for tier-1's fan-out so a test can go on to
// exercise the recording pass.
func extractInto(t *testing.T, root, planID string, limit int) {
	t.Helper()
	var plan phasea.DispatchPlan
	raw, rerr := os.ReadFile(phasea.PlanFilePath(root, planID))
	if rerr != nil {
		t.Fatalf("read plan: %v", rerr)
	}
	if jerr := json.Unmarshal(raw, &plan); jerr != nil {
		t.Fatalf("parse plan: %v", jerr)
	}
	for n, it := range plan.Items {
		if limit >= 0 && n >= limit {
			break // stand in for a fan-out that was killed partway
		}
		p := filepath.Join(root, it.ExpectedOutput.Path)
		body := regeneratedSidecar(it.ArtifactPath, it.ItemID)
		if werr := os.WriteFile(p, []byte(body), 0o644); werr != nil {
			t.Fatalf("stage extraction for %s: %v", it.ItemID, werr)
		}
	}
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
		// A LOADABLE but STALE sidecar. It has to be both: phasea.Plan
		// refuses the whole run if any artifact in the corpus fails to load
		// (a corpus it cannot read is one it cannot plan against), and it
		// only plans artifacts whose anchors have drifted. The quote below
		// deliberately does not appear in the parent body above, which is
		// what makes each fixture stale and therefore planned.
		mustWrite(t, base+".edikt.yaml",
			"schema_version: 2\n"+
				"topic: testing\n"+
				"path: docs/architecture/decisions/"+id+"-fixture.md\n"+
				"signals:\n  - testing\n"+
				"directives:\n"+
				"  - text: \"A stale rule MUST hold. (ref: "+id+")\"\n"+
				"    source_excerpts:\n"+
				"      - line_start: 7\n"+
				"        line_end: 7\n"+
				"        quote: \"text that is not in the parent\"\n")
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

func baseOpts(root string) Options {
	return Options{
		ProjectRoot: root,
		Force:       true,
		Concurrency: 2,
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
	opts := baseOpts(root)
	opts.Force = false

	res, err := Run(opts)
	if err == nil {
		t.Fatal("expected a refusal without --force, got nil error")
	}
	if got := plannedCount(res); got != 0 {
		t.Fatalf("refused run planned %d extraction(s); want 0", got)
	}
	if _, err := os.Stat(filepath.Join(root, LedgerRelPath)); err == nil {
		t.Fatal("a refused run wrote a ledger; it must leave no trace of a batch that did not happen")
	}
}

// TestReextractDispatchesWholeCorpusOnce covers the forced first run.
func TestReextractDispatchesWholeCorpusOnce(t *testing.T) {
	root := newCorpus(t, 5)

	res, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("forced run failed: %v", err)
	}
	if res.Dispatched != 5 || plannedCount(res) != 5 {
		t.Fatalf("planned %d; want 5 of 5 eligible", res.Dispatched)
	}
	// Succeeded is deliberately zero on a plan-and-hand-off run: this process
	// does not perform the extraction, and reporting a success count for work
	// it did not do is the unmeasured-as-measured shape. The completions are
	// recorded by the pass that follows tier-1's fan-out.
	if res.Succeeded != 0 {
		t.Fatalf("succeeded %d before any extraction ran; want 0", res.Succeeded)
	}
	extractInto(t, root, res.PlanID, -1)
	rec, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: res.PlanID})
	if rerr != nil {
		t.Fatalf("record: %v", rerr)
	}
	if rec.Recorded != 5 {
		t.Fatalf("recorded %d; want 5", rec.Recorded)
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
	first, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Stand in for tier-1's fan-out and the recording pass that follows it.
	// Run() only plans now, so without these the corpus is still stale and
	// the "second run" would legitimately replan — the test would pass for
	// the wrong reason on an implementation that never regenerated anything.
	extractInto(t, root, first.PlanID, -1)
	if _, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: first.PlanID}); rerr != nil {
		t.Fatalf("record: %v", rerr)
	}

	res, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Dispatched != 0 {
		t.Fatalf("re-invocation planned %d extraction(s); want 0", res.Dispatched)
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

	plan, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	extractInto(t, root, plan.PlanID, -1)

	var seenDuringRun int
	if _, rerr := Record(RecordOptions{
		ProjectRoot: root,
		PlanID:      plan.PlanID,
		OnRecorded: func(_ string, n int) {
			// On the third recording, two artifacts have already completed.
			// Read the ledger FROM DISK, which is where a kill would leave it.
			if n != 3 {
				return
			}
			var l Ledger
			b, rerr := os.ReadFile(ledgerPath)
			if rerr != nil || json.Unmarshal(b, &l) != nil {
				return
			}
			for _, e := range l.Artifacts {
				if e.Status == StatusDone {
					seenDuringRun++
				}
			}
		},
	}); rerr != nil {
		t.Fatalf("record: %v", rerr)
	}

	// Guard the denominator: a plan smaller than 3 items never reaches the
	// probe, and a zero here would read as "batched" when it means "never
	// looked".
	if plan.Dispatched < 3 {
		t.Fatalf("plan had %d item(s); the mid-run probe needs at least 3", plan.Dispatched)
	}
	if seenDuringRun < 2 {
		t.Fatalf("ledger held %d completed entry(ies) mid-run; want at least 2 — completions must be persisted as they happen, not at the end (a killed process never reaches the end)", seenDuringRun)
	}
}


// TestReextractResumesAfterKill is the resumability clause, and it is the one
// that pays for the whole ledger: a kill mid-batch must cost only the
// unfinished work.
func TestReextractResumesAfterKill(t *testing.T) {
	root := newCorpus(t, 6)

	// The kill is now modelled where it can actually happen: tier-1's fan-out
	// finished only 2 of the 6 artifacts before the process died, so only
	// those 2 have a sidecar for the recording pass to find. The other 4 are
	// recorded as failed — which is the state a resume must read as "still
	// owed", not as "never attempted".
	plan, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if plan.Dispatched != 6 {
		t.Fatalf("first run planned %d; want all 6", plan.Dispatched)
	}
	extractInto(t, root, plan.PlanID, 2)
	rec, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: plan.PlanID})
	if rerr == nil {
		t.Fatal("expected the partial batch to report failure for the 4 unextracted artifacts")
	}
	if rec.Recorded != 2 {
		t.Fatalf("recorded %d completed artifact(s); want the 2 the fan-out finished", rec.Recorded)
	}

	res, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if res.AlreadyDone != 2 {
		t.Fatalf("resume treated %d artifact(s) as done; want the 2 that completed before the kill", res.AlreadyDone)
	}
	if res.Dispatched != 4 {
		t.Fatalf("resume planned %d; want only the 4 remaining", res.Dispatched)
	}
}

// TestReextractRedispatchesEditedSidecar pins that "already regenerated" is a
// CHECKED claim: the ledger records the hash it wrote, so a sidecar changed
// after its dispatch is not counted as covered by this batch.
func TestReextractRedispatchesEditedSidecar(t *testing.T) {
	root := newCorpus(t, 3)
	first, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Complete the batch: fan-out plus the recording pass. Only then is
	// "already regenerated" a claim the ledger can be wrong about, which is
	// what this test is here to check.
	extractInto(t, root, first.PlanID, -1)
	if _, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: first.PlanID}); rerr != nil {
		t.Fatalf("record: %v", rerr)
	}
	// ...and the verify that closes it, as /edikt:gov:reextract runs it. A
	// batch left restored-but-unverified is an OPEN plan, and a later plan
	// over a moved sidecar is refused until it is closed (#11, c1-d) — this
	// test is about the ledger's hash check, not about that guard.
	if v, verr := phasea.Verify(phasea.VerifyOptions{ProjectRoot: root, PlanID: first.PlanID}); v == nil || v.ExitCode != phasea.ExitVerifyOK {
		t.Fatalf("verify: %v", verr)
	}
	// Verify stamps extractor_model/extractor_contract onto each ok sidecar
	// after Record hashed it; re-stamp the ledger (`gov reextract
	// --restamp-done`) so the only undeclared change left is the hand edit.
	done, derr := DoneSidecarPaths(root)
	if derr != nil {
		t.Fatal(derr)
	}
	if _, serr := RestampLedger(root, done); serr != nil {
		t.Fatal(serr)
	}
	edited := filepath.Join(root, "docs/architecture/decisions", adrID(2)+"-fixture.edikt.yaml")
	// Hand-edited: loadable (phasea.Plan refuses a corpus it cannot read, so
	// an unparseable edit would fail on the corpus gate rather than the hash
	// check this test is about) but anchor-stale, which is what puts it back
	// in the work-list while its two siblings stay out.
	mustWrite(t, edited,
		"schema_version: 2\n"+
			"topic: testing\n"+
			"path: docs/architecture/decisions/"+adrID(2)+"-fixture.md\n"+
			"signals:\n  - testing\n"+
			"directives:\n"+
			"  - text: \"A hand-edited rule MUST hold. (ref: "+adrID(2)+")\"\n"+
			"    source_excerpts:\n"+
			"      - line_start: 8\n"+
			"        line_end: 8\n"+
			"        quote: \"this quote is not in the parent either\"\n")

	res, serr := Run(baseOpts(root))
	if serr != nil {
		t.Fatalf("second run: %v", serr)
	}
	if res.Dispatched != 1 || plannedCount(res) != 1 {
		t.Fatalf("planned %d; want only the edited %s", res.Dispatched, adrID(2))
	}
}

// TestReextractNewPromptVersionStartsFreshBatch pins the batch identity: work
// done under a different extraction contract is not this batch's work.
func TestReextractNewPromptVersionStartsFreshBatch(t *testing.T) {
	root := newCorpus(t, 3)
	if _, err := Run(baseOpts(root)); err != nil {
		t.Fatalf("first run: %v", err)
	}
	mustWrite(t, filepath.Join(root, ".claude/agents/sidecar-extractor.md"),
		"---\nname: sidecar-extractor\nmodel: claude-opus-5\nprompt_version: 5\n---\nbody\n")

	res, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("run under the new contract: %v", err)
	}
	if res.Dispatched != 3 || plannedCount(res) != 3 {
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

	res, err := Run(baseOpts(root))
	if err == nil {
		t.Fatal("expected a refusal when the extraction contract version is unreadable")
	}
	if !strings.Contains(err.Error(), "UNKNOWN") {
		t.Fatalf("error %q does not report UNKNOWN; an unnameable contract must be reported, not substituted", err)
	}
	if plannedCount(res) != 0 {
		t.Fatalf("planned %d with an unknown contract; want 0", plannedCount(res))
	}
}

// TestReextractCleanTreePrecondition covers the "one commit from a clean tree"
// clause's precondition half.
func TestReextractCleanTreePrecondition(t *testing.T) {
	root := newCorpus(t, 2)
	gitInit(t, root)

	opts := baseOpts(root)
	opts.RequireCleanTree = true

	// Dirty: nothing is committed yet, so every file is untracked.
	res, derr := Run(opts)
	if derr == nil {
		t.Fatal("expected a refusal on a dirty tree")
	}
	if plannedCount(res) != 0 {
		t.Fatalf("planned %d on a dirty tree; want 0", plannedCount(res))
	}

	gitCommitAll(t, root)
	cleanRes, cerr := Run(opts)
	if cerr != nil {
		t.Fatalf("clean tree still refused: %v", cerr)
	}
	if plannedCount(cleanRes) != 2 {
		t.Fatalf("clean-tree run planned %d; want 2", plannedCount(cleanRes))
	}
}

// TestReextractTouchesOnlySidecars is the other half of the same clause: the
// batch's blast radius. Asserted against the FILESYSTEM (git status), not
// against the code's intent.
func TestReextractTouchesOnlySidecars(t *testing.T) {
	root := newCorpus(t, 4)
	gitInit(t, root)
	gitCommitAll(t, root)

	opts := baseOpts(root)
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
	opts := baseOpts(root)
	opts.Only = []string{adrID(3)}

	res, err := Run(opts)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Dispatched != 1 {
		t.Fatalf("planned %d; want only %s", res.Dispatched, adrID(3))
	}
}

// TestReextractFailureIsResumableNotSticky — a failed artifact stays eligible.
// A ledger that recorded failures as terminal would turn one bad dispatch into
// a permanently unregenerated artifact reported as covered.
func TestReextractFailureIsResumableNotSticky(t *testing.T) {
	root := newCorpus(t, 3)

	// The failure is now modelled where extraction actually happens: tier-1's
	// fan-out produced sidecars for two artifacts and not for ADR-002, so the
	// recording pass finds nothing to restore for it.
	plan, perr := Run(baseOpts(root))
	if perr != nil {
		t.Fatalf("plan: %v", perr)
	}
	extractAllExcept(t, root, plan.PlanID, adrID(2))
	if _, rerr := Record(RecordOptions{ProjectRoot: root, PlanID: plan.PlanID}); rerr == nil {
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

	res, err := Run(baseOpts(root))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if res.Dispatched != 1 {
		t.Fatalf("retry planned %d; want only the failed %s", res.Dispatched, adrID(2))
	}
}

// TestStatus_UnresolvableAgentDegradesRatherThanFails — F4,
// docs/internal/issues/agentmodel-resolver-no-global-fallback.md.
//
// Before this fix, Status() hard-failed the whole report ("cannot determine
// the extraction contract version") whenever the sidecar-extractor agent
// wasn't project-local — even when eligible/done/remaining counts are
// perfectly well-defined independent of it. That hard failure is what made
// /edikt:upgrade §7's own contract ("if the status check fails ... say
// nothing further") turn a resolvable-elsewhere agent into total silence.
// Status() must now degrade PromptVersion to UNKNOWN and still report.
func TestStatus_UnresolvableAgentDegradesRatherThanFails(t *testing.T) {
	root := newCorpus(t, 3)
	// Remove the project-local agent newCorpus installs — simulating a
	// project whose agents resolve globally instead.
	if err := os.Remove(filepath.Join(root, phasea.ExtractorAgentRelPath)); err != nil {
		t.Fatal(err)
	}
	// Isolate from whatever the test machine's real Claude profile has
	// installed — this test's premise is "nothing resolves anywhere".
	t.Setenv("CLAUDE_HOME", t.TempDir())

	st, err := Status(root)
	if err != nil {
		t.Fatalf("expected Status() to degrade rather than fail, got error: %v", err)
	}
	if st.PromptVersion != phasea.ExtractorPromptVersionUnknown {
		t.Fatalf("got PromptVersion %q, want %q", st.PromptVersion, phasea.ExtractorPromptVersionUnknown)
	}
	if st.Eligible != 3 {
		t.Fatalf("got Eligible %d, want 3 — counts must still be reported despite the unresolvable agent", st.Eligible)
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
	opts := baseOpts(root)
	opts.SkipFixtureProof = false
	opts.PromptVersion = "v999999" // not in the embedded validated list

	res, err := Run(opts)
	if err == nil {
		t.Fatal("dispatched an unvalidated extraction contract — the embedded fixture-validation record should have refused it")
	}
	if got := plannedCount(res); got != 0 {
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
	opts := baseOpts(root)
	opts.SkipFixtureProof = false

	res, err := Run(opts)
	if err != nil {
		t.Fatalf("a validated contract (v4, embedded) was refused: %v", err)
	}
	if plannedCount(res) != 2 {
		t.Fatalf("planned %d; want 2 with the contract validated", plannedCount(res))
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
	opts := baseOpts(root)
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
	// The control has to be genuinely schema-valid, not merely parseable.
	// It was built via writeSidecar with only Directives set, which omits
	// topic/path/signals — under the old contract nothing looked, so the
	// "valid control" was never actually valid. phasea.Plan validates every
	// artifact, so a half-built control would show up as a second corpus
	// error and the anti-contamination assertion would fail against itself.
	controlPath := filepath.Join(root, "docs/architecture/decisions", id2+"-fixture.edikt.yaml")
	mustWrite(t, controlPath, regeneratedSidecar("docs/architecture/decisions/"+id2+"-fixture.md", id2))

	// Corrupt artifact 1's PRIOR sidecar so sidecar.Load fails on it —
	// deliberately unparseable YAML, not merely "missing" (missing is the
	// ordinary, already-correct bootstrap path; this test is about the OTHER
	// case). A load failure on one artifact must not contaminate reporting
	// for another — that's what the now-genuinely-valid artifact 2 controls
	// for.
	corruptPath := filepath.Join(root, "docs/architecture/decisions", id1+"-fixture.edikt.yaml")
	mustWrite(t, corruptPath, "schema_version: 2\nartifact: ["+id1+"\n  not: [valid yaml at all\n")

	// The contract tightened here, and the subject survives the change.
	//
	// Run() used to report load failures through res.LoadFailedIDs and carry
	// on. Planning now goes through phasea.Plan, which REFUSES a corpus it
	// cannot fully read — "every artifact must load and validate before any
	// plan is written". That is strictly stronger than reporting: a corpus
	// with an unreadable sidecar cannot be planned against honestly, because
	// the plan would silently omit whatever it failed to read.
	//
	// So the assertion moves from "reported and continued" to "refused and
	// named", and the anti-contamination half is preserved by requiring the
	// refusal to name the corrupt artifact and NOT the valid control.
	_, err := Run(baseOpts(root))
	if err == nil {
		t.Fatal("a corpus with an unparseable sidecar was planned anyway — the exact swallow this test exists to catch")
	}
	msg := err.Error()
	if !strings.Contains(msg, id1) {
		t.Fatalf("artifact %s had an unparseable prior sidecar and the refusal does not name it: %v", id1, err)
	}
	if strings.Contains(msg, id2+"-fixture.edikt.yaml") {
		t.Fatalf("artifact %s (valid prior sidecar) was named in the refusal — contamination across artifacts: %v", id2, err)
	}
}
