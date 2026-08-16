package cmd

import (
	"bytes"
	"crypto/sha256"
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

// TestPlanArtifact_PartialV05x asserts plan returns the uniform
// dry-preserve action under the two-phase model (ADR-034). All
// sentinel shapes — including the previously-distinguished
// v0.5.x-partial — flow through the same structural path; the
// schema label is preserved as audit metadata in the cache but does
// not branch the lift.
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
	if res.action != "dry-preserve" {
		t.Fatalf("want dry-preserve (two-phase uniform path), got %q (warn=%v)", res.action, res.warnLines)
	}
	if res.directives != 1 {
		t.Fatalf("want 1 directive, got %d", res.directives)
	}
	// Schema detection is preserved as audit metadata in the cache.
	if res.cache == nil || res.cache.schema != schemaV05xPartial {
		t.Fatalf("expected schemaV05xPartial in cache as audit metadata, got %v", res.cache)
	}
}

// TestApplyArtifact_PartialV05x_NoLLMInTier2 pins ADR-030: the tier-2
// migrate command MUST NOT shell out to claude (or any LLM CLI). Under
// the two-phase migration model (Phase A: structural strip + preserve;
// Phase B: extractor runs during gov:compile), this test asserts that
// applyArtifact NEVER invokes a staged `claude` binary even when one is
// on PATH. The sidecar is written as a skeleton with topic:
// needs-extraction and the legacy content lifted into MigrationPreserved.
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
	if res.action != "wrote" {
		t.Fatalf("want wrote (two-phase skeleton); got %q err=%v warn=%v",
			res.action, res.err, res.warnLines)
	}
	loaded, err := sidecar.Load(sidecarPath)
	if err != nil {
		t.Fatalf("load skeleton sidecar: %v", err)
	}
	if loaded.Topic != "needs-extraction" {
		t.Fatalf("topic: want needs-extraction (skeleton awaiting extractor); got %q — looks like the stub claude was called", loaded.Topic)
	}
	if loaded.Topic == "stub-should-not-appear" {
		t.Fatal("ADR-030 violation: applyArtifact invoked the stub claude binary")
	}
	// MigrationPreserved must carry the legacy directive verbatim — the
	// extractor will lift it into the canonical directives field on the
	// next gov:compile.
	if loaded.MigrationPreserved == nil {
		t.Fatal("MigrationPreserved missing on skeleton sidecar")
	}
	if got := loaded.MigrationPreserved.Directives; len(got) != 1 || got[0] != "A directive in the prose." {
		t.Errorf("MigrationPreserved.Directives: want 1 verbatim entry; got %v", got)
	}
	updated, _ := os.ReadFile(mdPath)
	if strings.Contains(string(updated), openMarker) {
		t.Fatal("sentinel not removed from md after two-phase apply")
	}
}

