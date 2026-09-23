# Kwatch Helm Chart
monitor all changes in your Kubernetes(K8s) cluster, detects crashes in your running apps in realtime, and publishes notifications to your channels (Slack,
Discord, etc.) instantly

## Add Repository

```console
helm repo add kwatch https://kwatch.dev/charts
helm repo update
```

## Install & Upgrade your Chart

```console
helm install [RELEASE_NAME] kwatch/kwatch 
helm upgrade -i [RELEASE_NAME]  --install --namespace <ns> ./ [--version CHART-VERSION] --debug

```

## Uninstall Chart

```console
helm delete --purge [RELEASE_NAME]
```

## Configuration

### Using helm starter https://github.com/MahaGamal/helm-starter/tree/main/templates 


The following table lists the configurable parameters of the chart and their default values.

Parameter | Description | Default
--- | --- | ---
`image.registry` | server container image registry | ``
`image.repository` | server container image repository | ``
`image.tag` | server container image tag | `v0.8.0`
`image.pullPolicy` | server container image pull policy | `IfNotPresent`
`image.runAsUser` | User ID of the server process. Value depends on the Linux distribution used inside of the container image. | `101`
`server.replicaCount` | desired number of server pods | `1`
`server.httpPort` | The port that the server container listens on for http connections. | `80`
`server.livenessProbe.initialDelaySeconds` | Delay before liveness probe is initiated | 10
`server.livenessProbe.periodSeconds` | How often to perform the probe | 10
`server.livenessProbe.timeoutSeconds` | When the probe times out | 5
`server.livenessProbe.successThreshold` | Minimum consecutive successes for the probe to be considered successful after having failed. | 1
`server.livenessProbe.failureThreshold` | Minimum consecutive failures for the probe to be considered failed after having succeeded. | 3
`server.livenessProbe.port` | The port number that the liveness probe will listen on. | 8080
`server.readinessProbe.initialDelaySeconds` | Delay before readiness probe is initiated | 10
`server.readinessProbe.periodSeconds` | How often to perform the probe | 10
`server.readinessProbe.timeoutSeconds` | When the probe times out | 1
`server.readinessProbe.successThreshold` | Minimum consecutive successes for the probe to be considered successful after having failed. | 1
`server.readinessProbe.failureThreshold` | Minimum consecutive failures for the probe to be considered failed after having succeeded. | 3
`server.readinessProbe.port` | The port number that the readiness probe will listen on. | 8080
`server.resources` | server pod resource requests & limits | `{}`
`server.envs` | any additional environment variables to set in the pods | `{}`
`server.VolumeMounts` |  volumeMounts to the server main container | `{}`
`server.Volumes` |  volumes to the server pod | `{}`
`server.tolerations` | node taints to tolerate (requires Kubernetes >=1.6) | `[]`
`server.affinity` | node/pod affinities (requires Kubernetes >=1.6) | `{}`
`server.nodeSelector` | node labels for pod assignment | `{}`
`serviceAccounts.enabled` | if `true`, Create service accounts | `false`
`service.ports.http` | Sets service http port | `80`
`service.type` | type of server service to create | `ClusterIP`
Service Monitoring configurations 
`serviceMonitor.enabled` | if `true`, enable Prometheus metrics | `false`
