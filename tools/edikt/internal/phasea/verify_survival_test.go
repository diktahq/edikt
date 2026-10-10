package phasea

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// verify_survival_test.go — the body-loss guard on every re-extraction of an
// existing sidecar (#11 comment item 3, c6-b).
//
// Before it, only a migration-lifted item (stale-migration-preserved) had its
// output compared with what it replaced. A stale-quote-drift or `requested`
// re-extraction — `gov reextract`, a per-artifact compile — could drop a
// directive whose prose is still in the artifact, and verify consumed the
// plan: the rule left the corpus and nothing said so.
//
// Identity is by ANCHOR: a prior body survives when some output anchor quotes
// the same passage (equal, containing, or contained) as one of its anchors that
// is still in the parent. A body whose anchors drifted falls back to the
// lexical comparator, reported as the weaker instrument. A body with no anchor
// left in the parent and no lexical match is `source-removed`: named, not
// failed. Everything else is lost.

// Three rules, each on its own line of the parent, so a body can be dropped
// while its prose stays.
const (
	survLineJSON   = "Every hook MUST emit JSON on stdout."
	survLineAnchor = "The extractor MUST NEVER invent an anchor."
	survLineEdit   = "Sidecars MUST NOT be hand-edited."

	survTextJSON   = "Hooks MUST emit JSON on stdout. (ref: ADR-900)"
	survTextAnchor = "The extractor MUST NOT invent an anchor; every anchor quotes the parent. (ref: ADR-900)"
	survTextEdit   = "Sidecars MUST NOT be edited by hand. (ref: ADR-900)"
	survTextGone   = "Configuration MUST be written as TOML files. (ref: ADR-900)"
)

func survivalParent() string {
	return "# Title\n\n**Status:** Accepted\n\n" +
		survLineJSON + "\n\n" + // line 5
		survLineAnchor + "\n\n" + // line 7
		survLineEdit + "\n" // line 9
}

func survAnchor(line int, quote string) []sidecar.SourceExcerpt {
	return []sidecar.SourceExcerpt{{LineStart: line, LineEnd: line, Quote: quote}}
}

func survDirective(text string, line int, quote string) sidecar.Directive {
	return sidecar.Directive{Text: text, SourceExcerpts: survAnchor(line, quote)}
}

func survProhibition(text string, line int, quote string) sidecar.Prohibition {
	return sidecar.Prohibition{Text: text, SourceExcerpts: survAnchor(line, quote)}
}

// A prior body whose anchor still quotes the parent at its lines.
func priorJSON() sidecar.Directive   { return survDirective(survTextJSON, 5, survLineJSON) }
func priorAnchor() sidecar.Directive { return survDirective(survTextAnchor, 7, survLineAnchor) }

// A prior body whose anchor quotes prose the parent no longer carries — what
// makes the prior stale, and what a source-removed body looks like.
func priorGone() sidecar.Directive { return survDirective(survTextGone, 9, staleQuote) }

// newSurvivalFixture plans one ADR whose prior sidecar is `prior`. reason
// picks the planning path: ReasonStaleQuoteDrift plans the corpus (the prior
// must carry a drifted anchor), ReasonRequested plans the artifact by path
// (SR-011, what `gov reextract` and a per-artifact compile do).
func newSurvivalFixture(t *testing.T, reason string, prior *sidecar.Sidecar) *verifyFixture {
	t.Helper()
	return newSurvivalFixtureWithParent(t, reason, survivalParent(), prior)
}

