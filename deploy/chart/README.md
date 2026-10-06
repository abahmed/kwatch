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
| `config.healthCheck.port` | Health port: container, probe and NetworkPolicy port. The health check cannot be disabled | `8060` |
| `networkPolicy.enabled` | Render a NetworkPolicy that admits the health port and allows DNS, `networkPolicy.apiServerPorts` and kubelet (`networkPolicy.kubeletPort`, default 10250) egress | `false` |
| `networkPolicy.allowDefaults` | Admit the health port and open DNS, `apiServerPorts`, kubelet and DNS metrics egress. `false` only when your own rules cover the API server, kubelets and probes | `true` |
| `networkPolicy.apiServerPorts` | TCP ports open to any address for the API server and HTTPS providers; at least one is required (an empty list would open all egress) | `[443, 6443]` |
| `networkPolicy.kubeletPort` | Kubelet HTTPS port opened for node stats | `10250` |
| `networkPolicy.dnsMetricsPort` | Cluster DNS metrics port opened to the DNS pods in `kube-system` (DNS request and SERVFAIL rates); `0` omits it | `9153` |
| `networkPolicy.allowProbeEgressAll` | Allow all egress from the pod. Needed for active probes, `autoServices`, `autoDependencies`, a proxy or providers on unusual ports unless you open them one by one; `helm install` warns when these are configured with the policy on | `false` |
| `networkPolicy.ingress` / `networkPolicy.egress` | Extra full NetworkPolicy rules, for example to limit a port to one address | `[]` |
| `networkPolicy.extraEgressPorts` | Extra TCP egress ports open to any address, for providers not on 443: SMTP (`25`, `465` or `587`) for email, or a webhook's custom port | `[]` |
| `networkPolicy.kubeletCIDRs` | Node address ranges the kubelet egress rule is limited to; empty allows any address | `[]` |
| `image.repository` / `image.tag` / `image.pullPolicy` | Container image; an empty tag uses the chart's `appVersion` | `ghcr.io/abahmed/kwatch` / `""` / `IfNotPresent` |
| `crd.install` | Install the shared `KwatchConfig` CRD with this release; skipped automatically when another release owns it. `config.crd.enabled` is separate: it turns the live overlay on | `true` |
| `rbac.mode` | `full` grants read-only `list`/`watch` on every resource (and `get` only on namespaces, nodes, `nodes/stats`, `nodes/metrics` and `pods/log`); `least-privilege` grants an explicit `list`/`watch` set, and kinds outside it show as `permission_denied` in `/health` coverage | `full` |
| `persistence.size` | Size of the state volume (PVC). The state file is capped at 512 MiB of logical data (evidence 128 MiB), is rewritten at startup when it grows more than a quarter past that, and no backups are kept; the file and the temporary rewrite copy share the volume | `2Gi` |
| `persistence.existingClaim` | Reuse an existing PVC for the state file | `""` |
| `persistence.emptyDir` | Keep state in an `emptyDir` instead of a PVC. All state is lost when the Pod is rescheduled, so use it only for evaluation; installing with it prints a warning | `false` |
| `persistence.pinToStorageDriverNodes` | When the StorageClass's CSI driver is registered on only some nodes, require the driver's topology label so the state claim can bind where the Pod lands. Looked up at install/upgrade; `helm template` and Argo CD renders cannot look it up | `true` |
| `persistence.storageDriverTopologyKey` | The driver's topology label to require explicitly (for offline renders), for example `topology.ebs.csi.aws.com/zone` | `""` |
| `persistence.emptyDirSizeLimit` | `sizeLimit` of the `emptyDir`; also passed to kwatch as `KWATCH_VOLUME_LIMIT` so the startup rewrite checks the real limit | `2Gi` |
| `watch.secrets` | Watch Secrets; `false` removes all Secret RBAC (in both `rbac.mode` values) and Secret-based checks report that they cannot verify. Secrets are watched by default, which lets the kwatch ServiceAccount list and watch Secrets cluster-wide; set `watch.secrets: false` to remove all Secret RBAC and the Secret watch. | `true` |
| `resources` | CPU and memory requests/limits; kwatch sets `GOMEMLIMIT` to 90% of the memory limit | `100m` CPU request, `500m` CPU and `512Mi` memory limit |
| `securityContext.runAsUser` / `securityContext.runAsGroup` | Numeric user and group of the container; they match the image's `kwatch` user | `1000` / `1000` |
| `podSecurityContext.fsGroup` | Group that owns the state volume | `1000` |
| `persistence.storageClass` | StorageClass of the state claim; empty uses the cluster default | `""` |
| `securityContext.runAsNonRoot` | Run without root privileges | `true` |
| `securityContext.readOnlyRootFilesystem` | Use a read-only root filesystem | `true` |
| `securityContext.allowPrivilegeEscalation` | Prevent privilege escalation | `false` |
| `securityContext.capabilities.drop` | Linux capabilities removed from the container | `[ALL]` |
| `securityContext.seccompProfile.type` | Seccomp profile | `RuntimeDefault` |
| `podAnnotations` | Pod annotations; the defaults ask node autoscalers not to evict kwatch (see below), and your keys are merged with them | `karpenter.sh/do-not-disrupt: "true"`, `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"` |
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
and a gap in monitoring. The default `podAnnotations` are the plain
annotations `karpenter.sh/do-not-disrupt: "true"` and
`cluster-autoscaler.kubernetes.io/safe-to-evict: "false"`. They only ask a
node autoscaler to leave the pod alone, and they do nothing where none reads
them. To opt out, remove a key (`--set 'podAnnotations.karpenter\.sh/do-not-disrupt=null'`) or set the
permissive value: `"false"` for `karpenter.sh/do-not-disrupt`, `"true"` for
`cluster-autoscaler.kubernetes.io/safe-to-evict`. (`"false"` on `safe-to-evict`
is the protective value.)

Delivery is at least once: queued notifications are kept in a persisted outbox
(at most 2048 jobs, none older than 24 hours), so a restart does not lose them,
and a send interrupted mid-request may repeat.

Adoption telemetry is on by default: an anonymous installation ID and the kwatch
version, sent at most once every 7 days. Set `telemetry.enabled: false` in
`config` (or in the Secret's `config.yaml`) to turn it off (see the [configuration reference](../../docs/configuration.md#-minimal-adoption-telemetry)).

The full configuration reference is in [`docs/configuration.md`](../../docs/configuration.md).
