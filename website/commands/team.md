# /edikt:team

Onboard team members and show what's shared across the team.

## Usage

```
/edikt:team
/edikt:team setup
/edikt:team init "Team Name"
```

## No argument — team overview

Shows what's committed to git and what each member needs locally.

```
/edikt:team

Team: Orders API

Shared config (committed to git):
  .edikt/config.yaml     — project config, rules, stack
  .claude/rules/        — 6 rule packs
  .claude/agents/       — 9 specialist agents
  .mcp.json             — linear, github (keys not committed)
  .github/              — PR template

Each member needs these environment variables:
  LINEAR_API_KEY    — https://linear.app/settings/api
  GITHUB_TOKEN      — https://github.com/settings/tokens

Onboarding a new member:
  git clone ... && cd project
  /edikt:team setup    — validates their local environment
```

## `setup` — validate member environment

Run this when joining a project or after the team adds new MCP servers.

```
/edikt:team setup

edikt Team Setup — Orders API

  ✅ Git configured (Jane Smith <jane@example.com>)
  ✅ Claude Code v2.1.70
  ✅ edikt config found
  ✅ LINEAR_API_KEY set
  ⚠️  GITHUB_TOKEN not set
      Get a token (repo scope): https://github.com/settings/tokens
      Add to ~/.zshrc: export GITHUB_TOKEN="ghp_..."

1 item needs attention. Fix it, then run /edikt:team setup again.

Once all green: run /edikt:context to load project context.
```

Checks:
- Git name and email configured
- Claude Code installed
- `.edikt/config.yaml` exists (confirms edikt-initialized repo)
- All env vars required by `.mcp.json` are set

## `init {name}` — add team config

```
/edikt:team init "Orders API Team"
```

Adds a `team:` block to `.edikt/config.yaml`:

```yaml
team:
  name: "Orders API Team"
```

Commit the updated config so the team name appears in `/edikt:team` output for everyone.

## Onboarding workflow

For the team lead (once):
```bash
# 1. Run init
/edikt:init

# 2. Configure MCP servers
/edikt:mcp add linear
/edikt:mcp add github

# 3. Commit everything
git add .claude/ .edikt/ .mcp.json docs/
git commit -m "chore: initialize edikt"
git push
```

For each new team member:
```bash
# 1. Clone and open in Claude Code
git clone ... && cd project

# 2. Validate environment
/edikt:team setup

# 3. Fix any missing env vars (shown in setup output)

# 4. Load context
/edikt:context
```

That's it — they're productive from the first session.
