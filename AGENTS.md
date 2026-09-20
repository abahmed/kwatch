# AGENTS.md — conventions for working on kwatch

Guidance for humans and AI agents making changes to this repository. For the
user-facing contribution process and published technical documentation, see
[kwatch.dev/docs](https://kwatch.dev/docs). The root `CONTRIBUTING.md` is a
short repository entry point; it must not become a second public documentation
source.

## The gate

Every change must pass before you are done:

```sh
go build ./... && go vet ./... && go test ./... && golangci-lint run
```

The repository also enforces formatting and line length. Run the complete gate
with `make verify`; it includes `line-check` and `git diff --check` should be
clean before handoff.

## Reliability invariants

The following rules are enforced by the current implementation and must remain
true when extending the system:

- Delivery reconfiguration is a generation transition, not an application
  failure. Old provider queues drain before replacement; jobs accepted before
  replacement keep their generation and fallback lookup.
- Notifications created before delivery starts wait in a bounded pending queue.
  They must never perform provider I/O on the caller goroutine or use a
  background context for live delivery.
- Required persistence savers use the active lifecycle context for periodic
  writes and a bounded final-write context during shutdown. A disabled write
  gate is a clean stop, not an unexpected component crash.
- A missing or unavailable source skips detection and never creates or resolves
  a synthetic incident. Source diagnostics use bounded reason codes and exclude
  disabled pipelines.
- Matrix HTML escapes all event-derived data while preserving only Kwatch's
  generated tags. Provider response bodies are parsed when an HTTP 2xx can
  still contain a provider-level error.
- GoAlert requires an explicit real endpoint; placeholder example URLs must
  never receive credentials. Pushover priority `2` requires valid `retry` and
  `expire` values; priority `1` does not.
- Any new long-running loop must expose cancellation, completion, progress or
  synchronization state, and a bounded shutdown path. Tests must use event
  completion rather than sleeps.
- Generated Kubernetes deep-copy code must detach pointers, maps, slices, and
  nested monitor configuration before a CRD object is handed to another
  controller or goroutine.

- Linters: errcheck, gocritic, gocyclo, govet, ineffassign, unparam, unused (`.golangci.yml`).
- **Cyclomatic complexity limit is 20** (`gocyclo min-complexity: 20`), tests included. When a
  function exceeds it, extract helpers or table data instead of raising the threshold.
- Formatting: `goimports` with `local-prefixes github.com/abahmed/kwatch` (stdlib first,
  third-party second, kwatch last).
- Test files are exempt from errcheck/unparam/gocritic/gocyclo; `internal/controller` is
  exempt from errcheck (informer wiring intentionally ignores AddEventHandler returns).

## Coding style directives

These directives apply to humans and coding agents. Preserve the existing
architecture unless a change explicitly expands its scope.

- Use `goimports`, not only `gofmt`, with the repository local prefix. Keep
  imports grouped as standard library, third-party dependencies, then kwatch.
- Keep every touched or newly created line at 80 columns or fewer. Wrap calls,
  signatures, composite literals, and comments; do not hide violations by
  disabling the line checker.
- Keep every hand-written Go file below 400 lines. Generated catalog data is
  exempt; test files should be split when practical. When a hand-written file
  grows, split it by responsibility and use semantic names such as
  `conditions.go` or `payload_limits_test.go`, not anonymous numeric
  fragments.
- Never create `part2`, `part3`, `extra`, or similarly history-based test file
  names. Split tests by domain behavior, lifecycle, transport, persistence,
  or fixture responsibility; examples include `grouping_scope_test.go`,
  `controller_queue_test.go`, and `payload_limits_test.go`. A cohesive test
  file may be large when it covers one clear unit, but a large package must
  not be hidden behind arbitrary numbered fragments.
- Match the package name to the final directory component. For example,
  `internal/graphcontext` must declare `package graphcontext`.
- Add short comments only where they explain an invariant, persisted-format
  compatibility rule,
  or non-obvious decision. Avoid decorative separator comments and restating
  the code.
- Prefer small functions with one responsibility. Extract `build*`, `parse*`,
  `apply*`, and `validate*` helpers before complexity or readability suffers.
- Return errors to callers. `os.Exit` belongs only in the top-level command
  entrypoint; libraries and subcommands must remain testable.
- Inject clocks, HTTP clients, listers, and other time- or I/O-dependent
  collaborators through constructors or setters. Production code must not
  call `time.Now()` directly when a decision can be tested with a fake clock.
- Keep package globals immutable or narrowly scoped. Use an explicit registry
  or dependency seam for mutable process state; do not add transitional
  aliases or wrappers.
- Use `metrics.DefaultRegistry()` at internal call sites; do not introduce
  hidden singleton clients.
- Construct shared Kubernetes or HTTP clients in `internal/app` and pass them
  into monitors, providers, and integrations that need them.
- Import `internal/graphcontext` with an explicit alias when the standard
  library `context` is also in scope; never disguise a package-name mismatch.

- Do not duplicate provider transport or retry logic. Providers build payloads
	and call `delivery/transport`; shared transport decides status
	classification, timeout, retry, and rate-limit behavior.
- Preserve external behavior while a migration is in progress, but do not let
  the current internal package layout constrain the target design. Persisted
  formats may change before the first stable release only through an explicit
  versioned migration, backup, or clearly documented reset path.

### Naming standard

- Use `New<Type>` for constructors. Use `Set<Type>` only for narrow optional
  state that is explicitly documented; source wiring must use one-time
  `ConfigureSources` and must not use mutable production setters. Keep
  constructor arguments ordered as configuration, required dependencies, then
  optional dependencies.
- Name methods after the domain action: `Process`, `Resolve`, `Snapshot`, and
  `Validate`. Avoid vague verbs such as `Do`, `HandleIt`, or `Create` when the
  resource type is known.
- Use singular package names and lower-case file names. Group files by one
  responsibility: `graph_resources.go`, `group_flush.go`, and
  `payload_limits_test.go` are preferred examples.
- Follow Go initialisms consistently: `ID`, `UID`, `URL`, `HTTP`, `API`, `PVC`,
  and `JSON`. Do not introduce a new spelling variant for an existing public
  identifier. Preserve wrappers only for supported external APIs; internal
  package renames should converge on the canonical domain vocabulary.
- Use `camelCase` for local names, `PascalCase` for exported names, and avoid
  redundant package prefixes such as `config.ConfigManager`.
- Use domain vocabulary in new code: `incidentEngine`, `deliveryManager`, and
  `persistenceManager`. Do not introduce new `correlator`, `alertManager`, or
  `stateMgr` identifiers; those names describe the old implementation layout.
- Use `Test<Type><Behavior>` for tests. Name table cases by behavior, not by
  implementation order or issue number.

### Test and refactor directives

- Before splitting a test file, identify package-level fixtures, helper types,
  and imports. Copy required declarations into the correct focused file and
  run that package's tests immediately after the split.
- When a test package needs multiple files, keep shared setup in a clearly
  named `fixtures_test.go` or `test_helpers_test.go` file and keep assertions
  next to the behavior they specify. The file name should help a contributor
  choose the smallest focused test command.
- Keep tests deterministic: use injected clocks and fake clients rather than
  sleeps, wall-clock assertions, or live network calls.
- Add or update focused tests for every behavior change, especially lifecycle
  transitions, suppression decisions, retries, and persisted-format migration.
- Do not mechanically rewrite unrelated files. Review `git diff` after each
  refactor and preserve user changes already present in the worktree.
- Remove transitional APIs once their callers are migrated. Confirm with
  `rg`, then run focused validation and the full repository gate.

## Package map

Dependency direction flows downward; never import upward.

| Package | Responsibility |
|:--|:--|
| `cmd/kwatch` | Thin entrypoint: flag parsing, subcommand dispatch, calls `app.Run()` |
| `internal/app` | Composition root: builds config, controller, incident engine, persistence and runs them |
| `internal/monitor` | Monitor descriptors and the extension contract for user-visible detection modules |
| `internal/controller` | Informer wiring, workqueues, graph state, and narrow family contracts |
| `internal/monitor/pod/policy` | Pure, deterministic Pod/container detection rules and policy decisions |
| `internal/monitor/pod/enrichment` | Kubernetes-backed Pod event, owner, log, and suppression enrichment |
| `internal/monitor/node` | Node detection policy and direct queue runtime |
| `internal/monitor/network` | Network detection policy and direct queue runtime |
| `internal/monitor/cluster` | Cluster-resource detection and direct queue runtime |
| `internal/observe` | Kubernetes objects → `model.Observation`; the single pod-ownership resolver |
| `internal/config` | Config loading/validation, suppression index builder |
| `internal/filter` | Pure detect-time suppression matching over the compiled index |
| `internal/incident` | Incident identity, lifecycle, attribution, grouping, and notification decisions; the **only** lifecycle emitter (`emit.go`) |
| `internal/insight` | Cause/impact/recent-change analysis over the dependency graph |
| `internal/event`, `internal/graphcontext`, `internal/model` | Shared types |
| `internal/delivery/*` | Delivery manager, routing, retries, rate limits, transport, and provider dispatch |
| `internal/delivery/api` | Neutral provider contract shared by delivery and the static catalog |
| `internal/alert/*` | Provider adapters; one subpackage per provider |
| `internal/alert/catalog` | Statically linked provider construction selected by the application |
| `internal/persistence` | Restart-safe ConfigMap persistence, format migrations, and recovery |
| `internal/rbac` | Permission auditing and RBAC health; not resource-security detection |
| `internal/k8s/dynamicwatch` | Shared dynamic informer discovery, lifecycle, cache-sync, and optional-resource status |
| `internal/startup`, `internal/upgrader` | Startup lifecycle and upgrade checks |
| `internal/{pvc,heartbeat,health,audit,crdwatch,integration}` | Periodic watchdogs and integrations |
| `internal/k8s`, `internal/kubelet`, `internal/client`, `internal/resource` | Kubernetes access helpers |

Application composition is split by phase so startup wiring stays readable:

- `internal/app/bootstrap.go` owns infrastructure and startup state.
- `internal/app/runtime_build.go` assembles domain components.
- `internal/app/runtime_controller.go` wires listers, restore, graph, and readiness.
- `internal/app/runtime_optional.go` builds optional monitor runs.
- `internal/app/runtime_deps.go` records runtime ownership for serving and shutdown.

`config.RuntimeConfig` is the immutable snapshot of normalized namespaces,
reasons, provider settings, routes, retry policy, suppression rules, monitor
policies, CRD rules, delivery templates, intervals, and worker settings. Build
it after configuration overlays and do not add new derived fields to the
YAML-facing model. Runtime accessors return defensive copies where needed;
delivery and integrations must not reparse raw YAML maps in production.

`internal/client.ClientSet` is the application-owned client boundary. Build
typed, dynamic, discovery, REST, HTTP, DNS, and kubelet dependencies once in
the composition root and pass only the narrow client each component needs.
All production constructors receive application-owned clients and runtime
dependencies directly. Transitional compatibility constructors were removed
before the first stable release.

`controller.Controller` groups queue pipelines, typed family source views,
namespace scope, and informer diagnostics in focused state bundles. The
controller remains the single owner of synchronized informer sources; family
runtimes receive only their matching view through explicit wiring.

`controller.NewWithRuntimeConfig` is the composition entrypoint. Application
code and tests pass the already compiled `config.RuntimeConfig`; family
constructors follow the same rule.

`incident.AttributionSources` is the only source boundary used by incident
attribution. The incident package must not retain Kubernetes listers or import
client-go lister implementations. The controller supplies an adapter during
composition, before processing starts.

The application owns lifecycle goroutines and shutdown. Health starts and
stops only its HTTP server; the application calls `Open`, supervises `Serve`,
and calls `Stop`. Health must not create a second
context-shutdown goroutine. Every background persistence saver, watcher,
ticker, and worker has an owner, cancellation path, bounded shutdown, and
observable failure.

Health diagnostics are wired once through `health.Dependencies` before
`HealthServer.Open`; production code must not use individual health dependency
setters. A component returning to the running state clears its previous safe
degradation reason.

The PVC monitor snapshots state while holding its mutex, releases the lock,
then emits observations or performs persistence I/O. No callback into an
incident or delivery boundary may run while the PVC state lock is held.

Providers and watchers use shared transport and application-owned clients. New
code must not add a second raw HTTP, retry, status-classification, dynamic
informer, or REST-client implementation.

RCA is split by behavior inside `internal/insight`: cause evidence and root
ranking, confidence, impact, recent changes, and next steps each belong in
their semantic file. Keep `engine.go` as orchestration rather than adding new
analysis algorithms there.

Rules of thumb:

- `model` / `event` / `graphcontext` / `constant` / `format` must stay leaf
  packages.
- Provider packages under `alert/` depend on `event`,
  `model`, and shared transport; rich renderers may also use
  `message` and `insight`. Providers must not import `controller`, `handler`,
  `incident`, or Kubernetes clients.
- Providers that talk HTTP call `delivery/transport`; never `net/http` directly
  in production provider code (the architecture check enforces this). The
  transport is where a status code becomes success, rate-limited, permanent or
  retryable — a provider must not have its own opinion.
  Provider constructors receive explicit cluster identity and one
  `transport.Dependencies` value as their typed outbound dependency bundle.
  Providers must not retain `config.App`; the application supplies the HTTP
  client and tests use explicit dependency fixtures.
- SDK-backed providers receive the configured outbound `http.Client` from the
  delivery composition root. They must not import `internal/k8s` or construct
  process-wide clients themselves.
- Nothing outside `incident` calls the delivery manager for incident
  notifications. Monitor runtimes feed `Engine.Process` and stop; the engine announces every decision — live
  events, resolves, group flushes, renotify, mass failures — through `LifecycleHook`, so
  audit, diagnosis and delivery cannot diverge between paths.
- When a hint carries a fact a renderer needs (a memory limit, a probe endpoint, a delay),
  put it in `model.Facts` next to the hint. Renderers read facts; they never parse the hint.
- `model.Incident` is five embedded parts — `Subject`, `Status`, `Evidence`, `Attribution`,
  `Delivery` — each with one writer (see the type comment). Reads are promoted
  (`inc.Reason`, `inc.Count`); composite literals name the part. `PersistedIncident` stays
  flat: it is the on-disk format and must not change shape.
- `monitor/pod/policy.Context` contains only configuration, the object under evaluation,
  injected time, and policy findings. It must not gain clients, listers, event sources,
  log caches, or delivery dependencies.
- `monitor/pod/enrichment.Context` contains policy findings plus explicit enrichment
  sources. New monitor code must convert to policy context for detection and use the
  enrichment package for events, owners, and logs.
- Direct Pod policy receives owner/event lookups through the Pod runtime's
  `ConfigureSources` seam. Direct monitor families receive their own read-only source
  view through the controller-owned `controller.RuntimeSet` family bundle.
- Startup baseline summaries are built and delivered by `internal/app` through
  `startup.BuildSummary`. The controller only records baseline counts and does
  not know about delivery.
- Time-based decisions read an injected clock (`monitor/pod/enrichment.Sources.Now`,
  `Engine.now`, `insight.Engine.now`, `ReportBuilder.now`), not
  `time.Now()` directly, so
  "unready for 5 minutes" is testable without waiting 5 minutes.
- Keep files focused; ~400 lines is the soft ceiling — split by responsibility within the
  same package rather than growing a god file.
- The controller receives its `controller.RuntimeSet`, a set of narrow
  capability interfaces. Do not add methods to a universal interface when a
  family capability is sufficient.
- Run `make architecture-check` when adding a package or moving a dependency;
  `make verify` runs it automatically.
- During incremental work, use `make verify-focused PKGS="./internal/foo/..."`
  to run compile, tests, vet, lint, architecture, layout, and diff checks only
  for the affected package group. Run the full gate at workstream boundaries.
- For a small edit, use `make verify-fast PKGS="./internal/foo/..."` to run
  only package tests, vet, and lint. Run repository-wide checks once after the
  workstream rather than after every file.

Monitor family listers follow one availability contract: a missing lister means
the capability is not ready, so the family skips detection and does not create
or resolve a synthetic incident. The condition must be visible through health
or diagnostics. Source wiring is synchronized and must be complete before a
runtime begins processing.

Persistence consumers should use the narrow contracts in
`internal/persistence/interfaces.go` (`BaselineStore`, `IncidentStore`,
`FeedbackStore`, `ChangeHistoryStore`, and `TelemetryStore`). PVC state uses
the consumer-owned `pvc.StateStore` port. The concrete manager belongs at the
composition root. Legacy `any` APIs are isolated to persisted-format migration
code and are not runtime ports.

Provider identities are defined by the dependency-free provider catalog leaf;
the static alert catalog must have exactly one factory for every identity,
including intentional aliases such as `incidentio` and `incident.io`.

Dynamic Gateway API and storage watchers use `internal/k8s/dynamicwatch` for
discovery, namespace scope, transforms, informer registration, and cache-sync
status. Their graph packages keep resource-specific relationship and failure
semantics in their own callbacks.

## Controller conventions

The controller watches many resource kinds through one abstraction:

- `resourcePipeline` (`pipeline.go`) bundles everything one watched kind needs: a named
  rate-limiting queue, informer sync state, a sync function, and a `startWorkers` flag that
  gates both worker startup and baseline seeding.
- `NewWithRuntimeConfig()` constructs all pipelines; per-kind wiring lives in small
  `wire*` functions
  (`wiring.go`) that attach listers/informers via:
  - `watch(pipeline, informers...)` — registers HasSynced + event handler + starts workers;
  - `listen(pipeline, informers...)` — attaches handlers only (used when sync state is
    tracked separately).
- Sync dispatch functions in `sync.go` share one signature:
  `func (c *Controller) syncX(_ context.Context, key string) error`.
  - Event handlers come from `enqueue.go`: `recordChange` /
    `changeRecordingHandler` for change tracking, plus the graph-aware Pod
    handler.

Graph rebuilds use `graphBuilder` (`graph_builder.go`), a small snapshot of
graph listers and graph state. Do not create a reduced `Controller` copy for
graph construction; Controller owns queues, lifecycle, and informer
diagnostics that are not graph inputs.

**Adding a new monitored resource:**

1. Add a pipeline field to `Controller` and construct it in `New()` with a name matching
   the ChangeTracker label (set `track` explicitly if the label differs from the name).
2. Write a `wire*` function in `wiring.go` using `watch()` (or `listen()`), returning any
   factories the shutdown path needs. It stores the lister on the `Controller`; it does not
   talk to a monitor family.
3. Call it from `New()`, wire its `syncFn`, add it to `allPipelines()`, and
   expose only the lister needed by its family configuration contract. Dispatch
   through the controller-owned `RuntimeSet` capability; do not grow a
   universal interface or add a handler lister setter.
4. If it needs periodic sweeps (like control-plane pods), check `startWorkers` in `Run()`.
5. Add its graph edges in `graph_resources.go` if insight analysis should see it.

## Naming conventions

- `sync*` — workqueue dispatch functions (controller).
- `wire*` / `watch` / `listen` — informer wiring (controller).
- `process*` — workqueue worker entry points.
- `Detect*` — filter/monitor detection entry points returning events or issues.
- `build*` / `extract*` / `apply*` / `prepare*` / `warn*` — small pure-ish helpers extracted
  to keep complexity ≤ 20; prefer these over inline branching when extending logic.
- Table-driven pattern lists (see `imagePullPatterns`) beat long switch chains.

## Behavior-preservation notes

Some quirks are load-bearing. Preserve them unless a change explicitly says otherwise:

- `wirePDB` awaits every PDB informer's HasSynced so baseline seeding never
  runs against a partially populated namespace cache.
- StatefulSet listers are always wired and their sync awaited even when monitoring is off;
  only workers/listeners are gated.
- Severity map keys (`SeverityByOwnerKind`, `SeverityByReason`) must be preserved verbatim —
  never `strings.Title` them (breaks multi-word kinds like `DaemonSet`).
- Exit code 137 with a reason other than `OOMKilled` is a plain SIGKILL, not an OOM.
- Suppression consolidation: deprecated `ignore*` fields become synthetic `SilenceRule`s in
  `appendIgnoreFieldSilences`; keep both paths reading the unified index.
- `Engine.processLocked` runs five stages in a fixed order — baseline, attribution (node →
  shared dependency → owning workload), cooldown, identity, announcement. Attribution comes
  *before* cooldown on purpose: a pod whose key is cooling down is still its owner's symptom
  and must keep being counted against it. Add a new kind of cause to `attribution.go`, not
  as a new check in `processLocked`.
- The audit skip reasons `baseline`, `node_inhibition`, `mass_failure`,
  `cascading_suppression`, `cooldown` are stable strings people grep for.

## Extension contract

New user-visible modules must be added deliberately. A monitor, provider, or
integration is not complete when its package compiles. The change must include:

1. A domain-specific descriptor and stable name.
2. Explicit composition-root wiring.
3. Configuration and validation, when configurable.
4. Structured logs, bounded-cardinality metrics, and health behavior.
5. Deterministic unit tests and integration tests where Kubernetes semantics
   matter.
6. Generated reference metadata.
7. User, operator, and contributor documentation as applicable.
8. Migration and release notes for changed behavior.

The monitor registry is metadata, not a service locator. Do not hide runtime
wiring in a global registry.

### Monitor family boundaries

Keep resource-specific detection in cohesive monitor families rather than in
one detection god object or one package per Kubernetes resource. The preferred
families are pod/container, workload, node, network, security, and storage.
Concrete detector packages under `internal/monitor` currently cover pod,
workload, node, network, and security. Telemetry, control-plane, probe, and
storage monitors remain in existing cohesive packages until a typed boundary
adds clarity without duplicating their lifecycle wiring. Existing cohesive
packages such as `pvc`, `probe`, `controlplane`, and `statuswatch` may remain
where they already own a clear lifecycle.

Family modules use small typed dependencies and injected clocks. They may
produce observations or use a narrow incident sink, but they must not receive
the delivery manager, write persistence directly, or import application
composition. Do not create one universal monitor interface with every
resource operation; different families have different inputs and lifecycles.

The workload slices are the reference migration: the controller uses the
family-owned runtimes in `monitor/workload` directly through the matching
`RuntimeSet` capability and configures all workload listers in one operation
through `workload.SourceConfig` and `workload.Sources`. Deployment, ReplicaSet,
Job, DaemonSet, StatefulSet, CronJob, HPA, and PDB queue processing now use
direct family wiring in production. Canonical family wiring uses one
error-returning `ConfigureSources` operation; resource-specific lister setup is
private to the workload package and is not an exported production seam.
The aggregate workload capability has been removed. New controller wiring must
use the specific family capability for each resource kind.

Node, network, cluster-resource, and Pod queue processing follows the same
direct family wiring pattern through `monitor/node`, `monitor/network`,
`monitor/cluster`, and `monitor/pod`. Cluster listers are passed through one
`cluster.SourceConfig`/`cluster.Sources` operation. The Pod runtime owns queue lookup,
policy evaluation, reference checks, deletion recovery, and source wiring.
New production paths must use the family capability and its narrow source
configuration interface.

Network and security runtimes use the same source-bundle rule. The controller
assembles one `network.Sources` or `security.Sources` value and applies it
through the family's `SourceConfig` seam. Do not restore positional lister
setters or add family-crossing policy dependencies.

The same one-time source rule applies to TLS, control-plane, probe, metrics,
kubelet metrics, PVC, status, graph, and RBAC integrations. Their canonical
source bundles are configured before processing starts; source mutation after
startup is rejected.

Optional Gateway API, storage, status, and KwatchConfig resources share
informer construction and transform mechanics through
`internal/k8s/dynamicwatch`. Domain packages retain their own graph or status
semantics. `crdwatch` remains separate because it owns late-install and
restart behavior for KwatchConfig resources.

The deployment runs two Kwatch replicas by default with Kubernetes Lease leader
election. Exactly one replica owns observation, delivery, and mutable
persistence; all other replicas are standby and do not start active monitor
workers. A one-replica override is supported for constrained clusters but has
no Kwatch self-failover. Do not add active-active processing without a separate
deduplication and persistence design.

### Provider checklist

Provider adapters live under `internal/alert/<provider>`, static construction
lives in `internal/alert/catalog`, while routing,
retries, rate limits, status classification, and transport live under
`internal/delivery`. Every provider implements the canonical context-aware
contract:

```go
SendEvent(context.Context, *event.Event) error
SendMessage(context.Context, string) error
```

A provider must validate configuration, render its own payload, call
`delivery/transport`, and return errors. It must not
import Kubernetes or orchestration packages, construct a process-wide HTTP
client, decide retry policy, or log credentials, secret-bearing URLs, or
complete payloads. Rich incident/thread capabilities use the same caller
context. New providers require catalog metadata, focused HTTP/error tests,
user documentation, and release notes when behavior changes.

The application passes `catalog.NewProvider` to
`delivery.Manager.InitRuntime`. Delivery must not import concrete provider
packages. Production and test composition pass `RuntimeConfig` directly.

The delivery manager invokes providers directly through the canonical
context-aware contract. There is no legacy provider adapter or context bridge.
HTTP providers receive the application-owned client explicitly; transport
selection must not be hidden in a context value or package global.

Provider generations are the only production provider storage. Lookup and
fallback use stable names and deterministic order; no code may retain a
pointer into a mutable provider slice. A reconfiguration must stop the old
generation before its channels can be reclaimed.

Queued deliveries retain the generation that accepted them. Fallback lookup
for an in-flight job uses that immutable generation, so reconfiguration cannot
silently change the job's fallback route.

Fallback cycles are rejected at generation construction by disabling the
fallback that closes the cycle. This keeps fallback dispatch bounded and
ensures future recursive fallback changes cannot loop indefinitely.

Slack token API calls and Discord webhook execution are the only approved SDK
transport exceptions. They use injected HTTP clients and caller contexts;
generic retry and status policy remains in `delivery/transport`. See
`docs/adr/0005-sdk-provider-transport-exceptions.md` before adding another
exception.

The PVC monitor copies a namespace-filter callback while holding its mutex but
invokes that callback only after unlocking. Observations, resolutions, and
persistence I/O likewise happen outside the PVC state lock.

Health diagnostics expose bounded component state and safe reason codes rather
than arbitrary error strings. Persistence exposes a complete migration report
for the startup cycle, and dynamic watcher status is tied to a generation so a
stale watcher cannot clear current readiness or degradation state.

Monitor source availability is exposed through controller diagnostics as
`unavailableSources` and a bounded source-unavailable metric. A missing active
lister means that capability is not
ready: the family skips detection and does not create or resolve a synthetic
incident. Disabled pipelines are omitted from this diagnostic list.

Dynamic watcher status includes skipped optional resources and cache-sync
failures. CRD discovery failures are reported to the application health
boundary through the status sink injected during construction; waiting for a
missing CRD is a normal degraded/waiting state, not a process restart
condition.

Persistence migrations return structured `MigrationResult` values and retain
the complete startup-cycle `MigrationReport`. Migration status and failures
contribute to bounded SRE metrics; persisted keys and formats remain
compatibility-bound.

Health diagnostics expose bounded component state and safe reason codes. Raw
error strings remain in logs only. `/healthz` is liveness, `/readyz` describes
required monitoring readiness, and optional API/source failures remain visible
through `/health` without making readiness fail.

## Observability contract

- Use structured `klog` fields consistently; include `component` and
  `operation` at boundaries and resource/incident/provider identity when safe.
- Never log credentials, tokens, webhook URLs containing secrets, or complete
  provider payloads by default.
- Use `metrics.DefaultRegistry()` for internal metrics.
- Metric labels must be bounded. Do not use pod names, incident IDs, URLs, or
  arbitrary error strings as labels.
- Readiness describes whether Kwatch can perform its monitoring function;
  liveness describes whether the process is alive.
- Every retryable external failure must have an observable retry count and
  final failure signal.
- Delivery dead letters, queue saturation, optional API unavailability,
  watcher synchronization, component degradation, persistence migration
  failures, and shutdown timeouts have bounded counters in the default
  registry. Add labels only when their cardinality is fixed and documented.

## Documentation contract

Published documentation is canonical in `kwatch.dev`. The main repository
owns code-derived generators, `AGENTS.md`, and a short `CONTRIBUTING.md` entry
point. Do not maintain a second manually edited copy of public configuration,
feature, provider, CLI, RBAC, metrics, or architecture documentation.

Use these documentation types:

- Tutorials teach a complete first success.
- How-to guides solve one operator or contributor task.
- Reference pages state exact behavior and are generated when possible.
- Explanation pages describe concepts, trade-offs, and architecture.
- Operations pages describe production failure modes and recovery.

Documentation is part of the definition of done. Code changes that alter
behavior, configuration, persistence, observability, security, or extension
contracts require a documentation review.

## Refactoring contract

Refactors must proceed in seams that compile and test independently. The
runtime domain packages are `internal/incident`, `internal/delivery`, and
`internal/persistence`; do not recreate the retired correlation, alert-manager,
or state-manager package boundaries. Verify imports with `rg` and run the
architecture check after package changes.
Persisted state changes require a schema version, migration or explicit reset
path, recovery guidance, and tests.

Do not use a broad mechanical rewrite to conceal domain changes. Preserve
load-bearing behavior listed above unless an ADR explicitly approves a new
contract.

## Working in the current tree

Inspect `git status` before editing. Existing dirty changes belong to the user;
preserve them and review overlapping diffs before applying a refactor. Never
reset, clean, or overwrite unrelated work. The `/Users/macos/kwatch.dev`
repository is independent and must not be edited as part of code changes.

This project has not reached a stable public API release. Do not add internal
compatibility constructors, setters, aliases, or wrappers merely to preserve
old tests or historical call paths. Migrate callers to the canonical contract
and remove the obsolete seam. Persisted data is different: preserve its format
through explicit versioned migrations, backups, and round-trip tests.

Before adding a long-running component, define its owner, required or optional
classification, cancellation path, completion handle, startup and shutdown
deadlines, progress signal, health reason, metrics, and deterministic tests.
Before adding a provider or monitor, follow the extension checklists below and
run the smallest focused package validation first.

## Production-grade guardrails

Kwatch is operated with two replicas and Lease election by default. Only the
leader runs monitoring, delivery, and mutable persistence; standby Pods keep
health and election alive so one failed leader can be replaced. A one-replica
`Recreate` deployment remains an advanced low-resource mode without
self-failover. Election does not protect against total cluster, node, API, or
network failure and does not promise exactly-once external delivery.

Production readiness means that required readiness, bounded shutdown, safe
persistence recovery, Kubernetes and provider outage behavior, bounded queues,
safe diagnostics, reviewed RBAC, signed release artifacts, and upgrade/rollback
procedures are tested and documented. Do not claim zero-loss delivery,
zero-downtime upgrades, or HA without evidence and an approved design.

The default deployment uses a 60-second termination grace period and a
multi-replica PodDisruptionBudget with `minAvailable: 1`. The chart renders the
budget only when more than one replica is configured; one-replica deployments
remain supported without self-failover.

The application supervisor treats an unexpected component return as a failure.
Required components have bounded startup and stall deadlines; components that
can be idle must still report lifecycle progress. A required failure or stall
removes readiness, cancels the active generation, fences delivery and
persistence, releases leadership, and lets Kubernetes restart the Pod.
Optional components retry with `1s, 2s, 4s, 8s` backoff capped at `60s`; the
backoff resets after a minute of healthy execution. Component completion is
awaited during shutdown instead of being inferred from sleeps.

The application composition root owns Kubernetes, dynamic, discovery, REST,
HTTP, DNS, kubelet, and clock construction. Domain constructors receive narrow
explicit dependencies. Production code must not silently use default clients,
resolvers, nil clocks, locally constructed clients, or clients stored in
contexts.

`RuntimeConfig` is the only production runtime configuration boundary. The YAML
model remains external and stable; derived policies are compiled once. Runtime
views and accessors return detached values. Delivery and integrations must not
reparse raw configuration or add a second normalization path.

Family sources are configured once before processing starts. A missing lister or
source means that capability is unavailable: skip detection and do not create or
resolve a synthetic incident. Report the condition using bounded reason codes.
Disabled pipelines must not report degradation. Compatibility setters, when
they exist, are not production wiring points.

The application owns component goroutines and shutdown. Every worker, ticker,
retry timer, informer, and watcher needs an owner, cancellation path, bounded
wait, and observable failure behavior. Health owns only its HTTP listener.
Shutdown must stop producers before final persistence and must respond to both
signals and parent-context cancellation.

Watcher replacement is synchronous at the lifecycle boundary: cancellation is
issued first and the previous generation completion handle is awaited before a
replacement is published. CRD discovery and informer goroutines participate in
the same completion boundary. Provider generations are built through runtime
initialization; there is no production late-registration mutation seam. Runtime
changes replace the complete immutable provider generation and in-flight
fallback stays within the generation that accepted the job.

Canonical dynamic and CRD watcher stop operations accept a caller-owned
context and return a bounded error. Domain monitor `Stop` methods must not hide
watcher shutdown errors; application-owned lifecycle wrappers record them as
safe `shutdown_timeout` or `component_failed` states.

Health and public diagnostics expose safe status codes only. Never expose raw
errors, credentials, tokens, complete payloads, secret-bearing URLs, incident
internals, or arbitrary Kubernetes object data. Metrics use fixed bounded labels
and transition-based counters.

Provider generations are immutable and use stable names for lookup and fallback.
Providers validate, render, and call shared transport; delivery owns retries,
status classification, pacing, and queue policy. Slack and Discord SDK calls
are the only approved transport exceptions and require the rules in
`docs/adr/0005-sdk-provider-transport-exceptions.md`.

Provider runtime initialization validates the complete configured generation
before replacing an active generation. Unknown or unconstructable configured
providers return an error to the composition root; fallback lookup remains
generation-bound and missing fallback names are disabled deterministically.
Provider construction is the only supported registration path; tests replace
the complete generation rather than mutating provider storage.

Persisted formats are compatibility contracts. Any persisted change requires an
explicit schema version, migration or reset path, backup/recovery guidance, and
round-trip tests. Migration diagnostics must report every startup operation with
safe bounded outcomes.

## Current lifecycle safeguards

Application readiness is coordinated per leadership epoch. A leader is not ready
until restore, required informer sources, required persistence writers, the
incident engine, and configured delivery are all active. Standby replicas remain
live for election and health serving but are never ready for monitoring. A stale
callback from an earlier epoch must not restore readiness or clear a newer failure.

Delivery generations have explicit `accepting`, `draining`, `stopped`, and
`failed` states. Reconfiguration publishes a new generation only after the old
one has stopped. A successful replacement is not a component failure; a failed
drain is surfaced to the required delivery supervisor. Manager shutdown and
generation replacement have separate completion signals.

Final persistence writes are created by the application shutdown coordinator with
a bounded context and must pass the current leadership/write fence immediately
before I/O. Saver loops use their active generation context; they must not create
detached background writes that can outlive shutdown or leadership loss.

CRD and dynamic watcher generations own their discovery and informer goroutines.
Failure paths cancel and wait for those routines before retrying or replacing a
generation. Public status uses safe reason codes only. A new watcher must not be
published while an old generation can still deliver callbacks or clear status.

Provider payload limits are explicit catalog policy. Providers with protocol
limits use deterministic truncation; providers whose limits are owned by their
renderer are marked `provider_owned` rather than receiving an arbitrary default.
Every provider must have a policy entry, and payloads and secret-bearing details
must never be logged in full.

## Change checklists

When changing a monitor, provider, filter, RCA rule, persistence field,
configuration field, metric, integration, RBAC rule, or deployment manifest:

1. Use the existing ownership seam and do not introduce an upward dependency.
2. Add deterministic focused tests for success, failure, cancellation, and
   recovery where applicable.
3. Run `make architecture-check`, catalog checks, test-layout checks, and
   `git diff --check` after the workstream rather than after every edit.
4. Update generated catalogs or manifests when their source metadata changes.
5. Review health, metrics, logging, redaction, and shutdown behavior.
6. Review configuration, migration, upgrade, rollback, and release-note impact.
7. Update the canonical website through its separate reviewed synchronization
   workflow; do not treat local documentation as a second public source.

For monitor and integration changes, verify source configuration, optional API
degradation, missing-lister behavior, namespace filtering, and readiness.
For provider changes, verify context cancellation, transport classification,
fallback generation, payload limits, redaction, and catalog registration.
For persistence changes, verify old formats, corruption, conflicts, trimming,
partial writes, recovery, and complete migration reports.
For deployment or RBAC changes, render and lint the chart, inspect the raw
manifest, review least privilege, and document operational consequences.

Use focused validation for local work:

```sh
make verify-fast PKGS="./internal/changed/package/..."
make verify-focused PKGS="./internal/changed/package/..."
```

Run targeted race tests for concurrency changes. Run the full race suite,
security scans, Kind tests, outage tests, and release verification at completed
milestones or final handoff. Record unavailable tools instead of treating them
as passed.

## Final operational safeguards

The production Kind harness distinguishes monitoring readiness from deployment
availability: `/readyz` is successful only for the active Lease holder, while
`/availabilityz` lets every elected leader or standby participate in a safe
Deployment rollout. Rollout checks wait for all expected Pods to be running and
assert that the Lease holder is the active monitoring-ready Pod.

Managed installs rewrite the Lease name with the release identity so separate
managed installations cannot share a Lease. Direct raw-manifest installs use
the manifest's explicit installation identity and must change it when multiple
installations share a namespace.

Dynamic discovery dependencies must implement client-go's context-aware
discovery interface. Do not add a fallback to an unbounded, contextless
discovery call; discovery must stop with its watcher generation.

Protected diagnostics, including informer, persistence, security, kubelet,
control-plane, incident, dead-letter, test-alert, and pprof endpoints, require
the configured bearer token. Liveness, readiness, health, and metrics remain
separate public operational endpoints. Empty diagnostic credentials must never
turn a protected handler into an anonymous endpoint.

Required persistence savers report periodic progress even when their input is
idle. New periodic components must either expose equivalent progress or be
explicitly classified as idle-safe; otherwise the supervisor cannot distinguish
healthy idleness from a stalled component.

Standby replicas may construct immutable dependencies and serve health/election,
but they must not restore incidents, groups, baselines, provider threads, engine
state, or other mutable monitoring state before acquiring the Lease. Active
restore, required cache synchronization, source configuration, and saver startup
belong to the current leadership epoch. A restore failure keeps that epoch
not-ready and prevents monitoring and delivery from starting.

Migration reports distinguish restore, migrate, recover, write, and initialize
operations. Do not add an ambiguous duplicate result for the same store without
recording the operation phase explicitly. Legacy and future persisted formats
must remain preserved and diagnosable.

Kubernetes list/watch code must handle reflector relists, expired resource
versions, watch closure, tombstones, handler panics, queue retries, and API
throttling without creating duplicate workers or synthetic incidents. Required
cache loss removes readiness; optional API loss degrades health only.

Production controller construction receives its application lifecycle context
through `controller.RuntimeDependencies`. Compatibility callers may use the
bounded fallback, but new production code must not introduce unbounded
`context.Background()` calls for namespace resolution, watcher shutdown,
transport, or persistence writes.

The security workflow pins scanner and SBOM container images by digest and
publishes source and image CycloneDX SBOMs. The release workflow also generates
SBOMs for the exact release tag and image digest, signs the release checksum
manifest with Cosign, verifies image provenance, and scans the published image
digest. Vulnerability exceptions must be time-bounded and documented according
to `docs/vulnerability-exceptions.md`. OpenSSF Scorecard runs separately.
When local Docker, Kind, kubectl, ShellCheck, or actionlint are unavailable,
the corresponding CI checks remain mandatory and must be reported as pending,
never as locally passed.
