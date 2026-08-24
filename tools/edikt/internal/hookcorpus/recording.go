package hookcorpus

// recording.go — the artifact, its on-disk layout, and the guard that stops a
// private path from being committed inside one.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Recording is one scenario's captured behaviour.
//
// Schema: templates/schemas/hook-corpus-recording.v1.schema.json. Field order
// here matches the schema's `required` list so the two read as one document.
type Recording struct {
	SchemaVersion       int    `json:"schema_version"`
	Hook                string `json:"hook"`
	ScenarioID          string `json:"scenario_id"`
	ScenarioInputSHA256 string `json:"scenario_input_sha256"`
	NormalizerVersion   string `json:"normalizer_version"`
	RecordedAt          string `json:"recorded_at"`
	Oracle              string `json:"oracle"`
	OracleSHA256        string `json:"oracle_sha256,omitempty"`
	ExitCode            int    `json:"exit_code"`
	StdoutSHA256        string `json:"stdout_sha256"`
	StderrSHA256        string `json:"stderr_sha256"`
	StdoutBytes         int    `json:"stdout_bytes"`

	// WallClockMS is INFORMATIONAL and never compared. SPEC-014 spec.md:265
	// declines to set a runtime-path budget for the recorder, so a gate here
	// would cite a number the spec refuses to set.
	WallClockMS int64 `json:"wall_clock_ms"`

	VolatileFieldsNormalized []string `json:"volatile_fields_normalized"`
	FSDiff                   FSDiff   `json:"fs_diff"`
}

// RecordingDir is <corpus>/<hook>/recordings/<scenario-id>/.
func RecordingDir(corpusRoot, hook, scenario string) string {
	return filepath.Join(corpusRoot, hook, "recordings", scenario)
}

// MatrixPath is <corpus>/<hook>/matrix.yaml.
func MatrixPath(corpusRoot, hook string) string {
	return filepath.Join(corpusRoot, hook, "matrix.yaml")
}

// LoadRecording reads one recording.json.
func LoadRecording(dir string) (*Recording, error) {
	p := filepath.Join(dir, "recording.json")
	raw, err := os.ReadFile(p) //nolint:gosec // corpus path
	if err != nil {
		return nil, fmt.Errorf("recording: %w", err)
	}
	var r Recording
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("recording %s: %w", p, err)
	}
	if r.SchemaVersion != 1 {
		return nil, fmt.Errorf("recording %s: schema_version %d, want 1", p, r.SchemaVersion)
	}
	if r.ScenarioInputSHA256 == "" || r.NormalizerVersion == "" {
		return nil, fmt.Errorf("recording %s: missing scenario_input_sha256 or normalizer_version — "+
			"both are required pins; a recording without them cannot be compared safely", p)
	}
	if r.Oracle != OracleBash && r.Oracle != OracleGo {
		return nil, fmt.Errorf("recording %s: oracle %q is not %q or %q", p, r.Oracle, OracleBash, OracleGo)
	}
	return &r, nil
}

// LeakGuard refuses to write an artifact containing a host path that the
// sandbox should have made unreachable.
//
// WHY IT IS A REFUSAL AND NOT A SCRUB. If the real home path appears in the
// output of a hook that ran with HOME redirected into a sandbox, something
// reached the ambient environment through a channel the sandbox did not close.
// Quietly rewriting the bytes would hide that; committing them would put
// private content — session-start.sh reads $HOME/.claude/.../memory/MEMORY.md —
// into a public fixture tree. Stopping, loudly, is the only response that is
// both safe and informative.
type LeakGuard struct {
	// Needles are literal strings no committed artifact may contain.
	Needles []string
}

// NewLeakGuard builds a guard for the AMBIENT environment — the home the
// recorder process itself inherited, before any sandbox override.
func NewLeakGuard(ambientHome, ediktRoot string) LeakGuard {
	var needles []string
	for _, n := range []string{ambientHome, ediktRoot} {
		if n == "" || n == "/" {
			continue
		}
		needles = append(needles, filepath.Clean(n))
	}
	return LeakGuard{Needles: needles}
}

// Check reports the first needle found in b, if any.
func (g LeakGuard) Check(label string, b []byte) error {
	for _, n := range g.Needles {
		if i := bytes.Index(b, []byte(n)); i >= 0 {
			return fmt.Errorf("refusing to write %s: it contains the recorder's own host path %q "+
				"at offset %d.\n"+
				"  The scenario ran with HOME redirected into a sandbox, so a host path in its "+
				"output means the hook reached the ambient environment through a channel the "+
				"sandbox did not close.\n"+
				"  Committing this artifact would publish host-local content; scrubbing it would "+
				"hide the escape. Fix the escape, then re-record.", label, n, i)
		}
	}
	return nil
}

