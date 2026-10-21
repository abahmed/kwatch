# Feature and Kubernetes permission matrix

The deployment ClusterRole is read-only for observed resources. It also grants
the narrowly scoped `create` operation required for RBAC self-checks.
The namespaced roles are limited to the state lock Lease in the kwatch namespace,
`get` on pods in that namespace (restart evidence), read access to the
control-plane Leases in `kube-system`, the KwatchConfig overlay, and, only
when a license key is configured, `get` and `update` on the one license
instance Secret.
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
| Finalizer handler evidence | Roles, RoleBindings, ClusterRoles, ClusterRoleBindings | list/watch |
| RBAC health | SelfSubjectAccessReviews | create |
| CRD overlay | KwatchConfig in the kwatch namespace (a Role), CustomResourceDefinitions | list/watch |
| State lock | Lease in the kwatch namespace | create; get/update on kwatch's Lease name only |
| License instance key | Secret `kwatch-license-instance` in the kwatch namespace (Role `<release>-license`, rendered only with a license key) | get/update on that name only |
| Built-in classes read through the dynamic source | PriorityClasses, RuntimeClasses, IngressClasses, ReplicationControllers | list/watch |
| Restart evidence | Pods in the kwatch namespace | get |
| Control-plane health | scheduler and controller-manager Leases in `kube-system` | get |

Roles, RoleBindings, ClusterRoles and ClusterRoleBindings are watched as full
objects (the API server sends their rules), but kwatch keeps only the custom
API groups a role may update or patch and the ServiceAccounts a binding grants
it to. That is how it tells which workload serves a stuck finalizer's API
group; it never keeps resource names, other subjects or Secret data. Without
the permission, a stuck finalizer is still reported, but no workload is named
as its controller by the roles it runs with.

Endpoints and CSI drivers are not in the least-privilege list: kwatch reads
EndpointSlices instead of Endpoints, and CSI drivers are covered only where
the wildcard grant of `rbac.mode: full` applies.
The chart's default `rbac.mode` is `least-privilege`: it grants `list` and
`watch` on the per-kind rows above only, so custom resources are not watched
and show as `permission_denied` in the `/health` coverage (kwatch logs one
warning about them after its first discovery, and the install notes say so).
`rbac.mode: full` (and the raw `deploy/deploy.yaml`) grants `list` and `watch`
on every resource instead, and still grants `get` only on the single-object
resources listed. `rbac.mode: full` together with `watch.secrets: false` is
least-privilege: RBAC cannot remove Secrets from a wildcard. A KwatchConfig
overlay needs only the namespaced Role.

Secret read access is on by default because missing Secret references and
certificate expiry need it. **By default kwatch's ServiceAccount can read
every Secret in the cluster**; kwatch keeps only hashes, but the permission
exists in both RBAC modes (`rbac.mode: full` also grants every other kind).
Set `watch.secrets: false` to remove every Secret permission; those checks
then report that they cannot verify. `rbac.mode: least-privilege` alone does
not remove it. The chart prints this warning on install and kwatch logs it
once at startup. The Community edition runs no detector on Secrets, but it
keeps the Secret watch because its root-cause engine reads Secrets to name a
missing Secret reference; with `watch.secrets: false` that explanation is
given up. The one Secret permission that stays with `watch.secrets: false` is
the license instance key, and only when a license key is configured.

Any custom RBAC profile must be validated with `kubectl auth can-i` in the
target cluster.

The raw manifest and Helm chart must keep the lock, state volume, probe,
and security-context settings equivalent. This page is guidance;
rendered manifests and the checked-in RBAC rules are the executable source of
truth.
