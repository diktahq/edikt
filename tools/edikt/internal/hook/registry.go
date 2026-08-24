// Package hook is the migration core: a data-driven registry of hook
// handlers, a host-neutral dispatcher with ONE panic-recovery site, and the
// per-invocation journaling that makes a hook's silence readable.
//
// NOTHING IS MIGRATED HERE. This package ships with zero handlers registered.
// Phases 12-22 add them one at a time, and the parity gate below is what makes
// each addition a checked fact rather than a claim.
//
// # HOW A NAME RESOLVES, AND WHY IT IS NOT A SWITCH
//
// SP-007 is explicit: `bin/edikt hook run <name>` must resolve names from
// DATA, never from a hardcoded Go enumeration, so a future name-pattern change
// does not require a binary change. The data is
// templates/hooks/registry.yaml. The runtime map here is populated by each
// handler file's own init() calling Register — no central dispatch function
// anyone has to remember to edit — and TestRegistryParity binds the two
// together: every `status: migrated` row must have a registered handler, and
// every registered handler must have a `status: migrated` row, with any
// mismatch named. Resolution therefore goes through a map that is PROVEN equal
// to the file, instead of paying a 2 KB YAML parse on every hook fire to
// re-derive an answer the gate already guarantees.
//
// The distinction that matters is not map-versus-file, it is
// data-versus-enumeration: adding, renaming or removing a hook edits YAML and
// one handler file. It never edits a switch.
package hook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// Handler is one hook's Go implementation.
//
// A handler returns a Result; it never writes to stdout itself, never calls
// os.Exit, and never encodes a reply. All three are the dispatcher's job, which
// is what keeps the exit-code and serializer discipline in ONE place instead of
// twenty-two.
//
// A handler MAY panic. That is not permission to be careless — it is the
// admission that a nil map, a short slice or a bad type assertion will
// eventually happen in one of twenty-two handlers, and the dispatcher is built
// so that when it does, the write still goes through.
type Handler func(*Invocation) Result

// NamePattern is the shape a hook name may take. Identical to the `name`
// pattern in templates/schemas/hook-registry.v1.schema.json — the same rule
// written twice would drift, so the test asserts these agree.
var NamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Registry maps hook names to handlers.
//
// Safe for concurrent lookup. Registration happens in package init(), which is
// single-goroutine by definition, but the mutex costs nothing and removes the
// question.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	// tolerant names the handlers that must still run when the payload does
	// not decode. See RegisterPayloadTolerant for why this is opt-in.
	tolerant map[string]bool
	// panicFallbacks names the handlers that supply their own reply when they
	// crash, instead of the dispatcher's silent fail-open.
	panicFallbacks map[string]PanicFallback
}

// NewRegistry returns an empty registry. Tests use one of these to install
// stand-in handlers without mutating the process-wide registry the real
// handlers register into.
func NewRegistry() *Registry {
	return &Registry{handlers: map[string]Handler{}}
}

var defaultRegistry = NewRegistry()

// Register adds a handler under name.
//
// PANICS on a duplicate name, an empty handler, or a name that does not match
// NamePattern. All three are programming errors visible at process start, and
// a registry that quietly dropped one would produce a hook that resolves to
// the wrong handler — or to none — with nothing reporting the substitution.
// Panicking in init() fails every invocation loudly; that is the correct
// blast radius for a build that is wrong about its own hooks.
func Register(name string, h Handler) { defaultRegistry.Register(name, h) }

// Register adds a handler under name. See the package-level Register.
func (r *Registry) Register(name string, h Handler) {
	if !NamePattern.MatchString(name) {
		panic(fmt.Sprintf("hook.Register: %q is not a valid hook name (want %s)", name, NamePattern))
	}
	if h == nil {
		panic(fmt.Sprintf("hook.Register: nil handler for %q", name))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = map[string]Handler{}
	}
	if _, dup := r.handlers[name]; dup {
		panic(fmt.Sprintf("hook.Register: %q registered twice", name))
	}
	r.handlers[name] = h
}

// RegisterPayloadTolerant registers a handler that must ALSO run when the host
// payload cannot be decoded.
//
// WHY AN OPT-IN AND NOT THE DEFAULT. The dispatcher's normal answer to an
// undecodable payload is to journal it and reply with nothing: the hook was
// found, it did not run, and the write proceeds. For twenty-one handlers that
// is right — an injector with no payload has nothing to inject, and running it
// against a zero Payload would make it act on fields the host never sent.
//
// It is wrong for a GATE. verify-gate's bash original denies an unparseable
// payload (INV-011: absence of inspectable input is not a pass) precisely
// because the payload is the attacker-controlled input; if a truncated JSON
// object buys a pass, the gate is one malformed byte from irrelevant. Leaving
// that to the dispatcher's default would have moved a deny to an allow as a
// SIDE EFFECT of the migration seam, with no handler line to point at.
//
// So the tolerance is per-handler and declared, and Invocation.PayloadErr is
// how the handler learns which case it is in. A handler registered this way
// MUST treat a non-nil PayloadErr as "I could not inspect this", never as
// "there was nothing to inspect" — see verifyGate.
func RegisterPayloadTolerant(name string, h Handler) {
	defaultRegistry.RegisterPayloadTolerant(name, h)
}

