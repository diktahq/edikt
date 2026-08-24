package hookcorpus

// record.go — executing one scenario, and writing down what happened.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Options configures a recorder or a differ.
//
// Every path is explicit and every default is derived from RepoRoot, so a test
// can point the whole apparatus at a fixture tree without touching the repo it
// runs inside — which is also what lets TestRecorderNeverWalksProjectRoot make
// a claim about the real project root at all.
type Options struct {
	RepoRoot     string
	CorpusRoot   string // default <RepoRoot>/test/fixtures/hook-corpus
	HooksDir     string // default <RepoRoot>/templates/hooks
	RegistryPath string // default <HooksDir>/registry.yaml
	BaseTree     string // default <CorpusRoot>/_base

	// Binary is the built bin/edikt used when the live implementation is Go.
	Binary string

	// SandboxParent is where per-scenario sandboxes are created.
	SandboxParent string

	// KeepContent writes the added/modified file bytes alongside the manifests.
	KeepContent bool

	// Trace observes filesystem operations; nil in production.
	Trace Trace

	// Now is the clock. Injected rather than read so an ordering-axis scenario
	// with delay_ms can be advanced instead of slept (SPEC-014 Testing
	// Strategy: the recorder MUST NOT sleep wall-clock for a dedup TTL).
	Now func() time.Time

	// Stderr receives progress and diagnostics.
	Stderr io.Writer
}