func newSurvivalFixtureWithParent(t *testing.T, reason, parent string, prior *sidecar.Sidecar) *verifyFixture {
	t.Helper()
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	dir := filepath.Join(root, filepath.FromSlash("docs/architecture/decisions"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(dir, "ADR-900-test.md")
	if err := os.WriteFile(md, []byte(parent), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := filepath.Join(dir, "ADR-900-test.edikt.yaml")
	prior.SchemaVersion = 2
	prior.Topic = "testing"
	prior.Path = "docs/architecture/decisions/ADR-900-test.md"
	writeMarshalledSidecar(t, sc, prior)

	opts := PlanOptions{ProjectRoot: root}
	if reason == ReasonRequested {
		opts.Artifact = "docs/architecture/decisions/ADR-900-test.md"
	}
	out, err := Plan(opts)
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 1 {
		t.Fatalf("fixture did not produce a one-item plan: exit %d, %s", out.ExitCode, out.Message)
	}
	if got := out.Plan.Items[0].Reason; got != reason {
		t.Fatalf("fixture item reason is %q, want %q", got, reason)
	}
	if _, err := os.Stat(PinSnapshotPath(root, out.Plan.PlanID, out.Plan.Items[0].ItemID)); err != nil {
		t.Fatalf("fixture did not capture a pin snapshot: %v", err)
	}
	if err := writeRestoreReceipt(root, out.Plan.PlanID, &RestoreOutcome{ExitCode: ExitRestoreOK}); err != nil {
		t.Fatalf("planting the restore receipt: %v", err)
	}
	return &verifyFixture{root: root, planID: out.Plan.PlanID, plan: out.Plan, mdPath: md, scPath: sc, itemID: out.Plan.Items[0].ItemID}
}

// extract stands in for the re-extraction: what the extractor wrote.
func (f *verifyFixture) extract(t *testing.T, ds []sidecar.Directive, ps []sidecar.Prohibition) {
	t.Helper()
	writeMarshalledSidecar(t, f.scPath, &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/ADR-900-test.md",
		Directives: ds, Prohibitions: ps,
	})
}

func verifyItem(t *testing.T, f *verifyFixture, onLoss string) (ReceiptItem, *VerifyOutcome) {
	t.Helper()
	out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID, OnLoss: onLoss})
	if out == nil || out.Receipt == nil || len(out.Receipt.Items) != 1 {
		t.Fatalf("verify produced no one-item receipt: %+v", out)
	}
	return out.Receipt.Items[0], out
}

// assertLoss holds a lost body to the whole contract: lossless-loss, retryable,
// exit 1, and a diagnostic and correction_prompt that both name the body and
// the parent range its anchor records.
func assertLoss(t *testing.T, it ReceiptItem, out *VerifyOutcome, wantNamed ...string) {
	t.Helper()
	if it.Status != StatusLosslessLoss {
		t.Fatalf("status %q, want %q — a body whose prose is still in the parent left the corpus: %s",
			it.Status, StatusLosslessLoss, it.Diagnostic)
	}
	if IsTerminalStatus(it.Status) || out.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("a lost body must stay retryable (exit %d), got exit %d terminal=%v",
			ExitVerifyIncomplete, out.ExitCode, IsTerminalStatus(it.Status))
	}
	if out.Receipt.Consumed {
		t.Fatal("the plan was consumed with a body lost")
	}
	if it.CorrectionPrompt == "" {
		t.Fatal("a lost body carries no correction_prompt — the INCOMPLETE line reports it instead of re-dispatching it")
	}
	for _, want := range wantNamed {
		if !strings.Contains(it.Diagnostic, want) {
			t.Errorf("diagnostic omits %q:\n%s", want, it.Diagnostic)
		}
		if !strings.Contains(it.CorrectionPrompt, want) {
			t.Errorf("correction_prompt omits %q:\n%s", want, it.CorrectionPrompt)
		}
	}
}

// ── AC-5.1: a dropped body whose prose remains is lossless-loss ───────────

func TestVerify_StaleQuoteDriftDroppedBodyProseRemainsIsLosslessLoss(t *testing.T) {
	f := newSurvivalFixture(t, ReasonStaleQuoteDrift, &sidecar.Sidecar{
		Directives: []sidecar.Directive{priorJSON(), priorAnchor(), priorGone()},
	})
	f.extract(t, []sidecar.Directive{priorJSON()}, nil)
	it, out := verifyItem(t, f, "")
	assertLoss(t, it, out, "The extractor MUST NOT invent an anchor", "lines 7-7")
	// The drifted body is not the loss: its prose is gone.
	if strings.Contains(it.CorrectionPrompt, "TOML") {
		t.Errorf("the correction_prompt asks to restore a body whose source prose is gone:\n%s", it.CorrectionPrompt)
	}
}

// The reproduced case: a contract-change re-extraction of a FRESH sidecar
// (nothing stale) drops a rule, and verify consumed the plan.
func TestVerify_RequestedDroppedBodyProseRemainsIsLosslessLoss(t *testing.T) {
	f := newSurvivalFixture(t, ReasonRequested, &sidecar.Sidecar{
		Directives: []sidecar.Directive{priorJSON(), priorAnchor()},
	})
	f.extract(t, []sidecar.Directive{priorJSON()}, nil)
	it, out := verifyItem(t, f, "")
	assertLoss(t, it, out, "The extractor MUST NOT invent an anchor", "lines 7-7")
}

func TestVerify_ProhibitionDroppedIsLosslessLoss(t *testing.T) {
	f := newSurvivalFixture(t, ReasonStaleQuoteDrift, &sidecar.Sidecar{
		Directives:   []sidecar.Directive{priorJSON(), priorGone()},
		Prohibitions: []sidecar.Prohibition{survProhibition(survTextEdit, 9, survLineEdit)},
	})
	f.extract(t, []sidecar.Directive{priorJSON()}, nil)
	it, out := verifyItem(t, f, "")
	assertLoss(t, it, out, "prohibition", "Sidecars MUST NOT be edited by hand", "lines 9-9")
}

// ── AC-5.2: no false positives ────────────────────────────────────────────

// Rewording is the extractor's job. A body that keeps its anchor survives,
// whatever its text now says — and so does one whose anchor narrowed to a
// slice of the same passage, or widened to a span that contains it.
func TestVerify_RewordedBodySameAnchorIsOK(t *testing.T) {
	cases := map[string]sidecar.Directive{
		"same anchor, new words": survDirective(
			"No anchor may be invented by the extractor. (ref: ADR-900)", 7, survLineAnchor),
		"narrower anchor": survDirective(
			"No anchor may be invented by the extractor. (ref: ADR-900)", 7, "MUST NEVER invent an anchor"),
		"wider anchor": {
			Text: "The extractor MUST quote, never invent. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 7, LineEnd: 9,
				Quote: survLineAnchor + "\n\n" + survLineEdit}},
		},
	}
	for name, reworded := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSurvivalFixture(t, ReasonRequested, &sidecar.Sidecar{
				Directives: []sidecar.Directive{priorJSON(), priorAnchor()},
			})
			f.extract(t, []sidecar.Directive{priorJSON(), reworded}, nil)
			it, out := verifyItem(t, f, "")
			if it.Status != StatusOK || out.ExitCode != ExitVerifyOK {
				t.Fatalf("a reworded body on the same passage is not a loss: status %q exit %d: %s",
					it.Status, out.ExitCode, it.Diagnostic)
			}
			if it.Diagnostic != "" {
				t.Errorf("an anchor-covered body needs no note, got: %s", it.Diagnostic)
			}
		})
	}
}

func TestVerify_MergedBodyCarryingBothAnchorsIsOK(t *testing.T) {
	f := newSurvivalFixture(t, ReasonRequested, &sidecar.Sidecar{
		Directives: []sidecar.Directive{priorJSON(), priorAnchor()},
	})
	merged := sidecar.Directive{
		Text: "Hooks MUST emit JSON, and the extractor MUST NOT invent an anchor. (ref: ADR-900)",
		SourceExcerpts: []sidecar.SourceExcerpt{
			{LineStart: 5, LineEnd: 5, Quote: survLineJSON},
			{LineStart: 7, LineEnd: 7, Quote: survLineAnchor},
		},
	}
	f.extract(t, []sidecar.Directive{merged}, nil)
	it, out := verifyItem(t, f, "")
	if it.Status != StatusOK || out.ExitCode != ExitVerifyOK {
		t.Fatalf("two bodies merged into one carrying both anchors is not a loss: status %q: %s",
			it.Status, it.Diagnostic)
	}
}

