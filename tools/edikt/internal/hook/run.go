package hook

// run.go — the dispatcher behind `bin/edikt hook run <name>`.
//
// THIS FILE HOLDS THE ONLY defer recover() IN THE HOOK TIER.
//
// FR-006 is specific about that, and the specificity is the value: twenty-two
// handlers each with their own recovery is twenty-two chances to get the exit
// code wrong, and getting it wrong at PreToolUse means a Go panic BLOCKS the
// user's write. That is strictly worse than the missing-interpreter behaviour
// this migration exists to retire. One site, audited once, is the whole design.
// tools/edikt/internal/hook's TestSinglePanicRecoverySite counts them.
//
// THREE WAYS A HOOK CAN SAY NOTHING, AND THEY ARE NOT THE SAME THING
//
//	the name resolved to no handler   the hook was never found
//	the payload would not decode      the hook was found and never ran
//	the handler panicked              the hook ran and broke
//
// All three exit 0 with an empty reply, because the hook tier fails open. All
// three are journaled with their OWN outcome type, because at the hook's own
// output they are byte-identical to a hook that had nothing to say — and
// telling those apart is the entire reason this journal exists (ADR-060:d18).
//
// WHAT THIS FILE DELIBERATELY DOES NOT DO
//
// No index read. No config read. No project-root resolution. `hook match`
// costs 23.3ms against the binary's 9.4ms floor, and 14ms of that is per-call
// index work — a 2.6 MB YAML read plus a sha256, paid before any cache can be
// consulted. A trivial hook like instructions-loaded runs in 2.5ms today; a
// dispatcher that eagerly touched the index would make it ~10x slower for
// nothing. Handlers that need the index ask for it; the ones that do not, do
// not pay. TestNoEagerIO proves it with a FIFO rather than by inspection.

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// Result is what a handler returns.
type Result struct {
	// Response is the reply, in host-neutral terms. The dispatcher encodes it
	// through the active host adapter; the handler never serializes anything.
	Response adapter.Response

	// ExitCode is the process exit status. 0 for every ordinary outcome;
	// non-zero only where the hook protocol gives a non-zero code a meaning
	// (a gate's deny). A handler that wants to report an internal failure
	// returns 0 with a Message — a non-zero exit from an injector is read by
	// the host as an instruction, not as a complaint.
	ExitCode int
}

// Invocation is everything a handler is given.
//
// It carries no raw bytes and no host-shaped value: a handler that could reach
// the original payload would have no reason to respect the seam, and the leak
// check would be enforcing a boundary the type system had already opened.
type Invocation struct {
	// Name is the registry name this handler was resolved under.
	Name string

	// Payload is the decoded, host-neutral hook payload.
	Payload adapter.Payload

	// PayloadErr is non-nil ONLY for a handler registered through
	// RegisterPayloadTolerant, and only when the host payload failed to
	// decode. Every other handler sees nil here because the dispatcher
	// already returned before reaching it.
	//
	// A non-nil value means the bytes arrived and were not JSON. It does not
	// mean the bytes were absent — ask Payload.Absent for that. A gate that
	// must fail closed on uninspectable input has to treat BOTH as
	// uninspectable, which is why they are two questions and not one flag.
	PayloadErr error

	// Stderr is where a handler writes diagnostics for a human. It is never
	// the hook protocol channel — stdout is, and the dispatcher owns it.
	Stderr io.Writer

	// Step is the dispatch-orchestrating step protocol's argv, or nil for the
	// twenty-one hooks that are a plain exec shim.
	//
	// It is nil for every handler but one, and that is the point: a handler
	// that never reads it cannot be affected by a flag it does not know
	// exists, and the ADR-063 Amendment's second shape stays confined to the
	// one hook that genuinely orchestrates multi-round-trip dispatch. A nil
	// value means "the caller invoked me with no step" — which the one
	// handler that reads it treats as step 1, so `hook run <name>` with no
	// flags remains the corpus recorder's own entry point.
	Step *StepArgs

	rootOnce sync.Once
	root     string
	rootErr  error
	rootFn   func() (string, error)
}

// Root resolves the project root, LAZILY and at most once.
//
// Lazy is not a micro-optimisation here: it is what keeps the no-eager-IO
// property true by construction rather than by discipline. A handler that
// never asks pays nothing, and the common path for the trivial injectors is
// exactly that.
func (inv *Invocation) Root() (string, error) {
	inv.rootOnce.Do(func() {
		fn := inv.rootFn
		if fn == nil {
			fn = defaultRoot(inv.Payload.Cwd)
		}
		inv.root, inv.rootErr = fn()
	})
	return inv.root, inv.rootErr
}

