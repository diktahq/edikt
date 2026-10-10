package cmd

// upgrade_versiongate_test.go — SPEC-024 phase 2, the consumer side of SR-002.
//
// The helper's own behaviour is asserted in internal/versioncmp. What is
// asserted HERE is that `package cmd` actually goes through it — all four call
// sites, not the three the requirement first named — and that each refusal
// `edikt upgrade` can produce says WHICH mechanism produced it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fx "github.com/diktahq/edikt/tools/edikt/internal/spec024fixtures"
	"github.com/diktahq/edikt/tools/edikt/internal/versioncmp"
)

// TestUpgradeGate_AllConsumersUseHelper — AC-2.2.
//
// SR-002 named three consumers — pinRemedy, semverGreater and semverMajor —
// and MISSED the one that actually fired. semverGreater("0.9.0","dev") was TRUE
// ([0,9,0] > [0,0,0]), so the already-up-to-date refusal never stopped a dev
// install; the minor-line gate at upgrade.go:144-145 did, by calling
// semverParts(...)[1] directly and finding 9 - 0 > 1. A migration that moved
// the named three and left that one behind would have deleted the only
// protection a dev install had while every grep-shaped check stayed green.
//
// So all FOUR sites are asserted, each through its own decision point:
//
//	upgrade.go:114     the already-up-to-date refusal
//	upgrade.go:120-121 the cross-major rejection
//	upgrade.go:144-145 the minor-line gate
//	pinwarn.go:130     pinRemedy
//
// The last subtest carries the sufficiency AC-2.1's grep cannot: a rename to
// cmpSemver inside `package cmd` satisfies a grep for the three old names and
// `go build` succeeds on every tree including the one before this change.
func TestUpgradeGate_AllConsumersUseHelper(t *testing.T) {
	t.Run("upgrade.go:114 — the already-up-to-date refusal orders through the helper", func(t *testing.T) {
		cases := []struct {
			current, latest string
			want            upgradeRefusal
			why             string
		}{
			{"dev", "0.9.0", refusalNotComparable,
				"a channel name has no position in a release ordering; before this change it read as 0.0.0 and every release compared newer"},
			{"0.9.0", "0.8.0", refusalUpToDate, "the latest release is older than what is active"},
			{"0.9.0", "0.9.0", refusalUpToDate, "the same version is not an upgrade"},
			{"0.8.0", "0.8.1", refusalNone, "an ordinary patch bump proceeds"},
			{"0.8.0-rc1", "0.8.0", refusalNone,
				"a pre-release sorts before its release (§11.3), so the release IS newer and the refusal correctly stops firing"},
		}
		for _, c := range cases {
			got, msg := upgradeVersionGate(c.current, c.latest)
			if got != c.want {
				t.Errorf("upgradeVersionGate(current=%q, latest=%q) = %q, want %q — %s\n%s",
					c.current, c.latest, got, c.want, c.why, msg)
			}
		}
	})

	t.Run("upgrade.go:120-121 — the cross-major rejection", func(t *testing.T) {
		if got, _ := upgradeVersionGate("0.9.0", "1.0.0"); got != refusalCrossMajor {
			t.Errorf("upgradeVersionGate(0.9.0, 1.0.0) = %q, want %q", got, refusalCrossMajor)
		}
		// The channel case must NOT reach this branch: semverMajor("dev") was
		// 0, so a 1.x release read as a major upgrade FROM a dev checkout —
		// a second wrong answer stacked on the first (fixture row VO-10).
		if got, _ := upgradeVersionGate("dev", "1.0.0"); got != refusalNotComparable {
			t.Errorf("upgradeVersionGate(dev, 1.0.0) = %q, want %q — a channel is not a major-version-0 install",
				got, refusalNotComparable)
		}
	})

	t.Run("upgrade.go:144-145 — the minor-line gate, the site SR-002's list omitted", func(t *testing.T) {
		if got, _ := upgradeVersionGate("0.7.0", "0.9.0"); got != refusalMinorSkip {
			t.Errorf("upgradeVersionGate(0.7.0, 0.9.0) = %q, want %q", got, refusalMinorSkip)
		}
		if got, _ := upgradeVersionGate("0.8.0", "0.9.0"); got != refusalNone {
			t.Errorf("upgradeVersionGate(0.8.0, 0.9.0) = %q, want %q — the adjacent line must still pass", got, refusalNone)
		}
		// THE MIGRATION THIS SUBTEST EXISTS FOR. This pair was refused before
		// the change, by this gate, via 9 - 0 > 1 — the right outcome for the
		// wrong reason. It must still be refused, and now by the mechanism
		// that can say why.
		got, msg := upgradeVersionGate("dev", "0.9.0")
		if got == refusalMinorSkip {
			t.Errorf("upgradeVersionGate(dev, 0.9.0) still refuses as a minor-line skip, which means the numeric distance from a channel name is still being computed:\n%s", msg)
		}
		if got != refusalNotComparable {
			t.Errorf("upgradeVersionGate(dev, 0.9.0) = %q, want %q", got, refusalNotComparable)
		}
	})

	t.Run("pinwarn.go:130 — pinRemedy", func(t *testing.T) {
		if got := pinRemedy("0.8.1-rc1", "0.8.1"); !strings.Contains(got, "behind") {
			t.Errorf("pinRemedy(0.8.1-rc1, 0.8.1) does not state the direction a pre-release ordering makes available:\n%s", got)
		}
		if got := pinRemedy("0.8.1", "0.8.1-rc1"); !strings.Contains(got, "AHEAD") {
			t.Errorf("pinRemedy(0.8.1, 0.8.1-rc1) does not report the pin as ahead:\n%s", got)
		}
		got := pinRemedy("0.8.1-rc1", "dev")
		if strings.Contains(got, "AHEAD") || strings.Contains(got, "behind") {
			t.Errorf("pinRemedy claims a direction against a channel install — SR-002's named falsification, verbatim:\n%s", got)
		}
	})

	t.Run("package cmd holds no version arithmetic of its own", func(t *testing.T) {
		// Scanned as SYNTAX, not as text. The comments in upgrade.go and in
		// the fixture harness name the deleted functions deliberately — that
		// is the record of what changed and why — and a grep cannot tell a
		// record from a call. Walking identifiers reads only what compiles.
		//
		// The needles are BUILT rather than written so this file does not
		// itself contain the names it forbids; Go folds the concatenation at
		// compile time.
		forbidden := map[string]bool{
			"semver" + "Parts":   true,
			"semver" + "Greater": true,
			"semver" + "Major":   true,
		}

		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
			return strings.HasSuffix(fi.Name(), ".go")
		}, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing the cmd package directory: %v", err)
		}
		scanned := 0
		for _, pkg := range pkgs {
			for name, file := range pkg.Files {
				scanned++
				ast.Inspect(file, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.Ident:
						if forbidden[node.Name] {
							t.Errorf("%s:%d still names %s as an identifier — the ordering moved to internal/versioncmp, and a copy left behind in package cmd is the two-implementations state SR-002 forbids",
								name, fset.Position(node.Pos()).Line, node.Name)
						}
					case *ast.FuncDecl:
						// The renamed-private-copy case AC-2.1's grep concedes
						// it cannot see. The deleted parser's signature is its
						// fingerprint: a [3]int of version components.
						if node.Type.Results == nil {
							return true
						}
						for _, r := range node.Type.Results.List {
							if arr, ok := r.Type.(*ast.ArrayType); ok {
								if lit, ok := arr.Len.(*ast.BasicLit); ok && lit.Value == "3" {
									if id, ok := arr.Elt.(*ast.Ident); ok && id.Name == "int" {
										t.Errorf("%s:%d — func %s returns [3]int, the deleted version parser's own signature; a rename is not a migration",
											name, fset.Position(node.Pos()).Line, node.Name.Name)
									}
								}
							}
						}
					}
					return true
				})
			}
		}
		// A scan that read nothing reports a pass while measuring nothing.
		if scanned < 2 {
			t.Fatalf("the scan parsed %d .go file(s) in the cmd package — that is not a measurement", scanned)
		}

		for _, f := range []string{"upgrade.go", "pinwarn.go"} {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("reading %s: %v", f, err)
			}
			if !strings.Contains(string(data), "internal/versioncmp") {
				t.Errorf("%s does not import internal/versioncmp, so whatever it orders versions with, it is not the one helper", f)
			}
		}
	})
}

