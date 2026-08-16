package govrun

// proposals.go — routing extraction-time PROPOSALS out of sidecars and into
// the approval ceremony's pending queue.
//
// Two transient sidecar fields carry unapproved inferences: `proposed_paths`
// and `proposed_topic_description`. The extractor writes them into the sidecar
// because the extractor is Write-only to its one target (INV-010) and cannot  edikt-guard:allow
// write a second file. Tier-2 routes them out, and strips them, so a steady-
// state sidecar never carries an unadjudicated guess.
//
// The routing rule that matters: a proposal NEVER becomes authoritative here.
// Nothing in this file writes .edikt/topics.yaml or a sidecar's `paths:`. It
// moves bytes into a queue a human has to act on, and `bin/edikt sidecar
// approve --kind …` is the only thing that promotes them. That separation is
// what makes "no description is ever silently invented" a structural property
// rather than a promise.
//
// It also OPENS SLOTS: every corpus topic with no approved registry entry and
// no pending proposal gets an empty-description pending file naming the
// artifacts that use the topic. An empty slot cannot be approved (the registry
// validator rejects an empty description), so the slot is a visible piece of
// outstanding work, never a path to a blank entry. This is what "seeded
// through the ceremony with visible pending status" means on upgrade: the
// backlog is enumerated and reported, and compile carries on.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
	"gopkg.in/yaml.v3"
)

// ProposalRoutingReport is what the routing step observed and did.
//
// Every field is a count the caller can print with a denominator. A routing
// step that reported nothing would be indistinguishable from one that never
// ran (INV-013).  edikt-guard:allow
type ProposalRoutingReport struct {
	PathsQueued            []string `json:"paths_queued,omitempty"`
	VerifiesQueued         []string `json:"verifies_queued,omitempty"`
	TopicDescriptionQueued []string `json:"topic_description_queued,omitempty"`
	TopicSlotsOpened       []string `json:"topic_slots_opened,omitempty"`
	SidecarsStripped       int      `json:"sidecars_stripped"`
}

// Empty reports whether the routing step had nothing to do. A no-subject run
// stays silent rather than announcing non-coverage of something that does not
// exist.
func (r ProposalRoutingReport) Empty() bool {
	return len(r.PathsQueued) == 0 && len(r.VerifiesQueued) == 0 &&
		len(r.TopicDescriptionQueued) == 0 && len(r.TopicSlotsOpened) == 0
}

// Report renders the one-line summary compile prints.
func (r ProposalRoutingReport) Report() string {
	parts := []string{}
	if n := len(r.PathsQueued); n > 0 {
		parts = append(parts, fmt.Sprintf("%d paths proposal(s) queued (%s)", n, strings.Join(r.PathsQueued, ", ")))
	}
	if n := len(r.VerifiesQueued); n > 0 {
		parts = append(parts, fmt.Sprintf("%d behavioral verify proposal(s) queued (%s)", n, strings.Join(r.VerifiesQueued, ", ")))
	}
	if n := len(r.TopicDescriptionQueued); n > 0 {
		parts = append(parts, fmt.Sprintf("%d extracted topic description(s) queued (%s)", n, strings.Join(r.TopicDescriptionQueued, ", ")))
	}
	if n := len(r.TopicSlotsOpened); n > 0 {
		parts = append(parts, fmt.Sprintf("%d topic(s) awaiting a description with none proposed (%s)", n, strings.Join(r.TopicSlotsOpened, ", ")))
	}
	return "Proposals — " + strings.Join(parts, "; ") +
		". Review with `bin/edikt sidecar approve --kind <verify|paths|topic-description> --list`."
}

// pendingPathsOut / pendingTopicOut mirror the shapes cmd/sidecar_approve_kinds.go
// decodes. They are written here and read there; the field names are the
// contract between the two halves of the ceremony.
type pendingPathsOut struct {
	ID            string                 `yaml:"id"`
	SidecarPath   string                 `yaml:"sidecar_path"`
	ProposedPaths []sidecar.ProposedPath `yaml:"proposed_paths"`
	ProposedAt    string                 `yaml:"proposed_at,omitempty"`
}

