---
paths: "**/*.{go,ts,tsx,js,jsx,py,rb,php,rs,java,kt,swift,c,cpp,cs}"
version: "0.1.0"
---
<!-- edikt:generated -->

# Code Quality

Rules for writing clean, maintainable, production-grade code.

## Critical

- NEVER use generic names: `utils`, `helpers`, `common`, `shared`, `misc`. A `utils.go` with 10 unrelated functions is a design failure — each function belongs in the module it serves.
- NEVER commit TODO/FIXME/HACK comments without a linked issue number.
- MUST keep functions under 50 lines. If a function grows beyond that, extract helpers — not because of the line count, but because the function is doing too much.

## Standards

- Prefer early returns over nested conditions. Max nesting depth: 3 levels. If you need 4 levels, flatten with guards.
- No circular dependencies between packages or modules.
- A package must not import from a sibling's internal details — only from its public interface.
- Business logic must not live in HTTP handlers, UI components, or database layers.
- HTTP concerns (status codes, headers, serialization) must not leak into business logic.
- NEVER call a function, method, or API without verifying it exists in the version of the dependency the project uses. If you cannot verify a function exists, state that explicitly rather than guessing the signature.
- NEVER submit placeholder implementations — `// TODO`, `pass`, `throw new Error("not implemented")`, or functions that return hardcoded values. Every function must contain real logic. If you cannot implement it, say so.
- MUST match the conventions of the existing codebase. Before creating a new function or type, read existing examples in the same project to understand error handling patterns, naming, parameter ordering, and file organization. Consistency with the codebase overrides language defaults.
- NEVER introduce an abstraction (interface, factory, registry, plugin system) unless there are at least two concrete implementations needed today — "we might need this later" is not a justification.
- NEVER share mutable state between concurrent execution paths (goroutines, threads, async tasks, Promise.all) without explicit synchronization — mutex, channel, or immutable data.
- MUST handle edge case inputs: empty collections (return early or handle explicitly), nil/null pointers (guard before dereferencing), zero values (check before division), boundary values (max int, empty string). The happy path is not the only path.

## Practices

- Add comments for: why a non-obvious approach was chosen, business rules not evident from code, and workarounds with links to issues. Not for restating what the code does.

## Critical

- NEVER use generic names: `utils`, `helpers`, `common`, `shared`, `misc`.
- MUST keep functions under 50 lines.
