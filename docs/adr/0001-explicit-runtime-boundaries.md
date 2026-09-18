# ADR 0001: explicit runtime boundaries

- Status: accepted
- Date: 2026-09-13

## Context

Kwatch monitors many Kubernetes resources, performs diagnosis, persists state,
and supports many notification providers. A single coordinator can make the
runtime work, but it makes ownership difficult to discover and causes small
changes to cross unrelated concerns.

## Decision

Keep orchestration in `internal/app` and `internal/controller`. Monitor
families produce observations. `internal/incident` owns incident lifecycle.
`internal/insight` performs read-only diagnosis. `internal/delivery` owns
transport policy, and `internal/alert/*` contains provider-specific payload
adapters. `internal/persistence` owns formats, migrations, and recovery.

Boundaries use small typed interfaces and explicit composition. Metadata
registries describe capabilities but never perform runtime service lookup. The
controller owns only narrow family queue/configuration contracts; monitor
packages own detection and observation behavior.

## Consequences

- A new monitor can be added without changing lifecycle or providers.
- A new provider can be added without changing Kubernetes controllers.
- Recovery and notification decisions have one owner.
- More wiring is visible in the composition root, which is intentional and
  easier to review than hidden registries.
- Transitional compatibility adapters were removed before the first stable
  release. Persisted-data migrations remain because they protect existing
  release-candidate installations.

## Alternatives rejected

- A universal monitor interface was rejected because monitor families have
  different inputs and lifecycles.
- Runtime Go plugins were rejected in favor of static, deterministic wiring.
- A second public documentation source was rejected; `kwatch.dev` remains
  canonical.
- Leader election is implemented separately because high availability needs an explicit
  ownership and deduplication design.
