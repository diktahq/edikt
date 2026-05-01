package cmd

// migrate.go — ports the bash `edikt migrate` command to Go.
//
// Faithfully implements the same migration steps as bin/edikt:
//   M1 — flat layout → versioned layout
//   M2 — CLAUDE.md HTML sentinels → markdown link-ref form
//   M3 — flat command names → namespaced
//   M4 — compile schema v1 → v2
//   M5 — config.yaml schema additions
//   M6 — no-op
//
// ADR-022 Phase 3: once this lands, bin/edikt-shell is deleted.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// semverRe matches a strict semver with optional pre-release/build metadata.
var semverRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([.\-][A-Za-z0-9]+)*$`)

// Events log rotation threshold — 10 MiB (matches bash constant).
const eventsMaxBytes = 10 * 1024 * 1024

// migrateEntries are the layout artifacts that M1 moves to the versioned dir.
var migrateEntries = []string{"hooks", "templates", "commands", "VERSION", "CHANGELOG.md"}

// preservedEntries are untouched by M1.
var preservedEntries = []string{"config.yaml", "custom", "backups", "events.jsonl", "lock.yaml"}

var migrateDryRun bool
var migrateYes bool
var migrateAbortFlag bool

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate a pre-v0.5.0 flat layout to the versioned layout",
	Long: `Migrate a pre-v0.5.0 flat layout (hooks/ as directory) into the versioned
layout. Dry-run previews the move plan without mutation. --yes bypasses the
interactive confirmation. --abort restores state from any crashed or in-progress
migration.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}
		claudeRoot := resolveClaudeRoot()
		return runMigrate(ediktRoot, claudeRoot, migrateDryRun, migrateYes, migrateAbortFlag)
	},
}

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "preview migration plan without making changes")
	migrateCmd.Flags().BoolVarP(&migrateYes, "yes", "y", false, "skip confirmation prompt")
	migrateCmd.Flags().BoolVar(&migrateAbortFlag, "abort", false, "restore state from a crashed or in-progress migration")
	rootCmd.AddCommand(migrateCmd)
}

// ─── Public entry-point ──────────────────────────────────────────────────────

func runMigrate(ediktRoot, claudeRoot string, dryRun, assumeYes, abortOnly bool) error {
	if abortOnly {
		if !migrationInProgress(ediktRoot) {
			fmt.Fprintln(os.Stderr, "nothing to abort (no staging or pre-migration directory found)")
			return nil
		}
		lck, unlk, err := acquireLock(ediktRoot)
		if err != nil {
			return err
		}
		_ = lck
		defer unlk()
		return doMigrateAbort(ediktRoot, "")
	}

	if !needsMigration(ediktRoot) && migrationInProgress(ediktRoot) {
		fmt.Fprintln(os.Stderr, "warn: partial migration detected. Run: edikt migrate --abort")
		return fmt.Errorf("partial migration in progress")
	}

	if !needsMigration(ediktRoot) {
		// Check for secondary migration signals only.
		if !hasSecondarySignal(ediktRoot, claudeRoot) {
			fmt.Fprintln(os.Stderr, "No migration needed.")
			return nil
		}

		if dryRun {
			fmt.Println("Migration plan (dry-run):")
			return runSecondaryMigrations(ediktRoot, claudeRoot, true, "")
		}

		_, unlk, err := acquireLock(ediktRoot)
		if err != nil {
			return err
		}
		defer unlk()

		ts := tsNow()
		backupDir := "" // will be created lazily by secondaries that need it
		summary, err := runSecondaryMigrationsCapture(ediktRoot, claudeRoot, false, ts, &backupDir)
		if err != nil {
			emitEvent(ediktRoot, "migration_partial_failure",
				map[string]interface{}{"step": "secondary", "backup": backupDir})
			fmt.Fprintf(os.Stderr, "error: secondary migration(s) failed — backups at %s/. Re-run 'edikt migrate' to retry, or 'edikt doctor' to inspect.\n",
				backupDir)
			return err
		}
		fmt.Println("\nMigration complete.\n\nMigration summary:")
		printSecondarySummary(summary, backupDir)
		return nil
	}

	// ── M1 is needed ──────────────────────────────────────────────────────────
	version, err := readVersion(ediktRoot)
	if err != nil {
		return err
	}
	if !semverRe.MatchString(version) {
		return fmt.Errorf("refusing to migrate: VERSION (%s) does not match semver pattern", version)
	}

	// Print plan.
	printM1Plan(ediktRoot, claudeRoot, version)

	if dryRun {
		fmt.Println("Secondary migrations (M2-M6) — detection plan:")
		_ = runSecondaryMigrations(ediktRoot, claudeRoot, true, "")
		fmt.Println("\n(dry-run: no changes written)")
		return nil
	}

	if !assumeYes {
		if err := confirmInteractive(); err != nil {
			return err
		}
	}

	// Set up signal handling — on SIGINT/SIGTERM call abort.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	_, unlk, err := acquireLock(ediktRoot)
	if err != nil {
		return err
	}
	defer unlk()

	ts := tsNow()
	staging := filepath.Join(ediktRoot, fmt.Sprintf(".migrate-staging-%s-%d", ts, os.Getpid()))
	predir := filepath.Join(ediktRoot, fmt.Sprintf(".pre-migration-%s-%d", ts, os.Getpid()))
	backupDir := filepath.Join(ediktRoot, "backups", fmt.Sprintf("migration-%s-%d", ts, os.Getpid()))

	// Async abort on signal.
	abortErrCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		// Only abort if the main goroutine hasn't completed M1 yet.
		select {
		case abortErrCh <- doMigrateAbort(ediktRoot, version):
		default:
		}
	}()

	if err := runM1(ctx, ediktRoot, claudeRoot, version, ts, staging, predir, backupDir); err != nil {
		// Try abort if not already done.
		select {
		case <-abortErrCh:
		default:
			_ = doMigrateAbort(ediktRoot, version)
		}
		return err
	}

	// Signal that M1 is complete so the abort goroutine doesn't fire.
	stop()

	// Run secondary migrations with an already-created backup dir.
	summary, secErr := runSecondaryMigrationsCapture(ediktRoot, claudeRoot, false, ts, &backupDir)
	if secErr != nil {
		emitEvent(ediktRoot, "migration_partial_failure",
			map[string]interface{}{"step": "secondary", "rc": 1, "backup": backupDir})
		fmt.Fprintf(os.Stderr, "error: secondary migration(s) failed — backups at %s/. Re-run 'edikt migrate' to retry.\n", backupDir)
		fmt.Println("\nMigration summary (partial — M1 succeeded, secondary step(s) failed):")
		fmt.Printf("  M1: flat → versioned (target %s)\n", version)
		printSecondarySummary(summary, backupDir)
		return secErr
	}

	fmt.Println("\nMigration complete.\n\nMigration summary:")
	fmt.Printf("  M1: flat → versioned (target %s)\n", version)
	printSecondarySummary(summary, backupDir)
	return nil
}

