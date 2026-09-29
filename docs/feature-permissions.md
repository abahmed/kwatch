# Feature and Kubernetes permission matrix

The deployment ClusterRole is read-only for observed resources. It also grants
the narrowly scoped `create` operation required for RBAC self-checks.
The namespaced roles are limited to the Lease lock in the kwatch namespace and
read access to the control-plane leader Leases in `kube-system`.
Use this matrix when disabling features for a least-privilege deployment.

| Feature | API resources | Access |
| --- | --- | --- |
| Core Pod/workload monitoring | Pods, events, deployments, ReplicaSets, DaemonSets, StatefulSets, Jobs, CronJobs | get/list/watch |
| Node and cluster resources | Nodes, leases, namespaces, ResourceQuotas, LimitRanges | get/list/watch |
| Network and admission | Services, Endpoints, EndpointSlices, Ingresses, NetworkPolicies, webhook and admission policy resources | get/list/watch |
| Storage and PVC | PersistentVolumeClaims, PersistentVolumes, StorageClasses, VolumeAttachments, CSI drivers, snapshots | get/list/watch |
| Kubelet and logs | `nodes/proxy`, `pods/log`, `pods/proxy` | get/list/watch |
| Metrics API evidence | APIService `v1beta1.metrics.k8s.io`, Services, EndpointSlices | get/list/watch |
| RBAC health | SelfSubjectAccessReviews | create |
| CRD overlay | KwatchConfig and CustomResourceDefinitions | get/list/watch |
| Lease lock | Lease in the kwatch namespace | create/get/update |
| Control-plane health | scheduler and controller-manager Leases in `kube-system` | get |

Secret read access remains because TLS monitoring and dependency-graph
references can require Secret-backed endpoints. Operators that do not use those
features should remove that rule in a custom ClusterRole and disable the
corresponding monitors. Any custom RBAC profile must be validated with
`kubectl auth can-i` in the target cluster.

The raw manifest and Helm chart must keep the lock, state volume, probe,
and security-context settings equivalent. This page is guidance;
rendered manifests and the checked-in RBAC rules are the executable source of
truth.
