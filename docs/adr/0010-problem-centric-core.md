# ADR 0010: Cluster knowledge model and root-cause core

## Status

Accepted (2026-09-28). Partially implemented: the cutover is done and the
previous engine is removed, but several parts of this design are still
planned. See "Implementation status" below. Sections that describe the
target design (storage retention, the rule catalog, situational awareness)
state the goal, not the current behavior.

This ADR supersedes ADRs 0002, 0004, 0006 and 0007 and part of ADR 0001.

ADR 0011 (core redesign: health model, propagation table, explanation
solver, full-cluster coverage, record/replay) supersedes this ADR's
reasoning-engine, storage-layout and message sections. The rest is kept as
history.

## Context

### Measured problem

A 12-hour staging replay of v1.0.0-rc.10 (`kwatch-scorecard`) measured:

| KPI | rc.10 |
| --- | --- |
| Notifications | 3,132 (266/h) |
| Messages per incident | 7.7 (p95 28) |
| Updates with no visible change | 39% |
| Re-created (flapping) incidents | 166 |
| Repeated "Recovered" messages | 78 |
| Messages without a cause | 34% |

A synthetic storm of 5,000 failing pods in 500 workloads produces 500
creates and 4,500 updates.

### Root causes of the noise

1. Kwatch notifies per `(object, reason)` symptom. It does not notify per
   problem.
2. Updates follow internal counters rather than changes a person cares
   about.
3. There is no alert tiering.
4. Resolve has no hysteresis.
5. Root cause is guessed from graph reachability and is rarely checked
   against the candidate's own health. Every pod is "connected" to a node,
   so a node is "likely" even when the node is healthy.
6. Change tracking records informer resyncs and status churn as changes, and
   calls any workload update a rollout.
7. Seven overlapping grouping and suppression layers each have gaps.

### Goal

Kwatch must understand the whole cluster: every resource, how resources
relate, what changed, and what is abnormal. It uses that understanding to
find the root cause and tell one story per problem.

The design must be extensible without redesign. Future sources such as a
node agent (CPU, memory, processes, container runtime details), cloud
provider APIs, or Prometheus must plug in as data sources, detectors and
rules. The reasoning core never changes for them.

## Decision

### Architecture

```
┌──────────────────────── Sources (plugins) ─────────────────────────┐
│ K8s informers · Events · kubelet summary/metrics · pod logs ·      │
│ probes (DNS, API, TLS) · metrics-server · CRD status ·             │
│ future: node agent · cloud APIs · Prometheus · service mesh        │
└───────────────────────────────┬────────────────────────────────────┘
                                │ Facts (one common format)
┌───────────────────────────────▼────────────────────────────────────┐
│ 1. Cluster model: entities · relations · state · change history    │
│    · baselines                                                     │
├────────────────────────────────────────────────────────────────────┤
│ 2. Signals: detectors (plugins, per entity type) emit abnormal     │
│    states                                                          │
├────────────────────────────────────────────────────────────────────┤
│ 3. Reasoning: causal rules (data) + generic engine → root cause,   │
│    chain, impact, confidence                                       │
├────────────────────────────────────────────────────────────────────┤
│ 4. Problems → policy (tiers, material change, hysteresis) → story  │
│    writers                                                         │
└───────────────────────────────┬────────────────────────────────────┘
                                ▼
                   Delivery (all existing providers)
```

The core (model, reasoning engine, problems, policy) knows nothing about
Pods or Nodes. All Kubernetes knowledge lives in plugins. There are four
kinds:

- **source:** produces facts;
- **entity schema:** kinds, attributes and relation extractors;
- **detector:** produces signals;
- **causal rule:** explains how failure propagates across a relation.

### 1. Cluster model

#### Fact: the only input format

```go
type Fact struct {
    Source string       // "k8s-informer", "kubelet", "node-agent", ...
    Time   time.Time
    Entity EntityID     // kind + namespace + name (+ uid)
    Kind   FactKind     // Observed | Attribute | Relation | Change | Gone
    Attr   string       // "ready", "memory.used.pct", "image", ...
    Value  Value        // typed scalar, bounded size
    Target EntityID     // for Relation facts
    Rel    RelationType // "runs-on", "owned-by", ...
    Diff   []FieldDiff  // for Change facts (spec paths, redacted)
}
```

#### Entity

An entity is anything with identity. Built-in kinds cover every Kubernetes
API kind (see the catalog below). There are also virtual entities that
Kubernetes does not model as objects but that fail independently:

- `container` (a container inside a pod);
- `image` and `registry`;
- `cluster-dns`, `apiserver`, `etcd`, `scheduler`, `controller-manager`;
- `kubelet`, `container-runtime`, `cni` and `csi-node` per node;
- `external-endpoint` (a DNS name or IP the workloads call);
- `cloud-loadbalancer` and `cloud-disk`, for later sources.

Future sources add kinds such as `process`, `disk-device` or `nic` without
changing the core.

#### Relation types

Relations are typed, directed, and carry a failure-propagation direction.

| Relation | From → To | Meaning |
| --- | --- | --- |
| `owned-by` | Pod → ReplicaSet → Deployment, Pod → StatefulSet/DaemonSet/Job, Job → CronJob, any → ownerRef | Controller ownership |
| `runs-on` | Pod → Node; container → Pod | Placement |
| `part-of` | container → Pod; kubelet/runtime/cni → Node | Composition |
| `selects` | Service/PDB/NetworkPolicy/HPA target → Pods | Label selection |
| `backs` | EndpointSlice → Service | Endpoints of a Service |
| `routes-to` | Ingress/HTTPRoute/GRPCRoute → Service; Gateway → Route; Service(ExternalName) → external-endpoint | Traffic path |
| `references` | Pod → Secret/ConfigMap/ServiceAccount/PVC/image pull Secret/PriorityClass/RuntimeClass/ResourceClaim | Spec reference |
| `mounts` | Pod → PVC → PV → StorageClass → CSIDriver; PV → VolumeAttachment → Node | Storage chain |
| `scales` | HPA → Deployment/StatefulSet; HPA → APIService (metrics) | Autoscaling |
| `intercepts` | Mutating/ValidatingWebhookConfiguration and admission policies → (namespace, kind) scope; webhook → Service | Admission path |
| `serves` | APIService → Service; CRD → conversion webhook Service | Aggregated APIs |
| `constrains` | ResourceQuota/LimitRange → Namespace; PDB → Pods; NetworkPolicy → Pods; taints → Pods | Policy limits |
| `authorizes` | RoleBinding/ClusterRoleBinding → ServiceAccount; binding → Role | RBAC |
| `resolves-via` | Pod → cluster-dns | DNS dependency |
| `pulls` | Pod/container → image → registry | Image supply |
| `calls` | Pod → Service/external-endpoint (from logs, mesh or agent) | Runtime dependency, learned |
| `schedules` | scheduler → Pod; ResourceSlice/DeviceClass → ResourceClaim | Placement machinery |
| `leases` | Node → Lease (node heartbeat); component → Lease | Liveness heartbeats |

