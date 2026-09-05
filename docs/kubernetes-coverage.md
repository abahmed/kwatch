# 🎯 Kubernetes failure coverage

This page answers one question: **what can kwatch notice?** It is a technical
reference for operators who want to understand the signals behind an alert.
For a quick overview, see the [README](../README.md).

kwatch combines informer state, status conditions, Kubernetes Events, logs,
node summary data, active probes, and persisted incident state. It does not
require Prometheus, Grafana, or another external monitoring product. No single Kubernetes object
status proves that application traffic, DNS, or runtime health is working, so
the categories below intentionally use different signal sources.

## Object and lifecycle signals

- Pods: Pending and every `PodScheduled=False` reason, `PodReadyToStartContainers`,
  `DisruptionTarget`, Failed/Unknown phases, init containers, waiting and
  terminated container states, restart transitions, CrashLoopBackOff, image
  pull/config/create/sandbox/probe/lifecycle-hook failures, OOM and eviction
  evidence, ephemeral-storage and filesystem messages, and stuck pod deletion
  finalizers.
- Deployments, ReplicaSets, StatefulSets, DaemonSets, Jobs and CronJobs: rollout
  and availability conditions, replica failure, scheduling/taint symptoms,
  partition/update state, Job deadline/backoff/suspension and indexed-job
  evidence, plus CronJob suspension, missed schedule, concurrency and starting
  deadline handling.
- Nodes: Ready, memory/disk/PID/network pressure, sustained pressure, lease
  heartbeat staleness, node deletion finalizers, bootstrap grace periods,
  request/allocatable overcommit, and optional kubelet-summary filesystem and
  inode usage thresholds.
- Storage: mounted volume usage, PVC Pending/Lost, filesystem-resize and
  controller/node resize failures, modify-volume failures, PV
  Released/Failed (including status reason/message), and stuck PVC/PV
  finalizers.
- Services and admission backends: EndpointSlice readiness/serving/terminating
  semantics, missing endpoints, named/numeric port publication mismatches,
  LoadBalancer provisioning and Service failure conditions, and webhook services
  with no usable endpoints.
- Cluster resources: exhausted ResourceQuota, contradictory LimitRange
  constraints, stuck Namespace termination, and ReplicaSet status failures.
- Resource-level Events: recent failure-shaped Warning Events for scheduling,
  storage attach/provision/mount, autoscaling, admission, discovery, and node
  health are correlated to their involved object. Pod and Cluster Autoscaler
  Events continue through their specialized context-rich detectors.
- Security and admission: RBAC capability self-checks, missing ServiceAccounts,
  required Secret/ConfigMap references in volumes and environment injection,
  unreachable mutating/validating webhook backends, malformed Pod Security
  Admission namespace labels, and ValidatingAdmissionPolicy type-checking
  warnings or bindings that reference a missing policy. These signals preserve
  the exact object and reference name so the alert points to the correction.
- Built-in platform APIs outside the main workload pipelines: MutatingAdmissionPolicy,
  CertificateSigningRequest/PodCertificateRequest, legacy Endpoints, and API Priority and Fairness
  (FlowSchema/PriorityLevelConfiguration) failure conditions are watched when
  their API and feature gate are present.
- Control plane: active `/readyz` availability and latency checks for the API
  server, health endpoint checks for scheduler/controller-manager/etcd when
  their Pods are visible, and a diagnostic informer status with received-event
  freshness plus watch-interruption counters. Component absence is reported as
  unsupported rather than failure because managed control planes are commonly
  hidden from tenant RBAC.
- DNS: an in-cluster lookup of `kubernetes.default.svc` validates the complete
  CoreDNS/service-discovery path from the kwatch Pod, not merely the CoreDNS
  Pod phase.
- Services: optional `activeProbeMonitor.autoServices` performs in-cluster TCP
  checks for advertised Service ports and HTTP checks for ports named `http*`,
  connecting the result back to the Service in the dependency graph. It is
  opt-in because a declared port is not proof that an application listener is
  intended.
- Application probes can enforce per-target HTTP latency warning/critical
  thresholds in addition to status-code checks, giving kwatch a lightweight
  SLO signal without a metrics server.
