# ADR 0004: compatibility and operating model

## Status

Superseded by ADR 0010 (2026-09-28).

No longer applies: the persisted ConfigMap names, incident layouts and
group-key serialization (replaced by the bbolt store, no migration needed
before a stable release), and the two-replica default with standby Pods
(now one replica, `Recreate`, Lease as a write lock). Still valid: provider
names, configuration fields and metrics are compatibility contracts, and
in-cluster election does not protect against total cluster failure.

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
