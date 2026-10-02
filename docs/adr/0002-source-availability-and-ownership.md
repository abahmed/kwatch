# ADR 0002: source availability and monitor ownership

## Status

Superseded by ADR 0010 (2026-09-28).

The `monitor/*` families, controller-owned source bundles, `networkgraph` and
`pvc` no longer exist. The part that still holds: a missing or unavailable
source means the capability is unavailable, detection skips it, and no
synthetic problem is created or resolved. Sources now live under
`internal/inventory` and report availability through health.

## Decision

Monitor families receive typed, controller-owned source bundles. A missing
lister means that capability is unavailable: the family skips detection and
does not create or resolve a synthetic incident. The unavailable capability is
reported through diagnostics or health when the integration exposes status.

Resource policy stays with the family that owns the resource. Cluster
resources belong to `monitor/cluster`; admission and TLS belong to
`monitor/security`; permission auditing belongs to `internal/rbac`; network
relationships belong to `networkgraph`; PVC usage belongs to `pvc`.

Source configuration occurs before processing starts. Later configuration is
ignored or rejected rather than changing a running runtime.

## Consequences

Controllers remain responsible for informer synchronization and queue
delivery, while detection remains deterministic and independently testable.
Optional Kubernetes APIs can degrade health without generating false outage
incidents.
