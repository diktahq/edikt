---
paths: "**/*.py"
version: "0.1.0"
---
<!-- edikt:generated -->

# Python

Rules for writing clean, idiomatic Python code.

## Critical

- NEVER use bare `except:` or `except Exception:` without re-raising — it swallows every exception including KeyboardInterrupt and SystemExit. Catch the specific exception type you expect.
- NEVER use `from module import *` — it pollutes the namespace and makes the source of names impossible to trace.
- MUST add type hints to all function signatures (parameters and return types). Run mypy or pyright in CI — hints without a checker are documentation, not safety.

## Standards

- Catch specific exceptions, not broad ones. If you need to catch broad exceptions (e.g., at a top-level handler), log and re-raise.
- Use context managers (`with`) for all resource management: files, connections, locks. Don't manually call `.close()`.
- Use `dataclasses` or Pydantic `BaseModel` for structured data — not plain dicts. Use `frozen=True` on dataclasses for immutable value objects. Use Pydantic for data from external sources.
- Follow PEP 8: `snake_case` for functions and variables, `PascalCase` for classes, `UPPER_SNAKE` for constants. Boolean names use `is_`, `has_`, `can_` prefixes.
- Import order: standard library, third-party, local — separated by blank lines. Use absolute imports. Never use relative imports for top-level packages.
- Use `asyncio` for I/O-bound concurrency. NEVER call blocking I/O inside an async function — use `asyncio.to_thread()`. Use `asyncio.gather()` or `asyncio.TaskGroup()` (3.11+) for concurrent tasks.

## Practices

- Use `from __future__ import annotations` for forward references and modern annotation syntax on Python 3.9 and earlier.
- Use `pyproject.toml` for project configuration — not `setup.py` or `setup.cfg`.
- Keep `__init__.py` minimal. It defines the public API — import what should be public, leave internals private. Don't put logic in it.
- One concern per module. Don't put models, views, and utilities in the same file.
- Use `pytest.mark.parametrize` for table-driven tests. Use `pytest.fixture` for shared setup.
- Consider `typing.Protocol` over abstract base classes for structural subtyping — it removes the inheritance requirement for callers.

## Critical

- NEVER use bare `except:` — always catch the specific exception type.
- MUST type-hint all function signatures and run a type checker in CI.
