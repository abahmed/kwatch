# 🧠 How kwatch thinks

This page is for contributors and curious operators who want to understand what
happens after Kubernetes reports a problem. For installation, start with the
[interactive manager](./kwatch-sh.md); for settings, use the
[configuration reference](./configuration.md). The design record is
[ADR 0010](./adr/0010-problem-centric-core.md).

kwatch is not a tool that forwards every Kubernetes event to your chat.
Events happen constantly in a cluster and most of them are harmless. kwatch
keeps a model of the whole cluster, finds what is abnormal, works out the one
root cause behind it, and tells **one story per problem**.

The unit of notification is a **problem**, not a `(object, reason)` symptom.
A node running out of memory that makes forty pods unhealthy is one problem
with one message that names the node.

## The pipeline

```text
 Kubernetes API, kubelet, probes, pod logs
              │  facts (one common format)
              ▼
 ┌───────────────────────────── internal/core ─────────────────────────┐
 │ knowledge model ──► detectors ──► signal tracker ──► problems      │
 │  entities             (signals)     raised/changed     settle,      │
 │  relations                          /cleared           digest,      │
 │  attributes             ▲                              flapping,    │
 │  changes, notes         │ explain                      tiers        │
 │                    reasoning rules ◄────────────────────┘           │
 └───────────────────────────────┬─────────────────────────────────────┘
                                 │ decisions (announce / update / resolve)
                                 ▼
                    scope + silences + maintenance
                                 ▼
                    story writer → notice.Message
                                 ▼
                 audit log · delivery manager · providers
```

One goroutine, the engine loop in `internal/core`, owns the whole path from
fact to decision. Sources submit facts from any goroutine into a bounded
queue; the loop applies them to the model in order, evaluates the touched
entities, feeds signal transitions to the problem manager, and ticks problem
lifecycles. Model updates, signal tracking, and problem decisions therefore
need no further locking. Time-based conditions ("not ready for five minutes")
ask the loop to re-check an entity later instead of polling. Each loop
iteration reports progress, so a stuck pipeline shows up as a stall.

### 1. Facts and the cluster model

`internal/knowledge` knows nothing about Kubernetes. Sources translate
objects into **facts** (observed, attribute, relation, changed, gone). The
`Model` applies them and keeps:

- **entities**: anything with identity, such as pods, containers, nodes,
  workloads, Services, PVCs, Secrets, images, and virtual entities for things
  that fail on their own (the API server, etcd, cluster DNS, probe targets);
- **relations**: typed edges such as owned-by, runs-on, selects, references,
  mounts, and routes-to, indexed so a lookup costs the size of the answer, not
  the size of the cluster;
- **attributes**: the current state detectors read;
- **changes**: meaningful changes only (a spec template, image, replica count,
  Secret or ConfigMap digest, Service selector, node taint), with the actor
  when Kubernetes recorded one. The initial list only observes objects, and
  status-only updates refresh attributes without recording a change, so a
  restart never reports the whole cluster as "created";
- **notes**: warning Events attached to the object they are about.

The model is in memory and pruned to a bounded change history.

### 2. Sources

`internal/knowledge/kube` holds the Kubernetes plugins. Every supported kind
has a schema that describes identity, attributes, and relations and diffs two
versions into changes.

- **Informers** for pods, nodes, workloads, Jobs, CronJobs, HPAs, Services,
  EndpointSlices, Ingresses, Secrets, ConfigMaps, ServiceAccounts, PVCs, PVs,
  StorageClasses, Namespaces, LimitRanges, PDBs, ResourceQuotas,
  NetworkPolicies, VolumeAttachments, webhook configurations, and Events.
- **Dynamic and custom resources**: built-in APIs reported through status
  conditions are watched when the cluster serves them, and served CRDs with
  failure-shaped conditions are discovered and watched at runtime.
- **Kubelet statistics** through the API server proxy, for node and volume
  usage, container CPU and memory against limits, and pressure stall data.
- **Control-plane probes**: API server `/readyz` (which also reports etcd),
  the scheduler and controller-manager leader Leases, and an in-cluster DNS
  lookup.
- **Active probes**: opt-in HTTP, TCP, and DNS checks for configured targets,
  and optional TCP checks of Service ports.
- **Log excerpts**: read on demand when a problem is announced, redacted
  before they leave the reader.

A source that cannot list a resource (missing API, missing permission)
degrades the rules that use it. It never creates a problem.

### 3. Signals

`internal/signal` and `internal/signal/detect` turn entity state into
**signals**: an abnormal condition with a stable reason name, a severity, and
evidence. Detectors are small and pure: they read one entity (and its
relations when needed) and return the signals that hold now. A detector for a
kind whose informer has not synced returns nothing, so a missing source never
looks like a recovery.

The `Tracker` compares successive results per entity and reports only
transitions: raised, changed (severity or summary differs), or cleared.
Repeated identical evaluations produce nothing downstream.

