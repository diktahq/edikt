# INV-005 — Managed-region integrity is verified before overwrite

**Status:** Active

## Statement

Every edikt-managed region in a user file has a cryptographic integrity mechanism that MUST be verified before the region is overwritten. Two variants exist:

**(a) Markdown-hosted regions** (CLAUDE.md, ADRs, invariants, plans, rule files, guidelines) are delimited by `[edikt:NAME:start]: #` / `[edikt:NAME:end]: #` sentinel lines with an inline `[edikt:NAME:sha256]: # <64-hex>` hash anchor line placed inside the region. Edits are validated by **byte-range overlap** of the resolved file — never by regex over `old_string` or `new_string`.

**(b) JSON-hosted regions** (settings.json) cannot embed sentinels (JSON has no comment syntax). Their integrity is recorded **out-of-band** in a sidecar at `~/.edikt/state/settings-managed.json` with the shape `{settings_path, managed_keys, managed_hash, sentinel_version, installed_at}`. The install/upgrade writer verifies the live JSON's managed-key hash against the sidecar before overwriting managed keys.

In both variants, an edit whose resolved byte range (markdown) or managed-key hash (JSON) would mutate the region is blocked unless issued by an explicitly allowlisted edikt operation (compile, install, upgrade, migration). The specific signalling mechanism is an implementation detail — see Implementation below.

## Rationale

The v0.5.0 security audit (2026-04-17) found two failure modes. First, `pre-tool-use.sh` checked for sentinel literals only in `old_string`/`new_string`; an Edit targeting a non-sentinel line inside the region was approved even when it modified managed content (audit finding HI-4). Second, sentinel regions had no content hash — a malicious PR could prepend a fake sentinel region to a file and the next compile would trust attacker-planted content (audit finding LOW-3).

A byte-range check on the resolved file closes HI-4; a hash anchor inside the region closes LOW-3. The split between markdown and JSON variants exists because JSON files cannot host markdown link-reference syntax.

## Consequences of violation

- A crafted Edit that modifies compiled directives without matching the sentinel literal silently changes governance — INV-002 ("ADRs immutable") becomes unenforceable.
- A fake sentinel region injected by a malicious PR poisons the next compile, which trusts attacker content as authoritative.
- A JSON settings.json without integrity tracking allows silent permission-block tampering — a PR that edits `permissions.allow` is indistinguishable from a legitimate edikt update.

## Implementation

**Markdown (pre-tool-use guard):** on every Edit/Write of a file under a governance path, the guard:
1. Reads the resolved on-disk content of `file_path`.
2. Scans for every `[edikt:NAME:start]: #` / `[edikt:NAME:end]: #` pair.
3. For an Edit, locates `old_string` in the content and computes the byte range the patch would replace.
4. If that range overlaps any sentinel-bounded region, checks the inline `[edikt:NAME:sha256]: # <hex>` anchor.
5. **Bootstrap rule:** if the anchor is absent, treats the region as unarmed — blocks the edit by byte range (same as armed), but does NOT attempt hash verification. Compile seeds the hash on first run.
6. If the anchor is present and matches, blocks the edit unless one of the allowlisted bypass signals is set. Current bypass signals are the environment variables `EDIKT_COMPILE_IN_PROGRESS=1` (set by `/edikt:gov:compile`) and `EDIKT_MIGRATION_IN_PROGRESS=1` (set by `bin/edikt upgrade` during the upgrade transaction). This list is an implementation detail; additions require an ADR or a revision of this invariant.
7. If the anchor is present and mismatches, blocks with an instruction to run `/edikt:gov:compile`.

**JSON (install/upgrade writer):** on every write to `settings.json`:
1. Reads `~/.edikt/state/settings-managed.json` if present.
2. Computes the current hash of the live `settings.json`'s managed keys (canonical JSON, sorted keys, UTF-8).
3. If the sidecar is absent → bootstrap; seed on this write.
4. If the live hash matches the sidecar hash → managed; safe to overwrite managed keys only.
5. If the live hash differs → user has customized; prompt before overwriting; offer `(y)es replace / (n)o skip / (d)iff`.
6. After any write, recompute and store the new hash in the sidecar.

## Anti-patterns

Forbidden (regex-only guard — audit HI-4):
```bash
case "$old_string" in *'[edikt:directives:start]: #'*) decision=block ;; esac
```

Required (byte-range guard):
```python
# pseudocode inside the pre-tool-use python block
content = Path(file_path).read_text()
regions = find_sentinel_regions(content)  # returns list of (start_byte, end_byte, name)
edit_range = locate_edit_range(content, old_string, new_string)
for r in regions:
    if ranges_overlap(edit_range, (r.start_byte, r.end_byte)):
        if not bypass_env_set():
            emit_block(f"edit overlaps managed region {r.name}")
```

## Known limitations

`O_NOFOLLOW` on macOS only refuses when the final path component is itself a symlink — not when any ancestor directory in the path is. For the current threat model (attacker swaps the file for a symlink during the read-write window), O_NOFOLLOW on open() is sufficient and the post-read re-check before os.replace closes the race on the destination. A fully ancestor-safe path would require opening each directory with `O_DIRECTORY | O_NOFOLLOW` and using `openat`-style navigation, which is out of scope for the bash + python3 hook runtime. Tracked as a v0.6.x hardening item.

## Enforcement

- Pre-tool-use hook executes the byte-range guard on every Edit/Write under governance-watched paths.
- Every managed sentinel region carries a hash anchor after the first compile following v0.5.0 upgrade. Missing anchor on an active block → compile seeds it.
- Unit tests at `test/security/sentinel/` cover the overlap matrix (adjacent, inside, outside, cross-boundary, CRLF, Unicode) plus the hash mismatch and bootstrap paths.
- `~/.edikt/state/settings-managed.json` is required for any edikt-managed `settings.json`; its absence triggers the interactive permissions prompt on upgrade.

## Directives

[edikt:directives:start]: #
source_hash: 8c8148740609239cfb63802048453ffc4b60cf746894cc40194bef0a4729be06
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "templates/hooks/pre-tool-use.sh"
  - "bin/edikt"
  - "install.sh"
  - "commands/gov/compile.md"
scope:
  - implementation
  - review
directives:
  - Managed markdown regions MUST be guarded by byte-range overlap checks on the resolved file, NEVER by regex over `old_string` or `new_string`. (ref: INV-005)
  - Managed markdown regions MUST carry an inline `[edikt:NAME:sha256]: # <hex>` anchor line inside the region. Compile MUST write this anchor line on the first run that encounters a region without one (bootstrap rule). (ref: INV-005)
  - Managed JSON regions (settings.json) MUST have an out-of-band integrity record at `~/.edikt/state/settings-managed.json` recording the managed-key hash. (ref: INV-005)
  - Edit that would overlap a managed region MUST be blocked unless an allowlisted bypass signal is set (currently `EDIKT_COMPILE_IN_PROGRESS=1` for compile, `EDIKT_MIGRATION_IN_PROGRESS=1` for upgrade/migration). (ref: INV-005)
manual_directives: []
suppressed_directives: []
canonical_phrases:
  - "managed region"
  - "sentinel hash anchor"
  - "byte-range guard"
  - "INV-005"
behavioral_signal:
  cite:
    - "INV-005"
[edikt:directives:end]: #
