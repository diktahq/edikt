package cmd

// Red-first record (PLAN-v091 phase 1, corrected 2026-10-10 after review).
// One-line mutation made to see each test in this file go red:
//
//	GatesOnNeverExitsFour: pointed `doctor --contract-state` at a walled verb
//	  (the old `gov reextract --contract-state` home) -> red, exit 4. Its
//	  control (`gov reextract --status` DOES exit 4 under the same conditions)
//	  goes red if the harness ever re-enables a gate bypass.
//	WritesNothing: made runContractState call emitEvent("verb_invoked", ...)
//	  -> red (events.jsonl appears under the edikt home). Its control
//	  (`gov reextract --status` with an invoker DOES change the snapshot) goes
//	  red if the snapshot cannot see a write.
//	SubdirectoryAgreesWithPhaseaPlan: made runContractState resolve the root
//	  from the working directory instead of projectroot.FindFromCwd -> red
//	  (doctor reports unmeasured from a subdirectory, phasea plan names v11).

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// gateEnv is an environment with NO edikt identity and NO gate bypass: the
// shape of an agent or script calling the binary cold. Anything EDIKT_* or
// CLAUDE_* in the developer's own environment is dropped first, so the test
// does not inherit the very invoker it is proving is unnecessary.
func gateEnv(home string, extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "EDIKT_") || strings.HasPrefix(kv, "CLAUDE_") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"HOME="+home,
		"CLAUDE_HOME="+filepath.Join(home, ".claude"),
		"CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"),
		"EDIKT_HOME="+filepath.Join(home, ".edikt"),
		"EDIKT_ROOT="+filepath.Join(home, ".edikt"),
	)
	return append(env, extra...)
}

