# 📦 kwatch Helm chart

The Helm chart installs kwatch and its Kubernetes resources in one release.
Use it when you manage cluster configuration with Helm or GitOps.

If you want the fewest decisions, use the [interactive manager](https://kwatch.dev/docs/kwatch-manager).

## ✅ Requirements

- Helm 3 or newer
- A supported Kubernetes cluster
- Permission to create resources in the target namespace

## 🚀 Install

Add the kwatch chart repository:

```bash
helm repo add kwatch https://kwatch.dev/charts
helm repo update
```

Create a local `config.yaml`. Credentials must be file references:

```yaml
crd:
  enabled: true
alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
app:
  clusterName: "production"
```

Create an existing Secret from that file and a local credential file:

```bash
kubectl create namespace kwatch --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace kwatch \
  pod-security.kubernetes.io/enforce=restricted \
  pod-security.kubernetes.io/audit=restricted \
  pod-security.kubernetes.io/warn=restricted --overwrite
kubectl -n kwatch create secret generic kwatch-config \
  --from-file=config.yaml \
  --from-file=slack-webhook
```

Then set the Secret name in `values.yaml`:

```yaml
configSecretName: kwatch-config
```

When `configSecretName` is set, the Secret's `config.yaml` is the complete base
configuration; `.Values.config` is not merged into it. The chart cannot read
that file, so keep `config.healthCheck.port` in your values equal to the
Secret's port. The Pod restarts on a Secret change only when Helm can `lookup`
the Secret on a live cluster; offline renders (`helm template`, Argo CD) hash
the Secret name instead, so run `kubectl rollout restart` after editing it.

Install it:

```bash
helm install kwatch kwatch/kwatch \
  --namespace kwatch \
  --values values.yaml
```

The public chart repository contains stable releases. To test a release
candidate, use its tagged Kubernetes manifests instead.

## 🔎 Verify the install

```bash
kubectl get pods -n kwatch
kubectl logs -n kwatch deployment/kwatch
```

The pod should show `READY 1/1` and `STATUS Running`. A Pod that stays
`Pending` while `kubectl get pvc -n kwatch` shows the claim `Pending` is
waiting for storage: `kubectl describe pvc -n kwatch kwatch-data` names the
cause. `no topology key found for node` means the StorageClass's CSI driver
runs on only some nodes. `helm install` and `helm upgrade` detect this and
pin the Pod to those nodes (`persistence.pinToStorageDriverNodes`); an offline
render needs `persistence.storageDriverTopologyKey`, or use
`persistence.emptyDir` (see the production guide, "Volume requirements").

## ⬆️ Upgrade

```bash
helm repo update
helm upgrade kwatch kwatch/kwatch \
  --namespace kwatch \
  --values values.yaml
```

When a license key is configured (`license.key`, `license.existingSecret` or
`license.instanceSecret: true`), the chart ships the empty Secret
`kwatch-license-instance` and a Role that gives kwatch get and update on that
one name (no `create` on Secrets). Without a key neither is rendered. Set
`license.instanceSecret: true` when the key reaches kwatch another way, for
example inside the `configSecretName` Secret. When the key lives in
`license.existingSecret`, the chart rolls the pod when that Secret's content
changes, but only where Helm `lookup` can read it (a live `helm install` or
`upgrade`); an offline render (`helm template`, Argo CD) hashes the Secret's
name, so restart the Deployment yourself after changing the key. The chart never
renders `data` for it and keeps it on uninstall (`helm.sh/resource-policy:
keep`). Because neither the previous nor the new manifest has `data`, Helm's
three-way merge leaves the instance key kwatch wrote alone on upgrade. If you
install without this chart's
templates, create that Secret empty (type Opaque) yourself, or the license
stays pending and Community. Besides the instance key, `kwatch license
deactivate` writes a `deactivated-at` field to the same Secret so the running
leader drops its lease and falls back to Community. Licenses use 3 day leases
with a 4 day grace, so a cluster that cannot reach the license server keeps
Pro for at most 7 days.

Helm upgrades the `KwatchConfig` CRD automatically. The chart keeps the CRD
when the release is removed so configuration resources are not deleted by
surprise.

The CRD is cluster-wide, so only one release can own it. Helm stamps it with
the first release that created it, and that release keeps managing it. When
you install a second release (another name or namespace), the chart looks the
CRD up and, if another release owns it, leaves it alone, so the install does
not fail with "exists and cannot be imported". Offline renders (`helm
template`, Argo CD) cannot look it up and always include it; set
`crd.install: false` on every release but one there, or when you apply
[`deploy/crd.yaml`](../crd.yaml) yourself. The CRD lives in `templates/`, not
in `crds/`, because Helm never upgrades `crds/`, and this CRD's schema changes
with kwatch. Upgrade notes: the schema now rejects a `resyncSeconds` that is
not 0 or at least 30, an `app.logFormatter` other than `text` or `json`, and
any `healthCheck` field, matching what kwatch itself accepts; an existing
`KwatchConfig` that breaks these rules is rejected on its next edit and, if
already stored with a `healthCheck` section, has it pruned.

The chart uses `/availabilityz` for the Kubernetes Deployment probe because a
Pod still waiting for the state lock is intentionally not
monitoring-ready. Use `/readyz` when checking whether the state lock holder
can monitor the cluster.

## 🧹 Uninstall

```bash
helm uninstall kwatch --namespace kwatch
```

The Secret `kwatch-license-instance` (the license instance identity) is
annotated `helm.sh/resource-policy: keep`, so uninstall leaves it in place and
a reinstall keeps the same identity. kwatch also keeps a copy of its signed
Pro lease and clock guard there (field `lease-cache`), so Pro offline grace
survives a lost data volume, including `persistence.emptyDir`. Delete the
Secret yourself to start fresh:

```bash
kubectl delete secret kwatch-license-instance --namespace kwatch
```

Delete the namespace only if it contains no other resources you want to keep:

```bash
kubectl delete namespace kwatch
```

## ⚙️ Values

| Value | Purpose | Default |
| --- | --- | --- |
| `config` | Non-sensitive kwatch configuration | `{crd: {enabled: true}}` |
| `configSecretName` | Existing Secret containing `config.yaml` | `""` |
| `config.resyncSeconds` | Informer resync, a safety net for lost watch events: `0` (off) or at least `30`; the schema rejects other values | `300` |
| `config.app.logFormatter` | `text` or `json` | `text` |
| `config.app.caBundlePath` / `config.app.insecureSkipTLSVerify` | Extra PEM CA bundle for outbound HTTP (added to the system roots) and a switch that skips TLS verification (testing only). Not settable from a `KwatchConfig` | `""` / `false` |
| `license.existingSecret` | Existing Secret with the key `license-key`, read as `KWATCH_LICENSE_KEY` (optional: the pod starts as Community without it) | `""` |
| `license.key` | The license key itself (discouraged: it lands in release history). When set and `existingSecret` is empty, the chart creates the Secret `<release>-license` | `""` |
| `license.instanceSecret` | Render the license instance Secret and its Role even though no key is set through the chart (the key reaches kwatch another way). Otherwise they are rendered only with `license.key` or `license.existingSecret` | `false` |
| `config.healthCheck.statusEndpoint` | Serve the read-only `/status` view (open incident object names, no authentication) on the health port. Off by default; with the NetworkPolicy on it is limited like the rest of the port. Reach it with `kubectl port-forward` | `false` |
| `config.healthCheck.port` | Health port: container, probe and NetworkPolicy port. The health check cannot be disabled | `8060` |
| `networkPolicy.enabled` | Render a NetworkPolicy that admits the health port (own namespace, `kubeletCIDRs` and `healthIngressFrom` only) and allows DNS, `networkPolicy.apiServerPorts` and kubelet (`networkPolicy.kubeletPort`, default 10250) egress | `false` |
| `networkPolicy.allowDefaults` | Admit the health port and open DNS, `apiServerPorts`, kubelet and DNS metrics egress. `false` only when your own rules cover the API server, kubelets and probes. With `license.key` or `license.existingSecret` set it also opens TCP 443 to any address for license activation and renewal, even if `apiServerPorts` omits 443 | `true` |
| `networkPolicy.apiServerPorts` | TCP ports open to any address for the API server and HTTPS providers; at least one is required (an empty list would open all egress) | `[443, 6443]` |
| `networkPolicy.kubeletPort` | Kubelet HTTPS port opened for node stats | `10250` |
| `networkPolicy.dnsMetricsPort` | Cluster DNS metrics port opened to the DNS pods in `kube-system` (DNS request and SERVFAIL rates); `0` omits it | `9153` |
| `networkPolicy.allowProbeEgressAll` | Allow all egress from the pod. Needed for active probes, `autoServices`, `autoDependencies`, a proxy or providers on unusual ports unless you open them one by one; `helm install` warns when these are configured with the policy on | `false` |
| `networkPolicy.ingress` / `networkPolicy.egress` | Extra full NetworkPolicy rules, for example to limit a port to one address | `[]` |
| `networkPolicy.extraEgressPorts` | Extra TCP egress ports open to any address, for providers not on 443: SMTP (`25`, `465` or `587`) for email, or a webhook's custom port | `[]` |
| `networkPolicy.kubeletCIDRs` | Node address ranges the kubelet egress rule is limited to; empty allows any address. Also admitted to the health port, for kubelet probes on a network plugin that enforces policy on node traffic | `[]` |
| `networkPolicy.healthIngressFrom` | Extra peers (`namespaceSelector`, `podSelector` or `ipBlock`) that may reach the health port (`/health`, `/metrics`, `/status`), such as your Prometheus namespace. Without it only pods in the release namespace (and `kubeletCIDRs`) can | `[]` |
| `image.repository` / `image.tag` / `image.pullPolicy` | Container image; an empty tag uses the chart's `appVersion` | `ghcr.io/abahmed/kwatch` / `""` / `IfNotPresent` |
| `crd.install` | Install the shared `KwatchConfig` CRD with this release; skipped automatically when another release owns it. `config.crd.enabled` is separate: it turns the live overlay on | `true` |
| `rbac.mode` | `least-privilege` grants an explicit read-only `list`/`watch` set, and kinds outside it (custom resources included) show as `permission_denied` in `/health` coverage (kwatch logs one warning about them and the install notes say so); `full` grants `list`/`watch` on every resource, except that `full` with `watch.secrets: false` renders the least-privilege list. Both grant `get` only on namespaces, nodes, `nodes/stats`, `nodes/metrics` and `pods/log`. Set `full` to watch custom resources | `least-privilege` |
| `persistence.size` | Size of the state volume (PVC). The state file is capped at 512 MiB of logical data (evidence 128 MiB), is rewritten at startup when it grows more than a quarter past that, and no backups are kept; the file and the temporary rewrite copy share the volume | `2Gi` |
| `persistence.existingClaim` | Reuse an existing PVC for the state file | `""` |
| `persistence.emptyDir` | Keep state in an `emptyDir` instead of a PVC. All state is lost when the Pod is rescheduled, so use it only for evaluation; installing with it prints a warning | `false` |
| `persistence.pinToStorageDriverNodes` | When the StorageClass's CSI driver is registered on only some nodes, require the driver's topology label so the state claim can bind where the Pod lands. Looked up at install/upgrade; `helm template` and Argo CD renders cannot look it up | `true` |
| `persistence.storageDriverTopologyKey` | The driver's topology label to require explicitly (for offline renders), for example `topology.ebs.csi.aws.com/zone` | `""` |
| `persistence.emptyDirSizeLimit` | `sizeLimit` of the `emptyDir`; also passed to kwatch as `KWATCH_VOLUME_LIMIT` so the startup rewrite checks the real limit | `2Gi` |
| `watch.secrets` | Watch Secrets; `false` removes all Secret RBAC (in both `rbac.mode` values) and Secret-based checks report that they cannot verify. Secrets are watched by default, which lets the kwatch ServiceAccount list and watch Secrets cluster-wide; set `watch.secrets: false` to remove all Secret RBAC and the Secret watch. | `true` |
| `resources` | CPU and memory requests/limits; kwatch sets `GOMEMLIMIT` to 90% of the memory limit. Live heap is about 26 KB per pod: the default 512Mi suits up to about 8,000 pods, 10,000 pods need `768Mi` and 20,000 need `1Gi` (see docs/production-operations.md, Memory) | `100m` CPU request, `500m` CPU and `512Mi` memory limit |
| `securityContext.runAsUser` / `securityContext.runAsGroup` | Numeric user and group of the container; they match the image's `kwatch` user | `1000` / `1000` |
| `podSecurityContext.fsGroup` | Group that owns the state volume | `1000` |
| `persistence.storageClass` | StorageClass of the state claim; empty uses the cluster default | `""` |
| `securityContext.runAsNonRoot` | Run without root privileges | `true` |
| `securityContext.readOnlyRootFilesystem` | Use a read-only root filesystem | `true` |
| `securityContext.allowPrivilegeEscalation` | Prevent privilege escalation | `false` |
| `securityContext.capabilities.drop` | Linux capabilities removed from the container | `[ALL]` |
| `securityContext.seccompProfile.type` | Seccomp profile | `RuntimeDefault` |
| `podAnnotations` | Pod annotations; the default asks the cluster autoscaler not to scale down kwatch's node (see below), and your keys are merged with it | `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"` |
| `priorityClassName` | PriorityClass of the kwatch pod, so the pod that reports failures is not the first one preempted | `""` |
| `imagePullSecrets` | Pull secrets for the kwatch image (a private registry mirror); a list of `{name: <secret>}` | `[]` |
| `extraVolumes` / `extraVolumeMounts` | Extra volumes and mounts of the kwatch container, for example a Secret holding the CA bundle `config.app.caBundlePath` names. The names `config-volume` and `data` and the paths `/config` and `/var/lib/kwatch` are taken | `[]` |
| `podLabels` | Additional Pod labels | `{}` |
| `nodeSelector` | Choose nodes by label | `{}` |
| `tolerations` | Allow configured taints | `[]` |
| `affinity` | Control Pod placement; also the way to keep kwatch on nodes where the StorageClass's CSI driver is registered when the state claim stays `Pending` | `{}` |
| `config.upgrader.disableUpdateCheck` | Disable the update check (GitHub API at startup, then daily); with `configSecretName`, set `upgrader.disableUpdateCheck` in the Secret's `config.yaml` | `false` |
| `config.kubelet.insecureSkipVerify` | Skip kubelet serving-certificate verification for node stats; only for kubelets with self-signed certificates | `false` |
| `defaultTolerations` | Not-ready/unreachable `NoExecute` tolerations (30s) merged after `tolerations`, so kwatch leaves a failed node quickly | 30s not-ready and unreachable |
| `terminationGracePeriodSeconds` | Time for workers and savers to stop; must be at least 60 | `60` |
| `sacAnnotations` | Annotations for the kwatch ServiceAccount (for example a cloud IAM role) | `{}` |
| `nameOverride` | Override the chart name used in resource names and labels | `""` |
| `fullnameOverride` | Override the full resource name prefix | `""` |

Kwatch writes one state file under one lock, so an eviction means a restart
and a gap in monitoring, but the restart resumes from the volume. The default
`podAnnotations` carry only `cluster-autoscaler.kubernetes.io/safe-to-evict:
"false"`, a plain annotation that asks the cluster autoscaler not to scale
down kwatch's node; remove it (`--set
'podAnnotations.cluster-autoscaler\.kubernetes\.io/safe-to-evict=null'`) where
scale-down matters more. The chart does **not** set
`karpenter.sh/do-not-disrupt`: it pins the node and Karpenter reports it as
`DisruptionBlocked` for as long as the pod runs. Ask for it yourself with
`--set-string 'podAnnotations.karpenter\.sh/do-not-disrupt=true'`. The chart
ships no PodDisruptionBudget: on a single pod `minAvailable: 1` would block
every drain of its node, and `maxUnavailable: 1` allows the one eviction a
drain needs. Create your own if you want one.

Delivery is at least once: queued notifications are kept in a persisted outbox
(at most 2048 jobs, none older than 24 hours), so a restart does not lose them,
and a send interrupted mid-request may repeat.

Adoption telemetry is on by default: an anonymous installation ID (with its
own salt, not the license's cluster ID) and the kwatch version, sent at most
once every 7 days. With a Pro license kwatch also checks in every 30 minutes
with a hashed cluster ID, the version and the node count (never cluster
names); set `clusterCheckins: false` in
`config` or `KWATCH_CLUSTER_CHECKINS=false` to turn that off. Set
`telemetry.enabled: false` in `config` (or in the Secret's `config.yaml`) to
turn telemetry off (see the [configuration reference](../../docs/configuration.md#-minimal-adoption-telemetry)).

The full configuration reference is in [`docs/configuration.md`](../../docs/configuration.md).