### 4. Reasoning

`internal/reason` finds the root cause of a signal. Rules propose
**hypotheses** by walking the knowledge graph upstream from a symptom (node,
rollout, config, reference, backend, scheduling, pods, admission, quota,
network policy, metrics API, DNS, topology, registry). Reachability alone is
never enough: every rule requires the candidate to be unhealthy or to have
changed, so a healthy node is not blamed just because a pod runs on it.
Hypotheses carry weighted evidence for and against; the engine keeps the
best one above a confidence floor and records the rest as a trace. When no
hypothesis clears the floor the story says the cause is unknown.

### 5. Problems

`internal/problem` attaches each signal to the problem of its explained root.
Root resolution follows explanations until nothing better remains, so a
Service symptom explained by a Deployment whose pods fail because of a node
ends at the node. A problem moves through these states:

| State | Meaning |
|:--|:--|
| Settling | Collecting related signals before the first message. Defaults to 75 seconds, 15 for page-tier problems. A problem that recovers here is never announced. |
| Open | Announced and still failing. |
| Recovering | No active signals; waits for a hold before resolving. |
| Flapping | Recovered and failed again repeatedly; transitions stay silent. |
| Resolved | Closed, remembered for a week for recurrence. |

Noise controls live here:

- **Material-change digest**: an update is sent only when the fingerprint of
  what a reader would notice changes: tier, root, cause, the root's own
  conditions, and a bucketed impact size (1, 2-3, 4-7, 8-15, 16+).
  Counters, timestamps, and pods failing one by one never trigger an update.
- **Adaptive resolve hold**: a root must stay healthy for a hold (3 minutes by
  default) that doubles for each recent recovery, up to 30 minutes.
- **Flapping**: three recoveries within 30 minutes make one flapping problem
  that resolves only after staying stable for the maximum hold.
- **Routine**: a problem that opens at about the same time of day on at least
  three days is treated as learned normal and delivered in the digest tier.
- **Tiers**: silent, digest, notify, and page. A critical failure that reaches
  users through an Ingress, or a lost node, pages; informational, planned
  (node draining), and digest-only reasons such as certificate expiry or CPU
  throttling are digest tier. `severityByReason` and `severityByOwnerKind`
  override the derived tier.

### 6. Scope, stories, and audit

The engine drops decisions for problems that are out of scope before
delivery. `internal/filter` applies the configured namespaces, reasons, the
namespace label selector, silence rules, and maintenance holds (annotations
on the object, its pod, or its namespace). An out-of-scope problem is still
tracked so reasoning keeps its evidence.

`internal/story` writes the message from the problem's own evidence: what
broke, why (the proven cause or an honest "cause unknown"), who is affected,
the timeline in order, application output, and next steps with commands for
the real objects, plus configured runbook links. Sections without content are
omitted. The result is a provider-neutral `notice.Message` with a status, a
conversation key, and routing fields (namespaces, reasons, severity derived
from the tier).

When kwatch starts with no restored problems, announcements collected during
the initial list and one settle period are sent as a single **startup
summary** instead of one message each.

Every decision is written to the optional audit log (`internal/audit`) with
the tier, cause state, confidence, and decision reason. `internal/scorecard`
replays an audit log offline and reports noise KPIs: volume, messages per
problem, updates without a visible change, flapping, repeated recoveries, and
diagnosis quality.

## Persistence

State lives in one bbolt file, `state.db`, on the PVC mounted at
`/var/lib/kwatch` (`KWATCH_DATA_DIR` overrides the directory). There is no
ConfigMap state and no database service. `internal/knowledge/store` provides
keyed and time-ordered collections with retention compaction and a schema
version. The file is created with owner-only permissions.

Persisted, by the core: problems (so a restart never repeats a message and
open problems resume), object fingerprints, and lifecycle values (cluster
identity, version, last-seen time, runtime session, telemetry and upgrade
bookkeeping). The engine saves after a decision and at least once a minute,
and once more on shutdown; a failed save is logged and retried, and never
stops delivery.

**Epoch fencing.** The writer claims the store with a number derived from the
Lease transition count. Every write transaction first verifies that its claim
is still the newest, so a process that lost the Lease cannot overwrite newer
state.

**Downtime reconciliation.** Object fingerprints (template hash, replicas,
Secret and ConfigMap digests, Service selector and ports, NetworkPolicy
digest, kubelet version and taints) are saved with a timestamp. After the
first full sync of every source, kwatch compares them with the live cluster
and records changes made while it was not running as changes dated at the
last snapshot, attributed to "unknown". This lets root-cause analysis find a
deployment that happened during a restart. Restored problems wait ten minutes
for their detectors to re-raise signals before they may recover.

## Lifecycle and readiness