// runGated execs the built binary with stdin a PIPE (not a terminal, not
// /dev/null: the binary treats any character device as a terminal) and exactly
// the env given. It does not route through runSandboxed, which turns both
// gates off.
func runGated(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()
	c := exec.Command(buildBinary(t), args...)
	c.Dir = dir
	c.Env = env
	c.Stdin = pr
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	code := 0
	if err := c.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("edikt %v did not run: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

func writeTemplateAgent(t *testing.T, version string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tpl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "sidecar-extractor.md")
	body := "---\nname: sidecar-extractor\nprompt_version: " + version + "\n---\n\nbody\n"
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// AC-1.7: both gates ON (no invoker, piped stdin, a project pinned below the
// floor), the report prints one line and never exits 4.
func TestContractState_GatesOnNeverExitsFour(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	bbWriteExtractorAgent(t, root, 11)
	bbWriteConfig(t, root, "edikt_version: \"0.7.2\"\n")
	env := gateEnv(home)

	// Control: the same conditions DO stop a walled, gated verb with exit 4.
	// If this ever exits anything else the harness cannot see a gate, and the
	// assertions below prove nothing.
	_, ctlErr, ctlCode := runGated(t, root, env, "gov", "reextract", "--status")
	if ctlCode != 4 {
		t.Fatalf("control: `gov reextract --status` under gates-on exited %d, want 4 (the harness cannot see a gate)\nstderr: %s", ctlCode, ctlErr)
	}

	for _, tc := range []struct {
		name     string
		args     []string
		wantCode int
		wantWord string
	}{
		{"installed", []string{"doctor", "--contract-state"}, 10, "v11"},
		{"agent-file validated", []string{"doctor", "--contract-state", "--agent-file", writeTemplateAgent(t, "4")}, 0, "v4"},
		{"agent-file rejected", []string{"doctor", "--contract-state", "--agent-file", writeTemplateAgent(t, "11")}, 10, "v11"},
		{"agent-file missing", []string{"doctor", "--contract-state", "--agent-file", filepath.Join(t.TempDir(), "sidecar-extractor.md")}, 11, "UNMEASURED"},
	} {
		out, errOut, code := runGated(t, root, env, tc.args...)
		if code == 4 {
			t.Fatalf("%s: exited 4 (a gate answered first)\nstdout: %s\nstderr: %s", tc.name, out, errOut)
		}
		if code != tc.wantCode {
			t.Errorf("%s: exit %d, want %d\nstdout: %s\nstderr: %s", tc.name, code, tc.wantCode, out, errOut)
		}
		if errOut != "" {
			t.Errorf("%s: stderr must be empty, got %q", tc.name, errOut)
		}
		if strings.TrimSpace(out) == "" || strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
			t.Errorf("%s: stdout must be exactly one non-empty line, got %q", tc.name, out)
		}
		if !strings.Contains(out, tc.wantWord) {
			t.Errorf("%s: stdout %q should name %q", tc.name, out, tc.wantWord)
		}
	}
}

type fileSig struct {
	size int64
	sum  string
	mode fs.FileMode
}

// snapshotTree maps every file under dir (relative path) to its size and
// sha256 and MODE; directories are recorded too (with their mode), so an empty
// directory appearing, or a chmod with identical bytes, is a change as well.
func snapshotTree(t *testing.T, dir string) map[string]fileSig {
	t.Helper()
	m := map[string]fileSig{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		if d.IsDir() {
			m[rel+"/"] = fileSig{mode: info.Mode()}
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		h := sha256.Sum256(b)
		m[rel] = fileSig{size: int64(len(b)), sum: hex.EncodeToString(h[:]), mode: info.Mode()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func sameSnapshot(a, b map[string]fileSig) (bool, string) {
	for k, v := range a {
		if w, ok := b[k]; !ok {
			return false, "removed: " + k
		} else if w != v {
			return false, "changed: " + k
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			return false, "added: " + k
		}
	}
	return true, ""
}

// AC-1.8: the report changes no file under the project or the edikt home.
func TestContractState_WritesNothing(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	bbWriteExtractorAgent(t, root, 11)
	bbWriteConfig(t, root, "")
	// A test-private TMPDIR, so a write under os.TempDir() lands somewhere the
	// snapshot can see.
	tmp := t.TempDir()
	env := gateEnv(home, "TMPDIR="+tmp)
	tpl := writeTemplateAgent(t, "4")
	tplDir := filepath.Dir(tpl)

	// home holds the edikt home, HOME, CLAUDE_HOME and CLAUDE_CONFIG_DIR.
	snap := func() (map[string]fileSig, map[string]fileSig) {
		return snapshotTree(t, root), snapshotTree(t, home)
	}
	beforeP, beforeH := snap()
	beforeT, beforeTmp := snapshotTree(t, tplDir), snapshotTree(t, tmp)
	for _, args := range [][]string{
		{"doctor", "--contract-state"},
		{"doctor", "--contract-state", "--agent-file", tpl},
	} {
		if out, errOut, code := runGated(t, root, env, args...); code != 0 && code != 10 && code != 11 {
			t.Fatalf("%v: unexpected exit %d\nstdout: %s\nstderr: %s", args, code, out, errOut)
		}
	}
	afterP, afterH := snap()
	if ok, why := sameSnapshot(beforeT, snapshotTree(t, tplDir)); !ok {
		t.Errorf("the report changed the directory holding the --agent-file template: %s", why)
	}
	if ok, why := sameSnapshot(beforeTmp, snapshotTree(t, tmp)); !ok {
		t.Errorf("the report wrote under TMPDIR: %s", why)
	}
	if ok, why := sameSnapshot(beforeP, afterP); !ok {
		t.Errorf("the report changed the project: %s", why)
	}
	if ok, why := sameSnapshot(beforeH, afterH); !ok {
		t.Errorf("the report changed the edikt home / user dirs: %s", why)
	}

	// Control: a walled verb with an invoker DOES leave a mark (the front-door
	// journal line), so this snapshot can see a write.
	ctlEnv := gateEnv(home, "EDIKT_INVOKER=ci", "EDIKT_SKIP_VERSION_GATE=1")
	if _, errOut, code := runGated(t, root, ctlEnv, "gov", "reextract", "--status"); code != 0 {
		t.Fatalf("control: gov reextract --status exit %d: %s", code, errOut)
	}
	_, ctlH := snap()
	if ok, _ := sameSnapshot(beforeH, ctlH); ok {
		t.Errorf("control: `gov reextract --status` with an invoker changed nothing; the snapshot cannot see a write")
	}
}

var versionInLineRe = regexp.MustCompile(`contract (v[0-9]+)`)

// The report and `phasea plan` resolve the project root through one finder,
// so from a subdirectory of a project with its own extractor and NO profile
// extractor they name the same prompt version.
func TestContractState_SubdirectoryAgreesWithPhaseaPlan(t *testing.T) {
	for _, tc := range []struct {
		version int
		want    string
	}{{11, "v11"}, {99, "v99"}} {
		p := csProject(t, tc.version)
		sub := filepath.Join(p.root, "docs", "deep")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		dOut, dErr, _ := csRun(t, sub, p.home, "doctor", "--contract-state")
		_, planErr, pcode := csRun(t, sub, p.home, "phasea", "plan")
		if pcode != 0 {
			t.Fatalf("%s: phasea plan from a subdirectory: exit %d\n%s", tc.want, pcode, planErr)
		}
		dm := versionInLineRe.FindStringSubmatch(dOut)
		pm := versionInLineRe.FindStringSubmatch(planErr)
		if dm == nil || pm == nil {
			t.Fatalf("%s: version not named: doctor %q (%s) / plan %q", tc.want, dOut, dErr, planErr)
		}
		if dm[1] != pm[1] || dm[1] != tc.want {
			t.Errorf("doctor names %s, phasea plan names %s, want %s", dm[1], pm[1], tc.want)
		}
	}
}
