package phasea


// The exact bytes GL-003's extraction produced: a backtick escaped inside a
// double-quoted YAML scalar. Backtick is not a YAML escape — only \" and \\
// are — so this is a hard parse error and the sidecar cannot be loaded.
// Copied from the failing artifact rather than invented, because a fixture
// that merely "looks broken" would not prove the gate catches the shape
// production actually emits.
const d20InvalidSidecar = "schema_version: 1\n" +
	"topic: interfaces\n" +
	"path: docs/guidelines/GL-003-dual-mcp-rest-interface.md\n" +
	"signals:\n" +
	"  - mcp server\n" +
	"directives:\n" +
	// ONE backslash before each backtick. Two would be a legal escaped
	// backslash and the fixture would parse, silently testing nothing —
	// which is what the first version of this file did.
	"  - text: \"Both interfaces MUST call the same \\`internal/rag\\` code. (ref: GL-003)\"\n" +
	"    source_excerpt:\n" +
	"      line_start: 12\n" +
	"      line_end: 12\n" +
	"      quote: \"Both interfaces call the same code.\"\n"

// validSidecar is a schema-valid gov-sidecar body used as a fixture by the
// anchor-verification tests.
//
// Rescued from parsegate_test.go, deleted with ClaudeRunner in PLAN-v080
// Phase 4. That file's tests exercised ClaudeRunner.Resync's parse gate and
// went with their subject — the gate itself did not go, it moved into
// verify.go's per-item check (SPEC-013), where verify_test.go and
// verify_anchors_test.go cover it. Only the fixture needed a new home.
const validSidecar = `schema_version: 1
topic: interfaces
path: docs/guidelines/GL-003-dual-mcp-rest-interface.md
signals:
  - mcp server
directives:
  - text: "Both interfaces MUST call the same internal/rag code. (ref: GL-003)"
    source_excerpt:
      line_start: 12
      line_end: 12
      quote: "Both interfaces call the same code."
`

// parentBody is the prose the fixtures' anchors quote. Line 12 carries the
// quoted sentence, matching the fixtures' recorded line_start/line_end.
//
// It exists because the ADR-056 anchor gate resolves anchors against the real
// parent: a fixture with an anchor and no parent file is a dispatch nothing
// could verify, which the gate correctly refuses. Writing the parent makes
// these tests exercise the gate rather than trip over its unmeasured path.
const parentBody = `# GL-003 — Dual interface

## Rule

Line four.
Line five.
Line six.
Line seven.
Line eight.
Line nine.
Line ten.
Both interfaces call the same code.
`
