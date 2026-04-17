import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'

export default withMermaid(defineConfig({
  title: 'edikt',
  description: 'The governance layer for agentic engineering — govern the full cycle from requirements through execution to verification.',
  base: '/',
  cleanUrls: true,

  rewrites: {
    'commands/compile': 'commands/gov/compile',
    'commands/review-governance': 'commands/gov/review',
    'commands/rules-update': 'commands/gov/rules-update',
    'commands/sync': 'commands/gov/sync',
    'commands/prd': 'commands/sdlc/prd',
    'commands/spec': 'commands/sdlc/spec',
    'commands/spec-artifacts': 'commands/sdlc/artifacts',
    'commands/plan': 'commands/sdlc/plan',
    'commands/review': 'commands/sdlc/review',
    'commands/drift': 'commands/sdlc/drift',
    'commands/audit': 'commands/sdlc/audit',
    'commands/docs': 'commands/docs/review',
    'commands/intake': 'commands/docs/intake',
    'commands/adr': 'commands/adr/new',
    'commands/invariant': 'commands/invariant/new',
  },

  head: [
    ['link', { rel: 'icon', href: '/favicon.ico' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    ['link', { href: 'https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600;700&family=IBM+Plex+Sans:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap', rel: 'stylesheet' }],
    ['meta', { name: 'robots', content: 'index, follow' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'edikt' }],
    ['meta', { property: 'og:image', content: '/og-image.png' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
  ],

  sitemap: {
    hostname: 'https://edikt.dev'
  },

  themeConfig: {
    logo: { light: '/logo.svg', dark: '/logo-dark.svg' },
    siteTitle: false,

    nav: [
      { text: 'Guides', link: '/guides/solo-engineer' },
      { text: 'How It Works', link: '/governance/chain' },
      { text: 'Reference', link: '/commands/' },
    ],

    sidebar: [
      {
        text: 'Get Started',
        items: [
          { text: 'What is edikt?', link: '/what-is-edikt' },
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'Philosophy', link: '/philosophy' },
        ],
      },
      {
        text: 'For You',
        items: [
          { text: 'Solo Engineers', link: '/guides/solo-engineer' },
          { text: 'Teams', link: '/guides/teams' },
          { text: 'Multi-Project', link: '/guides/multi-project' },
        ],
      },
      {
        text: 'How It Works',
        items: [
          { text: 'Governance Chain', link: '/governance/chain' },
          { text: 'How Governance Compiles', link: '/governance/compile' },
          { text: 'Sentinel Blocks', link: '/governance/sentinels' },
          { text: 'Extensibility', link: '/governance/extensibility' },
          { text: 'Agents', link: '/agents' },
          { text: 'Quality Gates', link: '/governance/gates' },
          { text: 'Evaluator', link: '/governance/evaluator' },
          { text: 'Drift Detection', link: '/governance/drift' },
          { text: 'Configurable Features', link: '/governance/features' },
        ],
      },
      {
        text: 'Architecture Decisions',
        items: [
          { text: 'What they are', link: '/governance/architecture-decisions' },
          { text: 'Writing good ADRs', link: '/governance/writing-adrs' },
        ],
      },
      {
        text: 'Invariant Records',
        items: [
          { text: 'What they are', link: '/governance/invariant-records' },
          { text: 'Writing good invariants', link: '/governance/writing-invariants' },
          { text: 'Example: tenant isolation', link: '/governance/canonical-invariants/tenant-isolation' },
          { text: 'Example: money precision', link: '/governance/canonical-invariants/money-precision' },
        ],
      },
      {
        text: 'Guidelines',
        items: [
          { text: 'What they are', link: '/governance/guidelines' },
        ],
      },
      {
        text: 'Project Setup',
        items: [
          { text: 'Greenfield Projects', link: '/guides/greenfield' },
          { text: 'Existing Projects', link: '/guides/brownfield' },
          { text: 'Monorepos', link: '/guides/monorepo' },
          { text: 'Natural Language', link: '/natural-language' },
          { text: 'Daily Workflow', link: '/guides/daily-workflow' },
          { text: 'Specialist Agents', link: '/guides/specialist-agents' },
          { text: 'Security', link: '/guides/security' },
          { text: 'CI/CD Governance', link: '/guides/ci' },
          { text: 'Keeping Up to Date', link: '/guides/upgrading' },
          { text: 'Upgrading to v0.5.0', link: '/guides/v0.5.0-upgrade' },
        ],
      },
      {
        text: 'Experiments',
        items: [
          { text: 'EXP-001: Rule Compliance', link: '/experiments/exp-001-rule-compliance' },
          { text: 'EXP-002: Extended Compliance', link: '/experiments/exp-002-extended-compliance' },
        ],
      },
      {
        text: 'Commands',
        link: '/commands/',
        items: [
          {
            text: 'Governance',
            items: [
              { text: 'gov:compile', link: '/commands/gov/compile' },
              { text: 'gov:review', link: '/commands/gov/review' },
              { text: 'gov:score', link: '/commands/gov/score' },
              { text: 'gov:rules-update', link: '/commands/gov/rules-update' },
              { text: 'gov:sync', link: '/commands/gov/sync' },
            ],
          },
          {
            text: 'SDLC Chain',
            items: [
              { text: 'sdlc:prd', link: '/commands/sdlc/prd' },
              { text: 'sdlc:spec', link: '/commands/sdlc/spec' },
              { text: 'sdlc:artifacts', link: '/commands/sdlc/artifacts' },
              { text: 'sdlc:plan', link: '/commands/sdlc/plan' },
              { text: 'sdlc:review', link: '/commands/sdlc/review' },
              { text: 'sdlc:drift', link: '/commands/sdlc/drift' },
              { text: 'sdlc:audit', link: '/commands/sdlc/audit' },
            ],
          },
          {
            text: 'Decisions',
            items: [
              { text: 'adr:new', link: '/commands/adr/new' },
              { text: 'adr:compile', link: '/commands/adr/compile' },
              { text: 'adr:review', link: '/commands/adr/review' },
              { text: 'invariant:new', link: '/commands/invariant/new' },
              { text: 'invariant:compile', link: '/commands/invariant/compile' },
              { text: 'invariant:review', link: '/commands/invariant/review' },
              { text: 'guideline:new', link: '/commands/guideline/new' },
              { text: 'guideline:review', link: '/commands/guideline/review' },
            ],
          },
          {
            text: 'Docs',
            items: [
              { text: 'docs:review', link: '/commands/docs/review' },
              { text: 'docs:intake', link: '/commands/docs/intake' },
            ],
          },
          {
            text: 'Daily Use',
            items: [
              { text: 'capture', link: '/commands/capture' },
              { text: 'context', link: '/commands/context' },
              { text: 'status', link: '/commands/status' },
              { text: 'brainstorm', link: '/commands/brainstorm' },
              { text: 'session', link: '/commands/session' },
              { text: 'doctor', link: '/commands/doctor' },
              { text: 'init', link: '/commands/init' },
              { text: 'upgrade', link: '/commands/upgrade' },
              { text: 'agents', link: '/commands/agents' },
              { text: 'mcp', link: '/commands/mcp' },
              { text: 'config', link: '/commands/config' },
            ],
          },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Rule Packs', link: '/rules/' },
          { text: 'FAQ', link: '/faq' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/diktahq/edikt' },
    ],

    footer: {
      message: 'Released under the Elastic License 2.0. Free to use, not for resale.',
    },

    search: {
      provider: 'local',
    },
  },
}))
