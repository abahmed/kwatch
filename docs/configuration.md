# Configuration reference

Every kwatch configuration option, monitor, and endpoint in one place. For the 60-second
install and a quick overview, see the [README](../README.md).

String values may reference a mounted file with the exact form `${file:/path}`. This is
useful for notification credentials: kwatch reads the file at startup without placing the
credential in a ConfigMap or command line. The interactive installer uses this form and
stores the referenced files in a Kubernetes Secret.

> **The good news: you probably don't need this page.** Every option below has a safe
> default and works out of the box. Use this reference when you want to *change* something —
> fewer alerts, a different channel, a custom message — or when a term in an alert confuses
> you. After editing your `config.yaml`, run `kwatch lint` (add `--check` to also verify your
> alert-provider credentials).

## 🔧 General

Decide **what** to watch and **how often**. These are the knobs most people change first —
narrowing the watch list and turning off the noisy reasons.

| Parameter | What it does |
|:---|---|
| `maxRecentLogLines` | How many recent log lines to include in each alert (default: 50) |
| `smartGrouping.namespaceFanOutThreshold` | How many owners failing the same way in one namespace collapse into a single alert (default: 3, `0` disables) |
| `resyncSeconds` | How often to re-scan everything for problems. `0` = only react to live events (recommended). |
| `workers` | How many checks to run in parallel (default: 1, raise for big clusters) |
| `namespaces` | 🔽 Watch only these namespaces — or use `!kube-system` to watch *everything except* it |
| `namespaceSelector` | 🏷️ Pick namespaces by K8s label selector (use *instead of* `namespaces`, not with it) |
| `reasons` | 🔽 Only alert on these event reasons — or exclude some with `!` (e.g. `reasons: ["!Started"]`) |
| `ignoreFailedGracefulShutdown` | ✅ Skip containers stopped by a clean/graceful shutdown (default: true — keep it) |
| `ignoreDisruptionTerminations` | ✅ Skip pods evicted during node drains (default: true — keep it) |
| `runbooks` | 📚 Add a link to your runbook for each error reason, so every alert comes with help attached |
| `containerRestartThreshold` | Alert if a container restarts this many times (default: 0, off) |
| `adaptiveThresholds` | Add bounded workload-aware grace during small partial rollouts (default: true) |
| `maintenance` | Suppress explicitly marked pod/container maintenance without disabling cluster monitoring |
| `reportStartupBaseline` | 📋 Send one startup summary of pre-existing issues (default: true). Anything already broken when kwatch starts is otherwise quiet for **24 hours** and re-captured on every restart, so this summary is the only way you hear about it — keep it on |
| `ignore*` fields | 🔕 Deprecated filters (`ignoreContainerNames`, `ignorePodNames`, `ignoreLogPatterns`, `ignoreContainerMessages`, `ignoreNodeReasons`, `ignoreNodeMessages`) — use the more flexible `silences` below |

#### 🔽 Filter by namespace

```yaml
# Watch only these namespaces
namespaces:
  - default
  - production

# Or exclude some (can't mix both)
namespaces:
  - !kube-system
  - !monitoring
```

#### 🔽 Filter by reason

```yaml
# Only these reasons trigger alerts
reasons:
  - CrashLoopBackOff
  - ImagePullBackOff

# Or exclude some
reasons:
  - !Started
  - !Killing
```

#### 🔧 Maintenance mode

Mark a workload's pod template (or an individual Pod) when a deliberate change
should not page for its pod/container symptoms:

```yaml
maintenance:
  enabled: true
  annotation: kwatch.io/maintenance
  untilAnnotation: kwatch.io/maintenance-until
```

Use `kwatch.io/maintenance: "true"`, or set the until annotation to an RFC3339
time such as `2026-08-31T23:00:00Z`. This suppresses pod/container symptoms only;
kwatch continues monitoring nodes, control plane, storage, and workload-level
conditions. Invalid expiry values are ignored rather than suppressing alerts.

The same annotations may be placed on Deployment, StatefulSet, DaemonSet, Job,
CronJob, HPA, or PDB objects to suppress their own workload-level alerts during
an intentional maintenance operation. Node and control-plane alerts are never
suppressed by this setting.

## 📱 App settings

Small but useful global options — mostly about **what alerts look like** and how kwatch
talks to the outside world.

| Parameter | What it does |
|:---|---|
| `app.proxyURL` | 🔗 Proxy for outgoing HTTP requests |
| `app.clusterName` | 🏷️ Name shown in alerts so you know which cluster |
| `app.disableStartupMessage` | Silence the "kwatch is alive" welcome message |
| `app.logFormatter` | Log format: `text` (default) or `json` |
| `app.insecureSkipTLSVerify` | 🔓 Skip TLS verification on outbound HTTP (default: false) |
| `app.caBundlePath` | 📜 Path to a PEM CA bundle for outbound HTTP |
| `includeEvents` | 📋 Include K8s events in alerts (default: true). At most the 40 most recent are attached; older ones are summarised as `... N earlier event(s) omitted`. A churning pod can accumulate hundreds, and an unbounded list pushes the message past the chat provider's size limits, which loses the whole alert rather than just the surplus |
| `includeLogs` | 📋 Include container logs in alerts (default: true) |