// defaultRoot prefers the working directory the host reported and falls back
// to the process's own. The host's value is preferred because a hook can be
// spawned with a cwd that is not the project (a worktree event names its own
// path), and the process cwd would then govern the wrong tree.
func defaultRoot(hostCwd string) func() (string, error) {
	return func() (string, error) {
		if strings.TrimSpace(hostCwd) != "" {
			return hostCwd, nil
		}
		return os.Getwd()
	}
}

// Options configures one dispatch.
type Options struct {
	// Name is the hook name to resolve. In production it is a literal baked
	// into the shim at generation time, never runtime input — see the
	// unregistered-name path for what happens when someone passes their own.
	Name string

	// Event is the lifecycle event the shim was generated for. Used only when
	// the payload does not carry one (a malformed or empty payload), so a
	// reply still echoes the right event back.
	Event string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Registry overrides the process-wide registry. Tests use this to install
	// stand-in handlers; production leaves it nil.
	Registry *Registry

	// Journal overrides where invocation records go. nil means the real
	// journal. Overriding it is for tests that must assert on a record without
	// racing the user's own journal file.
	Journal func(hookmatch.Record) error

	// Now overrides the clock, for deterministic duration assertions.
	Now func() time.Time

	// RootFn overrides project-root resolution for the handler.
	RootFn func() (string, error)

	// Step carries the dispatch-orchestrating step protocol's argv scalars
	// straight through to the handler. See Invocation.Step.
	Step *StepArgs
}

// StepArgs is the bash-to-Go half of the dispatch-orchestrating caller
// protocol the ADR-063 Amendment (2026-08-22) admits.
//
// EVERY FIELD IS A SCALAR, AND THAT IS THE CONSTRAINT, NOT A SIMPLIFICATION.
// The caller may not build JSON (INV-003 with no interpreter available per
// INV-001), so the only things it can hand back across a round trip are argv
// scalars and the path of a file it redirected a dispatch's output into. A
// richer envelope would have to be constructed somewhere, and the only place
// left to construct it is the shell.
type StepArgs struct {
	// Step is which leg of the protocol to run, 1-based.
	Step int

	// StatePath is the cross-step state file Go itself wrote on the previous
	// leg. Empty on the first step.
	StatePath string

	// DispatchOut is the file the caller redirected the previous dispatch's
	// output into. Empty when the previous leg produced no output file.
	DispatchOut string

	// DispatchExit is the previous dispatch's exit status. Passed EXPLICITLY
	// rather than inferred from an empty output file: INV-013 — a dispatch
	// that failed and a dispatch that returned nothing must not produce the
	// same state.
	DispatchExit int

	// DispatchTool is the NAME of the CLI the caller execs, supplied as data.
	// The binary never dispatches a model (INV-012) and therefore never names
	// one; it formats this into operator-facing text so a different host's
	// caller yields a message naming ITS tool rather than a hardcoded vendor.
	DispatchTool string

	// DispatchAvailable reports whether the caller could resolve that tool on
	// PATH. The binary cannot look: `exec.LookPath` on a model CLI is exactly
	// the pattern ADR-030's purity gate exists to refuse.
	DispatchAvailable bool
}

