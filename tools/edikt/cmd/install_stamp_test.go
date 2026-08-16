package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStampPayloadVersion_overwritesStaleValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("0.6.0-rc4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stampPayloadVersion(dir, "0.6.0")

	got, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0.6.0\n" {
		t.Fatalf("VERSION = %q, want %q", got, "0.6.0\n")
	}
}

func TestStampPayloadVersion_createsWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	stampPayloadVersion(dir, "0.6.1")

	got, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0.6.1\n" {
		t.Fatalf("VERSION = %q, want %q", got, "0.6.1\n")
	}
}

func TestStampPayloadVersion_leavesMatchingFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")
	// Trailing whitespace variants of the same version must not be rewritten.
	if err := os.WriteFile(path, []byte("0.6.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	stampPayloadVersion(dir, "0.6.0")

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("VERSION was rewritten despite matching content")
	}
}