// TestUpgradeRefusal_ReasonByName — AC-2.4.
//
// FOUR mechanisms refuse an `edikt upgrade`: isDevLinked, which reads the
// SYMLINK; the cross-major rejection; the minor-line gate; and, added by this
// change, "not comparable". A test asserting only a non-zero exit passes even
// if isDevLinked is deleted, because the incomparable path refuses the same
// input with the same exit status and a plausible message. So the reason is
// asserted BY NAME, and the dev-link case is given a version pair the version
// gate ACCEPTS — leaving the symlink as the only thing that can refuse it.
func TestUpgradeRefusal_ReasonByName(t *testing.T) {
	t.Run("dev-linked — refused by the filesystem, on a version pair that would otherwise proceed", func(t *testing.T) {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "versions", "dev"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("versions", "dev"), filepath.Join(root, "current")); err != nil {
			t.Fatal(err)
		}

		// The control: this pair is ACCEPTED by every version-derived gate, so
		// a refusal below cannot have come from one of them.
		if got, _ := upgradeVersionGate("0.8.0", "0.8.1"); got != refusalNone {
			t.Fatalf("the control pair (0.8.0 -> 0.8.1) is refused as %q, so this subtest could not tell a symlink refusal from a version one", got)
		}

		devLinked, target := isDevLinked(root)
		if !devLinked {
			t.Fatalf("isDevLinked(%s) = false — the fixture does not reproduce a dev-linked install", root)
		}
		reason, msg := upgradeDevLinkGate(devLinked, target)
		if reason != refusalDevLinked {
			t.Fatalf("upgradeDevLinkGate on a dev-linked root = %q, want %q", reason, refusalDevLinked)
		}
		if !strings.Contains(msg, target) {
			t.Errorf("the dev-link refusal does not name the symlink target it read (%s):\n%s", target, msg)
		}
		if !strings.Contains(msg, "dev-linked") {
			t.Errorf("the dev-link refusal does not say what it is:\n%s", msg)
		}
	})

	t.Run("each version-derived refusal names its own mechanism", func(t *testing.T) {
		cases := []struct {
			current, latest string
			want            upgradeRefusal
			phrase          string
		}{
			{"dev", "0.9.0", refusalNotComparable, "not comparable"},
			{"0.9.0", "0.8.0", refusalUpToDate, "already up to date"},
			{"0.9.0", "1.0.0", refusalCrossMajor, "major upgrade detected"},
			{"0.7.0", "0.9.0", refusalMinorSkip, "skips a minor line"},
		}
		for _, c := range cases {
			got, msg := upgradeVersionGate(c.current, c.latest)
			if got != c.want {
				t.Errorf("upgradeVersionGate(current=%q, latest=%q) = %q, want %q", c.current, c.latest, got, c.want)
				continue
			}
			if !strings.Contains(msg, c.phrase) {
				t.Errorf("the %q refusal does not say %q, so a reader has to infer which guard fired:\n%s", got, c.phrase, msg)
			}
		}
	})

	t.Run("the four reasons are distinct", func(t *testing.T) {
		seen := map[upgradeRefusal]bool{}
		for _, r := range []upgradeRefusal{
			refusalDevLinked, refusalNotComparable, refusalCrossMajor, refusalMinorSkip,
		} {
			if r == refusalNone {
				t.Error("a refusal reason is the empty string, which means `no refusal` — it would read as a pass")
			}
			if seen[r] {
				t.Errorf("refusal reason %q is used twice, so two mechanisms cannot be told apart", r)
			}
			seen[r] = true
		}
		if len(seen) != 4 {
			t.Errorf("expected four distinct refusal reasons, got %d: %v", len(seen), seen)
		}
	})
}

