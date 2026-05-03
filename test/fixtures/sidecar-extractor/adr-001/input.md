# ADR-001 — Two-Stage Voice Pipeline: Deepgram for Transcription, Claude for Extraction

**Status:** Accepted
**Date:** 2026-03-28
**Deciders:** Daniel Gomes

## Context

The AI DDD Companion needs to capture live conversations between tech leads and domain experts, identify who is speaking, transcribe in real-time, and extract structured DDD artifacts (bounded contexts, relationships, UL terms) from the conversation.

No single product handles all three requirements (transcription, speaker diarization, semantic extraction) well enough to rely on alone. The key constraint is speaker diarization — knowing who said what is critical because the domain expert's words carry different weight than the tech lead's.

## Decision Drivers

- Real-time speaker diarization is non-negotiable — the product is built around understanding who said what
- Must have an official Go SDK for backend integration
- Cost must be reasonable for sessions lasting 1-2 hours
- Latency must support live canvas updates (sub-30s processing)
- Structured JSON output for DDD artifact extraction

## Considered Options

### A. Gemini 3.1 Flash Live (single-stage: audio → structured output)
- Pros: lowest latency, cheapest ($0.15-0.20/hr), native audio understanding (tone, emphasis), structured JSON output
- Cons: **no reliable speaker diarization** (prompt-based, not native), 15-minute session limit, preview-only (not GA)

### B. Deepgram Nova-3 (transcription) + Claude (extraction) — two-stage
- Pros: only mature real-time streaming diarization, official Go SDK (deepgram-go-sdk v2), custom vocabulary, $0.46/hr + ~$0.10-0.20/hr AI = ~$0.70/hr total
- Cons: two services to manage, lose audio-level context (tone, emphasis) in text stage

### C. OpenAI gpt-4o-transcribe-diarize (transcription) + Claude (extraction)
- Pros: native diarization with speaker name mapping, official Go SDK
- Cons: **batch-only** — no streaming, must buffer 15-30s chunks, speaker continuity across chunks unreliable

### D. AssemblyAI with LeMUR (single platform)
- Pros: closest to single-platform solution, real-time diarization, LLM-over-transcript
- Cons: **Go SDK discontinued** (April 2025), extraction quality less controllable than custom prompts

### E. Whisper + pyannote (self-hosted)
- Pros: full control, no API costs
- Cons: significant infrastructure to run, latency, no real-time streaming diarization without custom engineering

## Decision

**Two-stage pipeline: Deepgram Nova-3 for transcription + diarization, Claude for DDD extraction.**

Stage 1 (Deepgram): WebSocket streaming → diarized transcript segments with speaker labels, timestamps. Custom vocabulary from existing glossary terms.

Stage 2 (Claude): takes diarized transcript chunks every 20-30s → structured JSON output via `output_format` with DDD artifact schema. Prompt caching for cost reduction.

Fallback: OpenAI gpt-4o-transcribe-diarize for transcription (batch mode), any capable LLM for extraction.

## Consequences

### Good
- Reliable speaker attribution from day one — product-critical
- Provider pattern (internal/stt/provider.go) allows swapping STT providers without architectural changes
- When Gemini adds native diarization, it can replace both stages
- Custom vocabulary feeds glossary terms back into transcription accuracy

### Bad
- Two external service dependencies for voice features
- Lose audio-level context (tone, emphasis) that Gemini could provide
- Higher total cost ($0.70/hr) vs Gemini alone ($0.20/hr)

### Neutral
- Chat-based discovery works without voice pipeline — voice is additive, not foundational

## Confirmation

- Integration tests with recorded multi-speaker audio verify diarization accuracy
- Provider interface allows switching STT without touching business logic

[edikt:directives:start]: #
source_hash: 6dfdc9cdb699d838e4d93a0c30dadf9f06801fdce21a376dece75fc738be8a2b
directives_hash: a5dc6047b62804ddf3f41d2fe6ea76555a172c7692a0a90670896bc83f9811dd
compiler_version: "0.4.3"
paths:
  - internal/stt/**/*.go
  - internal/ai/**/*.go
  - internal/voice/**/*.go
scope:
  - planning
  - design
  - review
directives:
  - Use the provider pattern (internal/stt/provider.go) to swap STT providers without architectural changes (ref: ADR-001)
  - Always configure Deepgram with custom vocabulary from existing glossary terms (ref: ADR-001)
  - Process diarized transcript chunks through Claude every 20-30s using the output_format parameter with DDD artifact schema (ref: ADR-001)
  - NEVER use a single-stage pipeline that lacks native speaker diarization for the voice feature (ref: ADR-001)
reminders:
  - "Before adding a new STT provider → verify it implements internal/stt/provider.go interface (ref: ADR-001)"
  - "Before changing voice pipeline stages → confirm native speaker diarization is preserved (ref: ADR-001)"
verification:
  - "[ ] STT provider is accessed only through internal/stt/provider.go interface (ref: ADR-001)"
  - "[ ] Deepgram is configured with custom vocabulary from glossary terms (ref: ADR-001)"
  - "[ ] Claude extraction uses output_format with DDD artifact schema and 20-30s chunk intervals (ref: ADR-001)"
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #
