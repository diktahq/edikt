package cmd

// prune_candidates.go — `bin/edikt gov prune-candidates --window N --json`.
// A deterministic recorder: reads what the synthesizer already wrote,
// computes a set difference, and removes nothing. commands/gov/score.md
// passes its output through unparsed rather than parsing it for
// control flow.
//
// Lives in package cmd, not cmd/gov, so its tests are reachable from
// `go test ./cmd/` — the exact package path this AC's own verify
// command names — while still nesting under `edikt gov` at runtime via
// gov.Cmd.AddCommand below (cmd already imports cmd/gov; the reverse
// import would cycle).
//
// WHAT IT ANSWERS: of the directives the compiled governance corpus
// carries (delivery-map.yaml — the one scope source), which ones no
// phase-end review has cited as FAIL/NEEDS_REVIEW across the active
// plan's last N post-flight reports. That is a removal CANDIDATE list,
// not a removal — the backfill acts on it, this command only reports.
//
// THE CARE POINT: a post-flight report written before cited_directives
// existed carries no such key at all. Reading that absence as "zero
// citations" would make every directive in the corpus look like an
// unused candidate the moment the window includes one old report — a
// false abundance of candidates manufactured by a schema gap, not by
// anything actually going uncited. So a report that lacks the key
// contributes NOTHING to the measured set (neither a citation nor a
// zero), and if EVERY report in the window lacks it, the whole run
// reports itself as unmeasured rather than emitting a candidate list
// that looks like evidence and isn't.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/cmd/gov"
	"github.com/diktahq/edikt/tools/edikt/internal/activeplan"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	pruneCandidatesWindow int
	pruneCandidatesJSON   bool
)

func init() {
	c := &cobra.Command{
		Use:   "prune-candidates",
		Short: "List compiled directives with zero citations across the active plan's recent post-flight reports",
		Long: `Reads the last --window post-flight reports for the active plan and lists
every compiled directive (delivery-map.yaml) with zero citations across
them, as a removal candidate. Never removes anything — the caller acts
on the list. Reports written before cited_directives existed count as
unmeasured, never as zero citations.`,
		Args: cobra.NoArgs,
		RunE: runPruneCandidates,
	}
	c.Flags().IntVar(&pruneCandidatesWindow, "window", 5, "how many of the active plan's most recent post-flight reports to scan")
	c.Flags().BoolVar(&pruneCandidatesJSON, "json", false, "emit machine-readable JSON instead of a table")
	gov.Cmd.AddCommand(c)
}

// pruneCandidatesReport is the command's whole output contract — the
// shape commands/gov/score.md passes through unparsed.
type pruneCandidatesReport struct {
	Window          int      `json:"window"`
	Plan            string   `json:"plan,omitempty"`
	ReportsScanned  int      `json:"reports_scanned"`
	ReportsMeasured int      `json:"reports_measured"`
	Measured        bool     `json:"measured"`
	Candidates      []string `json:"candidates"`
	Note            string   `json:"note,omitempty"`
}

// pruneCandidatesPlansConfig reads only what this command needs from
// .edikt/config.yaml — a single field, not the broader SDLC corpus
// config cmd/gov's cheat-rate discovery reads (specs/prds/plans
// together); duplicating one YAML key here is boilerplate, not the
// scoping RULE GL-002 cares about not duplicating.
type pruneCandidatesPlansConfig struct {
	Paths struct {
		Plans string `yaml:"plans"`
	} `yaml:"paths"`
}

func loadPruneCandidatesPlansConfig(projectRoot string) pruneCandidatesPlansConfig {
	var cfg pruneCandidatesPlansConfig
	raw, err := os.ReadFile(filepath.Join(projectRoot, ".edikt", "config.yaml"))
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(raw, &cfg)
	return cfg
}

