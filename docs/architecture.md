# How kwatch works

This page is for operators who want to know what kwatch does with their
cluster, and for contributors who are about to change it. For installation
use the [interactive manager](./kwatch-sh.md); for settings use the
[configuration reference](./configuration.md). The bar kwatch must meet is in
[production goals](./production-goals.md).

kwatch does not forward Kubernetes events to chat. It keeps a model of the
whole cluster, finds what is unhealthy, works out the fewest causes that
explain it, and sends one short message per incident. A node that runs out of
memory and takes forty pods with it is one incident that names the node.

## The pipeline

```text
 Kubernetes API   kubelet stats   probes   pod logs
        │ observations (one common format)
        ▼
 ┌─────────────────────── internal/pipeline ───────────────────────────┐
 │  decision loop (one goroutine, no I/O)                              │
 │                                                                     │
 │  inventory ─► detection ─► rootcause/explain ─► incident            │
 │  objects      health +      fewest causes        settle, update,    │
 │  links        failure mode  that cover the       resolve, flapping  │
 │  changes      per object    failures                  │             │
 │                                                       ▼             │
 │                                       notification/compose          │
 └───────┬───────────────────────────────────────────────┬─────────────┘
         │ bounded workers                               │ Message
         ▼                                               ▼
   investigation (logs, scheduler text)        delivery manager
   storage writer (about once a second)        retries, pacing, fallback
         │                                               │
         ▼                                               ▼
   state.db (bbolt, one file)                  providers (alert/*)
```

One goroutine, the engine loop in `internal/pipeline`, owns every decision.
Sources submit observations from any goroutine into a bounded queue. The loop
applies them to the inventory in order, asks the detectors about each touched
object, solves root cause once for the whole batch, and ticks the incident
lifecycles. Slow work never runs on the loop. Log reads and other
investigations run on a bounded worker pool with deadlines. Storage writes go
to one writer that batches the latest snapshot. Delivery has its own queues.
Every loop iteration reports progress, so a stuck loop shows up as a stall and
Kubernetes restarts the pod.

Each stage is its own package, and the arrow is also the import direction.
[Contributor architecture](./contributor-architecture.md) lists the packages
and the rules.

## The four concepts

| Concept | Meaning | Code |
| --- | --- | --- |
| Object | Anything with identity: every Kubernetes object, plus virtual ones such as a registry, cluster DNS, a zone, a node pool or the API server. Identity is group, kind, namespace and name. | `inventory.Entity`, `inventory.EntityID` |
| Link | A typed, directed relation between objects: owned-by, runs-on, selects, references, mounts, routes-to, serves, pulls and a few more. Links say how failure can travel. | `inventory.RelationType`, `explain.LinkType` |
| Health | The state a detector computes for an object: healthy, degraded, failing or unknown, plus a short failure mode such as `CrashLoop` or `MemoryPressure`, and when it started. | `detection.Finding` (`Health`, `Mode`, `Since`) |
| Change | A recorded edit to an object: what changed, who changed it, when. Status churn is not a change. | `inventory.Change` |

Unknown is a real state. An object kwatch cannot observe (no permission, API
not served, cache not synced) is unknown. Unknown never causes a blame and
never lets an incident resolve.

## One incident, from crash to message

A bad release of `payments` in namespace `shop` makes its pods crash. This is
the `bad-rollout` scenario in `internal/scenarios`.

1. **A change is recorded.** Someone sets a new image on the Deployment. The
   Deployment schema in `inventory/kube` diffs the two versions, sees that
   `spec.template` changed, and the model keeps a `Change` with the actor and
   revision. A status-only update would record nothing.
2. **The pod crashes.** The pod informer delivers the new pod. `PodSchema`
   turns it into an observation with the container's restart count and waiting
   reason. The source calls `Engine.Submit`.
3. **The loop applies it.** `Engine.drain` folds the observation into the
   `Model` and returns the touched objects.
4. **A detector sets health.** `detection.Registry.Evaluate` runs the
   detectors for the container. `detectors.Container` returns a finding with
   reason `CrashLoopBackOff`, severity critical and evidence (the last
   error). `Classify` fills in health `failing` and mode `CrashLoop`. The `Tracker` compares it with the last result and
   reports a raised transition.
