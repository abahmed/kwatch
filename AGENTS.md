# AGENTS.md — conventions for working on kwatch

Guidance for humans and AI agents making changes to this repository. For user-facing
contribution process, see [CONTRIBUTING](./CONTRIBUTING.md); for how the product behaves,
see [docs/architecture.md](./docs/architecture.md).

## The gate

Every change must pass before you are done:

```sh
go build ./... && go vet ./... && go test ./... && golangci-lint run
```

The repository also enforces formatting and line length. Run the complete gate
with `make verify`; it includes `line-check` and `git diff --check` should be
clean before handoff.

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
- Keep every Go file below 400 lines. When a file grows, split it by
  responsibility and use semantic names such as `conditions.go` or
  `payload_limits_test.go`, not anonymous numeric fragments.
- Match the package name to the final directory component. For example,
  `internal/graphcontext` must declare `package graphcontext`.
- Add short comments only where they explain an invariant, compatibility rule,
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
  or dependency seam for mutable process state; retain compatibility aliases
  only when removing them would break external users.
- Use `metrics.DefaultRegistry()` at internal call sites. Do not introduce new
  direct uses of `metrics.Default` or hidden singleton clients.
- Construct shared Kubernetes or HTTP clients in `internal/app` and pass them
  into monitors, providers, and integrations that need them.
- Import `internal/graphcontext` with an explicit alias when the standard
  library `context` is also in scope; never disguise a package-name mismatch.
- Do not duplicate provider transport or retry logic. Providers build payloads
  and call `alert/util.Send`; shared utilities decide status classification,
  timeout, retry, and rate-limit behavior.
- Preserve persistence formats and public constructor compatibility where
  possible. Prefer optional dependencies or adapters over breaking call sites.

### Naming standard

- Use `New<Type>` for constructors and `Set<Type>` for optional wiring. Keep
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
  identifier; add a compatibility wrapper when a rename is unavoidable.
- Use `camelCase` for local names, `PascalCase` for exported names, and avoid
  redundant package prefixes such as `config.ConfigManager`.
- Use `Test<Type><Behavior>` for tests. Name table cases by behavior, not by
  implementation order or issue number.

### Test and refactor directives

- Before splitting a test file, identify package-level fixtures, helper types,
  and imports. Copy required declarations into the correct focused file and
  run that package's tests immediately after the split.
- Keep tests deterministic: use injected clocks and fake clients rather than
  sleeps, wall-clock assertions, or live network calls.
- Add or update focused tests for every behavior change, especially lifecycle
  transitions, suppression decisions, retries, persistence, and compatibility.
- Do not mechanically rewrite unrelated files. Review `git diff` after each
  refactor and preserve user changes already present in the worktree.
- Do not remove a legacy package or alias until `rg` confirms there are no
  remaining imports and the full repository gate passes.

## Package map

Dependency direction flows downward; never import upward.

| Package | Responsibility |
|:--|:--|
| `cmd/kwatch` | Thin entrypoint: flag parsing, subcommand dispatch, calls `app.Run()` |
| `internal/app` | Composition root: builds config, controller, correlator, persistence and runs them |
| `internal/controller` | Informer wiring, workqueues/pipelines, cluster graph, baseline seeding |
| `internal/handler` | Turns raw Kubernetes objects into candidate incidents (filters + hints) |
| `internal/filter` | Detect-time suppression filters (pod status, owners, reasons) |
| `internal/config` | Config loading/validation, suppression index builder |
| `internal/correlation` | Incident lifecycle (create/update/resolve/skip), attribution, grouping, cooldowns. The **only** emitter of notifications (`emit.go`) |
| `internal/insight` | Cause/impact/recent-change analysis over the dependency graph |
| `internal/event`, `internal/graphcontext`, `internal/model` | Shared types |
| `internal/alert/*` | One subpackage per provider + routing/retry/ratelimit plumbing |
| `internal/state`, `internal/startup`, `internal/upgrader` | Crash-safe persistence in ConfigMaps, baseline, upgrades |
| `internal/{pvc,heartbeat,health,audit,crdwatch,integration}` | Periodic watchdogs and integrations |
| `internal/k8s`, `internal/client`, `internal/resource` | Kubernetes access helpers |

