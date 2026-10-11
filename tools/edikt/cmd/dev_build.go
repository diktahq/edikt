package cmd

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// errNoDevModule: the linked tree carries no tools/edikt module, so there is
// nothing to build — dev link falls back to whatever bin/edikt the tree holds.
var errNoDevModule = errors.New("no tools/edikt/go.mod in the dev source")

// goTool resolves the Go toolchain: PATH first, then the GOROOT this binary
// was built with (a test process usually has the latter and not the former).
func goTool() (string, error) {
	if p, err := exec.LookPath("go"); err == nil {
		return p, nil
	}
	//lint:ignore SA1019 fallback for a test process without go on PATH; the toolchain that built this binary is the right one there
	cand := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(cand); err == nil {
		return cand, nil
	}
	//lint:ignore SA1019 see above
	return "", fmt.Errorf("go toolchain not found on PATH or under GOROOT (%s)", runtime.GOROOT())
}

// buildDevLauncher builds <src>/bin/edikt from <src>/tools/edikt with the VCS
// stamp, and returns the stamp it read back from the built file. The stamp is
// the point: a launcher whose provenance is unknown is the binary-skew problem
// dev link exists to solve, wearing a different hat.
func buildDevLauncher(src string) (binaryProvenance, error) {
	mod := filepath.Join(src, "tools", "edikt")
	if _, err := os.Stat(filepath.Join(mod, "go.mod")); err != nil {
		return binaryProvenance{}, errNoDevModule
	}
	goBin, err := goTool()
	if err != nil {
		return binaryProvenance{}, err
	}
	out := filepath.Join(src, "bin", "edikt")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return binaryProvenance{}, fmt.Errorf("creating %s: %w", filepath.Dir(out), err)
	}
	run := func(buildvcs string) ([]byte, error) {
		cmd := exec.Command(goBin, "build", "-buildvcs="+buildvcs, "-o", out, ".")
		cmd.Dir = mod
		return cmd.CombinedOutput()
	}
	if b, err := run("true"); err != nil {
		// Not a VCS checkout at all (an exported tree): build unstamped and
		// let the caller say so. Any other failure is a failure.
		if !strings.Contains(string(b), "VCS") {
			return binaryProvenance{}, fmt.Errorf("go build in %s: %v\n%s", mod, err, strings.TrimSpace(string(b)))
		}
		if b2, err2 := run("false"); err2 != nil {
			return binaryProvenance{}, fmt.Errorf("go build in %s: %v\n%s", mod, err2, strings.TrimSpace(string(b2)))
		}
	}
	return readProvenanceOf(out)
}

// readProvenanceOf reads the vcs stamp of a binary on disk (not the running
// one — that is readBinaryProvenance).
func readProvenanceOf(path string) (binaryProvenance, error) {
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return binaryProvenance{}, fmt.Errorf("reading build info of %s: %w", path, err)
	}
	var p binaryProvenance
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			p.Revision = s.Value
		case "vcs.modified":
			p.Modified = s.Value == "true"
		}
	}
	return p, nil
}

// devLauncherPolicy decides whether the launcher just built from the tree may
// replace the user's `edikt`. A dirty tree is refused: the stamp would say
// "modified" and nothing could later prove which edits the launcher carries.
// An unstamped build (no VCS) is allowed and named by the caller.
func devLauncherPolicy(src string, p binaryProvenance) error {
	if p.Modified {
		return fmt.Errorf("dev link: refusing to swap the launcher — the tree at %s is DIRTY (vcs.modified=true at %s). "+
			"Commit or stash, then run `edikt dev link` again. The payload symlinks are in place; `edikt` on PATH is unchanged",
			src, short(p.Revision))
	}
	return nil
}
