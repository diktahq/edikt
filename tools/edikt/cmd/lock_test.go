package cmd

// lock_test.go — SPEC-024 phase 3, AC-3.1 and AC-3.2.
//
// THERE WAS NO TEST FILE FOR cmd/lock.go AT ALL, and the writer is hand-rolled:
// marshalLock builds YAML with a WriteString per field, each guarded by a
// non-empty check. A field declared on the struct and forgotten in the
// marshaller therefore round-trips to ABSENT — no compile error, no test
// failure, just a key that is never written.
//
// That is a data-loss defect rather than a tidiness one, because absent
// provenance READS AS PUBLISHED (SSP-004): forget the one line that writes
// `source`, and every locally built payload starts reporting itself as a
// re-fetchable release, which is exactly the value that lets `edikt upgrade`
// replace it. One missing WriteString destroys an install.
//
// So the round-trip below is driven by REFLECTION over the struct rather than
// by a literal a future field would not appear in: a new field is filled
// because it exists, and if the marshaller does not write it the comparison
// fails naming it.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// spec024FillDistinct gives every leaf field under v a distinct, non-zero
// value derived from its path, and returns how many it set.
//
// A kind it has no strategy for is a FATAL error rather than a skip: a field
// silently left at its zero value would round-trip through a marshaller that
// never writes it and compare equal, which is the failure this whole file
// exists to make impossible.
func spec024FillDistinct(t *testing.T, v reflect.Value, path string, n *int) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		*n++
		v.SetString(fmt.Sprintf("%s-%d", path, *n))
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !v.Field(i).CanSet() {
				t.Fatalf("%s.%s is unexported, so this test cannot give it a value and cannot tell whether the marshaller writes it", path, f.Name)
			}
			spec024FillDistinct(t, v.Field(i), path+"."+f.Name, n)
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 2, 2)
		for i := 0; i < 2; i++ {
			spec024FillDistinct(t, s.Index(i), fmt.Sprintf("%s%d", path, i), n)
		}
		v.Set(s)
	default:
		t.Fatalf("%s is a %s and this test has no way to give it a distinct non-zero value — teach it one here rather than letting a new field round-trip unmeasured",
			path, v.Kind())
	}
}

// TestLockFile_ReflectionRoundTrip — AC-3.1.
func TestLockFile_ReflectionRoundTrip(t *testing.T) {
	var want LockFile
	set := 0
	spec024FillDistinct(t, reflect.ValueOf(&want).Elem(), "lock", &set)

	// The filler must have reached every leaf. LockFile has two []LockHistory
	// fields (History, StagedSources); each gets 2 synthetic entries, each
	// entry NumField(LockHistory) string leaves. The remaining LockFile
	// fields are plain string scalars, one leaf each.
	wantSet := reflect.TypeOf(LockFile{}).NumField() - 2 + 4*reflect.TypeOf(LockHistory{}).NumField()
	if set != wantSet {
		t.Fatalf("the filler set %d field(s); the struct shape says %d — the walk missed something, so a miss below would prove nothing", set, wantSet)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "lock.yaml"), []byte(marshalLock(want)), 0o644); err != nil {
		t.Fatalf("writing the marshalled lock: %v", err)
	}
	got, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}

	// FIELD BY FIELD, by name. reflect.DeepEqual on the two structs would say
	// "not equal" and leave the reader to find which key the writer dropped.
	wv, gv := reflect.ValueOf(want), reflect.ValueOf(got)
	for i := 0; i < wv.NumField(); i++ {
		f := wv.Type().Field(i)
		if f.Name == "History" || f.Name == "StagedSources" {
			continue
		}
		if wv.Field(i).Interface() != gv.Field(i).Interface() {
			t.Errorf("LockFile.%s (yaml %q) did not survive marshalLock -> readLock: wrote %q, read back %q. "+
				"A field declared on the struct and not written by marshalLock round-trips to ABSENT, and an absent source reads as published",
				f.Name, f.Tag.Get("yaml"), wv.Field(i).Interface(), gv.Field(i).Interface())
		}
	}
	assertLockHistorySliceRoundTrips(t, "history", want.History, got.History)
	assertLockHistorySliceRoundTrips(t, "staged_sources", want.StagedSources, got.StagedSources)

	// The field this phase adds, named explicitly. The reflection walk above
	// already covers it; naming it means a future edit that drops the walk
	// still cannot drop the provenance check silently.
	if got.Source != want.Source {
		t.Errorf("LockFile.source is the provenance the upgrade gate reads: wrote %q, read back %q", want.Source, got.Source)
	}
	if len(got.History) > 0 && got.History[0].Source != want.History[0].Source {
		t.Errorf("LockHistory[0].source: wrote %q, read back %q", want.History[0].Source, got.History[0].Source)
	}
	if len(got.StagedSources) > 0 && got.StagedSources[0].Source != want.StagedSources[0].Source {
		t.Errorf("StagedSources[0].source: wrote %q, read back %q", want.StagedSources[0].Source, got.StagedSources[0].Source)
	}
}

