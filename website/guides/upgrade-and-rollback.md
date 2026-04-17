# Upgrade and rollback

edikt's payload (templates, commands, hooks, agents) is versioned independently of the launcher binary. You can upgrade, roll back, pin a version, or run multiple versions side by side.

## Upgrade

Fetch the latest payload and activate it:

```bash
edikt upgrade
```

If you're on Homebrew, `brew upgrade edikt` updates the launcher. `edikt upgrade` updates the payload. They're independent.

## Rollback

Revert to the previous payload version:

```bash
edikt rollback
```

**Rollback is payload-only.** `edikt rollback` flips `~/.edikt/current` back to the previous generation. It does not undo migrations (M1-M5). Migrations are permanent structural changes to your `~/.edikt/` layout and `~/.claude/` command files. If a migration caused a problem, contact support — don't expect rollback to fix it.

### v0.5.0 host-file rollback

v0.5.0 added a dedicated rollback for host-level changes (the new `settings.json` permissions block, the managed-region sidecar, and grandfather verdict stubs):

```bash
edikt rollback v0.5.0
```

This:

- Restores `~/.claude/settings.json` from the pre-upgrade backup at `~/.edikt/backup/pre-v0.5.0-<timestamp>/`.
- Removes the managed-region sidecar at `~/.edikt/state/settings-managed.json` so the next install prompts fresh.
- Removes any grandfather verdict stubs created by the upgrade migration (JSON files under `docs/product/plans/verdicts/` with `meta.grandfathered: true`).

It's idempotent — re-running is safe. The backup is preserved after rollback; delete it manually when you're satisfied:

```bash
rm -rf ~/.edikt/backup/pre-v0.5.0-*
rm -f ~/.edikt/backup/pre-v0.5.0-marker
```

Unlike payload rollback, this form takes a version argument so future major releases can add similar host-level rollback paths (`edikt rollback v0.6.0`, etc.).

## Pinning a version

Stay on a specific version:

```bash
edikt use v0.5.0        # activate v0.5.0 immediately
edikt upgrade --pin v0.5.0  # fetch v0.5.0 and pin it
```

When pinned, `edikt upgrade` is a no-op until you clear the pin:

```bash
edikt upgrade --pin clear   # remove pin, next upgrade proceeds
```

## Listing installed versions

```bash
edikt list
```

Output:

```
  v0.4.3
* v0.5.0   (current)
```

## Pruning old versions

Keep only the N most recent versions (default: 3):

```bash
edikt prune          # remove all but 3 most recent + current
edikt prune --keep 5 # keep 5
edikt prune --dry-run  # preview what would be removed
```

`edikt prune` never removes the current version or any pinned version.

## What `edikt upgrade` does

1. Fetches the latest release tarball from GitHub
2. Verifies the SHA256 checksum against `SHA256SUMS`
3. Extracts to `~/.edikt/versions/<tag>/`
4. Runs any pending migrations (M1-M5) against your current install
5. Flips `~/.edikt/current` to the new version
6. Updates `~/.edikt/lock.yaml`

If verification fails, the new version is not activated. Your current version is untouched.

## Project-mode installs

If edikt is installed per-project (`.edikt/` inside the repo), `edikt upgrade` run from the project directory upgrades that project's payload independently of the global install.

```bash
cd my-project
edikt upgrade    # upgrades the project-local payload only
```