// ─── M1 ──────────────────────────────────────────────────────────────────────

func runM1(ctx context.Context, ediktRoot, claudeRoot, version, ts, staging, predir, backupDir string) error {
	// Create backup directory.
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return fmt.Errorf("creating backup dir: %w", err)
	}

	// Collect present entries.
	var present []string
	for _, e := range migrateEntries {
		p := filepath.Join(ediktRoot, e)
		if _, err := os.Lstat(p); err == nil {
			present = append(present, e)
		}
	}
	if len(present) == 0 {
		return fmt.Errorf("no legacy layout entries found to back up")
	}

	// Create pre-migration tarball.
	tarPath := filepath.Join(backupDir, "pre-migration.tar.gz")
	if err := createTarGzFromRoot(ediktRoot, tarPath, present); err != nil {
		return fmt.Errorf("creating pre-migration backup tarball: %w", err)
	}
	// Verify readability.
	if err := verifyTarGzReadable(tarPath); err != nil {
		return fmt.Errorf("pre-migration backup tarball is not readable: %w", err)
	}
	// SHA256 sidecar.
	hash, err := sha256File(tarPath)
	if err != nil {
		return fmt.Errorf("computing backup sha256: %w", err)
	}
	sidecar := tarPath + ".sha256"
	if err := os.WriteFile(sidecar, []byte(hash+"  pre-migration.tar.gz\n"), 0o640); err != nil {
		return fmt.Errorf("writing backup sha256 sidecar: %w", err)
	}
	fmt.Fprintf(os.Stderr, "migrate: wrote pre-migration backup to %s\n", tarPath)

	// Stage: copy each entry into .migrate-staging-<ts>/<version>/
	if err := os.Mkdir(staging, 0o755); err != nil {
		return fmt.Errorf("staging dir already exists: %s", staging)
	}
	stageVersionDir := filepath.Join(staging, version)
	if err := os.Mkdir(stageVersionDir, 0o755); err != nil {
		return fmt.Errorf("creating stage version dir: %w", err)
	}

	for _, e := range present {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		src := filepath.Join(ediktRoot, e)
		dst := filepath.Join(stageVersionDir, e)
		info, err := os.Lstat(src)
		if err != nil {
			continue
		}
		if info.IsDir() && !isSymlink(info) {
			if err := copyDirFull(src, dst); err != nil {
				return fmt.Errorf("staging %s: %w", e, err)
			}
		} else {
			if err := copyFilePath(src, dst, 0o644); err != nil {
				return fmt.Errorf("staging %s: %w", e, err)
			}
		}
	}

	// Write manifest.json.
	if err := writeManifestJSON(stageVersionDir, version, present); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Swap: move source entries to predir.
	if err := os.Mkdir(predir, 0o755); err != nil {
		return fmt.Errorf("creating predir: %w", err)
	}
	for _, e := range present {
		src := filepath.Join(ediktRoot, e)
		if _, err := os.Lstat(src); err == nil {
			dst := filepath.Join(predir, e)
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("moving %s sideways — will be restored by abort: %w", e, err)
			}
		}
	}

	// Move staging to versions/<version>.
	versionsDir := filepath.Join(ediktRoot, "versions")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		return fmt.Errorf("creating versions dir: %w", err)
	}
	targetVersionDir := filepath.Join(versionsDir, version)
	if _, err := os.Stat(targetVersionDir); err == nil {
		return fmt.Errorf("versions/%s already exists — refusing to overwrite", version)
	}
	if err := os.Rename(stageVersionDir, targetVersionDir); err != nil {
		// Cross-filesystem fallback.
		if err2 := copyDirFull(stageVersionDir, targetVersionDir); err2 != nil {
			return fmt.Errorf("installing version dir: %w", err2)
		}
		os.RemoveAll(stageVersionDir)
	}

	// Create symlinks: current → versions/<version>, hooks → current/hooks,
	// templates → current/templates, $CLAUDE_ROOT/commands/edikt → $EDIKT_ROOT/current/commands.
	currentLink := filepath.Join(ediktRoot, "current")
	if err := atomicSymlink(filepath.Join("versions", version), currentLink); err != nil {
		return fmt.Errorf("creating current symlink: %w", err)
	}
	if err := ensureExternalSymlinks(ediktRoot, claudeRoot); err != nil {
		return fmt.Errorf("creating external symlinks: %w", err)
	}

	// Write lock.yaml.
	if err := writeLock(ediktRoot, version, "migration"); err != nil {
		fmt.Fprintf(os.Stderr, "warn: activated but lock.yaml update failed: %v\n", err)
	}

	emitEvent(ediktRoot, "layout_migrated", map[string]interface{}{
		"from":    "flat",
		"to":      "versioned",
		"version": version,
		"backup":  backupDir,
	})

	// Clean up predir and staging.
	os.RemoveAll(predir)
	os.RemoveAll(staging)

	fmt.Fprintf(os.Stderr, "migration complete: %s activated\n", version)
	return nil
}

// ─── M2: CLAUDE.md HTML → markdown sentinels ─────────────────────────────────

type secondarySummary struct {
	ranM2         bool
	ranM3         bool
	ranM4         bool
	ranM5         bool
	m3Preserved   []string
}

