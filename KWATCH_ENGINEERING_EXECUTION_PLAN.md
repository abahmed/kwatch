# Kwatch Engineering Audit and Next-Phase Execution Plan

Status: execution handoff plan; no implementation work is represented as complete

Prepared against: `80a0b97` (`chore: release v0.11.0-rc.6 - bump pinned versions`)

Prepared: 2026-08-29

## 1. Mission

Audit the actual Kwatch implementation, fix confirmed defects, harden its Kubernetes and
release behavior, verify its existing incident-intelligence capabilities, implement only
the genuinely missing next-phase capabilities, and finish with an independent audit and an
evidence-based engineering report.

This is a multi-stage engineering program, not a single undifferentiated coding task. Work
must proceed in bounded batches. A batch is complete only after its implementation, tests,
and required verification all pass.

The project must never be described as bug-free. The strongest permitted conclusion is:

> No additional defects were found within the audited and tested scope.

## 2. Controlling decisions

These decisions resolve contradictions in the consolidated request and govern execution.

1. The final appended read-only prompt beginning with “Analyze the KWatch repository” is
   treated as superseded. It conflicts with the main instruction to fix and implement.
   Read-only analysis remains Phase 0 and Phase 1, after which approved repository changes
   are expected.
2. LLM functionality must not be introduced. Existing intelligence must remain
   deterministic, explainable, reproducible, and locally testable.
3. “Next-phase” does not mean “absent.” The current repository already contains code for
   incident identity/deduplication, smart grouping, graph-based impact/blast radius,
   incident insights, recent-change correlation, mass-failure detection, persistence,
   and lifecycle handling. Each proposed feature begins with a gap assessment. Preserve or
   improve working behavior; do not create a second competing subsystem.
4. Do not implement the six capability areas simultaneously. Use the order in Section 12.
5. Do not push, tag, publish, create a GitHub release, overwrite an artifact, or modify an
   external cluster without explicit user authorization at that checkpoint. Local builds,
   tests, rendering, kind clusters, and release dry-runs are permitted when their required
   tools are available.
6. Do not change the flat shape of `model.PersistedIncident`. It is an on-disk compatibility
   boundary.
7. Preserve all load-bearing behavior documented in `AGENTS.md`, including attribution
   ordering, PDB sync behavior, StatefulSet lister wiring, severity-map key spelling,
   SIGKILL handling, suppression consolidation, audit skip strings, provider HTTP routing,
   and dependency direction.
8. Use injected clocks for time-based decisions. Do not add direct `time.Now()` calls to
   test-sensitive decision logic.
9. A failed Kubernetes lookup must not be interpreted as deletion. State replacement must
   be based on a complete, validated view.
10. Every confirmed defect must have a regression test where practical. Record exceptions
    and explain why a deterministic test could not be added.

## 3. Model roles and handoff

### Planner and independent reviewer

Use GPT-5.6 Sol with `xhigh` reasoning for architecture decisions and `max` reasoning for
the initial high-risk audit and the final independent audit.

Responsibilities:

- decide batch scope and invariants;
- review actual implementation and actual diffs;
- resolve cross-package design questions;
- assess graph, identity, persistence, concurrency, security, and HA changes;
- accept or reject batches using test evidence.

### Executor

Use GPT-5.6 Terra with `high` reasoning for normal batches and `xhigh` for complex batches.

Responsibilities:

- confirm current behavior before editing;
- implement only the current bounded batch;
- add focused regression tests;
- run the local tests and mandatory repository gate;
- report exact commands, results, limitations, and remaining risks.

Use Sol as executor as well for changes to resource-graph semantics, incident identity,
persistence compatibility, concurrency ownership, security boundaries, leader election,
HA, and release publishing logic.

### Review checkpoints

Return to Sol after:

- the initial inventory and risk register;
- resource graph and Kubernetes identity work;
- incident lifecycle and persistence work;
- HTTP/security/RBAC work;
- each new capability;
- HA design and implementation;
- final verification.

## 4. Repository constraints and current shape

Execution must respect these observed boundaries:

- `cmd/kwatch` is the thin command entrypoint.
- `internal/app` is the composition root.
- `internal/controller` owns informer wiring, queues, sync, graph population, and baseline
  seeding.
- `internal/handler` and `internal/filter` produce candidate events.
- `internal/correlation` is the incident lifecycle owner and the only incident-notification
  emitter.
