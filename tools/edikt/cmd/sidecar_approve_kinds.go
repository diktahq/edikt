package cmd

// sidecar_approve_kinds.go — the `--kind` extension of `edikt sidecar approve`.
//
// SPEC-011 Stage 0 (spec decision): "approval reuses the ADR-039 ceremony —  // edikt-guard:allow
// extend `bin/edikt sidecar approve` with `--kind paths` rather than a new
// verb (the `approve` verb is already permitted; one ceremony, two kinds)."
// `--kind topic-description` lands the same way for the topic registry.
//
// Extending the existing verb rather than adding a new one is deliberate:
// ADR-029 Rule 3's permit list is enumerated data, and `sidecar approve` is  // edikt-guard:allow
// already on it (ADR-039). A new verb would need a permit row before any  // edikt-guard:allow
// tier-1 command could call it; a new --kind needs none. The contract
// extension itself is recorded in the phase-3 FR-009 ADR, per the plan.
//
// The ADR-039 exit-code contract is UNCHANGED and binds all three kinds:  // edikt-guard:allow
//
//	0 — state mutated as requested (or intentional no-op for --decision=defer)
//	1 — validation or IO error
//	2 — pending-id not found
//	3 — invalid args
//
// Per ADR-030 this file stays LLM-agnostic — covered by the  // edikt-guard:allow
// no-llm-in-tier-2 grep gate.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/pathsproposal"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
	"gopkg.in/yaml.v3"
)

// Kind names. "verify" is the ADR-039 original and stays the default so every  // edikt-guard:allow
// existing invocation keeps its exact meaning.
const (
	kindVerify           = "verify"
	kindPaths            = "paths"
	kindTopicDescription = "topic-description"
)

// pendingDirForKind maps a kind to its state subdirectory. Each kind gets its
// own directory rather than a shared one with a type field: a mis-typed --kind
// then reads an empty directory and exits 2 ("not found"), instead of loading
// a differently-shaped file and failing with a decode error that says nothing
// about the mistake.
var pendingDirForKind = map[string]string{
	kindVerify:           "pending-verifies",
	kindPaths:            "pending-paths",
	kindTopicDescription: "pending-topic-descriptions",
}

// pendingPaths is the on-disk shape of one path-scope proposal awaiting
// approval, at .edikt/state/pending-paths/<id>.yaml.
type pendingPaths struct {
	ID               string                    `yaml:"id"`
	SidecarPath      string                    `yaml:"sidecar_path"`
	ProposedPaths    []sidecar.ProposedPath    `yaml:"proposed_paths,omitempty"`
	ProposedRemovals []sidecar.ProposedRemoval `yaml:"proposed_removals,omitempty"`
	ProposedAt       string                    `yaml:"proposed_at,omitempty"`
}

// pendingTopicDescription is the on-disk shape of one registry description
// awaiting approval, at .edikt/state/pending-topic-descriptions/<topic>.yaml.
//
// There is no `description_hash` here on purpose. The hash is what the
// APPROVAL stamps over the bytes a human actually saw; accepting a
// producer-supplied hash would let the proposal certify its own content, which
// is the whole failure the hash exists to catch (SEC #7).
type pendingTopicDescription struct {
	Topic       string `yaml:"topic"`
	Description string `yaml:"description"`
	Evidence    string `yaml:"evidence,omitempty"`
	ProposedAt  string `yaml:"proposed_at,omitempty"`
}

// resolveProjectRoot walks up from CWD to the nearest directory carrying a
// .edikt/ marker, falling back to CWD.
func resolveProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; {
		if info, statErr := os.Stat(filepath.Join(dir, ".edikt")); statErr == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
		dir = parent
	}
}

// resolvePendingDir returns .edikt/state/<subdir> for the given kind.
func resolvePendingDir(kind string) (string, error) {
	sub, ok := pendingDirForKind[kind]
	if !ok {
		return "", fmt.Errorf("unknown kind %q", kind)
	}
	root, err := resolveProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".edikt", "state", sub), nil
}

// ── batch review (`--list`) ──────────────────────────────────────────────────

// listPendingEntry is one row of the --list report.
type listPendingEntry struct {
	ID      string         `json:"id"`
	Path    string         `json:"path"`
	Payload map[string]any `json:"payload"`
	Invalid string         `json:"invalid,omitempty"`
}