func (o *Options) applyDefaults() error {
	if o.RepoRoot == "" {
		return errors.New("hookcorpus: no repo root")
	}
	if o.CorpusRoot == "" {
		o.CorpusRoot = filepath.Join(o.RepoRoot, "test", "fixtures", "hook-corpus")
	}
	if o.HooksDir == "" {
		o.HooksDir = filepath.Join(o.RepoRoot, "templates", "hooks")
	}
	if o.RegistryPath == "" {
		o.RegistryPath = filepath.Join(o.HooksDir, "registry.yaml")
	}
	if o.BaseTree == "" {
		o.BaseTree = filepath.Join(o.CorpusRoot, "_base")
	}
	if o.SandboxParent == "" {
		o.SandboxParent = os.TempDir()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	return nil
}

// implementation is what a scenario will actually execute.
type implementation struct {
	oracle string   // OracleBash | OracleGo
	argv   []string // the command
	digest string   // sha256 of the executed artifact
	path   string   // the artifact, for diagnostics
}

// Recorder captures scenarios against the current implementation of a hook.
type Recorder struct {
	opts     Options
	registry *RegistryFile
	guard    LeakGuard
}

// NewRecorder builds a recorder.
func NewRecorder(opts Options) (*Recorder, error) {
	if err := opts.applyDefaults(); err != nil {
		return nil, err
	}
	reg, err := LoadRegistry(opts.RegistryPath)
	if err != nil {
		return nil, err
	}
	return &Recorder{
		opts:     opts,
		registry: reg,
		guard:    NewLeakGuard(os.Getenv("HOME"), os.Getenv("EDIKT_ROOT")),
	}, nil
}

// Registry exposes the parsed registry (the coverage report reads it).
func (r *Recorder) Registry() *RegistryFile { return r.registry }

// Options exposes the resolved options.
func (r *Recorder) Options() Options { return r.opts }

// resolveImplementation decides what a run of this hook executes, and refuses
// the combinations that would make a recording a lie.
//
// live is "auto" (follow the registry), "bash", or "go".
func (r *Recorder) resolveImplementation(row RegistryRow, live string) (implementation, error) {
	if row.IsLibrary() {
		return implementation{}, fmt.Errorf("hook %q is class %q with event null — a sourced "+
			"library has no payload × config × environment × ordering surface to characterize "+
			"by construction. It has no corpus, and asking for one is a category error, "+
			"not a coverage gap", row.Name, row.Class)
	}
	want := live
	if want == "" || want == "auto" {
		want = r.registry.OracleFor(row)
	}

	switch want {
	case OracleBash:
		if row.Status == StatusMigrated {
			return implementation{}, fmt.Errorf("refusing to run %q as a bash oracle: "+
				"%s marks it status: migrated, so `hook run %s` dispatches to the Go handler and "+
				"templates/hooks/%s is a shim.\n"+
				"  A recording captured now would characterize Go while claiming to characterize "+
				"bash, and the migration gate would then compare Go against Go — green, and proof "+
				"of nothing. Bash-oracle recordings must be captured BEFORE the row flips.",
				row.Name, r.registry.Path, row.Name, row.File)
		}
		path := filepath.Join(r.opts.HooksDir, row.File)
		body, err := os.ReadFile(path) //nolint:gosec // registry-declared path
		if err != nil {
			return implementation{}, fmt.Errorf("hook %q: %w", row.Name, err)
		}
		if looksLikeShim(body) {
			return implementation{}, fmt.Errorf("refusing to run %q as a bash oracle: %s is the "+
				"FR-004 exec shim (a short body whose logic is `hook run`), not a bash "+
				"implementation. Whatever it dispatches to is what would be recorded, under the "+
				"wrong label", row.Name, path)
		}
		digest, err := fileDigest(path)
		if err != nil {
			return implementation{}, err
		}
		return implementation{oracle: OracleBash, argv: []string{"bash", path}, digest: digest, path: path}, nil

	case OracleGo:
		bin, err := r.resolveBinary()
		if err != nil {
			return implementation{}, err
		}
		digest, derr := fileDigest(bin)
		if derr != nil {
			return implementation{}, fmt.Errorf("hashing %s: %w", bin, derr)
		}
		argv := []string{bin, "hook", "run", row.Name}
		if ev, ok := row.EventName(); ok {
			argv = append(argv, "--event", ev)
		}
		return implementation{oracle: OracleGo, argv: argv, digest: digest, path: bin}, nil
	}
	return implementation{}, fmt.Errorf("unknown implementation %q (want auto, bash or go)", live)
}

// EventName returns the row's event and whether it has one.
func (r RegistryRow) EventName() (string, bool) {
	if r.Event == nil {
		return "", false
	}
	return *r.Event, true
}

// resolveBinary finds a built bin/edikt, or explains every place it looked.
//
// AC-7.12: this NEVER degrades to "nothing to compare, so nothing failed". A
// gate that cannot find the thing it is gating has measured nothing, and
// reporting that as a pass is exactly INV-013's failure.
//
// AN EXPLICIT REQUEST IS NEVER SILENTLY SUBSTITUTED. If --bin or EDIKT_BIN
// names a path, that path is the answer or there is no answer. Falling back to
// another candidate would mean an operator who pointed the gate at a
// specific build — the release candidate, the branch build, the one they are
// actually trying to qualify — got a green result about a DIFFERENT binary,
// with nothing in the output saying so.
func (r *Recorder) resolveBinary() (string, error) {
	if explicit, source := r.explicitBinary(); explicit != "" {
		if err := usableBinary(explicit); err != nil {
			return "", fmt.Errorf("%s names %s, which is not a usable binary: %v.\n"+
				"  NOTHING WAS COMPARED — this is a failure, not an empty pass.\n"+
				"  An explicitly requested binary is never substituted for another: a green "+
				"result about a binary you did not ask for is worse than no result.",
				source, explicit, err)
		}
		return explicit, nil
	}

	var tried []string
	candidates := []string{filepath.Join(r.opts.RepoRoot, "bin", "edikt")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, exe)
	}
	for _, c := range candidates {
		tried = append(tried, c)
		if err := usableBinary(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no built edikt binary could be resolved, so NOTHING WAS COMPARED — "+
		"this is a failure, not an empty pass.\n"+
		"  Looked at: %s\n"+
		"  Build one with `cd tools/edikt && go build -o ../../bin/edikt .`, or pass --bin.",
		strings.Join(tried, ", "))
}

// explicitBinary returns the operator's stated choice and where it came from.
func (r *Recorder) explicitBinary() (string, string) {
	if r.opts.Binary != "" {
		return r.opts.Binary, "--bin"
	}
	if env := os.Getenv("EDIKT_BIN"); env != "" {
		return env, "EDIKT_BIN"
	}
	return "", ""
}

func usableBinary(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("is a directory")
	}
	if st.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("is not executable")
	}
	return nil
}

// Execution is one scenario's observed behaviour.
type Execution struct {
	ExitCode    int
	Stdout      []byte
	Stderr      []byte
	Diff        FSDiff
	Tokens      []string
	StdoutHash  string
	StderrHash  string
	WallClockMS int64
	Impl        implementation

	// Before and After are the sandbox snapshots the Diff was computed from.
	// They are carried out of Run rather than recomputed, because the sandbox
	// is gone the moment Run returns and a second execution would produce a
	// manifest describing a DIFFERENT run than the recording it sits beside.
	Before *Snapshot
	After  *Snapshot
}