- `internal/insight` performs deterministic cause, impact, and recent-change analysis.
- `internal/context` currently contains the resource graph and change tracker.
- `internal/state` persists baseline, incident, and monitor state through ConfigMaps.
- `internal/alert` contains routing and more than fifty provider packages. HTTP providers
  must use `internal/alert/util.Send`.
- `internal/health` exposes health, readiness, metrics, optional diagnostics, and optional
  pprof.
- `api/v1alpha1`, `deploy/crd.yaml`, raw manifests, Helm values, the Helm schema, runtime
  config, and documentation form one configuration compatibility surface.
- `.github/workflows/check.yml`, `codeql.yml`, `publish.yml`, and `release.yml` form the CI
  and release surface.
- The module currently declares Go 1.27 and Kubernetes client libraries at v0.37.0.
- The checkout contains approximately 335 Go files and 387 non-Git files; review must be
  systematic rather than relying on a few searches.

The resource graph deserves immediate scrutiny. Its current edge storage must be verified
against the requirement that the same resource pair may carry multiple relationship types.
Do not assume comments or existing tests prove the invariant.

## 5. Mandatory rules for every implementation batch

### Before editing

1. Confirm the working tree state and preserve unrelated user changes.
2. Identify the exact package boundary and responsible writer.
3. Reproduce or demonstrate the problem with a focused test or a minimal evidence case.
4. Record current behavior, desired behavior, invariants, and out-of-scope items.
5. Search for all callers and persisted/configured representations affected by the change.

### While editing

1. Keep dependency direction downward.
2. Keep functions below the cyclomatic complexity limit of 20.
3. Keep files focused; split responsibilities rather than extending god files.
4. Use `goimports` with local prefix `github.com/abahmed/kwatch`.
5. Preserve deterministic ordering; never depend on Go map iteration order.
6. Bound maps, slices, queues, histories, payloads, retries, response bodies, and goroutine
   lifetimes.
7. Classify cancellation and permanent errors separately from retryable failures.
8. Never log, persist, expose, or notify Secret values.

### Verification

Run focused package tests during iteration. To control model/tool usage, run the full
repository gate at the end of each phase and before any release, not after every small
batch. A high-risk batch that changes shared state, persistence, concurrency, or lifecycle
logic also needs its focused race test before the next batch begins. The required gate is:

```sh
go build ./...
go vet ./...
go test ./...
golangci-lint run
```
Also run `go test -race ./...` after any batch affecting shared state, goroutines, timers,
queues, graphs, caches, correlation, alerts, health servers, or persistence.

Run these when their relevant surfaces change:

```sh
helm lint deploy/chart
helm template kwatch deploy/chart
bash deploy/chart/test_helm.sh
staticcheck ./...
gosec ./...
govulncheck ./...
```

Only report a command as passing if it actually executed successfully. Distinguish missing
tools, unavailable infrastructure, baseline failures, newly introduced failures, and tests
that were intentionally not run.

### Batch report

Every completed batch must report:

- batch objective;
- confirmed findings with severity;
- files changed;
- behavioral effect;
- tests added or changed;
- exact commands and results;
- unresolved risks;
- whether Sol review is required before continuing.

## 6. Severity and finding ledger

Use one ledger entry per confirmed issue:

```text
ID:
Severity: P0 | P1 | P2 | P3
Status: suspected | confirmed | fixed | verified | deferred
Component:
Files/packages:
Invariant violated:
Root cause:
User/cluster impact:
Reproduction:
Fix:
Regression test:
Verification:
Remaining risk:
```

Severity meanings:

- P0: data/state corruption, broad security compromise, uncontrolled outage, or destructive
  release behavior requiring immediate halt.
- P1: lost/duplicate critical incidents, deadlock or persistent crash, material Secret
  exposure, graph corruption, broken recovery, unsafe release, or major Kubernetes
  correctness failure.
- P2: bounded reliability, performance, compatibility, observability, or correctness defect.
- P3: low-impact edge case, maintainability defect, documentation drift, or minor UX issue.

Finish P0 and P1 issues before feature development. P2 issues blocking a new capability
must also be fixed first. P3 findings may be deferred explicitly.

## 7. Phase 0 — Reproducible baseline

Goal: establish what is true before any implementation work.

Tasks:

1. Record Git SHA, branch, working-tree status, OS/architecture, Go version, toolchain,
   Docker, Helm, kubectl, kind, staticcheck, gosec, govulncheck, and golangci-lint versions.
