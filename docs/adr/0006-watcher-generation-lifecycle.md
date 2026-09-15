# ADR 0006: Generation-scoped optional watchers

## Status

Accepted

## Decision

Dynamic watchers expose an immutable generation handle for each successful
start. Cache synchronization and stop operations use that handle, so a stale
generation cannot observe or clear a replacement watcher.

Domain packages keep their own policy and use `dynamicwatch` only for informer
construction, discovery, synchronization, and lifecycle mechanics. Replacing
a watcher stops the previous generation first.

## Consequences

Optional API absence is observable as degraded status, while missing optional
APIs do not create synthetic incidents or fail readiness. Tests can exercise
replacement and stale-generation behavior without sleeps.
