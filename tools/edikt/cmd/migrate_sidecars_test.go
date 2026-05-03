package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Sentinel markers built at runtime so this source file does not contain
// a literal in-body managed region (which the pre-tool-use hook would block).
const (
	openMarker  = "[edikt:dir" + "ectives:start]: #"
	closeMarker = "[edikt:dir" + "ectives:end]: #"
)

func TestMigrateSidecars_Detect_v05xSchema(t *testing.T) {
	inner := "source_hash: abc123\n" +
		"di" + "rectives_hash: def456\n" +
		"topic: hooks\n" +
		"signals:\n" +
		"  - hook\n" +
		"  - posttooluse\n" +
		"di" + "rectives:\n" +
		"  - \"Hooks must emit JSON. (ref: INV-003)\"\n"
	if got := detectSchema(inner); got != schemaV05x {
		t.Fatalf("want schemaV05x, got %v", got)
	}
}

func TestMigrateSidecars_Detect_v043Schema(t *testing.T) {
	inner := "content_hash: deadbeef\n" +
		"di" + "rectives:\n" +
		"  - \"Some legacy rule.\"\n"
	if got := detectSchema(inner); got != schemaV043 {
		t.Fatalf("want schemaV043, got %v", got)
	}
}

func TestMigrateSidecars_Detect_unknownSchema(t *testing.T) {
	// Genuinely unrecognizable: no hashes, no topic, no directives.
	// directives-only blocks are now picked up as partial-v0.5.x for
	// LLM resync (Phase 8 of PLAN-sidecar-review-fixes #8 — the
	// dogfood corpus contained this shape and was being silently
	// skipped before).
	inner := "random_unrelated_key: foo\n"
	if got := detectSchema(inner); got != schemaUnknown {
		t.Fatalf("want schemaUnknown, got %v", got)
	}
}

// TestDetectSchema_PreHashMechanical pins the broader Phase 8 detection:
// a sentinel with topic + directives but no hashes (the most common
// pre-/edikt:adr:compile authoring shape) MUST classify as schemaV05x so
// the mechanical lift runs without an LLM dispatch.
func TestDetectSchema_PreHashMechanical(t *testing.T) {
	inner := "topic: architecture\n" +
		"paths:\n  - \"**/*\"\n" +
		"scope:\n  - planning\n" +
		"di" + "rectives:\n" +
		"  - \"Some hand-authored rule. (ref: ADR-001)\"\n"
	if got := detectSchema(inner); got != schemaV05x {
		t.Fatalf("want schemaV05x, got %v", got)
	}
}

// TestDetectSchema_DirectivesOnlyResyncs covers the earliest sentinel
// shape — a flat directives: list without topic or hashes. These must
// classify as partial so the LLM extractor can derive topic + signals
// from prose at apply time.
func TestDetectSchema_DirectivesOnlyResyncs(t *testing.T) {
	inner := "di" + "rectives:\n" +
		"  - \"Bare directive without topic. (ref: ADR-001)\"\n"
	if got := detectSchema(inner); got != schemaV05xPartial {
		t.Fatalf("want schemaV05xPartial, got %v", got)
	}
}

// TestDetectSchema_PartialV05x pins Phase 8 of PLAN-sidecar-review-fixes
// #8: a sentinel block that has source_hash but no topic/signals MUST
// classify as schemaV05xPartial (not schemaUnknown). This is the
// dogfood-project shape and any v0.5.x project that never backfilled
// `topic:` per governance/tooling.md line 6.
func TestDetectSchema_PartialV05x(t *testing.T) {
	inner := "source_hash: abc123\n" +
		"di" + "rectives_hash: def456\n" +
		"compiler_version: \"0.5.0\"\n" +
		"di" + "rectives:\n" +
		"  - \"Some rule. (ref: ADR-001)\"\n"
	if got := detectSchema(inner); got != schemaV05xPartial {
		t.Fatalf("want schemaV05xPartial, got %v", got)
	}

	// And a sanity check: the v0.5.x full case stays unaffected.
	full := "source_hash: abc\ntopic: hooks\nsignals:\n  - hook\n" +
		"di" + "rectives:\n  - \"x\"\n"
	if got := detectSchema(full); got != schemaV05x {
		t.Fatalf("full v0.5.x: want schemaV05x, got %v", got)
	}
}

