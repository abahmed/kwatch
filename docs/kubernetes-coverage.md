# Kubernetes failure coverage

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
- Cluster resources: exhausted ResourceQuota, stuck Namespace termination, and
  ReplicaSet status failures.
- Resource-level Events: recent failure-shaped Warning Events for scheduling,
  storage attach/provision/mount, autoscaling, admission, discovery, and node
  health are correlated to their involved object. Pod and Cluster Autoscaler
  Events continue through their specialized context-rich detectors.
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

Root-cause analysis also follows projected ConfigMaps/Secrets, image pull
Secrets, CSI drivers, Ingress TLS Secrets, PV StorageClasses, owner chains,
Service selectors, EndpointSlices (including unready/terminating endpoints),
node Leases, generic Custom Resource owners, VolumeAttachments, CSI drivers,
and local PV node affinity.

## Noise and recovery controls

The detection path supports startup baselines, persisted incidents, stable
identity keys, sustained windows, cooldowns, disruption suppression, node
inhibition, resolution, and scope-aware storage checks. Signals are sent through
the correlation engine so live, periodic, startup, and recovery decisions share
the same lifecycle and deduplication rules.

## Important boundary

Kubernetes API objects cannot expose every runtime failure. CPU throttling,
cAdvisor/kubelet health beyond the summary API, API latency, packet loss,
service-mesh health, cloud-provider volume state,
VPA/KEDA/Cluster Autoscaler internals, and application SLOs require metrics,
logs, traces, or active probes. kwatch consumes the runtime evidence available
through pod/node summaries, active probes, and events; it does not claim that informer status
alone covers these signals.

See the Kubernetes documentation for [Pod lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/),
[Node status](https://kubernetes.io/docs/reference/node/node-status/),
[PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/),
[EndpointSlices](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/),
and [observability](https://kubernetes.io/docs/concepts/cluster-administration/observability/).