// TestFixtures_VersionOrdering — AC-2.5, measured through the cmd harness.
//
// internal/versioncmp asserts the same rows by calling the helper directly.
// This one goes through spec024OrderAsShipped, the seam the fixture harness
// uses, so a package that stopped consulting the helper would be caught here
// even while the helper itself stayed correct.
func TestFixtures_VersionOrdering(t *testing.T) {
	tbl := spec024Load(t)
	rows := tbl.Group("version-ordering")
	if len(rows) != spec024GroupCounts["version-ordering"] {
		t.Fatalf("the version-ordering group holds %d rows, expected %d", len(rows), spec024GroupCounts["version-ordering"])
	}

	measured := 0
	for _, row := range rows {
		if _, excluded := spec024VersionOrderingExclusions[row.ID]; excluded {
			continue
		}
		left, _ := row.Fields["left"].(string)
		right, _ := row.Fields["right"].(string)
		intended, _ := row.Fields["intended"].(string)
		got := spec024OrderAsShipped(left, right)
		measured++
		if got != intended {
			t.Errorf("%s: spec024OrderAsShipped(%q, %q) = %q, intended %q", row.ID, left, right, got, intended)
		}
	}
	if measured != len(rows)-len(spec024VersionOrderingExclusions) {
		t.Errorf("measured %d version-ordering rows, expected %d", measured, len(rows)-len(spec024VersionOrderingExclusions))
	}

	// The excluded set is exactly what it claims to be — one row, named, with
	// a reason. An exclusion list that can grow silently is how a criterion
	// stops covering the thing it names.
	if len(spec024VersionOrderingExclusions) != 1 {
		t.Errorf("exactly one row may be excluded from this gate; the list holds %d: %v",
			len(spec024VersionOrderingExclusions), spec024VersionOrderingExclusions)
	}
	if _, ok := spec024VersionOrderingExclusions["VO-12"]; !ok {
		t.Error("VO-12 is not the excluded row — the exclusion is specific to the empty-version case owned by the SR-010 phase")
	}
	if row, ok := tbl.Lookup("VO-12"); !ok {
		t.Error("VO-12 is missing from the table")
	} else if row.Agreement != fx.AgreeNo {
		t.Errorf("VO-12 records agrees_today: %q — this phase excludes it BECAUSE it still disagrees, so a flip here means it was closed by a phase that does not own it", row.Agreement)
	}
}