func migrateM2ClaudemSentinels(ediktRoot string, dryRun bool, backupDir *string) (bool, error) {
	f := filepath.Join(ediktRoot, "CLAUDE.md")
	// File must exist and must not be a symlink.
	fi, err := os.Lstat(f)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("M2: stat failed: %w", err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return false, fmt.Errorf("M2: CLAUDE.md is a symlink — migration refuses to follow it")
	}

	// Read with O_NOFOLLOW.
	content, err := openNoFollow(f)
	if err != nil {
		return false, fmt.Errorf("M2: open failed: %w", err)
	}

	startToken := []byte("<!-- edikt:start -->")
	endToken := []byte("<!-- edikt:end -->")
	if !containsBytes(content, startToken) || !containsBytes(content, endToken) {
		return false, nil
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "M2 (CLAUDE.md sentinels): would rewrite HTML sentinels in %s to markdown link-ref form\n", f)
		return true, nil
	}

	// Ensure backup dir.
	if err := ensureSecondaryBackupDir(ediktRoot, backupDir); err != nil {
		return false, fmt.Errorf("M2: could not create backup dir: %w", err)
	}

	// Write backup.
	backupPath := filepath.Join(*backupDir, "CLAUDE.md.pre-m2")
	if err := os.WriteFile(backupPath, content, 0o640); err != nil {
		return false, fmt.Errorf("M2: backup write failed: %w", err)
	}

	// Rewrite.
	newContent := content
	newContent = replaceAll(newContent, startToken, []byte("[edikt:start]: #"))
	newContent = replaceAll(newContent, endToken, []byte("[edikt:end]: #"))

	// Atomic write with O_NOFOLLOW pre-check.
	if err := atomicWriteNoFollow(f, newContent, 0o644); err != nil {
		return false, fmt.Errorf("M2: atomic write failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "M2: rewrote sentinels in %s (backup at %s)\n", f, backupPath)
	emitEvent(ediktRoot, "migration_step_completed", map[string]interface{}{
		"step": "M2", "file": f,
	})
	return true, nil
}

// ─── M3: flat command names → namespaced ─────────────────────────────────────

func migrateM3FlatCommands(ediktRoot, claudeRoot string, dryRun bool) (bool, []string, error) {
	flatDir := filepath.Join(claudeRoot, "commands", "edikt")
	payloadDir := filepath.Join(ediktRoot, "current", "commands", "edikt")

	if _, err := os.Stat(flatDir); os.IsNotExist(err) {
		return false, nil, nil
	}

	// Check payload dir.
	if _, err := os.Stat(payloadDir); os.IsNotExist(err) {
		// Check for broken current symlink.
		if _, err2 := os.Lstat(filepath.Join(ediktRoot, "current")); err2 == nil {
			if _, err3 := os.Stat(filepath.Join(ediktRoot, "current")); err3 != nil {
				return false, nil, fmt.Errorf("M3: $EDIKT_ROOT/current is a broken symlink — cannot determine namespaced replacements")
			}
		}
		return false, nil, nil
	}

	// Find matching pairs: flat file → payload file at depth >= 2.
	type pair struct{ flat, replacement string }
	var pairs []pair

	flatEntries, err := os.ReadDir(flatDir)
	if err != nil {
		return false, nil, nil
	}
	for _, de := range flatEntries {
		if !de.Type().IsRegular() {
			continue
		}
		if de.Type()&fs.ModeSymlink != 0 {
			continue
		}
		base := de.Name()
		if !strings.HasSuffix(base, ".md") {
			continue
		}
		flatPath := filepath.Join(flatDir, base)
		// Lstat to confirm not a symlink (ReadDir can miss on some systems).
		fi, err := os.Lstat(flatPath)
		if err != nil || fi.Mode()&fs.ModeSymlink != 0 {
			continue
		}
		// Search payload at mindepth 2.
		repl, found := findAtMinDepth(payloadDir, base, 2)
		if !found {
			continue
		}
		pairs = append(pairs, pair{flat: flatPath, replacement: repl})
	}

	if len(pairs) == 0 {
		return false, nil, nil
	}

	if dryRun {
		for _, p := range pairs {
			fmt.Fprintf(os.Stderr, "M3 (flat commands): would compare %s vs %s\n", p.flat, p.replacement)
		}
		return true, nil, nil
	}

	customDir := filepath.Join(ediktRoot, "custom")
	var ran bool
	var preserved []string

	for _, p := range pairs {
		flatSHA, _ := sha256File(p.flat)
		replSHA, _ := sha256File(p.replacement)
		if flatSHA != "" && flatSHA == replSHA {
			// Unmodified: remove.
			if err := safeRemoveOrQuarantine(p.flat, "M3"); err != nil {
				return ran, preserved, fmt.Errorf("M3: failed to remove %s: %w", p.flat, err)
			}
			fmt.Fprintf(os.Stderr, "M3: removed unmodified flat command: %s\n", p.flat)
			emitEvent(ediktRoot, "migration_step_completed", map[string]interface{}{
				"step": "M3", "action": "removed", "file": p.flat,
			})
		} else {
			// User-modified: preserve.
			if err := os.MkdirAll(customDir, 0o755); err != nil {
				return ran, preserved, fmt.Errorf("M3: failed to create %s: %w", customDir, err)
			}
			dest := filepath.Join(customDir, filepath.Base(p.flat))
			if err := os.Rename(p.flat, dest); err != nil {
				return ran, preserved, fmt.Errorf("M3: failed to mv %s → %s: %w", p.flat, dest, err)
			}
			fmt.Fprintf(os.Stderr, "warn: Preserved user-modified command: %s → %s/\n",
				filepath.Base(p.flat), customDir)
			preserved = append(preserved, dest)
			emitEvent(ediktRoot, "migration_step_completed", map[string]interface{}{
				"step": "M3", "action": "preserved", "file": dest,
			})
		}
		ran = true
	}
	return ran, preserved, nil
}

// ─── M4: compile schema v1 → v2 ──────────────────────────────────────────────

func migrateM4CompileSchema(ediktRoot, claudeRoot string, dryRun bool) (bool, error) {
	gov := filepath.Join(claudeRoot, "rules", "governance.md")
	if _, err := os.Stat(gov); os.IsNotExist(err) {
		return false, nil
	}
	data, err := os.ReadFile(gov)
	if err != nil {
		return false, nil
	}
	if containsStr(string(data), "compile_schema_version: 2") {
		return false, nil
	}
	pendingMarker := filepath.Join(ediktRoot, ".m4-pending")
	if _, err := os.Stat(pendingMarker); err == nil {
		return false, nil
	}

	if dryRun {
		fmt.Fprintln(os.Stderr, "M4 (compile schema): would invoke 'claude -p /edikt:gov:compile' to migrate governance.md to schema v2")
		return true, nil
	}

	claudeBin, err := exec.LookPath("claude")
	if err == nil {
		fmt.Fprintln(os.Stderr, "M4 (compile schema): invoking 'claude -p /edikt:gov:compile' to upgrade governance.md to schema v2")
		out, err2 := exec.Command(claudeBin, "-p", "/edikt:gov:compile").CombinedOutput()
		_ = out
		if err2 == nil {
			// Check if schema v2 is now present.
			updated, _ := os.ReadFile(gov)
			if containsStr(string(updated), "compile_schema_version: 2") {
				emitEvent(ediktRoot, "migration_step_completed", map[string]interface{}{
					"step": "M4", "action": "compiled", "result": "schema_v2",
				})
				fmt.Fprintln(os.Stderr, "M4 (compile schema): governance.md upgraded to schema v2")
				return true, nil
			}
		}
		fmt.Fprintln(os.Stderr, "warn: M4: claude -p /edikt:gov:compile failed or produced no v2 sentinel — falling back to .m4-pending marker")
	} else {
		fmt.Fprintln(os.Stderr, "warn: M4: claude CLI not found — falling back to .m4-pending marker")
	}

	// Write .m4-pending marker.
	f, err2 := os.OpenFile(pendingMarker, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err2 != nil {
		fmt.Fprintf(os.Stderr, "warn: M4: could not write %s (non-fatal)\n", pendingMarker)
		return false, nil
	}
	f.Close()
	emitEvent(ediktRoot, "migration_step_completed", map[string]interface{}{
		"step": "M4", "action": "pending", "marker": pendingMarker,
	})
	fmt.Fprintln(os.Stderr, "warn: M4: governance.md schema migration pending — run /edikt:gov:compile to complete")
	return true, nil
}

// ─── M5: config.yaml schema additions ────────────────────────────────────────

func migrateM5ConfigAdditions(ediktRoot string, dryRun bool, backupDir *string) (bool, error) {
	f := filepath.Join(ediktRoot, "config.yaml")
	if _, err := os.Stat(f); os.IsNotExist(err) {
		return false, nil
	}
	data, err := os.ReadFile(f)
	if err != nil {
		return false, nil
	}
	content := string(data)

	var missing []string
	if !hasTopLevelKey(content, "paths") {
		missing = append(missing, "paths")
	}
	if !hasTopLevelKey(content, "stack") {
		missing = append(missing, "stack")
	}
	if !hasTopLevelKey(content, "gates") {
		missing = append(missing, "gates")
	}
	if len(missing) == 0 {
		return false, nil
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "M5 (config.yaml): would append missing keys to %s: %s\n",
			f, strings.Join(missing, ","))
		return true, nil
	}

	if err := ensureSecondaryBackupDir(ediktRoot, backupDir); err != nil {
		return false, fmt.Errorf("M5: could not create backup dir: %w", err)
	}
	backupPath := filepath.Join(*backupDir, "config.yaml.pre-m5")
	if err := copyFilePath(f, backupPath, 0o640); err != nil {
		return false, fmt.Errorf("M5: failed to back up %s: %w", f, err)
	}

	var extra strings.Builder
	if len(data) > 0 && data[len(data)-1] != '\n' {
		extra.WriteByte('\n')
	}
	for _, key := range missing {
		switch key {
		case "paths":
			extra.WriteString(`
# Added by edikt v0.5.0 migration
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  plans: docs/plans
  specs: docs/product/specs
  prds: docs/product/prds
  guidelines: docs/guidelines
  reports: docs/reports
  project-context: docs/project-context.md
`)
		case "stack":
			extra.WriteString(`
# Added by edikt v0.5.0 migration
stack: []
`)
		case "gates":
			extra.WriteString(`
# Added by edikt v0.5.0 migration
gates:
  quality-gates: true
`)
		}
	}

	newContent := append(data, []byte(extra.String())...)
	tmp := f + fmt.Sprintf(".tmp.%d", os.Getpid())
	if err := os.WriteFile(tmp, newContent, 0o644); err != nil {
		return false, fmt.Errorf("M5: failed to write temp file: %w", err)
	}
	if err := os.Rename(tmp, f); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("M5: atomic mv failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "M5: appended missing keys to %s: %s\n", f, strings.Join(missing, ","))
	emitEvent(ediktRoot, "migration_step_completed", map[string]interface{}{
		"step": "M5", "keys": strings.Join(missing, ","),
	})
	return true, nil
}

// ─── Secondary migrations coordinator ────────────────────────────────────────

// runSecondaryMigrations prints in dry-run mode.
func runSecondaryMigrations(ediktRoot, claudeRoot string, dryRun bool, ts string) error {
	backupDir := ""
	_, err := runSecondaryMigrationsCapture(ediktRoot, claudeRoot, dryRun, ts, &backupDir)
	return err
}

func runSecondaryMigrationsCapture(ediktRoot, claudeRoot string, dryRun bool, ts string, backupDir *string) (secondarySummary, error) {
	var s secondarySummary

	ranM2, err := migrateM2ClaudemSentinels(ediktRoot, dryRun, backupDir)
	if err != nil {
		return s, err
	}
	s.ranM2 = ranM2

	ranM3, preserved, err := migrateM3FlatCommands(ediktRoot, claudeRoot, dryRun)
	if err != nil {
		return s, err
	}
	s.ranM3 = ranM3
	s.m3Preserved = preserved

	ranM5, err := migrateM5ConfigAdditions(ediktRoot, dryRun, backupDir)
	if err != nil {
		return s, err
	}
	s.ranM5 = ranM5

	ranM4, err := migrateM4CompileSchema(ediktRoot, claudeRoot, dryRun)
	if err != nil {
		return s, err
	}
	s.ranM4 = ranM4

	// M6 is a no-op.
	return s, nil
}

func printSecondarySummary(s secondarySummary, backupDir string) {
	ranAny := false
	if s.ranM2 {
		fmt.Println("  M2: CLAUDE.md sentinels rewritten")
		ranAny = true
	}
	if s.ranM3 {
		fmt.Println("  M3: flat command names normalized")
		ranAny = true
		for _, f := range s.m3Preserved {
			fmt.Printf("    user-modified files preserved: %s\n", f)
		}
	}
	if s.ranM5 {
		fmt.Println("  M5: config.yaml keys added")
		ranAny = true
	}
	if s.ranM4 {
		fmt.Println("  M4: compile schema marked pending (deferred to Phase 7b)")
		ranAny = true
	}
	if !ranAny {
		fmt.Println("  (M2-M6: no signals detected)")
	}
	if (s.ranM2 || s.ranM5) && backupDir != "" {
		fmt.Printf("\nMigration backups retained at: %s/\n", backupDir)
		if s.ranM2 {
			fmt.Println("  CLAUDE.md.pre-m2    — M2 original")
		}
		if s.ranM5 {
			fmt.Println("  config.yaml.pre-m5  — M5 original")
		}
		fmt.Println("\nTo restore manually:")
		if s.ranM2 {
			fmt.Printf("  cp %s/CLAUDE.md.pre-m2 <path-to-CLAUDE.md>\n", backupDir)
		}
		if s.ranM5 {
			fmt.Printf("  cp %s/config.yaml.pre-m5 %s/config.yaml\n", backupDir, filepath.Dir(backupDir))
		}
	}
}

// ─── Abort ────────────────────────────────────────────────────────────────────

func doMigrateAbort(ediktRoot, version string) error {
	any := false

	// Find pre-migration dirs.
	predirs, _ := filepath.Glob(filepath.Join(ediktRoot, ".pre-migration-*"))
	stagings, _ := filepath.Glob(filepath.Join(ediktRoot, ".migrate-staging-*"))

	havePre := false
	for _, d := range predirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			havePre = true
			break
		}
	}

	willTouchVersions := version != "" && havePre
	if fi, _ := os.Stat(filepath.Join(ediktRoot, "versions", version)); fi != nil {
		_ = fi
	} else {
		willTouchVersions = false
	}

	if havePre || willTouchVersions {
		bkDir := findLatestBackupDir(ediktRoot)
		if bkDir == "" {
			fmt.Fprintf(os.Stderr, "error: migrate-abort: no readable backup tarball found under %s/backups/migration-*\n", ediktRoot)
			fmt.Fprintf(os.Stderr, "error: migrate-abort: refusing to mutate state without a verified backup\n")
			fmt.Fprintf(os.Stderr, "error: migrate-abort: inspect %s/.pre-migration-*/ and %s/versions/ manually\n", ediktRoot, ediktRoot)
			return fmt.Errorf("no backup available for abort")
		}
		fmt.Fprintf(os.Stderr, "migrate-abort: verified backup at %s/pre-migration.tar.gz\n", bkDir)
	}

	for _, pre := range predirs {
		fi, err := os.Stat(pre)
		if err != nil || !fi.IsDir() {
			continue
		}
		any = true
		fmt.Fprintf(os.Stderr, "migrate-abort: restoring flat layout from %s\n", filepath.Base(pre))

		// Remove symlinks that M1 created.
		for _, lnk := range []string{
			filepath.Join(ediktRoot, "current"),
			filepath.Join(ediktRoot, "hooks"),
			filepath.Join(ediktRoot, "templates"),
		} {
			if fi, err := os.Lstat(lnk); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
				os.Remove(lnk)
			}
		}

		// Remove the versions/<version> dir if we placed it.
		if version != "" {
			vdir := filepath.Join(ediktRoot, "versions", version)
			if _, err := os.Stat(vdir); err == nil {
				_ = safeRemoveOrQuarantine(vdir, "migrate-abort")
				if dirIsEmpty(filepath.Join(ediktRoot, "versions")) {
					os.Remove(filepath.Join(ediktRoot, "versions"))
				}
			}
			// Remove lock.yaml if it was written by this migration.
			lf, _ := readLock(ediktRoot)
			if lf.Active == version {
				os.Remove(filepath.Join(ediktRoot, "lock.yaml"))
			}
		}

		// Restore from predir.
		restoreFromPredir(ediktRoot, pre)
	}

	for _, stg := range stagings {
		if fi, err := os.Stat(stg); err == nil && fi.IsDir() {
			any = true
			fmt.Fprintf(os.Stderr, "migrate-abort: removing staging %s\n", filepath.Base(stg))
			os.RemoveAll(stg)
		}
	}

	if any {
		emitEvent(ediktRoot, "migration_aborted", map[string]interface{}{
			"reason": "abort", "root": ediktRoot,
		})
		fmt.Fprintln(os.Stderr, "migrate-abort: complete")
	}
	return nil
}

