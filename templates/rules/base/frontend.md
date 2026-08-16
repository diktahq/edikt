---
paths: "**/*.{ts,tsx,js,jsx,vue,svelte,css,scss}"
version: "0.1.0"
---
<!-- edikt:generated -->

# Frontend

Rules for building accessible, performant, maintainable user interfaces.

## Critical

- MUST handle all async states: loading (skeleton, not blank screen), error (message + retry), empty (helpful prompt, not blank space), and success. A blank screen for any state is a bug.
- MUST make every interactive element keyboard accessible: Tab, Enter, Escape, Arrow keys. If you can't complete the flow without a mouse, it's broken.
- NEVER use color as the only means of conveying information. Add text, icons, or patterns.

## Standards

- Components do ONE thing. If a component fetches data AND renders AND handles interactions, split it. Separate data-fetching (container) from presentation (pure) components.
- Local state first. Only lift state up when siblings need it. URL state for things users should bookmark (filters, pagination). Global state only for truly app-wide concerns (auth, theme, feature flags). Server state (React Query, SWR) for API data — not global store.
- Form inputs need associated `<label>` elements. Placeholders are not labels.
- Use semantic HTML (`<nav>`, `<main>`, `<article>`, `<button>`) over `<div>` with roles. Prefer native elements before reaching for ARIA.
- Use design tokens or CSS variables for colors, spacing, and typography. Never hardcode values.

## Practices

- Lazy load routes and heavy components. Don't load the settings page when the user is on the homepage.
- Images: use appropriate formats (WebP/AVIF), include width/height to prevent layout shift, lazy load below-the-fold.
- Memoize expensive computations only when profiling shows a bottleneck — don't premature-optimize.
- Check bundle imports: `import { debounce } from 'lodash'` pulls the entire library. Use `import debounce from 'lodash/debounce'` or a smaller alternative.
- Disable submit while a request is in flight. Show a loading state.

## Critical

- MUST handle all async states — no blank screens.
- MUST ensure keyboard accessibility for all interactive elements.
