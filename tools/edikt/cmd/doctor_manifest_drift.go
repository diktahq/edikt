package cmd

// doctor_manifest_drift.go — the detection half of upgrade-manifest reconciliation.
//
// `edikt upgrade` reconciles each managed surface against the PROJECT'S current
// contents rather than the VERSION'S manifest, so two of the three directions
// are unreachable from its loops:
//
//	present + stale      -> update    works everywhere
//	declared + absent    -> install   agents and rule packs iterate installed
//	                                  files, so an artifact the project lacks is
//	                                  never visited and never reported
//	present + retired    -> remove    hooks implement it; the guard requires the
//	                                  retired script's FILE to be gone, and
//	                                  nothing ever deletes retired hook scripts
//
// Measured on a real 0.8.0 upgrade: five agents declared `always:` were absent
// afterwards, including sidecar-extractor, which 14 command files dispatch and
// whose absence makes that dispatch resolve to nothing with no error. All five
// are Task-dispatched via subagent_type:, which resolves only from
// .claude/agents/ — which is why `always:` presence is the right subject set
// and evaluator-headless (install-root resolved, never Task-dispatched) is
// correctly not in it.
//
// WHY THIS IS NOT PART OF UPGRADE. A drift check that only runs during an
// upgrade cannot tell you that your LAST upgrade was incomplete — which is
// exactly the state a user is in when this fires. It reads the manifests and
// the project directly.
//
// Every line carries the manifest's count as its denominator. "0 missing" is
// only meaningful beside "of 9 declared"; without it, a surface that was never
// examined is indistinguishable from one that is complete.
//
// NOT A DUPLICATE OF checkAgentDrift. That check walks templates and compares
// CONTENT of copies that exist; its doc says outright "Missing copies are fine
// — project installs are optional and per-repo", and it skips a missing file.
// That is true for stack-conditional agents and false for the `always:` list,
// whose members tier-1 commands dispatch by name and whose absence makes the
// dispatch resolve to nothing. This check covers PRESENCE against `always:`
// only; content drift remains checkAgentDrift's.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// surfaceDrift is one managed surface's reconciliation state.
type surfaceDrift struct {
	Name      string
	Declared  int      // manifest count — the denominator
	Missing   []string // declared by the version, absent from the project
	Retired   []string // present in the project, no longer shipped
	Inspected bool     // false when the manifest could not be read
	Reason    string   // why not inspected
	OptedOut  bool     // surface is optional and this project installs none of it
}

// declaredAgents reads the agent registry's `always:` list — the agents every
// project gets regardless of stack. Stack-conditional entries are deliberately
// excluded: whether a project should have the Go agent depends on the project,
// but `always:` is unconditional and is what a version guarantees.
func declaredAgents(templatesRoot string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(templatesRoot, "agents", "_registry.yaml"))
	if err != nil {
		return nil, err
	}
	var reg map[string][]string
	if err := yaml.Unmarshal(b, &reg); err != nil {
		return nil, err
	}
	out := append([]string(nil), reg["always"]...)
	sort.Strings(out)
	return out, nil
}