func restoreFromPredir(ediktRoot, pre string) {
	for _, entry := range migrateEntries {
		src := filepath.Join(pre, entry)
		dst := filepath.Join(ediktRoot, entry)
		srcFi, err := os.Lstat(src)
		if err != nil {
			continue // not present in predir
		}
		_ = srcFi

		// Remove existing dst if present.
		dstFi, dstErr := os.Lstat(dst)
		if dstErr == nil {
			if dstFi.Mode()&fs.ModeSymlink != 0 {
				os.Remove(dst)
			} else if dstFi.IsDir() {
				if dirIsEmpty(dst) {
					os.Remove(dst)
				} else {
					conflict := fmt.Sprintf("%s.conflict-%s-%d", dst, tsNow(), os.Getpid())
					if err := os.Rename(dst, conflict); err != nil {
						fmt.Fprintf(os.Stderr, "warn: migrate_restore_from_predir: could not quarantine %s; skipping entry %s\n", dst, entry)
						continue
					}
					fmt.Fprintf(os.Stderr, "warn: migrate_restore_from_predir: refusing to rm-rf non-empty %s — renamed to %s for human review\n", dst, conflict)
				}
			} else {
				os.Remove(dst)
			}
		}

		os.Rename(src, dst)
	}
	os.Remove(pre) // rmdir (only succeeds if empty)
}