// Run executes one scenario end to end in a fresh sandbox.
func (r *Recorder) Run(hook string, sc Scenario, live string) (*Execution, error) {
	row, err := r.registry.Row(hook)
	if err != nil {
		return nil, err
	}
	impl, err := r.resolveImplementation(row, live)
	if err != nil {
		return nil, err
	}

	base := BaseTree{Root: r.opts.BaseTree}
	sb, err := NewSandbox(r.opts.SandboxParent, base, r.opts.Trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sb.Remove() }()

	// THE SANDBOX CARRIES A REAL edikt, AT ADR-041's RUNG 3.
	//
	// Some hooks shell out to `$EDIKT_BIN hook match`. Without this, which
	// branch such a hook takes depends on the RECORDING MACHINE: a maintainer
	// with a global edikt on PATH records "binary resolved"; a CI box without
	// one records the binary-absent degradation banner. Same scenario, two
	// byte streams, and nothing in the fixture says which one you got.
	//
	// Installed here rather than in the shared `_base` tree because a base
	// tree is a minimal fixture and ValidateBaseTree enforces that with an
	// 8 MB budget — a 14 MB binary is exactly what that budget exists to keep
	// out. A symlink, not a copy: the sandbox is rebuilt per scenario.
	//
	// It is the SAME binary the Go replay runs, so a bash oracle recorded
	// through the subprocess and a Go handler doing the work in-process
	// produce the same journal records rather than differing by construction.
	// Resolved even for a BASH oracle, which is the whole point: the bash
	// shim is the caller that shells out, so it is the one that needs a
	// binary to reach. r.opts.Binary is only set when an operator named one,
	// so falling back to the ordinary resolution is what makes this work on a
	// plain `record` run. A resolution failure is deliberately NOT fatal here
	// — see installBinary — because most hooks never shell out and refusing
	// to record them would be a hard stop for a dependency they do not have.
	binForSandbox := r.opts.Binary
	if binForSandbox == "" {
		if resolved, err := r.resolveBinary(); err == nil {
			binForSandbox = resolved
		}
	}
	if err := sb.installBinary(binForSandbox); err != nil {
		return nil, err
	}

	defaultConfig, _ := os.ReadFile(filepath.Join(sb.Project, ".edikt", "config.yaml")) //nolint:gosec
	res := &Resolver{
		RepoRoot:      r.opts.RepoRoot,
		HookDir:       filepath.Join(r.opts.CorpusRoot, hook),
		DefaultConfig: defaultConfig,
	}
	inputs, err := sc.resolveInputs(res)
	if err != nil {
		return nil, err
	}

	norm, err := NewNormalizer(r.normalizerConfig(sb, inputs))
	if err != nil {
		return nil, err
	}

	// SEEDED BEFORE THE tree-before SNAPSHOT, deliberately. These files are
	// part of the world the scenario starts in — a compiled directive index
	// that already existed — not something the hook did. Seeding them after
	// the snapshot would list every one as "added" in fs_diff and read as an
	// effect of the hook under test.
	if len(inputs) > 0 {
		for dest, body := range inputs[0].files {
			full := filepath.Join(sb.Project, filepath.FromSlash(dest))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return nil, fmt.Errorf("seeding %s: %w", dest, err)
			}
			if err := os.WriteFile(full, body, 0o644); err != nil { //nolint:gosec // fixture inside the sandbox
				return nil, fmt.Errorf("seeding %s: %w", dest, err)
			}
		}
	}

	before, err := SnapshotTree(sb.Root, r.opts.Trace)
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	exitCode := 0
	simulated := r.opts.Now()
	started := time.Now()
	multi := len(inputs) > 1
	for i, in := range inputs {
		// The clock is ADVANCED, never slept. A dedup-TTL scenario asserting a
		// 60-second boundary must not cost 60 seconds, and a recorder that
		// slept would make the corpus unrunnable in CI for a reason that has
		// nothing to do with the hooks.
		simulated = simulated.Add(time.Duration(in.delayMS) * time.Millisecond)
		if in.configName != ConfigDefault && in.configName != "" {
			if werr := writeSandboxConfig(sb, in.config); werr != nil {
				return nil, werr
			}
		}
		code, out, errOut, rerr := r.exec(impl, sb, in, simulated)
		if rerr != nil {
			return nil, rerr
		}
		stdout.Write(out)
		stderr.Write(errOut)
		if multi {
			// Per-step exit codes would otherwise be lost behind the last
			// step's, and F-036 is precisely the finding that an exit code can
			// change while stdout does not. Emitted identically on both sides
			// of a comparison, so it is compared rather than merely recorded.
			fmt.Fprintf(&stderr, "[hookcorpus] step %d exit=%d\n", i, code)
		}
		exitCode = code
	}
	elapsed := time.Since(started)

	after, err := SnapshotTree(sb.Root, r.opts.Trace)
	if err != nil {
		return nil, err
	}
	diff, fsTokens := DiffTrees(before, after, norm)

	outHash, outTokens := norm.Hash(stdout.Bytes())
	errHash, errTokens := norm.Hash(stderr.Bytes())

	ex := &Execution{
		ExitCode:    exitCode,
		Stdout:      stdout.Bytes(),
		Stderr:      stderr.Bytes(),
		Diff:        diff,
		Tokens:      mergeTokens(outTokens, errTokens, fsTokens),
		StdoutHash:  outHash,
		StderrHash:  errHash,
		WallClockMS: elapsed.Milliseconds(),
		Impl:        impl,
		Before:      before,
		After:       after,
	}
	return ex, nil
}