Relations come from specs (ownerRefs, selectors, references). Some are
inferred: `calls` comes from log or mesh evidence, `resolves-via` is
implicit for every pod. Inferred relations carry a weight and an expiry.

#### State, change history and baselines

- **State:** the current attributes per entity. Only attributes used by
  detectors or rules are kept. Informer transforms strip everything else,
  including Secret data.
- **Change history:** meaningful diffs only: spec, image, replicas, config
  and Secret data (as a hash, never the value), labels that affect
  selection, taints, and node add or remove. Status, resyncs and
  resourceVersion churn are excluded. A rollout is a `spec.template`
  change. Each change records the actor when known (managedFields manager,
  ReplicaSet revision, `kubectl.kubernetes.io/last-applied`).
- **Baselines:** per entity: normal restart rate, readiness time, pending
  time, Job duration and schedule, CPU and memory envelope (when a metrics
  source exists), and usual error lines. "Unusual for this one" is a
  first-class input.

### Keeping the cluster view current

The model is always the current cluster state, not a periodic snapshot.

- **Live:** informers stream every add, update and delete as it happens,
  including every catalog kind the RBAC allows. The model updates within
  about a second. Periodic re-lists repair anything a watch missed, and
  the model reconciles against them.
- **Complete:** discovery runs at start and whenever CRDs or APIServices
  change, so newly installed APIs and add-ons are watched without a
  restart (building on the rediscovery already in place).
- **Warm start:** on restart the model loads from the disk snapshot, then
  informers reconcile it with the API. Kwatch knows the previous state
  immediately, and what changed while it was down becomes changes, not a
  blind "everything created".
- **Time dimension:** the store keeps history, so kwatch can compare now
  with before: "3 nodes fewer than an hour ago", "replicas were 6 before
  the HPA hit its max", "this Secret changed while kwatch was down". Root
  cause and impact use this comparison.
- **Gaps are explicit:** a kind that cannot be watched (no RBAC, API
  missing) is marked unknown in the model. Rules that depend on it
  report "cannot verify X" instead of guessing, and health lists the
  missing permission.

### 2. Signals: detectors

A detector is `func(entity, state, history, baseline) []Signal`. A signal has:

- the entity and its kind;
- a stable reason;
- a severity hint;
- evidence: attribute values, event text, log lines, redacted;
- a start time;
- `derived: true`, when it is a symptom by construction (for example
  "Service has no endpoints").

Every check kwatch has today becomes a detector. None are removed.

### 3. Reasoning engine

The engine is generic. It works over entities, relations, signals, changes
and rules.

1. **Seed:** each new or changed signal seeds a search at its entity.
2. **Upstream walk:** follow relations in their propagation direction,
   bounded by depth 6 and a fan-in limit, collecting candidate causes:
   - entities with their own signals;
   - entities with changes inside the causal window;
   - virtual entities (DNS, API server, registry) whose rules match the
     symptom.
3. **Rule evaluation:** for each candidate, apply the causal rules for the
   relation path. A rule returns supporting and contradicting evidence with
   weights. A candidate with no health problem and no change is rejected,
   because reachability alone is never a cause.
4. **Scoring:** combine the evidence:
   - temporal order: the cause started or changed before the symptom;
   - coverage: the share of the candidate's dependents that are failing;
   - specificity: the error text names the candidate;
   - exclusivity: only dependents of this candidate fail, not their
     siblings;
   - the candidate's own severity;
   - the baseline deviation;
   - the rule's prior.

   The best explanation wins. Runners-up above a floor become "also
   possible". Nothing above the floor means the cause is unknown, stated
   plainly.
5. **Downstream walk:** from the root, follow relations the other way to
   collect impact: workloads, Services, Ingresses, Routes, dependent apps.
6. **Chain:** the path from root to the most user-visible impact becomes the
   story, for example "Secret rotated → pods crash → Service empty →
   Ingress 502".

Rules are data. They are declared per relation type and signal pattern,
versioned, and unit-tested with fixtures:

```yaml
- id: node-pressure-evicts-pods
  path: [pod, runs-on, node]
  when:
    cause: {signal: [MemoryPressure, DiskPressure, PIDPressure, NodePressureStall]}
    effect: {signal: [Evicted, OOMKilled, ContainersNotReady, ProbeFailed]}
  evidence:
    cause_first: 3
    coverage_pct_ge_30: 3
    same_node_only: 2
  prior: 0.6
```

A few rules need code, such as parsing error text. They implement the same
`Rule` interface.

### Change timeline and effect correlation

Every change is tracked forward to its effect. Every failure is traced
back to changes. Both results feed the same timeline.

**Change record:** entity, fields changed (spec paths, image, replicas,
config or Secret hash, taints, labels that affect selection, RBAC rules,
policy), old and new values (redacted), time, and the actor:
`managedFields` manager, user or ServiceAccount when the audit source is
available, ReplicaSet or ControllerRevision number, and the GitOps
annotation when present.

**Forward: change → effect.** After a change, kwatch watches its blast
radius for an effect window of 15 minutes (adaptive to the workload's
baseline ready time). The blast radius is the entities downstream of the
changed one in the graph: a Secret → pods that reference it → their
Services and Ingresses; a node taint → pods that must move; a NetworkPolicy
→ pods it selects. The outcome is recorded on the change:

- `healthy`: the dependents stayed or became healthy;
- `degraded`: the dependents developed signals after the change;
- `reverted`: the change was undone and the dependents recovered, which
  confirms causation strongly.

**Backward: effect → change.** For each problem, the reasoning engine
ranks changes in the upstream graph by:

- time proximity: the failure started after the change, and the closer
  the stronger;
- blast-radius overlap: the failing entities are the change's dependents;
- revision specificity: only the new revision or the changed consumers
  fail, and old revisions or unaffected siblings stay healthy;
- error match: the error names the changed field, key, image or port;
- recovery on revert: the dependents recovered after the change was
  undone;
- baseline: this kind of change for this workload has caused failures
  before (history of outcomes).

**Correlation across resources:** changes close in time on related
entities are grouped into one change set, for example a CI deploy that
updates a ConfigMap, a Secret and a Deployment together. The change set is
evaluated as one cause, so the message says "the 14:02 release (image
v2.3, ConfigMap app-config)" instead of three separate suspects.

**Timeline:** each problem carries one ordered timeline that merges
changes, signals, impact spread, recoveries and kwatch's own decisions.
The first alert shows the relevant part; updates append to it.