## 💓 Health checks

Endpoints kwatch serves so you can see it's alive, grab Prometheus metrics, and poke it for
tests.

| Parameter | What it does |
|:---|---|
| `healthCheck.enabled` | ✅ Health endpoints (default: true) |
| `healthCheck.port` | Port to serve health on (default: 8060) |
| `healthCheck.pprof` | 🔬 Go profiling endpoints (default: false) |
| `healthCheck.diagnostics` | 🩺 Extra endpoints: `/incidents`, `/test-alert`, `/deadletters` |
| `healthCheck.diagnosticsToken` | 🔑 Optional Bearer token for diagnostic endpoints (empty = unauthenticated) |

**Endpoints:**
- `GET /healthz` — ✅ Liveness
- `GET /readyz` — ✅ Readiness. Ready once every informer cache has synced. If one never does —
  a missing RBAC rule, an API group the cluster does not serve — kwatch exits after **5 minutes**
  with an error naming the unsynced resource, rather than sitting not-ready forever with only
  reflector errors in the log to explain why.
- `GET /health` — `{"status": "ok"}`
- `GET /metrics` — 📊 Prometheus-format metrics (incidents, notifications, baseline, dependency-graph size/rebuild latency, queues, and informer activity). It does not require Prometheus to be installed.

Informer caches discard Kubernetes `managedFields` metadata at ingestion time
to reduce memory on apply-heavy clusters. Labels, annotations, spec, status,
resource versions, and deletion metadata remain intact for detection and graph
analysis.
- `GET /incidents` — 📋 All active incidents (requires `diagnostics: true`)
- `POST /test-alert` — 📤 Send a test alert (requires `diagnostics: true`)
- `GET /deadletters` — 💀 Recent delivery failures (requires `diagnostics: true`)

> **Know when alerts are being lost.** A notification a provider rejects is dead-lettered,
> counted in `kwatch_notifications_dropped_total`, and — with `diagnostics: true` — listed
> at `/deadletters`. Diagnostics are off by default because `/test-alert` accepts
> unauthenticated POSTs; if you turn them on, set `diagnosticsToken`. Either way, alert on
> the counter: it is the difference between "no incidents" and "no deliveries".

## 🔐 Kubernetes permissions and graceful degradation

The bundled manifests contain a read-only ClusterRole and a namespace Role for
kwatch state ConfigMaps. kwatch never needs write access to monitored
workloads. The ClusterRole covers:

| API group | Resources used |
|:---|:---|
| core | Pods, pod logs, Events, Nodes, Nodes proxy, Services, Endpoints, PVCs, ConfigMaps, Secrets, ResourceQuotas, LimitRanges, Namespaces, Leases, ServiceAccounts |
| apps | Deployments, ReplicaSets, StatefulSets, DaemonSets |
| batch | Jobs, CronJobs |
| autoscaling | HorizontalPodAutoscalers |
| policy | PodDisruptionBudgets |
| networking.k8s.io | NetworkPolicies, Ingresses |
| storage.k8s.io | StorageClasses, VolumeAttachments, CSIDrivers, VolumeSnapshots and related resources |
| admissionregistration.k8s.io | Mutating/Validating webhook configurations and admission policies |
| certificates.k8s.io | CertificateSigningRequests and PodCertificateRequests |
| flowcontrol.apiserver.k8s.io | FlowSchemas and PriorityLevelConfigurations |
| apiregistration.k8s.io | APIServices |
| apiextensions.k8s.io | CustomResourceDefinitions |
| gateway.networking.k8s.io | GatewayClasses, Gateways, HTTP/TCP/TLS/gRPCRoutes, ReferenceGrants |
| authorization.k8s.io | SelfSubjectAccessReviews |

Watched resources need only `get`, `list`, and `watch`. Optional monitors expose
`unavailable` or `rbacDenied` when an API is not served or a permission is
missing; the controller continues with the remaining monitors. Metrics Server,
Prometheus, a service mesh, and a cloud-provider API are not required for core
Kubernetes object monitoring.

The `/security` capability audit is scoped to the monitors enabled in the
loaded configuration: disabled rollout, TLS, storage, networking, admission,
and other optional monitors are not reported as missing capabilities. The
bundled ClusterRole remains a static superset so one manifest can support any
configuration; installations requiring least privilege can remove rules for
disabled monitors from their copied manifest without changing kwatch.

The interactive `kwatch.sh` manager downloads the matching
`deploy/feature-catalog.tsv` from the installed release and caches it in a
separate ConfigMap. Run `kwatch.sh features` (or choose **Show capabilities**)
to see the available IDs, dependencies, and plain-language descriptions;
the catalog is informational and does not change runtime behavior. To disable
monitoring, use the monitor's normal `enabled` or threshold settings.

