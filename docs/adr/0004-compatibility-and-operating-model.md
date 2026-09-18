# ADR 0004: compatibility and operating model

## Status

Accepted

## Decision

Persisted ConfigMap names, JSON keys, incident layouts, group-key serialization,
provider names, aliases, configuration fields, metrics, and audit reasons are
compatibility contracts. Internal refactors may change package structure, but
persisted changes require an explicit versioned migration, backup/recovery
path, and tests for old data.

Kwatch uses two replicas by default with Lease-based leader election. Exactly
one replica owns monitoring, delivery, and mutable persistence; other replicas
are standby. Election and persisted-state recovery provide process and Pod
failover without requiring an external monitor. A one-replica override remains
supported but has no self-failover. Total cluster, API, node, and network
failures remain outside the protection of in-cluster election.

## Consequences

The deployment model remains single-writer and operationally clear. Adding
replicas is safe because standby Pods do not start active monitoring or
delivery components.
