# Contributing to edikt

edikt is source-available under the [Elastic License 2.0](LICENSE) — not OSI open source. You can
read the code, run it, and fork it under the licence terms; see `LICENSE` for what that does and
doesn't permit.

## The honest state of contribution right now

**Issues and discussion are welcome.** Bug reports, feature requests, and questions about behavior
are genuinely useful and read.

**Pull requests are not currently accepted.** Not a policy statement about wanting contributions in
principle — a practical one: edikt's test suite, architecture decision records, and governance corpus
that the tool's own behavior is compiled from live in a private companion repository. There is
currently no way for an external contributor to run the checks a change would need to pass before it
could be merged responsibly. Opening a PR here today means doing real work that cannot be validated
against the same bar the maintainer holds internal changes to — which is a worse experience than being
told upfront.

If that changes (a public-facing subset of the test suite, a contribution path that doesn't require
the private corpus), this file will say so and stop saying the opposite.

## What's actually in this repo

- `install.sh` — the bootstrap installer. Tag-pinned release URLs only (see `LICENSE` and the release
  process — no `main`-tracking or `latest`-alias install commands are ever correct here).
- `templates/` — hook scripts, agent templates, and rule-pack templates the installer places into a
  project.
- `commands/` — the slash-command surface (`commands/sdlc/spec.md` → `/edikt:sdlc:spec`, etc.).
- `website/` — this documentation site.

`tools/` carries the Go source behind the `edikt` binary and is source-available under the same
[Elastic License 2.0](LICENSE) as the rest of this repo. The test suite and the architecture-decision
corpus that drives what the tool enforces are maintained privately and are not part of this
repository — which is also why an external PR against `tools/` can't be validated against the same
bar an internal change is, per the posture stated above.

## Reporting a bug

Open an issue with: what you ran, what you expected, what happened instead, and your edikt version
(`edikt version` or the `VERSION` file in your install). If it's a hook or governance-compile issue,
include the relevant command output — hooks emit structured JSON on stdout/stderr and that's usually
enough to diagnose without needing your project's private content.

## Security issues

Do not open a public issue for a security vulnerability. [Contact information / security policy link
— fill in before publishing.]

## License

By participating (issues, discussion), you agree your contributions are subject to the project's
[Elastic License 2.0](LICENSE) terms.
