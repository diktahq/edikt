package hookcorpus

// sandbox.go — one disposable tree per scenario, and the snapshot/diff that
// makes a hook's filesystem effects observable.
//
// # WHY THE SANDBOX IS BUILT FROM A FIXTURE, NEVER FROM THE PROJECT
//
// The obvious implementation copies the project the recorder is run in. Measured
// in this repo: .edikt/state alone is 214 MB across 26,089 files. At 22 hooks ×
// four axes that is over two million stat calls to record a corpus once — and
// every one of those files is an input the hook could read, so a recording made
// that way would also depend on the contents of the maintainer's local state
// directory and reproduce nowhere else.
//
// So the base tree is a MINIMAL, committed fixture, and this file refuses to
// build from anything that looks like a live project (a .git directory, an
// .edikt/state directory) or that exceeds a file/byte budget. The refusal comes
// BEFORE the walk, so a mistake costs a stat, not a 26k-file traversal.
//
// # WHY $HOME IS PART OF THE SANDBOX AND PART OF THE DIFF
//
// These hooks write to $HOME. session-start.sh:28 truncates
// $HOME/.edikt/gate-overrides.jsonl; six hooks append under $HOME/.edikt/;
// session-start.sh:49 reads the maintainer's private auto-memory. Running them
// against the developer's real home would corrupt their install and could pull
// private content into a committed stdout.raw.
//
// HOME is therefore redirected into the sandbox, and the fake home is snapshot
// and diffed in the SAME fs_diff as the project tree. That second half matters
// as much as the first: F-074's whole class of findings is a hook whose only
// observable on a healthy run is a side effect, and a $HOME write that the diff
// could not see would be exactly that kind of invisible regression.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Sandbox budgets. Deliberately small: the base tree is a fixture, and a
// fixture that has grown past these is a fixture that has stopped being minimal.
const (
	MaxBaseTreeFiles = 512
	MaxBaseTreeBytes = 8 << 20 // 8 MiB
)

// Trace observes every filesystem operation the sandbox performs.
//
// It exists so a TEST can assert what was touched, rather than a comment
// claiming it. TestRecorderNeverWalksProjectRoot reads this.
type Trace func(op, path string)

func (t Trace) emit(op, path string) {
	if t != nil {
		t(op, path)
	}
}

// Sandbox is one scenario's disposable world.
type Sandbox struct {
	Root    string // <tmp>/<random>
	Project string // <Root>/project — the child's cwd
	Home    string // <Root>/home    — the child's $HOME
	Tmp     string // <Root>/tmp     — the child's $TMPDIR

	trace Trace
}

// BaseTree describes the committed fixture a sandbox is built from.
//
// Layout:
//
//	<base>/project/...   copied to <sandbox>/project  (required)
//	<base>/home/...      copied to <sandbox>/home     (optional)
//
// The two halves are separate directories in the fixture for the same reason
// they are separate directories in the sandbox: a scenario that needs a
// pre-existing $HOME/.edikt/gate-overrides.jsonl — so that a truncation shows up
// as `modified` rather than vanishing — has to be able to say so.
type BaseTree struct{ Root string }

