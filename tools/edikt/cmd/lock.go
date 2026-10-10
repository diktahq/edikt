package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3" // used only by readLock (Unmarshal)
)

// LockFile represents the top-level fields of lock.yaml.
//
// Source is PROVENANCE: where the active payload came from, which is a
// different question from InstalledVia (how it was put there). A payload
// fetched from a published release can be re-fetched; one built from a local
// tree cannot, so `edikt upgrade` may replace the first and must not silently
// replace the second (SR-002, ruling SSP-004).
type LockFile struct {
	Active       string        `yaml:"active"`
	Previous     string        `yaml:"previous,omitempty"`
	Pinned       string        `yaml:"pinned,omitempty"`
	InstalledVia string        `yaml:"installed_via,omitempty"`
	Source       string        `yaml:"source,omitempty"`
	History      []LockHistory `yaml:"history,omitempty"`
	// StagedSources records provenance for versions `install` has staged,
	// one entry per version, and — unlike its name's original scope — it
	// keeps recording that provenance for as long as the version's payload
	// directory exists, active or not. It is a SEPARATE field from History
	// — not a slice sharing History's 49-entry cap — because a payload can
	// sit staged (or sit installed-but-inactive after a later `use`
	// switches away from it) through arbitrarily many unrelated lock writes,
	// and History alone would silently evict its provenance before an
	// operator ever activates it, or before a later rollback needs it again
	// (D394: the promise is UNFORGETTABLE, and a ring buffer forgets). An
	// entry is removed only when its version's payload directory is removed
	// (pruneStagedSource, called from `prune`) — NOT the moment its version
	// activates. That earlier rule sounded safe (the top-level Source field
	// and a fresh history entry both carry the active tag's provenance
	// forward) and was not: both are true only while the tag STAYS active.
	// The moment a different tag activates, the top-level Source now holds
	// that other tag's provenance and the fresh history entry sits in the
	// SAME ring the write just trimmed to 49 — so a tag that activates,
	// deactivates, and is later reactivated (rollback, dev unlink) has
	// nothing left recording where its payload came from (D396).
	StagedSources []LockHistory `yaml:"staged_sources,omitempty"`
}

// LockHistory is a single history entry in lock.yaml.
type LockHistory struct {
	Version      string `yaml:"version"`
	InstalledAt  string `yaml:"installed_at,omitempty"`
	ActivatedAt  string `yaml:"activated_at,omitempty"`
	InstalledVia string `yaml:"installed_via,omitempty"`
	Source       string `yaml:"source,omitempty"`
}

// The recorded provenance of a payload.
//
// lockSourceUnknown is what an install written before this field existed
// carries. It reads as published: that is the common case (every payload the
// launcher ever fetched) and refusing every pre-provenance install would
// strand them all. The asymmetry is deliberate — being stranded on an old
// release is visible and recoverable, losing a locally built payload is
// silent — so ONLY an explicit `local` refuses.
const (
	lockSourcePublished = "published"
	lockSourceLocal     = "local"
	lockSourceUnknown   = ""
)

// effectiveLockSource resolves a recorded source to the one used for
// decisions. Absent reads as published (SSP-004). A value this binary does not
// recognise also reads as published, and that is a KNOWN LIMIT rather than an
// oversight: the ruling names two states and an absent one, and inventing a
// third refusal for an unrecognised string would be this binary deciding
// something no owner has.
func effectiveLockSource(recorded string) string {
	if recorded == lockSourceLocal {
		return lockSourceLocal
	}
	return lockSourcePublished
}

// lockSourceFor returns the provenance recorded for version v: the top-level
// source when v is the active version, then StagedSources (the durable,
// non-evicting per-version record — see its field comment), then the most
// recent history entry naming it. Returns lockSourceUnknown when nothing
// recorded it.
//
// The order matters. The top-level Source is checked first because it is the
// only field an ACTIVE payload's provenance can be updated through after the
// fact (e.g. re-installing the currently active tag from a different source —
// recordInstalledSource writes there, not to StagedSources, precisely so this
// check sees it). StagedSources comes before History because it is the
// non-evicting record; falling through to History first would let an old,
// still-present history entry for the same version (from a previous
// activation) shadow a newer staged answer.
//
// Activating an ALREADY-INSTALLED payload (edikt use, edikt rollback, dev
// unlink) must carry that payload's provenance forward rather than restamp it
// as published — the payload on disk did not change, so neither did where it
// came from.
func lockSourceFor(lf LockFile, v string) string {
	v = normalizeTag(v)
	if normalizeTag(lf.Active) == v && lf.Source != "" {
		return lf.Source
	}
	for i := len(lf.StagedSources) - 1; i >= 0; i-- {
		if normalizeTag(lf.StagedSources[i].Version) == v && lf.StagedSources[i].Source != "" {
			return lf.StagedSources[i].Source
		}
	}
	for i := len(lf.History) - 1; i >= 0; i-- {
		if normalizeTag(lf.History[i].Version) == v && lf.History[i].Source != "" {
			return lf.History[i].Source
		}
	}
	return lockSourceUnknown
}

