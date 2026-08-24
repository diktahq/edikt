package hookcorpus

// fixture_test.go — the fake repo every test in this package runs against.
//
// WHY A FAKE REPO AND NOT THE REAL ONE. These tests EXECUTE hooks. Running the
// real templates/hooks/*.sh would make the assertions depend on 22 files that
// change for unrelated reasons, and the recorder's own behaviour — sandboxing,
// snapshotting, normalization, refusal — is what is under test here, not any
// particular hook's logic. SPEC-014's Testing Strategy asks for exactly this:
// "a small fixture hook with a hand-verified matrix ... so the recorder's own
// logic has coverage independent of any real hook's complexity."
//
// The fixture hook is written to deliberately exercise every hard part: it
// emits a timestamp (so normalization must fire), prints $HOME (so a sandbox
// leak would be visible in the bytes), appends to $HOME/.edikt/ and TRUNCATES a
// seeded file there (the session-start.sh:28 shape), writes into the project
// tree, and takes its exit code from the environment (so the exit-code axis has
// something to move).

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixtureEnv is a complete fake repo.
type fixtureEnv struct {
	repo       string
	corpusRoot string
	hooksDir   string
	registry   string
	baseTree   string
	traced     []string
}

const fixtureHookBody = `#!/usr/bin/env bash
# A stand-in hook. Exercises: RFC3339 timestamps, $HOME echo, a $HOME append, a
# $HOME truncation, a project-tree write, stderr output, and a settable exit
# code. Nothing here is a real hook's logic; it is a surface for the recorder.
payload=$(cat)
printf '{"at":"%s","home":"%s","payload_bytes":%d}\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$HOME" "${#payload}"
mkdir -p "$HOME/.edikt"
printf 'fired at %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$HOME/.edikt/events.jsonl"
: > "$HOME/.edikt/gate-overrides.jsonl"
printf 'marker %s\n' "${EDIKT_FIXTURE_MARKER:-default}" > .edikt/last-fire.txt
echo "fixture-hook: ran" >&2
exit "${EDIKT_FIXTURE_EXIT:-0}"
`

// variantHookBody differs from fixtureHookBody in all three compared
// dimensions: one extra stdout field, one extra file, and a different exit
// code. It is what proves the gate can fail.
const variantHookBody = `#!/usr/bin/env bash
payload=$(cat)
printf '{"at":"%s","home":"%s","payload_bytes":%d,"extra":"CHANGED"}\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$HOME" "${#payload}"
mkdir -p "$HOME/.edikt"
printf 'fired at %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$HOME/.edikt/events.jsonl"
: > "$HOME/.edikt/gate-overrides.jsonl"
printf 'marker %s\n' "${EDIKT_FIXTURE_MARKER:-default}" > .edikt/last-fire.txt
printf 'unexpected\n' > .edikt/extra-file.txt
echo "fixture-hook: ran" >&2
exit 3
`

const slowHookBody = `#!/usr/bin/env bash
cat >/dev/null
sleep 0.05
echo "slow"
`

const shimHookBody = `#!/usr/bin/env bash
# The FR-004 exec shim shape.
EDIKT_BIN="${PWD}/bin/edikt"
[ -x "$EDIKT_BIN" ] || { printf 'edikt: binary absent\n' >&2; exit 0; }
exec "$EDIKT_BIN" hook run fixture-shim --event FileChanged
`

const fixtureRegistry = `schema_version: 1
hooks:
  - name: fixture-hook
    event: FileChanged
    file: fixture-hook.sh
    class: injector
    status: pending
    python3_baseline_count: 0
  - name: fixture-slow
    event: FileChanged
    file: fixture-slow.sh
    class: injector
    status: pending
    python3_baseline_count: 0
  - name: fixture-shim
    event: FileChanged
    file: fixture-shim.sh
    class: injector
    status: pending
    python3_baseline_count: 0
  - name: fixture-migrated
    event: FileChanged
    file: fixture-migrated.sh
    class: injector
    status: migrated
    python3_baseline_count: 0
  - name: fixture-lib
    event: null
    file: fixture-lib.sh
    class: library
    status: pending
    python3_baseline_count: 0
`