// assertLockHistorySliceRoundTrips checks a []LockHistory field (History or
// StagedSources) round-tripped through marshalLock -> readLock field by
// field, by name, so a mismatch names which key the writer dropped rather
// than just saying the slices differ.
func assertLockHistorySliceRoundTrips(t *testing.T, key string, want, got []LockHistory) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s round-tripped %d entr(ies), wrote %d", key, len(got), len(want))
	}
	for i := range want {
		wh, gh := reflect.ValueOf(want[i]), reflect.ValueOf(got[i])
		for j := 0; j < wh.NumField(); j++ {
			f := wh.Type().Field(j)
			if wh.Field(j).Interface() != gh.Field(j).Interface() {
				t.Errorf("%s[%d].%s (yaml %q) did not survive the round trip: wrote %q, read back %q",
					key, i, f.Name, f.Tag.Get("yaml"), wh.Field(j).Interface(), gh.Field(j).Interface())
			}
		}
	}
}

// TestWriteLock_RequiresProvenance — AC-3.2, the runtime half.
//
// The criterion's own point is that a test CANNOT observe this property on its
// own: `go test` passes just as happily against a variadic writeLock or one
// taking an options struct, because both compile at every existing call site
// while recording nothing. So the criterion asserts the SIGNATURE with a grep,
// and this test asserts the two things a grep cannot: that the signature is
// genuinely required (four non-variadic string parameters, no option struct to
// leave a field out of), and that provenance and installed-via are recorded as
// two DIFFERENT things rather than conflated into one.
func TestWriteLock_RequiresProvenance(t *testing.T) {
	ft := reflect.TypeOf(writeLock)
	if ft.IsVariadic() {
		t.Error("writeLock is variadic — an omitted source compiles, and an unrecorded source reads as published")
	}
	if ft.NumIn() != 4 {
		t.Fatalf("writeLock takes %d parameter(s), want 4 (ediktRoot, newActive, installedVia, source)", ft.NumIn())
	}
	for i := 0; i < ft.NumIn(); i++ {
		if ft.In(i).Kind() != reflect.String {
			t.Errorf("writeLock parameter %d is a %s; an aggregate parameter is a place a caller can leave provenance out of", i, ft.In(i).Kind())
		}
	}

	root := t.TempDir()
	if err := writeLock(root, "0.8.0-rc1", "launcher", lockSourceLocal); err != nil {
		t.Fatalf("writeLock: %v", err)
	}
	lf, err := readLock(root)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf.Source != lockSourceLocal {
		t.Errorf("lock.yaml records source %q, want %q — the value passed to writeLock is not what reached the file", lf.Source, lockSourceLocal)
	}
	if lf.InstalledVia != "launcher" {
		t.Errorf("lock.yaml records installed_via %q, want %q", lf.InstalledVia, "launcher")
	}
	if lf.InstalledVia == lf.Source {
		t.Error("installed_via and source hold the same value here, so this test could not tell one from the other — they answer different questions (how it was put there vs where it came from)")
	}
	if len(lf.History) != 1 || lf.History[0].Source != lockSourceLocal {
		t.Errorf("the history entry does not carry the provenance: %+v", lf.History)
	}

	// Absent is a state a caller may legitimately record — it is what a
	// migration from a flat layout knows — and it must reach the file as an
	// absence rather than as an invented value.
	root2 := t.TempDir()
	if err := writeLock(root2, "0.8.0", "migration", lockSourceUnknown); err != nil {
		t.Fatalf("writeLock: %v", err)
	}
	lf2, err := readLock(root2)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if lf2.Source != "" {
		t.Errorf("an unknown source was recorded as %q; a caller that cannot tell must not be made to claim", lf2.Source)
	}
	if effectiveLockSource(lf2.Source) != lockSourcePublished {
		t.Errorf("absent source resolves to %q, want %q (SSP-004: absent reads as published)", effectiveLockSource(lf2.Source), lockSourcePublished)
	}
}
