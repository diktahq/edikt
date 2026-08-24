package govrun

import (
	"strings"
	"testing"
	"time"

)

// EXP-006's observed max per-dispatch wall-clock, the evidence the default is
// derived from. Named here so lowering the ceiling under it fails loudly
// rather than silently converting healthy extractions into timeouts.
const exp006ObservedMaxDispatch = 494800 * time.Millisecond // ADR-0035, n=8

// The ceiling must keep real margin over the only measurement there is.
//
// This is a regression pin, not a re-derivation: it does not claim 15 minutes
// is optimal, only that a value at or under the observed max is definitely
// wrong. 10 minutes — the first value used here — passes this test at 1.21x,
// which is exactly why the test asserts a margin rather than mere ordering.
func TestDefaultDispatchTimeout_KeepsMarginOverMeasuredMax(t *testing.T) {
	if DefaultDispatchTimeout <= exp006ObservedMaxDispatch {
		t.Fatalf("ceiling %v is at or under EXP-006's observed max %v — healthy extractions will time out",
			DefaultDispatchTimeout, exp006ObservedMaxDispatch)
	}
	const wantMargin = 1.5
	got := float64(DefaultDispatchTimeout) / float64(exp006ObservedMaxDispatch)
	if got < wantMargin {
		t.Fatalf("ceiling %v is only %.2fx EXP-006's observed max (%v); want >= %.1fx. "+
			"n=8 on one machine is not a distribution — a box %.0f%% slower than that run "+
			"would false-trip.", DefaultDispatchTimeout, got, exp006ObservedMaxDispatch,
			wantMargin, (got-1)*100)
	}
}

// Both call paths must default to a live bound. This is the regression that
// matters: DispatchTimeout existed, was documented, was passed into a
// context.WithTimeout call — and was never assigned by anybody, so the guard
// in front of it never ran.
func TestDefaultDispatchTimeout_IsLive(t *testing.T) {
	if DefaultDispatchTimeout <= 0 {
		t.Fatalf("DefaultDispatchTimeout is %v; a non-positive default disables the bound", DefaultDispatchTimeout)
	}
	if DefaultDispatchConcurrency <= 0 {
		t.Fatalf("DefaultDispatchConcurrency is %d", DefaultDispatchConcurrency)
	}
}

// The compile path must not dispatch at all.
//
// This replaces TestCompileDispatchSiteWrapsItsRunner, which asserted that
// the Phase A dispatch site wrapped its runner in WithDispatchTimeout. That
// site no longer exists: fanning extractors out means invoking an LLM, which
// INV-012 forbids a tier-2 binary from doing, so Phase A now emits a plan for
// tier-1 and refuses (SPEC-013).
//
// The assertion is inverted rather than deleted. A bound on a dispatch that
// cannot happen protects nothing; what needs protecting now is that the
// dispatch does not come back. This is a source-level check because that is
// where the regression would appear — someone reinstating a Dispatcher here
// would be reintroducing the exemption ADR-030 is retiring.
func TestCompilePathDoesNotDispatch(t *testing.T) {
	src := readSource(t, "twophase.go")
	for _, banned := range []string{
		"phasea.Dispatcher{",
		"dis.Run(",
		"opts.Runner.Preflight()",
	} {
		if strings.Contains(src, banned) {
			t.Fatalf("twophase.go contains %q — Phase A dispatches from tier-2 again, which INV-012 forbids", banned)
		}
	}
	if !strings.Contains(src, "phasea.Plan(") {
		t.Fatal("twophase.go no longer emits a dispatch plan; tier-1 has nothing to fan out from")
	}
}
