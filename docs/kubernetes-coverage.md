# 🎯 Kubernetes failure coverage

This page answers one question: **what can kwatch notice?** It is a technical
reference for operators who want to understand the signals behind an alert.
For a quick overview, see the [README](../README.md).

kwatch builds a model of the cluster from informer state, status conditions,
Kubernetes Events, kubelet statistics, active probes, and short log excerpts.
Small detectors read that model and raise signals with stable reason names;
the reasoning rules then look for the one root cause behind them (see
[How kwatch thinks](./architecture.md)). It does not require Prometheus,
Grafana, or another external monitoring product. No single Kubernetes object
status proves that application traffic, DNS, or runtime health is working, so
the categories below intentionally use different signal sources.

## Object and lifecycle signals

Each item names what a detector reads. A failing object becomes a signal;
the signal is then attached to the problem of its explained root, so several
of these can appear together in one message.

- Pods: Pending and unschedulable reasons, scheduling gates, Failed/Unknown
  phases, init containers, waiting and terminated container states, restart
  transitions, CrashLoopBackOff, image pull/config/create/sandbox/probe/
  lifecycle-hook failures, OOM and eviction evidence, pods not ready longer
  than their own startup budget, and stuck pod deletion. The pod detector
  evaluates pod state; a separate container detector evaluates each container
  as its own entity, so the message names the container that failed.
- Deployments, ReplicaSets, StatefulSets, DaemonSets, Jobs and CronJobs:
  progress deadline, availability and replica-failure conditions, sustained
  unavailability, Job failure/deadline/backoff, and CronJob suspension or a
  missed schedule. Suspension is informational.
- Nodes: Ready, memory/disk/PID/network pressure, lease heartbeat staleness,
  request/allocatable overcommit, and kubelet-summary usage thresholds for
  memory, filesystem and inodes, plus pressure stall, network error and
  runtime error rates when the kubelet reports them. Nodes being drained are
  recognised so the disrupted pods are treated as expected maintenance.
- Storage: PVC Pending/Lost and resize failures, mounted volume usage and
  time-to-full estimated from recent growth, PV Released/Failed, and CSI
  VolumeAttachment attach errors.
- Services and admission backends: EndpointSlice readiness, missing
  endpoints, port mismatches, LoadBalancer provisioning, Ingresses whose
  backend Service does not exist, and webhook configurations whose Service is
  missing or has no usable endpoints.
- Cluster resources: exhausted ResourceQuota, contradictory LimitRange
  constraints, and stuck Namespace termination.
- Resource-level Events: recent failure-shaped Warning Events are attached
  as notes to the object they are about and are read as evidence by
  detectors and rules. Normal Events are ignored.
- References: pods that need a Secret, ConfigMap, or ServiceAccount nobody
  created, TLS Secrets that are expired or expiring soon, and Pod Security
  Admission and ValidatingAdmissionPolicy misconfiguration reported by the
  API. The signal preserves the exact object and reference name.
- Built-in platform APIs outside the typed informers: CertificateSigningRequest,
  API Priority and Fairness (FlowSchema/PriorityLevelConfiguration),
  ValidatingAdmissionPolicy and its bindings, and ResourceClaim are watched
  through the dynamic source when the cluster serves them.
- Control plane: the API server `/readyz` check (which also reports etcd), the
  scheduler and controller-manager leader Leases (a Lease that stopped
  renewing means the component is down even when its Pods are hidden on a
  managed control plane), and an in-cluster DNS lookup of
  `kubernetes.default.svc`, which validates the whole service-discovery path
  from the kwatch Pod rather than the CoreDNS Pod phase.
- Services (optional): `activeProbeMonitor.autoServices` runs in-cluster TCP
  checks of ClusterIP Service ports, bounded so a large cluster is not
  scanned. It is opt-in because a declared port is not proof that an
  application listener is intended.
- Active probes (optional): HTTP, TCP, and DNS checks for explicitly
  configured targets through `activeProbeMonitor`, with consecutive-failure
  and recovery thresholds and per-target HTTP latency warning and critical
  thresholds. Targets are never inferred from Services unless `autoServices`
  is on.
- Autoscaling: HPAs that cannot compute or apply a scale, or are maxed out
  (digest tier). A missing metrics API is treated as unavailable evidence, not
  as an application failure.
- Runtime usage: the kubelet Summary API, read through the API server
  proxy, provides actual per-container CPU and memory against declared
  limits, ephemeral-storage usage, and node and volume usage. Missing or
  unauthorised endpoints disable only the affected detector. CPU throttling
  is derived from the same data.
- Application output: when a problem is announced, a short redacted excerpt of
  the crashing container's previous (or current) log is read and added to the
  message. It is bounded to a few containers and lines per announcement.

## Dynamic status

kwatch watches APIService objects and discovers CRDs at runtime. Every served
CRD version with a status subresource is watched for failure-shaped
`Ready=False`, `Available=False`, `Degraded=True`, and `Progressing=False`
conditions. Informational conditions are ignored, and messages and reasons are
kept as evidence. Custom resources therefore cover Gateway API, snapshot, and
operator resources whenever their CRDs expose such conditions.

