package govrun

import "time"

// Phase A fans extractor dispatches out to a host LLM CLI. Both bounds on
// that fan-out live here, at named symbols, rather than as literals inside
// the dispatcher — because the dispatcher is being deleted and these two
// behaviours have to outlive it (PLAN-v080 D4).
//
// They are expressed as a Runner decorator rather than as Dispatcher fields
// for the same reason: a decorator composes with whatever drives the Runner,
// including the tier-1 orchestration that replaces the Go dispatcher.

// DefaultDispatchConcurrency bounds how many extractor dispatches run at
// once. Eight is the value the Go dispatcher has always defaulted to; it is
// named here so the bound survives that file's removal.
const DefaultDispatchConcurrency = 8

// DefaultDispatchTimeout bounds ONE artifact's extraction — not the batch.
//
// Before this existed, neither dispatch path had a live bound: the compile
// path built a context.Background() with no deadline, and the reextract path
// had a decorator whose timeout field was never assigned, so its
// `if r.timeout > 0` branch never ran. A wedged CLI hung the compile with no
// error and no output. See
// docs/internal/issues/phase-a-dispatch-is-unbounded-on-both-paths.md.
//
// The number comes from EXP-006's measured per-dispatch wall-clock
// (docs/internal/experiments/EXP-006-reextraction-quality-vs-turn-limit.md,
// "Wall-clock per artifact"), n=8, dispatched concurrently at this same
// concurrency of 8:
//
//	median  244.6s (4.1 min)
//	max     494.8s (8.2 min) — ADR-0035
//
// 15 minutes is 1.82x that observed max and 3.68x its median. A 10-minute
// ceiling was the first choice here and is only 1.21x the observed max —
// which means a machine merely 25% slower than EXP-006's would convert a
// healthy extraction into a failure. That is the wrong error to risk, because
// the two errors are not symmetric: a hang is infinite, so 10 and 15 catch it
// equally and differ only in how long you wait once, whereas a false trip
// kills a legitimate extraction mid-flight on the artifact it was rewriting.
//
// Treat the max as weak evidence, not a distribution: n=8, one machine, one
// day, and there is no p99 — the max IS the sample. Duration also does not
// scale cleanly with prose length (ADR-0035 at 226 lines outran ADR-0037 at
// 342), so the ceiling cannot be extrapolated from artifact size. Two live
// artifacts are longer than anything EXP-006 timed — ADR-063 (381 lines) and
// ADR-060 (343) — and both remain untimed.
//
// Re-derive this if the corpus grows or the extractor's turn budget changes;
// do not raise it on a single anecdote.
const DefaultDispatchTimeout = 15 * time.Minute

// NOTE ON SHAPE. These were briefly enforced here by a boundedRunner
// decorator wrapping phasea.Runner. That decorator is gone with the dispatch
// it wrapped: tier-2 no longer invokes an LLM, so there is no Resync call in
// this process to bound.
//
// The constants stay, and stay here, because D4 requires the two behaviours
// the deleted dispatcher carried to have named homes — and because they are
// still the values the contract is written against. What changed is who
// enforces them: they are now DECLARED by tier-2 for tier-1's fan-out to
// honour, not applied by tier-2 itself. Keeping the number without the
// mechanism is deliberate; deleting it would lose the only recorded answer to
// "how long is too long", which EXP-006 paid real wall-clock to establish.