// runApproveList emits every pending proposal of a kind as one JSON document.
//
// This is the batch-review mode the plan calls for: ~60 path proposals through
// a strictly one-at-a-time ceremony is a real cost. It is batch REVIEW, not
// batch approval — the tier-1 wrapper renders all rows at once and then issues
// one decision call per row. Nothing here mutates state, and there is
// deliberately no --decision=approve --all: a single keystroke approving sixty
// unread inferences would defeat the point of the ceremony.
//
// INV-013: an empty directory reports `"count": 0` with the scanned directory  edikt-guard:allow
// named. A caller must be able to distinguish "nothing pending" from "I looked
// somewhere else".
func runApproveList(kind string) error {
	dir, err := resolvePendingDir(kind)
	if err != nil {
		return &exitCodeError{code: 3, msg: err.Error()}
	}

	entries := []listPendingEntry{}
	dirEntries, readErr := os.ReadDir(dir)
	if readErr != nil && !os.IsNotExist(readErr) {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("read %s: %v", dir, readErr)}
	}
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".yaml") {
			continue
		}
		full := filepath.Join(dir, de.Name())
		row := listPendingEntry{
			ID:   strings.TrimSuffix(de.Name(), ".yaml"),
			Path: full,
		}
		// Decode loosely for display. A file that will not decode is REPORTED
		// as invalid rather than skipped — a proposal silently missing from a
		// review list is exactly the absence-as-pass shape INV-013 forbids.  edikt-guard:allow
		var payload map[string]any
		body, rdErr := os.ReadFile(full)
		if rdErr != nil {
			row.Invalid = fmt.Sprintf("read: %v", rdErr)
		} else if ymlErr := yaml.Unmarshal(body, &payload); ymlErr != nil {
			row.Invalid = fmt.Sprintf("parse: %v", ymlErr)
		} else {
			row.Payload = payload
		}
		entries = append(entries, row)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	out := map[string]any{
		"kind":        kind,
		"pending_dir": dir,
		"count":       len(entries),
		"pending":     entries,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("encode listing: %v", err)}
	}
	return nil
}

// ── `--kind paths` ───────────────────────────────────────────────────────────