2. Inventory all Go packages, tests, fuzz tests, benchmarks, Kubernetes manifests, CRDs,
   Helm templates, scripts, CI jobs, and release workflows.
3. Run and capture the mandatory gate without changes.
4. Run `go test -race ./...` and the existing Helm checks.
5. Run optional analyzers when installed; record unavailable tools instead of pretending
   they passed.
6. Search suspicious patterns, but review matches in context:

   ```sh
   rg -n 'TODO|FIXME|panic\(|log\.Fatal|os\.Exit|go func|time\.NewTicker|time\.After|context\.Background|context\.TODO|interface\{\}|\bany\b|_ =|_, _' .
   ```

7. Verify LLM removal with a case-insensitive search for `llm`, `openai`, `ollama`,
   `ai.?troubleshoot`, and `ai.?enrichment`. Classify every remaining match. Do not remove
   legitimate unrelated words merely because they contain “ai.”
8. Build a claims matrix comparing README and architecture documentation with code and
   tests. Mark each claim verified, contradicted, incomplete, or untested.
9. Establish a clean-checkout build in a temporary clone or worktree after dependencies
   are available.

Exit criteria:

- baseline commands and failures are recorded;
- the repository surface map is complete;
- no files have been changed other than audit artifacts explicitly requested by the user;
- the initial risk register and candidate batch order are ready for Sol review.

## 8. Phase 1 — Core correctness and Kubernetes control plane

Execute each subsection as its own reviewed batch.

### 1A. Resource graph and resource identity

Audit and test:

- namespace, kind, name, cluster scope, and UID identity semantics;
- delete/recreate with the same namespace/name but a different UID;
- explicit identity for multiple edge types between the same node pair;
- forward/reverse adjacency symmetry;
- duplicate edge insertion and removal of one relationship without removing another;
- update replacement constructed completely before mutation;
- failed lister/API lookups preserving the last valid state;
- tombstone and cache-miss cleanup;
- orphan nodes, empty adjacency maps, stale edges, and kind-scoped pruning;
- graph rebuild atomicity and readers during rebuild;
- cycles, diamonds, self-edges, traversal termination, and deterministic results;
- concurrent mutation and traversal under the race detector;
- graph node/edge accounting and traversal/rebuild latency;
- avoidance of full graph rebuilds or full traversal per routine event.

Required failure scenarios:

- Pod dependencies change from A to B;
- Pod references the same Secret through env, envFrom, and volume;
- one relationship is removed while others remain;
- lookup fails mid-update;
- EndpointSlice is deleted while another valid slice remains;
- resource is deleted and recreated with the same name;
- graph contains a cycle and a diamond;
- rebuild occurs while incidents are analyzed.

Do not broaden graph keys or persisted incident keys without a compatibility design review.

### 1B. Informers, listers, factories, and dynamic scope

Audit every pipeline and supporting informer:

- cache synchronization and the five-minute timeout;
- enabled/disabled monitor behavior;
- Add, Update, Delete, and `DeletedFinalStateUnknown` handling;
- resync idempotency;
- reconnect and API outage recovery;
- listener-only versus worker-backed pipelines;
- baseline seeding and startup order;
- historical PDB and StatefulSet sync behavior required by `AGENTS.md`;
- multi-namespace factories, namespace selectors, exclusions, and namespaces created after
  startup;
- shutdown ordering and worker termination;
- informer health/readiness observability.

Test startup with API unavailability, missing RBAC, an absent API group, slow initial sync,
context cancellation, and eventual recovery.

### 1C. Workqueues and Kubernetes API behavior

Audit:

- per-kind queue deduplication, growth, retry accounting, backoff, exhaustion, and shutdown;
- recovery after retries exhaust and no new watch event arrives;
- NotFound, Conflict, Forbidden, Unauthorized, 429, timeout, network/server error, and
  cancellation classification;
- worker starvation and tight retry loops;
- lister/cache use before direct API calls;
- repeated Get/List calls, API calls inside loops, and pagination of direct List calls;
- resourceVersion, generation, observedGeneration, and stale-update rejection;
- client QPS/burst assumptions and 429 visibility.

Add reconciliation or periodic recovery only where required by demonstrated failure modes;
do not create indiscriminate full-cluster polling.

Exit criteria for Phase 1:

- graph invariants are explicit and tested;
- failed lookups cannot destroy valid graph state;
- informer resync and tombstones are safe;
- queue errors are classified and bounded;
- exhausted transient failures can eventually reconcile;
- race tests and the repository gate pass.