// A drifted anchor cannot vouch for its body, so identity falls back to the
// lexical comparator. That fallback must run BEFORE source-removed: without
// it, every drifted body would be "source-removed" and the guard would be
// vacuous for exactly the items stale-quote-drift plans.
func TestVerify_DriftedAnchorBodyLexicallyCoveredIsOKWithFallbackNote(t *testing.T) {
	drifted := survDirective(survTextAnchor, 7, staleQuote)
	f := newSurvivalFixture(t, ReasonStaleQuoteDrift, &sidecar.Sidecar{
		Directives: []sidecar.Directive{priorJSON(), drifted},
	})
	// The same rule, re-anchored to a different passage than the drifted
	// quote: no anchor identity, only the words.
	f.extract(t, []sidecar.Directive{priorJSON(), survDirective(survTextAnchor, 9, survLineEdit)}, nil)
	it, out := verifyItem(t, f, "")
	if it.Status != StatusOK || out.ExitCode != ExitVerifyOK {
		t.Fatalf("a drifted body the output carries word for word is not a loss: status %q: %s",
			it.Status, it.Diagnostic)
	}
	for _, want := range []string{"lexical", "The extractor MUST NOT invent an anchor"} {
		if !strings.Contains(it.Diagnostic, want) {
			t.Errorf("the lexical match must be reported as the weaker instrument; diagnostic omits %q: %q",
				want, it.Diagnostic)
		}
	}
}

func TestVerify_DriftedAnchorBodyNotCoveredAndNotInProseIsSourceRemovedNamedNotFailed(t *testing.T) {
	f := newSurvivalFixture(t, ReasonStaleQuoteDrift, &sidecar.Sidecar{
		Directives: []sidecar.Directive{priorJSON(), priorGone()},
	})
	f.extract(t, []sidecar.Directive{priorJSON()}, nil)
	it, out := verifyItem(t, f, "")
	if it.Status != StatusOK || out.ExitCode != ExitVerifyOK {
		t.Fatalf("a body whose source prose is gone is not a loss: status %q: %s", it.Status, it.Diagnostic)
	}
	for _, want := range []string{"source-removed", "Configuration MUST be written as TOML"} {
		if !strings.Contains(it.Diagnostic, want) {
			t.Errorf("a source-removed body must be named, not passed silently; diagnostic omits %q: %q",
				want, it.Diagnostic)
		}
	}
}

// A `missing` item had no prior sidecar, so there is nothing to have lost. The
// check keys on the plan's reason, not on whatever file sits at the pin path:
// a stray snapshot planted for a missing item must not turn into a verdict.
func TestVerify_MissingReasonRunsNoSurvivalCheck(t *testing.T) {
	f := newFixture(t, "")
	if f.plan.Items[0].Reason != ReasonMissing {
		t.Fatalf("fixture reason %q, want %q", f.plan.Items[0].Reason, ReasonMissing)
	}
	stray := PinSnapshotPath(f.root, f.planID, f.itemID)
	if err := os.MkdirAll(filepath.Dir(stray), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMarshalledSidecar(t, stray, &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/ADR-900-test.md",
		// A body on line 3, whose prose is in the parent and which the output
		// below does not carry: were the check to run, this is a loss.
		Directives: []sidecar.Directive{{
			Text:           "Every ADR MUST record its status. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 3, LineEnd: 3, Quote: "**Status:** Accepted"}},
		}},
	})
	f.extractGood(t)
	out := f.verify(t)
	it := onlyItem(t, out)
	if it.Status != StatusOK {
		t.Fatalf("a missing item verified %q: %s", it.Status, it.Diagnostic)
	}
	for _, word := range []string{"source-removed", "lexical", "dropped"} {
		if strings.Contains(it.Diagnostic, word) {
			t.Errorf("a survival note (%q) on a missing item — the check ran on a reason that has no prior: %q",
				word, it.Diagnostic)
		}
	}
}

func TestVerify_OnLossAcceptPassesWithLossRecorded(t *testing.T) {
	f := newSurvivalFixture(t, ReasonRequested, &sidecar.Sidecar{
		Directives: []sidecar.Directive{priorJSON(), priorAnchor()},
	})
	f.extract(t, []sidecar.Directive{priorJSON()}, nil)
	it, out := verifyItem(t, f, "accept")
	if it.Status != StatusOK || out.ExitCode != ExitVerifyOK {
		t.Fatalf("--on-loss=accept must pass the item: status %q exit %d", it.Status, out.ExitCode)
	}
	for _, want := range []string{"loss accepted", "The extractor MUST NOT invent an anchor", "lines 7-7"} {
		if !strings.Contains(it.Diagnostic, want) {
			t.Errorf("an accepted loss must stay on the record; diagnostic omits %q: %q", want, it.Diagnostic)
		}
	}
}

