# Contributor architecture guide

This is a short source-tree guide for contributors. Published tutorials,
reference pages, and operational runbooks live at
[kwatch.dev/docs](https://kwatch.dev/docs).

## Find the right package

Start with the package that owns the behavior you are changing:

| Task | Package |
| --- | --- |
| Kubernetes informer, queue, or cache sync | `internal/controller` |
| Controller-owned family wiring | `internal/controller/runtime.go` and `contracts_*.go` |
| Pod or workload detection | `internal/monitor/pod` or `internal/monitor/workload` |
| Node, network, security, or cluster detection | matching `internal/monitor/*` family |
| Kubernetes identity and owner lookup | `internal/observe` |
| Suppression matching | `internal/filter` |
| Incident identity, recovery, or grouping | `internal/incident` |
| Cause, impact, or recent-change analysis | `internal/insight` |
| Retry, fallback, pacing, or HTTP transport | `internal/delivery` |
| Provider payload mapping | `internal/alert/<provider>` |
| Persisted state or migration | `internal/persistence` |
| Optional dynamic informer lifecycle | `internal/k8s/dynamicwatch` |
| Composition and shared clients | `internal/app` |

After YAML and CRD overlays are applied, `config.CompileRuntimeConfig` creates
the immutable derived snapshot used by composition. Keep user-facing fields on
`config.Config`; put normalized namespaces, provider names, compiled provider
routes/retry policy, suppression indexes, and effective intervals in the
runtime snapshot. Application composition passes that snapshot to
`delivery.Manager.InitRuntime`.

The normal flow is:

```text
controller → monitor family → observation/filter → incident → insight
  → delivery transport → provider adapter
```

The arrow is also a dependency rule. A monitor does not call a provider, a
provider does not read Kubernetes, and persistence does not make incident or
diagnosis decisions.

## Read the application flow

Start at `internal/app/run.go`. The composition is intentionally split by
phase:

- `bootstrap.go`: clients, startup state, health, providers, and telemetry.
- `runtime_build.go`: domain construction and top-level assembly.
- `runtime_controller.go`: lister, restore, graph, and readiness wiring.
- `runtime_optional.go`: optional monitor run functions.
- `runtime_deps.go`: the owned runtime dependency bundle passed to serving.

For RCA, start at `internal/insight/engine.go`, then follow the semantic
files: `cause.go`, `root_cause.go`, `cause_evidence.go`, `confidence.go`,
`impact.go`, `changes.go`, and `next_steps.go`. These files share one package
and engine, but each owns one analysis concern.

Supporting monitors follow the same rule. `internal/probe` separates target
configuration, lifecycle dispatch, graph linking, protocol probes, automatic
Service discovery, and failure-state transitions. `internal/metricsapi`
separates construction, lifecycle, API collection, and threshold policy.
`internal/statuswatch` separates construction, configuration compilation,
informer lifecycle, static processing, CRD handling, and admission handling.
`internal/resource` separates lifecycle, filesystem signals, and node
overcommit policy. When adding code to these packages, place it beside the
responsibility it changes instead of growing the package's coordinator file.

## Add a monitor

1. Choose the existing family that owns the Kubernetes resource.
2. Put pure detection in a small function that accepts the object, config, and
   injected time where needed.
3. Add a family runtime with typed sources and a
   `monitor.ObservationSink` or `monitor.ReconciliationSink`.
4. Wire the runtime in `internal/app`. The controller assembles synchronized
   listers and passes one typed source bundle to the family runtime through
   the narrow capability in `internal/controller/runtime.go` and the family
   contract in `internal/controller/contracts_*.go`. Workload runtimes use
   `workload.SourceConfig` and `workload.Sources`; cluster runtimes use
   `cluster.SourceConfig` and `cluster.Sources`; network and security runtimes
use their corresponding `SourceConfig` and `Sources` types.
TLS, RBAC, control-plane, probe, metrics, kubelet metrics, PVC, and watcher
integrations follow the same one-time `ConfigureSources` rule. Production code
uses only these canonical source bundles; transitional setter shims have been
removed before the first stable release.
5. Keep informer, queue, tombstone, and cache-sync ownership in the
   controller.
6. Add semantic unit tests and controller integration tests.
7. Add catalog metadata and the corresponding website documentation.

Do not add a method to a universal monitor interface. Put new detection in the
cohesive monitor family that owns the resource, with explicit dependencies.

Health lifecycle is application-owned: composition calls `HealthServer.Open`,
the supervisor runs `HealthServer.Serve`, and shutdown calls `Stop`.

Health responses expose bounded component states and reason codes. Detailed
errors belong in redacted logs, not public diagnostics. A missing lister is an
unavailable capability: detection skips it and never creates a synthetic
incident.

## Add a provider

Provider adapters validate settings, render payloads, and call the shared
`delivery/transport` boundary. HTTP adapters use the application-owned client
through that package. They do not classify HTTP status,
retry, rate-limit, construct Kubernetes clients, or log complete payloads.

Keep a large provider readable with semantic files such as `config.go`,
`payload.go`, `incident.go`, and `verify.go`. Do not create numbered or
history-based files. Add payload, error, cancellation, redaction, and size
tests, then update the generated provider catalog and website reference.

## Change persistence

Use the narrow store interface required by the consumer. Keep persisted DTOs
flat and preserve existing ConfigMap names and JSON keys. Any format change
requires a schema version, migration or documented reset path, backup/recovery
behavior, old-format fixture, round-trip test, and release note.

## Test and verify

Name tests after behavior and split large test files by responsibility, for
example `queue_retry_test.go` or `migration_test.go`. Shared fixtures belong
in `fixtures_test.go` or `test_helpers_test.go`.

Use fake clocks and fake clients instead of sleeps. Before handoff run:

```sh
make verify
go test -race -p 1 ./...
```

For small edits, use the cheaper package-scoped command and expand the package
list only when an interface changes:

```sh
make verify-fast PKGS="./internal/foo"
make verify-focused PKGS="./internal/foo ./internal/bar"
```

`verify-fast` skips repository-wide checks. Run `verify-focused` once after a
workstream, then reserve the full gate and complete race suite for handoff.

If a change affects a public setting, metric, provider, persistence format,
RBAC rule, or extension contract, include the documentation and migration
review in the same change.

## Operating model

The current deployment is intentionally single-replica and does not use Lease
leader election. This keeps observation and delivery ownership unambiguous.
High availability requires a separate design for leader ownership, failover,
deduplication, and persisted state coordination; it is not implicit in adding
another replica.