## 9. Phase 2 — Incident lifecycle, correlation, and persistence

### 2A. Incident identity and lifecycle

Verify the fixed five-stage engine order:

1. baseline;
2. attribution: node, shared dependency, owning workload;
3. cooldown;
4. identity;
5. announcement.

Test the complete lifecycle:

```text
healthy → failure → create → continued failure → recovery → pending resolve → resolved
```

Cover duplicate and out-of-order events, resync, restart, recurrence, escalation, renotify,
startup baseline, resolution hold-down, cooldown revival, crash-loop key folding, resource
deletion, and exact boundary timestamps.

Verify that only `internal/correlation` emits incident notifications and that every path—live
event, group flush, mass failure, resolve, renotify, escalation, and released symptom—passes
through the lifecycle hook.

### 2B. Attribution, grouping, inhibition, and mass failure

Audit:

- dependency versus causality;
- deterministic and explainable root-cause selection;
- attribution precedence and symptom accounting;
- group keys, exact window boundaries, stable ordering, overflow caps, and group resolution;
- unrelated incidents sharing a namespace, node, owner, image, ConfigMap, or Secret;
- inhibition and silence matching across namespace, reason, regex, and overlapping rules;
- suppressed incidents retaining their lifecycle state;
- release of still-failing symptoms after the cause resolves;
- mass-failure eligible/failed/excluded/unavailable population calculations;
- namespace exclusions and dynamic graph population;
- mass failure recovery and subsequent recurrence identity.

### 2C. Persistence and restart safety

Audit ConfigMap-backed state for:

- atomic in-memory behavior when reads or writes fail;
- malformed, missing, legacy, oversized, and partially migrated data;
- active-incident preservation during trimming;
- bounded history and baseline size;
- dirty-state/coalescing semantics;
- conflict handling and reload/retry behavior;
- startup restoration before new observations;
- no duplicate create or false resolve after restart;
- cleanup and retention;
- absence of Secret values and unnecessarily large Kubernetes objects;
- persisted schema compatibility, especially the flat `PersistedIncident` shape.

Required restart scenarios:

- active incident restored;
- pending resolve restored;
- mass failure restored;
- grouped members restored or deliberately reconstructed;
- ConfigMap write fails while memory remains valid;
- corrupted state is rejected safely;
- old schema migrates once and remains readable;
- resource is recreated with a new UID after restart.

Exit criteria for Phase 2:

- lifecycle actions occur exactly once where required;
- grouping and correlation are deterministic;
- silencing does not erase state;
- restart does not duplicate or lose active incidents within documented guarantees;
- persisted compatibility is preserved;
- focused tests, race suite, and repository gate pass.

## 10. Phase 3 — Detection and monitor correctness

Audit one monitor family per batch. For every monitor, cover add/update/delete, irrelevant
update filtering, resync, malformed objects, namespace exclusion, recovery, and exact
threshold boundaries.

### Pods and containers

- Pending, Failed, not-ready, CrashLoopBackOff, Error, OOMKilled, scheduling failures,
  restart threshold, deletion, disruption, graceful shutdown, and recovery;
- threshold minus one, exact threshold, and threshold plus one;
- Error/OOMKilled/CrashLoop transitions;
- exit 137 with a non-OOM reason remains SIGKILL, not OOM;
- condition timestamps and future/zero times;
- init containers, sidecars, nil states, missing owners, multiple owners, and bare pods.

### Nodes and node resources

- Ready=False/Unknown, MemoryPressure, DiskPressure, PIDPressure, NetworkUnavailable;
- recovery and suppression release;
- CPU/memory units, requests, limits, allocatable, pod overhead, init containers,
  DaemonSets, static pods, exclusions, and zero allocatable.

### Workloads and schedules

- Deployments: rollout success, progress deadline, pause, scale, delete, recovery;
- StatefulSets: rolling/partitioned update, scale, unavailable replicas, delete, recovery;
- DaemonSets: node changes, desired/current/ready counts, rollout, recovery;
- Jobs: failed, retry/backoff, suspended, success, delete;
- CronJobs: timezone, suspend, concurrency policy, starting deadline, missed schedules,
  controller delay, and no incident for intentional suspension;
- HPA: do not treat maxReplicas alone as failure;
- PDB: do not treat zero disruptionsAllowed alone as failure.