- Autoscaling: HPA condition details remain informer-native, while
  objects are covered by the dynamic CRD status watcher whenever their CRDs
  expose failure-shaped conditions. Missing metrics APIs are treated as an
  unavailable optional capability, not as an application incident.
- Storage lifecycle: CSI VolumeAttachment attach errors and CSI snapshot
  errors now produce incidents with their controller-provided reason/message;
  mount/unmount and provisioning failures continue to come from Kubernetes
  Warning Events and PVC/PV status.
- Runtime metrics: built-in kubelet Summary API collection of actual
  per-container CPU and memory usage against declared limits. The optional
  Metrics API integration is retained for users who already run Metrics Server,
  but is disabled by default and is not required.
- Active probes: opt-in HTTP, TCP, and DNS checks for explicitly configured
  targets through `activeProbeMonitor`, with consecutive-failure and recovery
  thresholds. Targets are never inferred automatically from Services.
- Kubelet telemetry: built-in kubelet `stats/summary` and
  `metrics/cadvisor` are queried directly through the API server proxy for PSI,
  node network/runtime error rates, per-container CPU/memory/ephemeral-storage
  usage, and CPU throttling. Missing or unauthorized endpoints disable only the affected
  detector and are logged at diagnostic verbosity; they do not create false
  incidents. New telemetry signals require consecutive samples before firing
  and recovery samples before resolving.

## Dynamic status

When enabled through the cluster-resource monitor, kwatch watches APIService
objects and discovers CRDs dynamically. Every served CRD version with a status
subresource is watched for failure-shaped `Ready=False`, `Available=False`,
`Degraded=True`, and `Progressing=False` conditions. Informational conditions
are ignored, and messages/reasons are preserved as alert evidence.
These rules can be customized through `crd.failureConditions` for operators
with different condition semantics.

Built-in APIs introduced in newer Kubernetes versions or protected by feature
gates are capability-aware: if the API is not served, its watcher remains
inactive without creating a false incident.

Some built-in APIs intentionally remain event/relationship based rather than
being treated as condition resources: DRA `ResourceClaim`/`ResourceSlice`,
`CSINode`, `VolumeAttributesClass`, and legacy `ReplicationController` do not
provide a stable, universal failure condition that can be alerted on safely.
Their scheduling, driver, and lifecycle failures are still covered when they
surface through Pod/Node status or Kubernetes Warning Events. Legacy `Endpoints`
is deprecated and EndpointSlice remains the authoritative service signal.

Root-cause analysis also follows projected ConfigMaps/Secrets, image pull
Secrets, CSI drivers, Ingress TLS Secrets, PV StorageClasses, owner chains,
Service selectors, EndpointSlices (including unready/terminating endpoints),
node Leases, generic Custom Resource owners, VolumeAttachments, CSI drivers,
VolumeSnapshots, VolumeSnapshotContents, VolumeSnapshotClasses, and local PV
node affinity. Gateway API routes are linked to Gateway/GatewayClass, backend
Services, and listener TLS Secrets; Ingress is linked to IngressClass. Explicit
HTTP/TCP/DNS probes are also linked to matching Kubernetes Service DNS names or
kept as external network targets. Generic CRD references can be configured as
`crd.graphReferences` paths such as `spec.backendRefs.name=service`; arrays are
traversed automatically.

## Noise and recovery controls

The detection path supports startup baselines, persisted incidents, stable
identity keys, sustained windows, cooldowns, disruption suppression, node
inhibition, resolution, and scope-aware storage checks. Signals are sent through
the correlation engine so live, periodic, startup, and recovery decisions share
the same lifecycle and deduplication rules.

Security diagnostics include a periodic RBAC self-check for the cluster-scoped
and selected namespace-scoped permissions needed by the enabled monitors.
Explicitly allowed namespaces are checked; with cluster-wide scope, the kwatch
namespace is checked to keep the operation bounded on large clusters. Results
are available from the diagnostics-protected `/security` health endpoint;
missing permissions are reported as capability gaps, not incidents, so
intentionally restricted deployments do not create alert noise.

## Important boundary

Kubernetes API objects cannot expose every runtime failure. CPU throttling,
cAdvisor/kubelet health beyond the summary API, API latency, packet loss,
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
