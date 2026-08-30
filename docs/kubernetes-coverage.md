# Kubernetes failure coverage

kwatch combines informer state, status conditions, Kubernetes Events, logs,
node summary data, and persisted incident state. No single Kubernetes object
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
  heartbeat staleness, node deletion finalizers, bootstrap grace periods and
  request/allocatable overcommit.
- Storage: mounted volume usage, PVC Pending/Lost and resize/modify errors, PV
  Released/Failed, and stuck PVC/PV finalizers.
- Services and admission backends: EndpointSlice readiness/serving/terminating
  semantics, missing endpoints, named/numeric port publication mismatches, and
  webhook services with no usable endpoints.
- Cluster resources: exhausted ResourceQuota, stuck Namespace termination, and
  ReplicaSet status failures.

## Dynamic status

When enabled through the cluster-resource monitor, kwatch watches APIService
objects and discovers CRDs dynamically. CRD versions with a status subresource
are watched for failure-shaped `Ready=False`, `Available=False`,
`Degraded=True`, and `Progressing=False` conditions. Informational conditions
are ignored, and messages/reasons are preserved as alert evidence.

## Noise and recovery controls

The detection path supports startup baselines, persisted incidents, stable
identity keys, sustained windows, cooldowns, disruption suppression, node
inhibition, resolution, and scope-aware storage checks. Signals are sent through
the correlation engine so live, periodic, startup, and recovery decisions share
the same lifecycle and deduplication rules.

## Important boundary

Kubernetes API objects cannot expose every runtime failure. Actual CPU/memory
usage, throttling, cAdvisor/kubelet health, API latency, DNS resolution, packet
loss, HTTP/TCP reachability, service-mesh health, cloud-provider volume state,
VPA/KEDA/Cluster Autoscaler internals, and application SLOs require metrics,
logs, traces, or active probes. kwatch consumes the runtime evidence available
through pod/node summaries and events; it does not claim that informer status
alone covers these signals.

See the Kubernetes documentation for [Pod lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/),
[Node status](https://kubernetes.io/docs/reference/node/node-status/),
[PersistentVolumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/),
[EndpointSlices](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/),
and [observability](https://kubernetes.io/docs/concepts/cluster-administration/observability/).
