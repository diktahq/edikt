package parse

import (
	"strings"
	"testing"
)

// BenchmarkExtractSentinel_TypicalADR measures the cost of parsing one
// typical-sized ADR's sentinel block. The fixture is built once at the
// outer level and reset before each timed iteration so allocation and
// per-iteration cost stay isolated to ExtractSentinel itself.
//
// Phase 7 of PLAN-sidecar-review-fixes #42 — informational SLO; CI
// reports the number but does not block on it.
func BenchmarkExtractSentinel_TypicalADR(b *testing.B) {
	body := buildTypicalADRBody()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ExtractSentinel(body); err != nil {
			b.Fatal(err)
		}
	}
}

// buildTypicalADRBody assembles a 200-line ADR with a populated sentinel
// block, mirroring the rough size and shape of governance/* artifacts in
// production. The block has a topic, signals, and 12 directives — enough
// to exercise the line-scan path.
func buildTypicalADRBody() string {
	var b strings.Builder
	b.WriteString("# ADR-100: Bench Fixture\n\n**Status:** Accepted\n\n## Context\n\n")
	for i := 0; i < 60; i++ {
		b.WriteString("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ")
	}
	b.WriteString("\n\n## Decision\n\n")
	for i := 0; i < 60; i++ {
		b.WriteString("Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. ")
	}
	b.WriteString("\n\n## Consequences\n\n")
	for i := 0; i < 60; i++ {
		b.WriteString("Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris. ")
	}
	b.WriteString("\n\n## Directives\n\n")
	b.WriteString("[edikt:directives:start]: #\n")
	b.WriteString("source_hash: deadbeef0123456789abcdef0123456789abcdef0123456789abcdef01234567\n")
	b.WriteString("directives_hash: cafef00d0123456789abcdef0123456789abcdef0123456789abcdef01234567\n")
	b.WriteString("compiler_version: \"0.6.0\"\n")
	b.WriteString("topic: bench-topic\n")
	b.WriteString("paths:\n  - \"commands/**\"\n")
	b.WriteString("scope:\n  - implementation\n")
	b.WriteString("signals:\n  - bench\n  - fixture\n  - typical\n")
	b.WriteString("directives:\n")
	for i := 0; i < 12; i++ {
		b.WriteString("  - \"Bench directive number ")
		b.WriteString(itoa(i))
		b.WriteString(" mandates a behavior. (ref: ADR-100)\"\n")
	}
	b.WriteString("[edikt:directives:end]: #\n")
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