func runPruneCandidates(cmd *cobra.Command, args []string) error {
	if pruneCandidatesWindow <= 0 {
		return &exitCodeError{code: 3, msg: "prune-candidates: --window must be a positive integer"}
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("prune-candidates: getwd: %v", err)}
	}

	cfg := loadPruneCandidatesPlansConfig(projectRoot)
	if cfg.Paths.Plans == "" {
		return emitPruneCandidates(cmd, pruneCandidatesReport{
			Window: pruneCandidatesWindow, Measured: false,
			Note: "no paths.plans configured — nothing to scan",
		})
	}
	plansDir := filepath.Join(projectRoot, cfg.Paths.Plans)

	activePlan, err := activeplan.Resolve(plansDir)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("prune-candidates: resolve active plan: %v", err)}
	}
	if activePlan == "" {
		return emitPruneCandidates(cmd, pruneCandidatesReport{
			Window: pruneCandidatesWindow, Measured: false,
			Note: "no active plan — nothing to scan",
		})
	}
	slug := activeplan.Slug(activePlan)

	reportsDir := filepath.Join(projectRoot, ".edikt", "state", "post-flight")
	reportPaths, err := recentPostFlightReports(reportsDir, slug, pruneCandidatesWindow)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("prune-candidates: list reports: %v", err)}
	}

	cited := map[string]struct{}{}
	measured := 0
	for _, p := range reportPaths {
		ids, ok, rerr := readCitedDirectives(p)
		if rerr != nil {
			// A malformed report is a data problem for doctor to surface
			// separately, not a reason to abort a soft-signal command.
			continue
		}
		if !ok {
			continue // no cited_directives key — this report is unmeasured, not zero
		}
		measured++
		for _, id := range ids {
			mapped, verr := directiveIDToDeliveryMapID(id)
			if verr != nil {
				continue // an invalid id from an LLM-written report is skipped, never displayed
			}
			cited[mapped] = struct{}{}
		}
	}

	report := pruneCandidatesReport{
		Window:          pruneCandidatesWindow,
		Plan:            slug,
		ReportsScanned:  len(reportPaths),
		ReportsMeasured: measured,
		Measured:        measured > 0,
	}
	if measured == 0 {
		report.Note = "no post-flight report in the window carries cited_directives yet — unmeasured, not zero"
		return emitPruneCandidates(cmd, report)
	}

	universe, err := compiledDirectiveUniverse(projectRoot)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("prune-candidates: read delivery map: %v", err)}
	}

	var candidates []string
	for id := range universe {
		if _, ok := cited[id]; !ok {
			candidates = append(candidates, id)
		}
	}
	sort.Strings(candidates)
	report.Candidates = candidates

	return emitPruneCandidates(cmd, report)
}

func emitPruneCandidates(cmd *cobra.Command, r pruneCandidatesReport) error {
	if pruneCandidatesJSON {
		raw, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return &exitCodeError{code: 1, msg: fmt.Sprintf("prune-candidates: marshal report: %v", err)}
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}
	if !r.Measured {
		fmt.Fprintf(cmd.OutOrStdout(), "UNMEASURED (window=%d): %s\n", r.Window, r.Note)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "plan: %s | window: %d | reports scanned: %d, measured: %d\n",
		r.Plan, r.Window, r.ReportsScanned, r.ReportsMeasured)
	if len(r.Candidates) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no zero-citation candidates")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%d zero-citation candidate(s):\n", len(r.Candidates))
	for _, c := range r.Candidates {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", c)
	}
	return nil
}

// postFlightReportNameRe matches <slug>-<phase>-<ts>.json, anchored on
// the timestamp's fixed shape (post-flight.md's `date -u
// +%Y%m%dT%H%M%SZ`) so a slug containing digits or hyphens cannot be
// mis-split.
var postFlightReportNameRe = regexp.MustCompile(`^(.+)-(\d+)-(\d{8}T\d{6}Z)\.json$`)

// recentPostFlightReports returns up to n paths under dir whose
// filename matches the given plan slug, most recent (by embedded
// timestamp) first.
func recentPostFlightReports(dir, slug string, n int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type cand struct {
		path string
		ts   string
	}
	var found []cand
	prefix := slug + "-"
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		m := postFlightReportNameRe.FindStringSubmatch(name)
		if m == nil || m[1] != slug {
			continue
		}
		found = append(found, cand{filepath.Join(dir, name), m[3]})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ts > found[j].ts })
	if len(found) > n {
		found = found[:n]
	}
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.path
	}
	return out, nil
}

