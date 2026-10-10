// Package permits reads the tier-1 -> tier-2 orchestration permit list
// (templates/permits/tier2-verbs.yaml) from an embedded, STRIPPED copy.
//
// WHY EMBEDDED, NOT READ FROM DISK. templates/permits/tier2-verbs.yaml
// lives in edikt-dev's own source tree. A consumer project that
// installed edikt does not have a templates/ directory at all — only
// this repo, building its own tooling against its own source, does.
// So the binary carries its own copy via go:embed (which cannot reach
// outside this Go module).
//
// WHY STRIPPED, NOT A BYTE-IDENTICAL MIRROR. The authoritative file's
// comments and its `provenance:` column cite internal ADR/INV/SPEC/PRD
// ids (D75(a)'s exact concern) as ordinary documentation. go:embed
// copies the raw bytes of whatever file it names verbatim into the
// compiled binary's data section — embedding the authoritative file
// unmodified made every one of those citations show up in a strings
// scan of bin/edikt, a real regression check-no-internal-refs.sh's
// binary scan caught. This mirror carries ONLY the two fields UserVerbs()
// actually reads (verb, audience) — no comments, no provenance — kept
// in sync with the authoritative file's verb+audience SET (not its
// bytes) by test/test-schema-copies.sh's dedicated semantic check.
package permits

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed tier2-verbs.yaml
var raw []byte

// Verb is one row of the permit list — deliberately narrower than the
// authoritative file's shape (no provenance): see the package doc for
// why the embedded mirror strips that field rather than carrying it.
type Verb struct {
	Verb     string `yaml:"verb"`
	Audience string `yaml:"audience"`
}

type permitFile struct {
	SchemaVersion int    `yaml:"schema_version"`
	Verbs         []Verb `yaml:"verbs"`
}

// UserVerbs returns the top-level verb names with audience: user, in
// file order. A parse failure returns (nil, err) — never a partial or
// guessed list, since callers gate a refusal on this set.
func UserVerbs() ([]string, error) {
	var f permitFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(f.Verbs))
	for _, v := range f.Verbs {
		if v.Audience == "user" {
			out = append(out, v.Verb)
		}
	}
	return out, nil
}