// RegisterPayloadTolerant registers a payload-tolerant handler. See the
// package-level RegisterPayloadTolerant.
func (r *Registry) RegisterPayloadTolerant(name string, h Handler) {
	r.Register(name, h)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tolerant == nil {
		r.tolerant = map[string]bool{}
	}
	r.tolerant[name] = true
}

// PayloadTolerant reports whether name opted into running on an undecodable
// payload.
func PayloadTolerant(name string) bool { return defaultRegistry.PayloadTolerant(name) }

// PayloadTolerant reports whether name opted into running on an undecodable
// payload.
func (r *Registry) PayloadTolerant(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tolerant[name]
}

// PanicFallback produces the reply to send when a handler panics.
//
// WHY A HOOK NEEDS ONE. The dispatcher recovers every panic at a single site
// and replies with nothing, which the host reads as allow. For twenty-one
// handlers that is the right answer — an injector that crashed has no context
// to inject and blocking the write would punish the user for the hook's bug.
//
// For a GATE it inverts the control. A gate that crashes has not decided the
// write is safe; it has failed to look at it, and INV-013 is explicit that a
// control must never report an outcome it did not achieve. Under a posture
// that is supposed to block, a crash silently becoming an allow is the same
// defect class as the inert deny channel ADR-061 fixed: the gate reports as
// enforcing while enforcing nothing.
//
// A fallback must not itself panic. The dispatcher runs it under the same
// recovery, so one that does falls back to the empty reply — the fail-open of
// last resort, which is still better than an unrecovered crash.
type PanicFallback func() adapter.Response

// RegisterPanicFallback records the reply to use when name's handler panics.
func RegisterPanicFallback(name string, fn PanicFallback) {
	defaultRegistry.RegisterPanicFallback(name, fn)
}

