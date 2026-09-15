# ADR 0004: compatibility and operating model

## Status

Accepted

## Decision

Persisted ConfigMap names, JSON keys, incident layouts, group-key serialization,
provider names, aliases, configuration fields, metrics, and audit reasons are
compatibility contracts. Internal refactors may change package structure, but
persisted changes require an explicit versioned migration, backup/recovery
path, and tests for old data.

Kwatch remains intentionally single-replica and does not use Lease-based
leader election. High availability requires a separate design covering
ownership, failover, deduplication, and persisted-state coordination.

## Consequences

The current deployment model remains operationally clear. Future HA work is
not hidden inside structural refactors or assumed to be safe by adding a
second replica.
