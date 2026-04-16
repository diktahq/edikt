# Installing edikt via Homebrew

edikt ships a Homebrew formula through the `diktahq/tap` tap. The formula
installs the launcher binary — a small POSIX shell script that manages edikt
payload versions on your machine.

## Install

```bash
brew install diktahq/tap/edikt
```

Then fetch the latest payload (templates, commands, hooks):

```bash
edikt install
```

## Two-tier update model

edikt has two independently versioned components, and they update separately.

### Launcher updates — `brew upgrade edikt`

The launcher (`bin/edikt`) is versioned by the Homebrew formula. Homebrew
controls when your launcher binary updates. Run `brew upgrade edikt` to get
a new launcher version.

The launcher is intentionally small and changes infrequently. Most edikt
improvements ship in the payload, not the launcher.

### Payload updates — `edikt upgrade`

The payload (templates, commands, hooks, agents) lives in `~/.edikt/versions/`
and is managed by the launcher. Running `edikt upgrade` fetches the latest
payload release and activates it — independent of Homebrew.

```bash
edikt upgrade          # fetch + activate latest payload
edikt upgrade --pin v0.5.0   # pin to a specific version
edikt rollback         # revert to the previous payload
```

### Why the split?

The launcher needs root-level `PATH` placement (handled by Homebrew). The
payload is user-space content (markdown files, shell scripts) that can be
updated without touching system paths. Keeping them separate means:

- Security patches to the launcher ship through Homebrew's signing infrastructure
- Payload updates are instant — no `sudo`, no tap PR, no formula review

## Verify your install

```bash
edikt version   # shows launcher version + active payload version
edikt doctor    # checks symlinks, manifest integrity, PATH placement
```