func (r *Recorder) normalizerConfig(sb *Sandbox, inputs []resolvedInput) NormalizerConfig {
	cfg := HostNormalizerConfig()
	cfg.SandboxRoot = sb.Root
	cfg.ProjectRoot = sb.Project
	cfg.HomeDir = sb.Home
	cfg.SessionIDs = sessionIDsFrom(inputs)
	return cfg
}

// sessionIDsFrom pulls literal session ids out of the scenario's own payloads.
//
// Masking a session id by SHAPE alone would either miss host-specific formats
// or over-match; the ids a scenario actually carries are known exactly, so they
// are masked by value and the shape pass is only a backstop.
func sessionIDsFrom(inputs []resolvedInput) []string {
	seen := map[string]bool{}
	for _, in := range inputs {
		if !in.hasPayload {
			continue
		}
		for _, key := range []string{`"session_id"`, `"sessionId"`} {
			idx := bytes.Index(in.payload, []byte(key))
			if idx < 0 {
				continue
			}
			rest := in.payload[idx+len(key):]
			c := bytes.IndexByte(rest, ':')
			if c < 0 {
				continue
			}
			rest = rest[c+1:]
			open := bytes.IndexByte(rest, '"')
			if open < 0 {
				continue
			}
			rest = rest[open+1:]
			end := bytes.IndexByte(rest, '"')
			if end <= 0 {
				continue
			}
			seen[string(rest[:end])] = true
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func writeSandboxConfig(sb *Sandbox, body []byte) error {
	dir := filepath.Join(sb.Project, ".edikt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o644) //nolint:gosec
}

// exec runs one step.
//
// THE ENVIRONMENT IS BUILT, NOT INHERITED. Two reasons, and both are load
// bearing. Hermeticity: INV-007 forbids the harness handing a subprocess the
// maintainer's environment, and these hooks read EDIKT_*, HOME and
// CLAUDE_CONFIG_DIR. Reproducibility: an inherited variable is an unrecorded
// input, so a recording made on one machine would compare against a different
// scenario on another and the mismatch would look like a regression.
func (r *Recorder) exec(impl implementation, sb *Sandbox, in resolvedInput, now time.Time) (int, []byte, []byte, error) {
	env := map[string]string{
		"PATH":              os.Getenv("PATH"),
		"HOME":              sb.Home,
		"TMPDIR":            sb.Tmp,
		"PWD":               sb.Project,
		"LANG":              "C",
		"LC_ALL":            "C",
		"SHELL":             "/bin/bash",
		"CLAUDE_CONFIG_DIR": filepath.Join(sb.Home, ".claude"),
		"EDIKT_ROOT":        filepath.Join(sb.Home, ".edikt"),

		// The injected clock, exposed to the child. Nothing reads it today; it
		// is the seam an ordering-axis scenario uses instead of a sleep, and it
		// is set unconditionally so a hook that starts reading it does not
		// change the environment set out from under every existing recording.
		"EDIKT_CORPUS_NOW":    strconv.FormatInt(now.Unix(), 10),
		"EDIKT_CORPUS_NOW_MS": strconv.FormatInt(now.UnixMilli(), 10),
	}
	for k, v := range in.env {
		if v == nil {
			delete(env, k) // an explicit unset is an environment-axis fact
			continue
		}
		env[k] = *v
	}
	envv := make([]string, 0, len(env))
	for _, k := range sortedKeys(env) {
		envv = append(envv, k+"="+env[k])
	}

	cmd := exec.Command(impl.argv[0], impl.argv[1:]...) //nolint:gosec // argv is registry-derived
	cmd.Dir = sb.Project
	cmd.Env = envv
	if in.hasPayload {
		cmd.Stdin = bytes.NewReader(in.payload)
	} else {
		cmd.Stdin = bytes.NewReader(nil)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			return 0, nil, nil, fmt.Errorf("executing %s: %w", strings.Join(impl.argv, " "), err)
		}
	}
	return code, out.Bytes(), errOut.Bytes(), nil
}

func mergeTokens(lists ...[]string) []string {
	seen := map[string]bool{}
	for _, l := range lists {
		for _, t := range l {
			seen[t] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// RecordResult reports what one Record call produced.
type RecordResult struct {
	Hook      string
	Scenarios []string
	Oracle    string
	Dir       string
	WallClock map[string]int64
}

// Record captures every scenario in a hook's matrix (or one named scenario).
//
// Returns an error when NOTHING was recorded. A record run that matched no
// scenario has produced no evidence, and "0 scenarios, no errors" must never be
// mistaken for a corpus (INV-013).
func (r *Recorder) Record(hook, scenarioID string) (*RecordResult, error) {
	m, err := LoadMatrix(MatrixPath(r.opts.CorpusRoot, hook))
	if err != nil {
		return nil, err
	}
	if m.Hook != hook {
		return nil, fmt.Errorf("%s declares hook %q but lives under %q", m.Path, m.Hook, hook)
	}
	scenarios := m.Scenarios
	if scenarioID != "" {
		sc, serr := m.Scenario(scenarioID)
		if serr != nil {
			return nil, serr
		}
		scenarios = []Scenario{sc}
	}
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no scenarios selected for %q — nothing would be recorded", hook)
	}

	res := &RecordResult{Hook: hook, Dir: filepath.Join(r.opts.CorpusRoot, hook), WallClock: map[string]int64{}}
	for _, sc := range scenarios {
		rec, err := r.RecordOne(hook, sc)
		if err != nil {
			return nil, fmt.Errorf("scenario %q: %w", sc.ID, err)
		}
		res.Scenarios = append(res.Scenarios, sc.ID)
		res.Oracle = rec.Oracle
		res.WallClock[sc.ID] = rec.WallClockMS
		fmt.Fprintf(r.opts.Stderr, "recorded %s/%s: oracle=%s exit=%d %dms\n",
			hook, sc.ID, rec.Oracle, rec.ExitCode, rec.WallClockMS)
	}
	return res, nil
}

// RecordOne captures a single scenario and writes its artifacts.
func (r *Recorder) RecordOne(hook string, sc Scenario) (*Recording, error) {
	ex, err := r.Run(hook, sc, "auto")
	if err != nil {
		return nil, err
	}
	digest, err := r.inputDigest(hook, sc)
	if err != nil {
		return nil, err
	}
	rec := &Recording{
		SchemaVersion:            1,
		Hook:                     hook,
		ScenarioID:               sc.ID,
		ScenarioInputSHA256:      digest,
		NormalizerVersion:        Version,
		RecordedAt:               r.opts.Now().UTC().Format(time.RFC3339),
		Oracle:                   ex.Impl.oracle,
		OracleSHA256:             ex.Impl.digest,
		ExitCode:                 ex.ExitCode,
		StdoutSHA256:             ex.StdoutHash,
		StderrSHA256:             ex.StderrHash,
		StdoutBytes:              len(ex.Stdout),
		WallClockMS:              ex.WallClockMS,
		VolatileFieldsNormalized: ex.Tokens,
		FSDiff:                   ex.Diff,
	}
	if rec.VolatileFieldsNormalized == nil {
		rec.VolatileFieldsNormalized = []string{}
	}
	dir := RecordingDir(r.opts.CorpusRoot, hook, sc.ID)
	if err := WriteRecording(dir, rec, ex.Stdout, ex.Stderr, ex.Before, ex.After, r.guard, r.opts.KeepContent); err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *Recorder) inputDigest(hook string, sc Scenario) (string, error) {
	base := BaseTree{Root: r.opts.BaseTree}
	if err := ValidateBaseTree(base.Root, r.opts.Trace); err != nil {
		return "", err
	}
	defaultConfig, err := os.ReadFile(filepath.Join(base.Root, "project", ".edikt", "config.yaml")) //nolint:gosec
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	res := &Resolver{
		RepoRoot:      r.opts.RepoRoot,
		HookDir:       filepath.Join(r.opts.CorpusRoot, hook),
		DefaultConfig: defaultConfig,
	}
	return sc.InputDigest(res)
}