```
14:01:50  ConfigMap app-config changed (feature.flags) by argocd
14:02:03  Deployment payments rollout started: image v2.2 → v2.3
14:02:41  payments-7c9 (new revision) CrashLoopBackOff:
          "missing key DB_PASSWORD_V2"
14:03:10  Service payments: 0/3 ready endpoints
14:03:12  Ingress shop /checkout → 502 (no backends)
14:03:30  orders: timeouts calling payments (dependent impact)
14:05:00  alert sent — cause: 14:02 release (high confidence)
14:09:15  Deployment payments rolled back to v2.2 → recovering
14:12:15  resolved; change marked reverted-and-confirmed
```

The timeline is stored with the problem, so recurrence and later alerts
can reference it ("the last v2.3 rollout failed the same way").

### 4. Problems, policy and story

- **Problem:** one per root cause, holding its chain, impact and evidence.
  Building is event-driven: a signal marks its entities dirty, and only the
  affected problems are recomputed.
- **Settle:** a new problem waits 60–90s before its first message, or 0s for
  the page tier, so the first message already has the whole picture. The
  deadline is a delayed requeue, not a global tick.
- **Tiers:** `page`, `notify`, `digest` or `silent`, set per signal by
  default. A problem takes its highest member tier.
- **Material change:**
  - a new tier;
  - a different root;
  - impact crossing 1→N or doubling;
  - a new kind of evidence.

  Counters never trigger an update.
- **Hysteresis:** the root must stay healthy for N minutes before resolve.
  Re-failure within that window reopens the same problem silently.
- **Persistence:** problem and decision state are persisted and versioned,
  so a restart never repeats or re-creates a message.
- **Story writers:** one writer per root kind. Each composes what broke,
  why, the chain, the impact, the confidence with its evidence, and the
  next step with real commands. Empty sections are omitted, and a message
  has at most one status emoji. Plain, markdown and HTML variants feed every
  provider.

### Situational awareness

For correct deductions, kwatch needs context beyond individual failures.
The model therefore tracks the following as first-class facts, and rules
use them.

1. **Expected disruption.** Operations that cause failures by design:
   - node drain and cordon;
   - control-plane or node upgrades (version changes on nodes);
   - autoscaler scale-down and node replacement;
   - spot interruption and termination events;
   - rollouts and HPA scaling in progress;
   - Job pods completing;
   - user maintenance windows.

   Signals inside an expected disruption are classified as expected. They
   alert only when they exceed its normal envelope, for example pods still
   unready long after a drain finished, or a PDB blocking the drain.
2. **Topology.** Zone, region, node pool, instance type, kernel, kubelet
   and container runtime version are attributes of every node, and
   therefore of every pod. Failures that share one of these, and only
   these, point to it: a zone outage, a bad node image, or a runtime
   version regression.
3. **Workload configuration.** The model also tracks:
   - requests and limits against actual usage;
   - probe definitions (port, path, timeouts) against the baseline ready
     time;
   - replica count, PDB, anti-affinity, priority, restartPolicy.

   With this, the root can be "memory limit too low for normal usage" (OOM
   at a steady level) rather than "leak" (growing), "readiness probe
   timeout shorter than the app's usual startup", or "single replica, no
   redundancy".
4. **Pod anatomy.** Init containers, native sidecars (restartable init
   containers), ephemeral containers, Job completion and terminating pods
   each have their own semantics. A sidecar crash is not reported as the
   main app failing. A completed Job pod is not a failure.
   Newer pod features are covered too:
   - in-place resize (`PodResizePending` Infeasible or Deferred, and
     `PodResizeInProgress` errors);
   - per-container restart rules;
   - projected volume and ServiceAccount token failures;
   - `hostPort` conflicts;
   - static and mirror pods (control plane);
   - pods on Windows nodes, which have different exit codes and no PSI;
   - graceful and non-graceful node shutdown (the reasons on terminated
     pods).
5. **Multiple simultaneous causes.** Independent problems stay separate.
   Merging requires a proven rule. One entity can be affected by two
   problems, and each message states only its own part.
6. **Negative evidence.** Healthy siblings, unaffected nodes, zones and
   revisions, and consumers of the same Secret that work fine are used
   actively to reject candidate causes.
7. **Data quality awareness.** Source health lowers confidence:
   - informer lag or re-list;
   - missing RBAC;
   - an unreachable kubelet;
   - log fetch failures;
   - a degraded store.

   Kwatch never blames or resolves something based on missing data. It
   says "cannot verify X" instead.
8. **Kwatch self-health.** Delivery failing, store near full or failing,
   Lock lost, and sources degraded are surfaced as kwatch's own problems
   (notify tier, rate-limited). They are never mixed into cluster
   problems.
9. **Startup.** On a cold start, problems that already exist are
   collected and sent as one startup summary, not as N separate alerts.
   On a warm start, the saved state continues and nothing is re-announced.
10. **User intent.** Existing configuration maps into policy without new
    concepts: namespace and reason filters, silences, maintenance windows,
    severity overrides and per-provider routing.
11. **Time.** API server timestamps are the order of record. Event series
    (`count`, `lastTimestamp`) are respected, and clock skew between nodes
    is tolerated with a small ordering margin.
12. **Store security.** Values are redacted before they are written, and
    Secret data is stored only as hashes. Files are mode 0600, and
    encryption at rest relies on the volume. The store is never exposed
    over the network.

### Investigation and verification

These make each deduction checkable and each alert complete.

1. **Automatic investigation per root kind.** When a problem opens, the
   investigator collects exactly the evidence its root kind needs, within
   a bounded budget, before the first message:
   - crash: previous-container logs and the exit reason;
   - node: node conditions, top pods by memory or CPU, kubelet summary;
   - pending: the scheduler message and matching node counts;
   - config or Secret: which keys changed (names only, never values);
   - admission: the webhook Service endpoints and the failure text.

   An operator should never need to run `kubectl describe` to understand
   the alert.
2. **Noisy neighbour attribution.** Node pressure is traced to the pods
   using the resource (kubelet summary per pod). The root becomes "pod
   batch-7f on node X uses 11 GiB of 14 GiB", and the evicted pods are its
   impact.
3. **Scheduler message understanding.** FailedScheduling text is parsed
   into per-reason node counts: insufficient memory on 12 nodes, taint on
   3, volume zone conflict on 2. The dominant blocker becomes the cause,
   with its specific fix.
4. **Log intelligence.** From the crash output, kwatch picks the first
   meaningful error (not the last line of a stack trace) and normalises it
   into a signature by removing IDs, timestamps and addresses. Signatures
   are deduplicated across replicas and matched to known causes (a
   dependency named, a DNS failure, a missing file or key). The same
   signature across different workloads links them to a shared cause.