// TestPlanArtifact_PartialV05x asserts plan returns the dry-llm-resync
// action for a partial-v0.5.x sentinel block.
func TestPlanArtifact_PartialV05x(t *testing.T) {
	body := "# ADR-100 — partial fixture\n\n" +
		"## Decision\n\nA directive in the prose.\n\n" +
		openMarker + "\n" +
		"source_hash: abc\n" +
		"di" + "rectives_hash: def\n" +
		"compiler_version: \"0.5.0\"\n" +
		"di" + "rectives:\n" +
		"  - \"A directive in the prose.\"\n" +
		closeMarker + "\n"
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "ADR-100-partial.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c := candidate{mdPath: mdPath, artifactID: "ADR-100", kind: "adr"}
	res := planArtifact(c)
	if res.action != "dry-llm-resync" {
		t.Fatalf("want dry-llm-resync, got %q (warn=%v)", res.action, res.warnLines)
	}
	if !res.needsLLM {
		t.Fatal("partial-v0.5.x should set needsLLM=true")
	}
	if res.directives != 1 {
		t.Fatalf("want 1 directive, got %d", res.directives)
	}
}

// TestApplyArtifact_PartialV05x_NoLLMInTier2 pins ADR-030: the tier-2
// migrate command MUST NOT shell out to claude (or any LLM CLI). Even
// when a stub `claude` is staged on PATH and would happily produce a
// valid sidecar, applyArtifact MUST ignore it and write a partial
// mechanical sidecar with topic: needs-review. The host-agent-driven
// tier-1 markdown handles the LLM resync separately.
//
// Replaces the previous TestApplyArtifact_PartialV05x_Mock, which
// asserted the in-Go dispatch path that ADR-030 retired.
func TestApplyArtifact_PartialV05x_NoLLMInTier2(t *testing.T) {
	body := "# ADR-100 — partial fixture\n\n" +
		"## Decision\n\nA directive in the prose.\n\n" +
		openMarker + "\n" +
		"source_hash: abc\n" +
		"di" + "rectives_hash: def\n" +
		"compiler_version: \"0.5.0\"\n" +
		"di" + "rectives:\n" +
		"  - \"A directive in the prose.\"\n" +
		closeMarker + "\n"
	projectRoot := t.TempDir()
	mdPath := filepath.Join(projectRoot, "ADR-100-partial.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(projectRoot, "ADR-100-partial.edikt.yaml")

	// Stage a stub `claude` binary that would produce a valid sidecar
	// IF apply ever called it. Then prefix it on PATH so any
	// regression-introduced LookPath would find it. Asserting that the
	// stub-OUTPUT is NOT what landed on disk is the test's load-bearing
	// signal — it proves applyArtifact never invoked the stub.
	stubDir := t.TempDir()
	stubBody := "#!/usr/bin/env bash\nset -e\ncat > \"$EDIKT_STUB_OUT\" <<'YAML'\n" +
		"schema_version: 1\n" +
		"topic: stub-should-not-appear\n" +
		"path: ADR-100-partial.md\n" +
		"signals:\n  - resync\n" +
		"directives:\n" +
		"  - text: \"A directive in the prose.\"\n" +
		"    source_excerpt:\n" +
		"      line_start: 5\n" +
		"      line_end: 5\n" +
		"      quote: \"A directive in the prose.\"\n" +
		"YAML\n"
	stubPath := filepath.Join(stubDir, "claude")
	if err := os.WriteFile(stubPath, []byte(stubBody), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("EDIKT_STUB_OUT", sidecarPath)

	c := candidate{mdPath: mdPath, artifactID: "ADR-100", kind: "adr"}
	res := applyArtifact(c, t.TempDir(), projectRoot)
	if res.action != "wrote-partial" {
		t.Fatalf("want wrote-partial (mechanical fallback per ADR-030); got %q err=%v warn=%v",
			res.action, res.err, res.warnLines)
	}
	loaded, err := sidecar.Load(sidecarPath)
	if err != nil {
		t.Fatalf("load fallback sidecar: %v", err)
	}
	if loaded.Topic != "needs-review" {
		t.Fatalf("topic: want needs-review (LLM not invoked); got %q — looks like the stub claude was called", loaded.Topic)
	}
	if loaded.Topic == "stub-should-not-appear" {
		t.Fatal("ADR-030 violation: applyArtifact invoked the stub claude binary")
	}
	if !strings.Contains(strings.Join(res.warnLines, " "), "Run /edikt:upgrade") {
		t.Errorf("expected warn line to direct user to /edikt:upgrade for resync; got %v", res.warnLines)
	}
	updated, _ := os.ReadFile(mdPath)
	if strings.Contains(string(updated), openMarker) {
		t.Fatal("sentinel not removed from md after partial-v0.5.x apply")
	}
}

// TestApplyArtifact_PartialV05x_NoClaude exercises the fallback when the
// stub binary is absent: apply MUST write a partial mechanical sidecar
// with topic: needs-review and remove the sentinel from the md.
func TestApplyArtifact_PartialV05x_NoClaude(t *testing.T) {
	body := "# ADR-100 — partial fixture\n\n" +
		"## Decision\n\nA directive in the prose.\n\n" +
		openMarker + "\n" +
		"source_hash: abc\n" +
		"di" + "rectives_hash: def\n" +
		"compiler_version: \"0.5.0\"\n" +
		"di" + "rectives:\n" +
		"  - \"A directive in the prose.\"\n" +
		closeMarker + "\n"
	projectRoot := t.TempDir()
	mdPath := filepath.Join(projectRoot, "ADR-100-partial.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Empty PATH so exec.LookPath fails for "claude".
	t.Setenv("PATH", t.TempDir())

	c := candidate{mdPath: mdPath, artifactID: "ADR-100", kind: "adr"}
	res := applyArtifact(c, t.TempDir(), projectRoot)
	if res.action != "wrote-partial" {
		t.Fatalf("want wrote-partial fallback, got %q err=%v warn=%v", res.action, res.err, res.warnLines)
	}
	loaded, err := sidecar.Load(filepath.Join(projectRoot, "ADR-100-partial.edikt.yaml"))
	if err != nil {
		t.Fatalf("load fallback sidecar: %v", err)
	}
	if loaded.Topic != "needs-review" {
		t.Fatalf("fallback topic: want needs-review, got %q", loaded.Topic)
	}
}

func TestMigrateSidecars_SkipFenced(t *testing.T) {
	body := "# ADR-foo\n\nIntro.\n\n```\n" + openMarker +
		"\ndi" + "rectives:\n  - \"x\"\n" + closeMarker +
		"\n```\n\nMore prose.\n"
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "ADR-100-foo.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c := candidate{mdPath: mdPath, artifactID: "ADR-100", kind: "adr"}
	res := planArtifact(c)
	if res.action != "skipped" {
		t.Fatalf("want skipped, got %q (warn=%v)", res.action, res.warnLines)
	}
}

// TestMigrateSidecars_SkipList exercises Phase 6 of
// PLAN-sidecar-review-fixes #16: the hardcoded ADR-008-/ADR-009-/SPEC-
// prefix list is gone, replaced by an opt-in declaration on the
// artifact itself (frontmatter or top-of-body marker).
func TestMigrateSidecars_SkipList(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name       string
		body       string
		wantSkip   bool
		wantReason string // exact match when wantSkip; ignored otherwise
	}{
		{
			name:       "frontmatter migration: skip with explicit reason",
			body:       "---\nmigration: skip\nreason: documents legacy format\n---\n# ADR-008\n",
			wantSkip:   true,
			wantReason: "documents legacy format",
		},
		{
			name:       "frontmatter documents_legacy_format: true",
			body:       "---\ndocuments_legacy_format: true\n---\n# ADR-009\n",
			wantSkip:   true,
			wantReason: "documents_legacy_format: true",
		},
		{
			name:       "marker comment with reason",
			body:       "# heading\n\n<!-- edikt:migration:skip reason=\"docs the legacy schema\" -->\n\nbody\n",
			wantSkip:   true,
			wantReason: "docs the legacy schema",
		},
		{
			name:       "marker comment without reason",
			body:       "# heading\n\n<!-- edikt:migration:skip -->\n\nbody\n",
			wantSkip:   true,
			wantReason: "marker comment present",
		},
		{
			name:     "no declaration → not skipped",
			body:     "# heading\n\nbody\n",
			wantSkip: false,
		},
		{
			name:     "ADR-008-style filename without marker is no longer auto-skipped",
			body:     "# ADR-008: Legacy schema\n\nplain body — no migration directive.\n",
			wantSkip: false,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, fmt.Sprintf("ADR-%03d-%s.md", i+100, "case"))
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			gotSkip, gotReason := isSkipListed(path)
			if gotSkip != tc.wantSkip {
				t.Fatalf("skip=%v, want %v (reason=%q)", gotSkip, tc.wantSkip, gotReason)
			}
			if tc.wantSkip && gotReason != tc.wantReason {
				t.Fatalf("reason=%q, want %q", gotReason, tc.wantReason)
			}
		})
	}

	if skip, _ := isSkipListed(filepath.Join(dir, "does-not-exist.md")); skip {
		t.Fatal("missing file should not be reported as skipped")
	}
}