// runApprovePaths promotes an approved path proposal into the sidecar's
// enforced `paths:` list.
//
// The mechanical validation re-runs HERE, against the live tree, even though
// extraction already ran it. The proposal's own `matched_example` is not
// consulted. A proposal that certifies itself is not a check, and the tree can
// have changed between proposal and ceremony.
func runApprovePaths(pendingID, decision string) error {
	dir, err := resolvePendingDir(kindPaths)
	if err != nil {
		return &exitCodeError{code: 3, msg: err.Error()}
	}
	pendingPath := filepath.Join(dir, pendingID+".yaml")

	if _, statErr := os.Stat(pendingPath); os.IsNotExist(statErr) {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("pending-id not found: %s", pendingPath)}
	} else if statErr != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("stat pending file: %v", statErr)}
	}

	if decision == "defer" {
		fmt.Fprintf(os.Stdout, "ok: deferred %s (no state change)\n", pendingID)
		return nil
	}

	pp, err := loadPendingPaths(pendingPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("load pending file: %v", err)}
	}

	if decision == "reject" {
		if err := os.Remove(pendingPath); err != nil {
			return &exitCodeError{code: 1, msg: fmt.Sprintf("remove pending file: %v", err)}
		}
		fmt.Fprintf(os.Stdout, "ok: rejected %s (%d proposed add(s), %d proposed removal(s) discarded, pending file removed)\n",
			pendingID, len(pp.ProposedPaths), len(pp.ProposedRemovals))
		return nil
	}

	// ── approve ─────────────────────────────────────────────────────────────
	if pp.SidecarPath == "" {
		return &exitCodeError{code: 1, msg: "pending file missing sidecar_path"}
	}
	if strings.Contains(pp.SidecarPath, "..") {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("sidecar_path %q: contains traversal", pp.SidecarPath)}
	}
	if len(pp.ProposedPaths) == 0 && len(pp.ProposedRemovals) == 0 {
		return &exitCodeError{code: 1, msg: "pending file carries zero proposed paths or removals (nothing to approve)"}
	}

	root, err := resolveProjectRoot()
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("resolve project root: %v", err)}
	}

	var res pathsproposal.Result
	if len(pp.ProposedPaths) > 0 {
		res, err = pathsproposal.ValidateAgainstRoot(pp.ProposedPaths, root)
		if err != nil {
			// UNMEASURED — refuse rather than approve on an unvalidatable tree.
			return &exitCodeError{code: 1, msg: fmt.Sprintf("validate proposals: %v", err)}
		}
		if !res.OK() {
			var b strings.Builder
			fmt.Fprintf(&b, "refusing to approve %s: %d of %d proposed globs failed validation (against %d files)",
				pendingID, res.Checked-res.Accepted, res.Checked, res.Files)
			for _, f := range res.Findings {
				fmt.Fprintf(&b, "\n  %s", f.String())
			}
			return &exitCodeError{code: 1, msg: b.String()}
		}
	}

	sidecarPath := pp.SidecarPath
	if !filepath.IsAbs(sidecarPath) {
		sidecarPath = filepath.Join(root, sidecarPath)
	}
	if _, statErr := os.Stat(sidecarPath); statErr != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("sidecar not found: %s", sidecarPath)}
	}

	sc, err := sidecar.Load(sidecarPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("load sidecar: %v", err)}
	}

	// ── removals (F-033): preview and refuse BEFORE any mutation ─────────
	//
	// Every removal is previewed against the tree first, and the whole batch
	// is refused if ANY removal would drop a file the sidecar's own
	// directive/prohibition text literally names — that is not a judgment
	// call for this tool to make, it is a defect. Nothing is written to sc
	// until every removal in the batch has cleared this check, so a refusal
	// leaves the on-disk sidecar untouched (atomic: all-or-nothing per
	// approval, matching the add path's own all-proposals-validated-first
	// discipline above).
	var removalPreviews []pathsproposal.RemovalPreview
	if len(pp.ProposedRemovals) > 0 {
		present := map[string]bool{}
		for _, g := range sc.Paths {
			present[g] = true
		}
		removedSet := map[string]bool{}
		for _, r := range pp.ProposedRemovals {
			removedSet[strings.TrimSpace(r.Glob)] = true
		}
		// "Remaining" coverage includes globs this SAME approval is adding —
		// a narrow-and-replace proposal (drop a broad glob, add its evidenced
		// replacements) is the common case, and the replacements are exactly
		// what should keep a named file covered.
		var remainingAfterRemovals []string
		for _, g := range sc.Paths {
			if !removedSet[g] {
				remainingAfterRemovals = append(remainingAfterRemovals, g)
			}
		}
		for _, p := range pp.ProposedPaths {
			remainingAfterRemovals = append(remainingAfterRemovals, strings.TrimSpace(p.Glob))
		}

		for _, r := range pp.ProposedRemovals {
			g := strings.TrimSpace(r.Glob)
			if !present[g] {
				return &exitCodeError{code: 1, msg: fmt.Sprintf(
					"refusing %s: proposed removal %q is not in the sidecar's current paths[] — nothing to remove", pendingID, g)}
			}
			prev, err := pathsproposal.PreviewRemoval(g, remainingAfterRemovals, sc, root)
			if err != nil {
				return &exitCodeError{code: 1, msg: fmt.Sprintf("preview removal %q: %v", g, err)}
			}
			if len(prev.NamedLost) > 0 {
				var b strings.Builder
				fmt.Fprintf(&b, "refusing %s: removing %q would drop %d file(s) that the sidecar's own directive/prohibition text names:",
					pendingID, g, len(prev.NamedLost))
				for _, f := range prev.NamedLost {
					fmt.Fprintf(&b, "\n  %s", f)
				}
				fmt.Fprintf(&b, "\nA narrowing that un-governs a file the text mentions is a defect, not a judgment call — reword the directive or the glob, then re-propose.")
				return &exitCodeError{code: 1, msg: b.String()}
			}
			removalPreviews = append(removalPreviews, prev)
		}
	}

	existing := map[string]bool{}
	for _, g := range sc.Paths {
		existing[g] = true
	}
	added := 0
	for _, p := range pp.ProposedPaths {
		g := strings.TrimSpace(p.Glob)
		if existing[g] {
			continue
		}
		existing[g] = true
		sc.Paths = append(sc.Paths, g)
		added++
	}

	removed := 0
	if len(pp.ProposedRemovals) > 0 {
		removeSet := map[string]bool{}
		for _, r := range pp.ProposedRemovals {
			removeSet[strings.TrimSpace(r.Glob)] = true
		}
		kept := sc.Paths[:0]
		for _, g := range sc.Paths {
			if removeSet[g] {
				removed++
				continue
			}
			kept = append(kept, g)
		}
		sc.Paths = kept
	}
	sort.Strings(sc.Paths)

	// The APPROVED proposals have been adjudicated and must not survive into
	// steady state — but sc.ProposedPaths is the SIDECAR's own field, and it
	// can carry entries this approval never touched: an independent,
	// hand-authored pending proposal targets the same sidecar without being a
	// copy of what the extractor last proposed there (F-077 — ADR-062's own  edikt-guard:allow
	// extraction proposed two paths; a later, differently-sourced manual
	// proposal for a third path was approved, and an unconditional nil here
	// discarded both of the extractor's originals, one of which nobody had
	// reviewed or rejected). Remove only the globs this approval just
	// promoted; anything else pending stays pending.
	approvedGlobs := map[string]bool{}
	for _, p := range pp.ProposedPaths {
		approvedGlobs[strings.TrimSpace(p.Glob)] = true
	}
	remaining := sc.ProposedPaths[:0]
	for _, p := range sc.ProposedPaths {
		if !approvedGlobs[strings.TrimSpace(p.Glob)] {
			remaining = append(remaining, p)
		}
	}
	if len(remaining) == 0 {
		sc.ProposedPaths = nil
	} else {
		sc.ProposedPaths = remaining
	}

	// Same surgical-removal discipline for ProposedRemovals as ProposedPaths
	// above (F-077's own defect was clobbering a field this approval never
	// touched) — remove only the removal proposals this batch just applied.
	approvedRemovals := map[string]bool{}
	for _, r := range pp.ProposedRemovals {
		approvedRemovals[strings.TrimSpace(r.Glob)] = true
	}
	remainingRemovals := sc.ProposedRemovals[:0]
	for _, r := range sc.ProposedRemovals {
		if !approvedRemovals[strings.TrimSpace(r.Glob)] {
			remainingRemovals = append(remainingRemovals, r)
		}
	}
	if len(remainingRemovals) == 0 {
		sc.ProposedRemovals = nil
	} else {
		sc.ProposedRemovals = remainingRemovals
	}

	// RECEIPT (F-004). Without this the approval left no trace at all: the
	// post-approval sidecar was byte-identical to one where an extractor wrote
	// paths[] directly, so nothing downstream could tell an approved scope from
	// a declared one — and AC-4.4's "approved fields survive regeneration
	// byte-intact" was asserting a property the data could not express.
	//
	// Hashed over the globs rather than stamped with a bare timestamp, because
	// a timestamp-only receipt still validates after someone hand-edits paths[]
	// afterwards, which is precisely the case worth catching.
	sc.PathsApproval = &sidecar.PathsApproval{
		ApprovedAt:  time.Now().UTC().Format(time.RFC3339),
		GlobsSHA256: sidecar.HashGlobs(sc.Paths),
	}

	if err := sc.Validate(); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("validate after approve: %v", err)}
	}
	out, err := sidecar.Marshal(sc)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("marshal sidecar: %v", err)}
	}
	if err := atomicWriteNoFollow(sidecarPath, out, 0o644); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("write sidecar: %v", err)}
	}
	if err := os.Remove(pendingPath); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("remove pending file (sidecar already updated): %v", err)}
	}

	fmt.Fprintf(os.Stdout, "ok: approved %s (--kind paths)\n", pendingID)
	fmt.Fprintf(os.Stdout, "    sidecar:   %s\n", sidecarPath)
	if len(pp.ProposedPaths) > 0 {
		fmt.Fprintf(os.Stdout, "    validated: %d/%d added globs against %d files\n", res.Accepted, res.Checked, res.Files)
	}
	fmt.Fprintf(os.Stdout, "    paths:     %d added, %d removed, %d total\n", added, removed, len(sc.Paths))
	for _, prev := range removalPreviews {
		sample := prev.Lost
		if len(sample) > 5 {
			sample = sample[:5]
		}
		fmt.Fprintf(os.Stdout, "    removed %q: %d file(s) lose this artifact's directives", prev.RemovedGlob, len(prev.Lost))
		if len(prev.Lost) > 0 {
			fmt.Fprintf(os.Stdout, " (e.g. %s)", strings.Join(sample, ", "))
		}
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

func loadPendingPaths(path string) (*pendingPaths, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := yaml.NewDecoder(bufio.NewReader(f))
	dec.KnownFields(true)
	var pp pendingPaths
	if err := dec.Decode(&pp); err != nil {
		return nil, err
	}
	return &pp, nil
}

// ── `--kind topic-description` ───────────────────────────────────────────────

// runApproveTopicDescription writes an approved description into
// .edikt/topics.yaml.
//
// The registry is the one pinned-judgment artifact of the release: nothing
// else may write it, and a description is never invented. Approval stamps
// `approved_at` AND `description_hash` over the exact bytes approved, so a
// later edit to the description without a re-approval is mechanically
// detectable — `approved_at` alone only proves someone approved something
// once, not that these bytes are it (pre-flight SEC #7).
func runApproveTopicDescription(pendingID, decision string) error {
	dir, err := resolvePendingDir(kindTopicDescription)
	if err != nil {
		return &exitCodeError{code: 3, msg: err.Error()}
	}
	pendingPath := filepath.Join(dir, pendingID+".yaml")

	if _, statErr := os.Stat(pendingPath); os.IsNotExist(statErr) {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("pending-id not found: %s", pendingPath)}
	} else if statErr != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("stat pending file: %v", statErr)}
	}

	if decision == "defer" {
		fmt.Fprintf(os.Stdout, "ok: deferred %s (no state change)\n", pendingID)
		return nil
	}

	ptd, err := loadPendingTopicDescription(pendingPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("load pending file: %v", err)}
	}

	if decision == "reject" {
		if err := os.Remove(pendingPath); err != nil {
			return &exitCodeError{code: 1, msg: fmt.Sprintf("remove pending file: %v", err)}
		}
		fmt.Fprintf(os.Stdout, "ok: rejected %s (no registry entry written, pending file removed)\n", pendingID)
		return nil
	}

	topic := strings.TrimSpace(ptd.Topic)
	if topic == "" {
		topic = pendingID
	}
	description := ptd.Description

	// --edited-content lets the human correct the proposed line without
	// hand-editing the pending file. Same flag, same meaning, as the verify
	// kind: what gets approved is what the human supplied.
	if approveEditedContent != "" {
		body, readErr := readEditedContent(approveEditedContent)
		if readErr != nil {
			return readErr
		}
		description = body
	}

	root, err := resolveProjectRoot()
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("resolve project root: %v", err)}
	}
	registryPath := topicregistry.PathFor(root)

	reg, err := topicregistry.LoadOrEmpty(registryPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("load registry: %v", err)}
	}

	entry := topicregistry.Entry{
		Description:     description,
		DescriptionHash: topicregistry.HashDescription(description),
		ApprovedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := topicregistry.ValidateEntry(topic, entry); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("reject %s: %v", topic, err)}
	}
	reg[topic] = entry

	out, err := topicregistry.Marshal(reg)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("marshal registry: %v", err)}
	}
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("mkdir registry dir: %v", err)}
	}
	if err := atomicWriteNoFollow(registryPath, out, 0o644); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("write registry: %v", err)}
	}
	if err := os.Remove(pendingPath); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("remove pending file (registry already updated): %v", err)}
	}

	fmt.Fprintf(os.Stdout, "ok: approved %s (--kind topic-description)\n", pendingID)
	fmt.Fprintf(os.Stdout, "    registry:         %s\n", registryPath)
	fmt.Fprintf(os.Stdout, "    topic:            %s\n", topic)
	fmt.Fprintf(os.Stdout, "    description_hash: %s\n", entry.DescriptionHash)
	fmt.Fprintf(os.Stdout, "    approved_at:      %s\n", entry.ApprovedAt)
	return nil
}

