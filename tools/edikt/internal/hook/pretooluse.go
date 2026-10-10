package hook

// pretooluse.go — the Go replacement for templates/hooks/pre-tool-use.sh
// (PLAN-v080-reliability-core Phase 20). The INV-005 managed-region guard.
//
// THIS IS THE ONLY HOOK THAT CAN DENY A WRITE. Everything else in the file is
// a route to not-denying. Four branches deny, at three distinct reasons:
//
//	wholly-generated  a compiled surface edited by hand
//	settings.json     an in-scope JSON file whose region format cannot be verified
//	overlap           an Edit whose byte range intersects a managed region
//	region-dropping   a Write whose new content omits an existing region
//
// The last two share a reason by construction: both are "this change would
// modify region N", reached by different routes.
//
// NO python3 GATE, DELIBERATELY. The bash original degraded when python3 was
// absent — its guard WAS a python heredoc, so the degradation was real and its
// banner ("the INV-005 sentinel guard could NOT check this write") was true.
// It is false here: this handler has no python3 dependency and checks fine.
// Reproducing the gate for byte-parity would stand a security guard down on a
// host where nothing is wrong — ADR-063:d02 exists to forbid exactly that.
// Sixteen already-migrated handlers do reproduce it; see
// docs/internal/issues/go-handlers-gate-on-a-dependency-they-do-not-have.md
// (measured: 0 Go handlers invoke python3, 16 gate on it). This hook could be
// corrected because its registry row was still `pending` when the corpus was
// recorded.
//
// The project-context advisory rides INSIDE the reply, never after it. Two
// top-level JSON objects are invalid JSON and the host discards ALL of them,
// deny included — that defect shipped through v0.7.3 and is why this hook's
// guard did not enforce (see the advisory in docs/reports/).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

func init() {
	Register("pre-tool-use", preToolUse)
}

// maxSentinelScanBytes caps the read so a multi-GB file under a governance
// dir cannot exhaust hook memory. Real artifacts are far below it; truncating
// a pathological input is safe because the capped prefix is a strict superset
// of any plausible sentinel-bearing region.
const maxSentinelScanBytes = 2 << 20

// Two real alternatives, not one pattern straining to cover both. The bash
// original's single pattern could never match the unnamed form's end marker,
// so `regions` was ALWAYS empty for CLAUDE.md — a structural reason the guard
// never protected it, on top of the deny-channel bug ADR-061 fixed.
var (
	sentinelStartRe = regexp.MustCompile(`(?m)^\[edikt:(?:start|([a-z][a-z0-9-]*):start)\]:\s*#(?:\s.*)?$`)
	sentinelEndRe   = regexp.MustCompile(`(?m)^\[edikt:(?:end|([a-z][a-z0-9-]*):end)\]:\s*#(?:\s.*)?$`)
)

var whollyGeneratedBasenames = map[string]bool{
	"manifest.yaml":      true,
	"delivery-map.yaml":  true,
	adapter.SkillFileKey: true,
}

// scopeKey normalizes an externally-controlled value before it is compared
// against this guard's allowlists (INV-006).
//
// WHY THIS IS A SECURITY CONTROL AND NOT TIDINESS. Every scope decision below
// is an allowlist comparison — `base == "CLAUDE.md"`, `base == "settings.json"`
// — against a basename taken straight from the hook payload, which is
// attacker-influenced. Without normalization a Unicode lookalike that NFKC
// folds to "CLAUDE.md" compares UNEQUAL, the file is judged out of scope, and
// the guard allows a write it should have denied. The bypass costs one
// homoglyph.
//
// The bash original did not normalize either, so this closes a gap rather
// than preserving one — which is what ADR-063:d02 licenses: the recording is
// evidence of what bash did, not a specification of what the guard should do.
//
// Comparisons are one-directional: normalizing only WIDENS what matches the
// allowlist, so this can put a file in scope that was previously skipped, and
// can never take one out.
func scopeKey(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(strings.TrimSpace(b.String()))
}

type sentinelRegion struct {
	start, end int
	name       string
}

