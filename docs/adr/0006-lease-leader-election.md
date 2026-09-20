# ADR 0006: Lease leader election and standby replicas

## Status

Accepted

## Decision

Kwatch uses a namespaced Kubernetes Lease as its single-writer coordination
point. Deployments run two replicas by default: one leader starts monitoring,
delivery, and mutable persistence; the other replica remains a standby that
serves health endpoints and participates in election. The same contract applies
to any configured replica count: one leader and `N-1` standbys.

The Lease is the authority. Standbys do not start informers, queues, monitor
sweeps, provider workers, or persistence savers. Leadership loss makes the
leader unready, cancels its active generation, fences persistence writes, and
exits so Kubernetes can restart it. A standby can then acquire the Lease and
restore persisted state before becoming ready.

The application does not claim exactly-once notification delivery. Persisted
state reduces duplicate notifications during takeover, but events and provider
responses can expire or be unavailable during a monitoring gap. Election also
does not protect against a shared Kubernetes API, network, node, or cluster
failure.

## Consequences

Scaling adds standby capacity rather than monitoring throughput. Topology spread
or anti-affinity should be used when node-failure protection matters. One replica
remains supported as a low-resource mode, but it has no Kwatch self-failover.