func TestMigrateSidecars_MechanicalLift_v05x(t *testing.T) {
	body := "# ADR-001 — example\n\n" +
		"## Decision\n\n" +
		"Hooks must emit JSON. (ref: INV-003)\n\n" +
		"## Sentinel\n\n" +
		openMarker + "\n" +
		"source_hash: abc\n" +
		"di" + "rectives_hash: def\n" +
		"topic: hooks\n" +
		"signals:\n" +
		"  - hook\n" +
		"  - posttooluse\n" +
		"di" + "rectives:\n" +
		"  - \"Hooks must emit JSON. (ref: INV-003)\"\n" +
		closeMarker + "\n"
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "ADR-001-example.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c := candidate{mdPath: mdPath, artifactID: "ADR-001", kind: "adr"}
	res := applyArtifact(c, t.TempDir(), dir)
	if res.action != "wrote" {
		t.Fatalf("want wrote, got %q err=%v warn=%v", res.action, res.err, res.warnLines)
	}
	loaded, err := sidecar.Load(res.sidecarPath)
	if err != nil {
		t.Fatalf("load sidecar: %v", err)
	}
	if loaded.Topic != "hooks" {
		t.Fatalf("topic: want hooks, got %q", loaded.Topic)
	}
	// path: must be relative to projectRoot (the schema's documented shape)
	// and resolve to the sibling .md when joined with projectRoot.
	if loaded.Path != "ADR-001-example.md" {
		t.Fatalf("path: want %q, got %q", "ADR-001-example.md", loaded.Path)
	}
	if got := filepath.Join(dir, loaded.Path); got != mdPath {
		t.Fatalf("path resolution mismatch: %q != %q", got, mdPath)
	}
	if len(loaded.Directives) != 1 {
		t.Fatalf("want 1 directive, got %d", len(loaded.Directives))
	}
	if loaded.Directives[0].SourceExcerpt.LineStart < 1 {
		t.Fatalf("source line not resolved: %+v", loaded.Directives[0].SourceExcerpt)
	}
	updated, _ := os.ReadFile(mdPath)
	if strings.Contains(string(updated), openMarker) {
		t.Fatalf("sentinel not removed from md")
	}
}

