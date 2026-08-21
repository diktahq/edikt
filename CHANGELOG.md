# edikt changelog

## v0.7.2 (2026-08-19)

A defect sweep sourced from the 2026-08-19 strategic review (§5, D1–D14) and
the BRAIN-006..009 verification passes (N1–N6), plus a handful of post-flight
naming/schema bugs and a brainstorm allocator collision found while verifying
those brainstorms. Bug fixes only — D3 (the one architectural item in the
review) is deferred; see below. A second sweep, sourced from a real
downstream upgrade field report, follows in its own **Fixed** section below.

### Fixed

- **Path resolution.** `docs:intake`, `doctor`, `session`, and `code-review`
  hardcoded `{base}/decisions`, `{base}/invariants`, `docs/plans`, and similar
  paths instead of honoring the configured `paths.*` keys, so imported ADRs
  and invariants could be invisible to compile, doctor under-counted
  decisions/invariants/plans against a project's real layout, and the drift
  command's spec pick was alphabetical rather than recency-ordered
  (`ls | head -1` vs `ls -t | head -1`). All four commands now resolve
  `paths.decisions`, `paths.invariants`, `paths.plans`, and `paths.specs`
  explicitly. `doctor` also drops its stale `/edikt:intake` suggestion in
  favor of the real `/edikt:docs:intake` name, and `intake.md`'s section
  numbering no longer skips a section.
- **Init fixes.** `/edikt:init` never ran `/edikt:gov:compile` after
  generating governance, so a fresh init ended in the exact "compile pending"
  state `doctor` immediately flags — init's §4b now runs a conditional
  compile whenever sidecars exist. Init's config template omitted the
  `brainstorms` path key that `config.md` documents, so it's now written by
  default. The five generated README stubs (and the brownfield guide's
  manual-capture walkthrough) taught dead command names
  (`/edikt:adr`, `/edikt:invariant`, `/edikt:plan`, …) instead of the real
  `:new`/`sdlc:` forms — both now print commands that actually run. The
  guideline sidecar-extraction glob matched `README.md`/`index.md` alongside
  real guidelines, dispatching the extractor at non-guideline files; it now
  skips them.
- **Dead `/edikt:team` route removed.** `templates/CLAUDE.md.tmpl` and the
  live `CLAUDE.md` both routed "team onboard" to `/edikt:team`, a command
  file that has never existed — not deprecated, just dead. The trigger row
  now points at `/edikt:init` directly, and the three tests that pinned the
  old row (`test-v031-team-consolidation.sh` and two upgrade
  3-way-merge/safe-replace pytest cases) were updated to the new literal.
- **Brainstorm ID allocation.** `/edikt:brainstorm` allocated the next
  `BRAIN-NNN` id by counting existing files rather than taking the max
  in-use number, so a gap or an out-of-order deletion could mint a
  colliding id. Allocation now goes through a new `bin/edikt next-id brain`
  (max-based, aware of both file-form `BRAIN-NNN-slug.md` and directory-form
  `BRAIN-NNN-slug/` entries), with a max-based prose fallback when the
  binary is unavailable.
- **Spec AC-form consistency.** `spec.md`'s prose acceptance-criteria
  template showed `AC-001` while its own YAML sidecar example used
  `AC-001-1`, an internal contradiction. The prose template now matches the
  sidecar: pass-through PRD criteria keep `AC-{FR}-{seq}` verbatim, and
  criteria the spec itself adds use `SAC-NNN`.
- **Post-flight L1 contract.** `post-flight.md` expected lowercase
  `plan-<slug>` L1 verdict filenames while `bin/edikt verify` actually
  writes `PLAN-` case — a mismatch that only happened to work on
  case-insensitive filesystems and would silently fail to find L1 verdicts
  on Linux CI. It now sources the single naming definition in
  `templates/hooks/_plan-naming.sh` and tries both known forms. The L1
  schema reference was pointing at the wrong artifact; it now validates
  against `templates/schemas/verify-report.v1.schema.json`, a new schema
  artifact that pins the actual shape `bin/edikt verify` writes. The
  HEAD~1..HEAD diff fallback used to degrade silently; it now surfaces a
  `diff_fallback` field and a visible warning in the command's output.
- **Docs truth fixes.** The v0.6.0 CHANGELOG entry claimed an "init style
  detection" feature that was never actually implemented in `init.md`; the
  claim is removed with a dated correction note. `website/guides/brownfield.md`
  advertised audit output edikt doesn't produce and used the same dead
  command names as the README stubs above; both are corrected.
  `docs/internal/assumptions.md` was titled "Harness Assumptions" but
  contains only model assumptions — retitled with a note distinguishing it
  from `docs/architecture/assumptions.md`. The Claude Code parity doc's
  `SubagentStart` row was marked unshipped even though it's registered in
  `settings.json.tmpl`; corrected to shipped.

### Fixed (second sweep — real downstream `dev`→0.7.1 upgrade field report)

Nine defects found on an actual `dcsg/bok-services` upgrade run (24 ADRs, 15
invariants, 39 sidecars), triaged and fixed against this repo's own source
rather than the report's own diagnosis — three of its twelve findings turned
out to already be handled or to name a command that already shipped; see
`docs/internal/audits/TRIAGE-2026-08-20-bok-services-071.md` for the full
classification.

- **`migrate to-v2` no longer credits an unvalidated pass as "already v2."**
  `WouldConvertToV2`/`ConvertFileToV2` only ever detected the ABSENCE of v1
  markers, never the PRESENCE of what v2 requires — a hand-authored metadata
  card carrying neither reached the "already v2" bucket unvalidated, exactly
  how two malformed sidecars on the field-report corpus passed `migrate to-v2
  --dry-run` as current while `doctor` correctly flagged them SCHEMA INVALID
  on the same run. Reuses `sidecar.Discover`'s own `Load()` result — the same
  call doctor's check makes — so a file that fails to load now lands in a
  new `invalid` bucket instead.
- **`/edikt:upgrade` merges hooks by basename, not by whole-type presence.**
  A project with `PreToolUse`/`PostToolUse` already registered (true for
  nearly every pre-0.7.1 install) got nothing added when the template's
  script list under those types grew — `verify-gate.sh` and both
  `inject-directives-{pre,post}.sh` silently never installed, and `doctor`'s
  own remedy ("Run `/edikt:upgrade` to install it") would not have worked,
  since upgrade never diffed below the type level. The merge is now a
  two-pass, basename-level algorithm: add any template command missing from
  an existing type, then separately retire a hook only when it's edikt-owned,
  no longer shipped anywhere, and its file no longer exists on disk (closing
  a related gap where `pre-compact.sh`, retired since v0.5.0/ADR-014, stayed
  registered into a permanent, non-self-healing `doctor` ERROR).
- **§2c's legacy agent classifier's diff feed, not its v0.4.3 rule, was
  wrong.** Three defects around the pre-v0.6.0 fallback path, the
  classification rule itself unchanged: the prescribed `diff -u` had its
  `+`/`-` backwards relative to the very next line of prose; the classifier
  diffed the raw template against a stack-FILTERED install, producing a
  constant spurious USER DIVERGENCE for every stack-gated agent
  (`backend.md`, `frontend.md`, `qa.md`); and it had no way to tell "this
  installed file matches an older, unedited template version" from "this is
  a real edit" — both looked like the same raw deletions. Fixed the diff
  direction, materialize a stack-filtered template before comparing (the
  same `apply_stack_filter` the provenance-first path's own Step 5 already
  runs), and check retained prior template versions before finalizing
  USER DIVERGENCE.
