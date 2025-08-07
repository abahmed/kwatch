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
helm install [RELEASE_NAME] kwatch/kwatch --version 0.10.3
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
| `securityContext.runAsUser` | Container processes' UID to run the entrypoint | 101 |
| `securityContext.runAsGroup` | Container processes' GID to run the entrypoint | 101 |
| `securityContext.readOnlyRootFilesystem` | Container's root filesystem is read-only | true |
| `resources` | CPU/Memory resource requests/limits | {limits: memory: 128Mi cpu: 100m} |
| `nodeSelector` | Node labels for pod assignment | {} |
| `tolerations` | Tolerations for pod assignment | [] |
| `affinity` | affinity for pod | {} |
| `config` | [kwatch configuration](https://github.com/abahmed/kwatch#configuration) | {} |