// Dispatch runs one hook end to end and returns the process exit code.
//
// It NEVER returns an error and NEVER panics out of itself. Both would push a
// decision onto a caller that has no better answer than this one does, and the
// caller here is a shim whose only remaining move is to exit.
func Dispatch(o Options) int {
	stderr := o.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	stdout := o.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	reg := o.Registry
	if reg == nil {
		reg = defaultRegistry
	}
	start := now()

	// ── 1. RESOLVE THE NAME, BEFORE ANYTHING ELSE ───────────────────────
	//
	// Before reading stdin, before decoding: an unresolvable name has nothing
	// to read a payload for, and doing the work first would mean a typo'd name
	// still paid for it.
	h, ok := reg.Lookup(o.Name)
	if !ok {
		fmt.Fprintf(stderr,
			"edikt hook run: %q is not a registered hook name — nothing ran.\n"+
				"  This is NOT a no-op that can be ignored: a shim pointing at an unknown\n"+
				"  name means that hook's governance is not firing at all. Registered: %v\n"+
				"  Declared: templates/hooks/registry.yaml\n",
			o.Name, reg.Names())
		journal(o, hookmatch.Record{
			Kind:    hookmatch.KindHookRun,
			Hook:    o.Name,
			Outcome: string(hookmatch.OutcomeUnregisteredHookName),
			Detail:  fmt.Sprintf("no handler registered for %q", o.Name),
		}, 0, now().Sub(start))
		writeReply(stdout, stderr, o.Event, adapter.Response{})
		return 0
	}

	// ── 2. READ AND DECODE THE PAYLOAD ──────────────────────────────────
	//
	// Read in full, never streamed: every bash hook it replaces does `$(cat)`,
	// and no payload in the current 22 is large enough for buffering to cost
	// anything measurable.
	var raw []byte
	if o.Stdin != nil {
		b, err := io.ReadAll(o.Stdin)
		if err != nil {
			// An unreadable stdin is the payload being unavailable, which is
			// the same fact as an undecodable one from the handler's side.
			fmt.Fprintf(stderr, "edikt hook run %s: reading payload: %v — hook did not run\n", o.Name, err)
			journal(o, hookmatch.Record{
				Kind:    hookmatch.KindHookRun,
				Hook:    o.Name,
				Outcome: string(hookmatch.OutcomePayloadUnparseable),
				Detail:  "read stdin: " + err.Error(),
			}, 0, now().Sub(start))
			writeReply(stdout, stderr, o.Event, adapter.Response{})
			return 0
		}
		raw = b
	}

	payload, err := adapter.Parse(raw)
	if err != nil && !reg.PayloadTolerant(o.Name) {
		fmt.Fprintf(stderr,
			"edikt hook run %s: host payload could not be decoded (%v) — the hook was\n"+
				"  found and did NOT run. Governance for this event did not fire.\n", o.Name, err)
		journal(o, hookmatch.Record{
			Kind:    hookmatch.KindHookRun,
			Hook:    o.Name,
			Outcome: string(hookmatch.OutcomePayloadUnparseable),
			Detail:  truncate(err.Error(), 400),
		}, 0, now().Sub(start))
		writeReply(stdout, stderr, o.Event, adapter.Response{})
		return 0
	}
	payloadErr := err
	if payloadErr != nil {
		// JOURNALED, NOT PRINTED. The payload WAS unparseable and that fact is
		// worth recording, so the journal gets the same record it would for a
		// non-tolerant hook. Stderr does not, because a tolerant handler has
		// opted into ANNOUNCING the degradation itself, on the protocol
		// channel where the model reading the reply will actually see it —
		// verify-gate's unparseable-payload deny says exactly this in its own
		// reason string. A second copy on stderr would add a channel without
		// adding a fact, and it is the handler, not the dispatcher, that
		// knows whether degrading was a deny or an allow.
		journal(o, hookmatch.Record{
			Kind:    hookmatch.KindHookRun,
			Hook:    o.Name,
			Outcome: string(hookmatch.OutcomePayloadUnparseable),
			Detail:  truncate(payloadErr.Error(), 400),
		}, 0, now().Sub(start))
	}

	event := payload.Event
	if event == "" {
		event = o.Event
	}
	if event == "" {
		event = declaredEvent(o.Name, o.RootFn)
	}

	// ── 3. RUN IT, UNDER THE ONE RECOVERY SITE ──────────────────────────
	inv := &Invocation{Name: o.Name, Payload: payload, PayloadErr: payloadErr, Stderr: stderr, rootFn: o.RootFn, Step: o.Step}
	res, panicked := invoke(h, inv)
	elapsed := now().Sub(start)

	if panicked != nil {
		// A handler that declared a panic reply gets to send it. The reply is
		// produced under the SAME recovery — a fallback that panics falls back
		// to the empty one rather than escaping — so declaring one can never
		// turn a recovered crash into an unrecovered one.
		reply := adapter.Response{}
		disposition := "an empty reply so the write is not blocked"
		// FROM reg, NOT THE PACKAGE REGISTRY. The handler was resolved from
		// reg, so its declared panic reply must come from there too — a test
		// that substitutes a stand-in handler under a real hook's name would
		// otherwise inherit the real hook's fallback and be graded against a
		// reply its own handler never declared.
		if fb, ok := reg.LookupPanicFallback(o.Name); ok {
			// THROUGH invoke, NOT A SECOND recover(). The fallback runs after
			// a panic has already been recovered and must not be able to
			// escape, but adding its own recovery site would put two of them
			// in a tier whose whole claim is that there is exactly one — and
			// that claim is asserted by TestSingleRecoverSite. Wrapping the
			// fallback as a Handler reuses the site instead of duplicating it.
			if r, fbPanicked := invoke(func(*Invocation) Result {
				return Result{Response: fb()}
			}, inv); fbPanicked == nil {
				reply = r.Response
				disposition = "this hook's declared panic reply"
			} else {
				fmt.Fprintf(stderr,
					"edikt hook run %s: its panic fallback ALSO panicked (%v) — falling back to the empty reply\n",
					o.Name, fbPanicked.value)
			}
		}
		fmt.Fprintf(stderr,
			"edikt hook run %s: PANIC recovered — %v\n"+
				"  Exiting 0 with %s. This is a\n"+
				"  fail-open unless the hook said otherwise: the record is journaled as %s.\n%s\n",
			o.Name, panicked.value, disposition, hookmatch.OutcomePanicRecovered, indent(panicked.stack))
		journal(o, hookmatch.Record{
			Kind:    hookmatch.KindHookRun,
			Hook:    o.Name,
			Outcome: string(hookmatch.OutcomePanicRecovered),
			Detail:  truncate(fmt.Sprintf("%v", panicked.value), 400),
		}, 0, elapsed)
		writeReply(stdout, stderr, event, reply)
		return 0
	}

	// ── 4. ENCODE AND REPORT ────────────────────────────────────────────
	//
	// A ZERO Response writes NOTHING to stdout — never "{}". Every bash hook
	// this dispatcher replaces has an early-exit guard ("not an edikt
	// project", "feature disabled") that reaches its own `exit 0` with no
	// printf/python3 call at all, so the oracle recording for that scenario
	// is zero stdout bytes. Encoding the zero Response as literal "{}" would
	// be semantically equivalent (the host reads either as "no opinion") but
	// byte-DIFFERENT, and the corpus gate (SPEC-014 FR-002) compares raw
	// bytes after normalization, not re-parsed JSON values — so this is the
	// one branch where matching the host's tolerance is not enough; the
	// bytes must match the recording. This applies only to a handler's own
	// successful "nothing to say" — the three fail-open paths above (name
	// unresolved, payload unparseable, panic recovered) deliberately keep
	// emitting "{}" unconditionally: TestPanicRecovery and
	// TestUnregisteredHookName assert stdout decodes as valid JSON on those
	// paths, a guarantee those failure classes need and a normal "no
	// comment" outcome does not.
	//
	// An encode failure is journaled as panic-recovered on purpose. The class
	// means "the hook ran and broke", and a Result that cannot be serialized
	// is precisely that — the handler produced something the protocol cannot
	// express. Giving it a fourth class would split one fact across two names.
	if !res.Response.IsZero() {
		if encodeErr := writeReply(stdout, stderr, event, res.Response); encodeErr != nil {
			journal(o, hookmatch.Record{
				Kind:    hookmatch.KindHookRun,
				Hook:    o.Name,
				Outcome: string(hookmatch.OutcomePanicRecovered),
				Detail:  truncate("encode reply: "+encodeErr.Error(), 400),
			}, 0, elapsed)
			return 0
		}
	}

	// IDENTITY IS RECORDED WHERE THE HOST SUPPLIES IT.
	//
	// Every hook_run record used to carry only hook/exit_code/duration, so a
	// journal full of subagent-stop entries could not say which skill produced
	// any of them. The host sends agent_type and agent_id on the Subagent
	// events and prompt_id on all four lifecycle events; this writes them
	// down. hookmatch.ResolveIdentity is REUSED rather than reimplemented — it
	// already encodes the rule that unknown is never guessed as "parent".
	//
	// Events that carry no identity are unaffected: actor resolves to parent,
	// agentType to "-", and PromptID/AgentTranscriptPath stay empty and are
	// omitempty, so no record grows a field the host never sent.
	actor, contextID, agentType := hookmatch.ResolveIdentity(payload)
	journal(o, hookmatch.Record{
		Kind:                hookmatch.KindHookRun,
		Hook:                o.Name,
		Outcome:             string(hookmatch.OutcomeHookRan),
		SessionID:           payload.SessionID,
		Path:                payload.FilePath,
		Actor:               actor,
		AgentType:           agentType,
		ContextID:           contextID,
		PromptID:            payload.PromptID,
		AgentTranscriptPath: payload.AgentTranscriptPath,
	}, res.ExitCode, elapsed)
	return res.ExitCode
}

