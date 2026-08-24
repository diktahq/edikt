// Package injectmatch is the ONE implementation of the write-time injection
// tier's match-filter-dedup-journal sequence.
//
// WHY IT IS A PACKAGE AND NOT A FUNCTION IN cmd/.
//
// That sequence had exactly one caller — `edikt hook match` — and the shims
// reached it by SHELLING OUT to the binary they were already running inside.
// Migrating inject-directives-pre to Go removes the subprocess, and the
// tempting shortcut is to reimplement the sequence in the handler. That would
// produce two implementations of grade filtering, dedup ordering, and the
// journal's Bounced/DedupSuppressed/BudgetExhausted distinction — and the two
// would drift in the direction nobody watches: the CLI is what tests and
// `doctor` exercise, the handler is what actually governs writes.
//
// So the sequence moves here and both callers use it. The CLI keeps its flags
// and its output shape; the handler gets the same answer without a process.
//
// ORDERING IS THE CONTRACT. Grade filter, THEN dedup, THEN journal — dedup
// keys on exactly the directives the caller would act on, so filtering after
// deduping would key on a set the caller never sees. Re-deriving
// OutcomeMatched to OutcomeNoMatch after an empty grade filter is part of it:
// "entries existed but none of this grade" is a NO-MATCH for this caller, and
// reporting it as a match with zero results would make the pre shim bounce on
// nothing.
package injectmatch

import (
	"fmt"
	"io"
	"sort"

	"github.com/diktahq/edikt/tools/edikt/internal/hookcfg"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// Request is one injection-tier query.
type Request struct {
	Root      string
	Path      string
	Grade     string // "must" | "advisory" | "" for no filter
	SessionID string
	Shim      string // "pre" | "post" | "probe"
	Dedup     bool

	Actor           string
	AgentType       string
	ParentSessionID string
	ContextID       string
}

// Outcome is the answer, in the shape both callers render from.
type Outcome struct {
	hookmatch.Result
	DedupSuppressed bool `json:"dedup_suppressed"`
	BudgetExhausted bool `json:"budget_exhausted"`
}

// Run performs the match and journals it.
//
// warnw receives operator-facing diagnostics (an unreadable config, an
// unavailable dedup store). It is never the protocol channel and may be nil.
//
// NEVER RETURNS AN ERROR. Every failure class in this tier is fail-open by
// ADR-060:d07 — blocking an editor because governance is broken is worse than
// missing an injection — and an error return would invite a caller to treat a
// governance bug as a reason to stop a write. Failures are journaled instead,
// which is what makes fail-open non-silent.
func Run(req Request, warnw io.Writer) Outcome {
	res := hookmatch.Match(req.Root, req.Path)

	// Presentation-only ordering: a human reading a multi-directive deny sees
	// the highest structural class first. Does not change which entries match,
	// whether the write is denied, or the dedup key — all of which key on the
	// entry ID set, never its order.
	sort.SliceStable(res.Entries, func(i, j int) bool {
		return ClassPriority(res.Entries[i].Class) < ClassPriority(res.Entries[j].Class)
	})

	if req.Grade != "" {
		res.Entries = hookmatch.FilterGrade(res.Entries, req.Grade)
		if len(res.Entries) == 0 && res.Outcome == hookmatch.OutcomeMatched {
			res.Outcome = hookmatch.OutcomeNoMatch
		}
	}

	suppressedByDedup := false
	budgetExhausted := false
	if req.Dedup && res.Outcome == hookmatch.OutcomeMatched {
		budget := hookmatch.BounceBudget
		if cfg, err := hookcfg.Load(req.Root); err != nil {
			// Fail toward the compiled default rather than toward zero: a
			// budget of 0 is disguised enforcement-off, which hookcfg.Load
			// itself refuses to represent, and an unreadable config must not
			// reach that state through this path instead.
			warnf(warnw, "edikt hook match: .edikt/config.yaml unreadable (%v); using default bounce budget %d\n", err, budget)
		} else {
			budget = cfg.BounceBudget
		}
		result, err := hookmatch.AlreadyBounced(req.SessionID, req.ContextID, res.Entries, budget)
		if err != nil {
			// Bounce. Failing toward TELLING the agent about a MUST-grade rule
			// is the safe direction.
			warnf(warnw, "edikt hook match: dedup state unavailable (%v); bouncing anyway\n", err)
		} else if result.BudgetExhausted {
			// Entries stay populated: the caller must still surface them, as
			// advisory rather than a deny — "proceeds LOUDLY", not "proceeds
			// unheard".
			budgetExhausted = true
		} else if !result.Bounce {
			suppressedByDedup = true
			res.Entries = nil
		}
	}

	ids := make([]string, 0, len(res.Entries))
	for _, e := range res.Entries {
		ids = append(ids, e.ID)
	}
	// The journal error is ignored on purpose: a journal that cannot be
	// written must not break the write it is observing. `hook report`
	// compensates by reporting staleness.
	_ = hookmatch.Append(hookmatch.Record{
		Shim:      req.Shim,
		Outcome:   string(res.Outcome),
		Detail:    res.Detail,
		Path:      res.NormPath,
		Matched:   len(res.Entries),
		SessionID: req.SessionID,

		Actor:           req.Actor,
		AgentType:       req.AgentType,
		ParentSessionID: req.ParentSessionID,
		ContextID:       req.ContextID,
		DirectiveIDs:    ids,
		// BOUNCED means the deny actually fired. A match suppressed by dedup
		// delivered nothing new — the agent was already told — and counting it
		// would inflate the one number that is near-proof a rule changed an
		// outcome. A budget-exhausted match is neither: it delivered something
		// new (advisory) but did not deny.
		Bounced:         req.Shim == "pre" && len(res.Entries) > 0 && !suppressedByDedup && !budgetExhausted,
		DedupSuppressed: suppressedByDedup,
		BudgetExhausted: budgetExhausted,
	})

	return Outcome{Result: res, DedupSuppressed: suppressedByDedup, BudgetExhausted: budgetExhausted}
}

func warnf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}

// ClassPriority orders invariant before ADR before guideline before unknown.
func ClassPriority(class string) int {
	switch class {
	case "invariant":
		return 0
	case "adr":
		return 1
	case "guideline":
		return 2
	default:
		return 3
	}
}