// ─── Detection helpers ────────────────────────────────────────────────────────

// needsMigration returns true when $EDIKT_ROOT/hooks is a real directory (not
// symlink) AND $EDIKT_ROOT/versions does not exist. That signals a pre-v0.5.0
// flat layout.
func needsMigration(ediktRoot string) bool {
	hooksPath := filepath.Join(ediktRoot, "hooks")
	fi, err := os.Lstat(hooksPath)
	if err != nil || fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		return false
	}
	versionFile := filepath.Join(ediktRoot, "VERSION")
	if _, err := os.Stat(versionFile); err != nil {
		return false
	}
	versionsDir := filepath.Join(ediktRoot, "versions")
	if _, err := os.Stat(versionsDir); err == nil {
		return false
	}
	return true
}

// migrationInProgress returns true when any staging or pre-migration directory exists.
func migrationInProgress(ediktRoot string) bool {
	for _, pattern := range []string{
		filepath.Join(ediktRoot, ".migrate-staging-*"),
		filepath.Join(ediktRoot, ".pre-migration-*"),
	} {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if _, err := os.Lstat(m); err == nil {
				return true
			}
		}
	}
	return false
}

// hasSecondarySignal returns true when any secondary migration signal is present.
func hasSecondarySignal(ediktRoot, claudeRoot string) bool {
	// M2 signal: HTML sentinels in CLAUDE.md.
	claudemd := filepath.Join(ediktRoot, "CLAUDE.md")
	if fi, err := os.Lstat(claudemd); err == nil && fi.Mode()&fs.ModeSymlink == 0 {
		if data, err := os.ReadFile(claudemd); err == nil {
			if containsStr(string(data), "<!-- edikt:start -->") &&
				containsStr(string(data), "<!-- edikt:end -->") {
				return true
			}
		}
	}

	// M3 signal: flat .md files in $CLAUDE_ROOT/commands/edikt/ that match payload at depth >= 2.
	flatDir := filepath.Join(claudeRoot, "commands", "edikt")
	payloadDir := filepath.Join(ediktRoot, "current", "commands", "edikt")
	if _, err := os.Stat(flatDir); err == nil {
		if _, err := os.Stat(payloadDir); err == nil {
			if des, err := os.ReadDir(flatDir); err == nil {
				for _, de := range des {
					if !de.Type().IsRegular() {
						continue
					}
					fp := filepath.Join(flatDir, de.Name())
					if fi, err := os.Lstat(fp); err == nil && fi.Mode()&fs.ModeSymlink == 0 {
						if _, found := findAtMinDepth(payloadDir, de.Name(), 2); found {
							return true
						}
					}
				}
			}
		}
	}

	// M5 signal: missing keys in config.yaml.
	configPath := filepath.Join(ediktRoot, "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		content := string(data)
		if !hasTopLevelKey(content, "paths") || !hasTopLevelKey(content, "stack") || !hasTopLevelKey(content, "gates") {
			return true
		}
	}

	// M4 signal: governance.md missing schema v2 and no .m4-pending marker.
	gov := filepath.Join(claudeRoot, "rules", "governance.md")
	if data, err := os.ReadFile(gov); err == nil {
		if !containsStr(string(data), "compile_schema_version: 2") {
			if _, err := os.Stat(filepath.Join(ediktRoot, ".m4-pending")); err != nil {
				return true
			}
		}
	}

	return false
}