5. **Impact-aware tiering.** The tier reflects impact, not only the signal
   type:
   - user-facing exposure (reachable through an Ingress, Gateway or
     LoadBalancer);
   - the share of replicas lost (1 of 10 vs 3 of 3);
   - dependents affected;
   - the namespace's criticality: labels or annotations, with production
     namespaces learned from the presence of Ingress, PDB and HPA when not
     set.

   A dev pod crash loop is digest; checkout with no endpoints is page.
6. **Continuous verification.** After the first alert, the hypothesis is
   re-tested as evidence arrives. When new evidence contradicts it, the
   problem's cause is revised and the update says so ("cause revised: the
   node is healthy; pods fail on all nodes with the same config error").
   Confidence is never inflated by repetition.
7. **Resolution explanation.** When a problem resolves, kwatch states what
   fixed it: a rollback, a config fix, a node replaced, scaling, or
   recovery with no change. This feeds change outcomes and recurrence.
8. **Explainability.** Every conclusion keeps its reasoning trace:
   candidates, evidence, scores and rejected alternatives. The trace is
   stored with the problem and shown in a compact "why" line and in the
   thread. It is also what rule tests assert on.
9. **Safe next steps.** Suggested commands use the real object names,
   namespaces and revisions from the model, and are read-only by default.
   Remediations such as `rollout undo` are labelled clearly as changes.

## Resource catalog: failures and links

This catalog is the minimum built-in coverage. "Links" are the relations
the model extracts. "Root when" is the rule condition that lets this
resource be blamed for its dependents' failures. Coverage is Kubernetes
itself. Gateway API and CSI VolumeSnapshots are included because kwatch
already monitors them and both are Kubernetes SIG projects.

### Workloads

| Kind | Failure signals | Links | Root when |
| --- | --- | --- | --- |
| Pod | Pending/Unschedulable, ContainerCreating stuck, admission rejected, Evicted, stuck Terminating, not Ready, PodDisruptionCondition | owned-by, runs-on, references, mounts, selects (reverse), resolves-via, pulls | Rarely. Only a single, ownerless pod |
| container | CrashLoopBackOff, Error/exit code, OOMKilled, ImagePull*, CreateContainerConfigError, CreateContainerError, RunContainerError, probe failures (liveness, readiness, startup), lifecycle hook failure, restarts, CPU throttling, memory near limit | part-of Pod, pulls image | Its own app error when no upstream candidate explains it |
| ReplicaSet | FailedCreate (quota, admission, SA), replicas mismatch | owned-by Deployment | FailedCreate names quota, webhook or ServiceAccount; that becomes the root |
| Deployment | ProgressDeadlineExceeded, Available=False, ReplicaFailure, rollout stuck, unavailable replicas | owns ReplicaSets | A recent `spec.template` change and only new-revision pods fail |
| StatefulSet | Rollout stuck, ordinal blocked, PVC not bound for an ordinal, unavailable | owns Pods, PVC templates | Template change; or the volume chain for its ordinal |
| DaemonSet | Misscheduled, unavailable on nodes, rollout stuck | owns Pods (per node) | Template change; or the specific node (per-node failures) |
| Job | BackoffLimitExceeded, DeadlineExceeded, FailedIndexes, pod failure policy | owned-by CronJob, owns Pods | Its pods' root (image, config, OOM) |
| CronJob | Missed schedules, too many missed starts, last N runs failed, suspended unexpectedly | owns Jobs | Recurring failure across runs → config or image |
| HPA | ScalingActive=False (FailedGetResourceMetric), AbleToScale=False, ScalingLimited at max, flapping | scales workload, depends on metrics APIService | Stuck at max during load → capacity; metric failures → root is the metrics APIService or metrics-server |
| ReplicationController | Same as ReplicaSet | owns Pods | Same |
| PodTemplate / ControllerRevision | None (history source) | revision-of workload | Supplies rollout evidence |
| PodGroup / Workload (gang scheduling) | Group unschedulable, partial placement | groups Pods | Capacity or quota for the group |

### Networking and service

| Kind | Failure signals | Links | Root when |
| --- | --- | --- | --- |
| Service | No endpoints, partial backends, port mismatch, LoadBalancer pending, ExternalName unresolvable, selector matches nothing | selects Pods, backed by EndpointSlice, routes-to external | Selector or port misconfig (a change to the Service); otherwise a symptom of its pods |
| EndpointSlice / Endpoints | No ready endpoints, terminating only | backs Service | Never. Always a symptom |
| Ingress | Backend Service missing or empty, TLS Secret missing or expired, class missing, address not assigned | routes-to Service, references Secret, IngressClass | Its own spec change, missing backend or TLS Secret |
| IngressClass | Controller missing | used by Ingress | Controller down → all its Ingresses |
| Gateway / GatewayClass | Accepted=False, Programmed=False, listener conflict | routes (reverse), class | Controller or config → all attached Routes |
| HTTPRoute / GRPCRoute / TLSRoute | Accepted=False, ResolvedRefs=False (BackendNotFound) | routes-to Service, parent Gateway | Its own change or missing backend |
| NetworkPolicy | Traffic denied (from probes, logs or agent), selects no pods | constrains Pods | A policy change just before connection failures of the pods it selects |
| ServiceCIDR / IPAddress | CIDR exhaustion, IP allocation failure | used by Service | Allocation failures on Service create |
| cluster-dns (virtual) | CoreDNS pods down, probe failures, SERVFAIL | resolves-via (all pods) | DNS-shaped errors across several unrelated workloads + DNS unhealthy |

### Config and storage

| Kind | Failure signals | Links | Root when |
| --- | --- | --- | --- |
| ConfigMap | Missing (referenced), changed | referenced by Pods | Missing; or changed shortly before its consumers failed |
| Secret | Missing, changed, TLS cert expired or expiring, pull Secret invalid | referenced by Pods, Ingresses, SAs | Missing, changed, or expired, and the consumers' errors fit |
| PersistentVolumeClaim | Pending (no PV, provisioner or capacity), Lost, resize pending or failed, usage high or full and growth rate ("full in ~6h", from kubelet volume stats stored over time), inodes high, mounted by no pod for a long time (orphaned), Multi-Attach conflicts, access-mode mismatch | mounted by Pods, bound to PV, StorageClass, VolumeAttachment | Pending, Lost or full → root for pods that mount it; a predicted fill is a proactive warning |
| PersistentVolume | Failed or Released stuck, node affinity conflict | bound PVC, StorageClass | PV failure → PVC → pods |
| StorageClass | Provisioner missing, bad parameters | used by PVCs | All PVCs of the class Pending with provisioning errors |
| VolumeAttachment | Attach or detach error, stuck | PV ↔ Node | FailedAttachVolume / Multi-Attach for dependent pods |
| CSIDriver / CSINode | Driver not registered on a node, driver pods down | node ↔ driver | Mount failures on nodes missing the driver |
| CSIStorageCapacity | Capacity exhausted | StorageClass × topology | Provisioning failures in that topology |
| VolumeSnapshot (CRD) | ReadyToUse=False, errors | PVC | Backup failures |
| VolumeAttributesClass | Modify failures | PVC | PVC modify errors |