// ValidateBaseTree refuses a base tree that is not a minimal fixture.
//
// Every refusal here is a refusal to do something expensive or non-reproducible,
// and each names what it saw. The checks run before any walk.
func ValidateBaseTree(base string, tr Trace) error {
	if base == "" {
		return fmt.Errorf("no base tree configured — the recorder has no default and will not " +
			"fall back to the project root: a sandbox built from a live project walks its whole " +
			"state directory and records the maintainer's local files as inputs")
	}
	tr.emit("validate-base", base)
	st, err := os.Stat(base)
	if err != nil {
		return fmt.Errorf("base tree %s: %w", base, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("base tree %s is not a directory", base)
	}
	for _, marker := range []string{".git", filepath.Join(".edikt", "state")} {
		if _, err := os.Stat(filepath.Join(base, marker)); err == nil {
			return fmt.Errorf("base tree %s contains %s — this is a live project, not a corpus "+
				"fixture; refusing to walk or copy it (a sandbox built from one is neither "+
				"bounded nor reproducible)", base, marker)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "project")); err != nil {
		return fmt.Errorf("base tree %s has no project/ subdirectory: %w", base, err)
	}
	files, bytes, err := measureTree(base, tr)
	if err != nil {
		return err
	}
	if files == 0 {
		return fmt.Errorf("base tree %s contains zero files — an empty sandbox measures nothing "+
			"— nothing was measured", base)
	}
	if files > MaxBaseTreeFiles || bytes > MaxBaseTreeBytes {
		return fmt.Errorf("base tree %s holds %d files / %d bytes, over the %d file / %d byte "+
			"budget — a corpus base tree is a minimal fixture; this one has stopped being one",
			base, files, bytes, MaxBaseTreeFiles, MaxBaseTreeBytes)
	}
	return nil
}

func measureTree(root string, tr Trace) (files int, total int64, err error) {
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		tr.emit("walk", p)
		if d.IsDir() {
			return nil
		}
		files++
		if files > MaxBaseTreeFiles {
			// Stop early. The point of the budget is not to report an exact
			// count for an over-budget tree; it is to not pay for walking one.
			return fmt.Errorf("over file budget at %s", p)
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		total += info.Size()
		return nil
	})
	if err != nil && strings.Contains(err.Error(), "over file budget") {
		return MaxBaseTreeFiles + 1, total, nil
	}
	return files, total, err
}

// NewSandbox materializes a scenario's world under parent.
func NewSandbox(parent string, base BaseTree, tr Trace) (*Sandbox, error) {
	if err := ValidateBaseTree(base.Root, tr); err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(parent, "hookcorpus-")
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	sb := &Sandbox{
		Root:    root,
		Project: filepath.Join(root, "project"),
		Home:    filepath.Join(root, "home"),
		Tmp:     filepath.Join(root, "tmp"),
		trace:   tr,
	}
	if err := copyTree(filepath.Join(base.Root, "project"), sb.Project, tr); err != nil {
		return nil, err
	}
	srcHome := filepath.Join(base.Root, "home")
	if _, err := os.Stat(srcHome); err == nil {
		if err := copyTree(srcHome, sb.Home, tr); err != nil {
			return nil, err
		}
	} else if err := os.MkdirAll(sb.Home, 0o755); err != nil {
		return nil, fmt.Errorf("sandbox home: %w", err)
	}
	if err := os.MkdirAll(sb.Tmp, 0o700); err != nil {
		return nil, fmt.Errorf("sandbox tmp: %w", err)
	}
	return sb, nil
}

// installBinary links a built edikt into the sandbox home at the path
// ADR-041's rung 3 resolves ($EDIKT_ROOT/bin/edikt, and the recorder points
// EDIKT_ROOT at this home).
//
// A MISSING BINARY IS NOT AN ERROR HERE. Recording a bash oracle does not
// require one — most hooks never shell out — and failing the whole run would
// turn "this hook does not need a binary" into a hard stop. The hooks that DO
// need one say so by recording the binary-absent branch, which is a visible
// result rather than a silent substitution.
func (s *Sandbox) installBinary(bin string) error {
	if bin == "" {
		return nil
	}
	abs, err := filepath.Abs(bin)
	if err != nil {
		return nil //nolint:nilerr // see the doc comment: absence is recordable, not fatal
	}
	if fi, err := os.Stat(abs); err != nil || fi.IsDir() {
		return nil
	}
	dir := filepath.Join(s.Home, ".edikt", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sandbox binary dir: %w", err)
	}
	if err := os.Symlink(abs, filepath.Join(dir, "edikt")); err != nil {
		return fmt.Errorf("sandbox binary link: %w", err)
	}
	return nil
}

// Remove deletes the sandbox.
func (s *Sandbox) Remove() error { return os.RemoveAll(s.Root) }

// copyTree copies src to dst.
//
// Symlinks are RECREATED, never dereferenced, and a symlink whose target
// escapes src is refused by name (INV-007's escape rule). Dereferencing would
// let a fixture pull arbitrary host content into a sandbox whose whole purpose
// is that it contains only what the fixture declares.
func copyTree(src, dst string, tr Trace) error {
	realSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("base tree %s: %w", src, err)
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		tr.emit("copy-walk", p)
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&os.ModeSymlink != 0:
			link, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			resolved := link
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(p), link)
			}
			if !within(realSrc, filepath.Clean(resolved)) {
				return fmt.Errorf("base tree symlink %s -> %s escapes %s — refusing",
					p, link, realSrc)
			}
			return os.Symlink(link, target)
		default:
			tr.emit("copy-file", p)
			return copyFile(p, target)
		}
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // fixture path, validated by the caller
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// ── snapshots and the fs_diff ───────────────────────────────────────────────