// recovered carries what a panic produced, so the caller can journal it
// without a second recover().
type recovered struct {
	value any
	stack string
}

// invoke is THE panic-recovery site.
//
// Every handler invocation in edikt goes through this function and no other.
// It returns the zero Result on a panic, which encodes to the minimal valid
// reply and exits 0 — a partial or mid-panic reply is not an option, since a
// half-written protocol object is worse for the host to read than no object.
func invoke(h Handler, inv *Invocation) (res Result, panicked *recovered) {
	defer func() {
		if r := recover(); r != nil {
			panicked = &recovered{value: r, stack: string(debug.Stack())}
			res = Result{}
		}
	}()
	return h(inv), nil
}

// writeReply encodes through the active host adapter and writes to stdout.
//
// v0.8.0 has exactly one adapter and the choice is unconditional. This is the
// point a future multi-host dispatch plugs into — one function, not twenty-two
// call sites (SP-004).
func writeReply(stdout, stderr io.Writer, event string, r adapter.Response) error {
	out, err := adapter.Encode(event, r)
	if err != nil {
		fmt.Fprintf(stderr, "edikt hook run: encoding reply: %v — emitting an empty reply instead\n", err)
		// The empty reply cannot fail to encode; if it somehow did there
		// would be nothing left to fall back to, so write the literal.
		if fallback, ferr := adapter.Encode(event, adapter.Response{}); ferr == nil {
			_, _ = stdout.Write(fallback)
			_, _ = io.WriteString(stdout, "\n")
		}
		return err
	}
	_, _ = stdout.Write(out)
	// The trailing "\n" is supplied HERE, uniformly, for every reply shape —
	// except the one Response.NoTrailingNewline documents: status-line.sh's
	// `printf '%s'` contract, where bash's own output carries no trailing
	// byte at all and the corpus gate compares raw bytes with no
	// newline-trimming pass. See adapter.Response.NoTrailingNewline's own
	// doc comment for why this is a flag on the reply rather than a second
	// encoding path.
	if !(r.Text != "" && r.NoTrailingNewline) {
		_, _ = io.WriteString(stdout, "\n")
	}
	return nil
}

