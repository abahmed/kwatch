# ADR 0003: immutable provider generations

## Status

Accepted

## Decision

Delivery stores providers in immutable generations. Each generation owns its
provider lookup, deterministic order, fallback names, and worker channels.
Queued jobs retain the generation that accepted them, so fallback resolution
cannot change during reconfiguration.

The application owns delivery start and stop. Delivery providers implement the
context-aware provider contract and use shared transport for HTTP request
creation, status classification, retries, and redaction.

## Consequences

Provider additions and reconfiguration are explicit lifecycle operations. Old
workers are drained or cancelled before their channels are reclaimed, and
providers cannot accidentally send after shutdown.