// TestApplyArtifact_PartialV05x_NoClaude exercises the apply path when
// no claude binary is on PATH: same skeleton-with-MigrationPreserved
// shape — the binary's migrate is LLM-agnostic per ADR-030.
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
	if res.action != "wrote" {
		t.Fatalf("want wrote (two-phase skeleton); got %q err=%v warn=%v", res.action, res.err, res.warnLines)
	}
	loaded, err := sidecar.Load(filepath.Join(projectRoot, "ADR-100-partial.edikt.yaml"))
	if err != nil {
		t.Fatalf("load skeleton sidecar: %v", err)
	}
	if loaded.Topic != "needs-extraction" {
		t.Fatalf("skeleton topic: want needs-extraction, got %q", loaded.Topic)
	}
	if loaded.MigrationPreserved == nil || len(loaded.MigrationPreserved.Directives) != 1 {
		t.Fatalf("MigrationPreserved.Directives: want 1 entry, got %+v", loaded.MigrationPreserved)
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
		{
			name:       "ADR Status: Superseded by ADR-NNN is skipped",
			body:       "# ADR-002: Old approach\n\n**Date:** 2026-03-06\n**Status:** Superseded by ADR-006\n\n## Context\n\nOld content.\n",
			wantSkip:   true,
			wantReason: "ADR superseded — directives no longer authoritative",
		},
		{
			name:       "Superseded recognition is case-insensitive",
			body:       "# ADR-x\n\n**Status:** superseded BY ADR-007\n",
			wantSkip:   true,
			wantReason: "ADR superseded — directives no longer authoritative",
		},
		{
			name:     "Status: Accepted is NOT skipped",
			body:     "# ADR-100\n\n**Status:** Accepted\n\n## Decision\n\nrule.\n",
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
	// Two-phase migration: skeleton with topic: needs-extraction.
	// The legacy "hooks" topic becomes a hint in MigrationPreserved; the
	// extractor will produce the final canonical topic on next compile.
	if loaded.Topic != "needs-extraction" {
		t.Fatalf("topic: want needs-extraction (Phase A skeleton), got %q", loaded.Topic)
	}
	// path: must be relative to projectRoot (the schema's documented shape)
	// and resolve to the sibling .md when joined with projectRoot.
	if loaded.Path != "ADR-001-example.md" {
		t.Fatalf("path: want %q, got %q", "ADR-001-example.md", loaded.Path)
	}
	if got := filepath.Join(dir, loaded.Path); got != mdPath {
		t.Fatalf("path resolution mismatch: %q != %q", got, mdPath)
	}
	// Canonical directives are empty in the skeleton — extractor fills them.
	if len(loaded.Directives) != 0 {
		t.Fatalf("want 0 canonical directives in skeleton, got %d", len(loaded.Directives))
	}
	// Legacy directive lives verbatim in MigrationPreserved.
	if loaded.MigrationPreserved == nil {
		t.Fatal("MigrationPreserved missing on Phase A skeleton")
	}
	if got := loaded.MigrationPreserved.Directives; len(got) != 1 || got[0] != "Hooks must emit JSON. (ref: INV-003)" {
		t.Errorf("MigrationPreserved.Directives: want 1 verbatim entry; got %v", got)
	}
	if loaded.MigrationPreserved.Topic != "hooks" {
		t.Errorf("MigrationPreserved.Topic: want hooks (preserved hint); got %q", loaded.MigrationPreserved.Topic)
	}
	if got := loaded.MigrationPreserved.Signals; len(got) != 2 || got[0] != "hook" || got[1] != "posttooluse" {
		t.Errorf("MigrationPreserved.Signals: want [hook, posttooluse]; got %v", got)
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

	// Two-phase migration: optional fields are preserved in
	// MigrationPreserved, not at the top level of the skeleton sidecar.
	// The extractor (Phase B) reads them and outputs them in the
	// canonical fields per its preservation rule.
	mp := loaded.MigrationPreserved
	if mp == nil {
		t.Fatal("MigrationPreserved missing on Phase A skeleton")
	}

	// manual_directives — user-authored rules must survive migration
	if len(mp.ManualDirectives) != 1 || mp.ManualDirectives[0] != "Always verify the hook script is executable." {
		t.Errorf("MigrationPreserved.manual_directives not carried: %v", mp.ManualDirectives)
	}

	// suppressed_directives — user rejections must survive migration
	if len(mp.SuppressedDirectives) != 1 || mp.SuppressedDirectives[0] != "Do not cache hook results across sessions." {
		t.Errorf("MigrationPreserved.suppressed_directives not carried: %v", mp.SuppressedDirectives)
	}

	// reminders — pre-action reminders must survive migration
	if len(mp.Reminders) != 1 || !strings.Contains(mp.Reminders[0], "ref: ADR-010") {
		t.Errorf("MigrationPreserved.reminders not carried: %v", mp.Reminders)
	}

	// verification — checklist items must survive migration
	if len(mp.Verification) != 1 || !strings.HasPrefix(mp.Verification[0], "[ ]") {
		t.Errorf("MigrationPreserved.verification not carried: %v", mp.Verification)
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

// v1.2 migration tests — verify_kind defaulting, idempotency, and byte safety.

// v12Fixture writes a minimal v1.1 sidecar with the given directives/prohibitions
// to dir and returns its path. Directives at indices in withVerify get a verify
// field; the rest do not. Prohibitions all get a verify field.
func v12Fixture(t *testing.T, dir, name string, numDirectives, withVerifyCount, numProhibitions int) string {
	t.Helper()
	sc := &sidecar.Sidecar{
		SchemaVersion: 1,
		Topic:         "test-topic",
		Path:          name + ".md",
		Signals:       []string{"test"},
	}
	for i := range numDirectives {
		d := sidecar.Directive{
			Text: fmt.Sprintf("directive %d", i),
			SourceExcerpt: sidecar.SourceExcerpt{
				LineStart: i + 1,
				LineEnd:   i + 1,
				Quote:     fmt.Sprintf("directive %d", i),
			},
		}
		if i < withVerifyCount {
			d.Verify = fmt.Sprintf("echo ok # directive %d", i)
		}
		sc.Directives = append(sc.Directives, d)
	}
	for i := range numProhibitions {
		sc.Prohibitions = append(sc.Prohibitions, sidecar.Prohibition{
			Text: fmt.Sprintf("prohibition %d", i),
			SourceExcerpt: sidecar.SourceExcerpt{
				LineStart: numDirectives + i + 1,
				LineEnd:   numDirectives + i + 1,
				Quote:     fmt.Sprintf("prohibition %d", i),
			},
			Verify: fmt.Sprintf("echo ok # prohibition %d", i),
		})
	}
	out, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("v12Fixture marshal: %v", err)
	}
	p := filepath.Join(dir, name+".edikt.yaml")
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatalf("v12Fixture write: %v", err)
	}
	return p
}

// TestMigrateV12_StructuralDefault verifies that the v1.2 migration pass sets
// verify_kind: structural on directives/prohibitions that carry verify but no
// verify_kind, and leaves directives without verify untouched.
func TestMigrateV12_StructuralDefault(t *testing.T) {
	dir := t.TempDir()
	// 3 directives: first 2 have verify (no verify_kind), third has neither.
	p := v12Fixture(t, dir, "ADR-200", 3, 2, 0)

	n, err := runV12MigrationPass([]string{p})
	if err != nil {
		t.Fatalf("runV12MigrationPass: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 directives migrated; got %d", n)
	}

	loaded, err := sidecar.Load(p)
	if err != nil {
		t.Fatalf("reload after migration: %v", err)
	}
	for i, d := range loaded.Directives {
		if i < 2 {
			if d.VerifyKind != "structural" {
				t.Errorf("directive[%d]: want verify_kind=structural; got %q", i, d.VerifyKind)
			}
		} else {
			if d.VerifyKind != "" {
				t.Errorf("directive[%d]: want verify_kind empty (no verify); got %q", i, d.VerifyKind)
			}
		}
	}
}

// TestMigrateV12_Idempotent verifies that running the migration pass twice
// produces byte-equal sidecar content and returns 0 on the second run.
func TestMigrateV12_Idempotent(t *testing.T) {
	dir := t.TempDir()
	// 2 directives both with verify, 1 prohibition with verify.
	p := v12Fixture(t, dir, "ADR-201", 2, 2, 1)

	n1, err := runV12MigrationPass([]string{p})
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if n1 != 3 {
		t.Fatalf("first pass: want 3 migrated (2 directives + 1 prohibition); got %d", n1)
	}
	after1, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	n2, err := runV12MigrationPass([]string{p})
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second pass: want 0 migrated (idempotent); got %d", n2)
	}
	after2, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(after1, after2) {
		t.Errorf("second pass mutated the sidecar — not byte-equal:\nbefore:\n%s\nafter:\n%s", after1, after2)
	}
}

// TestMigrateV12_ByteEqualCompile verifies that the migration pass does not
// touch pre-compiled governance output files (.md). Migration is confined to
// *.edikt.yaml sidecars; compiled governance rules must remain byte-equal
// before and after (ADR-027 §1).
func TestMigrateV12_ByteEqualCompile(t *testing.T) {
	dir := t.TempDir()
	decisionsDir := filepath.Join(dir, "docs/architecture/decisions")
	govDir := filepath.Join(dir, ".claude/rules/governance")
	for _, d := range []string{decisionsDir, govDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Seed a compiled governance file to simulate pre-existing gov:compile output.
	govFile := filepath.Join(govDir, "adr-decisions.md")
	govContent := []byte("# Compiled governance\n\n## Directives\n\n- directive 0\n- directive 1\n")
	if err := os.WriteFile(govFile, govContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Sidecar with two verify-carrying directives (no verify_kind yet).
	v12Fixture(t, decisionsDir, "ADR-202", 2, 2, 0)

	// Snapshot governance dir before migration.
	snapBefore := dirHash(t, govDir)

	// Run the migration pass on the decisions dir.
	dirs := artifactDirs{
		decisions:  decisionsDir,
		invariants: filepath.Join(dir, "docs/architecture/invariants"),
		guidelines: filepath.Join(dir, "docs/guidelines"),
	}
	paths := collectYAMLSidecarsForV12(dirs)
	if len(paths) == 0 {
		t.Fatal("collectYAMLSidecarsForV12: found no sidecars")
	}
	if _, err := runV12MigrationPass(paths); err != nil {
		t.Fatalf("migration pass: %v", err)
	}

	// Snapshot governance dir after migration.
	snapAfter := dirHash(t, govDir)
	if snapBefore != snapAfter {
		t.Errorf("migration mutated governance dir — byte-equal contract violated\nbefore: %s\nafter:  %s", snapBefore, snapAfter)
	}
}

// dirHash returns a stable hex digest of all regular files under root,
// sorted by path, as a proxy for "nothing changed."
func dirHash(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h.Write([]byte(p))
		h.Write(data)
		return nil
	})
	if err != nil {
		t.Fatalf("dirHash walk: %v", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