// journal writes one invocation record.
//
// Its error is swallowed for the reason hookmatch.Append documents: a journal
// that cannot be written must not break the write it is observing. exitCode
// and duration are passed as values and stored as POINTERS, so "exited 0" is
// distinguishable from "this record carried no exit code" (INV-013).
func journal(o Options, r hookmatch.Record, exitCode int, d time.Duration) {
	code := exitCode
	us := d.Microseconds()
	r.ExitCode = &code
	r.DurationUS = &us
	appendFn := o.Journal
	if appendFn == nil {
		appendFn = hookmatch.Append
	}
	// Fail-open, but NEVER SILENT — the tier's standing posture. The append
	// still cannot break the event it is observing, so the error does not
	// propagate; it is announced instead. Before this, a journal that could
	// not be resolved at all (an EDIKT_ROOT that is relative, refused since
	// 2026-08-27) produced no record and no diagnostic, so the journal simply
	// stopped growing and nothing said why. `hook report` reports staleness
	// eventually; this names the cause at the moment it happens.
	if err := appendFn(r); err != nil && o.Stderr != nil {
		fmt.Fprintf(o.Stderr, "edikt hook %s: journal unavailable (%v); the hook ran, but its record was not written\n", r.Hook, err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

// declaredEvent resolves the FIRING event from the hook's own registry row,
// for the case where neither the payload nor the --event flag named one.
//
// The host always sends hook_event_name. The hook test harness, the corpus
// recordings, and any older host do not — and with the event keyed on the
// payload alone, adapter.Encode's union gate (hookevents.go, 2026-09-02)
// refused to emit a PreToolUse deny it could not name and the dispatcher
// FAILED OPEN with "{}": a MUST-grade governed write went through, loud only
// on stderr. Found by test_inject_directives_pre.sh ("MUST-grade match did NOT
// deny") and the governed-write-denied corpus scenario, both first
// mis-attributed to the routing fix of the same day. A validation gate whose
// missing input turns into an allow is the INV-013 shape exactly.
//
// The registry row is the authority on what a hook fires on; the payload's
// field, when present, is the host's word about THIS firing and still wins.
// Returns "" when the registry cannot be resolved or the row declares no
// event, which leaves the encoder's refusal in place — for a hook with no
// declared event there is nothing truthful to put in the envelope.
func declaredEvent(name string, rootFn func() (string, error)) string {
	root := "."
	if rootFn != nil {
		if r, err := rootFn(); err == nil && r != "" {
			root = r
		}
	} else if wd, err := os.Getwd(); err == nil {
		root = wd
	}
	path, _, err := ResolveRegistryPath(root)
	if err != nil {
		return ""
	}
	f, err := LoadRegistryFile(path)
	if err != nil {
		return ""
	}
	for _, r := range f.Hooks {
		if r.Name == name {
			if e, ok := r.EventName(); ok {
				return e
			}
			return ""
		}
	}
	return ""
}
