package cmd

// doctor_delivery_gaps.go — four ways a compile succeeds while delivering less
// than it appears to.
//
// All four were found on 2026-08-29 in edikt's own corpus or in fixtures, all
// four are cheap to detect, and none of them warned anybody. They are states,
// not style, which is why they live in `doctor` and not in a linter: nothing is
// wrong with how anything is WRITTEN, and a compile that reports success is
// telling the truth about what it did. The gap is between what it did and what
// the author believed it did.
//
// The shared shape is a surface that LOOKS healthy. Topic files render, the
// compile exits zero, doctor is quiet — and the always-loaded core is empty, or
// the write-time index routes nothing, or an accepted artifact governs nothing
// at all. Every one of these is invisible from the output of the thing that
// caused it.
//
// EACH CHECK NAMES THE CONSEQUENCE, NOT ONLY THE CONDITION. "3 proposals
// queued" is a fact a reader can skip; "until approved, `paths:` stays empty,
// so these directives do not scope and do not route" is the reason to act. The
// checks that fired all day and were ignored were the ones that reported a
// count.

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/parse"
)

// governedArtifactRe matches a governed artifact filename. assumptions.md and
// evaluator-tuning.md are prose in the same directory and are not artifacts;
// requiring a sidecar of them would be a false finding.
var governedArtifactRe = regexp.MustCompile(`^(ADR|INV|GUIDE)-`)

// directiveBulletRe matches a rendered ambient directive line.
var directiveBulletRe = regexp.MustCompile(`(?m)^- .*\(ref:\s*[A-Za-z]+-[A-Za-z0-9-]*\d+\)\s*$`)

// deliveryGap is one finding. Consequence is what the user loses, in their
// terms, and is never omitted.
//
// Remedy is the command that closes the gap. It is separate from Consequence
// because the two answer different questions — what you lose, and what you
// type — and a finding that names only the first is one a reader can agree
// with and still not act on.
//
// Fatal marks a gap doctor reports as an ERROR rather than a warning. Only
// one is fatal today: an ACCEPTED artifact with no sidecar. The others
// describe a surface that under-delivers; this one describes a record the
// project has already committed to which governs nothing at all, and the
// project believes it is governed. That is a false belief about the state of
// enforcement, which is the class INV-013 exists for, so it is RED.
type deliveryGap struct {
	Title       string
	Detail      []string
	Consequence string
	Remedy      string
	Fatal       bool
}

// runDeliveryGapsCheck reports compiles that succeeded while delivering less
// than they appear to. ran=false when there is no compiled surface to judge —
// an unmeasured project is not a healthy one, and saying nothing is correct
// only when there is genuinely nothing to say (INV-013).
func runDeliveryGapsCheck(projectRoot string, w io.Writer) (errs, warns int, ran bool) {
	// LoadConfig returns usable DEFAULTS for a directory with no edikt config,
	// so it cannot answer "is this an edikt project". Without this stat the
	// check reports "ok" for any directory on disk — a healthy verdict on a
	// project it never looked at, which is the false-pass INV-013 forbids.
	if _, err := os.Stat(filepath.Join(projectRoot, ".edikt", "config.yaml")); err != nil {
		return 0, 0, false
	}
	cfg, err := govrun.LoadConfig(projectRoot)
	if err != nil {
		return 0, 0, false
	}

	var gaps []deliveryGap
	gaps = append(gaps, emptyDeliverySurface(projectRoot)...)
	gaps = append(gaps, standingProposalQueue(projectRoot)...)
	gaps = append(gaps, artifactsWithoutSidecar(projectRoot, cfg)...)
	gaps = append(gaps, invariantsExcludedByStatus(projectRoot, cfg)...)
	gaps = append(gaps, strandedProposals(projectRoot, cfg)...)

	fmt.Fprintln(w, "  ── Delivery Gaps ──────────────────────────────")
	if len(gaps) == 0 {
		fmt.Fprintln(w, "    ok   compiled surface delivers what it declares")
		return 0, 0, true
	}
	for _, g := range gaps {
		if g.Fatal {
			fmt.Fprintf(w, "    🔴 ERROR: %s\n", g.Title)
			errs++
		} else {
			fmt.Fprintf(w, "    warn %s\n", g.Title)
			warns++
		}
		for _, d := range g.Detail {
			fmt.Fprintf(w, "           %s\n", d)
		}
		fmt.Fprintf(w, "           → %s\n", g.Consequence)
		if g.Remedy != "" {
			fmt.Fprintf(w, "           run: %s\n", g.Remedy)
		}
	}
	return errs, warns, true
}

