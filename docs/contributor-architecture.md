# Contributor architecture guide

This is the source-tree guide. Read [How kwatch works](./architecture.md)
first for behavior. Published tutorials and runbooks live at
[kwatch.dev/docs](https://kwatch.dev/docs). The dependency rules below are
enforced by `scripts/check-architecture.sh` (`make architecture-check`).

## Packages

Each line matches the package's `doc.go`.

| Package | What it does |
| --- | --- |
| `cmd/kwatch` | Thin entrypoint: flags, subcommands, calls `app`. |
| `internal/app` | Assembles kwatch and owns its process lifecycle: clients, Lease, supervision, readiness, shutdown. |
| `internal/inventory` | The in-memory model of the cluster: entities, relations, attributes and recent changes. Knows nothing about Kubernetes. |
| `internal/inventory/kube` | Turns Kubernetes objects into observations: one schema per kind, discovery, watch modes, kubelet stats, probes, log reads. |
| `internal/inventory/kube/dynamicwatch` | Shared dynamic informer mechanics for discovered kinds. |
| `internal/detection` | Finds abnormal conditions (findings with health and mode); the tracker reports raised, changed and cleared. |
| `internal/detection/detectors` | The built-in detectors. Read entity attributes, return findings. |
| `internal/detection/reasons` | Stable reason codes. A leaf, shared by every layer. |
| `internal/rootcause` | What the root-cause engine shares with the layers above it: the `Cause` record, confidence levels, scheduler-message helpers. |
| `internal/rootcause/explain` | The engine: propagation table, candidate walk, scorers, set-cover solver, incremental cache. |
| `internal/incident` | Groups findings that share a cause into an incident and decides when people hear about it: announce, update, resolve. |
| `internal/notification` | The provider-neutral `Message`, severities and rendering helpers. A leaf. |
| `internal/notification/compose` | Writes the note for an incident decision as a short narrative. |
| `internal/scope` | Namespaces, reasons, silences and maintenance holds: which findings are in scope. |
| `internal/pipeline` | The decision loop, bounded investigation and storage workers, downtime reconciliation, audit entries. |
| `internal/pipeline/announce` | Startup summary, digest, roll-up and namespace outage collecting. |
| `internal/pipeline/coverage` | Memory and timings of the coverage backstop; its `doc.go` explains the whole check. |
| `internal/pipeline/investigate` | Evidence investigators run by the investigation pool. |
| `internal/storage` | The bbolt state file: one bucket per data class, epoch fencing, retention, reset. |
| `internal/delivery` | Routing, retries, pacing, fallback and provider dispatch. |
| `internal/delivery/transport` | The shared outbound HTTP boundary providers use. |
| `internal/delivery/api` | The small contracts shared by delivery and the provider catalog. |
| `internal/delivery/providertest` | Provider test fixtures. Never imported by production code. |
| `internal/delivery/signing` | AWS request signing for providers that talk to AWS. |
| `internal/alert/*` | One adapter per provider: builds the payload, sends it through transport. |
| `internal/alert/catalog` | Statically links provider adapters and constructs them. |
| `internal/provider/catalog` | Provider identities and metadata. |
| `internal/health` | `/healthz`, `/readyz`, `/availabilityz`, `/health` and `/metrics`. |
| `internal/metrics` | The process-wide Prometheus registry; labels are always bounded. |
| `internal/config` | Loads, normalizes and validates configuration. |
| `internal/config/crd` | Watches `KwatchConfig` resources, including late installs. |
| `internal/rbac` | Audits the permissions kwatch uses and reports what is missing. |
| `internal/kubeclient` | Kubernetes client construction and informer helpers. Only `app` constructs clients. |
| `internal/redact` | Removes secrets from text before kwatch keeps or sends it. A leaf. |
| `internal/format`, `internal/ratelimit`, `internal/clock` | Pure text helpers, the rate-limit error type, the time dependency. |
| `internal/audit` | One JSON line per incident decision, for offline scoring. |
| `internal/replay` | Records and replays observations on a simulated clock. Test tooling only. |
| `internal/scenarios` | Labelled scenario library and the scorecard gate. Tests only. |
| `internal/scorecard` | Measures notification quality against the production goals. |
| `internal/feature`, `internal/heartbeat`, `internal/telemetry`, `internal/upgrader`, `internal/version` | Feature identifiers, the external heartbeat ping, adoption telemetry, release check, build version. |
| `internal/architecture` | Tests that enforce repository layout. No production code. |

## Import direction

The flow is the dependency direction. Lower layers never import upper ones.

```text
inventory → detection → rootcause → incident → notification/compose
          → pipeline → app
```

The script enforces these rules:

- `inventory` imports none of detection, rootcause, incident, notification,
  pipeline, storage, scope, delivery, alert, app, health or rbac. Only
  `inventory/kube` may import Kubernetes libraries.
- `detection` does not import rootcause, incident, notification, pipeline,
  storage, scope, delivery, alert or app. `rootcause` does not import
  incident or anything above it. `incident` does not import notification,
  pipeline, storage, scope, delivery, alert or app. `notification/compose`
  does not import pipeline, storage, scope, delivery, alert or app.
- `storage` imports no domain package. `scope` imports nothing at or above
  rootcause, and `rbac` nothing at or above detection.
- `pipeline` does not import delivery, alert, app, health or config. Only
  `app` (and `replay`, which is test tooling) imports `pipeline`. Its
  subpackages `announce`, `coverage` and `investigate` never import
  `pipeline` or each other; the pipeline wires them.
- Leaf packages (`notification`, `format`, `redact`, `ratelimit`,
  `detection/reasons`) import no upper layer.
- `alert/*` and `delivery` import no domain package, no `kube`, no
  Kubernetes client library and not `app`. Providers never use `net/http`
  directly; they call `delivery/transport`, which decides status
  classification, timeout and retry.
- Clients are built in `internal/kubeclient` and `internal/app` only. Dynamic
  informers are built only in `dynamicwatch`. `config.Config` is read only
  by `config`, `app` and commands. Production code does not call `time.Now`
  (use an injected clock), `http.DefaultClient`, `net.DefaultResolver`,
  `context.WithValue` or dynamic Prometheus label values.
- Retired packages (`controller`, `insight`, `persistence`, `handler`,
  `monitor`, `model`, `message` and others listed in the script) must not
  return.

## Where to put new code

| You want to | Put it in | Guide |
| --- | --- | --- |
| Detect a new abnormal state | `detection/detectors`, reason in `detection/reasons`, mode in `detection/health.go`, registered in `app/pipeline.go` | [detector](./contributing-detector.md) |
| Teach root cause a new way failure spreads | A row in `rootcause/explain/table_rows*.go` | [propagation rule](./contributing-propagation-rule.md) |
| Watch a new kind or add a source | `inventory/kube` (schema, registration, watch mode) | [source](./contributing-source.md) |
| Say a new fact in the message | A sentence writer in `notification/compose` | [message fact](./contributing-message-fact.md) |
| Prove a behavior end to end | A labelled scenario in `internal/scenarios` | [scenario](./contributing-scenario.md) |
| Add a provider | `internal/alert/<provider>`, catalog metadata, transport tests | below |
| Change the state file | `internal/storage`, bump `SchemaVersion` | below |

Keep clients, queues and cache sync in the source. Detectors and rules see
only the model and an injected clock. Do not add a method to a universal
interface; add a small function or table row instead.

## Providers

An adapter validates settings, renders its payload and calls
`delivery/transport` with the application-owned client. It does not classify
HTTP status, retry, rate-limit, build Kubernetes clients or log full
payloads. Use semantic files (`config.go`, `payload.go`, `incident.go`). Add
payload, error, cancellation, redaction and size tests, then update the
provider catalog and the website reference.

## Persistence

Keep persisted records flat. A format change bumps `storage.SchemaVersion`.
Because an unreadable or mismatched file is renamed to `state.db.corrupt`
(one copy, replaced by a later reset) and a fresh one is created, add a
test that an old-version file is reset and reported, and a release note.

## Test and verify

Name tests after behavior. Split large test files by responsibility
(`queue_coalesce_test.go`), with shared setup in `fixtures_test.go`. Use fake
clocks and fake clients, not sleeps. Before handoff run:

```sh
make verify
```

`make verify` runs the test suite once, with `-race` and coverage, and the
alert-quality gates run inside that run. `make ci` is what the required `CI`
workflow runs: `make verify` plus actionlint, ShellCheck and a `go mod tidy`
drift check. Locally, a check whose tool is not installed is skipped with a
notice; in CI it fails. `make alert-quality` prints the alert-quality report
without failing, and `make alert-quality-gate` runs only those gates.
Real-cluster suites run in the `E2E` workflow: nightly, on demand, or on a
pull request labelled `e2e`.

During work use the cheaper package-scoped gates:

```sh
make verify-fast PKGS="./internal/foo"
make verify-focused PKGS="./internal/foo ./internal/bar"
```

Runnable examples that show the core packages in a few lines:
`internal/rootcause/explain/example_test.go` and
`internal/notification/compose/example_test.go`.

## Real-cluster scenarios

`test/e2e/` runs the real image in Kind from source manifests. See
`test/e2e/README.md`. Every new scenario updates
`test/e2e/coverage/coverage.yaml`, uses bounded watch-based waits, asserts
forbidden behavior as well as expected behavior, and cleans up its
namespace. Issue reproductions are sanitized before they join the suite.

## Operating model

One replica, `Recreate` strategy, a PVC at `/var/lib/kwatch`. The Lease is a
lock, not a failover mechanism; the state file is fenced by its own epoch
counter, claimed only by the Lease holder.
The supervisor starts components when the Lease is held and stops them when
it is lost. See [production operations](./production-operations.md).