// readLock reads $EDIKT_ROOT/lock.yaml. Returns a zero-value LockFile
// (not an error) when the file does not exist.
func readLock(ediktRoot string) (LockFile, error) {
	p := filepath.Join(ediktRoot, "lock.yaml")
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return LockFile{}, nil
	}
	if err != nil {
		return LockFile{}, fmt.Errorf("reading lock.yaml: %w", err)
	}
	var lf LockFile
	// SPEC-009 Plan A AC-1.2: loads $EDIKT_ROOT/lock.yaml.  // edikt-guard:allow
	// Not *.edikt.yaml. KnownFields off intentional — lock file is edikt-managed; user doesn't extend it.
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return LockFile{}, fmt.Errorf("parsing lock.yaml: %w", err)
	}
	return lf, nil
}

// writeLock atomically writes lock.yaml at $EDIKT_ROOT/lock.yaml.
// newActive is the version being activated; installedVia is a short label
// ("launcher", "dev-link", etc.); source is the payload's PROVENANCE
// (lockSourcePublished, lockSourceLocal, or lockSourceUnknown when the caller
// genuinely cannot tell). The previous field is set from the current active
// (if different). History is preserved up to 50 entries.
//
// source is a REQUIRED POSITIONAL PARAMETER, not an option, and that is the
// point: a caller that flips `current` without saying where the payload came
// from does not compile. An options struct or a variadic would let the same
// call site keep compiling while recording nothing, and an unrecorded source
// reads as published — the value that permits an auto-advance over a local
// payload. The omission this shape forbids is the one that destroys data.
func writeLock(ediktRoot, newActive, installedVia, source string) error {
	existing, _ := readLock(ediktRoot)

	lf := LockFile{
		Active:       normalizeTag(newActive),
		InstalledVia: installedVia,
		Source:       source,
	}
	// Preserve or advance previous.
	if existing.Active != "" && existing.Active != newActive {
		lf.Previous = existing.Active
	} else if existing.Previous != "" {
		lf.Previous = existing.Previous
	}

	// Carry forward history (cap at 49 to leave room for new entry).
	hist := existing.History
	if len(hist) > 49 {
		hist = hist[len(hist)-49:]
	}
	hist = append(hist, LockHistory{
		Version:      normalizeTag(newActive),
		InstalledVia: installedVia,
		Source:       source,
	})
	lf.History = hist

	// Carry forward StagedSources UNCHANGED — including the tag now
	// activating (D396). Dropping that tag's entry here used to be
	// justified as "its provenance is now immune to eviction via the fresh
	// history entry just appended" — false: that entry is appended into the
	// SAME ring the lines above just trimmed to 49, so ~49 unrelated writes
	// evict it, and the top-level Source field only ever holds the
	// CURRENTLY active tag's provenance. So: install a payload locally,
	// use it, use something else, let ~49 ordinary lock writes pass, roll
	// back — the entry this used to drop was the only thing standing
	// between that rollback and a local payload silently reading as
	// published. An entry is removed only when its version's payload
	// directory is removed (pruneStagedSource, called from `prune`).
	lf.StagedSources = existing.StagedSources

	return atomicWriteLock(ediktRoot, lf)
}

// atomicWriteLock marshals lf and writes it to $EDIKT_ROOT/lock.yaml via a
// temp-file-plus-rename, shared by writeLock and recordInstalledSource so
// there is exactly one place that touches the file on disk.
func atomicWriteLock(ediktRoot string, lf LockFile) error {
	data := []byte(marshalLock(lf))

	p := filepath.Join(ediktRoot, "lock.yaml")
	tmp := p + fmt.Sprintf(".tmp.%d", os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing lock.yaml tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming lock.yaml: %w", err)
	}
	return nil
}

