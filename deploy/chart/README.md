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
configuration; `.Values.config` is not merged into it.

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

The pod should show `READY 1/1` and `STATUS Running`.

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
| `service.port` | Health-check port | `8060` |
| `resources` | CPU and memory requests/limits | `100m` CPU, `256Mi` memory |
| `securityContext.runAsNonRoot` | Run without root privileges | `true` |
| `securityContext.readOnlyRootFilesystem` | Use a read-only root filesystem | `true` |
| `securityContext.allowPrivilegeEscalation` | Prevent privilege escalation | `false` |
| `securityContext.capabilities.drop` | Linux capabilities removed from the container | `[ALL]` |
| `securityContext.seccompProfile.type` | Seccomp profile | `RuntimeDefault` |
| `podAnnotations` | Additional Pod annotations | `{}` |
| `podLabels` | Additional Pod labels | `{}` |
| `nodeSelector` | Choose nodes by label | `{}` |
| `tolerations` | Allow configured taints | `[]` |
| `affinity` | Control Pod placement | `{}` |
| `upgrader.disableUpdateCheck` | Disable the startup update check | `false` |

The full configuration reference is in [`docs/configuration.md`](../../docs/configuration.md).