// Snapshot is a tree state: relative path → content hash.
type Snapshot struct {
	Files map[string]string
	// Raw keeps the bytes of every file, so the diff can hash added/modified
	// content under the SAME normalization as stdout. Bounded by the sandbox
	// budget above, which is why holding it is affordable here and would not
	// be against a real project tree.
	Raw map[string][]byte
}

// excludedSnapshotPath is the one sandbox-relative path the snapshot
// deliberately never observes.
//
// "home/.edikt/state/hook-journal.jsonl" is dispatcher-owned bookkeeping
// (internal/hook/run.go's journal() call, via internal/hookmatch.Append) that
// EVERY `hook run` dispatch writes unconditionally, on success, on a panic, on
// an unresolved name — gated by nothing a handler controls and written by
// code this package deliberately does not import (SSP-002). A bash oracle
// recording never contains it, because bash hooks never wrote it; a live Go
// comparison always does. Diffing it would fail every single migrated hook's
// corpus, forever, for a reason that is not a regression in that hook's own
// behaviour — the journal write itself already has its own proof
// (TestJournalRecordFields, TestPanicRecovery in internal/hook), so this
// package's job is comparing HOOK behaviour, not re-proving dispatcher
// plumbing a different test suite already covers.
//
// A `const`, not a package-level `var` (still less a map): TestPackageIsolation
// forbids ANY package-level var here (SSP-002 — this package links into the
// same binary that runs on every hook fire, and a package-level map literal
// is exactly the "non-trivial package state" that rule exists to catch), and
// one excluded path needs no map to test against.
const excludedSnapshotPath = "home/.edikt/state/hook-journal.jsonl"