// WriteRecording persists a recording and its raw captures.
//
// Layout (SPEC-014 spec.md "Where recordings live"):
//
//	recording.json     the artifact the gate reads
//	stdout.raw         the pre-normalization bytes the hash was taken over
//	stderr.raw         same, for stderr
//	tree-before.json   the sandbox snapshot the fs_diff was computed from
//	tree-after.json    ditto, after the run
//
// The tree snapshots are path→hash manifests rather than content copies by
// default: the sandbox includes a fake $HOME, and a hook that copied something
// into it would otherwise land in a committed fixture. `--keep-content` opts
// into content copies for a reviewer who needs the actual bytes; the same leak
// guard applies to every one of them.
func WriteRecording(dir string, r *Recording, stdout, stderr []byte, before, after *Snapshot,
	guard LeakGuard, keepContent bool) error {
	for label, body := range map[string][]byte{"stdout.raw": stdout, "stderr.raw": stderr} {
		if err := guard.Check(filepath.Join(dir, label), body); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("recording dir: %w", err)
	}
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding recording: %w", err)
	}
	body = append(body, '\n')
	if err := guard.Check(filepath.Join(dir, "recording.json"), body); err != nil {
		return err
	}
	writes := []struct {
		name string
		body []byte
	}{
		{"recording.json", body},
		{"stdout.raw", stdout},
		{"stderr.raw", stderr},
	}
	manifestBefore, err := manifestBytes(before)
	if err != nil {
		return err
	}
	manifestAfter, err := manifestBytes(after)
	if err != nil {
		return err
	}
	writes = append(writes,
		struct {
			name string
			body []byte
		}{"tree-before.json", manifestBefore},
		struct {
			name string
			body []byte
		}{"tree-after.json", manifestAfter})

	for _, w := range writes {
		if err := os.WriteFile(filepath.Join(dir, w.name), w.body, 0o644); err != nil { //nolint:gosec
			return fmt.Errorf("writing %s: %w", w.name, err)
		}
	}
	if keepContent {
		if err := writeTreeContent(filepath.Join(dir, "tree-after"), after, r.FSDiff, guard); err != nil {
			return err
		}
	}
	return nil
}

type manifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

func manifestBytes(s *Snapshot) ([]byte, error) {
	entries := make([]manifestEntry, 0, len(s.Files))
	for _, p := range sortedKeys(s.Files) {
		entries = append(entries, manifestEntry{Path: p, SHA256: s.Files[p], Bytes: len(s.Raw[p])})
	}
	// A manifest carries its own denominator so a reader can tell an empty
	// sandbox from an unrecorded one (INV-013).
	doc := struct {
		FileCount int             `json:"file_count"`
		Files     []manifestEntry `json:"files"`
	}{len(entries), entries}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func writeTreeContent(dir string, after *Snapshot, d FSDiff, guard LeakGuard) error {
	touched := map[string]bool{}
	for _, a := range d.Added {
		touched[a.Path] = true
	}
	for _, m := range d.Modified {
		touched[m.Path] = true
	}
	for _, p := range sortedBoolKeys(touched) {
		body, ok := after.Raw[p]
		if !ok {
			continue
		}
		if err := guard.Check(filepath.Join(dir, p), body); err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil { //nolint:gosec
			return err
		}
	}
	return nil
}

// looksLikeShim reports whether a hook file is the FR-004 exec shim rather than
// a bash implementation.
//
// This is the second half of what makes `oracle: bash` an auditable claim. The
// registry's `status` field is the first half, and it is a fact maintained by a
// human in the same commit that lands a handler. This one is derived from the
// bytes actually about to be executed: a ≤12-line file whose body execs
// `hook run` is a dispatcher to the Go implementation, so a recording captured
// through it would characterize Go while claiming to characterize bash — the
// exact self-diff the gate exists to prevent, wearing the right label.
func looksLikeShim(body []byte) bool {
	if !bytes.Contains(body, []byte("hook run")) {
		return false
	}
	code := 0
	for _, line := range strings.Split(string(body), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		code++
	}
	return code <= 12
}

func fileDigest(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // caller-validated path
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
