# Contributing to edikt

Contributions are welcome: bug reports, ideas, fixes to the documentation, and code.

edikt is source-available under the [Elastic License 2.0](LICENSE), which is not an OSI
open-source licence. You can read the code, run it, change it and fork it within the licence's
terms; `LICENSE` says what those are.

## Ways to contribute

- **Report a bug.** Open an issue. "Reporting a bug" below says what helps.
- **Propose a change or ask a question.** Open an issue. For anything large, do this before
  writing code, so you learn early whether it fits.
- **Send a pull request.** The next section says how it lands, because it does not land with
  the merge button.

## How a pull request lands

This repository is published from a private development repository. The full test suite lives
there, together with the architecture decisions that edikt's own governance is compiled from.
`main` here is replaced from that repository at each release, so a pull request cannot be merged
directly. This is what happens instead:

1. You open the pull request here.
2. A maintainer reviews it. If it is going in, the maintainer applies your change in the
   development repository, in a commit authored in your name, and runs the full suite there.
3. Your change reaches this repository with the next release. The changelog entry for that
   release credits you.
4. The pull request is closed with a link to the release that carries it.

If the full suite finds a problem your change could not have shown you, the maintainer says so
on the pull request and works it out with you.

## Running the checks

Tool versions are pinned in `mise.toml`:

```bash
mise install        # Go, lefthook, shellcheck, staticcheck, gosec, bun
lefthook install    # arms the hooks in lefthook.yml
```

The hooks run `gofmt` and `shellcheck` on the files you stage, and `go vet`, `staticcheck` and
`gosec` on push. You can run the last three yourself with `scripts/go-lint.sh all`; they pass on
a clean checkout of a release.

The Go tests are a different matter, and you should know before you run them:

```bash
cd tools/edikt && go test ./...
```

As of v0.9.1, 43 packages pass in this repository and nine fail, because their tests read
fixtures that are kept in the development repository: `cmd`, `cmd/gov`, `internal/globmatch`,
`internal/migrate`, `internal/pathsproposal`, `internal/phasea`, `internal/reextract`,
`internal/sidecar` and `internal/versioncmp`. A failure in one of those nine on an untouched
checkout is not caused by your change. Run the tests of the package you changed, and say in the
pull request what you ran.

Commands (`commands/`) and templates (`templates/`) are markdown, YAML and shell. They have no
build step.

To build the documentation site: `cd website && bun install && bun run build`.

## What is in this repository

- `install.sh`: the installer. Install commands always name a release tag, never `main` and
  never a "latest" alias.
- `commands/`: the slash commands (`commands/sdlc/spec.md` is `/edikt:sdlc:spec`).
- `templates/`: the agents, rule packs, hooks and schemas the installer places in a project.
- `tools/edikt/`: the Go source of the `edikt` binary.
- `website/`: the documentation site.

## Reporting a bug

Open an issue with what you ran, what you expected, what happened instead, and your edikt
version (`edikt version`). For a hook or a governance-compile problem, include the command's
output; it is usually enough to diagnose the problem without your project's content.

## Security issues

Do not open a public issue for a vulnerability. Report it privately from this repository's
**Security** tab, with **Report a vulnerability**.

## Licence

By contributing (issues, discussion, pull requests), you agree your contributions are subject to
the project's [Elastic License 2.0](LICENSE) terms.