// recordInstalledSource records where a payload `install` just staged came
// from. It does NOT touch Active or Previous — install stages a payload, it
// does not activate one (reportInstallNotActivated already says so to the
// operator), so this must not make the new version look current.
//
// Without this, `edikt install` places a payload with no lock.yaml entry at
// all: lockSourceFor later finds nothing for that version, returns
// lockSourceUnknown, and effectiveLockSource reads that as published
// (SSP-004's deliberate default for a genuinely unknown provenance) — so a
// locally built payload staged by `install` and later activated by `use`
// reads as a re-fetchable release and a subsequent `upgrade` auto-advances
// over it, which is the exact loss SR-002 exists to prevent (D382).
//
// It ALWAYS upserts a StagedSources entry (the durable, non-evicting record
// — see that field's own comment), and additionally writes the top-level
// Source field when tag is the active version:
//
//   - StagedSources is written unconditionally, so a record made while tag
//     is active survives the moment a different tag activates and
//     overwrites the top-level field. Before this, only the top-level field
//     was written for the active-tag case, so a reinstall's provenance was
//     legible only until the NEXT `use` of a different tag — exactly the
//     defect D397 named ("the active-tag branch never staging at all").
//   - The top-level Source is ALSO written when tag IS the active version
//     (re-installing it, e.g. after its versions/ dir was removed and
//     `install` recreates it from a different source, or via the
//     already-installed refusal path in install.go which calls this
//     directly): lockSourceFor checks that field FIRST for the active tag,
//     before StagedSources or History are ever consulted, so nothing short
//     of updating it here would be read back immediately (D392).
//
// Idempotent by outcome: if lockSourceFor already resolves this version to
// this same source (e.g. `install` re-run over an existing payload), nothing
// is written.
func recordInstalledSource(ediktRoot, tag, source string) error {
	lf, err := readLock(ediktRoot)
	if err != nil {
		return err
	}
	tag = normalizeTag(tag)

	if lockSourceFor(lf, tag) == source {
		return nil
	}

	entry := LockHistory{
		Version:      tag,
		InstalledVia: "install",
		Source:       source,
		InstalledAt:  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	replaced := false
	for i := range lf.StagedSources {
		if normalizeTag(lf.StagedSources[i].Version) == tag {
			lf.StagedSources[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		lf.StagedSources = append(lf.StagedSources, entry)
	}

	if normalizeTag(lf.Active) == tag {
		lf.Source = source
	}

	return atomicWriteLock(ediktRoot, lf)
}

// pruneStagedSource removes tag's entry from lf.StagedSources in place,
// called when the version's payload directory under $EDIKT_ROOT/versions/
// is actually removed from disk (see cmd/prune.go) — the counterpart to
// writeLock no longer dropping an entry just because its tag stopped being
// active (D396). Reports whether anything changed, so a caller writing
// several tags in one pass (a `prune` run removing more than one version)
// can persist the lock file once rather than once per tag.
//
// History is untouched: it is an audit trail of past activations rather
// than a "does this payload still exist" record, and pruning it here would
// be a second, unrelated change to a field this fix does not concern.
func pruneStagedSource(lf *LockFile, tag string) bool {
	tag = normalizeTag(tag)
	kept := lf.StagedSources[:0]
	changed := false
	for _, s := range lf.StagedSources {
		if normalizeTag(s.Version) == tag {
			changed = true
			continue
		}
		kept = append(kept, s)
	}
	lf.StagedSources = kept
	return changed
}

// marshalLock produces a YAML string for LockFile with all string scalars
// double-quoted so tests and parsers get a consistent format.
//
// HAND-ROLLED, so every field must be written here as well as declared on the
// struct. A field added above and forgotten below round-trips to ABSENT, and
// an absent source reads as published — one missing WriteString is enough to
// auto-advance over a local payload. TestLockFile_ReflectionRoundTrip
// (lock_test.go) walks the struct by reflection precisely so that omission
// fails a test instead of destroying an install.
func marshalLock(lf LockFile) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("active: %q\n", lf.Active))
	if lf.Previous != "" {
		sb.WriteString(fmt.Sprintf("previous: %q\n", lf.Previous))
	}
	if lf.Pinned != "" {
		sb.WriteString(fmt.Sprintf("pinned: %q\n", lf.Pinned))
	}
	if lf.InstalledVia != "" {
		sb.WriteString(fmt.Sprintf("installed_via: %q\n", lf.InstalledVia))
	}
	if lf.Source != "" {
		sb.WriteString(fmt.Sprintf("source: %q\n", lf.Source))
	}
	writeLockHistoryBlock(&sb, "history", lf.History)
	writeLockHistoryBlock(&sb, "staged_sources", lf.StagedSources)
	return sb.String()
}

// writeLockHistoryBlock writes one []LockHistory field under key, shared by
// marshalLock's History and StagedSources blocks so there is exactly one
// place that knows a LockHistory entry's shape — the same reasoning that
// keeps marshalLock itself as one function: two copies of this loop is two
// places a future LockHistory field can be added to one and forgotten in the
// other.
func writeLockHistoryBlock(sb *strings.Builder, key string, entries []LockHistory) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString(key + ":\n")
	for _, h := range entries {
		sb.WriteString(fmt.Sprintf("    - version: %q\n", h.Version))
		if h.InstalledAt != "" {
			sb.WriteString(fmt.Sprintf("      installed_at: %q\n", h.InstalledAt))
		}
		if h.ActivatedAt != "" {
			sb.WriteString(fmt.Sprintf("      activated_at: %q\n", h.ActivatedAt))
		}
		if h.InstalledVia != "" {
			sb.WriteString(fmt.Sprintf("      installed_via: %q\n", h.InstalledVia))
		}
		if h.Source != "" {
			sb.WriteString(fmt.Sprintf("      source: %q\n", h.Source))
		}
	}
}

// normalizeTag strips a leading "v" from a version string.
func normalizeTag(tag string) string {
	return strings.TrimPrefix(tag, "v")
}