### Identity, access and policy

| Kind | Failure signals | Links | Root when |
| --- | --- | --- | --- |
| ServiceAccount | Missing (referenced), token problems | referenced by Pods | Missing SA blocks pod creation |
| Role / ClusterRole / bindings | Changed; the app gets "forbidden" | authorizes SA | An RBAC change before "forbidden" errors from that SA |
| CertificateSigningRequest / PodCertificateRequest / ClusterTrustBundle | Denied, pending, expiring | used by kubelet and workloads | Kubelet or client certificate failures |
| ResourceQuota | Exceeded | constrains Namespace | FailedCreate "exceeded quota" in the namespace |
| LimitRange | Violated | constrains Namespace | FailedCreate "limit" violations |
| PodDisruptionBudget | Disruptions blocked, unhealthy | constrains Pods | Drains or upgrades blocked (a node can't drain) |
| PriorityClass | Missing, preemption | referenced by Pods | Preempted pods → the higher-priority workload that caused it |
| Namespace | Terminating stuck (finalizers) | contains all | Its stuck deletion blocks resources inside |

### Admission and API extension

| Kind | Failure signals | Links | Root when |
| --- | --- | --- | --- |
| Mutating/ValidatingWebhookConfiguration | Backend Service has no endpoints, timeouts, TLS errors, failurePolicy=Fail with errors | intercepts (scope), backed by Service | Creates or updates in its scope fail naming it → root for many workloads at once |
| (Mutating/Validating)AdmissionPolicy + Binding | Policy rejects, CEL errors | intercepts (scope) | Rejections naming the policy after its change |
| CustomResourceDefinition | Not established, conversion webhook failing, storage version issues | served by conversion Service | CR reads or writes failing |
| APIService | Available=False (FailedDiscoveryCheck) | serves via Service | metrics.k8s.io down → HPA failures; any aggregated API → discovery errors |
| CRD instances (operators) | Ready=False, Degraded=True and similar failure conditions | owned resources | Operator-reported failure → its owned objects |
| FlowSchema / PriorityLevelConfiguration | API throttling (429s, rejected requests) | apiserver | Controllers or kwatch throttled → wide slowness |

### Cluster and nodes

| Kind | Failure signals | Links | Root when |
| --- | --- | --- | --- |
| Node | NotReady, Unknown (lost), Memory/Disk/PIDPressure, NetworkUnavailable, PSI stall, filesystem or inode high, unschedulable (cordon), taints added, overcommit, lease stale, node added or removed (scale-in, spot) | hosts Pods, has kubelet/runtime/cni/csi-node | Node signal first + a high share of its pods failing + pods on other nodes fine |
| Lease (node heartbeat) | Stale renewals | Node | Never alone. It is evidence for the Node |
| kubelet / container-runtime / cni (virtual, per node) | PLEG not healthy, runtime errors, CNI "network not ready", sandbox creation failures | part-of Node | Pod sandbox or network failures confined to that node |
| RuntimeClass | Handler missing on nodes | referenced by Pods | Pods using it fail on nodes without the handler |
| ResourceClaim / ResourceSlice / DeviceClass / DeviceTaintRule (DRA) | Claim unallocated, device tainted | referenced by Pods, on nodes | Pods pending or evicted for devices |
| apiserver / etcd / scheduler / controller-manager (virtual + static pods) | Probe failures, latency, component pods failing, leader Lease stale | cluster-wide | Wide, simultaneous symptoms: everything Pending (scheduler), no reconciliation (controller-manager), API errors |
| cluster-autoscaler (Kubernetes Events) | FailedToScaleUp, NotTriggerScaleUp | capacity for Pending pods | Pending pods with an autoscaler failure → capacity root |
| Event | Warning events (every reason kwatch knows today) | involved object | Evidence only. Events attach to their entity |
| ComponentStatus | Deprecated; used only if present | components | Evidence only |

Additional built-in kinds:

| Kind | Failure signals | Links | Root when |
| --- | --- | --- | --- |
| kube-proxy (virtual, per node) | Pods down, sync errors, conntrack full | part-of Node, programs Services | Service traffic fails only from pods on that node |
| StorageVersionMigration / StorageVersion | Migration failed or stuck | CRD or resource | Reads of that resource fail after an upgrade |
| LeaseCandidate | Component cannot win or renew its leader Lease | control-plane component | Controllers not reconciling during an upgrade |
| CompositePodGroup | Group unschedulable | groups PodGroups | Capacity or quota for the gang |
| ResourceClaimTemplate | Template invalid, claims fail to generate | generates ResourceClaims | Pods pending for devices after its change |
| cluster (virtual) | Control-plane/kubelet version skew, deprecated APIs in use, cluster certificates expiring | all | Upgrade-related failures; proactive warnings (digest tier) |

### Custom resources

Built-in coverage is limited to Kubernetes itself. Third-party add-ons
(cert-manager, Karpenter, KEDA, Argo, Flux and others) get no dedicated
schemas for now. They can be added later as optional plugins without
touching the core. Until then, any custom resource is covered by one
generic rule: a failing status condition (`Ready=False`, `Degraded=True`,
`Synced=False`) makes it a root candidate for the objects it owns through
ownerReferences.

### External and future sources

| Entity | Signals | Links | Root when |
| --- | --- | --- | --- |
| image / registry (virtual) | Pull errors, auth, rate limit (429), not found | pulls | ImagePull failures across workloads share a registry or pull Secret |
| external-endpoint (virtual) | Timeouts, refused, DNS NXDOMAIN (from logs, probes or agent) | calls | Several workloads fail calling the same endpoint |
| node agent: process, disk-device, nic, container runtime detail | CPU and memory saturation, IO wait, disk errors, NIC drops, OOM victims, zombie processes | part-of Node / container | The saturating process or device is the root for slow or failing containers |
| cloud-loadbalancer / cloud-disk (cloud API source) | Provisioning failure, health checks failing, quota | LoadBalancer Service, PV | Explains LB pending or attach failures |

## Root-cause rule set (initial)

Rules are grouped by the relation they cross. Each lists its evidence;
unmet contradictions lower the score.

1. **Node → pods**
   - Supports: the node signal appears first; ≥30% of its pods fail; the
     sibling replicas on other nodes are healthy.
   - Contradicts: the node is healthy.
2. **Rollout → pods** (template change)
   - Supports: the failure starts within 15m of the change; only the new
     revision fails; the old revision is healthy.
3. **Config or Secret change → pods**
   - Supports: the data hash changed before the failure; the consumers
     restarted after it; the error names a key or file.
4. **Missing reference → pods**
   - Supports: the referenced object doesn't exist;
     CreateContainerConfigError or FailedMount names it.
5. **Volume chain → pods**
   - Supports: the PVC is Pending or full, or the VolumeAttachment failed;
     only pods using the volume are affected.
6. **Capacity → pending pods**
   - Supports: Insufficient cpu or memory, taints, or affinity in
     FailedScheduling; an autoscaler failure; many pending pods with the
     same reason.
7. **Quota or LimitRange → workloads**
   - Supports: FailedCreate says "exceeded quota" or "forbidden: limit";
     scoped to the namespace.
8. **Admission → workloads**
   - Supports: FailedCreate or update names the webhook or policy; the
     webhook backend is unhealthy or the policy changed; many kinds in the
     scope are affected.
9. **DNS → workloads**
   - Supports: DNS error signatures in several unrelated workloads; CoreDNS
     is unhealthy or the probe fails.
10. **Metrics API → HPAs**
    - Supports: metrics.k8s.io APIService is unavailable or metrics-server
      is down; many HPAs fail at once.
11. **Control plane → cluster**
    - Supports: component unhealthy; cluster-wide symptoms match (scheduling
      stalls, no reconciliation, API errors).
12. **Network policy → pods**
    - Supports: the policy changed; connection refused or timeouts from the
      selected pods; unselected pods are fine.
13. **RBAC → workloads**
    - Supports: a binding or role changed; "forbidden" errors come from that
      ServiceAccount.
14. **Registry → pulls**
    - Supports: the same registry or pull Secret appears across ImagePull
      failures; auth errors, 429s or not-found.
15. **Service misconfiguration → routes**
    - Supports: a Service port, selector or targetPort change; endpoint loss
      while the pods are healthy.
16. **Ingress, Gateway or Route → traffic**
    - Supports: a missing backend or TLS problem; the route is not accepted
      after its change.
17. **Certificate expiry → consumers**
    - Supports: the certificate expired; TLS errors in the consumers.
18. **Node lifecycle → workloads**
    - Supports: a node was removed or replaced (spot, scale-in) just before
      pods were rescheduled or became Pending; PDB violations.
19. **Operator CR → owned objects**
    - Supports: the CR reports a failure condition before its owned
      resources fail.
20. **Workload-local** (fallback)
    - No upstream candidate passes. The root is the workload itself.
    - The story shows the strongest error evidence and never invents a
      cause.

21. **Topology → workloads**
    - Supports: the failures share one zone, node pool, instance type, or
      kernel or runtime version, and only it; siblings elsewhere are
      healthy.
22. **Workload configuration → own failures**
    - Supports: OOM at a steady usage near the limit (limit too low, versus
      a leak that grows); a readiness or liveness timeout shorter than the
      baseline startup; a probe port or path mismatch with the container
      ports; a single replica during node loss.
23. **Expected disruption → quiet**
    - Supports: signals fall inside a drain, upgrade, scale-down, rollout
      or maintenance window and stay within the expected envelope.
    - The result is to classify as expected, not to alert, unless the
      envelope is exceeded.
24. **Sidecar and init containers**
    - Supports: a failing sidecar or init container blocks or restarts the
      pod.
    - The root is that container (for example a mesh proxy or a secrets
      injector), not the app.

Rules are added over time (for example, from a node agent: "process
saturation → container slow") without touching the engine.

## Storage: one disk, full cluster memory

With a disk, kwatch keeps everything that improves its understanding. It
does not keep only what fits in a ConfigMap.

### What is stored

| Data | Why it helps | Retention (default) |
| --- | --- | --- |
| Full graph: entities, relations, trimmed state | Instant warm start; diff of the state saved at shutdown and the state at start | Live, plus snapshots every 15m for 7d |
| Change history: spec diffs, image, replicas, config and Secret hashes, RBAC, taints, node add/remove, actor | "What changed before it broke" across all resources | 30d |
| Signals and problem history: root, chain, impact, evidence, timeline, resolution | Recurrence ("3rd time this week") and flap detection | 90d |
| Baselines: restart rate, ready time, pending time, Job durations, resource envelope | "Unusual for this workload" | Rolling aggregates, dropped 7d after the workload is gone |
| Event digest: Warning events, deduplicated | Evidence long after the API server drops events (1h) | 7d |
| Evidence excerpts: log lines and termination messages used in problems (redacted) | Show what the app said, even after the pod is gone | 30d, size-capped |
| Learned relations (`calls`, from logs, mesh or agent) | Dependency map that Kubernetes doesn't have | Decays when not seen for 7d |
| Agent and metric aggregates (future) | Saturation context | 1-minute resolution for 24h, 1-hour resolution for 30d |
| Decision state: open problems, sent messages, thread IDs | No repeats, no losses | While open, plus 7d |

All data is redacted before it is written. Total size is capped: default
512 MiB of logical data (evidence 128 MiB) on a 2Gi volume, with the oldest
history compacted first. Retention and caps are fixed defaults in this
release (see ADR 0011).

### Store engine

The store is bbolt: pure Go, one file, ACID transactions and crash safety.
It is the storage engine used by etcd.

- **One writer:** a single replica on an RWO volume. bbolt's file lock also
  prevents two processes on the same node from opening it. Every write
  transaction first checks the stored Lease epoch and aborts if a newer
  holder has written, so a stale pod can never overwrite newer state.
- **Layout:** buckets per data class (graph, changes, problems, baselines,
  events, evidence, decisions). Keys are ordered as entity plus time, so
  range scans are cheap.
- **Retention:** a background compactor enforces the retention table and
  the size cap in small batches. It runs with a bounded duration and never
  on the hot path.
- **Only store:** the disk is the single source of truth for all state,
  including decisions (open problems, sent messages, thread IDs). Kwatch
  keeps no state in ConfigMaps. The only ConfigMap left is the user's
  configuration.

### Deployment layout: one replica, one disk, a Lease lock

Multi-replica HA is removed. Kwatch is not in any request path. A 1–2
minute monitoring gap while the pod restarts is acceptable. A problem that
is still happening is detected again, and the persisted delivery outbox
keeps notifications across restarts (at least once; a send interrupted
mid-request may repeat). Standbys, warm failover, the PDB,
and the ReadWriteMany requirement cost more complexity than they return.
`/availabilityz` is retained as the readiness and availability probe for
rollouts; `/readyz` still reports whether monitoring is active.

```
Deployment kwatch   replicas: 1, strategy: Recreate
  tolerations: node not-ready / unreachable for 30s
  volume: PVC kwatch-data (RWO, 2Gi), optional
Lease kwatch-lock   hold before sending or writing; stop on loss
```

- **Deployment, not StatefulSet.** On node loss a StatefulSet waits until
  the old pod is confirmed gone, which can take a long time for an
  unreachable node. That stalls monitoring. A Deployment starts the
  replacement after the short toleration. A standalone PVC can be resized
  in place, unlike `volumeClaimTemplates`.
- **The Lease is a lock, not HA.** Two pods can still overlap: during a
  rollout without `Recreate`, or when a partitioned node keeps the old pod
  running. The pod must hold the Lease before it sends notifications or
  writes to the store. It stops both as soon as a renewal fails. Every
  store record carries the Lease epoch, so a stale holder's late writes
  are rejected on read.
- **The disk is required.** The installer uses the default StorageClass,
  or one the user names. A cluster without any StorageClass can use an
  `emptyDir` volume for evaluation. It runs the same bbolt code, but all
  state, including sent-message decisions, is lost when the pod moves, and
  open problems may be announced once more. The installer warns about
  this.
- **Node failure:** after the toleration (30s) the pod is rescheduled. The
  RWO volume attaches once the cloud detaches it; this can take up to about
  6 minutes on some clouds, and is faster with non-graceful node shutdown
  handling. State on the disk keeps notifications correct across the gap.

### Volume options

| Volume | When | Behaviour |
| --- | --- | --- |
| **PVC** (default) | A StorageClass exists | All state persists across restarts and moves |
| **emptyDir** (evaluation) | No StorageClass | Same store; state lost when the pod is deleted or moved |

There is one `Store` implementation (bbolt) and no alternative backends.

## What the store enables

The store exists to make alerts correct, complete and quiet. Nothing else.

| Capability | How the store is used | Effect on alerts |
| --- | --- | --- |
| **Change memory** | Every change with its actor, kept across restarts; diff of the state saved at shutdown and the state at start | Root cause can name the change, including one made while kwatch was down |
| **Learned normal** | Per-workload baselines: restarts, ready time, pending time, Job duration, usual errors | Alerts on real deviations; routine behaviour stays quiet |
| **Recurrence and flapping** | Problem history keyed by root and kind | No repeated alerts; the message says "3rd time this week" |
| **Evidence that outlives pods** | Log excerpts, termination messages and events stored with the problem | The alert keeps its evidence after the pod or events are gone |
| **Predictions** | Usage history for PVCs, node disks, container memory; certificate expiry | Warn before failure: "PVC full in ~6h", "memory grows 40 MiB/h" |
| **Decision state** | Open problems, sent messages, thread IDs | No lost updates across restarts; a rare duplicate is possible |

Out of scope for this work: reports, postmortem export, query APIs or
CLI, self-tuning, and capacity trends. They can be built on the same store
later.

## Flapping and recurring issues

A flap is something that fails, recovers, fails again, and so on. Every
problem has a lifecycle with memory, not a boolean.

```
           signal                  root healthy
 (none) ─────────► OPEN ─────────────────────────► RECOVERING
                    ▲                                  │
                    │ re-fails within hold              │ healthy for hold
                    └──────────── (silent) ◄───────────┤
                                                        ▼
   FLAPPING ◄── ≥3 open/recover cycles in 30m ──  RESOLVED
     │                                                  │
     │ stable for adaptive hold                         │ same root+kind
     ▼                                                  ▼ re-fails later
   RESOLVED                                  REOPENED (linked to history)
```

- **Adaptive hysteresis:** the resolve hold starts at 3 minutes and grows
  with the entity's flap history: it doubles per recent cycle, up to 30
  minutes. A workload that flaps a lot must stay healthy longer before
  "resolved" is sent.
- **Flapping state:** after 3 cycles in 30 minutes, the problem turns into
  one "flapping" problem. One message is sent, for example "orders is
  flapping: failed 5 times in 20m, each time for about 40s, while probe
  latency spikes". Individual transitions are then silent. Later updates
  come only when the pattern changes (a failure lasts longer, or the
  impact grows), plus one summary when it becomes stable.
- **Recurrence:** a new problem with the same root and kind as a stored one
  links to it. The message says "happened again: 3rd time this week, last
  on Tue for 12m, resolved without changes". After a configurable count it
  is escalated as a recurring problem.
- **Known routines:** baselines learn expected patterns, such as nightly
  Job pods Pending during node scale-up or a readiness blip on every
  deploy. These stay at digest or silent tier unless they deviate from
  their normal duration or frequency.
- **Pod churn vs problem identity:** problems are keyed by root entity and
  kind, not by pod name. Replaced pods (new names, same owner) continue
  the same problem instead of creating new ones.
- **Oscillating resources:** HPA scale flapping, Endpoints churn, and node
  add/remove loops are detected as rates over time, not as repeated
  incidents. They produce a single "unstable" signal when the rate exceeds
  the baseline.

## Scalability

- **Size:** 5,000 pods and 500 nodes on one pod within 256–512 MiB. The
  model stores only the attributes rules use. Informer transforms strip
  the rest (done for Secrets).
- **Indexes:** relation adjacency in both directions, by kind and by
  namespace, and by node. Change history is indexed by entity and by time.
  There are no scans over all incidents or changes.
- **Incremental work:** reasoning runs only for dirty entities. The walks
  are bounded (depth 6, fan-in caps with sampling for huge fan-ins such as
  node → 110 pods). Results are cached per problem and invalidated by the
  relation changes they used.
- **Bounded memory:** the hot working set is in memory (current model,
  open problems, the last 2h of changes per entity). Everything older is
  on disk under the retention caps above and is loaded on demand through
  indexes.
- **Sources:** each runs with its own concurrency, rate and memory budget.
  Log fetches happen only for problem evidence, never on the hot path.
  Agent samples are aggregated before becoming facts.
- **API budget:** an explicit QPS budget. Lease renewal uses its own client
  (done).
- **Storm:** 1,000 pods failing at once must produce at most 3 messages
  within 2 minutes (scale-test gate).

## Acceptance (scorecard gates for cutover)

| KPI | Target |
| --- | --- |
| Notifications per hour (rc.10 staging replay) | ≤ 20 |
| Messages per problem | ≤ 3 |
| Unchanged updates | 0% |
| Re-created problems | ≤ 5% |
| Repeated recoveries | 0 |
| Circular causes | 0 |
| Page or notify problems without a cause or a next step | ≤ 10% |
| Correct root on labelled scenarios | ≥ 90% |

Labelled scenarios cover one case per rule above: bad deploy, Secret key
removed, node memory pressure, node lost (spot), CoreDNS down, blocking
webhook, quota exhausted, PVC full, registry auth, metrics-server down,
NetworkPolicy change, RBAC change, certificate expired, capacity pending,
an operator CR failure, a zone or node-pool failure, a memory limit too
low, a probe timeout too short, a failing sidecar, and a node drain within
its envelope (must stay quiet), a noisy-neighbour memory hog, a
scheduler blocker mix, and a cause revised after contradicting evidence. They also cover negative cases: a healthy node
with a crashing app must not blame the node.

## Rollout

The rollout is complete:

1. The model (sources, entity schema, relations) was built and verified
   against the previous graph.
2. Every previous check was ported to detectors, with the reason parity
   test guarding coverage.
3. The reasoning engine and rules were built with fixture tests per rule.
4. Problems, policy and writers were built next to the previous engine.
5. The cutover happened on the `feat/kwatch-plan` branch. The previous
   engine, grouping layers, feedback learning and pattern filler are
   removed. There is no `KWATCH_CORE` flag; the core is the only engine.

The scorecard gates below were used as targets. They are not yet enforced
in CI (see "Implementation status").

## Implementation status

Implemented:

- Cluster model from informer, event, kubelet, log, probe and CRD sources,
  with typed entities and relations (`internal/inventory`,
  `internal/inventory/kube`).
- Detectors (`internal/detection`, `internal/detection/detectors`) and the
  root-cause engine with the rules in `internal/rootcause` (rollout, config change, node, failing pods,
  registry, cluster DNS, dependency, admission, quota, scheduling,
  topology, network policy, backend and metrics API).
- Problems with tiers, material-change policy, hysteresis and one story per
  problem (`internal/incident`, `internal/notification/compose`,
  `internal/notification`), orchestrated by `internal/pipeline` and delivered
  through the existing providers.
- Crash investigation that attaches recent previous-container log excerpts
  for crash roots.
- bbolt store (`internal/storage`) with Lease-fenced writes. It persists problems, object
  fingerprints and state (provider thread IDs, notified version).
- Single replica, `Recreate`, Lease as a write lock. `/availabilityz` is the
  readiness probe.
- Removal of the previous engine, ConfigMap persistence and the obsolete
  configuration sections listed under Consequences.

Not yet implemented:

- Persisted change history, baselines and evidence excerpts. Only problems,
  object fingerprints and state are on disk today.
- The compactor, the 2 GiB size cap and configurable retention.
- A generic upstream-walk engine with rules as data. Rules are Go code.
- Rules 11, 13, 15, 16, 17, 18, 19, 22 and 24 of the rule set.
- Self-health problems (kwatch reporting its own degradation as a problem).
- Resolution explanation (why a problem resolved).
- Markdown and HTML story variants for providers.
- Digest batching of low-tier problems.
- Scorecard gates enforced in CI, and the storm gate (1,000 failing pods,
  at most 3 messages in 2 minutes) has no test.

### Naming

After implementation the packages and types were renamed to common SRE
vocabulary. This ADR keeps its original wording; read it with this mapping:

| ADR term / old package | Current name |
|:--|:--|
| Fact (`knowledge.Fact`) | Observation (`inventory.Observation`) |
| Cluster knowledge model (`internal/knowledge`) | Inventory (`internal/inventory`) |
| `internal/knowledge/kube` | `internal/inventory/kube` |
| `internal/knowledge/store` | `internal/storage` |
| Signal (`internal/signal`) | Finding (`internal/detection`, `detection.Finding`) |
| `internal/signal/detect` | `internal/detection/detectors` |
| Hypothesis (`internal/reason`) | Cause (`internal/rootcause`, `rootcause.Cause`) |
| Problem (`internal/problem`) | Incident (`internal/incident`, `incident.Incident`) |
| Story writer (`internal/story`) | `internal/notification/compose` (`compose.Writer`) |
| Notice (`internal/notice`) | `internal/notification` (`notification.Message`) |
| Core engine (`internal/core`) | Pipeline (`internal/pipeline`, `pipeline.Engine`) |
| `/problems` diagnostics endpoint | `/incidents` |

## Consequences

- Fewer, complete messages that explain the cause. The page tier bypasses
  settling.
- New data sources extend kwatch by adding plugins. The core is stable.
- Some configuration becomes obsolete (grouping windows, mass-failure
  thresholds, feedback). Each removal is proposed individually.
  The removals were approved: the `*Monitor` sections except
  `heartbeatMonitor` and `activeProbeMonitor`, `correlation`,
  `smartGrouping`, `inhibition`, `workers`, log and event inclusion
  settings, `ignoreLogPatterns` and `silences[].logPatterns`, restart
  thresholds and startup-baseline reporting, and `crd.failureConditions` and
  `crd.graphReferences`. Kwatch now always watches every supported resource
  and scope settings only filter delivery. RBAC is derived from the sources'
  declared access.
- No migration is needed. Kwatch has no stable release or production
  users yet, so the bbolt store replaces all current ConfigMap state
  (incidents, shards, groups, threads, baselines, changes, feedback) and
  the persistence manager. The formats are versioned from their first release, so
  later changes can migrate.
- RBAC: understanding everything needs list and watch on more kinds
  (storage, admission, flowcontrol, DRA, RBAC). Each is optional. A missing
  permission degrades only the rules that need it, is reported in health,
  and is documented with least-privilege guidance.

## Appendix: reasons superseded by the core

Every failure the previous engine detected is detected by the core. The
reasons below are not emitted as separate signals, because the core
expresses them differently:

| Previous reason | In the core |
| --- | --- |
| CrashLoopHighFrequency, OOMRepeating | Escalated summary and severity of CrashLoopBackOff / OOMKilled with the restart count |
| HighRestartCount | Emitted for running containers that keep restarting |
| LivenessProbeFailed, ReadinessProbeFailed, StartupProbeFailed, ProbeError | `Unhealthy` events are evidence; the effect is detected as restarts, crash loops or ContainersNotReady |
| BackOff | Event evidence for crash loops and image pulls |
| PostStartHookError, PreStopHookError | FailedPostStartHook / FailedPreStopHook event signals |
| ContainerCreating, PodInitializing, PodCompleted | Progress states, not failures |
| NodeAffinity | Part of Unschedulable, with the scheduler's blockers decoded |
| Preempting | Scheduler event evidence; the preempted pods are rescheduled |
| RegistryUnavailable | The registry rule: one root for pull failures from the same registry |
| DeploymentAvailableFalse, DeploymentProgressingFalse | DeploymentUnavailable and ProgressDeadlineExceeded symptoms |
| StatefulSetConditionFailure, DaemonSetConditionFailure | Availability and replica-failure signals of the workload |
| FailedGetMetrics, FailedComputeMetricsReplicas, ScalingDisabled, TooManyReplicas, HPAScalingLimited | HPA condition signals (metrics unavailable, cannot scale, maxed out) |
| NodeLeaseStale | NodeNotReady with the kubelet unreachable (Ready=Unknown) |
| ControlPlaneComponentFailure | Control-plane pods are ordinary pods (their own root); scheduler, controller-manager, etcd and API server have dedicated probes |
| SharedDependencyFailure | Replaced by root-cause problems |
| PreExistingAtStartup | The startup summary |
