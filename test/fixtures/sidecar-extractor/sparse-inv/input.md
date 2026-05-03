---
type: invariant
id: INV-099
title: "INV-099 — No Debug Logging in Production"
status: active
date: 2026-05-03
---

# INV-099 — No Debug Logging in Production

## Statement

The production logger MUST be configured at `info` level or above; debug-level logging MUST NOT be enabled in production deployments.