### Networking, admission, storage, TLS, and control plane

- Services: selectors, selectorless/headless behavior, selector transitions, deletion;
- EndpointSlices: multiple slices, readiness/serving/terminating, partial deletion;
- Ingress: hosts, paths, default backend, missing/changed services, zero endpoints;
- NetworkPolicy: no outage inferred merely from restrictive configuration;
- admission webhooks: service/endpoints, selectors, failure policy, timeout;
- PVC/PV/StorageClass usage, binding, capacity, and recovery;
- TLS expiry, near-expiry, malformed certs, bundles, multiple certs, and missing expiry;
- control-plane and cluster-autoscaler signals with correct lifecycle semantics.

Exit criteria:

- every enabled monitor has a documented signal and recovery matrix;
- benign configured states do not create incidents;
- malformed input cannot panic;
- update filters preserve all meaningful dependency/status changes;
- repository gate and relevant race tests pass.

## 11. Phase 4 — Delivery, HTTP, security, configuration, and packaging

### 4A. Alert delivery

Audit shared routing first, then providers by transport family rather than duplicating the
same review fifty times.

Verify:

- all incident paths use the manager and provider HTTP calls use `alert/util.Send`;
- timeouts, context propagation, body closure, body-size caps, connection reuse, redirects,
  TLS, proxy behavior, and custom CAs;
- 2xx success, permanent 4xx, provider-specific 409/429 behavior, retryable 5xx/timeouts,
  and cancellation;
- bounded retries, rate limits, queues, dead letters, and shutdown;
- one slow provider does not block Kubernetes reconciliation;
- ordering where required and isolation where not;
- payload caps and safe truncation;
- escaping Kubernetes-controlled text and protection against notification/log injection;
- credential and provider-response redaction;
- routing, fallback, and duplicate-notification behavior.

Use representative conformance tests for shared HTTP semantics plus focused tests for
provider-specific payload/authentication logic.

### 4B. Health, diagnostics, metrics, and outbound URLs

Audit:

- liveness as process functionality and readiness as initialized core capability;
- method restrictions and response limits;
- diagnostics and pprof exposure, authentication defaults, and constant-time token checks;
- incidents/deadletters/test-alert data exposure;
- server start failure propagation, panic safety, graceful shutdown, and timeouts;
- configurable proxy/provider/heartbeat/runbook URLs for SSRF and redirect bypass;
- localhost, link-local/cloud metadata, Kubernetes API, and internal-service access policy;
- bounded JSON and profile outputs where applicable.

Do not silently introduce a breaking SSRF policy. Define the intended trust model and
configuration migration first.

### 4C. Security, RBAC, container, and dependencies

Audit:

- Secret leakage, unsafe deserialization, path traversal, command execution, file output,
  log injection, and arbitrary URLs;
- least-privilege verbs/resources/API groups against actual informer and persistence calls;
- namespace-scoped versus cluster-scoped deployment implications;
- behavior under missing permissions without infinite retries;
- non-root execution, read-only root filesystem, dropped capabilities, seccomp, privilege
  escalation, host access, writable paths, and graceful termination;
- direct and transitive dependencies, unused modules, govulncheck results, SBOM, license,
  signing, and provenance readiness.

### 4D. Configuration and CRD consistency

Build a field-by-field matrix across:

```text
defaults → YAML parser → validator → runtime → CRD types/schema/watcher
         → config examples → Helm values/schema/template → documentation
```

Test unset, zero, false, empty, null, disabled, invalid durations, URLs, regex, enums,
thresholds, retries, and incompatible combinations. Document source precedence and restart
versus hot-reload behavior.

Verify CRD structural schema, defaults, bounds, nullable behavior, upgrades, conversion
expectations, and existing-object compatibility. Do not assume the Go CRD types currently
cover the full runtime configuration.

### 4E. Helm, raw manifests, CI, and release automation

Audit:

- Deployment, ServiceAccount, RBAC, Service, ConfigMap, probes, strategy, security context,
  resources, CRDs, and image references;
- default, null, wrong-type, empty-map/list, override, and upgrade renders;
- Helm ownership metadata and install/upgrade/rollback/uninstall behavior;
- chart version, appVersion, rendered image, raw manifest image, README pins, release tags,
  and binary version consistency;
- RC/stable/patch computation and the intentional RC tag-only manifest commit;
- dry-run behavior, rerun idempotency, immutable tags, latest-tag policy, architecture
  manifests, digest reporting, changelog bounds, and concurrency controls;