func preToolUse(inv *Invocation) Result {
	// The advisory is resolved FIRST so every reply below can carry it in the
	// same object. Never emitted separately.
	nudge := contextAdvisory()

	// B2 — a compile or migration is rewriting the very regions this guard
	// defends. Silent by design.
	if os.Getenv("EDIKT_COMPILE_IN_PROGRESS") == "1" ||
		os.Getenv("EDIKT_MIGRATION_IN_PROGRESS") == "1" ||
		fileExists(filepath.Join(".edikt", "state", ".migration-in-progress")) {
		// A bare `{"continue": true}` with NO advisory: bash exits here before
		// the advisory block is reached, and the corpus pins that.
		return Result{Response: adapter.Response{ContinueTrue: true}}
	}

	// B4 — the bypass switch. A payload that denies without it must not deny
	// with it.
	if os.Getenv("_EDIKT_SKIP_SENTINEL_GUARD") == "1" {
		// The advisory ALONE, with no `continue`. When the guard does not run
		// there is no decision object to fold into, so bash emits the bare
		// advisory and nothing else.
		return Result{Response: adapter.Response{Message: nudge}}
	}

	p := inv.Payload

	// B3 — no payload at all (test harness, manual invocation). Bash sets
	// _EDIKT_SKIP_SENTINEL_GUARD here rather than exiting, so the guard is
	// skipped but the advisory block still runs: a project-setup nag fires
	// even when the hook is exercised without a payload. With no advisory to
	// emit that means genuinely NO output — not `{"continue": true}`.
	if p.ToolName == "" && p.FilePath == "" && p.Path == "" {
		return Result{Response: adapter.Response{Message: nudge}}
	}

	// P1 — neither Edit nor Write: nothing to judge.
	if p.ToolName != "Edit" && p.ToolName != "Write" {
		return allowWith(nudge)
	}

	// INV-021 — a governance-producing path is not governed by what it  edikt-guard:allow
	// produces. The extractor writing a sidecar is production, not governed
	// work, and this is the one hook left that can deny a write (ADR-083 c).  edikt-guard:allow
	// Attestation is the HOST's agent_type on the payload, resolved by the
	// dispatcher — never an env var and never a field the writer sets, since
	// either would be the writer attesting to its own exemption. The empty
	// and placeholder values are not exempt: an unnamed writer is governed.
	if hookmatch.IsGovernanceProducingAgent(p.AgentType) {
		return allowWith(nudge)
	}

	filePath := p.FilePath
	if filePath == "" {
		filePath = p.Path
	}
	// P2 — no path at all. Distinct from P3: nothing to judge in scope,
	// rather than a path judged out of scope.
	if filePath == "" {
		return allowWith(nudge)
	}

	// P3 — scope narrowing.
	if !isInSentinelScope(filePath) {
		return allowWith(nudge)
	}

	real := resolvePath(filePath)
	base := filepath.Base(real)

	// P4 — a wholly generated surface. Checked BEFORE the region scan: these
	// files carry no markdown sentinel, so an empty scan must not be read as
	// "nothing to protect".
	//
	if whollyGeneratedBasenames[scopeKey(base)] ||
		(scopeKey(base) == "topics.yaml" && strings.Contains(real, string(os.PathSeparator)+".edikt"+string(os.PathSeparator))) {
		return denyWith(nudge, "edikt: "+base+" is generated by `gov compile` and must not be edited "+
			"directly. Every byte of it is compile output, so an edit here is lost on the next "+
			"compile — and until then the tree enforces something no sidecar says. Change the "+
			"source ADR/invariant/guideline and re-run `edikt gov compile`.")
	}

	// A compile-generated governance file is managed WHOLE, not by region.
	// bok round 2 (D81 b) edited a compiled topic file's `# Migrations`
	// heading and was allowed, while an edit inside the same file's
	// directives region was denied — but every byte of that file is compile
	// output, so the heading edit is lost on the next compile just the same,
	// and until then the tree carries a rule no sidecar states. INV-005's
	// region hash stays as it is for USER-owned files (CLAUDE.md,
	// settings.json), where a region is genuinely a part of a larger file
	// the user owns.
	if isWhollyCompiledPath(real) {
		return denyWith(nudge, "edikt: "+base+" is compiled by `gov compile` and is managed as a whole "+
			"file, not by region — every byte of it, headings included, is rebuilt from the source "+
			"ADRs, invariants and guidelines. An edit here is lost on the next compile, and until "+
			"then the tree enforces something no sidecar says. Change the source artifact and "+
			"re-run `edikt gov compile`.")
	}

	onDisk, err := readCapped(real)
	if err != nil {
		// Creating a new file: no existing region to protect.
		return allowWith(nudge)
	}

	regions := scanSentinelRegions(onDisk)

	if len(regions) == 0 {
		// P5a — settings.json is routed in scope but is JSON, so it can never
		// host a markdown sentinel. An empty scan here does NOT mean "nothing
		// to protect"; it means "this guard cannot verify this file at all",
		// which is a different claim and must not fall through to allow.
		if scopeKey(base) == adapter.SettingsFileKey {
			return denyWith(nudge, "edikt: settings.json edits cannot be verified against damage to a "+
				"managed region — the JSON-hosted region variant is not yet implemented, so this "+
				"guard cannot tell a safe edit from one that empties the deny list or removes a "+
				"hook registration. This check covers the Edit/Write tools only — it does not "+
				"intercept a write issued through Bash. Have a human review this edit directly, "+
				"or use `/edikt:config` for supported changes.")
		}
		// P5b — a markdown file that genuinely opted out. Allow.
		return allowWith(nudge)
	}

	start, end, ok := computeEditRange(p, onDisk, regions)
	// P6/P8 — no locatable range: nothing to compare against a region.
	if !ok {
		return allowWith(nudge)
	}

	// P9 (and P7, which reaches here by a different route).
	for _, r := range regions {
		if start < r.end && end > r.start {
			name := r.name
			if name == "" {
				name = "(unnamed)"
			}
			return denyWith(nudge, "edit would modify the edikt-managed sentinel region '"+name+"' in "+
				filePath+". The managed region is rebuilt by /edikt:gov:compile from source "+
				"artifacts (ADRs, invariants, guidelines). Edit the source artifact and re-run "+
				"compile instead of hand-editing the compiled block.")
		}
	}
	return allowWith(nudge)
}