The `/kubelet` health status reports `healthy`, `partial`, `unavailable`, or
`rbacDenied`, alongside Summary, cAdvisor, runtime, and node counts. The
`/controlplane`, `/security`, and `/informer` endpoints expose the same state
vocabulary and detailed last-error fields. `partial` means some built-in
capabilities are working; `rbacDenied` identifies an authorization gap rather
than a Kubernetes failure. Missing optional endpoints are visible without
becoming fabricated incidents.

## 🔄 Upgrader

At startup kwatch quietly asks "is there a newer version?" and mentions it. Turn it off if
you manage updates yourself (e.g. you pin images).

| Parameter | What it does |
|:---|---|
| `upgrader.disableUpdateCheck` | 🔕 Don't check for new kwatch versions |

## 📡 Minimal adoption telemetry

Official builds send a small pseudonymous heartbeat so the project can estimate
adoption. The payload contains only a stable, randomly generated installation
ID (kept in `kwatch-state`) and the kwatch version. The API field is named
`cluster_uuid` for wire compatibility; it is not the Kubernetes cluster UID.
It is sent after startup and at most once per week to
`https://api.kwatch.dev/v1/telemetry/heartbeat`.

| Parameter | What it does |
|:---|:---|
| `telemetry.enabled` | ✅ Send adoption heartbeats (default: `true`) |

Disable it with `telemetry.enabled: false`. Development builds and recognized CI
environments do not send telemetry. The service should use this data only for
aggregate adoption counts and version planning; no feature-usage or cluster
inventory is collected. Telemetry failures never affect monitoring startup or
runtime.

---

## 📊 Monitors

The **watchdogs**: continuous checks that catch problems *between* events — a disk filling
up, a node going sick, a rollout stuck. Each one begins with a plain-English summary, then a
small table (skip the table — defaults are fine). All monitors are **on by default** unless a
table says `default: false`.

### 💾 PVC Monitor — disk space alerts

It keeps an eye on how full your Persistent Volume Claims get, and warns you before
they fill up.

| Parameter | What it does |
|:---|---|
| `pvcMonitor.enabled` | ✅ Monitor disk usage (default: true) |
| `pvcMonitor.interval` | Check every N minutes (default: 5) |
| `pvcMonitor.threshold` | ⚠️ Warn at this % (default: 80) |
| `pvcMonitor.criticalThreshold` | 🚨 Critical at this % (default: 90) |
| `pvcMonitor.clearThreshold` | ✅ Resolve below this % (default: 75) |

### 🖥️ Node Monitor

 A node that's sick (NotReady, out of memory or disk) stays quiet for a few minutes
*before* kwatch tells you, so transient blips don't page you.

| Parameter | What it does |
|:---|---|
| `nodeMonitor.enabled` | ✅ Watch for node problems (default: true) |
| `nodeMonitor.sustainedMinutes` | ⏱️ Minutes a node condition must persist before alerting (default: 3) |

Catches: `NotReady`, `Unknown`, `MemoryPressure`, `DiskPressure`, `PIDPressure`, `NetworkUnavailable`.

### 🚀 Rollout Monitor

It catches a **bad rollout** — replicas that never become ready — before it takes
your whole Deployment down.

| Parameter | What it does |
|:---|---|
| `rolloutMonitor.enabled` | ✅ Watch for stuck deployments (default: true) |
| `rolloutMonitor.sustainedMinutes` | ⏱️ Minutes of unavailability before alerting (default: 5). Kubernetes' own `progressDeadlineSeconds` is 600; anything much shorter flags normal rollouts of slow-booting services |

### 📡 DaemonSet Monitor

It alerts when DaemonSet pods can't run on every node where they should.

| Parameter | What it does |
|:---|---|
| `daemonSetMonitor.enabled` | ✅ Watch for unavailable DaemonSet pods (default: true) |
| `daemonSetMonitor.sustainedMinutes` | ⏱️ Minutes of unavailability before alerting (default: 5) |

### 🧑‍💼 Job Monitor

 Failed (or stuck-suspended) Jobs get reported instead of silently failing in a corner.

| Parameter | What it does |
|:---|---|
| `jobMonitor.enabled` | ✅ Watch for failed/suspended Jobs (default: true) |

### ⏰ CronJob Monitor

 Suspended CronJobs and missed schedules get reported — a quiet cron that never
ran is a problem too.

| Parameter | What it does |
|:---|---|
| `cronJobMonitor.enabled` | ✅ Watch for suspended CronJobs or missed schedules (default: true) |
| `cronJobMonitor.sustainedMinutes` | ⏱️ Minutes a CronJob must stay suspended before alerting (default: 5) |

### 📈 HPA Monitor

 An autoscaler pinned at maximum replicas means your capacity is maxed out; the
default waits 20 minutes so brief spikes don't page you.

| Parameter | What it does |
|:---|---|
| `hpaMonitor.enabled` | ✅ Watch HPAs stuck at max replicas (default: true) |
| `hpaMonitor.sustainedMinutes` | ⏱️ How long before alerting (default: 20 min) |

