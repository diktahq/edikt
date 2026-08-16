---
paths: "**/*.rb"
version: "0.1.0"
---
<!-- edikt:generated -->

# Ruby on Rails

Rules for building Rails applications.

## Critical

- NEVER use `params.permit!` — it permits all parameters and bypasses strong parameter protection entirely. Always enumerate permitted params explicitly.
- NEVER use `raw` or `html_safe` on user-provided content — Rails escapes output by default, using these overrides re-opens XSS vulnerabilities.
- NEVER store plaintext passwords. Use `has_secure_password` or Devise. Never roll your own password hashing.

## Standards

- Eager load associations to avoid N+1 queries: `includes(:orders)`, `preload(:items)`. Never lazy-load in loops. Use the Bullet gem in development to detect N+1s automatically.
- Stick to RESTful actions: `index`, `show`, `new`, `create`, `edit`, `update`, `destroy`. If you need non-REST actions, extract a new resource.
- Define validations in models, not controllers. Use scopes for reusable query logic.
- Migrations MUST be reversible. Define both `up`/`down` or use `change` with reversible methods. Use `null: false` and defaults where appropriate. Add foreign key constraints.

## Practices

- Jobs MUST be idempotent. Set `retry_on` and `discard_on` for error handling. Use `deliver_later` for all emails — never `deliver_now` in a controller action.
- Use `credentials.yml.enc` for secrets. Never commit unencrypted secrets. Access via `Rails.application.credentials.key`.

## Critical

- NEVER use `params.permit!`.
- NEVER use `raw` or `html_safe` on user-provided content.