// RegisterPanicFallback records a panic reply. See the package-level form.
func (r *Registry) RegisterPanicFallback(name string, fn PanicFallback) {
	if fn == nil {
		panic(fmt.Sprintf("hook.RegisterPanicFallback: nil fallback for %q", name))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.panicFallbacks == nil {
		r.panicFallbacks = map[string]PanicFallback{}
	}
	r.panicFallbacks[name] = fn
}

// LookupPanicFallback returns name's panic reply producer, if it declared one.
func LookupPanicFallback(name string) (PanicFallback, bool) {
	return defaultRegistry.LookupPanicFallback(name)
}

// LookupPanicFallback returns name's panic reply producer, if any.
func (r *Registry) LookupPanicFallback(name string) (PanicFallback, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.panicFallbacks[name]
	return fn, ok
}

// Lookup resolves a name to its handler.
func Lookup(name string) (Handler, bool) { return defaultRegistry.Lookup(name) }

// Lookup resolves a name to its handler.
func (r *Registry) Lookup(name string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[name]
	return h, ok
}

// Names lists every registered name, sorted.
func Names() []string { return defaultRegistry.Names() }

// Names lists every registered name, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.handlers))
	for n := range r.handlers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ── the registry FILE ───────────────────────────────────────────────────────

// Row is one hook's declaration in templates/hooks/registry.yaml.
type Row struct {
	Name string `yaml:"name"`
	// Event is a pointer so `event: null` — the schema's way of saying "this
	// row is a sourced library, not a hook" — is distinguishable from an
	// absent key. Decoding both to "" would erase the one fact the field
	// exists to carry.
	Event  *string `yaml:"event"`
	File   string  `yaml:"file"`
	Class  string  `yaml:"class"`
	Status string  `yaml:"status"`
	// Shape is WHAT the shipped templates/hooks/<file> is, and it exists so
	// the byte-stability gate has a DECLARED subject rather than an inferred
	// one. Absent (the overwhelming default) means the canonical FR-004 exec
	// shim, which is diffed byte-for-byte against a fresh render. The one
	// other value is the ADR-063 Amendment's dispatch-orchestrating caller:
	// a hook that must perform the model dispatch itself because INV-012
	// forbids the binary from doing it, so its body is deliberately NOT the
	// canonical render and a byte-diff against one would always fail.
	//
	// Declared here rather than hardcoded in the gate for SP-007's reason:
	// the gate must resolve what a hook IS from data, so a second such hook
	// is a registry edit and not a Go edit.
	Shape                string `yaml:"shape"`
	Python3BaselineCount int    `yaml:"python3_baseline_count"`
}

// Shape values. The empty string is ShapeExecShim.
const (
	ShapeExecShim             = "exec-shim"
	ShapeDispatchOrchestrator = "dispatch-orchestrator"
)

// ShapeName returns the row's shape, defaulting to the exec shim.
func (r Row) ShapeName() string {
	if r.Shape == "" {
		return ShapeExecShim
	}
	return r.Shape
}

// EventName returns the row's event, and whether it has one at all.
func (r Row) EventName() (string, bool) {
	if r.Event == nil {
		return "", false
	}
	return *r.Event, true
}

// Class values.
const (
	ClassGate      = "gate"
	ClassMultiCall = "multi-call"
	ClassInjector  = "injector"
	ClassLibrary   = "library"
)

// Status values.
const (
	StatusPending  = "pending"
	StatusMigrated = "migrated"
)

// File is the parsed registry.yaml.
type File struct {
	SchemaVersion int   `yaml:"schema_version"`
	Hooks         []Row `yaml:"hooks"`
	// Path records where this was read from, so a caller reporting on the
	// registry can name its subject rather than assert about an unnamed file.
	Path string `yaml:"-"`
}

// MigratedNames returns the names of rows claiming a Go handler exists.
func (f File) MigratedNames() []string {
	var out []string
	for _, r := range f.Hooks {
		if r.Status == StatusMigrated {
			out = append(out, r.Name)
		}
	}
	sort.Strings(out)
	return out
}

// ParityDiff compares a registry file's migrated rows against the names
// actually registered in Go — the SP-007 parity gate's whole computation.
//
// It lives here rather than inside the test so the test can also run it
// against a SYNTHETIC mismatch and prove the comparison is capable of
// failing. A gate whose only evidence is that it passed on the real corpus has
// not demonstrated it can do anything else.
//
// Returns:
//
//	claimed  — rows saying `status: migrated` with no registered handler.
//	           A shim execs this name and nothing runs.
//	undeclared — registered handlers with no `status: migrated` row. The
//	           handler exists and no shim will ever reach it, and the corpus
//	           recorder does not know it is there.
//
// Both are named individually. A count mismatch tells a maintainer that
// something is wrong; a name tells them what.
func ParityDiff(migrated, registered []string) (claimed, undeclared []string) {
	inMigrated := map[string]bool{}
	for _, n := range migrated {
		inMigrated[n] = true
	}
	inRegistered := map[string]bool{}
	for _, n := range registered {
		inRegistered[n] = true
	}
	for _, n := range migrated {
		if !inRegistered[n] {
			claimed = append(claimed, n)
		}
	}
	for _, n := range registered {
		if !inMigrated[n] {
			undeclared = append(undeclared, n)
		}
	}
	sort.Strings(claimed)
	sort.Strings(undeclared)
	return claimed, undeclared
}

// RegistryFileName is the basename of the registration surface.
const RegistryFileName = "registry.yaml"

// LoadRegistryFile parses a registry.yaml.
//
// REFUSES an empty hooks list rather than returning an empty File. A registry
// with zero rows and a registry that failed to load are the same value and
// completely different facts, and every consumer of this file — the parity
// gate, the shim generator, the corpus recorder — reports "nothing to do" on
// the first and must not report it for the second (INV-013).
func LoadRegistryFile(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read hook registry: %w", err)
	}
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("parse hook registry %s: %w", path, err)
	}
	if f.SchemaVersion != 1 {
		return File{}, fmt.Errorf("hook registry %s: unsupported schema_version %d (want 1)", path, f.SchemaVersion)
	}
	if len(f.Hooks) == 0 {
		return File{}, fmt.Errorf("hook registry %s declares zero hooks — an empty registry is a broken one, not an empty project", path)
	}
	seen := map[string]string{}
	for _, r := range f.Hooks {
		if prev, dup := seen[r.Name]; dup {
			return File{}, fmt.Errorf("hook registry %s: %q declared twice (files %s and %s)", path, r.Name, prev, r.File)
		}
		seen[r.Name] = r.File
	}
	f.Path = path
	return f, nil
}

// ResolveRegistryPath finds the registration surface.
//
// Same precedence rule templates/permits and templates/schemas already use, in
// the same order, so a project override behaves identically across all three
// and no new resolution mechanism is invented here:
//
//  1. <projectRoot>/.edikt/hooks/registry.yaml   project override
//  2. $EDIKT_HOME/current/templates/hooks/…      the installed payload
//  3. <projectRoot>/templates/hooks/registry.yaml  dev mode / this repo
//
// Returns the first candidate that EXISTS, and the full candidate list when
// none do — a caller that cannot find the registry must be able to say where
// it looked, not just that it failed.
func ResolveRegistryPath(projectRoot string) (string, []string, error) {
	var candidates []string
	candidates = append(candidates, filepath.Join(projectRoot, ".edikt", "hooks", RegistryFileName))

	root := os.Getenv("EDIKT_HOME")
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil {
			root = filepath.Join(home, ".edikt")
		}
	}
	if root != "" {
		candidates = append(candidates, filepath.Join(root, "current", "templates", "hooks", RegistryFileName))
	}
	candidates = append(candidates, filepath.Join(projectRoot, "templates", "hooks", RegistryFileName))

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, candidates, nil
		}
	}
	return "", candidates, fmt.Errorf("hook registry not found; looked in %v", candidates)
}
