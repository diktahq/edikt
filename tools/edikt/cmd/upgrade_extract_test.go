package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractTarGz_FiltersAppleDoubleAndDSStore pins the macOS metadata
// filter in extractTarGz. The leak it closes: a payload tarball produced
// on macOS without COPYFILE_DISABLE=1 carries `._<name>` AppleDouble and
// `.DS_Store` entries; extracting them into ~/.edikt/.../commands creates
// phantom slash commands like `/edikt:._capture` that Claude Code
// enumerates and surfaces in the slash-command palette. The filter is
// belt-and-suspenders: install.sh / dev tooling should also export
// COPYFILE_DISABLE=1, but the extractor is the last line of defense.
func TestExtractTarGz_FiltersAppleDoubleAndDSStore(t *testing.T) {
	// Build a tarball in memory with a mix of legitimate and macOS-only
	// entries. The filter must keep the canonical files and skip the
	// metadata files at every directory level.
	entries := []struct {
		path string
		body string
	}{
		{"VERSION", "0.6.0\n"},
		{"._VERSION", "macos resource fork — should be skipped"},
		{".DS_Store", "macos finder metadata — should be skipped"},
		{"commands/edikt/context.md", "# context\n"},
		{"commands/edikt/._context.md", "macos sidecar — should be skipped"},
		{"commands/._edikt", "macos dir sidecar — should be skipped"},
		{"templates/foo.tmpl", "tmpl body\n"},
		{"templates/._foo.tmpl", "macos sidecar — should be skipped"},
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.path,
			Mode:     0o644,
			Size:     int64(len(e.body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "payload.tar.gz")
	if err := os.WriteFile(tarPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(tmpDir, "out")
	if err := extractTarGz(tarPath, dest); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	mustExist := []string{
		"VERSION",
		"commands/edikt/context.md",
		"templates/foo.tmpl",
	}
	for _, p := range mustExist {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Errorf("expected %s to be extracted: %v", p, err)
		}
	}

	mustNotExist := []string{
		"._VERSION",
		".DS_Store",
		"commands/edikt/._context.md",
		"commands/._edikt",
		"templates/._foo.tmpl",
	}
	for _, p := range mustNotExist {
		if _, err := os.Stat(filepath.Join(dest, p)); err == nil {
			t.Errorf("AppleDouble entry %s should have been skipped, but exists", p)
		}
	}

	// Also walk the destination tree and assert no `._*` or `.DS_Store`
	// entries snuck in via a different code path. The filter is meant to
	// be unconditional — this catches future regressions where someone
	// adds another extract call site without the filter.
	_ = filepath.Walk(dest, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, "._") || base == ".DS_Store" {
			t.Errorf("dest tree contains macOS metadata file: %s", p)
		}
		return nil
	})
}
