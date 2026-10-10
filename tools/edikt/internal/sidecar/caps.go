// caps.go — ONE definition of every sidecar length cap.
//
// Before this file the 500-char limit was stated in four Go locations plus
// three copies each of two JSON schemas, and the Go validator's number was
// hardcoded into its own error string. Raising a schema cap alone would have
// left the Go loader still rejecting at the old number, which is the exact
// divergence the v1/v2 schema-copy gate exists to prevent, one layer down.
//
// SOFT vs HARD. A soft cap is a TARGET the extractor is asked to meet. A hard
// cap is the schema's refusal. Between the two, a sidecar is ACCEPTED,
// WRITTEN, AND FLAGGED: it compiles, the directive keeps its information, and
// over-soft entries are reported to a reviewer. Never silent loss, and never a
// whole sidecar rejected because one field ran long.
//
// WHY THE HARD CAPS ARE 2000, measured 2026-08-30 over 95 sidecars and 2,354
// capped prose fields in this corpus:
//
//	field                     n     max   p99.9   soft   over-soft
//	text                   1039     473     463    500           0
//	intent                  414     294     294    300           0
//	falsifying_observation  900     296     296    300           0
//
// Zero fields exceed their soft cap, and the longest `text` anywhere is 473
// against a 500 limit -- with NOTHING in 474..500. A natural distribution has
// a tail; this one stops just short of the wall. That is survivorship, not
// headroom: over-length output was rejected and never reached the corpus, so
// the observed maximum is a property of the cap rather than of the writing.
// The true tail is unmeasured, which is precisely why the hard cap must not be
// set just above the censored maximum.
//
// 2000 is 4.2x the longest thing the corpus contains. 700 was considered and
// rejected: at 1.5x the observed max it is still a wall a few characters from
// where a real extraction already hit one, and it would re-create the failure
// while appearing to fix it.
//
// WRITE-TIME COST. A directive is injected per governed write -- measured at
// ~3,700 tokens per injection, ~60% of it already duplicated in the
// always-loaded ambient core. Longer directives make that worse on every
// write, which is an argument for delta-only delivery, not for a tighter hard
// cap that costs whole sidecars.
package sidecar

const (
	// SoftCapText is the target for directives[].text and prohibitions[].text.
	SoftCapText = 500
	// SoftCapIntent covers intent and falsifying_observation.
	SoftCapIntent = 300
	// SoftCapEvidence covers proposed_paths[].evidence and
	// proposed_removals[].evidence.
	SoftCapEvidence = 300
	// SoftCapTopicDescription is the target for proposed_topic_description.
	SoftCapTopicDescription = 300

	// HardCapProse is the schema refusal for every prose field above. One
	// number, deliberately: a per-field hard cap would be a second table to
	// drift, and nothing in the measurement distinguishes the fields at this
	// scale.
	HardCapProse = 2000

	// HardCapTopicDescription is lower than HardCapProse because this field
	// alone renders into the ALWAYS-LOADED core, one line per topic on every
	// edit. Eleven descriptions at 500 take the core to ~3,744 est-tokens,
	// past its ~3,500 target -- which is why the per-field cap cannot be the
	// only guard and doctor reports the assembled surface size separately.
	HardCapTopicDescription = 500
)
