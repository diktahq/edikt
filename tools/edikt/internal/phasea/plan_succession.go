package phasea

// plan_succession.go — the two plan-time checks that keep a new plan from
// landing on top of an old one (#11 body, c1-a–g; PLAN-v090-issue-fixes
// Phase 2).
//
//  1. resolveTerminalCollision — the derived plan_id names a plan that is
//     already TERMINAL (consumed, or FAILED). Writing it again would overwrite
//     a closed transaction's plan file and pins. A requested extraction derives
//     a deterministic successor; the corpus path refuses.
//
//  2. guardUnclosedPlans — the LATEST plan covering an item was never closed,
//     and the item's sidecar has moved since that plan pinned it. The moved
//     bytes are unrestored or failed extractor output; planning now would pin
//     them as the new baseline, and the open plan's pin is the only copy of
//     what its restore must reinstate.
//
// Both read state; neither writes. Both are pure functions of on-disk state
// (plan files, receipts, pins and the journal), so the same tree always
// reaches the same verdict and the same successor id.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Terminal plan states, as resolveTerminalCollision and the restore/verify
// refusals name them.
const (
	planStateOpen     = ""
	planStateConsumed = "consumed"
	planStateFailed   = "FAILED"
)

// receiptFailedTerminal reports a FAILED plan: not consumed, every item at a
// terminal status, and at least one of them not ok. No verify call can move
// such a plan again — every later call carries the same items forward.
func receiptFailedTerminal(r *Receipt) bool {
	if r == nil || r.Consumed || len(r.Items) == 0 {
		return false
	}
	nonOK := false
	for _, it := range r.Items {
		if !IsTerminalStatus(it.Status) {
			return false
		}
		if it.Status != StatusOK {
			nonOK = true
		}
	}
	return nonOK
}

// terminalState names a receipt's terminal state, or planStateOpen.
func terminalState(r *Receipt) string {
	switch {
	case r == nil:
		return planStateOpen
	case r.Consumed:
		return planStateConsumed
	case receiptFailedTerminal(r):
		return planStateFailed
	}
	return planStateOpen
}

// succession is resolveTerminalCollision's result: the id to use, the
// corpus_hash it was derived from, and the terminal plans it supersedes, in
// chain order, with the state each was in.
type succession struct {
	PlanID     string
	CorpusHash string
	Supersedes []string
	States     map[string]string
}

// resolveTerminalCollision derives the plan_id for items and, while that id
// names a terminal plan, either refuses (the corpus path) or appends it to the
// `supersedes` chain and derives again (a requested extraction).
//
// requested MUST come from the caller's explicit --artifact/--artifacts, never
// be inferred from ReasonRequested: the decision is about who asked, and an
// item's reason is a property of the item.
//
// An unreadable receipt refuses rather than being read as "not terminal": a
// plan that cannot be proven open is not written over: it fails closed. The loop is
// bounded by the number of plan files on disk — every id in the chain is a
// distinct terminal plan — so a corrupt state directory cannot spin it.
func resolveTerminalCollision(root string, items []workItem, kHash string, requested bool) (*succession, error) {
	bound := countPlanFiles(root)
	s := &succession{States: map[string]string{}}
	for {
		s.CorpusHash = corpusHashWithSuccession(items, s.Supersedes)
		s.PlanID = derivePlanID(s.CorpusHash, kHash)
		r, err := loadReceipt(root, s.PlanID)
		if err != nil {
			return s, fmt.Errorf("the receipt for %s at %s could not be read (%v), so it cannot be proven "+
				"open — refusing rather than writing over what may be a closed plan",
				s.PlanID, relPath(root, ReceiptFilePath(root, s.PlanID)), err)
		}
		state := terminalState(r)
		if state == planStateOpen {
			return s, nil
		}
		if !requested {
			return s, fmt.Errorf("this corpus state derives plan %s, which is %s — re-planning it would "+
				"overwrite a closed plan's file and pins.\n"+
				"  The corpus path never derives a successor. To extract these artifacts again, request them "+
				"by name (`edikt phasea plan --artifact <path>`, or the per-artifact compile command): a "+
				"requested extraction derives a successor plan that supersedes %s",
				s.PlanID, state, s.PlanID)
		}
		s.States[s.PlanID] = state
		s.Supersedes = append(s.Supersedes, s.PlanID)
		if len(s.Supersedes) > bound {
			return s, fmt.Errorf("the succession chain reached %d terminal plan(s) with only %d plan file(s) "+
				"on disk — the state directory is inconsistent; refusing rather than guessing",
				len(s.Supersedes), bound)
		}
	}
}