### 🚀 Cluster Autoscaler Monitor

It tells you when the cluster autoscaler *can't* add capacity, so pending pods never
quietly stall.

| Parameter | What it does |
|:---|---|
| `clusterAutoscalerMonitor.enabled` | ✅ Watch cluster-autoscaler events (default: true) |

Alerts when the cluster autoscaler reports `FailedToScaleUp` or `NotTriggerScaleUp` for a sustained 5 minutes, meaning pods can't be scheduled because the autoscaler can't add capacity.

### 💓 Heartbeat Monitor (dead man's switch)

| Parameter | What it does |
|:---|---|
| `heartbeatMonitor.enabled` | Send pings to a health-check URL (default: false) |
| `heartbeatMonitor.interval` | ⏱️ Seconds between pings (default: 300) |
| `heartbeatMonitor.url` | 🔗 External URL (e.g. Healthchecks.io) |

If kwatch stops or crashes, the external monitor stops getting pings and pages you. 🔔

### 🔒 TLS Certificate Monitor

It reminds you before before a TLS certificate expires — warn with 30 days to go, page with 3
days to go. Off by default.

| Parameter | What it does |
|:---|---|
| `tlsMonitor.enabled` | 🔐 Watch for expiring certs (default: false) |
| `tlsMonitor.threshold` | 📅 Days before warning (default: 30) |
| `tlsMonitor.criticalThreshold` | 🚨 Days before critical (default: 3) |

### 🔗 Service Endpoint Monitor

| Parameter | What it does |
|:---|---|
| `serviceMonitor.enabled` | 🔗 Watch for services with zero ready endpoints (default: true) |

Detects when a Service's backing Endpoints object has zero ready addresses, indicating no healthy pods are available to serve traffic. Includes a 60-second debounce to avoid flapping during rolling updates or brief endpoint transitions.

### 🧩 Admission Webhook Monitor

| Parameter | What it does |
|:---|---|
| `admissionWebhookMonitor.enabled` | 🧩 Watch for webhooks with unreachable backends (default: true) |

Monitors `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` resources. Alerts when a webhook's backing service has no ready endpoints, meaning admission requests may fail or timeout.

### 🏛️ Control-Plane Monitor

| Parameter | What it does |
|:---|---|
| `controlPlaneMonitor.enabled` | 🏛️ Watch for broken control-plane components and API health (default: true) |
| `controlPlaneMonitor.intervalSeconds` | Active API/component probe interval (default: 30) |
| `controlPlaneMonitor.apiServerLatencyWarningMs` | Sustained `/readyz` latency warning threshold (default: 1000) |
| `controlPlaneMonitor.failureThreshold` / `recoveryThreshold` | Consecutive samples to alert / resolve (defaults: 2 / 2) |

Detects container issues (CrashLoopBackOff, Error, OOMKilled, etc.) in control-plane pods (kube-apiserver, kube-scheduler, kube-controller-manager, etcd, kube-proxy, coredns), actively probes API server `/readyz`, and probes scheduler/controller-manager/etcd health endpoints through the Kubernetes API. It also exposes probe state at `/controlplane`; informer watch interruptions and event freshness are available at `/informer`. Runs a dedicated sweep at startup to catch pre-existing failures.

The same monitor performs an in-cluster DNS lookup of `kubernetes.default.svc`,
so CoreDNS failures are detected even when CoreDNS Pods still appear Running.

### 🌐 Ingress Backend Monitor

| Parameter | What it does |
|:---|---|
| `ingressMonitor.enabled` | 🌐 Watch for ingress backends with no ready endpoints (default: true) |

Alerts when an Ingress rule references a backend service that has zero ready endpoints, meaning traffic to that host/path would return an error.

### 🚧 Network Policy Monitor

In plain words: finds NetworkPolicies that block **all** inbound traffic — the classic
"accidentally locked myself out" mistake — so you learn about it before users do.

| Parameter | What it does |
|:---|---|
| `networkPolicyMonitor.enabled` | 🚧 Detect overly restrictive network policies (default: true) |

Detects `NetworkPolicy` resources that deny all ingress traffic (no ingress rules defined). Helps identify policies that may unintentionally block legitimate traffic.

### 🧩 StatefulSet Monitor

| Parameter | What it does |
|:---|---|
| `statefulSetMonitor.enabled` | ✅ Watch for unavailable StatefulSet pods (default: true) |
| `statefulSetMonitor.sustainedMinutes` | ⏱️ Minutes of unavailability before alerting, plus 15-minute rollout grace (default: 5) |

Monitors StatefulSets where `readyReplicas < replicas` for a sustained period, with a 15-minute grace window during rollouts to avoid alerting mid-update.

### 🔄 PDB Monitor

| Parameter | What it does |
|:---|---|
| `pdbMonitor.enabled` | ✅ Watch for PDBs blocking voluntary disruptions (default: true) |
| `pdbMonitor.sustainedMinutes` | ⏱️ Minutes of blocking before alerting (default: 5) |