// ─── Locking ──────────────────────────────────────────────────────────────────

// acquireLock acquires a file-based lock at $EDIKT_ROOT/.lock.
// Returns the lock file (informational), an unlock function, and an error.
// Falls back to mkdir-based lock when flock is unavailable.
func acquireLock(ediktRoot string) (*os.File, func(), error) {
	if err := os.MkdirAll(ediktRoot, 0o755); err != nil {
		return nil, func() {}, fmt.Errorf("cannot create EDIKT_ROOT: %w", err)
	}
	lockFile := filepath.Join(ediktRoot, ".lock")

	// Try flock via a lock file.
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, func() {}, fmt.Errorf("opening lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, func() {}, fmt.Errorf("another edikt process is running (lock: %s). Retry or remove stale lock (exit code %d)", lockFile, 4)
		}
		// flock unavailable — fall back to mkdir lock.
		f.Close()
		return acquireMkdirLock(ediktRoot)
	}

	_, _ = fmt.Fprintf(f, "%d %s\n", os.Getpid(), isoNow())
	unlock := func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
	return f, unlock, nil
}

func acquireMkdirLock(ediktRoot string) (*os.File, func(), error) {
	lockDir := filepath.Join(ediktRoot, ".lock.d")
	if err := os.Mkdir(lockDir, 0o755); err != nil {
		// Check if stale (owner pid dead).
		ownerFile := filepath.Join(lockDir, "owner")
		if data, err2 := os.ReadFile(ownerFile); err2 == nil {
			var ownerPID int
			fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &ownerPID)
			if ownerPID > 0 && !pidAlive(ownerPID) {
				fmt.Fprintf(os.Stderr, "warn: reclaiming stale lock dir %s\n", lockDir)
				os.RemoveAll(lockDir)
				if err3 := os.Mkdir(lockDir, 0o755); err3 != nil {
					return nil, func() {}, fmt.Errorf("another edikt process is running (lock dir: %s). Retry or remove stale lock", lockDir)
				}
			} else {
				return nil, func() {}, fmt.Errorf("another edikt process is running (lock dir: %s). Retry or remove stale lock", lockDir)
			}
		} else {
			return nil, func() {}, fmt.Errorf("another edikt process is running (lock dir: %s). Retry or remove stale lock", lockDir)
		}
	}
	ownerFile := filepath.Join(lockDir, "owner")
	_ = os.WriteFile(ownerFile, []byte(fmt.Sprintf("%d %s\n", os.Getpid(), isoNow())), 0o644)
	unlock := func() {
		os.RemoveAll(lockDir)
	}
	return nil, unlock, nil
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

// ─── Event emission ───────────────────────────────────────────────────────────

func emitEvent(ediktRoot, eventType string, extra map[string]interface{}) {
	if err := os.MkdirAll(ediktRoot, 0o755); err != nil {
		return
	}
	evlog := filepath.Join(ediktRoot, "events.jsonl")

	// Rotation: if > 10 MiB, rename to .1
	if fi, err := os.Stat(evlog); err == nil && fi.Size() > eventsMaxBytes {
		os.Rename(evlog, evlog+".1")
	}

	payload := map[string]interface{}{
		"event":     eventType,
		"timestamp": isoNow(),
	}
	for k, v := range extra {
		payload[k] = v
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return
	}
	line = append(line, '\n')

	f, err := os.OpenFile(evlog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line)
}

