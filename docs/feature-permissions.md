# Feature and Kubernetes permission matrix

The deployment ClusterRole is read-only for observed resources. It also grants
the narrowly scoped `create` operation required for RBAC self-checks.
The namespaced roles are limited to the state lock Lease in the kwatch namespace
and read access to the control-plane Leases in `kube-system`.
Use this matrix when disabling features for a least-privilege deployment.

| Feature | API resources | Access |
| --- | --- | --- |
| Core Pod/workload monitoring | Pods, events, deployments, ReplicaSets, DaemonSets, StatefulSets, Jobs, CronJobs | list/watch |
| Node and cluster resources | Nodes, leases, ResourceQuotas, LimitRanges | list/watch |
| Namespaces | Namespaces | get/list/watch |
| Single-object reads | Nodes (restart evidence, kubelet address) | get |
| Network and admission | Services, EndpointSlices, Ingresses, NetworkPolicies, webhook and admission policy resources | list/watch |
| Storage and PVC | PersistentVolumeClaims, PersistentVolumes, StorageClasses, VolumeAttachments, VolumeSnapshots (and their contents and classes) | list/watch |
| Kubelet stats and logs | `nodes/stats`, `nodes/metrics` (read from each kubelet directly), `pods/log` | get |
| API server and metrics | non-resource URLs `/readyz` and `/metrics`; APIService `v1beta1.metrics.k8s.io`, Services, EndpointSlices | get (URLs); list/watch |
| RBAC health | SelfSubjectAccessReviews | create |
| CRD overlay | KwatchConfig in the kwatch namespace (a Role), CustomResourceDefinitions | list/watch |
| State lock | Lease in the kwatch namespace | create; get/update on kwatch's Lease name only |
| Restart evidence | Pods in the kwatch namespace | get |
| Control-plane health | scheduler and controller-manager Leases in `kube-system` | get |

Endpoints and CSI drivers are not in the least-privilege list: kwatch reads
EndpointSlices instead of Endpoints, and CSI drivers are covered only where
the wildcard grant of `rbac.mode: full` applies.
The default `rbac.mode: full` grants `list` and `watch` on every resource
instead of the per-kind rows above, and still grants `get` only on the
single-object resources listed. A KwatchConfig overlay needs only the
namespaced Role.

Secret read access is on by default because missing Secret references and
certificate expiry need it. Set `watch.secrets: false` to remove every Secret
permission; those checks then report that they cannot verify. Any custom RBAC profile must be validated with
`kubectl auth can-i` in the target cluster.

The raw manifest and Helm chart must keep the lock, state volume, probe,
and security-context settings equivalent. This page is guidance;
rendered manifests and the checked-in RBAC rules are the executable source of
truth.