Alerts when a PodDisruptionBudget has `disruptionsAllowed=0` and `currentHealthy < desiredHealthy`, meaning voluntary disruptions (rollouts, node drains) are blocked.

### 🏭 Node Resource Monitor

In plain words: checks whether a node is **over-committed** — pods are promised more CPU or
memory than the machine can actually deliver. When promise exceeds capacity, the node
eventually pays the price.

| Parameter | What it does |
|:---|---|
| `nodeResourceMonitor.enabled` | ✅ Check node overcommit levels (default: true) |
| `nodeResourceMonitor.intervalSeconds` | ⏱️ How often to check (default: 300) |
| `nodeResourceMonitor.cpuWarning` | ⚠️ CPU overcommit ratio for warning (default: 2.0) |
| `nodeResourceMonitor.cpuCritical` | 🚨 CPU overcommit ratio for critical (default: 4.0) |
| `nodeResourceMonitor.memWarning` | ⚠️ Memory overcommit ratio for warning (default: 2.0) |
| `nodeResourceMonitor.memCritical` | 🚨 Memory overcommit ratio for critical (default: 4.0) |
| `nodeResourceMonitor.filesystemWarningPercent` | ⚠️ Node filesystem usage warning threshold (default: 90; 0 disables) |
| `nodeResourceMonitor.filesystemCriticalPercent` | 🚨 Node filesystem usage critical threshold (default: 95; 0 disables) |
| `nodeResourceMonitor.inodeWarningPercent` | ⚠️ Node inode usage warning threshold (default: 90; 0 disables) |
| `nodeResourceMonitor.inodeCriticalPercent` | 🚨 Node inode usage critical threshold (default: 95; 0 disables) |

Periodically computes the ratio of pod resource requests vs node allocatable for CPU and memory. Data is purely in-memory — no TSDB or persistent storage needed.

### 🌐 Active Probes

Active probes are opt-in and target only endpoints explicitly listed by the
operator by default. Set `autoServices: true` to probe every advertised
Service port from inside the kwatch Pod (TCP for all ports, plus HTTP for
ports whose name starts with `http`). A target must fail
`failureThreshold` consecutive checks before alerting and pass
`recoveryThreshold` consecutive checks before resolving.

Explicit `http`, `tcp`, and `dns` targets are the recommended low-noise mode.
`autoServices` is opt-in and probes every advertised Service port; it uses
paginated Kubernetes API lists so large clusters are not fetched as one large
response.

```yaml
activeProbeMonitor:
  enabled: true
  intervalSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3
  recoveryThreshold: 2
  autoServices: false
  http:
    - name: public-api
      url: https://api.example.com/ready
      expectedStatus: 200
      latencyWarningMs: 500
      latencyCriticalMs: 2000
  tcp:
    - name: postgres
      address: postgres.database.svc:5432
  dns:
    - name: cluster-dns
      host: kubernetes.default.svc
```

### 🧠 Kubelet Telemetry

`kubeletTelemetryMonitor` uses Kubernetes' built-in kubelet endpoints directly;
no Agent or Prometheus installation is required.

| Parameter | What it does |
|:---|:---|
| `kubeletTelemetryMonitor.enabled` | ✅ Enable built-in kubelet telemetry (default: true) |
| `kubeletTelemetryMonitor.intervalSeconds` | ⏱️ Collection interval (default: 60) |
| `kubeletTelemetryMonitor.failureThreshold` | 🔁 Consecutive failing samples before alerting (default: 2) |
| `kubeletTelemetryMonitor.recoveryThreshold` | ✅ Consecutive healthy samples before resolving (default: 2) |
| `kubeletTelemetryMonitor.persistState` | 💾 Persist counters and confirmation state across restarts (default: true) |
| `kubeletTelemetryMonitor.memoryWarningPercent` | ⚠️ Container memory usage warning (default: 90) |
| `kubeletTelemetryMonitor.memoryCriticalPercent` | 🚨 Container memory usage critical (default: 100) |
| `kubeletTelemetryMonitor.ephemeralStorageWarningPercent` | ⚠️ Container ephemeral-storage warning (default: 90) |
| `kubeletTelemetryMonitor.ephemeralStorageCriticalPercent` | 🚨 Container ephemeral-storage critical (default: 95) |
| `kubeletTelemetryMonitor.cpuWarningPercent` | ⚠️ Container CPU usage warning (default: 90) |
| `kubeletTelemetryMonitor.cpuCriticalPercent` | 🚨 Container CPU usage critical (default: 100) |
| `kubeletTelemetryMonitor.cpuThrottlingWarningPercent` | ⚠️ Container throttling warning (default: 25) |
| `kubeletTelemetryMonitor.cpuThrottlingCriticalPercent` | 🚨 Container throttling critical (default: 50) |
| `kubeletTelemetryMonitor.psiWarningPercent` | ⚠️ PSI warning threshold (default: 20) |
| `kubeletTelemetryMonitor.psiCriticalPercent` | 🚨 PSI critical threshold (default: 50) |
| `kubeletTelemetryMonitor.networkErrorRateWarning` | ⚠️ Node network errors/sec warning (default: 1) |
| `kubeletTelemetryMonitor.networkErrorRateCritical` | 🚨 Node network errors/sec critical (default: 10) |
| `kubeletTelemetryMonitor.runtimeErrorRateWarning` | ⚠️ Kubelet runtime errors/sec warning (default: 1) |
| `kubeletTelemetryMonitor.runtimeErrorRateCritical` | 🚨 Kubelet runtime errors/sec critical (default: 10) |