// declaredRulePacks reads the rule registry's default packs.
func declaredRulePacks(templatesRoot string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(templatesRoot, "rules", "_registry.yaml"))
	if err != nil {
		return nil, err
	}
	var reg map[string]struct {
		Default bool `yaml:"default"`
	}
	if err := yaml.Unmarshal(b, &reg); err != nil {
		return nil, err
	}
	var out []string
	for name, e := range reg {
		if e.Default {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// installedStems lists the base names of *.md files in dir, extension stripped.
// A missing directory yields an empty set and no error: a project that never
// installed a surface has zero of it, which is a real measurement, not a
// failure to measure.
func installedStems(dir string) (map[string]bool, error) {
	out := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out[strings.TrimSuffix(e.Name(), ".md")] = true
	}
	return out, nil
}

// driftFor reconciles one surface: what the manifest declares against what the
// project has, in both non-update directions.
//
// Undeclared project artifacts are only reported when no template ships for
// them. A user's own agent is not a retired one, and ownership must be provable
// before anything is called edikt's.
func driftFor(name string, declared []string, installed map[string]bool, templateExists func(string) bool) surfaceDrift {
	d := surfaceDrift{Name: name, Declared: len(declared), Inspected: true}
	declaredSet := map[string]bool{}
	for _, s := range declared {
		declaredSet[s] = true
		if !installed[s] {
			d.Missing = append(d.Missing, s)
		}
	}
	for s := range installed {
		if declaredSet[s] {
			continue
		}
		if !templateExists(s) {
			d.Retired = append(d.Retired, s)
		}
	}
	sort.Strings(d.Missing)
	sort.Strings(d.Retired)
	return d
}

// runManifestDriftCheck reports per-surface reconciliation drift. Returns
// (warnings, ran).
func runManifestDriftCheck(projectRoot, templatesRoot string, w io.Writer) (warns int, ran bool) {
	if templatesRoot == "" {
		return 0, false
	}
	if _, err := os.Stat(templatesRoot); err != nil {
		return 0, false
	}

	var surfaces []surfaceDrift

	if declared, err := declaredAgents(templatesRoot); err != nil {
		surfaces = append(surfaces, surfaceDrift{Name: "agents", Reason: "agent registry unreadable: " + err.Error()})
	} else {
		installed, ierr := installedStems(filepath.Join(projectRoot, ".claude", "agents"))
		if ierr != nil {
			surfaces = append(surfaces, surfaceDrift{Name: "agents", Reason: ".claude/agents unreadable"})
		} else {
			surfaces = append(surfaces, driftFor("agents", declared, installed, func(s string) bool {
				_, e := os.Stat(filepath.Join(templatesRoot, "agents", s+".md"))
				return e == nil
			}))
		}
	}

	if declared, err := declaredRulePacks(templatesRoot); err != nil {
		surfaces = append(surfaces, surfaceDrift{Name: "rule packs", Reason: "rule registry unreadable: " + err.Error()})
	} else {
		installed, ierr := installedStems(filepath.Join(projectRoot, ".claude", "rules"))
		if ierr != nil {
			surfaces = append(surfaces, surfaceDrift{Name: "rule packs", Reason: ".claude/rules unreadable"})
		} else if !anyInstalled(declared, installed) {
			// RULE PACKS ARE OPTIONAL, AND .claude/rules/ IS SHARED.
			//
			// A project that compiles governance keeps governance.md and
			// governance/ in this same directory and installs no packs at all.
			// Reporting every declared pack as "missing" there would fire four
			// warnings on every run of a correctly configured project, and a
			// check that cries wolf is one people learn to skip.
			//
			// This mirrors upgrade.md §2e's own posture: nothing installed is
			// an opt-out, not drift. PARTIAL installation is still reported —
			// that is the state where one pack went missing and the rest did
			// not.
			surfaces = append(surfaces, surfaceDrift{
				Name: "rule packs", Declared: len(declared), Inspected: true, OptedOut: true,
			})
		} else {
			surfaces = append(surfaces, driftFor("rule packs", declared, installed, func(s string) bool {
				for _, tier := range []string{"base", "lang", "framework"} {
					if _, e := os.Stat(filepath.Join(templatesRoot, "rules", tier, s+".md")); e == nil {
						return true
					}
				}
				return false
			}))
		}
	}

	if len(surfaces) == 0 {
		return 0, false
	}

	fmt.Fprintln(w, "  ── Upgrade Manifest Drift ─────────────────────")

	for _, s := range surfaces {
		if !s.Inspected {
			// A surface that could not be read was not measured, and must
			// never be folded into a clean result.
			fmt.Fprintf(w, "  WARN: %s — UNMEASURED (%s)\n", s.Name, s.Reason)
			warns++
			continue
		}
		if s.OptedOut {
			fmt.Fprintf(w, "  OK: %s — none of the %d declared are installed; this project does not use them\n", s.Name, s.Declared)
			continue
		}
		if len(s.Missing) == 0 && len(s.Retired) == 0 {
			fmt.Fprintf(w, "  OK: %s — %d of %d declared present, none retired\n", s.Name, s.Declared, s.Declared)
			continue
		}
		if len(s.Missing) > 0 {
			fmt.Fprintf(w, "  WARN: %s — %d of %d declared by this version are NOT installed:\n", s.Name, len(s.Missing), s.Declared)
			for _, m := range s.Missing {
				fmt.Fprintf(w, "      missing: %s\n", m)
			}
			warns += len(s.Missing)
		}
		if len(s.Retired) > 0 {
			fmt.Fprintf(w, "  WARN: %s — %d installed that this version no longer ships:\n", s.Name, len(s.Retired))
			for _, r := range s.Retired {
				fmt.Fprintf(w, "      retired: %s\n", r)
			}
			warns += len(s.Retired)
		}
	}

	if warns > 0 {
		fmt.Fprintln(w, "    `edikt upgrade` reconciles against what your project already has, so an")
		fmt.Fprintln(w, "    artifact this version declares but your project lacks is never visited.")
		fmt.Fprintln(w, "    Re-run the installer for this version, or install the named files by hand.")
		fmt.Fprintln(w, "    A missing agent makes its dispatch resolve to nothing, with no error.")
	}
	return warns, true
}

// anyInstalled reports whether the project has at least one of the declared
// artifacts. It is the signal that separates "opted out of this surface
// entirely" from "has it, and is missing some of it" — a distinction that
// matters because only the second is drift.
func anyInstalled(declared []string, installed map[string]bool) bool {
	for _, d := range declared {
		if installed[d] {
			return true
		}
	}
	return false
}
