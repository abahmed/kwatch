# Contributor architecture guide

This is a short source-tree guide for contributors. Published tutorials,
reference pages, and operational runbooks live at
[kwatch.dev/docs](https://kwatch.dev/docs).

## Find the right package

Start with the package that owns the behavior you are changing:

| Task | Package |
| --- | --- |
| Watching a Kubernetes kind, translating it to facts | `internal/knowledge/kube` |
| Entities, relations, attributes, change history | `internal/knowledge` |
| Deciding a state is abnormal | `internal/signal/detect` |
| Explaining a signal's root cause | `internal/reason` |
| Settling, updates, resolve hold, flapping, tiers | `internal/problem` |
| The message people read | `internal/story` and `internal/notice` |
| Scope, silences, maintenance holds | `internal/filter` |
| The engine loop, downtime reconciliation, audit entries | `internal/core` |
| Retry, fallback, pacing, or HTTP transport | `internal/delivery` |
| Provider payload mapping | `internal/alert/<provider>` |
| Persisted state, epoch fencing, retention | `internal/knowledge/store` |
| Permission audit | `internal/rbac` |
| Composition, Lease lock, readiness, supervision | `internal/app` |

After YAML and CRD overlays are applied, `config.CompileRuntimeConfig` creates
the immutable derived snapshot used by composition. Keep user-facing fields on
`config.Config`; put normalized namespaces, provider names, compiled provider
routes/retry policy, suppression rules, and effective intervals in the
runtime snapshot. Application composition passes that snapshot to
`delivery.Manager.InitRuntime`.

The normal flow is:

```text
sources → facts → knowledge model → detectors → signals → problems
  → decisions → scope → stories → delivery transport → provider adapter
```

The arrow is also a dependency rule. A source does not call a provider, a
detector does not read Kubernetes, a provider does not read the model, and the
state file does not make detection or reasoning decisions. Read
[How kwatch thinks](./architecture.md) for the behavior of each stage and
[ADR 0010](./adr/0010-problem-centric-core.md) for the design record.

## Read the application flow

Start at `internal/app/run.go`. The composition is split by responsibility:

- `bootstrap.go` and `serve.go`: clients, health, providers, and the serve loop.
- `leader_election.go` and `election_lock.go`: the Lease lock.
- `active.go`: the leader session, which opens and claims the state file, runs
  startup bookkeeping, and supervises components.
- `core.go`: the model, sources, detector registry, reasoner, and engine.
- `supervisor.go`, `components.go`, `readiness.go`: component lifecycle.

Inside the core, follow one signal end to end: a schema in
`knowledge/kube` produces facts, `core/engine.go` applies them and evaluates
detectors, `problem/manager.go` attaches the signal to its root, `Tick`
decides, and `story/writer.go` writes the message that reaches the sink.

## Add a detector or source

1. For a new Kubernetes kind, add its entity kind and `Schema` in
   `knowledge/kube` and register it with the informers. The RBAC audit derives
   from those registrations through `kube.SourceAccess()`.
2. Put the abnormal-state logic in a small `signal/detect` detector that
   reads attributes and relations and returns signals with stable reason names
   from `internal/constant`. Register it in `newDetectorRegistry`.
3. If the kind can be a cause, add or extend a rule in `internal/reason`, and
   register it in `newReasoner`. A rule must require the candidate to be
   unhealthy or changed.
4. Keep clients, queues, and cache sync in the source. Detectors and rules see
   only the model and an injected clock.
5. Add deterministic tests for detection, explanation, and the problem
   lifecycle, then update the coverage reference and the website documentation.

Do not add a method to a universal interface. Health lifecycle is
application-owned: composition calls `HealthServer.Open`, the supervisor runs
`HealthServer.Serve`, and shutdown calls `Stop`.

Health responses expose bounded component states and reason codes. Detailed
errors belong in redacted logs, not public diagnostics. A source that cannot
observe its resource is an unavailable capability: its detectors produce no
signals and never create a synthetic one.

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

State is one bbolt file owned by `internal/knowledge/store`. Use the typed
collections and keep persisted records flat. Any format change requires a bump
of `store.SchemaVersion`, a migration step or documented reset path,
backup/recovery guidance, an old-format fixture, a round-trip test, and a
release note.

## Test and verify

Name tests after behavior and split large test files by responsibility, for
example `queue_retry_test.go` or `downtime_test.go`. Shared fixtures belong
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

## Real-cluster regression scenarios

The semantic Kubernetes suite lives under `test/e2e/`. It uses Kind, the real
Kwatch image, source manifests applied with `kubectl`, and Go tests built on
`sigs.k8s.io/e2e-framework`. It does not use Helm or `kwatch.sh`; those
installation paths have separate validation.

Issue reproductions are converted into permanent sanitized scenarios before
they enter the release suite. The semantic workflow runs the committed
scenario code in Kind; it does not execute issue-provided commands or depend
on an issue-reproduction workflow.

Use `test/e2e/README.md` for the complete scenario contribution workflow.
Every new scenario must update `test/e2e/coverage/coverage.yaml`, use bounded
watch-based waits, assert forbidden behavior as well as expected behavior, and
clean up its namespace.

`verify-fast` skips repository-wide checks. Run `verify-focused` once after a
workstream, then reserve the full gate and complete race suite for handoff.

If a change affects a public setting, metric, provider, persistence format,
RBAC rule, or extension contract, include the documentation and migration
review in the same change.

## Operating model

The deployment runs one replica with the `Recreate` strategy and a PVC at
`/var/lib/kwatch`. The Lease is only a lock that prevents two processes from
sharing the volume, and its transition count fences the state file. There is
no second replica and no ConfigMap state. The application supervisor gates
the active components on the Lease and stops them when it is lost.