// postFlightCitedShape decodes just enough of a post-flight report to
// answer "did this report measure citations, and if so, which ids".
// CitedDirectives is a pointer so encoding/json distinguishes an
// absent key (nil) from a present-but-empty array (non-nil, len 0).
type postFlightCitedShape struct {
	CitedDirectives *[]string `json:"cited_directives"`
}

// readCitedDirectives returns (ids, true, nil) when the report carries
// cited_directives at all (even empty), (nil, false, nil) when the key
// is absent (unmeasured), and (nil, false, err) on a read/parse error.
func readCitedDirectives(path string) ([]string, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var shape postFlightCitedShape
	if err := json.Unmarshal(raw, &shape); err != nil {
		return nil, false, err
	}
	if shape.CitedDirectives == nil {
		return nil, false, nil
	}
	return *shape.CitedDirectives, true, nil
}

// directiveIDRe matches governance-verifier-verdict.schema.json's
// directive_id shape: an ADR/INV-anchored or topic-anchored directive
// index. Mirrors that schema's own pattern rather than a new one.
var directiveIDRe = regexp.MustCompile(`^((?:ADR|INV)-\d{3,}|[a-z][a-z0-9-]{0,79})\.directive\[(\d+)\]$`)

// directiveIDToDeliveryMapID converts a governance-verifier directive_id
// (e.g. "ADR-900.directive[0]") into delivery-map.yaml's id form
// (e.g. "ADR-900:d01") — phaseb.BuildDeliveryMap's entryID(artifactID,
// 'd', i+1) applied to the same (artifactID, zero-based index) pair a
// directive_id already encodes. Returns an error for a malformed or
// out-of-shape id, and for an artifact id that fails idvalidate.ArtifactID
// — an LLM-written report's id is never trusted into a display string
// or a lookup key without that check.
func directiveIDToDeliveryMapID(directiveID string) (string, error) {
	m := directiveIDRe.FindStringSubmatch(directiveID)
	if m == nil {
		return "", fmt.Errorf("directive_id %q does not match the expected shape", directiveID)
	}
	artifactID, idxStr := m[1], m[2]
	if err := idvalidate.ArtifactID(artifactID); err != nil {
		return "", err
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:d%02d", artifactID, idx+1), nil
}

// deliveryMapForUniverse mirrors hookmatch's own unexported
// deliveryMapFile shape (delivery-map.yaml's on-disk contract) — a
// second reader of the same file format for a different question
// (every distinct directive id across the whole map, not "what
// governs path X") rather than a second copy of hookmatch's own path
// resolution logic, which this still calls.
type deliveryMapForUniverse struct {
	Globs map[string][]struct {
		ID string `yaml:"id"`
	} `yaml:"globs"`
}

// deliveryMapDirectiveIDRe matches only 'd'-kind (directive) entries in
// delivery-map.yaml, e.g. "ADR-900:d01" — excluding 'p'-kind
// (prohibition) entries such as "ADR-900:p01". directive_id (the L2
// verdict field this whole command exists to cross-reference) only
// ever names directives, never prohibitions, so the universe this
// command scores against must be scoped the same way.
var deliveryMapDirectiveIDRe = regexp.MustCompile(`:d\d{2,}$`)

// compiledDirectiveUniverse returns every distinct directive (not
// prohibition) id in delivery-map.yaml, deduplicated across the many
// path globs a single directive can appear under.
func compiledDirectiveUniverse(projectRoot string) (map[string]struct{}, error) {
	path := hookmatch.DeliveryMapPath(projectRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]struct{}{}, nil
		}
		return nil, err
	}
	var m deliveryMapForUniverse
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := map[string]struct{}{}
	for _, entries := range m.Globs {
		for _, e := range entries {
			if deliveryMapDirectiveIDRe.MatchString(e.ID) {
				out[e.ID] = struct{}{}
			}
		}
	}
	return out, nil
}
