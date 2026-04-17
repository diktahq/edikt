package cmd

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	buf, err := runCmd(t, "version")
	if err != nil {
		t.Fatalf("version: %v\n%s", err, buf)
	}
	if !strings.Contains(buf, ".") {
		t.Errorf("version output should contain a semver dot, got: %q", buf)
	}
}