const fixtureMatrix = `schema_version: 1
hook: fixture-hook
scenarios:
- id: baseline
  axis:
  - payload
  payload: payloads/basic.json
  config: default
  notes: Baseline payload-axis scenario.
- id: env-exit-two
  axis:
  - environment
  payload: payloads/basic.json
  config: default
  env:
    EDIKT_FIXTURE_EXIT: '2'
    EDIKT_FIXTURE_MARKER: null
  notes: Environment-axis scenario that also moves the exit code (F-036 shape).
`

func newFixtureEnv(t *testing.T) *fixtureEnv {
	t.Helper()
	repo := t.TempDir()
	env := &fixtureEnv{
		repo:       repo,
		corpusRoot: filepath.Join(repo, "test", "fixtures", "hook-corpus"),
		hooksDir:   filepath.Join(repo, "templates", "hooks"),
	}
	env.registry = filepath.Join(env.hooksDir, "registry.yaml")
	env.baseTree = filepath.Join(env.corpusRoot, "_base")

	mustMkdir(t, env.hooksDir)
	mustWrite(t, env.registry, fixtureRegistry, 0o644)
	mustWrite(t, filepath.Join(env.hooksDir, "fixture-hook.sh"), fixtureHookBody, 0o755)
	mustWrite(t, filepath.Join(env.hooksDir, "fixture-slow.sh"), slowHookBody, 0o755)
	mustWrite(t, filepath.Join(env.hooksDir, "fixture-shim.sh"), shimHookBody, 0o755)
	mustWrite(t, filepath.Join(env.hooksDir, "fixture-migrated.sh"), shimHookBody, 0o755)

	// The minimal base tree: four files, two of them under the fake $HOME.
	mustMkdir(t, filepath.Join(env.baseTree, "project", ".edikt"))
	mustWrite(t, filepath.Join(env.baseTree, "project", ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\n", 0o644)
	mustWrite(t, filepath.Join(env.baseTree, "project", "CLAUDE.md"), "# sandbox\n", 0o644)
	mustMkdir(t, filepath.Join(env.baseTree, "home", ".edikt"))
	mustWrite(t, filepath.Join(env.baseTree, "home", ".edikt", "gate-overrides.jsonl"),
		"{\"seeded\":true}\n", 0o644)

	hookDir := filepath.Join(env.corpusRoot, "fixture-hook")
	mustMkdir(t, filepath.Join(hookDir, "payloads"))
	mustWrite(t, filepath.Join(hookDir, "matrix.yaml"), fixtureMatrix, 0o644)
	mustWrite(t, filepath.Join(hookDir, "payloads", "basic.json"),
		`{"session_id":"ses_fixture_abc123","tool_input":{"file_path":"/tmp/a b.txt"}}`+"\n", 0o644)
	return env
}

// options returns Options wired to this fixture, with tracing on.
func (e *fixtureEnv) options(t *testing.T) Options {
	t.Helper()
	return Options{
		RepoRoot:      e.repo,
		CorpusRoot:    e.corpusRoot,
		HooksDir:      e.hooksDir,
		RegistryPath:  e.registry,
		BaseTree:      e.baseTree,
		SandboxParent: t.TempDir(),
		Trace:         func(op, path string) { e.traced = append(e.traced, op+" "+path) },
		Now:           func() time.Time { return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC) },
		Stderr:        os.Stderr,
	}
}

func (e *fixtureEnv) recorder(t *testing.T) *Recorder {
	t.Helper()
	rec, err := NewRecorder(e.options(t))
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	return rec
}

// swapHook replaces the fixture hook's body — the stand-in for "someone
// rewrote this hook".
func (e *fixtureEnv) swapHook(t *testing.T, body string) {
	t.Helper()
	mustWrite(t, filepath.Join(e.hooksDir, "fixture-hook.sh"), body, 0o755)
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// hashHomeTree hashes a directory into one string, for the "was this touched"
// assertions. Returns "" when the directory does not exist.
func hashHomeTree(t *testing.T, root string) string {
	t.Helper()
	snap, err := SnapshotTree(root, nil)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("snapshot %s: %v", root, err)
	}
	out := ""
	for _, p := range sortedKeys(snap.Files) {
		out += p + " " + snap.Files[p] + "\n"
	}
	return out
}