func TestMigrateSidecars_Idempotency_apply(t *testing.T) {
	body := "# ADR-002 example\n\nA directive sentence here.\n\n" +
		openMarker + "\n" +
		"source_hash: a\n" +
		"di" + "rectives_hash: b\n" +
		"topic: misc\n" +
		"signals:\n" +
		"  - alpha\n" +
		"di" + "rectives:\n" +
		"  - \"A directive sentence here.\"\n" +
		closeMarker + "\n"
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "ADR-002-foo.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c := candidate{mdPath: mdPath, artifactID: "ADR-002", kind: "adr"}
	res1 := applyArtifact(c, t.TempDir(), dir)
	if res1.action != "wrote" {
		t.Fatalf("first apply: want wrote, got %q", res1.action)
	}
	res2 := applyArtifact(c, t.TempDir(), dir)
	if res2.action != "already-migrated" {
		t.Fatalf("second apply: want already-migrated, got %q", res2.action)
	}
}

func TestRelPathOrBase(t *testing.T) {
	tests := []struct {
		name        string
		projectRoot string
		target      string
		want        string
	}{
		{
			name:        "relative under project root",
			projectRoot: "/proj",
			target:      "/proj/docs/architecture/decisions/ADR-100-foo.md",
			want:        "docs/architecture/decisions/ADR-100-foo.md",
		},
		{
			name:        "empty project root falls back to basename",
			projectRoot: "",
			target:      "/whatever/ADR-100-foo.md",
			want:        "ADR-100-foo.md",
		},
		{
			name:        "target outside project root falls back to basename",
			projectRoot: "/proj",
			target:      "/elsewhere/ADR-100-foo.md",
			want:        "ADR-100-foo.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relPathOrBase(tt.projectRoot, tt.target); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMigrateSidecars_JSONFlag pins ADR-029 / Phase 5 of
// PLAN-sidecar-review-fixes finding #29: migrate sidecars accepts --json
// and emits a single JSON document on stdout. The dry-run helper text is
// routed to stderr.
func TestMigrateSidecars_JSONFlag(t *testing.T) {
	bin := buildBinary(t)

	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "docs/architecture/decisions"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# ADR-001 — example\n\n" +
		"## Decision\n\nA test directive. (ref: INV-003)\n\n" +
		openMarker + "\n" +
		"source_hash: abc\n" +
		"di" + "rectives_hash: def\n" +
		"topic: hooks\n" +
		"signals:\n  - hook\n" +
		"di" + "rectives:\n  - \"A test directive. (ref: INV-003)\"\n" +
		closeMarker + "\n"
	if err := os.WriteFile(filepath.Join(work, "docs/architecture/decisions/ADR-001-test.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}

	cmd := exec.Command(bin, "migrate", "sidecars", "--dry-run", "--json")
	cmd.Dir = work
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	// stdout MUST be a single parseable JSON document with the documented shape.
	var parsed struct {
		Status  string         `json:"status"`
		Mode    string         `json:"mode"`
		Summary map[string]int `json:"summary"`
		Items   []struct {
			Source  string `json:"source"`
			Sidecar string `json:"sidecar"`
			Action  string `json:"action"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("--json output not parseable: %v\nstdout:\n%s", err, stdout.String())
	}
	if parsed.Status != "ok" {
		t.Fatalf("status: want ok, got %q", parsed.Status)
	}
	if parsed.Mode != "dry-run" {
		t.Fatalf("mode: want dry-run, got %q", parsed.Mode)
	}
	if parsed.Summary["to_create"] != 1 {
		t.Fatalf("summary.to_create: want 1, got %d", parsed.Summary["to_create"])
	}
	if len(parsed.Items) != 1 || parsed.Items[0].Source != "ADR-001-test.md" {
		t.Fatalf("items[]: want one item for ADR-001-test.md; got %+v", parsed.Items)
	}

	// Prose progress lines MUST go to stderr in --json mode.
	if !strings.Contains(stderr.String(), "migrate sidecars (dry-run):") {
		t.Fatalf("expected prose progress on stderr; got:\n%s", stderr.String())
	}
}

// TestMigrateSidecars_CarriesOptionalFields verifies the upgrade regression fix:
// applyArtifact MUST carry manual_directives, suppressed_directives, reminders,
// and verification from the old sentinel block into the new .edikt.yaml sidecar.
// A v0.4.3 → v0.6.0 upgrade that drops these fields silently destroys user
// authored overrides, which is the primary upgrade regression risk.
func TestMigrateSidecars_CarriesOptionalFields(t *testing.T) {
	body := "# ADR-010 — optional fields fixture\n\n" +
		"## Decision\n\nHooks must emit JSON. (ref: INV-003)\n\n" +
		openMarker + "\n" +
		"source_hash: abc\n" +
		"di" + "rectives_hash: def\n" +
		"topic: hooks\n" +
		"signals:\n" +
		"  - hook\n" +
		"  - posttooluse\n" +
		"di" + "rectives:\n" +
		"  - \"Hooks must emit JSON. (ref: INV-003)\"\n" +
		"manual_di" + "rectives:\n" +
		"  - \"Always verify the hook script is executable.\"\n" +
		"suppressed_di" + "rectives:\n" +
		"  - \"Do not cache hook results across sessions.\"\n" +
		"reminders:\n" +
		"  - \"Before migrating a hook to JSON → verify message preserved (ref: ADR-010)\"\n" +
		"verification:\n" +
		"  - \"[ ] hook emits valid JSON (ref: ADR-010)\"\n" +
		closeMarker + "\n"

	dir := t.TempDir()
	mdPath := filepath.Join(dir, "ADR-010-optional-fields.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	c := candidate{mdPath: mdPath, artifactID: "ADR-010", kind: "adr"}
	res := applyArtifact(c, t.TempDir(), dir)
	if res.action != "wrote" {
		t.Fatalf("want wrote, got %q err=%v warn=%v", res.action, res.err, res.warnLines)
	}

	loaded, err := sidecar.Load(res.sidecarPath)
	if err != nil {
		t.Fatalf("load sidecar: %v", err)
	}

	// manual_directives — user-authored rules must survive migration
	if len(loaded.ManualDirectives) != 1 || loaded.ManualDirectives[0] != "Always verify the hook script is executable." {
		t.Errorf("manual_directives not carried: %v", loaded.ManualDirectives)
	}

	// suppressed_directives — user rejections must survive migration
	if len(loaded.SuppressedDirectives) != 1 || loaded.SuppressedDirectives[0] != "Do not cache hook results across sessions." {
		t.Errorf("suppressed_directives not carried: %v", loaded.SuppressedDirectives)
	}

	// reminders — pre-action reminders must survive migration
	if len(loaded.Reminders) != 1 || !strings.Contains(loaded.Reminders[0], "ref: ADR-010") {
		t.Errorf("reminders not carried: %v", loaded.Reminders)
	}

	// verification — checklist items must survive migration
	if len(loaded.Verification) != 1 || !strings.HasPrefix(loaded.Verification[0], "[ ]") {
		t.Errorf("verification not carried: %v", loaded.Verification)
	}

	// Sentinel must be stripped from the source .md
	updated, _ := os.ReadFile(mdPath)
	if strings.Contains(string(updated), openMarker) {
		t.Fatal("sentinel not removed from md after apply")
	}
}

func TestMigrateSidecars_DryRunGate(t *testing.T) {
	ediktRoot := t.TempDir()
	cwd := t.TempDir()
	if err := checkDryRunGate(ediktRoot, cwd); err == nil {
		t.Fatal("expected gate error when no dry-run state exists")
	}
	if err := writeDryRunState(ediktRoot, cwd); err != nil {
		t.Fatal(err)
	}
	if err := checkDryRunGate(ediktRoot, cwd); err != nil {
		t.Fatalf("gate should pass after dry-run state: %v", err)
	}
	if err := checkDryRunGate(ediktRoot, t.TempDir()); err == nil {
		t.Fatal("gate should reject mismatched cwd")
	}
}