// countPlanFiles counts .edikt/state/phasea/*.plan.json.
func countPlanFiles(root string) int {
	m, _ := filepath.Glob(filepath.Join(stateDirFor(root), "pap-*.plan.json"))
	return len(m)
}

// supersedesMessage is the success message's succession lines.
func supersedesMessage(s *succession) string {
	if s == nil || len(s.Supersedes) == 0 {
		return ""
	}
	var b strings.Builder
	for _, id := range s.Supersedes {
		fmt.Fprintf(&b, "\nsupersedes %s (%s) — a requested re-extraction of a terminal plan's input gets a successor id", id, s.States[id])
	}
	return b.String()
}

// guardUnclosedPlans refuses the whole plan when, for any item, the LATEST
// plan covering it is still open and the item's sidecar has moved since that
// plan pinned it.
//
// Latest means the last `plan` event in .edikt/state/phasea-dispatch.jsonl
// naming a plan that covers the item; a plan file the journal does not name
// is considered only when no journaled plan covers the item, ordered by file
// mtime. Older plans are never consulted: once a later plan covers the
// item, an abandoned earlier one is history.
//
// The latest plan is skipped — no refusal — when it IS planID (an idempotent
// re-plan), when it is in the supersedes chain (terminal by construction, and
// being succeeded right now), when its file cannot be loaded, or when it is
// consumed (its output is verified, legitimately the new baseline).
//
// "Moved" compares the live pre-dispatch bytes (the bytes the new plan would
// pin) against the open plan's pin. With no pin — the item had no sidecar when
// that plan was built — any sidecar now on disk has moved.
func guardUnclosedPlans(root, planID string, supersedes []string, items []workItem) error {
	skip := map[string]bool{planID: true}
	for _, id := range supersedes {
		skip[id] = true
	}

	plans := loadPlanIndex(root)
	type blocker struct {
		item    workItem
		planID  string
		state   string
		remedy  string
		pinPath string
		hasPin  bool
	}
	var blocked []blocker
	var readErrs []string

	for _, it := range items {
		latest := plans.latestCovering(it.ItemID)
		if latest == "" || skip[latest] {
			continue
		}
		r, err := loadReceipt(root, latest)
		if err != nil {
			readErrs = append(readErrs, fmt.Sprintf("%s: the receipt of plan %s could not be read (%v) — it cannot be proven closed",
				it.ItemID, latest, err))
			continue
		}
		if r != nil && r.Consumed {
			continue
		}

		pinPath := PinSnapshotPath(root, latest, it.ItemID)
		pin, perr := os.ReadFile(pinPath)
		hasPin := perr == nil
		if perr != nil && !os.IsNotExist(perr) {
			readErrs = append(readErrs, fmt.Sprintf("%s: the pin of plan %s at %s could not be read (%v)",
				it.ItemID, latest, relPath(root, pinPath), perr))
			continue
		}
		if hasPin == (it.beforeSidecar != nil) && bytes.Equal(pin, it.beforeSidecar) {
			continue // the sidecar is exactly what that plan pinned: nothing to lose
		}

		sc := it.ExpectedOutput.Path
		b := blocker{item: it, planID: latest, pinPath: relPath(root, pinPath), hasPin: hasPin}
		switch {
		case receiptFailedTerminal(r):
			b.state = "FAILED (terminal) — its extractor output is on disk"
			if hasPin {
				b.remedy = fmt.Sprintf("copy %s back over %s (or `git checkout -- %s`), then re-request — that derives a successor of %s",
					b.pinPath, sc, sc, latest)
			} else {
				b.remedy = fmt.Sprintf("%s took no pin (%s had no sidecar when it was planned): remove %s (or `git checkout -- %s` "+
					"if it is tracked), then re-request with --bootstrap — that derives a successor of %s", latest, it.ItemID, sc, sc, latest)
			}
		case restoreReceiptFor(root, latest) == nil:
			b.state = "dispatched and not restored"
			b.remedy = fmt.Sprintf("edikt phasea restore --plan %s, then edikt phasea verify --plan %s", latest, latest)
		default:
			b.state = "restored, not verified to a close (mid-retry)"
			b.remedy = fmt.Sprintf("finish %s's dispatch loop: re-dispatch its non-terminal items, then edikt phasea restore --plan %s "+
				"and edikt phasea verify --plan %s until it is consumed or FAILED", latest, latest, latest)
		}
		if hasPin && it.beforeSidecar == nil && !receiptFailedTerminal(r) {
			b.remedy += fmt.Sprintf("\n    (%s is absent — restore quarantined a failed write, or it was deleted; the pin still "+
				"holds the pre-dispatch bytes and can be copied back)", sc)
		}
		blocked = append(blocked, b)
	}

	if len(blocked) == 0 && len(readErrs) == 0 {
		return nil
	}

	var msg strings.Builder
	fmt.Fprintf(&msg, "the latest plan for %d item(s) was never closed and the sidecar "+
		"has moved since that plan pinned it. Planning now would pin unrestored or failed extractor output as the "+
		"new baseline, and the open plan's pin is the only copy of what its restore must reinstate "+
		"(manual_directives, approved paths, approvals).", len(blocked)+len(readErrs))
	abandoned := map[string]bool{}
	var abandonOrder []string
	for _, b := range blocked {
		fmt.Fprintf(&msg, "\n  %s: plan %s is %s (pin: %s)\n    remedy: %s",
			b.item.ItemID, b.planID, b.state, pinLabel(b.hasPin, b.pinPath), b.remedy)
		if !abandoned[b.planID] {
			abandoned[b.planID] = true
			abandonOrder = append(abandonOrder, b.planID)
		}
	}
	for _, e := range readErrs {
		fmt.Fprintf(&msg, "\n  %s", e)
	}
	for _, id := range abandonOrder {
		fmt.Fprintf(&msg, "\n  If the sidecar change is a deliberate human edit and %s is abandoned, remove its runtime state first:"+
			"\n    rm -r %s %s",
			id, relPath(root, PlanFilePath(root, id)), relPath(root, filepath.Join(stateDirFor(root), id+".pins")))
	}
	return errors.New(msg.String())
}