// computeEditRange returns the byte range a change would touch.
//
// For Write the range is the FIRST existing region the new content drops —
// a region-destroying write is judged by what it removes, not by where it
// writes, because a Write replaces the whole file and has no cursor.
func computeEditRange(p adapter.Payload, onDisk string, regions []sentinelRegion) (int, int, bool) {
	if p.ToolName == "Edit" && p.OldString != "" {
		idx := strings.Index(onDisk, p.OldString)
		if idx == -1 {
			return 0, 0, false
		}
		return idx, idx + len(p.OldString), true
	}
	if p.ToolName == "Write" {
		for _, r := range regions {
			if !strings.Contains(p.Content, onDisk[r.start:r.end]) {
				return r.start, r.end, true
			}
		}
	}
	return 0, 0, false
}

// scanSentinelRegions pairs start markers with the first unused end marker of
// the same name that follows them, so nested or repeated regions resolve.
func scanSentinelRegions(text string) []sentinelRegion {
	type marker struct {
		pos  int
		name string
	}
	var starts, ends []marker
	for _, m := range sentinelStartRe.FindAllStringSubmatchIndex(text, -1) {
		starts = append(starts, marker{pos: m[0], name: groupOrEmpty(text, m)})
	}
	for _, m := range sentinelEndRe.FindAllStringSubmatchIndex(text, -1) {
		ends = append(ends, marker{pos: m[1], name: groupOrEmpty(text, m)})
	}
	used := make(map[int]bool)
	var out []sentinelRegion
	for _, s := range starts {
		for i, e := range ends {
			if used[i] || e.pos <= s.pos || e.name != s.name {
				continue
			}
			used[i] = true
			out = append(out, sentinelRegion{start: s.pos, end: e.pos, name: s.name})
			break
		}
	}
	return out
}

func groupOrEmpty(text string, m []int) string {
	if len(m) >= 4 && m[2] >= 0 {
		return text[m[2]:m[3]]
	}
	return ""
}

func readCapped(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, maxSentinelScanBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return "", nil
	}
	return string(buf[:n]), nil
}

func resolvePath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			return resolved
		}
		return abs
	}
	return p
}

// contextAdvisory returns the project-context nudge, or "" when it does not
// apply. It is folded into whatever reply the guard produces — never emitted
// as a second object.
func contextAdvisory() string {
	if !fileExists(filepath.Join(".edikt", "config.yaml")) {
		return ""
	}
	if fileExists(filepath.Join("docs", "project-context.md")) {
		return ""
	}
	return "⚠ edikt: docs/project-context.md not found. Run /edikt:init to complete setup."
}

func allowWith(nudge string) Result {
	return Result{Response: adapter.Response{ContinueTrue: true, Message: nudge}}
}

func denyWith(nudge, reason string) Result {
	return Result{Response: adapter.Response{
		Decision: adapter.DecisionDeny,
		Reason:   reason,
		Message:  nudge,
	}}
}