// ── AC-5.3: remedy text ───────────────────────────────────────────────────

// The survival check is migration-blind: it guards a reextract or a
// per-artifact compile, where no migrate-time suffix exists. Telling the
// caller to re-dispatch "with the migrate-time suffix" there is an
// instruction it cannot follow.
func TestVerify_SurvivalDetailNeverMentionsMigrateSuffix(t *testing.T) {
	for _, reason := range []string{ReasonStaleQuoteDrift, ReasonRequested} {
		t.Run(reason, func(t *testing.T) {
			prior := &sidecar.Sidecar{Directives: []sidecar.Directive{priorJSON(), priorAnchor()}}
			if reason == ReasonStaleQuoteDrift {
				prior.Directives = append(prior.Directives, priorGone())
			}
			f := newSurvivalFixture(t, reason, prior)
			f.extract(t, []sidecar.Directive{priorJSON()}, nil)
			it, out := verifyItem(t, f, "")
			if it.Status != StatusLosslessLoss {
				t.Fatalf("premise: status %q, want %q", it.Status, StatusLosslessLoss)
			}
			for _, text := range []string{it.Diagnostic, it.CorrectionPrompt, out.Message} {
				low := strings.ToLower(text)
				for _, banned := range []string{"migrat", "suffix", reason} {
					if strings.Contains(low, banned) {
						t.Errorf("survival text mentions %q — the remedy must be reason-neutral and "+
							"migration-blind:\n%s", banned, text)
					}
				}
			}
		})
	}
}

// Every retryable status verify assigns is either re-dispatched as a
// correction pass carrying a non-empty correction_prompt, or is named here
// with the reason it is not. The INCOMPLETE next step reports an item with an
// empty correction_prompt instead of re-dispatching it, so a retryable status
// that silently carried none would be a retry the caller is told to skip.
func TestVerify_EveryRetryableStatusCarriesCorrectionPrompt(t *testing.T) {
	driven := map[string]func(t *testing.T) ReceiptItem{
		StatusLengthExceeded: func(t *testing.T) ReceiptItem {
			f := newFixture(t, "")
			f.extractGood(t, overCap(MaxItemTextLen+40, "The service MUST publish every domain event."))
			return onlyItem(t, f.verify(t))
		},
		StatusAnchorInvalid: func(t *testing.T) ReceiptItem {
			f := newFixture(t, "")
			f.extractGood(t, sidecar.Directive{
				Text:           "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)",
				SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: "The extractor SHOULD quote the artifact."}},
			})
			return onlyItem(t, f.verify(t))
		},
		StatusLosslessLoss: func(t *testing.T) ReceiptItem {
			f := newSurvivalFixture(t, ReasonRequested, &sidecar.Sidecar{
				Directives: []sidecar.Directive{priorJSON(), priorAnchor()},
			})
			f.extract(t, []sidecar.Directive{priorJSON()}, nil)
			it, _ := verifyItem(t, f, "")
			return it
		},
		// The lifted item's lossless-loss, the same status from the other
		// comparator: it carries a prompt too (conflict #6).
		StatusLosslessLoss + " (lifted)": func(t *testing.T) ReceiptItem {
			f := newLiftedFixture(t, []string{preservedRule})
			liftedOutput(t, f, "Something else entirely. (ref: ADR-900)")
			return verifyWith(t, f, "")
		},
	}
	// Retryable statuses that do not carry a prompt, and why. Each is
	// deliberate; a status absent from both maps fails the test.
	notCorrectionPass := map[string]string{
		StatusOutputMissing: "no output to correct — re-dispatched as a fresh extraction, which the INCOMPLETE next step says",
		StatusStalled:       "no output to correct — re-dispatched as a fresh extraction, which the INCOMPLETE next step says",
		StatusSchemaInvalid: "conditional: a localised parse error or an over-claim carries a prompt (TestVerify_UnparseableSidecarIsSchemaInvalidNotOutputMissing); " +
			"an unlocalised one carries none on purpose (TestCorrectionPromptEmptyOnNonCorrectableAxes)",
		StatusPromptVersionMismatch: "the contract moved under the plan; the remedy is a fresh plan, not a correction pass",
		StatusLosslessUnmeasured:    "there is no baseline to restore from; a re-dispatch cannot create one — --on-loss=accept or a human",
	}

	statuses := receiptStatusEnum(t)
	for _, st := range statuses {
		if IsTerminalStatus(st) {
			continue
		}
		_, isDriven := driven[st]
		reason, excluded := notCorrectionPass[st]
		switch {
		case isDriven && excluded:
			t.Errorf("status %q is both driven and excluded", st)
		case !isDriven && !excluded:
			t.Errorf("retryable status %q neither carries a correction_prompt here nor is named as not needing one", st)
		case excluded:
			t.Logf("%s: no correction pass — %s", st, reason)
		}
	}
	for name, drive := range driven {
		t.Run(name, func(t *testing.T) {
			it := drive(t)
			want := strings.TrimSuffix(name, " (lifted)")
			if it.Status != want {
				t.Fatalf("premise: drove status %q, want %q: %s", it.Status, want, it.Diagnostic)
			}
			if strings.TrimSpace(it.CorrectionPrompt) == "" {
				t.Fatalf("a retryable %q item carries no correction_prompt — it would be reported, not re-dispatched: %s",
					it.Status, it.Diagnostic)
			}
		})
	}

	// The two statuses re-dispatched without a prompt must be named by the
	// INCOMPLETE next step, or "an empty correction_prompt is reported" would
	// strand them.
	next := verifyVerdicts[ExitVerifyIncomplete].nextStep
	for _, st := range []string{StatusOutputMissing, StatusStalled} {
		if !strings.Contains(next, st) {
			t.Errorf("the INCOMPLETE next step does not say how a %s item is re-dispatched: %q", st, next)
		}
	}
}