// emptyDeliverySurface — the compiled surface exists and carries nothing.
//
// golden-repo compiled to an always-loaded governance.md with ZERO directives
// and a directive-index with zero globs, while its topic files looked healthy.
// Both halves are checked because they fail independently: a corpus can be
// fully ambient (index empty, legitimately, as v0.4.5 was) or fully scoped
// (ambient thin). Reporting both, with which one is empty, keeps a legitimate
// shape from reading as a defect.
func emptyDeliverySurface(root string) []deliveryGap {
	ambient := filepath.Join(root, ".claude", "rules", "governance.md")
	index := filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml")

	ab, aerr := os.ReadFile(ambient)
	ib, ierr := os.ReadFile(index)
	if aerr != nil && ierr != nil {
		return nil // nothing compiled here; not this check's business
	}

	var out []deliveryGap
	if aerr == nil && len(directiveBulletRe.FindAll(ab, -1)) == 0 {
		out = append(out, deliveryGap{
			Title:       "ambient core carries no directives",
			Detail:      []string{ambient},
			Consequence: "the surface loaded on EVERY edit is empty — no rule reaches the model unconditionally",
		})
	}
	if ierr == nil && countIndexGlobs(ib) == 0 {
		out = append(out, deliveryGap{
			Title:       "delivery map has no globs",
			Detail:      []string{index},
			Consequence: "no directive routes to any path, so no topic file loads when a governed file is touched",
		})
	}
	return out
}

// countIndexGlobs counts keys under the index's `globs:` mapping without
// pulling in a YAML dependency for a shape this simple. A glob key is the only
// two-space-indented key under that block.
func countIndexGlobs(b []byte) int {
	var in bool
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "globs:") {
			in = true
			continue
		}
		if !in {
			continue
		}
		if len(line) > 0 && !strings.HasPrefix(line, " ") {
			break // left the block
		}
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "-") {
			continue
		}
		if strings.HasSuffix(t, ":") && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			n++
		}
	}
	return n
}

// standingProposalQueue — proposals queued and never approved.
//
// PARTIALLY HANDLED ALREADY, and the residue is the point. compile prints
// "N paths proposal(s) queued" via govrun.ProposalRoutingReport.Report(), but
// only in the run that QUEUES them: Report is built from that run's own lists
// and Empty() makes a later compile silent while the queue still stands. It
// never affects the exit code. So a project can compile green forever on a
// standing queue, which is the state meterly sat in. This check reports the
// STANDING condition, which no compile does.
// strandedProposals reports unadjudicated proposals on artifacts the compile
// EXCLUDES, distinguishing the two cases — which look identical in a sidecar
// and are not the same thing at all.
//
// RouteProposals moves `proposed_paths` and `proposed_topic_description` into
// the pending queues and strips them. It only ever sees artifacts in `pairs`,
// and discover.go keeps superseded and non-accepted artifacts out of that set.
// So an excluded artifact keeps its proposals indefinitely, and because they
// never reach a queue, standingProposalQueue cannot see them either. The
// proposal is invisible to every check edikt has.
//
// Found 2026-08-30 on ADR-002. The finding that surfaced it (d03e5ad7) said
// "proposals.go should have routed it out and stripped it" — WRONG, and worth
// correcting rather than quietly fixing: proposals.go behaved exactly as
// designed and never saw the artifact. The defect is that nothing reports the
// consequence.
//
// THE TWO CASES:
//
//   - superseded / deprecated — the artifact will never be authoritative again,
//     so the proposal can never be adjudicated. Permanently stranded, and
//     indistinguishable from a live one when read.
//   - proposed / draft — excluded only until accepted, at which point the next
//     compile routes it normally. Waiting, not stranded, and reported as such
//     so a routine state is not dressed up as a defect.
func strandedProposals(root string, cfg govrun.Config) []deliveryGap {
	var stranded, waiting []string
	for _, d := range govDirs(cfg) {
		ents, err := os.ReadDir(filepath.Join(root, d))
		if err != nil {
			continue
		}
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".md") || !governedArtifactRe.MatchString(n) {
				continue
			}
			md := filepath.Join(root, d, n)
			doc, err := parse.LoadDocument(md)
			if err != nil || doc.IsIncluded(kindForDir(d)) {
				continue // included artifacts get routed on the next compile
			}
			raw, err := os.ReadFile(strings.TrimSuffix(md, ".md") + ".edikt.yaml")
			if err != nil {
				continue
			}
			var sc struct {
				ProposedPaths []struct {
					Glob string `yaml:"glob"`
				} `yaml:"proposed_paths"`
				ProposedTopicDescription string `yaml:"proposed_topic_description"`
			}
			if yaml.Unmarshal(raw, &sc) != nil {
				continue
			}
			if len(sc.ProposedPaths) == 0 && sc.ProposedTopicDescription == "" {
				continue
			}
			// artifact_id.go is the ONE extractor. Hand-rolling a trim here
			// produced "ADR-" instead of "ADR-002" on the first live run.
			id, ok := artifactIDFrom(n)
			if !ok {
				continue
			}
			if terminalStatusRe.MatchString(string(mustHead(md))) {
				stranded = append(stranded, id)
			} else {
				waiting = append(waiting, id)
			}
		}
	}
	var out []deliveryGap
	if len(stranded) > 0 {
		sort.Strings(stranded)
		out = append(out, deliveryGap{
			Title:  fmt.Sprintf("%d superseded artifact(s) hold a proposal that can never be adjudicated", len(stranded)),
			Detail: []string{strings.Join(stranded, ", ")},
			Consequence: "the artifact is excluded from compile, so the proposal never reaches a " +
				"pending queue and no other check can see it; it reads as a live proposal and is not",
		})
	}
	if len(waiting) > 0 {
		sort.Strings(waiting)
		out = append(out, deliveryGap{
			Title:  fmt.Sprintf("%d not-yet-accepted artifact(s) hold a proposal awaiting acceptance", len(waiting)),
			Detail: []string{strings.Join(waiting, ", ")},
			Consequence: "routine — the next compile after acceptance routes these normally; " +
				"listed so they are not mistaken for the stranded case above",
		})
	}
	return out
}