- workflow permissions, action pinning, untrusted input handling, secrets, attestations,
  provenance, signing, SBOM, and Artifact Hub publication.

Actual publishing remains a separately authorized final step.

Exit criteria for Phase 4:

- delivery failures are bounded and correctly classified;
- diagnostic/security boundaries are explicit and tested;
- RBAC matches runtime needs;
- configuration surfaces have a consistency matrix;
- local Helm/release validation passes;
- repository gate and race tests pass.

## 12. Phase 5 — Kwatch self-observability

Current metrics cover incident actions, notification attempts/drops, active incidents,
baseline size, and graph node/edge counts. Gap-assess before adding a metrics framework.

Add or verify low-cardinality status for:

- startup result and phase;
- informer sync/health;
- queue depth, processing duration, retries, exhaustion, and worker failures;
- Kubernetes API requests, latency, error classes, and throttling where observable through
  supported client hooks;
- cache sync and last successful reconciliation;
- graph size, rebuild/prune/analyze duration, and failures;
- incident processing duration and active counts;
- notification success/failure/retry/drop and last success;
- persistence load/save success/failure/latency and last success;
- leader status and transitions after HA exists;
- subsystem degraded status;
- bounded process/runtime metrics where appropriate.

Never label metrics with pod name, UID, raw error, URL, provider credential, arbitrary user
text, or other high-cardinality/untrusted values. Define HELP/TYPE output deterministically
and add parser-level tests.

Readiness must depend only on required core initialization. Optional notification providers
must not make the process unready unless that behavior is explicitly selected and documented.

Exit criteria:

- an operator can distinguish API, informer, queue, graph, incident, notification, and
  persistence degradation;
- metrics are low cardinality and tested;
- health/readiness semantics match Helm probes and documentation.

## 13. Phase 6 — Capability gap assessment and implementation

For every capability, first mark each requirement as `already implemented`, `partially
implemented`, `missing`, or `incorrect`. Strengthen the existing owner rather than adding a
parallel engine.

### 6A. Incident deduplication

Owner: `internal/correlation` and `internal/model`.

Verify stable keys across duplicate observations, resync, restart, owner encoding, reason
normalization, crash-loop state transitions, global image-pull scope, deletion/recreation,
and out-of-order events. Define UID compatibility before adding UID to keys. Preserve
provider thread/dedup identifiers.

Acceptance:

- one logical ongoing problem has one incident;
- resolution and genuine recurrence follow documented identity rules;
- no accidental cross-namespace or cross-workload grouping;
- persisted/restored identity is compatible and tested.

### 6B. Blast radius

Owner: `internal/insight` over `internal/context.ResourceGraph`.

Verify transitive downstream impact, named Service/Ingress output, count caps, namespace
scope, cycles, graph staleness, and shared dependencies. Distinguish potential exposure from
observed failure.

Acceptance:

- results are deterministic and bounded;
- cycles terminate;
- output states evidence and uncertainty;
- graph gaps do not fabricate impact;
- performance is measured on synthetic graphs.

### 6C. Incident intelligence

Owner: `internal/insight`, enrichment, report building, and correlation lifecycle hook.

Verify cause ranking uses health, timestamps, transitions, relationship type/depth, and
evidence—not topology alone. Make scoring/rules explicit, stable, and testable. Ensure every
delivery path receives the same structured facts and renderers never parse hints.

Acceptance:

- cause explanations identify why a candidate was selected;
- identical state produces identical output;
- absence of evidence is represented honestly;
- no LLM or remote inference is introduced.

### 6D. Maintenance and deployment awareness

Start only after current filters, disruption handling, rollout monitors, audit skips, and
change tracker have been mapped.

Define a deterministic maintenance model covering intentional deletion, eviction, drain,
rollout, pause, Job/CronJob suspension, and optionally explicit maintenance annotations or
windows. Maintenance must suppress announcement, not destroy lifecycle state. It must expire
and release still-failing incidents.

Acceptance:

- expected rollout/drain behavior does not page prematurely;
- genuine failures during maintenance remain observable and can escalate;
- maintenance expiration releases active problems exactly once;
- state survives restart where the maintenance source survives;
- rules and evidence are auditable.

### 6E. Change correlation

Owner: existing `ChangeTracker` and insight analysis.