// receiptStatusEnum reads phasea-receipt.v3's status enum, the authority on
// which statuses a receipt can carry.
func receiptStatusEnum(t *testing.T) []string {
	t.Helper()
	doc := readReceiptSchema(t)
	props, _ := doc["properties"].(map[string]any)
	items, _ := props["items"].(map[string]any)
	item, _ := items["items"].(map[string]any)
	itemProps, _ := item["properties"].(map[string]any)
	status, _ := itemProps["status"].(map[string]any)
	enum, _ := status["enum"].([]any)
	var out []string
	for _, v := range enum {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatal("could not read the receipt status enum — the check cannot run, and that is not a pass")
	}
	return out
}

// A re-extraction that drops dozens of bodies must still produce a receipt
// that validates: diagnostic and correction_prompt are capped at 4000
// characters by phasea-receipt.v3, and a receipt that fails its schema is a
// verify that recorded nothing.
func TestVerify_SurvivalTextFitsTheReceipt(t *testing.T) {
	parent := "# Title\n\n**Status:** Accepted\n\n"
	prior := &sidecar.Sidecar{}
	for i := 0; i < 60; i++ {
		line := fmt.Sprintf("Rule %02d: every component in this tier MUST record its obligation number %02d "+
			"in the audit log before it acts on any request from another tier.", i, i)
		parent += line + "\n\n"
		// Bodies distinct enough that the lexical fallback cannot match one
		// against another: near-identical texts are within its 0.10 ratio.
		word := strings.Repeat(string(rune('a'+i%26))+string(rune('a'+i/26)), 6)
		prior.Directives = append(prior.Directives, survDirective(
			fmt.Sprintf("Components MUST honour the %s constraint. (ref: ADR-900)", word), 5+2*i, line))
	}
	f := newSurvivalFixtureWithParent(t, ReasonRequested, parent, prior)
	f.extract(t, prior.Directives[:1], nil)
	it, _ := verifyItem(t, f, "")
	if it.Status != StatusLosslessLoss {
		t.Fatalf("premise: status %q, want %q", it.Status, StatusLosslessLoss)
	}
	if n := len(it.Diagnostic); n > 4000 {
		t.Errorf("diagnostic is %d characters, over the receipt's 4000", n)
	}
	if n := len(it.CorrectionPrompt); n > 4000 {
		t.Errorf("correction_prompt is %d characters, over the receipt's 4000", n)
	}
	if !strings.Contains(it.Diagnostic, "59 prior bod") {
		t.Errorf("a capped diagnostic must still state how many bodies were lost: %q", it.Diagnostic[:200])
	}
}
