# Kwatch Phase 0 Baseline

Prepared: 2026-08-29

## Scope

This is the read-only baseline required by Phase 0 of
`KWATCH_ENGINEERING_EXECUTION_PLAN.md`. It establishes the current checkout,
the local verification result, evidence for key architectural claims, and the
first confirmed audit findings. No production code was changed.

## Checkout and environment

| Item | Value |
|---|---|
| Git SHA | `80a0b9727ec06aced95b9dc812ad9598a0112cf4` |
| Branch | `main` |
| Host | Darwin 23.6.0, arm64 |
| Go | `go1.27.0 darwin/arm64` |
| Helm | `v4.1.1+g5caf004` |
| Go source files | 335 |
| Go test files | 135 |
| Working tree before audit artifacts | clean |

The audit-plan and this baseline document are currently untracked worktree
artifacts. No existing repository file was modified.


| Command | Result | Notes |
|---|---|---|
| `go build ./...` | pass | Completed successfully. |
| `go vet ./...` | pass | Completed successfully. |
| `go test ./...` | pass | All packages passed. |
| `go test -race ./...` | pass | All packages, including controller, correlation, graph, state, and integration tests, passed. |
| `golangci-lint run` | not run | `golangci-lint` is not installed/on PATH. This leaves the repository's mandatory gate incomplete locally. |
| `helm lint deploy/chart` | pass | One chart linted; no failures. |
| `helm template kwatch deploy/chart` | pass | Rendered successfully. |
| `bash deploy/chart/test_helm.sh` | pass | Default render, resource limits, and security-context checks passed. |
| `staticcheck ./...` | not run | Tool not installed/on PATH. |
| `gosec ./...` | not run | Tool not installed/on PATH. |
| `govulncheck ./...` | not run | Tool not installed/on PATH. |
| Kubernetes integration | not run | `kubectl`, `kind`, and Docker are not installed/on PATH. |

## Exact LLM-removal check

Independent literal searches for `llm`, `openai`, `ollama`, `ai troubleshoot`,
`ai-troubleshoot`, `ai enrichment`, and `ai-enrichment` found no matches outside
this audit work. No LLM implementation, dependency, configuration, Helm setting,
or documentation reference was found in the pre-existing repository.

## Architecture and claims matrix

| Claim | Evidence | Status |
|---|---|---|
| The controller owns informer wiring, queues, graph maintenance, and baseline seeding. | `internal/controller/controller.go`, `pipeline.go`, `wiring.go`, and `graph*.go`. | verified |
| Correlation is the incident lifecycle and notification-decision owner. | `internal/correlation/engine.go`, `emit.go`, lifecycle/grouping tests, and `AGENTS.md`. | verified |
| Incidents are deduplicated, grouped, persisted, and restored. | `internal/correlation/*`, `internal/state/*`, plus correlation and state tests. | verified, pending deep semantic audit |
| Insight supplies deterministic cause, impact, and recent changes. | `internal/insight/*`, `internal/context/graph.go`, and insight tests. | verified, pending correctness/performance audit |
| Mass failure and suppression exist. | `internal/insight/patterns.go`, `internal/correlation/attribution.go`, and regression tests. | verified, pending population/recovery audit |
| State is ConfigMap-backed and supports legacy migration. | `internal/state/*` and state migration/retention tests. | verified, pending write-failure and restart audit |
| Health, readiness, optional diagnostics/pprof, and metrics exist. | `internal/health/*`, `internal/metrics/metrics.go`, and health tests. | verified, pending access-control/cardinality audit |
| CRD-based live configuration exists. | `api/v1alpha1/*`, `internal/crdwatch/*`, and configuration code. | partially verified; CRD watcher has no direct package test in the baseline suite |
| README-supported monitor coverage exists. | Controller pipelines and handler monitor tests cover pod, node, workload, service/EndpointSlice, ingress, webhook, TLS, and related signals. | partially verified; each monitor still needs its own Phase 3 behavior matrix |
| Delivery resilience exists across all listed providers. | `internal/alert` routing/delivery plus provider packages and tests. | partially verified; shared HTTP semantics and provider-specific limits require audit |
| The product is currently single replica. | Helm default renders `replicas: 1` and `strategy: Recreate`; architecture docs state single replica. | verified; HA is not yet implemented |