Built-in APIs introduced in newer Kubernetes versions or protected by feature
gates are capability-aware: if the API is not served, its watcher stays
inactive without creating a false problem.

Some built-in APIs intentionally remain event/relationship based rather than
being treated as condition resources: DRA `ResourceSlice`, `CSINode`,
`VolumeAttributesClass`, and legacy `ReplicationController` do not provide a
stable, universal failure condition that can be alerted on safely. Their
scheduling, driver, and lifecycle failures are still covered when they surface
through Pod/Node status or Kubernetes Warning Events. Legacy `Endpoints` is
deprecated and EndpointSlice remains the authoritative service signal.

## How causes are found

Detecting a symptom is not the same as explaining it. The reasoning rules walk
the relations the model records to find a root that is itself unhealthy or has
just changed:

- owner chains (Pod, ReplicaSet, Deployment, Job, CronJob) and the node a pod
  runs on, with node health, draining, and shared-node failures;
- recent rollouts and changes to Secrets, ConfigMaps, Service selectors,
  NetworkPolicies, and node kubelet versions or taints, including changes made
  while kwatch was down;
- Secrets, ConfigMaps, ServiceAccounts, and image pull Secrets that pods
  reference, and image registries that fail for several workloads;
- Service selectors and EndpointSlices, including unready and terminating
  endpoints, and webhook backends that block admission;
- scheduling constraints, ResourceQuota, PVCs, PVs, StorageClasses, and
  VolumeAttachments;
- cluster DNS, the metrics API behind an HPA, and zone or node pool topology.

When no rule proves a cause, the message says the cause is unknown rather than
naming the nearest object.

## Noise controls

Detection is deliberately separate from notification. A signal never becomes a
message by itself; it joins a problem, and the problem decides when a person
hears about it.

- **Settling**: a new problem waits (75 seconds by default, 15 for page-tier
  problems) to collect related signals, so one root cause produces one
  message. A problem that recovers while settling is never announced.
- **Root-cause grouping**: symptoms attach to the problem of their explained
  root. Forty pods failing because of one node are one problem.
- **Material-change digest**: an announced problem is updated only when tier,
  root, cause, the root's own conditions, or the bucketed size of the impact
  change. Restart counters, timestamps, and replicas failing one by one never
  trigger an update.
- **Adaptive resolve hold**: a problem resolves only after its root stays
  healthy for a hold that doubles with each recent recovery. Failing again
  inside the hold reopens the same problem without a new message.
- **Flapping**: repeated recoveries collapse into one flapping problem whose
  transitions are silent until it is stable.
- **Recurrence and routine**: resolved problems are remembered for a week. One
  that opens at the same time of day on at least three days is treated as
  routine and reported in the digest tier.
- **Tiers**: silent, digest, notify, and page. Planned disruption such as node
  draining, informational signals, and digest-only reasons (certificate
  expiry, CPU throttling or high usage, HPA at maximum, stuck termination) are
  digest tier, which is delivered with `info` severity so a route can send it
  to a quieter channel. A critical failure that reaches users through an
  Ingress, or a lost node, pages. `severityByReason` and `severityByOwnerKind`
  override the derived tier.
- **Startup summary**: problems that already exist when kwatch first starts
  with no saved state are sent as one summary instead of one message each.
  Restored problems never repeat their message after a restart.
- **Scope, silences, and maintenance**: configured namespaces, reasons, the
  namespace label selector, and silence rules drop decisions before delivery,
  and objects, pods, or namespaces annotated for maintenance are held. Dropped
  problems are still tracked so later analysis keeps its evidence.
- **Delivery pacing**: sends to a provider are spaced out, a newer message of
  the same conversation replaces a queued one when the queue is full, and
  overflow is summarised in one digest message.

Security diagnostics include a periodic RBAC self-check. The set of checked
permissions is derived from the resources the sources actually watch, so it
cannot drift from them. Results are available from the diagnostics-protected
`/security` health endpoint; missing permissions are reported as capability
gaps, not problems, so intentionally restricted deployments do not create
alert noise.

## Important boundary

Kubernetes API objects cannot expose every runtime failure. Kubelet health beyond
the summary API, API latency, packet loss,
service-mesh health, cloud-provider volume state,
VPA/KEDA/Cluster Autoscaler internals, and application SLOs require metrics,
logs, traces, or active probes. kwatch consumes the runtime evidence available
through pod/node summaries, active probes, and events; it does not claim that informer status
alone covers these signals.

Individual Pod Security or ValidatingAdmissionPolicy denials are returned in
the API response (and may be present in audit logs), but Kubernetes does not
retain a watchable object for every denied request. kwatch therefore detects
durable policy misconfiguration and resulting object/event symptoms; request-
level denial analytics require an audit-log sink, which is not required.

See the Kubernetes documentation for [Pod lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/),
[Node status](https://kubernetes.io/docs/reference/node/node-status/),
[PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/),
[EndpointSlices](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/),
and [observability](https://kubernetes.io/docs/concepts/cluster-administration/observability/).