// ─── Filesystem helpers ───────────────────────────────────────────────────────

func isoNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func tsNow() string {
	return time.Now().UTC().Format("20060102T150405Z")
}

func readVersion(ediktRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(ediktRoot, "VERSION"))
	if err != nil {
		return "", fmt.Errorf("cannot read version from %s/VERSION: %w", ediktRoot, err)
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("VERSION file is empty at %s/VERSION", ediktRoot)
	}
	return v, nil
}

// createTarGzFromRoot creates a tar.gz of the given entries relative to rootDir.
func createTarGzFromRoot(rootDir, dest string, entries []string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, e := range entries {
		src := filepath.Join(rootDir, e)
		if err := addToTar(tw, src, e); err != nil {
			return err
		}
	}
	return nil
}

func addToTar(tw *tar.Writer, src, name string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return nil // skip missing
	}

	if fi.Mode()&fs.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name:     name,
			Typeflag: tar.TypeSymlink,
			Linkname: link,
			ModTime:  fi.ModTime(),
		}
		return tw.WriteHeader(hdr)
	}

	if fi.IsDir() {
		hdr := &tar.Header{
			Name:     name + "/",
			Typeflag: tar.TypeDir,
			Mode:     int64(fi.Mode().Perm()),
			ModTime:  fi.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, de := range entries {
			childSrc := filepath.Join(src, de.Name())
			childName := name + "/" + de.Name()
			if err := addToTar(tw, childSrc, childName); err != nil {
				return err
			}
		}
		return nil
	}

	// Regular file.
	hdr := &tar.Header{
		Name:    name,
		Size:    fi.Size(),
		Mode:    int64(fi.Mode().Perm()),
		ModTime: fi.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	ff, err := os.Open(src)
	if err != nil {
		return err
	}
	defer ff.Close()
	_, err = io.Copy(tw, ff)
	return err
}

func verifyTarGzReadable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		_, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// atomicSymlink creates or replaces a symlink at link pointing to target.
// Uses a temp symlink + rename for atomicity where possible.
func atomicSymlink(target, link string) error {
	newLink := link + fmt.Sprintf(".new.%d", os.Getpid())
	os.Remove(newLink)
	if err := os.Symlink(target, newLink); err != nil {
		return err
	}
	if err := os.Rename(newLink, link); err != nil {
		os.Remove(newLink)
		// Fall back to ln -sfn equivalent.
		os.Remove(link)
		return os.Symlink(target, link)
	}
	return nil
}

// ensureExternalSymlinks creates:
//   $EDIKT_ROOT/hooks       → current/hooks (or current/templates/hooks)
//   $EDIKT_ROOT/templates   → current/templates
//   $CLAUDE_ROOT/commands/edikt → $EDIKT_ROOT/current/commands[/edikt]
func ensureExternalSymlinks(ediktRoot, claudeRoot string) error {
	// hooks target resolution.
	hooksTarget, err := resolveHooksTarget(ediktRoot)
	if err != nil {
		return err
	}
	if err := atomicSymlink(hooksTarget, filepath.Join(ediktRoot, "hooks")); err != nil {
		return fmt.Errorf("creating hooks symlink: %w", err)
	}

	// templates.
	if err := atomicSymlink("current/templates", filepath.Join(ediktRoot, "templates")); err != nil {
		return fmt.Errorf("creating templates symlink: %w", err)
	}

	// commands.
	cmdsTarget, err := resolveCommandsTarget(ediktRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(claudeRoot, "commands"), 0o755); err != nil {
		return err
	}
	if err := atomicSymlink(cmdsTarget, filepath.Join(claudeRoot, "commands", "edikt")); err != nil {
		return fmt.Errorf("creating commands/edikt symlink: %w", err)
	}
	return nil
}

func resolveHooksTarget(ediktRoot string) (string, error) {
	currentDir := filepath.Join(ediktRoot, "current")
	if _, err := os.Stat(currentDir); err != nil {
		return "", fmt.Errorf("resolve_hooks_target: %s/current does not exist", ediktRoot)
	}
	if _, err := os.Stat(filepath.Join(currentDir, "hooks")); err == nil {
		return "current/hooks", nil
	}
	if _, err := os.Stat(filepath.Join(currentDir, "templates", "hooks")); err == nil {
		return "current/templates/hooks", nil
	}
	return "", fmt.Errorf("resolve_hooks_target: %s/current has neither hooks/ nor templates/hooks/ — payload is malformed", ediktRoot)
}

func resolveCommandsTarget(ediktRoot string) (string, error) {
	currentDir := filepath.Join(ediktRoot, "current")
	if _, err := os.Stat(currentDir); err != nil {
		return "", fmt.Errorf("resolve_commands_target: %s/current does not exist", ediktRoot)
	}
	if _, err := os.Stat(filepath.Join(currentDir, "commands", "edikt")); err == nil {
		return filepath.Join(ediktRoot, "current", "commands", "edikt"), nil
	}
	if _, err := os.Stat(filepath.Join(currentDir, "commands")); err == nil {
		return filepath.Join(ediktRoot, "current", "commands"), nil
	}
	return "", fmt.Errorf("resolve_commands_target: %s/current has neither commands/ nor commands/edikt/", ediktRoot)
}

// writeManifestJSON writes a manifest.json file for the staged version dir.
func writeManifestJSON(versionDir, version string, files []string) error {
	type Manifest struct {
		Version      string   `json:"version"`
		InstalledVia string   `json:"installed_via"`
		Files        []string `json:"files"`
		CreatedAt    string   `json:"created_at"`
	}
	m := Manifest{
		Version:      version,
		InstalledVia: "migration",
		Files:        files,
		CreatedAt:    isoNow(),
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(versionDir, "manifest.json"), data, 0o644)
}

// copyDirFull recursively copies a directory (preserving symlinks).
func copyDirFull(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		// Handle symlinks separately.
		fi, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			lnk, err := os.Readlink(path)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(lnk, target)
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFilePath(path, target, info.Mode())
	})
}

func copyFilePath(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// openNoFollow opens a file with O_NOFOLLOW semantics and reads its content.
// Falls back to Lstat symlink check on platforms without O_NOFOLLOW.
func openNoFollow(path string) ([]byte, error) {
	// Use Lstat to check symlink status first (portable fallback).
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink", path)
	}
	return os.ReadFile(path)
}

