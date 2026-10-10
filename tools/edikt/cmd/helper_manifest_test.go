package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestHelperManifest_UserAudienceOnly — the shipped permit manifest
// (templates/permits/tier2-verbs.yaml) admits audience: user only: a verb a
// tier-1 command may call is one a user's build carries. Dev tooling lives
// behind the edikt_dev build tag and is not in the manifest at all. The
// second half checks the binary this test was built into: without the tag
// `install benchmark` must be ABSENT from the command tree; with it, present.
func TestHelperManifest_UserAudienceOnly(t *testing.T) {
	root := repoRootForDoctorTests(t)
	raw, err := os.ReadFile(filepath.Join(root, "templates", "permits", "tier2-verbs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Verbs []struct {
			Verb     string `yaml:"verb"`
			Audience string `yaml:"audience"`
		} `yaml:"verbs"`
	}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Verbs) == 0 {
		t.Fatal("permit manifest declares zero verbs — nothing was checked")
	}
	for _, v := range m.Verbs {
		if v.Audience != "user" {
			t.Errorf("verb %q has audience %q; the shipped manifest admits user only", v.Verb, v.Audience)
		}
		if v.Verb == "benchmark" {
			t.Errorf("dev tooling (%q) is listed in the shipped manifest", v.Verb)
		}
	}
	var hasBenchmark bool
	for _, c := range installCmd.Commands() {
		if c.Name() == "benchmark" {
			hasBenchmark = true
		}
	}
	if hasBenchmark != devBuild {
		t.Fatalf("install benchmark present=%v but devBuild=%v — the build tag is not the only thing that decides", hasBenchmark, devBuild)
	}
}
