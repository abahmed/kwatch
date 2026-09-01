# kwatch Helm Chart

monitor all changes in your Kubernetes(K8s) cluster, detects crashes in your running apps in realtime, and publishes notifications to your channels (Slack,
Discord, etc.) instantly

## Add Repository

```console
helm repo add kwatch https://kwatch.dev/charts
helm repo update
```

## Install Chart

```console
helm install [RELEASE_NAME] kwatch/kwatch --version 0.10.5
```

## Uninstall Chart

```console
helm uninstall [RELEASE_NAME] --namespace kwatch
```

The chart owns and upgrades the `KwatchConfig` CRD automatically on install and upgrade.
The CRD is intentionally kept when the release is uninstalled so live configuration is not
removed accidentally. Delete it manually only when you are sure no KwatchConfig resources
are needed.

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podAnnotations` | Pod annotations | {} |
| `podLabels` | Pod labels | {} |
| `securityContext.runAsNonRoot` | Container runs as a non-root user | true |
| `securityContext.runAsUser` | Container processes' UID to run the entrypoint | 1000 |
| `securityContext.runAsGroup` | Container processes' GID to run the entrypoint | 1000 |
| `securityContext.readOnlyRootFilesystem` | Container's root filesystem is read-only | true |
| `service.port` | Health check port | 8060 |
| `resources` | CPU/Memory resource requests/limits | {limits: memory: 256Mi cpu: 100m} |
| `nodeSelector` | Node labels for pod assignment | {} |
| `tolerations` | Tolerations for pod assignment | [] |
| `affinity` | affinity for pod | {} |
| `config` | [kwatch configuration](../../docs/configuration.md); `crd.enabled` defaults to `true` because the chart installs the KwatchConfig CRD | `{crd: {enabled: true}}` |
| `configSecretName` | Existing Secret containing `config.yaml`; keeps notification credentials out of the ConfigMap | `""` |
| `upgrader.disableUpdateCheck` | Disable startup update check | `false` |