// isWhollyCompiledPath reports whether a path is a file `gov compile`
// generates in full: the compiled governance tree (.claude/rules/governance/
// and the ambient governance.md beside it) and edikt's own rendered skill
// packages (.claude/skills/edikt-*/). Every byte of these is compile output.
//
// It matches on the DIRECTORY, not the basename: a user's own governance.md
// or SKILL.md elsewhere in the tree is theirs, and only a file sitting in a
// compile output directory is managed whole.
func isWhollyCompiledPath(real string) bool {
	sep := string(os.PathSeparator)
	rulesDir := sep + filepath.FromSlash(adapter.GovernanceRulesSegment)
	if strings.Contains(real, rulesDir) {
		return true
	}
	// The ambient core sits one level up, beside the topic directory.
	if scopeKey(filepath.Base(real)) == "governance.md" &&
		strings.Contains(real, sep+adapter.HostConfigSegment+sep+"rules"+sep) {
		return true
	}
	if strings.Contains(real, sep+filepath.FromSlash(adapter.SkillsSegmentPrefix)) {
		return true
	}
	return false
}

// isInSentinelScope narrows what this guard will even read, in the bash
// original's own rule order.
//
// The narrowing is the difference between a guard and a nuisance: without it
// every write in the tree is a candidate, and the false-positive rate is what
// gets a security hook disabled by the people it protects.
func isInSentinelScope(fp string) bool {
	real := resolvePath(fp)
	base := filepath.Base(real)
	sep := string(os.PathSeparator)

	// Rule 1 — CLAUDE.md anywhere. It hosts managed regions at any depth.
	key := scopeKey(base)
	if key == adapter.InstructionFileKey {
		return true
	}

	// Rule 2 — settings.json under CLAUDE_HOME or any .claude/ ancestor.
	if key == adapter.SettingsFileKey {
		hostHome := os.Getenv(adapter.HostHomeEnv)
		if hostHome == "" {
			if home, err := os.UserHomeDir(); err == nil {
				hostHome = filepath.Join(home, adapter.HostHomeDirName)
			}
		}
		if hostHome != "" {
			chReal := resolvePath(hostHome)
			if real == chReal || strings.HasPrefix(real, chReal+sep) {
				return true
			}
		}
		return strings.Contains(real, sep+adapter.HostConfigSegment+sep)
	}

	// Rule 3 — compiled markdown under .claude/rules/.
	if strings.HasSuffix(key, ".md") {
		if strings.Contains(real, sep+adapter.HostConfigSegment+sep+"rules"+sep) &&
			(strings.Contains(real, sep+"governance"+sep) || key == "governance.md") {
			return true
		}
	}

	// Rule 3b — the wholly-generated surfaces, each pinned to the directory
	// that makes it a compile output rather than a user file of the same name.
	rulesMarker := sep + filepath.FromSlash(adapter.GovernanceRulesSegment)
	if (key == "manifest.yaml" || key == "delivery-map.yaml") && strings.Contains(real, rulesMarker) {
		return true
	}
	if key == "topics.yaml" && strings.Contains(real, sep+".edikt"+sep) {
		return true
	}
	if key == adapter.SkillFileKey && strings.Contains(real, sep+filepath.FromSlash(adapter.SkillsSegmentPrefix)) {
		return true
	}

	// Rule 4 — anything under a configured governance dir.
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	for _, rel := range governanceScopeDirs() {
		cand := resolvePath(filepath.Join(cwd, rel))
		if real == cand || strings.HasPrefix(real, cand+sep) {
			return true
		}
	}
	return false
}

// governanceScopeDirs returns the governance dirs, defaults only.
//
// The bash original hand-parsed .edikt/config.yaml and hardened that parse
// against path traversal — any value with a ".." segment, a leading "/", or a
// realpath escaping cwd was rejected, because a malicious config could
// otherwise redirect rule 4 to "/" and put every file in the tree back in
// scope. Reading no config at all is strictly safer than reproducing a
// hand-parser: the defaults cannot be redirected.
//
// The cost is that a project with custom governance paths falls back to
// defaults here. That is a narrowing, never a widening — this guard can miss
// a file it should have checked, but cannot be pointed at one it should not.
// Recorded rather than silent, because it is a real behaviour difference from
// the bash original and belongs in the migration's evidence.
func governanceScopeDirs() []string {
	return []string{
		filepath.Join("docs", "architecture", "decisions"),
		filepath.Join("docs", "architecture", "invariants"),
		filepath.Join("docs", "guidelines"),
	}
}