func loadPendingTopicDescription(path string) (*pendingTopicDescription, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := yaml.NewDecoder(bufio.NewReader(f))
	dec.KnownFields(true)
	var ptd pendingTopicDescription
	if err := dec.Decode(&ptd); err != nil {
		return nil, err
	}
	return &ptd, nil
}

// readEditedContent applies the same --edited-content validation the verify
// kind uses: no traversal, must exist, must be a regular file.
func readEditedContent(p string) (string, *exitCodeError) {
	if strings.Contains(p, "..") {
		return "", &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content %q: contains traversal", p)}
	}
	abs, absErr := filepath.Abs(p)
	if absErr != nil {
		return "", &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content: %v", absErr)}
	}
	info, statErr := os.Lstat(abs)
	if os.IsNotExist(statErr) {
		return "", &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content not found: %s", abs)}
	}
	if statErr != nil {
		return "", &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content: %v", statErr)}
	}
	if !info.Mode().IsRegular() {
		return "", &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content %s: not a regular file", abs)}
	}
	body, readErr := os.ReadFile(abs)
	if readErr != nil {
		return "", &exitCodeError{code: 1, msg: fmt.Sprintf("read --edited-content: %v", readErr)}
	}
	return strings.TrimRight(string(body), "\n\r\t "), nil
}