5. **Root cause is solved once.** `explain.Explain` groups the failing objects
   into connected areas. For each area it walks links upstream from every
   failure (container, pod, ReplicaSet, Deployment, node), keeps only
   candidates that are unhealthy or changed inside the causal window, and
   scores them. The Deployment changed just before the first crash, only
   new-revision pods fail, and old pods on the same node are fine. The
   `rollout` row of the propagation table matches, evidence adds up, and set
   cover picks the Deployment as the one cause. The node is not blamed.
6. **The incident manager decides.** `incident.Manager.Apply` attaches the
   findings to an incident rooted at the Deployment. Its tier is page. The
   incident settles for 15 seconds (75 for lower tiers) so late failures join
   it, then `Tick` returns an announce decision.
7. **Investigation joins in.** When the incident opened, a worker read a short
   log excerpt, redacted. If it arrives within a short wait it is attached;
   otherwise the message goes without it.
8. **The note is written.** `compose.Writer` turns the facts into sentences:
   what broke and why, who changed what, the one or two facts that prove it,
   who is affected, and one read-only `kubectl` suggestion. The result is one
   `notification.Message`.
9. **Delivery sends it.** The sink in `internal/app` records an audit entry
   and calls `NotifyIncident`. The delivery manager queues the message per
   provider, retries and paces it, and the provider adapter renders it.

When the Deployment's pods stay healthy for the hold time (three minutes), the
manager resolves the incident and the writer says who fixed it, for example
that someone rolled back.

```text
🔴 payments is down in shop after the 14:02 release of payments:2.3. alice
changed the image from payments:2.2 to payments:2.3. Only pods of the new
revision fail. Service payments and ingress storefront can't serve traffic.
Rolling back fixes it: kubectl rollout undo deployment/payments -n shop
```

## Where root cause comes from

How failure spreads is data, not code. `internal/rootcause/explain/table*.go`
holds the propagation table. Each row says that a cause in some modes, through
one link, explains effects in some modes, with a prior belief:

```text
node-not-ready      node: NotReady  runs-on  pod: NotReady, Evicted ...  0.8
rollout             changed spec    owns     pod: CrashLoop, NotReady    0.7
certificate-expired secret: Cert.Expired uses pod that shows TLS errors  0.8
```

Scorers then add or subtract evidence: temporal order, coverage, healthy
siblings elsewhere, an error that names the candidate, recent change, shared
node or zone, baseline deviation. Every weight is in `weights.go` with the
reason for its value. Each contribution carries a code and its numbers
(`rootcause.ProofCode`), and the incident keeps the cause as a
`rootcause.CauseRecord` with its row and mode: messages are written from
those fields, never from the solver's trace text. Below the confidence floor the cause is stated as
unknown. The solver is a pure function of a snapshot: no clock, no I/O.

## How kwatch watches

Sources turn Kubernetes objects into observations. Each kind has a schema in
`internal/inventory/kube` that extracts identity, attributes and links and
diffs versions into changes. Every resource the cluster serves that supports
list and watch is discovered at start and again when CRDs or APIServices
change. A watch mode bounds the memory per kind:

| Mode | What is kept | Used for |
| --- | --- | --- |
| `full` | The typed object, trimmed, with a kind-specific schema | Pods, nodes, workloads, services, storage, policy |
| `status` | Metadata, status and a small subset of spec | Custom resources and other kinds with status |
| `metadata` | Name, labels, owners, generation, finalizers | Leases, ControllerRevisions, RBAC |
| `hashed` | Metadata plus a hash of the data | Secrets and ConfigMaps |

There is a resource budget, a per-kind object cap and an overall object cap.
A kind kwatch cannot watch is reported in `/health` with a bounded reason code.
[Kubernetes coverage](./kubernetes-coverage.md) lists every kind and its mode.

Other sources feed the same observation format: kubelet statistics read
directly from each node's kubelet on port 10250 (RBAC `nodes/stats` and
`nodes/metrics`, not `nodes/proxy`), API server and control-plane probes,
optional active probes, and on-demand log excerpts. A source that cannot read a resource
makes that object unknown. It never creates or resolves an incident.

