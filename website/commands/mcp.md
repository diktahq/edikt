# /edikt:mcp

Connect Claude to project management tools via MCP (Model Context Protocol).

## Usage

```bash
/edikt:mcp
/edikt:mcp add linear|github|jira
/edikt:mcp remove {server}
/edikt:mcp status
```

## What is MCP?

MCP (Model Context Protocol) is Claude Code's integration system. It lets Claude natively access external tools — reading tickets, creating issues, browsing PRs — without copy-pasting context.

edikt generates `.mcp.json` in your project root with server configurations. Commit it to git — your team inherits the server config. Each person adds their own API keys locally.

## Supported servers

### Linear

```bash
/edikt:mcp add linear
```

Requires: `LINEAR_API_KEY` in your shell environment
Get one at: https://linear.app/settings/api

```bash
# Add to ~/.zshrc:
export LINEAR_API_KEY="lin_api_..."
```

### GitHub

```
/edikt:mcp add github
```

Requires: `GITHUB_TOKEN` in your shell environment
Get one at: https://github.com/settings/tokens (needs `repo` scope)

```bash
# Add to ~/.zshrc:
export GITHUB_TOKEN="ghp_..."
```

### Jira

```bash
/edikt:mcp add jira
```

Requires three env vars: `JIRA_URL`, `JIRA_USERNAME`, `JIRA_API_TOKEN`
Get a token at: https://id.atlassian.com/manage-profile/security/api-tokens

```bash
# Add to ~/.zshrc:
export JIRA_URL="https://yourorg.atlassian.net"
export JIRA_USERNAME="you@yourorg.com"
export JIRA_API_TOKEN="your-token"
```

## Status check

```
/edikt:mcp

MCP Servers:

  linear    ✅ configured, LINEAR_API_KEY set
  github    ⚠️  configured, GITHUB_TOKEN not set
                Set: export GITHUB_TOKEN="ghp_..."
                Get one: https://github.com/settings/tokens

Not configured:
  jira      /edikt:mcp add jira

.mcp.json is committed to git — team inherits server configs.
Each member needs their own API keys in their local environment.
```

## What gets generated

Running `/edikt:mcp add linear` creates or updates `.mcp.json`:

```json
{
  "mcpServers": {
    "linear": {
      "type": "http",
      "url": "https://mcp.linear.app/sse",
      "authorization_token": "${LINEAR_API_KEY}"
    }
  }
}
```

Environment variables are expanded from your shell at runtime — the actual key is never in the file.

## Team setup

`.mcp.json` is safe to commit — it contains server configs, not secrets. Each team member sets their own API keys in their local environment:

1. Commit `.mcp.json` (you)
2. Each teammate: add env vars to their `~/.zshrc`
3. Verify: `/edikt:team setup` shows ✅ for each configured key

## Natural language triggers

- "setup Linear" → `/edikt:mcp add linear`
- "add GitHub integration" → `/edikt:mcp add github`
- "what MCP servers are configured?" → `/edikt:mcp`