`internal/app` is the composition root: it builds configuration, the clients,
delivery, and health, then supervises the components.

- **One replica.** The deployment runs a single replica with the `Recreate`
  strategy. The Lease is only a lock that prevents two processes from
  writing the volume during a rollout; it is not high availability.
- **Leader session.** Once it holds the Lease, the process opens and claims
  the state file, runs startup bookkeeping (first run, upgrade, how the
  previous session ended, one startup message per key), and starts the
  components. Losing the Lease ends the session immediately.
- **Required components**: `delivery` and `core`. A required failure or stall
  removes readiness, cancels the session, and lets Kubernetes restart the
  Pod. Clean stops are not failures.
- **Optional components**: telemetry, upgrade check, the RBAC audit,
  heartbeat, the session alive stamp, and the KwatchConfig watcher when
  enabled. They retry with `1s, 2s, 4s, 8s` backoff capped at 60 seconds and
  report degradation through `/health`.
- **Readiness** is per leadership epoch. `/readyz` succeeds only when the
  state file is claimed, every source finished its initial list, and delivery
  is running. A stale callback from an earlier epoch cannot restore readiness.
  `/healthz` is liveness only.
- **Shutdown** stops the sources with the engine, waits for them, writes the
  final problem snapshot, and closes the store before ending the session.

## Delivery

`internal/delivery` receives stories from the engine sink; nothing else calls
it for problem notifications. It sends each story to every provider through
that provider's queue.

- Routing rules match namespaces, reasons, and severity, with an optional
  single-hop fallback provider; fallback cycles are disabled when the
  generation is built.
- A provider generation is immutable. Reconfiguration drains the old
  generation before publishing a new one; queued jobs keep the generation and
  fallback that accepted them.
- Shared `delivery/transport` decides status classification, timeouts,
  retries with bounded `Retry-After`, and rate limits. Providers only
  validate, render, and return errors.
- Sends to one provider are paced. When a provider queue is full, a queued
  story of the same conversation is replaced by the newer one, and what cannot
  be accepted is summarized in one overflow digest instead of being lost.
- Paging providers receive an alert keyed by the conversation, so an update
  or resolve acts on the same alert.
- Failed deliveries go to a bounded dead-letter list exposed by the
  diagnostics endpoints and counted in metrics.

Providers live in `internal/alert/<provider>` and are constructed through the
static `internal/alert/catalog`. They render `notice.Message`, use shared
transport, and never import Kubernetes or the core.

## RBAC

`internal/knowledge/kube.SourceAccess()` derives the list of permissions the
sources use directly from the informer registrations, marking each as
required (typed informers and Events) or optional (dynamic APIs, kubelet
proxy, pod logs, `/readyz`, kube-system Leases). `internal/rbac` runs one
`SelfSubjectAccessReview` per permission on a slow interval, adds the Lease in
kwatch's own namespace and the KwatchConfig resources when enabled, and reports
each sweep to health. Adding a watched kind therefore updates the audit
without a second list. The audit never changes behavior: sources degrade on
their own, and missing permissions are shown as capability gaps rather than
problems.

## Code architecture and naming

Dependencies flow one way:

```text
cmd/kwatch
    └── internal/app              composition root, Lease lock, supervisor
          ├── core                engine loop, downtime reconciliation
          │     ├── knowledge     model (leaf)
          │     ├── signal        detectors, tracker
          │     ├── reason        rules → hypotheses
          │     ├── problem       settle, digest, flapping, tiers
          │     └── story         message writer
          ├── knowledge/kube      informer sources and translators
          ├── knowledge/store     bbolt state, epoch fencing
          ├── filter              scope, silences, maintenance
          ├── delivery/*          routing, retries, transport
          ├── alert/*             provider adapters and catalog
          ├── rbac, audit         permission audit, decision log
          └── health, startup     diagnostics, startup lifecycle
```

`knowledge`, `notice`, `event`, `model`, `constant`, and `format` are leaf
packages: data and pure helpers that never import orchestration, providers,
or Kubernetes clients. Only the application package assembles concrete
implementations and shared clients; domain packages receive narrow injected
collaborators, including the clock.

Naming follows Go conventions and the domain vocabulary:

- Constructors use `New<Type>`; lifecycle methods use explicit verbs such as
  `Process`, `Resolve`, `Snapshot`, `Start`, `Stop`, and `Validate`.
- Files are lower-case and responsibility-oriented.
- Public initialisms are consistent: `ID`, `UID`, `URL`, `HTTP`, `API`, `PVC`,
  and `JSON`.

## 📊 What you should expect

Fewer messages, each with a cause. On a staging replay the previous design
sent 266 notifications an hour, 39% of updates showed nothing new, and a third
of messages had no cause. The problem-centric design targets one message per
problem, updates only when the reader would notice, and a cause (or an honest
"unknown") on every message. Measure your own cluster with the scorecard.