## Storage

State is one bbolt file, `state.db`, on the volume at `/var/lib/kwatch`. It has
one bucket per data class:

| Bucket | Holds | Kept |
| --- | --- | --- |
| `incidents` | The latest record per incident ID | Resolved ones for 7 days |
| `changes` | Recorded changes per object | 30 days |
| `baselines` | Rolling per-workload statistics | Dropped 7 days after the workload is gone |
| `evidence` | Log excerpts and termination messages | 30 days, at most 128 MiB |
| `timeline` | Health transitions, changes, events, decisions | 30 days |
| `audit` | One entry per incident decision | 30 days |
| `fingerprints` | Last seen digest per object | No expiry |
| `state` | Cluster identity, version, startup and telemetry markers | No expiry |
| `threads` | Provider thread IDs, so updates stay in one conversation | No expiry |
| `outbox` | Delivery jobs not yet accepted by a provider | Until accepted; at most 2048 jobs, none older than 24 hours |

A compactor off the decision loop enforces retention and a total size cap of
512 MiB of logical data (keys and values), of which evidence may use 128 MiB.
Only history is evicted, oldest first; open incidents, baselines, fingerprints,
state, threads and the outbox are never evicted. The file itself is larger than
its logical data, because bbolt keeps freed pages. When the file is more than a
quarter past the cap, the compactor lowers its target by the excess, and the
next start rewrites the file by copying the live data, if the volume has room
for the copy plus a 64 MiB margin. On an `emptyDir` volume the chart passes the
real size limit (`KWATCH_VOLUME_LIMIT`), so that room is judged against the
limit and not against the node's disk. The default 2Gi volume fits the file and
the temporary copy. Only the Lease holder opens the file for writing, and every
write checks a claim epoch, so a process that lost the Lease cannot overwrite
newer state.

Reset: if the file has another schema version or bbolt cannot read it, kwatch
deletes it and starts fresh. No backup is kept, and the reset is logged and
shown in `/health`. A single record that does not decode is skipped and
counted. There is no migration and no backup before the first stable
release. To reset by hand, stop kwatch and delete `state.db`; the next start
repeats the startup summary and relearns baselines.

## Endpoints

kwatch serves five endpoints and no others. There are no diagnostic, profiling
or user-facing API endpoints.

| Endpoint | Meaning |
| --- | --- |
| `/healthz` | Liveness: the process is alive. |
| `/readyz` | Monitoring readiness: this pod holds the Lease, claimed the state file, finished the initial list of every required source, and delivery is running. |
| `/availabilityz` | The pod takes part in the application lifecycle; used by Deployment rollouts. Not monitoring readiness. |
| `/health` | Optional component state and safe reason codes (unwatched kinds, a failed reset, missing permissions). |
| `/metrics` | Prometheus metrics with bounded labels. |

## One replica and a Lease lock

kwatch runs as one replica with the `Recreate` strategy. The state volume is
a PVC. A Kubernetes Lease is only a lock: it keeps two processes from sharing
the volume during a rollout or a node move. The state file is fenced by its
own epoch counter, which only the Lease holder claims. The application supervisor starts the components when the pod
holds the Lease and stops them when it is lost. A required component that
fails or stalls removes readiness and the pod restarts. There is no second
replica and no self-failover: if the pod or its node fails, Kubernetes
restarts it and kwatch resumes from the volume.

## Delivery guarantee

Delivery is at least once, not exactly once. Every queued delivery is written
to the persisted outbox (`outbox` bucket) when a provider queue accepts it and
removed when that provider, or its fallback, accepted it or it was
dead-lettered. After a restart or a crash the next session re-queues what is
left, oldest first, before any new work. The outbox keeps at most 2048 jobs;
past that the oldest job loses its crash protection (it is still sent in this
session), and a job older than 24 hours is dropped at restore. So a message is
not lost across a restart, within those bounds. A send that
was interrupted mid-request may be repeated, because kwatch cannot know
whether the provider received it. If the Lease is lost, nothing more is sent.
Drops are counted by `kwatch_delivery_outbox_dropped_total` and
`kwatch_delivery_dropped_total`.