- **`doctor` resolves `$CLAUDE_PROJECT_DIR` hook commands instead of
  mis-extracting them.** The hook-path checker's command extractor was a
  regex, escape-blind by construction — a hook quoted as
  `"$CLAUDE_PROJECT_DIR"/.claude/hooks/foo.sh` (reproducible live against
  this repo's own `tools/edikt/.claude/settings.json`) degraded to a lone
  captured backslash, reported as a false ERROR on a legitimate, existing
  hook. Replaced the regex with the JSON decode doctor's own basename checks
  already use, which resolves escapes correctly by construction, and added
  `$CLAUDE_PROJECT_DIR`/`${CLAUDE_PROJECT_DIR}` expansion alongside the
  existing `$HOME` handling.
- **`gov reextract --status` degrades instead of hard-failing when the
  sidecar-extractor agent isn't project-local.** The resolver
  (`internal/phasea/agentmodel.go`) was project-local-only with no fallback;
  `Status()` treated that as fatal even though the identical resolution
  failure is already non-fatal on the real dispatch path. It now falls back
  to the active Claude profile's `agents/` directory — the same location
  `doctor`'s own agent-drift check already covers, via a newly-shared
  `internal/claudepaths` resolver — and `Status()` degrades `PromptVersion`
  to `UNKNOWN` rather than failing the whole report. Closes the case where
  `/edikt:upgrade` §7's own "if the status check fails, say nothing further"
  contract turned a perfectly resolvable agent into silent, invisible
  skipping of the re-extraction offer.
- **`edikt install` reports when it did not activate the version it
  staged.** Install and `use` are deliberately separate verbs, but install's
  success path printed nothing about that split — `edikt version` correctly
  kept reporting the prior tag, with no explanation, easy to mistake for the
  install having failed. Install now prints `installed <tag> (not activated
  — run \`edikt use <tag>\` to activate)` unless the installed tag is
  already active.

### Deferred

- **D3 — `runner.go`'s `claude -p` shellout (ADR-030/INV-012 tension).** The
  owner ruling on this sweep was "whatever is better and more reliable for
  userland, no regressions," which resolves to deferring the rewrite rather
  than rushing it into this patch. The exempt-file entry for
  `tools/edikt/internal/phasea/runner.go` is re-anchored: the removal
  deadline moves to v0.8.0, pending the Phase A re-orchestration decision
  tracked in BRAIN-009 Q8 (pure tier-1 Task dispatch vs. a host-dispatch
  abstraction that still shells out). The rewrite gets its own cycle.

## v0.7.1 (2026-08-17)

Patch release fixing real defects found by an end-to-end install smoke test
run against the published v0.7.0 release assets (curl and brew paths).

### Fixed

- **`install.sh` left a stray tarball behind after every successful
  install.** `stage_launcher()` downloaded the launcher release tarball
  directly into `.edikt/bin/` instead of a temp staging directory, and never
  cleaned it up on the success path — reproducible on 2 of 2 clean installs.
  Now stages in a dedicated `mktemp -d` directory and removes it on every
  exit path, success included.
- **A genuine `SIGTERM` or Ctrl-C during install could bypass cleanup.** The
  fix above only ran on the script's own normal control flow; an external
  interrupt mid-download skipped it entirely. A `TERM`/`INT`-scoped signal
  trap now guarantees the staging directory is removed even on a real
  interrupt, without the downsides of a blanket `trap ... EXIT`.
- **A checksum-mismatch install printed a confusing second error.**
  `install_launcher()` was silently reporting success even when the launcher
  install actually failed (a `return $?` was reading an unrelated cleanup
  command's exit code instead of the real one), causing a generic "No such
  file or directory" crash to follow the real, already-printed error.
- **`edikt upgrade`'s "what's new" summary couldn't find `CHANGELOG.md`.**
  The release workflow's payload tarball never packaged it, even though the
  upgrade command already expected to read it from `~/.edikt/CHANGELOG.md`.

### Known limitation (unchanged from v0.7.0, now documented)

- Direct `Edit`/`Write` to `settings.json` is still denied by design — the
  JSON-region managed-hash verifier isn't built yet. `commands/upgrade.md`
  now documents the sanctioned workaround (merge via `Bash`, not
  `Edit`/`Write`). Tracked for a future version:
  `docs/internal/issues/settings-json-managed-region-verifier-unimplemented.md`.

## v0.7.0 (2026-08-14)

> **MIGRATION REQUIRED**
>
> v0.7.0 raises the version-line floor from 0.6 to 0.7 (`versionLineFloor`, gov-sidecar
> schema v2 → v3 compile output). This is a **directional** gate: it can stop a new v0.7
> binary from operating on a corpus older than it expects, but it cannot make an
> already-shipped v0.6 binary refuse a v2/v3 corpus cleanly. Upgrade the `edikt` binary
> itself before running any governance command against a project that has already been
> touched by v0.7.0 tooling. If your sidecars are still on schema v1, run
> `edikt migrate to-v2` before your next `gov compile` — a v1 sidecar left in place will
> cause the compile pipeline to refuse to dispatch at all once any v2 sidecar exists
> alongside it.

Fixes from the first full-SDLC field run of v0.6.0 (RCP v0.1, 2026-08-02), plus a rewrite of
write-time enforcement itself. **Read the next section even if you skip everything else** —
it changes what happens the moment you save a file, not just what a command prints.

### Write-time enforcement now actually enforces — expect real new denials

Three changes land together and compound each other. None of them are new bugs; they are
the deny channel finally doing what it was always supposed to do, plus a corpus that was
undercounting how much of it applies.

- **BREAKING: the deny channel now actually denies (ADR-061).** Previously, `{"continue":
  false}` was a post-hoc turn-kill: the write landed on disk *first*, and only afterward did
  the turn terminate. Measured directly against production transcripts: **14 of 14** denied
  writes had already gone through. This release fixes the shape so a MUST-grade write is
  refused *before* it happens. This is the single most surprising behavior change in this
  release: edits that used to silently succeed (with the turn dying strangely afterward) will
  now be flatly rejected, with a reason. If a workflow depended on "the turn stops, but my
  edit is still there," that workflow now needs to read the deny message and revise instead.
- **Directive grading corrected — the corpus was undercounting what's MUST-grade (ADR-064).**
  Grade derivation previously only recognized `MUST NOT` / `NEVER` / `NO EXCEPTIONS` as
  must-grade signals; a bare positive `MUST` / `SHALL` / `REQUIRED` fell through to
  `advisory`. Measured: **404 of 420 (96%)** directives graded `advisory` actually carried
  obligation strong enough to be `must`. Grade is now derived from full RFC-2119 modal force.
  **Combined effect with the fix above:** your corpus's *content* has not changed, but the
  volume of directives that can now genuinely deny a write roughly doubled at the same moment
  the deny channel started actually firing. Expect noticeably more write-time blocking after
  upgrading than you saw on v0.6.0, on the exact same project — this is the corpus finally
  being enforced as written, not new restrictions being added.
- **New mechanism: the write-time injection tier (ADR-060).** Enforcement no longer depends
  on whether a governing rule happened to already be loaded in context. A `PreToolUse` hook
  now runs `hook match` against the file actually being written, independent of ambient
  recall, and denies or annotates based on what that specific file's own directives say. This
  is what makes the two fixes above meaningful in practice, not just in theory.

### Schema

- **`gov-sidecar` schema v2 → compile output v3 (ADR-059).** The old keyword-routing table
  (629 terms) is replaced by a one-line topic index; `directive-index.yaml`, `manifest.yaml`,
  and per-topic `SKILL.md` packages are new or changed rendered surfaces; `compiled_at` is
  removed from compiled output entirely (for byte-for-byte determinism). Running
  `gov compile` or `doctor` after upgrading will show a differently-shaped output tree — this
  is expected, not an error.

### Security

- **F-042: a real integrity-verification gap in `edikt upgrade` is closed.** The upgrade path
  (distinct from `install.sh`, which was already correct) checked payload integrity against
  an unsigned, never-published `<url>.sha256` sidecar file. When that predictably 404'd, the
  fallback path recommended `EDIKT_INSTALL_INSECURE=1` — which also silently downgraded a
  cosign verification that had genuinely **failed** into a mere warning. Fixed: `SHA256SUMS`
  is cosign-verified first, and the payload digest is read only from that already-verified
  document. If you use `edikt upgrade` rather than re-running `install.sh`, this closes a real
  hole in what that command was supposed to already guarantee.
- **F-071: `Bash(git :*)` removed from the default permission allowlist.** The prior pattern
  let command-chaining (`git status && anything-else`, `git status; anything-else`) execute
  the *entire* chained line unprompted — the allowlist covered the first command and silently
  ran whatever followed it too. Every `git` invocation now prompts once, the same as any other
  command not on the allowlist. **You will see new permission prompts on git commands that
  previously ran silently — this is the fix working as intended, not a regression.**

### Everything else from the v0.6.0 field run

- **python3 is an undeclared runtime dependency of the hook tier (ADR-058) — recorded, NOT fixed here.** `docs/project-context.md` states "no runtime dependencies — installation is copy .md files only" and "shell scripts for hooks, bash, no external dependencies". Measured: **19 of 22 shipped hook templates invoke `python3` at runtime** (not 10 as first reported) — `phase-end-detector.sh` 23 times, `subagent-stop.sh` 14, `stop-hook.sh` 9, `verify-gate.sh` 7 — for JSON building, SHA-256, and timestamp math. It is undeclared and unchecked: `install.sh` uses python3 itself but never verifies it for the hooks it installs, and `doctor` does not probe for it. Invisible on macOS, silently inert on a minimal Linux image — the `stat -f` (D23) portability class. The irony is in the record: INV-003 requires serializer-built JSON, bash has no serializer, so python3 became the de facto one; the invariant is right and must not be weakened, and what changed is that a second implementation now exists. Remedy assessed: SPEC-011's own thin-shim + `edikt hook match` pattern generalises to retire it at near-zero marginal cost, since the Go binary is already a hard dependency of the heaviest python3 users. Recommendation — fold the PATTERN into the stage-2 hook work (which is already building shim+verb, permit rows, and the fail-open contract) and schedule the 19-file migration as its own tracked follow-up; fail-open must not regress into fail-closed when a verb is absent. Awaiting owner ruling.
- **Anchor verification (ADR-056) runs once, at extraction time, and never again — recorded, NOT fixed here.** ADR-056 (above, this changelog) added a third acceptance rung to Phase A: every anchor of every directive and prohibition must quote its parent `.md` byte-exactly at the recorded line range, or the dispatch fails and rolls back. Measured directly from a 2026-08-16 sidecar-mutation run (GL-002 Part III, `logs/sidecar-mutation-harness-20260816.log`): `sidecar.VerifyAnchors` has exactly one production call site in the entire codebase, inside that same Phase A acceptance gate. Nothing calls it again after a sidecar is accepted. `gov compile --check`'s own "anchor drift: N stale" line cannot report anything but zero — the zero is a literal in the format string, not a computed value. For a sidecar with no `paths:` — which is where the hard, ambient-core MUST invariants live — a dropped or corrupted anchor is invisible everywhere afterward: confirmed with a full, non-`--check` `gov compile`, whose rendered output was byte-identical before and after the mutation. This is not a claim that today's anchors are wrong; the corpus was verified once, at write time, and nothing recorded has silently corrupted one since. It means the release's claim to falsifiable, continuously-checked governance is narrower than it reads — "anchors are verified" holds at the moment of writing and is unmeasured forever after. Remedy assessed: wire `sidecar.VerifyAnchors` into `gov compile`'s standing run (every compile, not only Phase A's one-time acceptance gate), prioritizing `paths:`-less sidecars first, since `paths:`-declaring sidecars currently get partial, accidental coverage from an unrelated per-topic-file staleness fingerprint that `paths:`-less ones never touch. Awaiting owner ruling.
- **Topic descriptions are proposed once per topic, not left blank (ADR-057).** SPEC-011 ruling 1 specifies the topic registry is populated *extracted-then-approved*; phase 1 shipped *blank-then-owner-authored* — one pending slot per topic with `description: ""` and nothing proposing anything. Survivable in this repo (the owner authored all ten); a defect for an adopting project, which would install edikt, compile, and receive a dozen empty slots — and since the topic index REPLACED the 629-term routing table, an owner who leaves them empty has no routing tier at all. The constraint that caused it is real: a topic spans up to fifteen artifacts while extraction is per-artifact isolated (ADR-027), so no extractor dispatch ever sees a topic whole. Resolved as a ONE-SHOT PER-TOPIC PROPOSAL DISPATCH at seeding time, reading that topic's compiled directives across artifacts and writing a draft line plus evidence into the pending slot for the human to edit and approve — corpus-level judgment made once and pinned, never re-derived per compile (the paths-inference shape). Explicitly NOT per-artifact extractors each proposing with compile picking a winner: choosing inside compile is per-compile judgment, the v0.4.5 failure mode, and GL-003 forbids chaining the pass into compile's dispatch loop — so it lives in the seeding/upgrade flow as its own entry point. Fails soft: with the pass unavailable, slots open empty and compile proceeds exactly as today, and an empty slot still cannot be approved into a blank registry entry.
- **Anchors are verified, not requested (ADR-056).** Phase A's acceptance chain gains a third rung after parse and schema: every anchor of every directive and prohibition must quote the parent `.md` byte-exactly at the line range it records, or the dispatch FAILS and rolls back — never a warning. Provenance is measured rather than assumed: three successive extractor-prompt revisions moved the anchor error rate 1/203 → 5/200 → 1/184 on the frozen SPEC-011 fixture set and never reached zero, and the most specific instruction did not outperform the vaguest. The failure is invisible on inspection (the quote is real prose; only the recorded range is wrong), so the failure message names the offending anchor, its recorded range, and **what actually sits at those lines**. Stricter than drift detection on purpose — drift tolerates re-wrapped prose, the generation boundary does not, because an extractor reading the exact bytes has no reason to produce a quote that is not a byte slice of them. A sidecar with zero items is accepted without reading the parent (a measured zero); items present with an unreadable parent fails as unmeasured. Same change fixes the schema-dispatch defect it exposed: both validating call sites hardcoded the v1 schema, so the gov-sidecar.v2 corpus migration reported **70 of 82 valid sidecars as schema failures** — `schema_version` now selects the schema in one place, failing closed on an undeclared or unknown version.

- **BREAKING (plan flows): criteria sidecar is now spec-only (ADR-045).** `/edikt:sdlc:plan`'s docs described the v0.4 state-bearing sidecar shape that `bin/edikt verify` strict-rejects with exit 2. The sidecar now matches the binary exactly (shape shipped as `templates/schemas/plan-criteria.v1.schema.json`); evaluation state (`status`, `fail_count`, `fail_reason`, `block_reason`, `last_evaluated`) moves to `.edikt/state/plan-eval/PLAN-<slug>-eval.json` (`plan-eval-state.v1.schema.json`), written by the evaluator flow and `phase-end-detector.sh`, read by `post-compact.sh`. Evaluation history is machine-local (`.edikt/state/` is gitignored); the committed record remains the verdict JSONs (ADR-025). Migrate legacy sidecars once with `/edikt:sdlc:plan --sidecar-only <slug>`.
- **Configurable artifact-ID schemes.** New `ids.fr_pattern` / `ids.sr_pattern` / `ids.ac_pattern` config keys let projects use component-coded ids (e.g. `FR-PR-001`); the PRD ship flow resolves the configured pattern and errors clearly on non-matching arguments instead of silently re-prompting. PRD/SPEC sidecar schemas relax to a permissive superset (skills enforce the configured strict form).
- **Mid-session agent dispatch fallback (ADR-047).** Agents installed by `/edikt:init` register only at session start; routing flows now fall back to a `general-purpose` agent that adopts the persona file, and init/agents/upgrade print the restart requirement. Fixes init step 4b failing to dispatch the sidecar-extractor it had just installed.
- **sidecar-extractor turn budget.** `maxTurns` raised to 8 with auxiliary reads made optional and resource paths resolved absolutely — fixes silent "completed but no output" extractions in consumer projects; compile commands stage the schema under `.edikt/schemas/`.
- **Payload VERSION stamped at release (fixes upgrade-pin warning).** The release workflow now writes the tag into the payload's `VERSION` and asserts it post-tar; `edikt install`/`upgrade` also stamp `versions/<tag>/VERSION` from the tag. Payloads built from an rc-suffixed source tree no longer make every pinned project warn `this project pins edikt X but the active version is Y`.
- **Dropped dead `Write(**)` allow rule (ADR-046, amends ADR-017).** Claude Code matches file-writing tools against `Edit(path)` rules only; the rule produced a warning on every headless run.
- **Typed hook channel, capture gates, and corpus reclassification (ADR-050/051/052, GL-001, INV-012).** Stop-channel hooks emit typed records with MANDATORY subjects, a per-repo surfaced-ledger with `edikt hook ack/held/unack` (visible, reasoned, expiring suppression — auto-fix of governance state is rejected outright), G0 corpus checks before adr-candidates fire, and retired-artifact skip-awareness in the stale scan. GL-001 (the corpus's first guideline) encodes the capture gates — burden of proof on capture, reviewer default DEMOTE — enforced by required template sections (Considered Options, Reversal cost, Falsifiable by) and gate-running intake interviews. INV-002 now codifies the visible-amendment trail (ADR-051); permit lists moved out of ADR bodies to `templates/permits/tier2-verbs.yaml` (ADR-052, superseding ADR-031/033); ADR-003/014/019 demoted; ADR-030's NEVER promoted to INV-012; doctor detects dual-scope hook registration and offers (never silently performs) the cleanup.
- **Zero-file extractor dispatches now fail loudly (RCP field run, bok-services).** Phase A's runner verifies the sidecar actually landed on disk after each dispatch — a `claude` exit 0 with no file written (the stale-agent-definition failure mode) is now a per-task error naming the cause, instead of two silent 20+-agent rounds burning ~30k tokens each. `edikt doctor` additionally warns when an installed agent copy (project `.claude/agents/` or user-level `<claude-root>/agents/`) differs from the active payload template without an `<!-- edikt:custom -->` marker, since Claude Code caches agent definitions at session start.
- **Frontmatter-retired artifacts are excluded from compile (bok-services #2).** Discovery now honours frontmatter `status: superseded` / `status: deprecated` (any `superseded…` value), not just the bolded body `**Status:**` line — and skip-listed artifacts are excluded from Phase A dispatch, Phase B merge, and the verify gate alike, so a retired ADR's leftover or hand-written placeholder sidecar is announced as inert ("safe to delete") instead of compiling duplicate directives.
- **`next-id` computes max+1 and reports duplicates (bok-services #3).** Numbering was count-based, so corpus gaps or duplicate ids (two `SPEC-001-*` dirs) produced a "next" id that already existed on disk. Duplicate numbers are now surfaced as a WARNING line in the live block.
- **`gov compile`'s verify gate is scoped to governance sidecars (bok-services #4).** The post-compile gate now runs `verify all --gov-only`: prd / spec / plan verifies routinely reference deliberately-unbuilt future work and were turning every WIP-project compile into an error exit. Those classes keep their own runners. Phase A also emits a 30s heartbeat while extractions are in flight, and lock contention on `compile.lock` is announced ("held by another process — waiting") instead of blocking silently.
- **Anchor autorepair handles non-byte-exact quotes (bok-services #5).** Staleness and repair now fall back to whitespace-normalized matching across multi-line windows, so quotes with joined wrapped lines or stripped bullets/indentation re-anchor deterministically in one pass instead of whack-a-mole LLM re-dispatch (on the dogfood corpus this resolved 24 anchors with zero LLM calls). The extractor prompt's allowed-keys contract now lists `paths` / `scope` / `prohibitions`, spells out the per-directive key set, and forbids per-directive `scope:` (a `KnownFields` hard error).
- **`edikt verify` accepts both `PLAN-<id>` and bare `<id>` (bok-services #6).** Passing the sidecar's own stem no longer looks up `PLAN-PLAN-…-criteria.yaml`.
- **`CLAUDE_CONFIG_DIR` support.** The Claude-root resolver now honours Claude Code's own profile-selector env var: precedence is `CLAUDE_HOME` (edikt's explicit override) → `CLAUDE_CONFIG_DIR` → `~/.claude`. Multi-profile setups previously got edikt-managed files (notably the managed `permissions` block in `settings.json`) written to `~/.claude` — a directory no session under the active profile reads — with no diagnostic. `edikt doctor` now also warns when `CLAUDE_CONFIG_DIR` is set but overridden by `CLAUDE_HOME` to a different directory. Default behaviour with neither var set is unchanged.

## v0.6.0 (2026-05-24)

> **MIGRATION REQUIRED**
>
> v0.6.0 introduces sidecar architecture. After installing the v0.6.0 launcher, run `/edikt:upgrade` (or `edikt migrate sidecars --apply` for headless flows) to migrate every existing ADR, Invariant Record, and guideline from in-body sentinels to co-located `<artifact>.edikt.yaml` sidecars. **`/edikt:gov:compile` refuses to run until the migration is applied.** Fresh projects (no legacy sentinels) get a no-op scan and continue normally.

The largest release since v0.1.0. Governance metadata moves out of prose into co-located sidecars, the compile pipeline becomes deterministic, and a new verification layer makes completion claims falsifiable rather than self-reported. v0.5.x is retracted — v0.4.x users upgrade directly to v0.6.0.

### Sidecar architecture

- **BREAKING:** Governance directives move from in-body sentinel blocks to co-located `<artifact>.edikt.yaml` sidecars. edikt no longer writes to your ADR / Invariant / guideline `.md` files — the boundary between human-owned bytes (`.md`) and tool-owned bytes (`.edikt.yaml`, `.claude/rules/governance/*.md`) is now structural.
- **Deterministic two-phase compile.** A resync pass refreshes only the sidecars that are stale (the one step that calls an LLM); a merge pass renders the rule files with no LLM at all, so it is reproducible and fast — under 5s cold, under 500ms when nothing changed. `gov compile --check` is CI-safe and exits non-zero on stale sidecars.
- `edikt migrate sidecars` lifts legacy and v0.5.x artifacts into sidecars. `--dry-run` is mandatory before `--apply`; idempotent and fence-aware.
- Editing one sidecar regenerates only its rule file.
- `/edikt:adr:review`, `/edikt:invariant:review`, `/edikt:guideline:review` cross-check each sidecar against its prose body and warn on drift (read-only).

### Falsifiable verification

The governance layer stops trusting self-reported completion. Every mechanically-checkable rule can carry an executable `verify:` command, graded on whether it actually runs code and asserts a property — not whether it matches a pattern an agent can satisfy without doing the work.

- **`bin/edikt verify gov|prd|spec <id>` and `verify all`** execute every `verify:` under bash with a 30-second timeout and a clear exit-code contract; per-run reports persist under `.edikt/state/verify/`. `gov compile` runs the gate after merging.
- **Verify-gate hook.** A pre-edit hook detects completion-claim edits — flipping a sidecar to passing, marking a plan row done, ticking an acceptance-criteria box — and blocks them unless a fresh verify report was read in the same turn. Bypass envelope for the migrate / compile / upgrade flows.
- **Intent-aware diff verification.** `/edikt:gov:verify-diff` evaluates a diff against each directive's stated intent and its falsifying observation rather than the raw wording, so the check resists generator-controlled phrasing.
- **Cheat-rate benchmark.** `bin/edikt gov benchmark cheat-rate` dispatches an adversary model that tries to satisfy a `verify:` without honoring its intent; the aggregate cheat-rate carries a soft ceiling so weak verifies surface before they ship.
- **Human approval for behavioral verifies.** `bin/edikt sidecar approve` and `/edikt:sidecar:approve` capture the approval a behavioral verify requires before it compiles.
- **Compile-quality grader.** `/edikt:gov:grade-compile` scores the compiled governance tree on four editorial dimensions (read-only).

### Post-flight review pipeline

- **`/edikt:sdlc:post-flight`** composes criteria-verify + governance verifier + specialist review + synthesis into one deduplicated report under `.edikt/state/post-flight/`, and gates a plan phase's completion on the combined verdict.
- Fires automatically after a phase, but only when the criteria verify passes; every dispatch-failure mode degrades gracefully without losing the pass signal. Kill-switches: `EDIKT_DISABLE_POST_FLIGHT=1` and `post-flight.enabled: false`.

### PRD v2 and the SDLC chain

- **PRD v2 — split artifact.** Every PRD is a narrative `.md` plus a structured `.yaml` sidecar (functional requirements, acceptance criteria, protections), so multi-turn editing can't corrupt the structured data. v1 PRDs still load. Five non-skippable forcing questions; `solo | team | platform` rigor calibration drives the review threshold (70 / 80 / 90%).
- **Stable IDs end-to-end** — requirement → spec requirement → plan phase → test; `/edikt:sdlc:drift` flags requirements uncovered by any spec.
- New commands: `/edikt:sdlc:prd-review`, `/edikt:sdlc:spec-review`, `/edikt:sdlc:discovery`, plus PRD lifecycle verbs (`ship | supersede | deprecate | cancel`).
- JSON Schemas for the sidecars drive editor autocomplete via `yaml-language-server`.

### Governance accuracy

- Extraction is more faithful: file-path inference, lifecycle-scope defaults, prohibitions synthesized from rejected options, and modality preservation so contingency prose ("Fallback:", "Alternatively:", …) is no longer promoted to a hard requirement.
- `/edikt:adr:enrich` and `bin/edikt sidecar add-manual-directive` add a manual directive to a sidecar without editing an accepted ADR.
- Doctor gains rejected-options coverage, orphan manual-reference, routed-source, sidecar-verify coverage, and verify-gate posture checks.

### Platform, security & harness

- **Single Go binary** (`bin/edikt`) replaces the bash launcher; cross-compiled for macOS/Linux × arm64/amd64, with cosign-signed release assets and a Homebrew tap.
- **The binary never spawns an LLM** — agent dispatch lives in the markdown commands, executed by whichever host agent you run. Enforced by a CI gate, so the binary stays portable across host agents.
- Install and upgrade use tag-pinned, checksum-verified, cosign-verified download paths; `settings.json` is written with JSON-aware substitution and a managed-region integrity sidecar so an upgrade can't silently clobber your edits.
- Pre-push hook enforces edikt's own invariants (markdown-only command surface, immutable accepted decisions, structured-JSON hooks).

### Breaking changes

- Sidecar architecture replaces in-body sentinels; migration is mandatory on first upgrade.
- The pre-edit managed-region guard now scans only `CLAUDE.md`, `settings.json`, and `.edikt/` — edits to governance `.md` files no longer trip it.
- Behavioral verifies require human approval before they compile.
- Deprecated command stubs removed (`commands/deprecated/`) — intent-based routing handles discovery. `/edikt:compile` → `/edikt:gov:compile`, `/edikt:plan` → `/edikt:sdlc:plan`, `/edikt:prd` → `/edikt:sdlc:prd`, with the rest grouped under `gov` / `sdlc` / `docs`.
- SDLC reviews are namespaced under `sdlc:` and disambiguated (no aliases): `/edikt:sdlc:review` → `/edikt:sdlc:code-review` (it reviews the implementation), and the document reviews are `/edikt:sdlc:prd-review` and `/edikt:sdlc:spec-review`. Update any references.

## v0.5.1 (2026-05-01) — retracted, never released

> **Retracted.** v0.5.1 was cut during development but never published as a release. The packaging work below ships in v0.6.0. This section is kept for historical context only — do not install v0.5.x; the launcher refuses to activate it.

Patch release: multi-platform binaries.

v0.5.0 shipped a single linux-amd64 binary for everyone, breaking macOS Homebrew installs (Mach-O vs ELF mismatch). v0.5.1 fixes packaging:

- Release workflow cross-compiles for darwin-arm64, darwin-amd64, linux-arm64, linux-amd64.
- Asset naming: `edikt-v0.5.1-<goos>-<goarch>.tar.gz`. SHA256SUMS covers all four launchers + payload.
- Homebrew formula uses `on_macos`/`on_linux` × `on_arm`/`on_intel` blocks, served from the right asset per platform.
- `install.sh` detects `uname -s` / `uname -m` and fetches the matching tarball.
- No code or governance changes — pure packaging fix.

If you installed v0.5.0 via Homebrew on macOS: `brew upgrade edikt`. If via curl on Linux: `edikt upgrade` is a no-op (already on the right binary).

## v0.5.0 (2026-04-29) — retracted, never released

> **Retracted.** v0.5.0 was cut during development but never published as a release. The Go-binary rewrite, release signing, and security hardening below ship in v0.6.0 — the first public release of this line. This section is kept for historical context only — do not install v0.5.x; the launcher refuses to activate it.

First release with a pure Go binary, full release-integrity signing, and the security-hardened hook surface. Two themes: **edikt is now a single signed binary**, and **the security audit findings are closed with new invariants that prevent regression**.

### Highlights

- **Pure Go binary.** `edikt` is now a single static Go binary (`tools/edikt/`). The previous `edikt-shell` POSIX helper is deleted; `edikt migrate` is native Go. No runtime dependency on bash for user-facing commands.
- **Sigstore keyless release signing.** Every release publishes `SHA256SUMS.sig.bundle` signed by the release workflow's GitHub OIDC identity. `install.sh` and `edikt upgrade` verify with `cosign verify-blob` before extracting any artifact. Without cosign, install aborts unless `EDIKT_INSTALL_INSECURE=1` is set (loud banner).
- **Versioned payload layout + rollback.** Payloads live at `~/.edikt/versions/<tag>/` with a `current` symlink and `lock.yaml` tracking active, previous, pinned. `edikt upgrade` and `edikt rollback` swap generations atomically. Migrations carry forward and are not rolled back.
- **Homebrew distribution.** `brew install diktahq/tap/edikt` installs the launcher; `edikt install` fetches the payload. Launcher and payload update independently.
- **Evaluator verdict persistence.** Phase-end evaluator writes structured JSON to `docs/product/plans/verdicts/<plan>/phase-<N>.json` and updates the criteria sidecar in-place after every run. The plan harness rejects PASS for test-command criteria without `evidence_type: "test_run"` — coerced PASS verdicts are forced to BLOCKED. Existing `done` phases are grandfathered on first upgrade.
- **Directive hardening + governance benchmark.** Directive sentinels gain `canonical_phrases` and `behavioral_signal` fields (backward-compatible). New `/edikt:gov:benchmark` tier-2 command runs adversarial prompts against every governed directive. `/edikt:adr:review --backfill` retrofits `canonical_phrases` onto existing ADRs with per-entry approval. `/edikt:gov:compile` detects orphan ADRs with warn-then-block semantics. `/edikt:doctor` verifies every ADR/INV in the routing table exists on disk.
- **`gov:compile` schema-completeness gate.** The compile no longer silently produces `governance.md` from sentinel blocks missing required fields (`source_hash`, `directives_hash`, `compiler_version`, `manual_directives`, `suppressed_directives`). Aborts with a redirect to the per-artifact compile commands. The inline-fallback that wrote non-conforming blocks via the deprecated `content_hash` field is removed — `<artifact>:compile` is the only sentinel-writing path.

### Security hardening

The v0.5.0 security audit closed the following failure classes and locked them behind new invariants. Six new invariants, four new ADRs — each one prevents an entire category of regression, not a single bug.

#### New invariants

- **Hooks emit structured JSON**, never shell-concatenated strings. Every hook uses `python3 json.dumps` with untrusted values passed as argv. CI lint fails on `echo '{'` / `printf '{'` in hook scripts.
- **Hooks never instruct Claude to execute shell built from untrusted text.**
- **Managed-region integrity is verified before overwrite.** Markdown sentinels use byte-range overlap checks (not regex over `old_string`); `settings.json` uses an out-of-band sidecar at `~/.edikt/state/settings-managed.json`.
- **Externally-controlled inputs are shape-validated before use,** with NFKC + casefold + whitespace-strip normalization so Unicode lookalikes cannot bypass allowlists.
- **Benchmark and test sandboxes are hermetic.** No copy of the host's `~/.claude/settings.json`, user-global settings, or hooks; `setting_sources: ["project"]` only; `shutil.copytree(..., symlinks=True)` with a realpath guard.
- **Release install URLs are tag-pinned, never branch-tracking.** CI fails on `raw.githubusercontent.com/.../main/` or `releases/latest/download/` in `README.md`, `website/`, or `.github/workflows/`.

#### New ADRs

- **Release integrity and Sigstore keyless signing.**
- **Default permissions posture** in `settings.json.tmpl`: 23 deny patterns, 17 allow entries, `defaultMode: askBeforeAllow`. See `docs/guides/permissions.md`.
- **Evaluator verdict schema** with per-criterion `evidence_type`.
- **Narrow carve-out for four security-rewritten hooks.**

### Testing and CI

- **Three-layer harness.** Layer 1: hook unit tests with JSON fixtures (9 suites). Layer 2: Agent SDK integration tests against real Claude (6 tests + 4-test regression museum). Layer 3: sandboxed runner — `$HOME`, `$EDIKT_HOME`, `$CLAUDE_HOME` redirected to per-run temp. No test contaminates developer state.
- **CI gates.** Layers 1 + 3 on every PR. Layer 2 on tag push (requires `ANTHROPIC_API_KEY` secret).
- **Governance integrity tests.** Offline verification of sentinel hashes, routing table linkage, config schema completeness.

### Breaking changes — upgrade notes

- **Install URL changed.** Update bookmarks and CI scripts from `raw.githubusercontent.com/.../main/install.sh` to `https://github.com/diktahq/edikt/releases/download/v0.5.0/install.sh`.
- **New default permissions may prompt.** First-time Claude invocations of `curl http://` or other denied patterns now produce a permission prompt. Allow once if legitimate. User-added permissions belong in a `userPermissions` top-level key (outside the managed region).
- **Install requires cosign.** Set `EDIKT_INSTALL_INSECURE=1` to bypass (loud banner). Recommended: install cosign first.
- **`/edikt:gov:compile` evidence gate.** Existing `done` phases are grandfathered (`meta.grandfathered: true`) — no regression. New phases require `evidence_type: "test_run"` for test-command criteria.
- **`/edikt:gov:compile` aborts on incomplete sentinels.** First-time adoption on a project without sentinels now redirects to `/edikt:adr:compile`, `/edikt:invariant:compile`, `/edikt:guideline:compile` (each supports a no-arg "process all" invocation). Run those once to populate sentinels under the v0.5.0+ schema, then run `gov:compile`.
- **Multi-sentence directives warn.** Directives without `canonical_phrases` produce a compile warning in v0.5.0 (no block). Run `/edikt:adr:review --backfill` to retrofit. Hard-fail is targeted for a subsequent release.

### Rollback

`edikt rollback v0.5.0` restores `~/.claude/settings.json` from the pre-upgrade backup, removes the managed-region sidecar and grandfather stubs. Idempotent. Backup preserved at `~/.edikt/backup/pre-v0.5.0-<ts>/`.

---

## v0.4.3 (2026-04-14)

### Bug fixes

- **Phase-end evaluator now actually runs.** The phase-end evaluator relied on Claude voluntarily following instructions in plan.md to invoke it. When users executed plan phases directly (the common flow), the evaluator was never triggered. Added `phase-end-detector.sh` — a new Stop hook that detects phase completion signals in Claude's output, finds the in-progress phase from the active plan, and auto-invokes the headless evaluator with the phase's acceptance criteria. Logs `phase_completion_detected` and `phase_evaluation` events to `~/.edikt/events.jsonl`.
  - Detection patterns: "Phase N complete/done/finished/implemented", "Implemented phase N", "PHASE N DONE" completion promise format
  - Respects `evaluator.phase-end: false` config to disable
  - Test override: `EDIKT_EVALUATOR_DRY_RUN=1` to detect without invoking claude -p, `EDIKT_SKIP_PHASE_EVAL=1` to skip entirely

- **Upgrade no longer silently overwrites user customizations.** `/edikt:upgrade` compared installed agents against current templates using a simple hash diff and reported any difference as "template updated ⬆" — misleading language that prompted users to accept and lose their customizations. Now classifies diffs into three buckets:
  - **PURE EXPANSION** — template added content, no user content removed. Auto-applied.
  - **PATH SUBSTITUTION** — only paths differ (e.g., `docs/architecture/decisions/` → `adr/`). Flagged as user divergence.
  - **USER DIVERGENCE** — installed file has content not in the template. Prompts individually with diff preview and options: apply template (lose customizations), keep mine (add `<!-- edikt:custom -->` marker), or skip.

- **Evaluator could silently degrade to read-only PASS.** When invoked as a subagent (directly via the Agent tool, or as a fallback from headless), the evaluator inherited the parent session's permission sandbox — which may deny Bash even when the agent's `tools:` frontmatter declares it. With no way to signal "I couldn't verify this," the evaluator fell back to read-only inspection and returned PASS verdicts on acceptance criteria that required test execution.

### Features

- **BLOCKED verdict.** Both evaluator templates (`templates/agents/evaluator.md` and `templates/agents/evaluator-headless.md`) now declare BLOCKED as a valid per-criterion and overall verdict. Rule added: "if a criterion requires execution and execution is unavailable, verdict is BLOCKED — never PASS." The subagent template gained a Capability Self-Check section that probes Bash availability before claiming verdicts.

- **Visible evaluator fallback.** `/edikt:sdlc:plan` now attempts headless first when `evaluator.mode: headless`, falls back to subagent on headless failure with a visible `⚠ EVALUATOR FALLBACK` banner naming the reason and recovery hint, and emits a `✗ EVALUATION FAILED` banner when both modes fail. BLOCKED verdicts now surface per-criterion with recovery hints; the progress table gained a `blocked` state. No silent degradation paths remain.

- **Doctor evaluator probe.** `/edikt:doctor` now probes the evaluator: checks `claude` CLI is on PATH, runs a headless sanity call (`claude -p "echo ok"`), verifies both evaluator templates exist, and reports whether `evaluator.mode` is explicitly set. Each failure has actionable remediation (`claude login`, `/edikt:upgrade`, `/edikt:config set evaluator.mode headless`).

- **`--eval-only {phase}` flag on `/edikt:sdlc:plan`.** Re-run evaluation on a specific phase without re-running the generator. Recovery path for BLOCKED verdicts after the user has fixed the underlying cause (e.g. switching `evaluator.mode` to headless). Routes through the existing Phase-End Flow — no verdict-logic duplication. Optionally combines with `--plan {slug}` when multiple plans exist.

### Governance

- These changes form one decision: headless default, subagent as fallback, BLOCKED over silent PASS, visible warnings, doctor probe, no silent degradation.

### Tests

- 17 new tests in `test-phase-end-detector.sh` covering completion pattern detection, config respect, loop prevention, correct phase selection, event logging, and no-false-positive cases.
- 11 new assertions in `test-v040-evaluator.sh` covering BLOCKED verdicts, Capability Self-Check, never-PASS rule, parent-sandbox warning, fallback/failed banners, `--eval-only` flag documentation, and doctor evaluator probe.

## v0.4.2 (2026-04-13)

### Bug fixes

- **Spec preprocessing.** Blank line between frontmatter and `!`` preprocessing block caused shell corruption. Added missing `argument-hint`.
- **Plan pre-flight skipped.** Pre-flight specialist review and criteria validation (steps 10-11) were ordered after the "Next: execute Phase 1" conclusion (step 9). Claude naturally stopped at the conclusion. Reordered: pre-flight is now steps 8-9, write file is step 10, output is step 11.
- **Audit inline mode.** `--no-edikt` jump target said "step 6" (agent-spawning) but inline audit mode was at step 11. Fixed.
- **Gov review premature conclusion.** "Next: Run /edikt:gov:compile" appeared before staleness detection still needed to run. Moved to actual conclusion.

### Tests

- 15 preprocessing format regression tests (no blank lines, argument-hint, awk integrity)
- 5 step ordering regression tests (plan, audit, review)
- 24 evaluator flow tests (pre-flight + phase-end + bypass protection)
- Version check no longer hardcoded

## v0.4.1 (2026-04-12)

### Bug fixes

- **Upgrade: new agent detection.** `/edikt:upgrade` now detects agent templates added in newer versions. Core agents (evaluator, evaluator-headless) are installed automatically. Optional agents are offered to the user with a description — declined agents are added to `agents.custom` so future upgrades skip them.
- **Upgrade: config migration.** `paths.soul` renamed to `paths.project-context`. Upgrade auto-migrates existing configs.
- **CodeRabbit review fixes.** Subagent-stop override check now matches agent + finding on the same line (was two independent greps). WEAK PASS exit code corrected to 0. .gitignore negation patterns fixed. BSD-only stat removed. Agent count corrected to 18 across website docs.

### Documentation

- Updated `project-context.md` for v0.4.0: hook count (9→13), agent count, quality gates, plan harness features.

## v0.4.0 (2026-04-11)

### Plan Harness: Iteration Tracking, Context Handoff, Criteria Sidecar

The plan command now tracks failure history, carries context across phase boundaries, and emits a machine-readable criteria sidecar.

- **Iteration tracking:** progress table with Attempt column (`N/max`), 6 statuses (`pending`, `in-progress`, `evaluating`, `done`, `stuck`, `skipped`). After each evaluation failure, fail reasons are forwarded to the next attempt. Escalation warning at 3 consecutive failures on the same criterion. Phase goes `stuck` at max attempts (configurable, default 5) with human decision prompt.
- **Context handoff:** each phase has a `Context Needed` field listing files to read before starting. Artifact Flow Table maps producing phases to consuming phases. PostCompact hook re-injects context files, attempt count, and failing criteria after compaction.
- **Criteria sidecar:** `PLAN-{slug}-criteria.yaml` emitted alongside plan markdown. Per-criterion status tracking (pending/pass/fail), verification commands, fail counts. Evaluator reads and updates the sidecar — no markdown parsing needed.

### Evaluator: Headless Execution and Configuration

The evaluator now runs as a separate `claude -p` invocation with zero shared context from the generator session.

- **Evaluator config:** new `evaluator` section in `.edikt/config.yaml` with 5 keys: `preflight` (toggle pre-flight), `phase-end` (toggle evaluation), `mode` (headless or subagent), `max-attempts` (stuck threshold), `model` (sonnet/opus/haiku).
- **Headless mode (default):** evaluator runs via `claude -p --bare` with `--disallowedTools "Write,Edit"`. Fresh process, no shared context, no self-evaluation bias. Falls back to subagent when headless unavailable.
- **Protected agent:** evaluator templates are not user-overridable. Upgrade always overwrites them. Doctor warns on modifications. Plan blocks if template is missing.
- **LLM evaluator in experiments:** `--llm-eval` flag in experiment runner. Dual-mode: grep pre-check + LLM evaluation. LLM verdict is authoritative when both run. Three verdicts: PASS, WEAK PASS (all critical pass but important fails), FAIL. Severity tiers: critical (blocks), important (degrades), informational (logged only).

### Enforcement: Quality Gate UX and Artifact Lifecycle

Quality gates now log overrides with accountability, and artifact lifecycle is enforced uniformly across the SDLC chain.

- **Gate override logging:** overrides written to `~/.edikt/events.jsonl` with git identity (name + email). Three event types: `gate_fired`, `gate_override`, `gate_blocked`.
- **Re-fire prevention:** overridden findings don't fire again within the same session. Overrides cleared at session start.
- **Artifact lifecycle:** full status chain `draft → accepted → in-progress → implemented → superseded`. Plan auto-promotes `accepted → in-progress` when phase starts. Drift auto-promotes `in-progress → implemented` when no violations found.
- **Plan draft warning:** lists specific draft artifacts by name, offers proceed (with Known Risks) or stop.
- **Drift status filter:** skips `draft` and `superseded` artifacts, validates the rest.
- **Doctor:** flags spec-artifacts stuck in draft > 7 days. Parses both YAML frontmatter and comment header status formats.

### Breaking changes

- **Config key rename:** `paths.soul` → `paths.project-context`. `/edikt:upgrade` auto-migrates existing configs. Commands fall back to `soul` if `project-context` is not found.

### Documentation

- Updated `project-context.md` for v0.4.0: hook count (9→13), agent count (20→19), quality gates, plan harness features, context vs enforcement framing
- Fixed 12 pre-existing documentation gaps (stale agent/hook/command counts, old command names in AGENTS.md, missing index entries)
- Updated website: plan, gates, chain, features, doctor, drift pages with v0.4.0 features
- Removed stale AGENTS.md (Codex convention — edikt is Claude Code only)

### New config keys

```yaml
evaluator:
  preflight: true       # pre-flight criteria validation
  phase-end: true       # phase-end evaluation
  mode: headless        # headless | subagent
  max-attempts: 5       # max retries before stuck
  model: sonnet         # model for headless evaluator
```

## v0.3.1 (2026-04-11)

### Bug fixes

- **Init: guidelines path.** `/edikt:init` now writes `paths.guidelines` correctly.
- **VERSION stamp.** `VERSION` file updated to match release tag.
- **PRINCIPAL prefix.** Compile output no longer prefixes directives with `PRINCIPAL:`.
- **Review output.** `/edikt:sdlc:review` output formatting fixed.
- **SubagentStop hook: seniority prefix.** The fallback agent detection pattern matched "As Principal Architect" → `principal-architect` instead of `architect`, breaking slug lookup and gate matching. Now extracts only the role word.
- **Missing page.** Added `/edikt:guideline:compile` website page (was dead link).
- **Test fixes.** All 25 suites pass after v0.3.0 regressions.

### Artifact generation: JSONB support and domain class diagram

`/edikt:sdlc:artifacts` now handles projects using JSONB aggregate storage (common DDD pattern in PostgreSQL) and generates a domain class diagram alongside the data model.

- **Storage strategy detection.** When DB type is `sql` or `mixed`, the command scans spec content and migrations for JSONB signals (`jsonb`, `json column`, `aggregate storage`, `embedded entity`, `nested entity`, etc.). Detected strategy is shown in the state checkpoint and routing output.
- **Three entity modes in `data-model.mmd`.** When storage strategy is `jsonb-aggregate`, the ERD distinguishes physical tables (normal), JSONB-embedded entities (relationship label contains `jsonb`), and reference-only entities from external bounded contexts (relationship label contains `ref`). Makes nested structure visible instead of hiding it in JSONB column comments.
- **Domain class diagram (`model.mmd`).** New artifact type, always generated alongside the data model regardless of DB type. Mermaid `classDiagram` showing aggregate roots, value objects, entities, inheritance, composition, and domain methods. Reviewed by the architect agent.

### Configurable artifact spec versions

Artifact templates now use configurable spec versions instead of hardcoded values. Defaults updated to latest stable:

| Format | Previous | Now (default) |
|---|---|---|
| OpenAPI | 3.0.0 | **3.1.0** |
| AsyncAPI | 2.6.0 | **3.0.0** |
| JSON Schema | draft-07 | **2020-12** |

Teams can pin older versions in `.edikt/config.yaml`:

```yaml
artifacts:
  versions:
    openapi: "3.0.0"       # pin for tooling compatibility
    asyncapi: "2.6.0"      # pin if not ready for 3.0 breaking changes
    json_schema: "https://json-schema.org/draft/07/schema#"
```

The AsyncAPI template was updated for the 3.0 structure (separate `channels` and `operations` blocks replacing `publish`/`subscribe`). When pinning `asyncapi: "2.6.0"`, the agent uses the 2.x structure.

### New `/edikt:config` command

View, query, and modify `.edikt/config.yaml` with discovery, validation, and natural-language changes.

- **No args** — show all 34 config keys with current values and defaults
- **`get {key}`** — show a specific key's value, default, valid values, and which commands use it
- **`set {key} {value}`** — validate and write, with per-key validation rules

Protected keys like `edikt_version` cannot be set directly. Invalid values are rejected with explanation.

### `/edikt:team` deprecated — merged into init + config

`/edikt:team` served two purposes that belong elsewhere:
- **Member onboarding** → now in `/edikt:init`'s "existing project" path
- **Config management** → now in `/edikt:config`

When `/edikt:init` detects an existing `.edikt/config.yaml`, it runs member environment validation instead of saying "already initialized":
1. **Version gate** — blocks if installed edikt < project's `edikt_version`
2. **Environment checks** — git identity, Claude Code, MCP env vars (read dynamically from `.mcp.json`), `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`, pre-push hook, managed settings
3. **Governance gap sync** — missing rules/hooks/agents offered for install
4. **Shared config display** — what's committed to git

The `team:` config block is no longer used. Legacy blocks in existing configs are ignored silently. The deprecated stub redirects to init and will be removed in v0.5.0.

## v0.3.0 (2026-04-10)

### Project Adaptation

edikt can now adapt to existing projects. The compile pipeline supports a **three-list directive schema** with hash-based caching, and introduces **Invariant Records** as the formal governance artifact for hard constraints.

- **Three-list schema:** every compiled sentinel block now carries `directives:` (auto-generated), `manual_directives:` (user-authored), and `suppressed_directives:` (user-rejected). The merge formula `effective = (directives - suppressed) ∪ manual` gives users full control over what ships without losing compile automation. Hash-based caching (`source_hash` + `directives_hash`) skips Claude calls when nothing changed.
- **Invariant Records:** formalized the governance artifact for non-negotiable constraints. Formalized "Invariant Records" as the governance artifact for hard constraints (short form: INV). Template follows Statement/Rationale/Enforcement structure. Compile extracts directives from the Statement section, preserving declarative absolute language.
- **Extensibility plumbing:** template lookup chain (`project .edikt/templates/` → inline fallback), `/edikt:guideline:compile` command, auto-chain (`<artifact>:new` runs `<artifact>:compile`).
- **Init adapt mode:** adapt mode for existing `.edikt/` directories. Template-less refusal for v0.3.0+ projects. (corrected 2026-08-19: style detection was never shipped)
- **Flexible prose input:** ADR/invariant/guideline creation accepts natural language with automatic reference extraction to existing governance.
- **Doctor + upgrade integration:** doctor reports template overrides and stale hashes. Upgrade respects project templates.

### Compile Improvements

Experiment-driven improvements to the compile output format. These changes improve how well Claude follows governance directives.

- **"No exceptions." reinforcement:** invariant directives derived from absolute-language Statements ("every", "all", "total") now get "No exceptions." appended. Experiments showed this phrase prevents Claude from rationalizing edge cases.
- **Reminders sentinel (`[edikt:reminders:start/end]`):** compile now generates pre-action interrupts: "Before writing SQL → MUST include tenant_id." Aggregated into a `## Reminders` section in governance.md. Capped at 10.
- **Verification checklist:** compile generates a `## Verification Checklist` section with grep-verifiable items Claude checks before finishing. Capped at 15 items.
- **Per-directive LLM compliance scoring** in `/edikt:invariant:review`, `/edikt:adr:review`, `/edikt:guideline:review`: scores each compiled directive on token specificity, MUST/NEVER usage, grep-ability, ambiguity, and friction risk. Manual directives held to the same standard.
- **New `/edikt:gov:score` command:** aggregate governance quality report — context budget, compliance metrics, manual directive health. JSON output for CI.

### Pre-flight Criteria Validation

The evaluator agent now supports a **pre-flight mode** that validates acceptance criteria BEFORE the generator starts. Classifies each criterion as TESTABLE/VAGUE/SUBJECTIVE/BLOCKED and proposes verification commands. The plan command (step 11) invokes pre-flight automatically, preventing wasted iterations on untestable criteria.

### Experiments

Pre-registered experiments measuring whether governance directives change Claude's output on real coding tasks. 8 experiments across 4 scenario types.

| Scenario | Baseline | Governance | Effect |
|---|---|---|---|
| Existing codebase | PASS | PASS | Absent — code patterns self-teach |
| Greenfield | VIOLATION | PASS | **Present** — governance prevents architecture/tenant violations |
| New domain on existing | VIOLATION | PASS | **Present** — governance catches log/SQL misses |
| Long context (N=2) | 1/2 VIOLATION | 0/2 PASS | **Present** — governance stabilizes under context pressure |

Key findings: governance has measurable effect on greenfield and new-domain code. Directive format matters — MUST/NEVER with literal code tokens outperforms prose. Long context degrades convention compliance; governance in `.claude/rules/` survives because it's loaded separately from the conversation. Full methodology and results in `test/experiments/`.

### New commands

- `/edikt:gov:score` — aggregate governance quality scoring for CI

## v0.2.3 (2026-04-09)

### Compile schema version

`/edikt:gov:compile` now stamps generated governance files with a **compile schema version** — a small integer independent of edikt's marketing version — instead of the edikt version at compile time.

**Problem this fixes:** before v0.2.3, `.claude/rules/governance.md` was stamped with `version: "<edikt-version>"`, conflating two unrelated cadences. Every edikt point release (even pure bug fixes) implied governance was stale and needed regeneration, but the compile output format hadn't actually changed. In the dogfood repo, we kept hand-editing `governance.md`'s version via `sed` on each release to keep a test green — the file ended up lying about its own provenance (version said v0.2.2 but the compile timestamp was frozen at March 25).

**New format:**

```yaml
---
paths: "**/*"
compile_schema_version: 2
---
<!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
<!-- compiled_by: edikt v0.2.3 -->
<!-- compiled_at: 2026-04-09T10:30:00Z -->
```

Three fields, three purposes:

- **`compile_schema_version`** (YAML, enforced) — identifies the output format contract. `1` = v0.1.x flat governance, `2` = v0.2.x topic-grouped rule files. `/edikt:doctor` checks it against the constant declared in `commands/gov/compile.md` and recommends `/edikt:gov:compile` only when the format has actually changed.
- **`compiled_by`** (HTML comment, informational) — which edikt version ran compile. Diagnostic only, never enforced.
- **`compiled_at`** (HTML comment, informational) — ISO8601 timestamp. Diagnostic only, never enforced.

**Consequences:**
- No more false-positive staleness warnings on point releases. Users only see "regenerate governance" when the compile schema actually changed.
- Point releases can ship bug fixes without implying anything about compile output compatibility.
- `/edikt:doctor` gets smarter about stale governance detection.
- `/edikt:upgrade` has a new step that checks the project's schema version against the installed compile schema and recommends (but does not auto-run) regeneration when they diverge.
- Dogfooding stops hand-editing `governance.md`'s version field. The dogfood file now uses the new format honestly.

### Installer UX fixes

Three bug reports from real installs, all fixed in the same release.

- **No prompt on `curl | bash`.** The interactive "global vs project" prompt was skipped silently when stdin was a pipe (the common `curl -fsSL ... | bash` invocation). Now the installer reads from `/dev/tty` when available, so the prompt fires even when stdin is consumed by the curl pipe. Falls back to `--global` only when there's no TTY at all (CI, fully redirected).
- **Commands duplicated across global and project locations.** When a user installed globally in a directory that already had a project-local edikt install (either from a prior `--project` run, or from the dogfood repo itself), Claude Code ended up loading commands from both `~/.claude/commands/edikt/` and `.claude/commands/edikt/`, producing duplicates in the skill list. The installer now detects this condition at startup and emits a yellow warning pointing at the exact paths and the `rm -rf` to clean them up. Never auto-deletes.
- **No detection of existing install before project install.** If a user ran `install.sh --project` in a directory where `~/.edikt/VERSION` already existed, the two installs would silently overlap. Same detection now fires a warning for this case too. Both detection paths share the same `HAS_GLOBAL` / `HAS_PROJECT` flags.
- **New test scenarios in `test/test-install-e2e.sh`** — scenarios 6 and 7 cover the duplication-warning paths (6 = global install with leftover project files; 7 = project install with existing global install). Total scenarios now: 7. Total assertions: 28.

### Tests

- **New `test/test-v023-regressions.sh`** (21 assertions) — verifies `COMPILE_SCHEMA_VERSION` is declared in compile.md, output templates emit the new format, doctor.md checks the schema version, upgrade.md documents the migration step, and the dogfood governance file matches the constant.
- **`test-e2e.sh` version check refactored** — no longer enforces `GOV_VER == FILE_VER`. Instead it validates that `compile_schema_version` in the dogfood governance file matches the `COMPILE_SCHEMA_VERSION` constant in `commands/gov/compile.md`.

## v0.2.2 (2026-04-08)

Critical bug-fix release. The v0.2.1 installer was silently broken on the v0.1.x → v0.2.x upgrade path.

### Installer: upgrade from v0.1.x was silently broken

- **`((BACKUP_COUNT++))` under `set -euo pipefail` killed the installer on the first backup.** Postfix `++` returns the pre-increment value (`0` on the first call), which bash's `set -e` treats as a failure and exits the script. Symptoms: the cleanup loop removed *nothing*, the new namespaced commands were *never* installed, old flat files stayed in place, and the installer exited without any error message. This shipped in v0.2.1 and affected everyone upgrading from v0.1.x via `curl | bash`. Fixed by using `BACKUP_COUNT=$((BACKUP_COUNT + 1))`.
- **New integration test** (`test/test-install-e2e.sh`) — 22 assertions across five scenarios: fresh install, upgrade from v0.1.x, user-customized file preservation, network failure abort, and repeated-install idempotency. Shims `curl` with a mock that serves files from the local repo, so the full `install.sh` runs end-to-end against a fake `$HOME` in `/tmp`. This is the test we wished existed before v0.2.0 shipped — it caught the v0.2.1 regression immediately.

### `/edikt:upgrade`: migrate v0.1.x command references

- **New step 5 in `/edikt:upgrade`: rewrite old flat command references in project files to their new namespaced equivalents.** Projects initialized with v0.1.x have hardcoded references to `/edikt:adr`, `/edikt:plan`, `/edikt:compile`, etc. in their `CLAUDE.md` managed block and in compiled rule packs. Previously, `/edikt:upgrade` only migrated the *sentinel format* (HTML → visible) and left the *content* inside the sentinels untouched. Now upgrade runs a targeted string-replace across all 15 moved commands, scoped to edikt-owned content only (the CLAUDE.md managed block and rule pack files marked with `edikt:generated` or `edikt:compiled`). User content outside the managed blocks is never touched.
- **Idempotent and safe:** the instruction tells Claude to match only occurrences NOT already followed by `:`, using surrounding context (backticks, punctuation, end-of-line) for disambiguation. Running upgrade twice is a no-op.

## v0.2.1 (2026-04-08)

Bug-fix release following v0.2.0 field reports.

### Installer upgrade path

- **Old flat commands no longer linger after upgrade.** v0.1.x installed commands like `~/.claude/commands/edikt/adr.md`, `plan.md`, `compile.md` at the top level. v0.2.0 moved them into namespaces but the installer never removed the old files, so users saw both `/edikt:adr` (stale) and `/edikt:adr:new` (new) in their command list. The installer now deletes the 15 moved v0.1.x commands before installing new files, with backup. User-customized files (marked with `<!-- edikt:custom -->`) are preserved.
- **Silent curl failures now abort the install.** Every `curl -o` call now goes through a `_fetch` helper that enforces `--retry 2`, `--max-time 30`, non-empty download verification, and exits with an error on failure. Previously a network blip during `curl | bash` could leave files partially updated without any warning.

### `/edikt:init` ADR path adoption

- **init now configures `paths.decisions` to match detected ADR locations.** Previously, init detected existing ADRs in folders like `docs/decisions/` and offered to import them, but the import flow hardcoded the destination to edikt's default (`docs/architecture/decisions/`) and never wrote the detected path into `.edikt/config.yaml`. Users ended up with ADRs in one place and edikt looking for them somewhere else — `/edikt:gov:compile` and `/edikt:status` reported zero ADRs.
- New prompt: **[1] Adopt** (keep ADRs where they are, configure edikt to use that path), **[2] Migrate** (move to edikt's default layout), **[3] Skip**. Same flow for invariants.

### Command documentation cleanup

- **Seniority prefixes removed from `/edikt:sdlc:review` reviewer lenses.** The command documentation still labeled agents as `Principal DBA`, `Staff SRE`, `Staff Security`, `Senior API`, `Principal Architect`, `Senior Performance` — inconsistent with the agent templates which dropped seniority prefixes in v0.2.0. Now just `DBA`, `SRE`, `Security`, `API`, `Architect`, `Performance`.

### Website content

- **Fixed 10 dead links in `website/governance/chain.md`, `website/governance/compile.md`, `website/governance/drift.md`, and `website/commands/brainstorm.md`** — they referenced old flat command paths (`/commands/prd`, `/commands/spec`, `/commands/plan`, etc.) that broke the v0.2.0 VitePress deploy. Now use namespaced paths (`/commands/sdlc/prd`, `/commands/gov/compile`, etc.).

### Test coverage

- New `test/test-v021-regressions.sh` — 36 assertions guarding against all five v0.2.1 bugs so they can't silently return.

## v0.2.0 (2026-03-31)

### Intelligent Compile — topic-grouped rule files

`/edikt:compile` no longer produces a single flat `governance.md`. It now generates **topic-grouped rule files** under `.claude/rules/governance/` — each topic file contains full-fidelity directives from all sources (ADRs, invariants, guidelines), loaded automatically by path matching.

- **Directive sentinels** — ADRs and invariants can include `[edikt:directives:start/end]` blocks with pre-written LLM directives. Compile reads these verbatim — no extraction, no distillation.
- **Routing table** — `governance.md` becomes an index with invariants + a routing table. Claude matches task signals and scopes to load relevant topic files.
- **Three loading mechanisms** — `paths:` frontmatter (platform-enforced on file edits), `scope:` tags (activity-matched for planning/design/review), and signal keywords (domain-matched).
- **No directive cap** — the 30-directive limit is removed. Soft warning if a topic file exceeds 100 directives.
- **Reverse source map** — compile output shows which ADRs/guidelines contributed to which topic files.
- **Sentinel generation moved to compile** — `/edikt:compile` now generates missing sentinel blocks inline before compiling. No separate step needed. `/edikt:review-governance` is now pure language quality review + staleness detection.
- `/edikt:review-governance` redesigned — language quality review only. Detects stale sentinels and directs to compile. No longer generates anything.

### Command namespacing

edikt commands are now grouped into namespaces matching the artifacts they touch. Nested namespacing confirmed working in Claude Code.

**New structure:**
- `edikt:adr:new` / `:compile` / `:review` — ADR lifecycle
- `edikt:invariant:new` / `:compile` / `:review` — invariant lifecycle
- `edikt:guideline:new` / `:review` — guideline management
- `edikt:gov:compile` / `:review` / `:rules-update` / `:sync` — governance assembly
- `edikt:sdlc:prd` / `:spec` / `:artifacts` / `:plan` / `:review` / `:drift` / `:audit` — SDLC chain
- `edikt:docs:review` / `:intake` — documentation
- `edikt:capture` — mid-session decision sweep (new command)

**New commands:** `capture`, `guideline:new`, `guideline:review`, `adr:compile`, `adr:review`, `invariant:compile`, `invariant:review`

**Deprecated** (removed in v0.4.0): old flat names (`edikt:adr`, `edikt:compile`, `edikt:spec`, etc.) — each stub tells you the new name.

### Agent governance

All 19 agent templates now include governance frontmatter:

- **`maxTurns`** — 10 for advisory agents, 20 for code-writing agents, 15 for the evaluator.
- **`disallowedTools`** — advisory agents have `Write` and `Edit` disallowed at the platform level.
- **`effort`** — high for architect/security/qa/performance/compliance, medium for backend/frontend/dba/api/sre/docs/pm/data/platform/ux, low for gtm/seo.
- **Agent effort fixes** — `data` was `low` with `disallowedTools: [Write, Edit]` which blocked artifact creation. Fixed to `medium` with write access. `platform`, `compliance`, and `ux` effort levels corrected to match their review depth.
- **`initialPrompt`** — architect, security, and pm auto-load project context when run as main session agents.
- **New `evaluator` agent** — phase-end evaluator that verifies work against acceptance criteria with fresh context. Skeptical by default.

### Hook modernization

- **Conditional `if` field** on PostToolUse (scopes to code files only) and InstructionsLoaded (scopes to rule files only). Avoids spawning hook processes for non-matching files.
- **4 new hooks** — `StopFailure` (logs API errors), `TaskCreated` (tracks plan phase parallelism), `CwdChanged` (monorepo directory detection), `FileChanged` (warns on external governance file modifications).

### Harness improvements

- **Context reset guidance** — at phase boundaries, edikt recommends starting a fresh session. State lives in the plan file.
- **Phase-end evaluation** — evaluator agent checks acceptance criteria with binary PASS/FAIL per criterion before suggesting context reset.
- **Acceptance criteria per phase** — plans now include testable, binary assertions per phase. Specs enforce downstream flow.
- **Conditional evaluation** — `evaluate: true/false` per phase. High-effort phases evaluate by default, low-effort skip.
- **Evaluator tuning** — `docs/architecture/evaluator-tuning.md` tracks false positives/negatives for prompt refinement.
- **Harness assumptions** — `docs/architecture/assumptions.md` documents 6 testable assumptions about model limitations. `/edikt:upgrade` prompts for re-testing.

### Rule pack UX

- **Conflict detection** — `/edikt:rules-update` checks new rule packs against compiled governance before installing.
- **Install preview** — shows what will change (added/changed/removed sections) before applying updates.
- **Override transparency** — `/edikt:doctor` and `/edikt:status` report compiled governance status, sentinel coverage, and rule pack overrides.

### Installer safety

- **`--dry-run`** — preview what the installer would change without writing files.
- **Backup before overwrite** — existing files backed up to `~/.edikt/backups/{timestamp}/` before overwriting.
- **Existing install detection** — reports installed version and confirms before proceeding.

### Headless & CI foundations

- **`--json` output** — compile, drift, audit, doctor, review, and review-governance support `--json` for machine-readable output.
- **Headless mode** — `EDIKT_HEADLESS=1` with `headless-ask.sh` hook auto-answers interactive prompts for CI pipelines.
- **CI guide** — new website guide with GitHub Actions example, recommended settings, and environment variables.
- **Managed settings awareness** — `/edikt:team` detects organization-managed settings (`managed-settings.json`, `managed-settings.d/`).

### UX consistency improvements

- **Standardized completion signals** — all 25 commands end with `✅ {Action}: {identifier}` + `Next:` line.
- **Standardized error messages** — all commands that read config use the same missing-config message.
- **Config guards** — 10 additional commands now guard for missing `.edikt/config.yaml` instead of failing mid-execution.
- **Init rule preview** — step 3b shows a preview of actual rules before generating files, with customization paths taught at the moment of installation.
- **Init reconfigure protection** — content hash comparison detects edited files. Per-file `[1] Overwrite / [2] Keep mine / [3] Show diff` flow instead of silent overwrites.
- **Composite config screen** — SDLC options merged into the single combined rules/agents view. One screen, one confirmation.
- **Concrete init summary** — before/after with stack-specific examples from installed rules and agents.
- **Agent routing standardized** — all commands use `🔀 edikt: routing to {agents}` format.
- **Progress breadcrumbs** — compile, audit, review, drift, and review-governance show `Step N/M:` progress.
- **Numbered confirmation options** — letter-code choices (`[a]/[s]/[k]`) replaced with `[1]/[2]/[3]`.
- **Emoji key** — output conventions table added to CLAUDE.md template.

### Bug fixes

- **Plan ignores spec artifacts when generating phases** — `/edikt:plan` now scans the spec directory for all artifact files (fixtures, test strategy, API contracts, event contracts, migrations) and verifies each has plan coverage. Uncovered fixtures get a seeding phase, uncovered test categories get test tasks, uncovered API endpoints get a warning. A hard gate (step 6c) blocks plan writing if any artifact has no coverage — the user must add phases, defer explicitly, or cancel. Prevents silent failures where artifacts are generated but never consumed.
- **Cross-reference validation in compile and review-governance** — both commands now verify that every `(ref: INV-NNN)` and `(ref: ADR-NNN)` reference points to an actual source document. Fabricated references are stripped before writing.
- **Plan trigger not matching "let's create a plan to fix X"** — added trigger examples with trailing context ("plan to fix these issues", "plan these changes", "plan this work") so Claude matches the plan intent even when the sentence includes what to fix.
- **SessionStart hook errors on compact** — `set -euo pipefail` caused silent non-zero exits when Claude Code fires `SessionStart` after `/compact`. Relaxed to `set -uo pipefail` — the hook already guards every fallible command with `|| true`.
- **Test suite requires pyyaml** — agent and registry tests used `python3 -c "import yaml"` which fails silently when pyyaml isn't installed. Rewrote agent frontmatter checks in pure bash, registry checks to fall back to `yq`, and `assert_valid_yaml` to try `yq` when python3-yaml is unavailable.

### Platform alignment

- **Environment hardening** — `/edikt:team` checks for `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`. Security guide documents `sandbox.failIfUnavailable`.
- **SendMessage auto-resume** — documented on website for agent resumption.

## v0.1.4 (2026-03-28)

### Brainstorm command

New `/edikt:brainstorm` command — a thinking companion for builders. Open conversation grounded in project context, with specialist agents joining as topics emerge. Converges toward a PRD or SPEC when ready. Use `--fresh` for unconstrained brainstorming that challenges existing decisions. Brainstorm artifacts saved to `docs/brainstorms/`.

### Upgrade version check

`/edikt:upgrade` now checks for newer edikt releases before upgrading the project. If a newer version exists, it shows the install command and stops — ensuring project upgrades always use the latest templates. Skip with `--offline` for air-gapped environments.

## v0.1.3 (2026-03-27)

### Flexible plan input

`/edikt:plan` now accepts any input format — natural language prompts, existing plan names, ticket IDs, SPEC identifiers, or nothing (infers from conversation context). When the intent is ambiguous (natural language or conversation context), edikt offers a choice between a full phased plan (saved to `docs/plans/`) and a quick conversational plan.

- `PLAN-NNN` input: continue from current phase, re-plan remaining phases, or create a sub-plan
- Empty input: infers from current conversation context before asking
- Natural language: offers full vs quick plan disambiguation

## v0.1.2 (2026-03-27)

### Bug fix

- **Installer prompt auto-answered when piped** — `curl | bash` triggered the interactive install mode prompt which got EOF from stdin, flashing the prompt and auto-selecting global. Now detects non-terminal stdin and defaults to global silently. Use `--project` flag for project-local install.

## v0.1.1 (2026-03-27)

### Numbered findings in reviews

All review commands now enumerate findings with IDs (#1, #2, #3...) so users can select which to address by number.

- `/edikt:plan` — pre-flight findings numbered, triage prompt: "Which findings should I address? (e.g., #1, #4 or 'all critical')"
- `/edikt:review` — implementation review findings numbered across all agents
- `/edikt:audit` — security and reliability findings numbered across sections
- `/edikt:drift` — diverged findings include triage prompt
- `/edikt:doctor` — warnings and failures numbered for easy reference

### Natural language triggers for all 24 commands

The CLAUDE.md command table now matches intent, not exact phrases. All 24 commands have natural language triggers (was 14). Each command has an intent label and broader representative examples. "Create me a plan for this ticket", "help me plan this out", "spec this out", "are we on track with the spec", "run a security audit", "check my setup" — all trigger the right command.

### Bug fixes

- **Init hook filename hallucination** — `/edikt:init` now reads the settings template exactly as-is instead of generating hook filenames. Fixes `stop-signals.sh: No such file or directory` error.
- **PostToolUse gofmt error** — `gofmt -w` failures on invalid Go syntax no longer propagate as hook errors.
- **Drift report only saving frontmatter** — `/edikt:drift` now explicitly writes the full report (frontmatter + body), not just the frontmatter.
- **Plan mode guard** — All 8 interactive commands (`init`, `plan`, `prd`, `spec`, `spec-artifacts`, `adr`, `invariant`, `intake`) now detect plan mode and tell you to exit it first, instead of silently skipping the interview.
- **Installer preserves customized commands** — `install.sh` now checks for `<!-- edikt:custom -->` before overwriting, so customized commands survive reinstall.

### spec-artifacts redesign — design blueprints with database type awareness

`/edikt:spec-artifacts` now treats every artifact as a design blueprint: it defines intent and structure, not implementation. Your code is the implementation.

**Database-type-aware data model.** The data model artifact format is now resolved from your database type:

- SQL → `data-model.mmd` (Mermaid ERD with entities, relationships, index comments)
- MongoDB/Firestore → `data-model.schema.yaml` (JSON Schema in YAML)
- DynamoDB/Cassandra → `data-model.md` (access patterns, PK/SK/GSI design)
- Redis/KV stores → `data-model.md` (key schema table with TTL and namespace)
- Mixed stacks → both artifacts, suffixed to avoid collision (`data-model-sql.mmd`, `data-model-kv.md`, etc.)

**Database type resolution — four-priority chain:** spec frontmatter `database_type:` → config `artifacts.database.default_type` → keyword scan of spec content → ask the user. Config is set automatically by `/edikt:init` from code signals.

**Native artifact formats.** API contracts are now OpenAPI 3.0 YAML (`contracts/api.yaml`). Event contracts are AsyncAPI 2.6 YAML (`contracts/events.yaml`). Fixtures are portable YAML (`fixtures.yaml`). Migrations are numbered SQL files (`migrations/001_name.sql`). No more markdown wrappers.

**Migrations are SQL-only.** Document and key-value databases never produce migration files.

**Invariant injection.** Active invariants are loaded from your governance chain, stripped of frontmatter, and injected as structured constraints into every agent prompt. Superseded invariants are excluded. Empty invariant bodies emit a warning.

**Design blueprint header.** Every generated artifact gets a format-appropriate comment header marking it as a blueprint, not implementation code.

**Config contract.** `/edikt:init` now detects database type and migration tool from code signals and writes `artifacts.database.default_type` and `artifacts.sql.migrations.tool` to config. The `artifacts:` block is now part of the standard config schema.

### HTML sentinel migration — CLAUDE.md section boundaries now visible to Claude

Claude Code v2.1.72+ hides `<!-- -->` HTML comments when injecting `CLAUDE.md` into Claude's context. The old `<!-- edikt:start -->` / `<!-- edikt:end -->` sentinels were invisible to Claude, so asking Claude to "edit my CLAUDE.md" could accidentally overwrite edikt's managed section.

New format uses markdown link reference definitions, which survive Claude Code's injection intact:

```
[edikt:start]: # managed by edikt — do not edit this block manually
...
[edikt:end]: #
```

- `/edikt:init` writes the new format on fresh installs and migrates old markers when re-running
- `/edikt:upgrade` detects and migrates old HTML sentinels as part of the upgrade flow
- Both old and new formats are detected for backward compatibility

### Effort frontmatter on all commands

All 24 commands now declare `effort: low | normal | high` in their frontmatter. Claude Code uses this to tune the model's thinking budget per command.

- `low` — `agents`, `context`, `mcp`, `status`, `team`
- `normal` — `adr`, `compile`, `doctor`, `init`, `intake`, `invariant`, `review-governance`, `rules-update`, `session`, `sync`, `upgrade`
- `high` — `audit`, `docs`, `drift`, `plan`, `prd`, `review`, `spec`, `spec-artifacts`

### Init improvements

- **Existing ADR import** — `/edikt:init` now detects existing architecture decisions and offers to import them into edikt's governance structure.
- **Project-local install** — `install.sh --project` installs edikt into the current project (`.claude/commands/`, `.edikt/`) instead of globally. Default is still global.
- **Database detection** — `/edikt:init` detects database type and migration tool from 30+ code signals across Go, Node, Python, Ruby, C#, Elixir, and Rust. Definitive signals (e.g., `prisma/schema.prisma`) auto-configure. Inferred signals (package dependencies) are flagged. Nothing found triggers targeted greenfield questions.

## v0.1.0 (2026-03-23)

### First public release

edikt governs your architecture and compiles your engineering decisions into automatic enforcement. It governs the Agentic SDLC from requirements to verification — closing the gap between what you decided and what gets built.

**Architecture governance & compliance**
- `/edikt:compile` reads accepted ADRs, active invariants, and team guidelines, checks for contradictions, and produces `.claude/rules/governance.md` — directives Claude follows automatically every session
- 20 rule packs (10 base, 4 lang, 6 framework) — correctness guardrails, not opinions. 14-17 instructions per pack (research-validated sweet spot)
- Domain-specific governance checkpoints with pre-action and post-result verification
- Signal detection: stop hook detects architecture decisions mid-session, suggests ADR capture
- Quality gates: configure agents as gates in `.edikt/config.yaml`. Critical findings block progression with logged override
- Pre-push invariant check: violations block the push. Override with `EDIKT_INVARIANT_SKIP=1`

**Agentic SDLC governance**
- Full traceability chain: `/edikt:prd` → `/edikt:spec` → `/edikt:spec-artifacts` → `/edikt:plan` → execute → `/edikt:drift`
- Status-gated transitions: PRD must be accepted before spec, spec before artifacts
- `/edikt:drift` compares implementation against the full chain with confidence-based severity
- CI support: `--output=json` with exit code 1 on diverged findings

**18 specialist agents**
- architect, api, backend, dba, docs, frontend, performance, platform, pm, qa, security, sre, ux, data, mobile, compliance, seo, gtm
- Used in spec review, plan pre-flight, post-implementation review, and audit

**9 lifecycle hooks**
- SessionStart: git-aware briefing with domain classification
- UserPromptSubmit: injects active plan phase into every prompt
- PostToolUse: auto-formats files after edits
- PostCompact: re-injects plan + invariants after context compaction
- Stop: regex-based signal detection for decisions, doc gaps, security
- SubagentStop: logs agent activity, enforces quality gates
- InstructionsLoaded: logs active rule packs
- PreToolUse: validates governance setup
- PreCompact: preserves plan state

**24 commands**
- Governance chain: `init`, `prd`, `spec`, `spec-artifacts`, `plan`, `drift`, `compile`
- Decisions: `adr`, `invariant`
- Review: `review`, `audit`, `review-governance`, `doctor`
- Observability: `status`, `session`, `docs`
- Setup: `context`, `intake`, `upgrade`, `rules-update`, `sync`, `team`, `mcp`, `agents`

**Research**
- 123 eval runs across 2 experiments proving rule compliance mechanism
- First experiment: 15/15 compliance with rules vs 0/15 without on invented conventions
- Second experiment: holds under multi-rule conflict, multi-file sessions, Opus vs Sonnet, adversarial prompts
- Reproducible: `test/experiments/rule-compliance/exp-001-scenarios/` and `test/experiments/rule-compliance/exp-002-scenarios/`

**Website**
- Full documentation at edikt.dev
- Guides: solo engineer, teams, multi-project, greenfield, brownfield, monorepo, security, daily workflow
- Governance section: chain, gates, compile, drift, review-governance

**Zero dependencies**
- Every file is `.md` or `.yaml` — no build step, no runtime, no daemon
- `curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash`
- Claude Code only — uses platform primitives (path-conditional rules, lifecycle hooks, slash commands, specialist agents, quality gates)