// terminalStatusRe matches a status an artifact never returns from. Kept
// separate from discover.go's own skip rule deliberately: that one decides
// what compile READS, this one decides what a human is TOLD, and collapsing
// them would make a reporting change silently alter compilation.
var terminalStatusRe = regexp.MustCompile(`(?im)^\*\*Status:\*\*\s*(superseded|deprecated|retired)`)

// mustHead reads the first 4 KiB of a file, or nothing. A status line lives in
// the first few lines; reading the whole document to find it would make doctor
// proportional to corpus size for no gain.
func mustHead(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	return buf[:n]
}

func standingProposalQueue(root string) []deliveryGap {
	type q struct{ dir, what, consequence string }
	queues := []q{
		{"pending-paths", "path-scope",
			"until approved, `paths:` stays empty — those directives do not scope a topic file and do not route in the delivery map"},
		{"pending-verifies", "behavioral verify",
			"until approved, the directive compiles without its verify, so nothing measures whether the rule holds"},
		// CORRECTED 2026-08-30. This previously read "the topic renders with
		// no description". It renders WITH one: phaseb.RenderSkill emits a
		// placeholder naming the approve command, into `description:` — the
		// field the host matches a task against. Present-and-misleading, not
		// absent. The compiled-surface check reports the rendered result;
		// this one reports the outstanding work.
		{"pending-topic-descriptions", "topic description",
			"until approved, the skill's `description:` carries an approval instruction " +
				"instead of what the topic covers, so the host routes on a placeholder"},
	}
	var out []deliveryGap
	for _, qq := range queues {
		dir := filepath.Join(root, ".edikt", "state", qq.dir)
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
			}
		}
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		title := fmt.Sprintf("%d %s proposal(s) awaiting approval", len(names), qq.what)
		// The oldest proposal's age, from its proposed_at (D51 (b)): a queue
		// that has stood for a week is a different finding from one written
		// this morning, and two of a consumer's had stood since 2026-08-27
		// with nothing saying so.
		if age, ok := oldestProposalAge(dir, names); ok {
			title += fmt.Sprintf(", oldest %s", age)
		}
		out = append(out, deliveryGap{
			Title:       title,
			Detail:      []string{strings.Join(names, ", "), "run: edikt sidecar approve --list"},
			Consequence: qq.consequence,
		})
	}
	return out
}