// atomicWriteNoFollow writes data to path atomically (tmp + rename), with
// a pre-rename check that path has not become a symlink.
func atomicWriteNoFollow(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d", filepath.Base(path), os.Getpid()))

	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	// Sync.
	f, err := os.Open(tmp)
	if err == nil {
		_ = f.Sync()
		f.Close()
	}

	// Pre-rename check: ensure path is still not a symlink.
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		os.Remove(tmp)
		return fmt.Errorf("%s became a symlink during migration — aborting", path)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func safeRemoveOrQuarantine(tgt, reason string) error {
	fi, err := os.Lstat(tgt)
	if err != nil {
		return nil // already gone
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return os.Remove(tgt)
	}
	if fi.IsDir() {
		if dirIsEmpty(tgt) {
			if err := os.Remove(tgt); err != nil {
				return os.RemoveAll(tgt)
			}
			return nil
		}
		quar := fmt.Sprintf("%s.aborted-%s-%d", tgt, tsNow(), os.Getpid())
		if err := os.Rename(tgt, quar); err == nil {
			fmt.Fprintf(os.Stderr, "warn: %s: refusing to rm-rf non-empty %s — renamed to %s for human review\n", reason, tgt, quar)
			return nil
		}
		fmt.Fprintf(os.Stderr, "error: %s: could not quarantine %s (mv failed); leaving in place\n", reason, tgt)
		return fmt.Errorf("could not quarantine %s", tgt)
	}
	return os.Remove(tgt)
}

func dirIsEmpty(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	return err == io.EOF && len(names) == 0
}

func isSymlink(fi os.FileInfo) bool {
	return fi.Mode()&fs.ModeSymlink != 0
}

func ensureSecondaryBackupDir(ediktRoot string, backupDir *string) error {
	if *backupDir != "" {
		if _, err := os.Stat(*backupDir); err == nil {
			return nil
		}
	}
	ts := tsNow()
	*backupDir = filepath.Join(ediktRoot, "backups", fmt.Sprintf("migration-%s-%d", ts, os.Getpid()))
	return os.MkdirAll(*backupDir, 0o750)
}

// findAtMinDepth finds the first file named name under root at depth >= minDepth.
func findAtMinDepth(root, name string, minDepth int) (string, bool) {
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if d.IsDir() {
			return nil
		}
		if depth >= minDepth && filepath.Base(path) == name {
			found = path
		}
		return nil
	})
	return found, found != ""
}

func findLatestBackupDir(ediktRoot string) string {
	pattern := filepath.Join(ediktRoot, "backups", "migration-*")
	matches, _ := filepath.Glob(pattern)
	var latest string
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil || !fi.IsDir() {
			continue
		}
		if verifyBackupTarball(m) {
			latest = m
		}
	}
	return latest
}

func verifyBackupTarball(backupDir string) bool {
	tarPath := filepath.Join(backupDir, "pre-migration.tar.gz")
	if _, err := os.Stat(tarPath); err != nil {
		return false
	}
	if err := verifyTarGzReadable(tarPath); err != nil {
		return false
	}
	sidecar := tarPath + ".sha256"
	if _, err := os.Stat(sidecar); err != nil {
		return true // sidecar optional
	}
	observed, err := sha256File(tarPath)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(sidecar)
	if err != nil {
		return false
	}
	expected := strings.TrimSpace(strings.Fields(string(data))[0])
	return observed == expected
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return false
	}
	return strings.Contains(string(haystack), string(needle))
}

func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}

func replaceAll(data, old, new []byte) []byte {
	return []byte(strings.ReplaceAll(string(data), string(old), string(new)))
}

func hasTopLevelKey(yamlContent, key string) bool {
	for _, line := range strings.Split(yamlContent, "\n") {
		if strings.HasPrefix(line, key+":") {
			return true
		}
	}
	return false
}

// printM1Plan prints the migration plan (what will move, what will be preserved).
func printM1Plan(ediktRoot, claudeRoot, version string) {
	fmt.Printf("Migration needed: flat layout → versioned layout (target: %s)\n\n", version)
	fmt.Println("Will move:")
	for _, e := range migrateEntries {
		p := filepath.Join(ediktRoot, e)
		if _, err := os.Lstat(p); err == nil {
			fmt.Printf("  %s  →  %s/versions/%s/%s\n", p, ediktRoot, version, e)
		}
	}
	fmt.Println("\nWill create symlinks:")
	fmt.Printf("  %s/current      → versions/%s\n", ediktRoot, version)
	fmt.Printf("  %s/hooks        → current/hooks\n", ediktRoot)
	fmt.Printf("  %s/templates    → current/templates\n", ediktRoot)
	fmt.Printf("  %s/commands/edikt → %s/current/commands\n", claudeRoot, ediktRoot)
	fmt.Println("\nWill preserve:")
	for _, e := range preservedEntries {
		p := filepath.Join(ediktRoot, e)
		if _, err := os.Stat(p); err == nil {
			fmt.Printf("  %s (untouched)\n", p)
		}
	}
	fmt.Println("\nBackup:")
	fmt.Printf("  %s/backups/migration-<ts>/pre-migration.tar.gz\n", ediktRoot)
	fmt.Printf("  %s/backups/migration-<ts>/pre-migration.tar.gz.sha256\n", ediktRoot)
	fmt.Println("  (use 'edikt migrate --abort' to restore from the most recent backup)")
	fmt.Println()
}

func confirmInteractive() error {
	// Open /dev/tty for interactive prompt.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("non-interactive session detected. Re-run with --yes to confirm migration")
	}
	defer tty.Close()

	fmt.Fprint(tty, "Proceed? [y/N]: ")
	var reply string
	buf := make([]byte, 256)
	n, err := tty.Read(buf)
	if err != nil {
		return fmt.Errorf("cannot read from /dev/tty. Re-run with --yes")
	}
	reply = strings.TrimSpace(string(buf[:n]))
	switch strings.ToLower(reply) {
	case "y", "yes":
		return nil
	default:
		fmt.Fprintln(os.Stderr, "aborted")
		// Return a sentinel error that causes a clean exit 0.
		return errAborted
	}
}

var errAborted = fmt.Errorf("aborted")