## Confirmed findings

### F-001 — P1 — Graph rebuild can erase valid graph state on cache/list failure

| Field | Evidence |
|---|---|
| Component | `internal/controller` resource graph rebuild |
| Files | `internal/controller/graph.go:93-109` |
| Invariant violated | A failed Kubernetes/cache lookup must not destroy the last known valid graph. |
| Root cause | `buildGraph` calls `c.graph.Clear()` before `c.podLister.List`. If the lister returns an error, the function returns with an empty graph. |
| Impact | Insight, blast radius, mass-failure denominator calculation, and attribution may become empty or incorrect during a transient cache/list error. The last valid topology is lost until a later successful rebuild. |
| Reproduction | Seed a graph, make `podLister.List` return an error, call `buildGraph`, and assert the seeded graph remains. No such regression test currently protects the invariant. |
| Required direction | Build and validate a replacement graph off to the side, then atomically swap it in only after all required inputs are available and valid. Preserve the existing graph on failure. |
| Status | confirmed by direct code inspection; fix deferred to the Phase 1 graph batch |

### F-002 — P1 — Resource graph cannot preserve multiple relationship types for the same pair

| Field | Evidence |
|---|---|
| Component | `internal/context` resource graph |
| Files | `internal/context/graph.go:50-72,96-110`; `internal/controller/graph.go:16-88` |
| Invariant violated | The same resource pair may carry multiple relationships, and removing one must not remove the others. |
| Root cause | `edgeKey` uses only `from` and `to`, while `edges` stores a single `Edge` and adjacency maps are boolean. A second `AddEdge` for the same pair overwrites the first edge type. |
| Impact | A pod referencing a ConfigMap or Secret by volume and env/envFrom loses relationship identity. `RemoveEdgesFrom` can either preserve a relationship that should be removed or remove adjacency still justified by a different relationship. Insight output and cleanup can become stale or incomplete. |
| Reproduction | Add `pod → ConfigMap` as `mounts` and `env_ref`; the edge list contains one entry, not two. Removing `env_ref` deletes the only adjacency even though `mounts` remains. Existing `TestMultipleEdges` covers different targets, not different types for the same target. |
| Required direction | Introduce explicit edge identity including type (and, if necessary, source detail), preserve adjacency while at least one typed edge exists, and add same-pair/multiple-type removal tests. |
| Status | confirmed by direct code inspection; fix deferred to the Phase 1 graph batch |

## Review candidates, not yet confirmed defects

| ID | Priority for review | Evidence | Required next step |
|---|---|---|---|
| C-001 | high | `internal/controller/controller.go:135` calls `os.Exit(1)` from `Controller.New` when namespace resolution fails. | Determine whether this bypasses intended app-level shutdown/testability and replace with an error return only if the public construction path can safely change. |
| C-002 | high | Per-pod graph updates make lister calls while maintaining graph relationships. | Audit updates, lister failures, partial graph changes, service selector transitions, and informer resync with targeted fault tests. |
| C-003 | medium | `internal/metrics/metrics.go` renders an action map. | Verify deterministic output and Prometheus exposition correctness; do not treat it as a correctness defect until a test demonstrates impact. |
| C-004 | medium | Health diagnostics are optionally unauthenticated when enabled without a token. | Establish intended deployment trust model, default Helm exposure, and sensitive-data bounds before changing behavior. |
| C-005 | medium | CRD watcher has no direct baseline package test. | Audit hot-reload merge/validation/rollback behavior and add coverage if behavior is supported. |

## Phase 0 conclusion

The local Go build, vet, unit, race, and Helm baseline is green. Lint, static analysis,
security scanning, vulnerability scanning, Docker/kind integration, and real-cluster tests
are currently unavailable due missing local tools or infrastructure. Two P1 graph defects
are confirmed and should be the first remediation batch after the planned design review.
