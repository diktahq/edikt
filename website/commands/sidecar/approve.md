# edikt sidecar approve

**Review a pending behavioral verify proposal and promote, reject, or defer it.** Captures the human approval a behavioral verify command requires before it can compile. The pending proposal is written by the sidecar-extractor to `.edikt/state/pending-verifies/<id>.yaml`.

## Synopsis

```bash
/edikt:sidecar:approve                 # lists pending IDs and asks which to review
/edikt:sidecar:approve <pending-id>
```

## How it works

The binary is args-driven and non-interactive. This tier-1 command owns the human UX: it renders the proposal, captures your decision, dispatches `bin/edikt sidecar approve <id> --decision=<...>`, and surfaces the exit code. The binary's stdout is displayed verbatim — never parsed.

1. **Verify binary presence.** Requires the `edikt` tier-2 helper; refuses and directs you to `edikt install edikt` if absent.
2. **Resolve the pending ID.** Uses the supplied `<pending-id>` or lists everything under `.edikt/state/pending-verifies/`. If exactly one pending file exists, it proceeds with it; if several, it asks which to review; if none, it reports nothing to approve.
3. **Read and render the proposal.** Shows the sidecar path, directive index, proposed verify command, intent, falsifying observation, and timestamp, plus any sibling fixtures summary.
4. **Capture the human decision** via a prompt: **approve** (accept as-is), **reject** (discard the proposal and remove the pending file), **defer** (leave both pending file and sidecar untouched), or **edit** (revise the verify before approving).
5. **Dispatch the binary** with the chosen `--decision` (and `--edited-content` for an edit), then report the exit code: `0` done, `2` pending-id not found, `3` invalid arguments, other non-zero a failure to inspect.

## Notes

- This command does not compile governance. After approval, run `/edikt:gov:compile` to regenerate topic files with the promoted behavioral verify in scope.
- The pending file is removed only after the sidecar write succeeds (or on reject); a mid-approve failure leaves it in place so you can retry.
- The tier-1 wrapper never edits the sidecar directly — promotion goes through `bin/edikt sidecar approve` so `human_approved_at` and schema validation stay authoritative. All on-disk mutations are atomic.

## Related

- [`/edikt:gov:compile`](/commands/gov/compile) — brings the promoted behavioral verify into compiled governance.
- [`/edikt:sidecar:regenerate`](/commands/migrate#resolving-regressions) — regenerate sidecars from a migration manifest.
- [Sidecar architecture](/governance/sidecar) — sidecar shape and the verify lifecycle.