`runtimeMetricsMonitor` is an optional legacy Metrics Server integration and is
disabled by default. It is not required for standalone CPU/memory monitoring.

For dynamically watched CRDs, `crd.failureConditions` can override the default
failure rules. Use entries such as `Ready=False`, `Available=Unknown`,
`Degraded=True`, or `Progressing=False`.

When health checks are enabled, `/kubelet` exposes the latest per-node
availability counts for Summary, cAdvisor, and runtime endpoints, including
RBAC-denied nodes.

Kubelet usage thresholds learn a bounded per-container baseline after five
healthy samples. The learned warning threshold can rise up to one percentage
point below the configured critical threshold; the critical threshold itself
never changes. Baselines are persisted when telemetry persistence is enabled
and are discarded after the normal stale-state window.

The optional `runtimeMetricsMonitor` requires an additional `metrics.k8s.io`
read permission and a Metrics Server; the shipped RBAC deliberately does not
grant that unused permission by default.

### 💥 OOM Pattern Monitor

| Parameter | What it does |
|:---|---|
| `oomMonitor.enabled` | ✅ Track repeating OOMs (default: true) |
| `oomMonitor.threshold` | 🔢 OOM count within window to flag (default: 3) |
| `oomMonitor.windowMinutes` | ⏱️ Sliding window in minutes (default: 60) |

Tracks OOMKilled events per container in a sliding window. When the threshold is exceeded, the reason changes from `OOMKilled` to `OOMRepeating` with a hint suggesting a potential memory leak.

### 🎯 Scheduling Delay Diagnostics

In plain words: when a pod can't get scheduled, the alert also says **how long** the
scheduler has been stalling ("unschedulable for 5m30s"), not just that it stalled.

| Parameter | What it does |
|:---|---|
| `scheduleMonitor.enabled` | ✅ Compute unschedulable delay (default: true) |
| `pendingPodMonitor.enabled` | ✅ Watch pods stuck in Pending (default: true) |
| `pendingPodMonitor.threshold` | ⏱️ Seconds stuck in Pending before alerting (default: 300) |

When a pod is stuck Unschedulable, computes `now - PodScheduled.LastTransitionTime` and prepends the delay to the hint (e.g., `"unschedulable for 5m30s — ..."`).

### 🟡 Not Ready Monitor

| Parameter | What it does |
|:---|---|
| `notReadyMonitor.enabled` | ✅ Watch Running pods stuck NotReady (default: true) |

Alerts with `ContainersNotReady` when a Running pod's Ready condition stays false for longer than the pod is allowed. Container-level failures (crashes, waits, non-zero terminations) are handled by the container pipeline, so this fires for otherwise-healthy containers whose pod never becomes ready.

**How long is "too long" is derived from the pod, not from a fixed number.** There is nothing to configure:

- A pod that **has never been ready** is still starting up, so it gets whatever budget its own probes declare — `initialDelaySeconds + failureThreshold × periodSeconds`, taken from `startupProbe` (falling back to `readinessProbe`), across init and app containers. A service that legitimately takes 90 seconds to boot no longer alerts on every rollout, and one with no probes keeps the 60 second floor. The budget is capped at 15 minutes so a generous probe cannot defer an alert indefinitely.
- A pod that **was ready and stopped being ready** gets the plain 60 second floor. That is a regression rather than a slow start, and it should not wait.

The alert states the real elapsed time (`pod stopped being ready 3h12m ago`), never the threshold.

Restarting kwatch does not reset that clock. There is a short grace period after startup during which pre-existing conditions are not alerted, but the duration you are shown is always how long the pod has actually been unready.

### 🎯 Severity

Every alert carries a severity, shown as the colour of its headline, and you control it.
Severity drives urgent channels, escalation, and re-notification.

| Severity | Headline | Meaning |
|:--|:--|:--|
| `critical` | 🔴 | Something users feel right now |
| `high` | 🟠 | Needs a person soon |
| `warning` / `medium` | 🟡 | Worth a look |
| `normal` | 🔵 | Informational |

The scale is monotonic on purpose: red is reserved for critical, so a red headline always
means the worst case and a blue one never competes with it.

| Parameter | What it does |
|:---|---|
| `severityByOwnerKind` | Set severity per resource type, e.g. `StatefulSet: "high"` |
| `severityByReason` | Set severity per event reason, checked before owner kind, e.g. `OOMKilled: "high"` |

