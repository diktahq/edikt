// Package hookcfg reads hook behaviour from .edikt/config.yaml.
//
// WHY BEHAVIOUR IS CONFIG AND REGISTRATION IS NOT
//
// Registration is an invariant: init/upgrade wire every shipped hook, in full.
// The injection tier shipped UNREGISTERED for an entire release because
// registration was a decision someone had to remember — so it stopped being a
// decision. Behaviour is the thing that legitimately varies, and it varies in a
// committed file beside the governance it affects.
//
// ENFORCEMENT STRENGTH IS A TEAM PROPERTY. Settable per user, the guarantee is
// fiction — and invisible fiction, because everyone sees their own reality and
// assumes it is shared. Enforcement hooks therefore carry a FLOOR: `enabled:
// false` is REFUSED for them, because a range that includes "never enforce" is
// the bypass flag under another name.
//
// DISABLED IS AN EXPLICIT STATE. Status distinguishes enabled, disabled and
// unconfigured, so a caller can tell "someone turned this off" from "nobody
// ever said" — if the two looked alike we would have relocated the invisible
// fiction rather than removed it.
package hookcfg

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MinBounceBudget is the floor. Zero would mean "never bounce", which is not a
// budget — it is enforcement switched off wearing a number.
const MinBounceBudget = 1

// DefaultBounceBudget applies when nothing is configured.
const DefaultBounceBudget = 8

// Status is the tri-state a caller needs to tell an explicit decision from an
// absence.
type Status string

const (
	StatusEnabled      Status = "enabled"
	StatusDisabled     Status = "disabled"     // someone said so, in version control
	StatusUnconfigured Status = "unconfigured" // nobody said; defaults apply
)

type entry struct {
	Enabled *bool `yaml:"enabled"`
}

type injection struct {
	BounceBudget *int   `yaml:"bounce_budget"`
	DedupScope   string `yaml:"dedup_scope"`
}

type hooks struct {
	Enforcement map[string]entry `yaml:"enforcement"`
	Ergonomics  map[string]entry `yaml:"ergonomics"`
	Injection   injection        `yaml:"injection"`
}

type file struct {
	Hooks hooks `yaml:"hooks"`
}

// Config is the resolved hook behaviour.
type Config struct {
	BounceBudget int
	DedupScope   string
	enforcement  map[string]entry
	ergonomics   map[string]entry
}

// Load reads .edikt/config.yaml. A missing file is not an error — defaults
// apply, and every hook already exits early without a config, so an
// unconfigured project is inert rather than broken.
//
// A config that VIOLATES THE FLOOR is an error. Refusing at load is what makes
// "never enforce" unrepresentable rather than merely discouraged: a validator
// that warned would leave the value in force.
func Load(root string) (*Config, error) {
	c := &Config{BounceBudget: DefaultBounceBudget, DedupScope: "context"}
	b, err := os.ReadFile(filepath.Join(root, ".edikt", "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	var f file
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse .edikt/config.yaml: %w", err)
	}
	c.enforcement, c.ergonomics = f.Hooks.Enforcement, f.Hooks.Ergonomics

	for name, e := range f.Hooks.Enforcement {
		if e.Enabled != nil && !*e.Enabled {
			return nil, fmt.Errorf(
				"hooks.enforcement.%s.enabled: false is refused — enforcement hooks carry a floor. "+
					"A configuration range that includes \"never enforce\" is the bypass flag under "+
					"another name. Move the hook to hooks.ergonomics if it genuinely changes no "+
					"outcome, or record an exception where exceptions are reviewed", name)
		}
	}
	if f.Hooks.Injection.BounceBudget != nil {
		if *f.Hooks.Injection.BounceBudget < MinBounceBudget {
			return nil, fmt.Errorf(
				"hooks.injection.bounce_budget: %d is below the floor of %d — a budget of zero is not "+
					"a budget, it is enforcement switched off wearing a number",
				*f.Hooks.Injection.BounceBudget, MinBounceBudget)
		}
		c.BounceBudget = *f.Hooks.Injection.BounceBudget
	}
	if s := f.Hooks.Injection.DedupScope; s != "" {
		if s != "context" {
			return nil, fmt.Errorf(
				"hooks.injection.dedup_scope: %q is refused; only \"context\" is permitted. Keying "+
					"dedup on the session was measured to suppress every subagent injection after a "+
					"parent bounce (F-020): the subagent never saw the message it was told it "+
					"already had", s)
		}
		c.DedupScope = s
	}
	return c, nil
}

// StatusOf reports whether a hook is enabled, disabled, or unconfigured —
// three states, deliberately, so absence is never read as a decision.
func (c *Config) StatusOf(name string) Status {
	for _, m := range []map[string]entry{c.enforcement, c.ergonomics} {
		if e, ok := m[name]; ok {
			if e.Enabled != nil && !*e.Enabled {
				return StatusDisabled
			}
			return StatusEnabled
		}
	}
	return StatusUnconfigured
}

// IsEnforcement reports whether a hook is in the floored class.
func (c *Config) IsEnforcement(name string) bool {
	_, ok := c.enforcement[name]
	return ok
}