Rules of thumb:

- `model` / `event` / `graphcontext` / `constant` / `format` must stay leaf
  packages.
- Provider packages under `alert/` depend on `event`, `model`, `config`, and
  `alert/util`; rich renderers may also use `message` and `insight`. Providers
  must not import `controller`, `handler`, `correlation`, or `k8s` clients.
- Providers that talk HTTP call `alert/util.Send`; never `net/http` directly (the linter
  enforces this). Send is where a status code becomes success, rate-limited, permanent or
  retryable — a provider must not have its own opinion.
- Nothing outside `correlation` calls `AlertManager.NotifyIncident` for incidents. The
  handler feeds `Engine.Process` and stops; the engine announces every decision — live
  events, resolves, group flushes, renotify, mass failures — through `LifecycleHook`, so
  audit, diagnosis and delivery cannot diverge between paths.
- When a hint carries a fact a renderer needs (a memory limit, a probe endpoint, a delay),
  put it in `model.Facts` next to the hint. Renderers read facts; they never parse the hint.
- `model.Incident` is five embedded parts — `Subject`, `Status`, `Evidence`, `Attribution`,
  `Delivery` — each with one writer (see the type comment). Reads are promoted
  (`inc.Reason`, `inc.Count`); composite literals name the part. `PersistedIncident` stays
  flat: it is the on-disk format and must not change shape.
- `filter.Context` is `Sources` (read-only lookups the handler sets once) + the object under
  evaluation + `Findings` (what detectors concluded). A filter never writes `Sources`.
- The handler gets its informer-backed lookups in one `handler.Listers` value via
  `SetListers`, after all informers are wired. Do not add per-lister setters back to the
  `Handler` interface; a nil lister means "that monitor is off".
- Time-based decisions read an injected clock (`filter.Sources.Now`, `handler.now`,
  `Engine.now`, `insight.Engine.now`, `ReportBuilder.now`), not `time.Now()` directly, so
  "unready for 5 minutes" is testable without waiting 5 minutes.
- Keep files focused; ~400 lines is the soft ceiling — split by responsibility within the
  same package rather than growing a god file.

## Controller conventions

The controller watches many resource kinds through one abstraction:

- `resourcePipeline` (`pipeline.go`) bundles everything one watched kind needs: a named
  rate-limiting queue, informer sync state, a sync function, and a `startWorkers` flag that
  gates both worker startup and baseline seeding.
- `New()` constructs all pipelines; per-kind wiring lives in small `wire*` functions
  (`wiring.go`) that attach listers/informers via:
  - `watch(pipeline, informers...)` — registers HasSynced + event handler + starts workers;
  - `listen(pipeline, informers...)` — attaches handlers only (used when sync state is
    tracked separately).
- Sync dispatch functions in `sync.go` share one signature:
  `func (c *Controller) syncX(_ context.Context, key string) error`.
- Event handlers come from `enqueue.go`: `recordChange` / `changeRecordingHandler` for
  change tracking, plus the graph-aware pod handler.

**Adding a new monitored resource:**

1. Add a pipeline field to `Controller` and construct it in `New()` with a name matching
   the ChangeTracker label (set `track` explicitly if the label differs from the name).
2. Write a `wire*` function in `wiring.go` using `watch()` (or `listen()`), returning any
   factories the shutdown path needs. It stores the lister on the `Controller`; it does not
   talk to the handler.
3. Call it from `New()`, wire its `syncFn`, add it to `allPipelines()`, and add the lister to
   the single `handler.Listers{...}` value `New()` hands to `SetListers` (plus a field on
   `handler.Listers` itself).
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

- `wirePDB` awaits only the **first** PDB informer's HasSynced (historical behavior).
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