Defaults: `StatefulSet` → 🟠 high, everything else → 🔵 normal

### 🔇 Silences — stop the noise

In plain words: if a rule matches an incident, that incident is **completely ignored** — no
alert, no group, nothing. Build rules from anything on the incident: namespace, reason, pod
name pattern, container name, log text, or node.

```yaml
silences:
  - namespaces: ["kube-system", "monitoring"]
  - reasons: ["BackOff"]
  - podNamePatterns: ["my-fancy-pod-.*"]
```

Each rule can also filter by `containerNames`, `logPatterns`, `containerMessages`, `nodeReasons`, and `nodeMessages` (message substrings). An incident matching any rule is suppressed entirely. The deprecated top-level `ignore*` fields map onto these rules.

### 🚫 Inhibition — no double alerts

In plain words: when the **node** is down, don't also page you about every **pod** on it —
you can't fix pods that have no machine. The moment the node recovers, pod alerts resume.

| Parameter | What it does |
|:---|---|
| `inhibition.nodeSuppressesPods` | ✅ Don't alert on pod issues if the node itself is down (default: true) |

Pods are suppressed **only while the node is actually down** — once the node recovers, suppression lifts immediately, even during the resolve hold-down window (it doesn't wait for the "resolved" notification to be sent).

### 📝 Custom message templates

In plain words: if the default alert text isn't yours, write your own. Templates use Go
`{{.Field}}` placeholders and can access the incident, the action, and the hint.

```yaml
templates:
  CrashLoopBackOff: "{{.Incident.Name}} — {{.Action}} — {{.Incident.Hint}}"
```

### 🧠 Correlation — smart incident grouping

In plain words: this is kwatch's **memory** — how it remembers that "this crash" and "that
crash five minutes ago" are the same problem, when it's allowed to yell again, and when it
should escalate a recurring crash to you.

| Parameter | What it does |
|:---|---|
| `correlation.window` | ⏱️ Keep incidents in memory (default: 10 min) |
| `correlation.resolveHoldDown` | ⏱️ Wait before sending "resolved" (default: 300s) |
| `correlation.lifecycleInterval` | ⏱️ Lifecycle check frequency (default: 1 min) |
| `correlation.cooldownMinutes` | ⏱️ Min time between identical crash re-alerts (default: 10; 0 = off) |
| `correlation.maxBaseline` | 📈 Max baseline entries kept for startup comparison (default: 5000) |
| `correlation.escalation.enabled` | ✅ Escalate severity on repeated crashes (default: true) |
| `correlation.escalation.tiers` | 📊 Restart thresholds (default: `[3, 10]`): crossing the first → `high`, the second → `critical`. There is nothing above critical, so a third tier does nothing |
| `correlation.renotify.intervalBySeverity` | 🔔 Re-alert interval per severity (e.g. `high: 60`), `default` key as fallback; unset = off |
| `correlation.renotify.maxPerIncident` | 🔔 Max re-alerts per incident (default: 3) |

Incident state, baseline, telemetry state, and recent change history are stored
in Kubernetes ConfigMaps with retry-on-conflict updates. Older incident layouts
are migrated on load, and oversized history is truncated to the newest entries
so persistence cannot block the main monitoring loop. A persistence failure is
logged and monitoring continues; it does not fabricate a new incident.

### 🧹 Smart Grouping — coalesce duplicate notifications

In plain words: **many pods failing the same way = one alert**, not one per pod. Events
that share a root dimension are collected over a short window and summarized into a single
notification, then re-notified on a gentle cooldown instead of on every event.

| Parameter | What it does |
|:---|---|
| `smartGrouping.windowSeconds` | ⏱ Grouping window in seconds (default: 60). Set to 0 to disable. |
| `smartGrouping.namespaceFanOutThreshold` | 🧺 Owners failing the same way in one namespace, within one window, before their groups collapse into a single alert (default: 3; `0` disables) |

**The first failure alerts immediately.** Grouping only holds an incident back once a *second*
owner in the same namespace fails the same way inside the window — the earliest moment a
namespace-wide problem can be told apart from an isolated one. An isolated failure therefore
costs no grouping latency at all. If the fan-out threshold is then reached, the namespace-wide
alert counts that first owner in its total, and the first owner's own thread gets a short note
pointing at the wider alert. That thread stays open and resolves when the pod actually recovers
— it is never marked resolved just because a bigger alert took over. A buffer that ends the
window with a single member is sent as that incident, under its own name — never announced as
a "group" of one.

kwatch groups related incidents by the dimension that best captures each failure type's root cause. For example, OOMKilled and probe failures group by owner+namespace, node conditions group by node (not pod errors on the same node), image pull errors group by image (or globally for rate limits), and CrashLoopBackOff with a matching log signature bridges across owners. Each group notification shows affected pods, owners, nodes, or images depending on scope, with overflow counting above 1,000 entries. After a group notification is sent, the same condition is not silently repeated on every event: re-notifications are throttled by a cooldown (4× the grouping window, clamped between 5 and 30 minutes). While the underlying members keep failing, the group resumes with a periodic UPDATE after the cooldown lapses; it resolves (and stops notifying) once the members clear. This prevents per-event flooding while still surfacing ongoing incidents.

**Namespace fan-out collapses into one alert.** Most reasons group by reason + namespace + owner, which is right when one workload is unhealthy and wrong when the whole namespace is — a node going away can make a dozen deployments unready at once and produce a dozen alerts for one event. When `smartGrouping.namespaceFanOutThreshold` distinct owners (default **3**, `0` disables) fail the same way in one namespace inside one grouping window, their separate groups collapse into a single notification listing every affected owner. Groups already scoped to a shared cause — a node, an image, a log signature — are never merged this way, because they already describe the cause. This is the backstop for when the resource graph does not link the failures; when it does, mass-failure detection catches them first.

The order these decisions are made in — baseline, then attribution to a node / shared dependency / owning workload, then cooldown — is described in [How kwatch decides whether to speak](architecture.md#how-kwatch-decides-whether-to-speak).

**Mass failures suppress their own members.** When many workloads fail for one shared reason — a node going away, a ConfigMap breaking — kwatch raises a single blast-radius alert. Incidents whose dependency is already the subject of that alert are suppressed rather than sent alongside it, and recorded in the audit log with `skipReason: mass_failure`. Suppressed incidents are still tracked: they count toward the mass failure, resolve silently if they recover, and any that are **still broken when the mass failure clears are announced at that point** — a symptom that outlives its cause is not lost. The node's own incident is never suppressed: it is the root cause, not a symptom of itself.

**Evidence names the pod it came from.** An incident is keyed by owner rather than by pod, so it survives replicas being replaced and can list several pods under `Resources`. The logs and events attached to it come from exactly one of those pods, and the alert says which (`Events — from pod-abc123`) whenever the incident covers more than that pod.

### 🔐 Keeping credentials out of the ConfigMap

kwatch's config is a ConfigMap. ConfigMaps are readable by anyone with `get` access to the
namespace and are not encrypted at rest by default, so a provider token written there is a
token anyone in the namespace can take.

kwatch expands `${VAR}` in its config, so keep the secret in a Secret and reference it:

```yaml
# Secret
apiVersion: v1
kind: Secret
metadata:
  name: kwatch-credentials
stringData:
  SLACK_TOKEN: xoxb-...
```

```yaml
# kwatch container in the Deployment
env:
  - name: SLACK_TOKEN
    valueFrom:
      secretKeyRef:
        name: kwatch-credentials
        key: SLACK_TOKEN
```

```yaml
# config.yaml
alert:
  slack:
    token: "${SLACK_TOKEN}"
```

Only `${VAR}` (braced) is expanded, and only where the value is a string. The Helm chart does
not expose an `env` override yet; patch the rendered Deployment or use a values overlay.

If a token has ever been committed to a ConfigMap, rotate it — it should be treated as
disclosed, not merely moved.

### 📝 Audit log

In plain words: for every incident *transition* (created, updated, resolved, skipped),
kwatch writes one structured JSON line — feed it to your log pipeline if you want a
searchable history of everything it decided.

| Parameter | What it does |
|:---|---|
| `auditLog.enabled` | Write one structured JSON entry per incident transition (default: true) |
| `auditLog.output` | Destination: `stdout` (default) or a file path |

Emits `create`/`update`/`resolved`/`skip` entries (with `incidentKey`, `namespace`, `reason`, `severity`, `count`, `duration`, `skipReason`) to stdout or a file for feeding into your log pipeline.

**`skip` entries record state changes, not every evaluation.** Suppression is a standing condition: a baselined incident is re-evaluated on every poll, and writing a line each time buries everything else — a single baselined HPA can emit thousands of identical entries a day. A skip is therefore recorded the first time it applies to an incident key, and again only if the reason changes (say `baseline` → `cooldown`) or if the incident starts alerting and is later suppressed again. If you are counting suppression *events*, count polls elsewhere; this log answers "what did kwatch decide", not "how often did it re-decide the same thing".

### 📋 CRD — live config changes

In plain words: instead of editing the ConfigMap and restarting, you can push config changes
live with a small custom resource. It is off in the generic binary defaults when no CRD is
installed. The Helm chart installs the CRD and enables it by default; the interactive installer
does the same. Manual deployments must install the CRD before enabling it.

| Parameter | What it does |
|:---|---|
| `crd.enabled` | Watch `KwatchConfig` CRs for live config updates (default: false for the binary; true in Helm and the interactive installer) |

```yaml
apiVersion: kwatch.abahmed.dev/v1alpha1
kind: KwatchConfig
metadata:
  name: kwatch-config
  namespace: kwatch
spec:
  maxRecentLogLines: 100
  silences:
    - namespaces: ["kube-system"]
```