// spec024VersionOrderingExclusions names every row this gate does NOT assert,
// with the reason.
//
// VO-12 intends `error` for an empty version: the helper must make the caller
// HANDLE an absence rather than answer it. versioncmp returns `incomparable`
// instead, which is safe at every consumer and is not the same thing. The
// requirement that closes it guards an empty LATEST_VERSION in the tier-1 shell
// block, and it is a later phase's. Excluding it here is not a pass — the row
// stays recorded agrees_today: false, and the check just above fails if it
// stops being.
var spec024VersionOrderingExclusions = map[string]string{
	"VO-12": "empty version intends `error`; SR-010's phase owns it. versioncmp answers `incomparable`, which is safe but does not force the caller to handle the absence",
}

// A guard on the vocabulary the two sides share. The fixture records `intended`
// as a string and versioncmp returns an Ordering; they are compared directly,
// with no translation table, and that only stays true while the words agree.
func TestFixtures_VersionOrderingVocabularyIsShared(t *testing.T) {
	for _, o := range []versioncmp.Ordering{
		versioncmp.LeftNewer, versioncmp.RightNewer, versioncmp.Equal, versioncmp.Incomparable,
	} {
		if strings.TrimSpace(string(o)) == "" {
			t.Errorf("ordering constant %q has no fixture spelling", o)
		}
	}
	for word, want := range map[string]versioncmp.Ordering{
		"left_newer":   versioncmp.LeftNewer,
		"right_newer":  versioncmp.RightNewer,
		"equal":        versioncmp.Equal,
		"incomparable": versioncmp.Incomparable,
	} {
		if string(want) != word {
			t.Errorf("the fixture writes %q where versioncmp answers %q — one translation table away from the defect this spec is about", word, want)
		}
	}
}