func pinLabel(has bool, path string) string {
	if has {
		return path
	}
	return "none — the item had no sidecar when it was planned"
}

// planIndex is every loadable plan on disk, with the journal's ordering.
type planIndex struct {
	items      map[string]map[string]bool // plan_id -> item_ids it covers
	journalPos map[string]int             // plan_id -> index of its LAST `plan` event
	mtime      map[string]int64           // plan_id -> plan file mtime (ns)
}

// loadPlanIndex reads every .edikt/state/phasea/pap-*.plan.json this binary
// can load (same schema version, matching plan_id) and the journal's `plan`
// events. A plan file it cannot load is not an input: it cannot be restored or
// verified by this binary either, so it cannot be the plan holding the pin.
func loadPlanIndex(root string) *planIndex {
	idx := &planIndex{items: map[string]map[string]bool{}, journalPos: map[string]int{}, mtime: map[string]int64{}}
	files, _ := filepath.Glob(filepath.Join(stateDirFor(root), "pap-*.plan.json"))
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".plan.json")
		if !planIDPattern.MatchString(id) {
			continue
		}
		raw, err := os.ReadFile(f) //nolint:gosec // state path built from a validated id
		if err != nil {
			continue
		}
		var p DispatchPlan
		if json.Unmarshal(raw, &p) != nil || p.PlanID != id || p.SchemaVersion != SchemaVersionPlan {
			continue
		}
		cov := map[string]bool{}
		for _, it := range p.Items {
			cov[it.ItemID] = true
		}
		idx.items[id] = cov
		if st, err := os.Stat(f); err == nil {
			idx.mtime[id] = st.ModTime().UnixNano()
		}
	}

	jf, err := os.Open(filepath.Join(root, filepath.FromSlash(JournalRelPath))) //nolint:gosec // fixed project-relative path
	if err != nil {
		return idx
	}
	defer jf.Close()
	sc := bufio.NewScanner(jf)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	pos := 0
	for sc.Scan() {
		var rec JournalRecord
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		if rec.Event == JournalEventPlan {
			idx.journalPos[rec.PlanID] = pos
		}
		pos++
	}
	return idx
}

// latestCovering is the plan_id of the latest plan covering itemID, or "".
func (idx *planIndex) latestCovering(itemID string) string {
	var journaled, unjournaled []string
	for id, cov := range idx.items {
		if !cov[itemID] {
			continue
		}
		if _, ok := idx.journalPos[id]; ok {
			journaled = append(journaled, id)
		} else {
			unjournaled = append(unjournaled, id)
		}
	}
	if len(journaled) > 0 {
		sort.Slice(journaled, func(i, j int) bool { return idx.journalPos[journaled[i]] < idx.journalPos[journaled[j]] })
		return journaled[len(journaled)-1]
	}
	if len(unjournaled) > 0 {
		sort.Slice(unjournaled, func(i, j int) bool {
			a, b := unjournaled[i], unjournaled[j]
			if idx.mtime[a] != idx.mtime[b] {
				return idx.mtime[a] < idx.mtime[b]
			}
			return a < b
		})
		return unjournaled[len(unjournaled)-1]
	}
	return ""
}