// pendingVerifyOut mirrors cmd/sidecar_approve.go's pendingVerify. A
// behavioral verify claims a rule is demonstrably enforced by RUNNING
// something, which is the strongest claim any field in a sidecar makes — so it
// is the one field the ceremony has always required a human to approve.
type pendingVerifyOut struct {
	ID                    string `yaml:"id"`
	SidecarPath           string `yaml:"sidecar_path"`
	DirectiveIndex        int    `yaml:"directive_index"`
	ProposedVerify        string `yaml:"proposed_verify"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	ProposedAt            string `yaml:"proposed_at,omitempty"`
}

type pendingTopicOut struct {
	Topic       string `yaml:"topic"`
	Description string `yaml:"description"`
	Evidence    string `yaml:"evidence,omitempty"`
	ProposedAt  string `yaml:"proposed_at,omitempty"`
}

// RouteProposals moves transient proposals out of the sidecar set into the
// pending queues, opens slots for undescribed topics, and strips the transient
// fields from disk.
//
// projectRoot anchors every path. now is injected so the caller controls the
// timestamp — a routing step that read the wall clock itself could not be
// tested for determinism.
func RouteProposals(projectRoot string, pairs []sidecar.Pair, registry topicregistry.Registry, now time.Time) (ProposalRoutingReport, error) {
	rep := ProposalRoutingReport{}
	stamp := now.UTC().Format(time.RFC3339)

	pathsDir := filepath.Join(projectRoot, ".edikt", "state", "pending-paths")
	topicsDir := filepath.Join(projectRoot, ".edikt", "state", "pending-topic-descriptions")
	verifiesDir := filepath.Join(projectRoot, ".edikt", "state", "pending-verifies")

	// Deterministic order so two runs over the same corpus queue in the same
	// sequence and the report reads the same.
	sorted := append([]sidecar.Pair(nil), pairs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ArtifactID < sorted[j].ArtifactID })

	corpusTopics := map[string][]string{} // topic -> contributing artifact ids

	for _, p := range sorted {
		sc := p.Sidecar
		if sc == nil {
			continue
		}
		if sc.Topic != "" {
			corpusTopics[sc.Topic] = append(corpusTopics[sc.Topic], p.ArtifactID)
		}

		dirty := false

		if len(sc.ProposedPaths) > 0 {
			if err := os.MkdirAll(pathsDir, 0o755); err != nil {
				return rep, fmt.Errorf("mkdir %s: %w", pathsDir, err)
			}
			rel := sc.SourcePath
			if r, relErr := filepath.Rel(projectRoot, sc.SourcePath); relErr == nil && !strings.HasPrefix(r, "..") {
				rel = filepath.ToSlash(r)
			}
			out := pendingPathsOut{
				ID:            p.ArtifactID,
				SidecarPath:   rel,
				ProposedPaths: sc.ProposedPaths,
				ProposedAt:    stamp,
			}
			if err := writeYAML(filepath.Join(pathsDir, p.ArtifactID+".yaml"), out); err != nil {
				return rep, err
			}
			rep.PathsQueued = append(rep.PathsQueued, p.ArtifactID)
			sc.ProposedPaths = nil
			dirty = true
		}

		// BEHAVIORAL VERIFIES ARE PROPOSALS, NOT OUTPUT.
		//
		// The extractor may write `verify:` directly, and for structural and
		// tooling kinds that is fine — they inspect the tree. A BEHAVIORAL
		// verify asserts the rule is demonstrably enforced by running
		// something, and Phase B refuses one with no positive fixture (Plan C).
		// Before this routing existed, a re-extraction that synthesised one
		// behavioral verify failed compile FOR THE WHOLE PROJECT, and the only
		// remedies were to hand-edit a generated file or to weaken the gate.
		//
		// So it goes where every other unapproved inference goes: the pending
		// queue, with the sidecar stripped of the claim. F-005's gate blocks on
		// a non-empty queue, so this cannot become a silent drop.
		//
		// AN APPROVED VERIFY IS NEVER TOUCHED. human_approved_at is the record
		// of the ceremony having happened; re-queueing it would ask a human to
		// re-approve their own decision on every compile, and stripping it
		// would destroy the pinned state AC-4.4 requires to survive
		// regeneration byte-intact.
		for i := range sc.Directives {
			d := &sc.Directives[i]
			if d.Verify == "" || d.VerifyKind != "behavioral" {
				continue
			}
			if d.HumanApprovedAt != "" || d.PositiveFixturePath != "" {
				continue
			}
			if err := os.MkdirAll(verifiesDir, 0o755); err != nil {
				return rep, fmt.Errorf("mkdir %s: %w", verifiesDir, err)
			}
			rel := sc.SourcePath
			if r, relErr := filepath.Rel(projectRoot, sc.SourcePath); relErr == nil && !strings.HasPrefix(r, "..") {
				rel = filepath.ToSlash(r)
			}
			pid := fmt.Sprintf("%s-d%02d", p.ArtifactID, i)
			dest := filepath.Join(verifiesDir, pid+".yaml")
			if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
				out := pendingVerifyOut{
					ID:                    pid,
					SidecarPath:           rel,
					DirectiveIndex:        i,
					ProposedVerify:        d.Verify,
					Intent:                d.Intent,
					FalsifyingObservation: d.FalsifyingObservation,
					ProposedAt:            stamp,
				}
				if err := writeYAML(dest, out); err != nil {
					return rep, err
				}
				rep.VerifiesQueued = append(rep.VerifiesQueued, pid)
			}
			d.Verify = ""
			d.VerifyKind = ""
			dirty = true
		}

		if d := strings.TrimSpace(sc.ProposedTopicDescription); d != "" && sc.Topic != "" {
			// An already-approved topic does not get its description
			// re-proposed. The registry is pinned judgment; re-queueing a
			// machine suggestion against an approved entry every compile is
			// the regenerate-per-compile failure the pin exists to stop.
			if _, approved := registry[sc.Topic]; !approved {
				if err := os.MkdirAll(topicsDir, 0o755); err != nil {
					return rep, fmt.Errorf("mkdir %s: %w", topicsDir, err)
				}
				dest := filepath.Join(topicsDir, sc.Topic+".yaml")
				// First proposal wins. Overwriting would let the last-compiled
				// artifact silently replace a proposal a human is mid-review on.
				if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
					out := pendingTopicOut{
						Topic:       sc.Topic,
						Description: sc.ProposedTopicDescription,
						Evidence:    fmt.Sprintf("proposed by extraction of %s", p.ArtifactID),
						ProposedAt:  stamp,
					}
					if err := writeYAML(dest, out); err != nil {
						return rep, err
					}
					rep.TopicDescriptionQueued = append(rep.TopicDescriptionQueued, sc.Topic)
				}
			}
			sc.ProposedTopicDescription = ""
			dirty = true
		} else if sc.ProposedTopicDescription != "" {
			sc.ProposedTopicDescription = ""
			dirty = true
		}

		if dirty {
			body, err := sidecar.Marshal(sc)
			if err != nil {
				return rep, fmt.Errorf("marshal %s: %w", p.ArtifactID, err)
			}
			if err := os.WriteFile(sc.SourcePath, body, 0o644); err != nil {
				return rep, fmt.Errorf("write %s: %w", sc.SourcePath, err)
			}
			rep.SidecarsStripped++
		}
	}

	// Open a slot for every corpus topic that is neither approved nor already
	// proposed. The slot carries NO description — an empty one cannot be
	// approved, so the only way out of the slot is a human supplying text.
	names := make([]string, 0, len(corpusTopics))
	for t := range corpusTopics {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		if _, approved := registry[t]; approved {
			continue
		}
		dest := filepath.Join(topicsDir, t+".yaml")
		if _, statErr := os.Stat(dest); statErr == nil {
			continue
		}
		if err := os.MkdirAll(topicsDir, 0o755); err != nil {
			return rep, fmt.Errorf("mkdir %s: %w", topicsDir, err)
		}
		sources := corpusTopics[t]
		sort.Strings(sources)
		out := pendingTopicOut{
			Topic:       t,
			Description: "",
			Evidence: fmt.Sprintf(
				"no description proposed; topic used by %d artifact(s): %s",
				len(sources), strings.Join(sources, ", ")),
			ProposedAt: stamp,
		}
		if err := writeYAML(dest, out); err != nil {
			return rep, err
		}
		rep.TopicSlotsOpened = append(rep.TopicSlotsOpened, t)
	}

	return rep, nil
}

func writeYAML(path string, v any) error {
	body, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// v1ShapedSidecars returns the artifact ids whose sidecar still carries the v1
// single-anchor shape, sorted.
//
// "v1-shaped" means: declares schema_version 1, or carries at least one item
// whose anchors came from the singular `source_excerpt` key. Both are the same
// migration target; a sidecar can carry a v2 version number and still hold a
// singular anchor, which Validate already rejects — this catches it before a
// dispatch rather than after.
func v1ShapedSidecars(pairs []sidecar.Pair) []string {
	var out []string
	for _, p := range pairs {
		sc := p.Sidecar
		if sc == nil {
			continue
		}
		legacy := sc.SchemaVersion < sidecar.SchemaVersionV2
		if !legacy {
			for _, d := range sc.Directives {
				if len(d.SourceExcerpts) == 0 && d.SourceExcerpt.Quote != "" {
					legacy = true
					break
				}
			}
		}
		if !legacy {
			for _, ph := range sc.Prohibitions {
				if len(ph.SourceExcerpts) == 0 && ph.SourceExcerpt.Quote != "" {
					legacy = true
					break
				}
			}
		}
		if legacy {
			out = append(out, p.ArtifactID)
		}
	}
	sort.Strings(out)
	return out
}

// countLoaded is the denominator for the pre-flight message: how many sidecars
// were actually examined. A count of legacy sidecars with no denominator
// cannot be read as progress toward zero.
func countLoaded(pairs []sidecar.Pair) int {
	n := 0
	for _, p := range pairs {
		if p.Sidecar != nil {
			n++
		}
	}
	return n
}
