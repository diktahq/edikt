package hook

// worktreecreate.go — the Go replacement for
// templates/hooks/worktree-create.sh (PLAN-v080-reliability-core Phase 14).
// Echoes the worktree path (WorktreeCreate's own protocol: print the path,
// or print nothing to accept the default) and, best-effort, copies
// governance (.edikt/config.yaml) into the new worktree. Idempotent: safe
// to run multiple times against the same worktree.
//
// worktree-create.sh's own `INPUT=$(cat)` runs BEFORE its python3 preflight
// — the no-python3 path below has no established stdout-JSON contract to
// extend (there is none: this hook's protocol is bare-text-or-nothing), so
// it degrades to stderr only, matching the bash original exactly.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("worktree-create", worktreeCreate)
}

func worktreeCreate(inv *Invocation) Result {
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: worktree-create — python3 not found; governance copy skipped, default path used")
		return Result{}
	}

	// `d.get("worktree_path") or d.get("proposed_path") or d.get("path") or
	//  d.get("cwd") or ""` — every .get() call here has NO default arg, so
	// each returns None (falsy) on absence regardless of presence; no
	// Present() dependency, unlike task-created.sh's TaskName default.
	path := inv.Payload.WorktreePath
	if path == "" {
		path = inv.Payload.ProposedPath
	}
	if path == "" {
		path = inv.Payload.TopLevelPath
	}
	if path == "" {
		path = inv.Payload.Cwd
	}
	// `if path and all(c not in path for c in "\n\r\t"): print(path)` — a
	// path containing any of those three bytes is silently dropped (python
	// never prints), which resolves WORKTREE_PATH empty on the bash side.
	if path == "" || strings.ContainsAny(path, "\n\r\t") {
		return Result{}
	}

	// `[ -L "$WORKTREE_PATH" ]` — a git worktree is always a real directory;
	// a symlink at that path is refused. Lstat on a nonexistent path errs,
	// which correctly takes the "not a symlink" branch (the hook fires
	// before git creates the worktree, so the dir may not exist yet).
	if fi, lerr := os.Lstat(path); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		return Result{ExitCode: 1}
	}

	if os.Getenv("EDIKT_WORKTREE_SKIP") != "1" {
		if fi, serr := os.Stat(path); serr == nil && fi.IsDir() {
			if sourceCfg := worktreeCreateFindSourceConfig(path); sourceCfg != "" {
				targetEdikt := filepath.Join(path, ".edikt")
				if !worktreeCreateIsDir(targetEdikt) {
					_ = os.MkdirAll(targetEdikt, 0o755)
					worktreeCreateBestEffortCopy(sourceCfg, filepath.Join(targetEdikt, "config.yaml"))
				}
				home := os.Getenv("HOME")
				_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
				ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
				if body, encErr := adapter.WorktreeEventJSON(ts, "worktree_create", path); encErr == nil {
					appendEventLine(filepath.Join(home, ".edikt", "events.jsonl"), body)
				}
			}
		}
	}

	// "Always echo the path" — required by the WorktreeCreate protocol,
	// unconditional on everything above.
	return Result{Response: adapter.Response{Text: path}}
}

// worktreeCreateFindSourceConfig mirrors:
//
//	D="$PWD"
//	_home="${HOME:-/nonexistent}"
//	while [ "$D" != "/" ] && [ "$D" != "$_home" ]; do
//	    if [ -f "$D/.edikt/config.yaml" ] && [ "$D" != "$WORKTREE_PATH" ] && [ -O "$D/.edikt/config.yaml" ]; then
//	        SOURCE_CFG="$D/.edikt/config.yaml"; break
//	    fi
//	    D=$(dirname "$D")
//	done
//
// "$D" != "$WORKTREE_PATH" is a plain STRING comparison against whatever the
// payload named — bash never resolves WORKTREE_PATH to an absolute path
// first — so a relative worktreePath is virtually never equal to the
// absolute $D the walk produces, ported here with the same string equality.
func worktreeCreateFindSourceConfig(worktreePath string) string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/nonexistent"
	}
	for d != "/" && d != home {
		candidate := filepath.Join(d, ".edikt", "config.yaml")
		if worktreeCreateFileExists(candidate) && d != worktreePath && worktreeCreateOwnedByCurrentUser(candidate) {
			return candidate
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

func worktreeCreateFileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func worktreeCreateIsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// worktreeCreateOwnedByCurrentUser mirrors bash's `[ -O path ]` — "path
// exists and is owned by the effective user id of the process". Unix-only
// (INV-001's target platforms are darwin/linux); syscall.Stat_t carries Uid
// identically on both.
func worktreeCreateOwnedByCurrentUser(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return int(st.Uid) == os.Geteuid()
}

// worktreeCreateBestEffortCopy mirrors `cp -- "$SOURCE_CFG" "$TARGET" 2>/dev/null || true`.
func worktreeCreateBestEffortCopy(src, dst string) {
	body, err := os.ReadFile(src) //nolint:gosec // source validated by worktreeCreateFindSourceConfig
	if err != nil {
		return
	}
	_ = os.WriteFile(dst, body, 0o644) //nolint:gosec // matches bash's own `cp` default mode
}
