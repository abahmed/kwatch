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
helm install [RELEASE_NAME] kwatch/kwatch --version 0.11.0
```

## Uninstall Chart

```console
helm delete --purge [RELEASE_NAME]
```

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
| `config.llm.enabled` | Enable AI enrichment via kwatch-llm sidecar | false |
| `nodeSelector` | Node labels for pod assignment | {} |
| `tolerations` | Tolerations for pod assignment | [] |
| `affinity` | affinity for pod | {} |
| `config` | [kwatch configuration](https://github.com/abahmed/kwatch#configuration) | {} |
| `upgrader.disableUpdateCheck` | Disable startup update check | `false` |