// artifactsWithoutSidecar — an artifact that compiles to nothing.
//
// Only artifacts the status filter INCLUDES are reported. A superseded,
// deprecated or proposed artifact legitimately has no sidecar, and reporting
// those would bury the real finding in eleven false ones — which is exactly the
// ratio in edikt's own corpus, where twelve artifacts lack a sidecar and only
// ONE of them is accepted.
func artifactsWithoutSidecar(root string, cfg govrun.Config) []deliveryGap {
	var missing []string
	for _, d := range govDirs(cfg) {
		abs := filepath.Join(root, d)
		ents, err := os.ReadDir(abs)
		if err != nil {
			continue
		}
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".md") || !governedArtifactRe.MatchString(n) {
				continue
			}
			p := filepath.Join(abs, n)
			doc, err := parse.LoadDocument(p)
			if err != nil || !doc.IsIncluded(kindForDir(d)) {
				continue
			}
			if _, err := os.Stat(strings.TrimSuffix(p, ".md") + ".edikt.yaml"); os.IsNotExist(err) {
				missing = append(missing, filepath.Join(d, n))
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []deliveryGap{{
		Title:       fmt.Sprintf("%d accepted artifact(s) have no sidecar", len(missing)),
		Detail:      missing,
		Consequence: "an artifact with no sidecar contributes NO directives to any surface — it is accepted and governs nothing",
		Remedy:      compileRemedyFor(missing),
		Fatal:       true,
	}}
}

// compileRemedyFor names the compile command for each KIND present in the
// list, so the remedy is the command the reader actually types rather than a
// menu of three they have to choose between. The sidecar is written by the
// locked extractor agent the per-kind command dispatches — `gov compile`
// merges existing sidecars and would report success while writing none, so it
// is deliberately not the remedy named here.
func compileRemedyFor(missing []string) string {
	var cmds []string
	seen := map[string]bool{}
	for _, m := range missing {
		base := filepath.Base(m)
		var c string
		switch {
		case strings.HasPrefix(base, "ADR-"):
			c = "/edikt:adr:compile"
		case strings.HasPrefix(base, "INV-"):
			c = "/edikt:invariant:compile"
		case strings.HasPrefix(base, "GUIDE-"):
			c = "/edikt:guideline:compile"
		default:
			continue
		}
		if !seen[c] {
			seen[c] = true
			cmds = append(cmds, c)
		}
	}
	if len(cmds) == 0 {
		return ""
	}
	return strings.Join(cmds, " / ") + " — one per artifact listed above"
}

// invariantsExcludedByStatus — an invariant dropped for its status word.
//
// parse.IsIncluded admits an invariant only when its status is `active` or
// absent. `accepted` — the word every ADR uses, and the obvious thing to write —
// matches neither branch and the artifact is dropped. There is a skip line at
// compile, so this is not silent; it is easy to miss, and a skip line is not a
// check. The status is named so a deliberate `revoked` reads differently from
// an accidental `accepted`; doctor cannot know intent and does not guess.
func invariantsExcludedByStatus(root string, cfg govrun.Config) []deliveryGap {
	dir := cfg.Paths.Invariants
	if dir == "" {
		return nil
	}
	abs := filepath.Join(root, dir)
	ents, err := os.ReadDir(abs)
	if err != nil {
		return nil
	}
	var excluded []string
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".md") || !strings.HasPrefix(n, "INV-") {
			continue
		}
		doc, err := parse.LoadDocument(filepath.Join(abs, n))
		if err != nil || doc.IsIncluded("inv") {
			continue
		}
		st := strings.TrimSpace(doc.Frontmatter.Status)
		if st == "" {
			st = strings.TrimSpace(doc.BoldStatus)
		}
		excluded = append(excluded, fmt.Sprintf("%s (status: %q)", n, st))
	}
	if len(excluded) == 0 {
		return nil
	}
	sort.Strings(excluded)
	return []deliveryGap{{
		Title:       fmt.Sprintf("%d invariant(s) excluded by status", len(excluded)),
		Detail:      excluded,
		Consequence: "only `active` or no status is included; anything else — including `accepted` — drops the invariant from every surface",
	}}
}

func govDirs(cfg govrun.Config) []string {
	var out []string
	for _, d := range []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines} {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

func kindForDir(dir string) string {
	switch {
	case strings.Contains(dir, "invariant"):
		return "inv"
	case strings.Contains(dir, "guideline"):
		return "guideline"
	default:
		return "adr"
	}
}

// oldestProposalAge reads each proposal's proposed_at and returns the oldest
// as "N d" (or "N h" under a day). ok=false when no file carries a parseable
// stamp — the title then says nothing about age rather than "0 d" (INV-013).
func oldestProposalAge(dir string, names []string) (string, bool) {
	var oldest time.Time
	for _, n := range names {
		b := mustHead(filepath.Join(dir, n+".yaml"))
		m := proposedAtRe.FindSubmatch(b)
		if m == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, strings.Trim(string(m[1]), "\"'"))
		if err != nil {
			continue
		}
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	if oldest.IsZero() {
		return "", false
	}
	d := time.Since(oldest)
	if d < 24*time.Hour {
		return fmt.Sprintf("%d h", int(d.Hours())), true
	}
	return fmt.Sprintf("%d d", int(d.Hours()/24)), true
}

var proposedAtRe = regexp.MustCompile(`(?m)^proposed_at:\s*(\S+)`)
