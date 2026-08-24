# AGENTS.md — conventions for working on kwatch

Guidance for humans and AI agents making changes to this repository. For user-facing
contribution process, see [CONTRIBUTING](./CONTRIBUTING.md); for how the product behaves,
see [docs/architecture.md](./docs/architecture.md).

## The gate

Every change must pass before you are done:

```sh
go build ./... && go vet ./... && go test ./... && golangci-lint run
```

- Linters: errcheck, gocritic, gocyclo, govet, ineffassign, unparam, unused (`.golangci.yml`).
- **Cyclomatic complexity limit is 20** (`gocyclo min-complexity: 20`), tests included. When a
  function exceeds it, extract helpers or table data instead of raising the threshold.
- Formatting: `goimports` with `local-prefixes github.com/abahmed/kwatch` (stdlib first,
  third-party second, kwatch last).
- Test files are exempt from errcheck/unparam/gocritic/gocyclo; `internal/controller` is
  exempt from errcheck (informer wiring intentionally ignores AddEventHandler returns).

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
| `internal/correlation` | Incident lifecycle (create/update/resolve/skip), grouping, cooldowns |
| `internal/insight` | Cause/impact/recent-change analysis over the dependency graph |
| `internal/event`, `internal/context`, `internal/model` | Shared types |
| `internal/alert/*` | One subpackage per provider + routing/retry/ratelimit plumbing |
| `internal/state`, `internal/startup`, `internal/upgrader` | Crash-safe persistence in ConfigMaps, baseline, upgrades |
| `internal/{pvc,heartbeat,health,audit,crdwatch,integration}` | Periodic watchdogs and integrations |
| `internal/k8s`, `internal/client`, `internal/resource` | Kubernetes access helpers |

Rules of thumb:

- `model` / `event` / `context` / `constant` must stay leaf packages.
- Provider packages under `alert/` only depend on `model`, `message`, `config`, and `alert/util`.
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
   factories the shutdown path needs.
3. Call it from `New()`, wire its `syncFn`, add it to `allPipelines()`.
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