// SnapshotTree walks the whole sandbox — project AND fake home — and hashes it.
func SnapshotTree(root string, tr Trace) (*Snapshot, error) {
	snap := &Snapshot{Files: map[string]string{}, Raw: map[string][]byte{}}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// A file that vanished mid-walk is a real event in a tree a hook is
			// writing to; it is absent from this snapshot, which is the correct
			// record of it.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		tr.emit("snapshot", p)
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if rel == excludedSnapshotPath {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			// Never followed: the target's bytes are not this sandbox's fact.
			body := []byte("symlink -> " + link)
			snap.Raw[rel] = body
			snap.Files[rel] = hashBytes(body)
			return nil
		}
		body, rerr := os.ReadFile(p) //nolint:gosec // inside the sandbox
		if rerr != nil {
			if os.IsNotExist(rerr) {
				return nil
			}
			return rerr
		}
		snap.Raw[rel] = body
		snap.Files[rel] = hashBytes(body)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// FSDiff is the recorded tree delta (schema: hook-corpus-recording.v1 fs_diff).
type FSDiff struct {
	Added    []FSAdded    `json:"added"`
	Removed  []string     `json:"removed"`
	Modified []FSModified `json:"modified"`
}

// FSAdded is a path present after and absent before.
type FSAdded struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// FSModified is a path present both times with different content.
type FSModified struct {
	Path         string `json:"path"`
	SHA256Before string `json:"sha256_before"`
	SHA256After  string `json:"sha256_after"`
}

// DiffTrees computes the delta, hashing content through the normalizer.
//
// Content hashes go through the same masking as stdout because the files these
// hooks write carry the same volatile fields their stdout does — event-log.sh's
// JSONL line has an `at` timestamp, and an unmasked hash of it would differ on
// every run for a reason that is not a regression.
func DiffTrees(before, after *Snapshot, n *Normalizer) (FSDiff, []string) {
	d := FSDiff{Added: []FSAdded{}, Removed: []string{}, Modified: []FSModified{}}
	fired := map[string]bool{}
	note := func(tokens []string) {
		for _, t := range tokens {
			fired[t] = true
		}
	}

	afterPaths := make([]string, 0, len(after.Files))
	for p := range after.Files {
		afterPaths = append(afterPaths, p)
	}
	sort.Strings(afterPaths)

	for _, p := range afterPaths {
		beforeHash, existed := before.Files[p]
		hAfter, tokens := n.Hash(after.Raw[p])
		note(tokens)
		if !existed {
			d.Added = append(d.Added, FSAdded{Path: p, SHA256: hAfter})
			continue
		}
		if beforeHash == after.Files[p] {
			continue // identical raw bytes; no need to consult the normalizer
		}
		hBefore, tokensB := n.Hash(before.Raw[p])
		note(tokensB)
		if hBefore == hAfter {
			// The only difference was inside a mask. Recording it as modified
			// would make every run of a timestamp-writing hook look different
			// from every other; the masking is exactly what says it is not.
			continue
		}
		d.Modified = append(d.Modified, FSModified{Path: p, SHA256Before: hBefore, SHA256After: hAfter})
	}

	beforePaths := make([]string, 0, len(before.Files))
	for p := range before.Files {
		beforePaths = append(beforePaths, p)
	}
	sort.Strings(beforePaths)
	for _, p := range beforePaths {
		if _, still := after.Files[p]; !still {
			d.Removed = append(d.Removed, p)
		}
	}

	tokens := make([]string, 0, len(fired))
	for t := range fired {
		tokens = append(tokens, t)
	}
	sort.Strings(tokens)
	return d, tokens
}

// Equal compares two fs_diffs structurally: same paths added, removed and
// modified, with the same normalized content hashes.
//
// Returns the first difference in words, so a failing gate names what moved
// rather than printing two structures for a human to align by eye.
func (d FSDiff) Equal(other FSDiff) (bool, string) {
	if msg := compareAdded(d.Added, other.Added); msg != "" {
		return false, msg
	}
	if msg := compareRemoved(d.Removed, other.Removed); msg != "" {
		return false, msg
	}
	if msg := compareModified(d.Modified, other.Modified); msg != "" {
		return false, msg
	}
	return true, ""
}

func compareAdded(want, got []FSAdded) string {
	w := map[string]string{}
	for _, a := range want {
		w[a.Path] = a.SHA256
	}
	g := map[string]string{}
	for _, a := range got {
		g[a.Path] = a.SHA256
	}
	for _, p := range sortedKeys(w) {
		gh, ok := g[p]
		if !ok {
			return fmt.Sprintf("added file %q in the recording is not created by the live implementation", p)
		}
		if gh != w[p] {
			return fmt.Sprintf("added file %q differs in normalized content (recorded %s, live %s)",
				p, short(w[p]), short(gh))
		}
	}
	for _, p := range sortedKeys(g) {
		if _, ok := w[p]; !ok {
			return fmt.Sprintf("live implementation creates %q, which the recording does not", p)
		}
	}
	return ""
}

func compareRemoved(want, got []string) string {
	w := map[string]bool{}
	for _, p := range want {
		w[p] = true
	}
	g := map[string]bool{}
	for _, p := range got {
		g[p] = true
	}
	for _, p := range sortedBoolKeys(w) {
		if !g[p] {
			return fmt.Sprintf("recording removes %q; the live implementation does not", p)
		}
	}
	for _, p := range sortedBoolKeys(g) {
		if !w[p] {
			return fmt.Sprintf("live implementation removes %q; the recording does not", p)
		}
	}
	return ""
}

func compareModified(want, got []FSModified) string {
	w := map[string]FSModified{}
	for _, m := range want {
		w[m.Path] = m
	}
	g := map[string]FSModified{}
	for _, m := range got {
		g[m.Path] = m
	}
	for _, p := range sortedModKeys(w) {
		gm, ok := g[p]
		if !ok {
			return fmt.Sprintf("recording modifies %q; the live implementation does not", p)
		}
		if gm.SHA256After != w[p].SHA256After {
			return fmt.Sprintf("modified file %q differs in normalized content after the run "+
				"(recorded %s, live %s)", p, short(w[p].SHA256After), short(gm.SHA256After))
		}
	}
	for _, p := range sortedModKeys(g) {
		if _, ok := w[p]; !ok {
			return fmt.Sprintf("live implementation modifies %q; the recording does not", p)
		}
	}
	return ""
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedModKeys(m map[string]FSModified) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
