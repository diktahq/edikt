package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `prd ship` appends to revision_history unconditionally, which is correct:
// running the verb is a fact worth recording, and a note may be the entire
// point of the invocation — closing a PRD whose requirements all shipped
// earlier is exactly that case.
//
// What must NOT happen is the entry claiming a ship that did not occur. The
// message used to read "Marked shipped:  — <note>" with an empty id list,
// which on later review is indistinguishable from a ship whose ids were lost.
// Two such entries exist in PRD-007's real history.
func TestPRDShip_EmptyAffectedDoesNotClaimAShip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PRD-999-test.yaml")
	body := `schema_version: '1.0'
type: prd
id: PRD-999
title: Test
slug: test
status: accepted
rigor: solo
author: Test
created_at: '2026-08-24T00:00:00Z'
requirements:
- id: FR-001
  text: Already done.
  status: shipped
  shipped_at: "2026-08-24T00:00:00Z"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newPRDShipCmd()
	cmd.SetArgs([]string{
		"--file", path,
		"--frs", "FR-001",
		"--at", "2026-08-24T12:00:00Z",
		"--author", "Test",
		"--note", "closing note",
	})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ship: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	if strings.Contains(got, "Marked shipped:  ") || strings.Contains(got, "Marked shipped: '") ||
		strings.Contains(got, "Marked shipped: —") {
		t.Errorf("history claims a ship with no ids:\n%s", got)
	}
	if !strings.Contains(got, "No requirements changed") {
		t.Errorf("history does not record that nothing changed:\n%s", got)
	}
	if !strings.Contains(got, "closing note") {
		t.Errorf("the note — often the whole point of the call — was dropped:\n%s", got)
	}
}