Verify resource generation/resourceVersion handling, causal time windows, dependency and
owner changes, pod-creation exclusion, namespace isolation, caps, restart behavior, and
stale/future timestamps. Expand tracked change types only with a clear causal model.

Acceptance:

- only plausible causal changes are reported;
- results are deterministic and time-bounded;
- unrelated namespace churn is excluded;
- rollouts and dependency edits are distinguished;
- memory remains bounded.

### 6F. High availability

Implement last, after state ownership and persistence are proven.

First produce a design decision covering:

- leader-election mechanism and Lease RBAC;
- which components run on followers;
- who owns informers, queues, timers, persistence, and notification delivery;
- leader readiness and fencing;
- handoff of dirty in-memory state;
- duplicate suppression during failover;
- compatibility with current ConfigMap persistence;
- startup baseline semantics on promotion;
- Helm replica/strategy/PDB changes;
- split-brain and Kubernetes API partition behavior.

Running multiple replicas without notification fencing is not HA.

Required scenarios:

- two and three replicas;
- clean leader termination;
- abrupt leader loss;
- follower promotion during active incidents and pending resolves;
- API timeout/partition around Lease renewal;
- persistence write failure before failover;
- no duplicate create/resolve/group/renotify across leadership change;
- rollback to a non-HA release.

Acceptance:

- exactly one notification authority exists at a time within tested guarantees;
- failover time is measured;
- active lifecycle state remains coherent;
- leader/follower readiness and metrics are explicit;
- RBAC and Helm lifecycle tests pass.

## 14. Phase 7 — Integration, compatibility, failure injection, and fuzzing

Use real temporary Kubernetes clusters where practical. Select the oldest and newest
supported Kubernetes versions only after the project declares and documents that range.

### Integration matrix

- clean Helm install and rollout readiness;
- upgrade from latest supported stable state;
- rollback with state/CRD compatibility;
- uninstall and owned-resource cleanup;
- raw manifest installation;
- CRD create/update and watcher behavior;
- actual ServiceAccount permissions;
- startup with pre-existing failures;
- restart with active incidents;
- API outage, recovery, 429, timeout, Forbidden, NotFound, Conflict;
- informer reconnect/resync;
- EndpointSlice partial deletion;
- delete/recreate identity;
- notification endpoint slow/failing/rate-limited;
- graceful SIGTERM within termination grace period;
- HA failover after HA implementation.

### Failure injection

Inject failures at graph lookup, queue processing, persistence read/write, provider send,
health server bind/start, informer sync, API request, and leader renewal boundaries. Verify
bounded retries, recovery, state preservation, and observable degradation.

### Fuzzing

Prioritize parsers and identity boundaries:

- configuration and CRD translation;
- incident key/reason normalization;
- provider response classification;
- Kubernetes object extraction with nil/malformed fields;
- message truncation/escaping;
- persisted-state decoding and migration;
- regex validation and matching inputs.

Use time-bounded fuzz runs in routine CI and longer runs as an optional scheduled job.

Exit criteria:

- integration results are recorded by Kubernetes version;
- failure recovery is demonstrated, not inferred;
- fuzz targets preserve invariants and do not panic;
- unavailable infrastructure is listed as an explicit remaining risk.

## 15. Phase 8 — Performance and long-running stability

Create reproducible synthetic or kind-based workloads approximating 1K, 5K, and 10K+
resources. Do not claim cluster-size support from unit benchmarks alone.

Measure:

- startup and cache-sync duration;
- CPU and memory;
- goroutine count and growth;
- queue depth and processing latency;
- event throughput;
- incident-processing latency;
- graph node/edge count, mutation, traversal, prune, and rebuild latency;
- Kubernetes API QPS/error/throttle rate;
- notification throughput and queueing;
- persistence size and save latency;
- metric cardinality.

Compare steady state, burst failure, recovery, resync, and notification outage. Run an
appropriately bounded soak and capture duration. Investigate unbounded trends rather than
reporting only final values.

Exit criteria:

- methodology, fixtures, environment, and raw results are recorded;
- practical bottlenecks and untested scale limits are stated;
- no size guarantee is made beyond measured evidence;
- new capabilities do not cause full-cluster analysis on every event.

## 16. Phase 9 — Independent final audit

Use a fresh Sol review at `max` reasoning. Begin from the code and architecture rather than
from the original finding list. The reviewer must inspect the complete diff since Phase 0
and independently search for:

- state corruption and partial updates;
- races, deadlocks, goroutine/timer leaks, and shutdown failures;
- graph and identity inconsistencies;
- API misuse and throttling;
- retry/reconciliation gaps;
- lifecycle, grouping, and persistence defects;
- Secret exposure, SSRF, injection, diagnostic exposure, and excessive RBAC;
- compatibility, CRD, Helm, and release drift;
- performance regressions and high-cardinality metrics;
- documentation claims not supported by code or tests;
- reintroduced LLM references or dependencies.

Fix newly confirmed issues in new bounded batches and repeat their verification. Do not
silently fold final-audit findings into the initial list.

Run the complete applicable command matrix one final time from a clean checkout. Record
exact versions and results.

## 17. Phase 10 — Release readiness and optional publication

Before any external release action, produce a release-candidate evidence bundle containing:

- intended Git tag and source SHA;
- binary-reported version;
- container tag, architectures, and local image inspection;
- expected immutable digest behavior;
- chart version and appVersion;
- rendered image reference;
- raw manifest and documentation pins;
- CRD and RBAC result;
- Helm install/upgrade/rollback/uninstall result;
- SBOM, provenance, signing, and vulnerability-scan status;
- known limitations and unexecuted tests.

Then stop and request explicit authorization for tag creation, GitHub release creation,
container push, chart publication, or other external mutation. After authorization, verify
the published artifacts by digest and install the published artifact—not merely the local
source—to a temporary cluster.

Never overwrite an immutable release tag. `latest` may be a convenience pointer for stable
releases but must not be the authoritative release identity.

## 18. Final engineering report

The final report must contain:

1. Executive conclusion limited to actual audited/tested scope.
2. Initial findings by P0/P1/P2/P3, each with component, root cause, impact, reproduction,
   fix, and regression test.
3. Additional findings discovered during the independent final audit.
4. Verified-safe areas, naming the evidence used.
5. Remaining risks and all untested/infrastructure-dependent/provider-dependent behavior.
6. Exact command results for build, vet, unit, race, lint, staticcheck, gosec,
   govulncheck, Helm, Kubernetes integration, lifecycle, CRD, RBAC, container, release, HA,
   performance, and scale tests.
7. Release-verification table: tag, SHA, binary, container, digest, architectures, chart,
   appVersion, rendered image, and install/upgrade/rollback results.
8. LLM-removal search results with an explanation for every legitimate remaining match.
9. Capability status for deduplication, blast radius, incident intelligence, maintenance
   awareness, change correlation, and HA using `Implemented`, `Tested`, `Verified`, and
   `Remaining limitations` as separate fields.
10. Documentation changes and claims verified against implementation.

## 19. Global definition of done

The program is complete only when all applicable statements are evidence-backed:

- actual source, tests, manifests, Helm, CI, releases, and docs were audited;
- all P0/P1 findings are fixed or explicitly accepted by the user;
- required blocking P2 findings are fixed;
- confirmed fixes have regression tests where practical;
- graph reconciliation is safe and graph invariants are tested;
- Kubernetes API failures cannot erase valid state;
- informer and workqueue lifecycles recover and shut down correctly;
- incident lifecycle, attribution, grouping, inhibition, and mass failure are deterministic;
- persistence and restart behavior preserve active lifecycle state within documented limits;
- race testing passes;
- notification delivery is bounded and isolated;
- security, RBAC, container, and diagnostic exposure are reviewed and verified;
- configuration, CRD, Helm, raw manifests, runtime, and docs agree;
- self-observability diagnoses Kwatch’s own major subsystems without high cardinality;
- existing intelligence features are verified before gaps are implemented;
- HA is either safely implemented and tested or explicitly documented as unsupported;
- performance and practical scale limits are measured;
- the final independent audit is complete;
- all unexecuted checks and remaining risks are explicit;
- no LLM functionality has been introduced.

## 20. Executor kickoff

After switching to the executor model, use this instruction:

```text
Read AGENTS.md and KWATCH_ENGINEERING_EXECUTION_PLAN.md completely.

Begin with Phase 0 only. Do not implement fixes yet. Establish the reproducible baseline,
create the architecture/claims matrix and initial finding ledger, and report actual command
results. Preserve unrelated working-tree changes. Do not push, tag, publish, or mutate an
external cluster.

At the end of Phase 0, stop for the planned Sol review checkpoint. Do not proceed into a
large repository-wide rewrite and do not claim a suspected issue is confirmed without a
reproduction or direct code evidence.
```
