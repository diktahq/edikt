---
type: guideline
id: guideline-099
title: "Guideline-099 — Go Error Wrapping"
status: active
date: 2026-05-03
---

# Guideline-099 — Go Error Wrapping

## Convention

Use `fmt.Errorf` with `%w` to wrap errors when returning them from package-internal functions.
Include the function name in the error message to aid debugging.
Return `nil` only when the operation succeeds completely with no partial state.

## Rationale

Consistent error wrapping enables `errors.Is` and `errors.As` to traverse the error chain,
which makes structured error handling at call sites possible without string matching.
