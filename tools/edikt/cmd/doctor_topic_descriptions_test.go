package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePendingTopicFile(t *testing.T, dir, topic, description string) {
	t.Helper()
	body := "topic: " + topic + "\ndescription: " + "\"" + description + "\"\n"
	writeFile(t, filepath.Join(dir, topic+".yaml"), body)
}

func TestTopicDescriptionsCheck_noDir(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	warn, ran := runTopicDescriptionsCheck(root, &out)
	if ran {
		t.Fatalf("expected ran=false when pending dir doesn't exist, got ran=true warn=%d\n%s", warn, out.String())
	}
}

func TestTopicDescriptionsCheck_empty(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".edikt", "state", "pending-topic-descriptions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var out bytes.Buffer
	warn, ran := runTopicDescriptionsCheck(root, &out)
	if !ran {
		t.Fatal("expected ran=true for an existing but empty dir")
	}
	if warn != 0 {
		t.Fatalf("expected 0 warnings for an empty dir, got %d\n%s", warn, out.String())
	}
}

func TestTopicDescriptionsCheck_mixedProposedAndEmpty(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".edikt", "state", "pending-topic-descriptions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writePendingTopicFile(t, dir, "security", "")
	writePendingTopicFile(t, dir, "auth", "Handles authentication and session boundaries")

	var out bytes.Buffer
	warn, ran := runTopicDescriptionsCheck(root, &out)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warn != 1 {
		t.Fatalf("expected 1 warning, got %d\n%s", warn, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "security") {
		t.Fatalf("must name the empty-slot topic:\n%s", s)
	}
	if !strings.Contains(s, "auth") {
		t.Fatalf("must name the proposed-but-unapproved topic:\n%s", s)
	}
	if !strings.Contains(s, "2 topic(s)") {
		t.Fatalf("must total both categories:\n%s", s)
	}
}
