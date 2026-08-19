<p align="center">
  <a href="https://kwatch.dev">
    <img src="./assets/logo.svg" width="30%"/>
  </a>
  <br />
  <a href="https://kwatch.dev">
    <img src="https://img.shields.io/badge/%F0%9F%92%A1%20kwatch-website-00ACD7.svg" />
  </a>
  <a href="https://pkg.go.dev/github.com/abahmed/kwatch">
    <img src="https://pkg.go.dev/badge/github.com/abahmed/kwatch" />
  </a>
  <a href="https://github.com/abahmed/kwatch/actions/workflows/check.yaml">
    <img src="https://github.com/abahmed/kwatch/workflows/Check/badge.svg?branch=main" />
  </a>
  <a href="https://goreportcard.com/report/github.com/abahmed/kwatch">
    <img src="https://goreportcard.com/badge/github.com/abahmed/kwatch" />
  </a>
  <a href="https://codecov.io/gh/abahmed/kwatch">
    <img src="https://codecov.io/gh/abahmed/kwatch/branch/main/graph/badge.svg?token=ZMCU75JJO7"/>
  </a>
  <a href="https://github.com/abahmed/kwatch/releases/latest">
    <img src="https://img.shields.io/github/v/release/abahmed/kwatch?label=kwatch" />
  </a>
  <a href="https://github.com/abahmed/kwatch/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/abahmed/kwatch" />
  </a>
  <a href="https://github.com/abahmed/kwatch">
    <img src="https://img.shields.io/github/go-mod/go-version/abahmed/kwatch" />
  </a>
  <a href="https://artifacthub.io/packages/helm/kwatch/kwatch">
    <img src="https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kwatch" />
  </a>
  <a href="https://discord.gg/kzJszdKmJ7">
    <img src="https://img.shields.io/discord/911647396918870036?label=Discord&logo=discord">
  </a>
</p>

> **👋 New to Kubernetes? No problem.**  
> kwatch watches your cluster 24/7. When something fails, it tells you **what broke and why** — with the error reason, diagnostic hints, logs, and events — straight to your team chat.  
> ✨ **60 seconds to install. No backend. No dashboards. No YAML spaghetti.**

---

## 🧐 What is kwatch?

kwatch is like a **smart friend** for your Kubernetes cluster:

- 💥 Something crashes → you get a message that says *why* (not just "pod is broken")
- 🔇 Smart about noise — groups related issues into a single notification, ignores flapping
- ⚡ Works in **under a minute** — just one command and a config file

No Prometheus. No Grafana. No 50-step setup. Just alerts that **make sense**.

---

## 🆚 kwatch vs the scary stuff

| | ✨ kwatch | 😰 DIY Prometheus + Alertmanager | 💸 Heavy SaaS |
|---|---|---|---|
| ⏱️ Setup time | **~5 minutes** | hours of YAML | agent + backend setup |
| 📦 Size | ~70 MB single binary | whole monitoring stack | per-node agents + cloud costs |
| 💬 Alerts | Self-explaining ("OOMKilled — raise memory limit") | Rule-defined message | Depends on configuration |
| 🗄️ Storage | No backend — state lives in a ConfigMap (incidents survive restarts) | Prometheus TSDB | Full retention (costly) |
| 📚 Learning curve | One ConfigMap | PromQL + alert rules | Platform-specific DSL |

---

## 🚨 Before vs After

| Raw kubectl output 🤷 | kwatch tells you 💡 |
|---|---|
| `CrashLoopBackOff` | 🚨 **OOMKilled** (memory limit: 512Mi) — try raising `limits.memory` · here are the logs + events |
| `Error` | 🚨 **HTTP probe** failing on `:8080/healthz` (exit 137) — container ran out of memory |

---

## ⚡️ 60-second install

### 📦 Helm (easiest 🏆)

```shell
helm repo add kwatch https://kwatch.dev/charts
helm install [RELEASE_NAME] kwatch/kwatch --namespace kwatch --create-namespace --version 0.11.0
```

More details in the [chart docs](https://github.com/abahmed/kwatch/blob/main/deploy/chart/README.md)

### 🐙 kubectl

```shell
curl -L https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/config.yaml -o config.yaml
# ✏️ Edit config.yaml with your Slack/Discord/email webhook
kubectl apply -f config.yaml
kubectl apply -f https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/deploy.yaml
```

---

## 🎯 What does it catch?

Every monitor below is **on by default** — zero config needed:

| Signal | Default | What you get |
|--------|---------|-------------|
| 🟥 Pod crashes (CrashLoop, OOM, ImagePull, Error) | ✅ **on** | Container state + previous logs + events — tells you *why* |
| ⏳ Pending pods (stuck Unschedulable) | ✅ **on** | Alerts after 300s stuck; includes scheduling delay in hint |
| 🟡 Pods stuck NotReady | ✅ **on** | `ContainersNotReady` — Running pods with `Ready=False` for 60s |
| 🎯 Scheduling delay diagnostics | ✅ **on** | Prepends `"unschedulable for XmYs"` duration to Unschedulable hints |
| 🖥️ Node issues (NotReady, Disk/Memory pressure) | ✅ **on** | Per-condition severity |
| 💾 PVC running out of space | ✅ **on** | Warn at 80%, critical at 90% |
| ❌ Failed Jobs | ✅ **on** | `JobFailed` / `JobSuspended` |
| 🚀 Stuck rollouts | ✅ **on** | `ProgressDeadlineExceeded` — deployment didn't finish |
| 🚦 Deployment unavailable | ✅ **on** | `DeploymentUnavailable` — unavailable replicas for `rolloutMonitor.sustainedMinutes` consecutive minutes |
| 📡 DaemonSet pods not running | ✅ **on** | Unavailable pods detected |
| ⏰ CronJob suspended or missing runs | ✅ **on** | Not scheduled in 24h? Alert. |
| 📈 HPA stuck at max replicas | ✅ **on** | After 20 minutes sustained |
| 🚀 Cluster-autoscaler scale failures | ✅ **on** | `FailedToScaleUp` / `NotTriggerScaleUp` sustained 5 min |
| 🔗 Service endpoint health | ✅ **on** | Detects endpoints with zero ready addresses |
| 🧩 Admission webhook backends | ✅ **on** | Alerts when a webhook's backing service has no ready endpoints |
| 🏛️ Control-plane component health | ✅ **on** | Detects broken control-plane pods (apiserver, scheduler, etc.) |
| 🧩 StatefulSet unavailable | ✅ **on** | `StsUnavailable` — pods not ready for `statefulSetMonitor.sustainedMinutes` minutes |
| 🔄 PDB blocking disruptions | ✅ **on** | `PdbViolation` — PodDisruptionBudget has `disruptionsAllowed=0` and unhealthy pods |
| 🏭 Node overcommit prediction | ✅ **on** | `NodeResourceHigh/Critical` — warning at 2×, critical at 4× CPU/mem overcommit |
| 💥 OOM pattern detection | ✅ **on** | `OOMRepeating` — 3+ OOM kills in 60-minute sliding window flags potential memory leak |
| 🌐 Ingress backend health | ✅ **on** | Alerts when ingress backend services have no ready endpoints |
| 🚧 NetworkPolicy over-restriction | ✅ **on** | Detects policies that may block all ingress traffic |
| 🔒 TLS certs expiring | ❌ off | Enable if you want cert expiry warnings |
| 🧠 Context-aware intelligence (dependency analysis) | ✅ **on** | Links incidents to root causes — unhealthy nodes, bad rollouts, misconfigured ConfigMaps/Secrets |
| 📊 Mass failure detection | ✅ **on** | Detects when 30%+ of dependents sharing a node/configmap/secret fail simultaneously |

✅ **TLS and heartbeat are the only ones off** — everything else just works out of the box.

---

### 🧠 Context-aware intelligence

kwatch builds a **dependency graph** of your cluster from pod informers — mapping pods to their nodes, owners (Deployments/StatefulSets/DaemonSets), Services, PVCs, and referenced ConfigMaps/Secrets. When an incident fires, the insight engine analyzes it against the graph and answers:

- **What likely caused this?** — unhealthy node, failed rollout, or misconfigured resources (ConfigMap, Secret, PVC)
- **What's the impact?** — how many pods, services, or dependents are affected
- **Recent changes?** — correlated changes on the same resource or namespace

The graph is built at startup from the informer cache, rebuilt periodically, and updated incrementally as pods come and go. No configuration needed.

#### Mass failure detection

The correlation engine periodically scans all active incidents for shared dependencies. If more than 30% of dependents sharing a node, ConfigMap, Secret, or PVC are in failure, a mass failure alert fires. The threshold is dynamic — computed per dependency based on the current scope. Mass failures automatically resolve when the underlying incidents clear.

#### 💾 Crash-safe state (ConfigMap)

kwatch keeps its state in plain ConfigMaps in its own namespace — no database or storage backend to run. Active incidents and the startup baseline are written to `kwatch-incidents` / `kwatch-baseline` ConfigMaps (plus `kwatch-state` for cluster identity and `kwatch-pvc` for PVC usage), so after a restart or pod reschedule kwatch resumes alerting where it left off instead of treating everything as brand new.

---

## ⚙️ Configuration (simple)

### 🔧 General

| Parameter | What it does |
|:---|---|
| `maxRecentLogLines` | How many log lines to include in alerts (default: 50) |
| `resyncSeconds` | Check for problems periodically (0 = only on events, recommended) |
| `workers` | How many parallel workers (default: 1, raise for big clusters) |
| `namespaces` | 🔽 Limit to specific namespaces, or use `!kube-system` to exclude |
| `namespaceSelector` | 🏷️ K8s label selector to pick namespaces to watch (mutually exclusive with `namespaces`) |
| `reasons` | 🔽 Only alert on specific reasons, or exclude some with `!` |
| `ignoreFailedGracefulShutdown` | ✅ Skip containers killed during graceful shutdown (default: true) |
| `ignoreDisruptionTerminations` | ✅ Skip pods evicted during node drains (default: true) |
| `runbooks` | 📚 Add links to your runbooks per error reason |
| `containerRestartThreshold` | Alert if a container restarts this many times (0 = off) |
| `reportStartupBaseline` | 📋 Send one startup summary of pre-existing issues (default: true) |
| `ignore*` fields | 🔕 Deprecated filters (`ignoreContainerNames`, `ignorePodNames`, `ignoreLogPatterns`, `ignoreContainerMessages`, `ignoreNodeReasons`, `ignoreNodeMessages`) — prefer `silences` below |

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

### 📱 App settings

| Parameter | What it does |
|:---|---|
| `app.proxyURL` | 🔗 Proxy for outgoing HTTP requests |
| `app.clusterName` | 🏷️ Name shown in alerts so you know which cluster |
| `app.disableStartupMessage` | Silence the "kwatch is alive" welcome message |
| `app.logFormatter` | Log format: `text` (default) or `json` |
| `app.insecureSkipTLSVerify` | 🔓 Skip TLS verification on outbound HTTP (default: false) |
| `app.caBundlePath` | 📜 Path to a PEM CA bundle for outbound HTTP |
| `includeEvents` | 📋 Include K8s events in alerts (default: true) |
| `includeLogs` | 📋 Include container logs in alerts (default: true) |

### 💓 Health checks

| Parameter | What it does |
|:---|---|
| `healthCheck.enabled` | ✅ Health endpoints (default: true) |
| `healthCheck.port` | Port to serve health on (default: 8060) |
| `healthCheck.pprof` | 🔬 Go profiling endpoints (default: false) |
| `healthCheck.diagnostics` | 🩺 Extra endpoints: `/incidents`, `/test-alert`, `/deadletters` |
| `healthCheck.diagnosticsToken` | 🔑 Optional Bearer token for diagnostic endpoints (empty = unauthenticated) |

**Endpoints:**
- `GET /healthz` — ✅ Liveness
- `GET /readyz` — ✅ Readiness
- `GET /health` — `{"status": "ok"}`
- `GET /metrics` — 📊 Prometheus metrics (incidents, notifications, baseline)
- `GET /incidents` — 📋 All active incidents (requires `diagnostics: true`)
- `POST /test-alert` — 📤 Send a test alert (requires `diagnostics: true`)
- `GET /deadletters` — 💀 Recent delivery failures (requires `diagnostics: true`)

### 🔄 Upgrader

| Parameter | What it does |
|:---|---|
| `upgrader.disableUpdateCheck` | 🔕 Don't check for new kwatch versions |

---

## 📊 Monitors

### 💾 PVC Monitor — disk space alerts

| Parameter | What it does |
|:---|---|
| `pvcMonitor.enabled` | ✅ Monitor disk usage (default: true) |
| `pvcMonitor.interval` | Check every N minutes (default: 5) |
| `pvcMonitor.threshold` | ⚠️ Warn at this % (default: 80) |
| `pvcMonitor.criticalThreshold` | 🚨 Critical at this % (default: 90) |
| `pvcMonitor.clearThreshold` | ✅ Resolve below this % (default: 75) |

### 🖥️ Node Monitor

| Parameter | What it does |
|:---|---|
| `nodeMonitor.enabled` | ✅ Watch for node problems (default: true) |
| `nodeMonitor.sustainedMinutes` | ⏱️ Minutes a node condition must persist before alerting (default: 3) |

Catches: `NotReady`, `Unknown`, `MemoryPressure`, `DiskPressure`, `PIDPressure`, `NetworkUnavailable`.

### 🚀 Rollout Monitor

| Parameter | What it does |
|:---|---|
| `rolloutMonitor.enabled` | ✅ Watch for stuck deployments (default: true) |
| `rolloutMonitor.sustainedMinutes` | ⏱️ Minutes of unavailability before alerting (default: 2) |

### 📡 DaemonSet Monitor

| Parameter | What it does |
|:---|---|
| `daemonSetMonitor.enabled` | ✅ Watch for unavailable DaemonSet pods (default: true) |
| `daemonSetMonitor.sustainedMinutes` | ⏱️ Minutes of unavailability before alerting (default: 5) |

### 🧑‍💼 Job Monitor

| Parameter | What it does |
|:---|---|
| `jobMonitor.enabled` | ✅ Watch for failed/suspended Jobs (default: true) |

### ⏰ CronJob Monitor

| Parameter | What it does |
|:---|---|
| `cronJobMonitor.enabled` | ✅ Watch for suspended CronJobs or missed schedules (default: true) |
| `cronJobMonitor.sustainedMinutes` | ⏱️ Minutes a CronJob must stay suspended before alerting (default: 5) |

### 📈 HPA Monitor

| Parameter | What it does |
|:---|---|
| `hpaMonitor.enabled` | ✅ Watch HPAs stuck at max replicas (default: true) |
| `hpaMonitor.sustainedMinutes` | ⏱️ How long before alerting (default: 20 min) |

### 🚀 Cluster Autoscaler Monitor

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
| `controlPlaneMonitor.enabled` | 🏛️ Watch for broken control-plane components (default: true) |

Detects container issues (CrashLoopBackOff, Error, OOMKilled, etc.) in control-plane pods (kube-apiserver, kube-scheduler, kube-controller-manager, etcd, kube-proxy, coredns). Runs a dedicated sweep at startup to catch pre-existing failures.

### 🌐 Ingress Backend Monitor

| Parameter | What it does |
|:---|---|
| `ingressMonitor.enabled` | 🌐 Watch for ingress backends with no ready endpoints (default: true) |

Alerts when an Ingress rule references a backend service that has zero ready endpoints, meaning traffic to that host/path would return an error.

### 🚧 Network Policy Monitor

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

| Parameter | What it does |
|:---|---|
| `nodeResourceMonitor.enabled` | ✅ Check node overcommit levels (default: true) |
| `nodeResourceMonitor.intervalSeconds` | ⏱️ How often to check (default: 300) |
| `nodeResourceMonitor.cpuWarning` | ⚠️ CPU overcommit ratio for warning (default: 2.0) |
| `nodeResourceMonitor.cpuCritical` | 🚨 CPU overcommit ratio for critical (default: 4.0) |
| `nodeResourceMonitor.memWarning` | ⚠️ Memory overcommit ratio for warning (default: 2.0) |
| `nodeResourceMonitor.memCritical` | 🚨 Memory overcommit ratio for critical (default: 4.0) |

Periodically computes the ratio of pod resource requests vs node allocatable for CPU and memory. Data is purely in-memory — no TSDB or persistent storage needed.

### 💥 OOM Pattern Monitor

| Parameter | What it does |
|:---|---|
| `oomMonitor.enabled` | ✅ Track repeating OOMs (default: true) |
| `oomMonitor.threshold` | 🔢 OOM count within window to flag (default: 3) |
| `oomMonitor.windowMinutes` | ⏱️ Sliding window in minutes (default: 60) |

Tracks OOMKilled events per container in a sliding window. When the threshold is exceeded, the reason changes from `OOMKilled` to `OOMRepeating` with a hint suggesting a potential memory leak.

### 🎯 Scheduling Delay Diagnostics

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

Alerts with `ContainersNotReady` when a Running pod's Ready condition stays false for 60 seconds. Container-level failures (crashes, waits, non-zero terminations) are handled by the container pipeline, so this fires for otherwise-healthy containers whose pod never becomes ready.

### 🎯 Severity

| Parameter | What it does |
|:---|---|
| `severityByOwnerKind` | Set severity per resource type, e.g. `StatefulSet: "high"` |
| `severityByReason` | Set severity per event reason, checked before owner kind, e.g. `OOMKilled: "high"` |

Defaults: `StatefulSet` → 🔴 high, everything else → 🟡 normal

### 🔇 Silences — stop the noise

```yaml
silences:
  - namespaces: ["kube-system", "monitoring"]
  - reasons: ["BackOff"]
  - podNamePatterns: ["my-fancy-pod-.*"]
```

Each rule can also filter by `containerNames`, `logPatterns`, `containerMessages`, `nodeReasons`, and `nodeMessages` (message substrings). An incident matching any rule is suppressed entirely. The deprecated top-level `ignore*` fields map onto these rules.

### 🚫 Inhibition — no double alerts

| Parameter | What it does |
|:---|---|
| `inhibition.nodeSuppressesPods` | ✅ Don't alert on pod issues if the node itself is down (default: true) |

Pods are suppressed **only while the node is actually down** — once the node recovers, suppression lifts immediately, even during the resolve hold-down window (it doesn't wait for the "resolved" notification to be sent).

### 📝 Custom message templates

```yaml
templates:
  CrashLoopBackOff: "{{.Incident.Name}} — {{.Action}} — {{.Incident.Hint}}"
```

### 🧠 Correlation — smart incident grouping

| Parameter | What it does |
|:---|---|
| `correlation.window` | ⏱️ Keep incidents in memory (default: 10 min) |
| `correlation.resolveHoldDown` | ⏱️ Wait before sending "resolved" (default: 300s) |
| `correlation.lifecycleInterval` | ⏱️ Lifecycle check frequency (default: 1 min) |
| `correlation.cooldownMinutes` | ⏱️ Min time between identical crash re-alerts (default: 10; 0 = off) |
| `correlation.maxBaseline` | 📈 Max baseline entries kept for startup comparison (default: 5000) |
| `correlation.escalation.enabled` | ✅ Escalate severity on repeated crashes (default: true) |
| `correlation.escalation.tiers` | 📊 Restart thresholds: `[3, 10, 50]` |
| `correlation.renotify.intervalBySeverity` | 🔔 Re-alert interval per severity (e.g. `high: 60`), `default` key as fallback; unset = off |
| `correlation.renotify.maxPerIncident` | 🔔 Max re-alerts per incident (default: 3) |

### 🧹 Smart Grouping — coalesce duplicate notifications

| Parameter | What it does |
|:---|---|
| `smartGrouping.windowSeconds` | ⏱ Grouping window in seconds (default: 60). Set to 0 to disable. |

kwatch groups related incidents by the dimension that best captures each failure type's root cause. For example, OOMKilled and probe failures group by owner+namespace, node conditions group by node (not pod errors on the same node), image pull errors group by image (or globally for rate limits), and CrashLoopBackOff with a matching log signature bridges across owners. Each group notification shows affected pods, owners, nodes, or images depending on scope, with overflow counting above 1,000 entries. After a group notification is sent, the same condition is not silently repeated on every event: re-notifications are throttled by a cooldown (4× the grouping window, clamped between 5 and 30 minutes). While the underlying members keep failing, the group resumes with a periodic UPDATE after the cooldown lapses; it resolves (and stops notifying) once the members clear. This prevents per-event flooding while still surfacing ongoing incidents.

### 📝 Audit log

| Parameter | What it does |
|:---|---|
| `auditLog.enabled` | Write one structured JSON entry per incident transition (default: true) |
| `auditLog.output` | Destination: `stdout` (default) or a file path |

Emits `create`/`update`/`resolved`/`skip` entries (with `incidentKey`, `namespace`, `reason`, `severity`, `count`, `duration`, `skipReason`) to stdout or a file for feeding into your log pipeline.

### 📋 CRD — live config changes

| Parameter | What it does |
|:---|---|
| `crd.enabled` | Watch `KwatchConfig` CRs for live config updates (default: false) |

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

---

## 🔔 Alert providers

### 💬 Slack

**Webhook mode:**
| Parameter | What it does |
|:---|---|
| `alert.slack.webhook` | 🔗 Slack webhook URL |
| `alert.slack.channel` | 📢 Override channel |
| `alert.slack.title` | ✏️ Custom title |
| `alert.slack.text` | ✏️ Custom text |
| `alert.slack.compact` | 📏 Single-line mode |

**Bot Token mode:**
| Parameter | What it does |
|:---|---|
| `alert.slack.token` | 🔑 Bot token (`xoxb-...`) |
| `alert.slack.channel` | 📢 Channel to post to |
| `alert.slack.title` | ✏️ Custom title |
| `alert.slack.text` | ✏️ Custom text |
| `alert.slack.compact` | 📏 Single-line mode |

**Compact mode:**
```yaml
alert:
  slack:
    webhook: "https://hooks.slack.com/..."
    compact: true
```

> 💡 **Pro tip:** When using bot token mode, alerts become threaded conversations — root message on first alert, updates as replies. Clean and organized! 🧹

#### 📮 Provider Routing & Retry

```yaml
alert:
  slack:
    webhook: "<url>"
    routes:
      - namespaces: ["production"]
        severities: ["high", "critical"]
    retry:
      maxAttempts: 3
      delay: 5s
```

Need a backup? Set a fallback:
```yaml
alert:
  slack:
    webhook: "<url>"
    fallback: "pagerduty"    # 🆘 tries PagerDuty if Slack fails
    retry:
      maxAttempts: 3
```

### 💬 Discord

| Parameter | What it does |
|:---|---|
| `alert.discord.webhook` | 🔗 Discord webhook URL |
| `alert.discord.title` | ✏️ Custom title |
| `alert.discord.text` | ✏️ Custom text |

### 📧 Email

| Parameter | What it does |
|:---|---|
| `alert.email.from` | 📤 From address |
| `alert.email.password` | 🔑 From password |
| `alert.email.host` | 🖥️ SMTP host |
| `alert.email.port` | 🔌 SMTP port |
| `alert.email.to` | 📥 Receiver email |

### 🚨 PagerDuty

| Parameter | What it does |
|:---|---|
| `alert.pagerduty.integrationKey` | 🔑 PagerDuty integration key |

### ✈️ Telegram

| Parameter | What it does |
|:---|---|
| `alert.telegram.token` | 🔑 Bot token |
| `alert.telegram.chatId` | 💬 Chat ID |

### 💼 Microsoft Teams

| Parameter | What it does |
|:---|---|
| `alert.teams.webhook` | 🔗 Webhook URL |
| `alert.teams.title` | ✏️ Custom title |
| `alert.teams.text` | ✏️ Custom text |

### 🚀 Rocket Chat

| Parameter | What it does |
|:---|---|
| `alert.rocketchat.webhook` | 🔗 Webhook URL |
| `alert.rocketchat.text` | ✏️ Custom text |

### 🌐 Mattermost

| Parameter | What it does |
|:---|---|
| `alert.mattermost.webhook` | 🔗 Webhook URL |
| `alert.mattermost.title` | ✏️ Custom title |
| `alert.mattermost.text` | ✏️ Custom text |

### 🔔 Opsgenie

| Parameter | What it does |
|:---|---|
| `alert.opsgenie.apiKey` | 🔑 API Key |
| `alert.opsgenie.title` | ✏️ Custom title |
| `alert.opsgenie.text` | ✏️ Custom text |

### 🏗️ Matrix

| Parameter | What it does |
|:---|---|
| `alert.matrix.homeServer` | 🖥️ HomeServer URL |
| `alert.matrix.accessToken` | 🔑 Access token |
| `alert.matrix.internalRoomID` | 🆔 Room ID |
| `alert.matrix.title` | ✏️ Custom title |
| `alert.matrix.text` | ✏️ Custom text |

### 🔔 DingTalk

| Parameter | What it does |
|:---|---|
| `alert.dingtalk.accessToken` | 🔑 Access token |
| `alert.dingtalk.secret` | 🔐 Signing secret |
| `alert.dingtalk.title` | ✏️ Custom title |

### 🐦 FeiShu

| Parameter | What it does |
|:---|---|
| `alert.feishu.webhook` | 🔗 Webhook URL |
| `alert.feishu.title` | ✏️ Custom title |

### 🛡️ Zenduty

| Parameter | What it does |
|:---|---|
| `alert.zenduty.integrationKey` | 🔑 Integration Key |
| `alert.zenduty.alertType` | 🏷️ Alert type (default: critical) |

### 💬 Google Chat

| Parameter | What it does |
|:---|---|
| `alert.googlechat.webhook` | 🔗 Webhook URL |
| `alert.googlechat.text` | ✏️ Custom text |

### 📳 Gotify

| Parameter | What it does |
|:---|---|
| `alert.gotify.url` | 🔗 Gotify server URL |
| `alert.gotify.token` | 🔑 App token |

```yaml
alert:
  gotify:
    url: "https://gotify.example.com"
    token: "SOMETOKEN"
```

### 🔔 Ntfy

| Parameter | What it does |
|:---|---|
| `alert.ntfy.topic` | 📢 Topic to publish to |
| `alert.ntfy.url` | 🔗 Server URL (default: `https://ntfy.sh`) |
| `alert.ntfy.token` | 🔑 Optional auth token |
| `alert.ntfy.priority` | 🎚️ Priority 1-5 (default: 4) |

```yaml
alert:
  ntfy:
    topic: "kwatch-alerts"
```

### 📲 Pushover

| Parameter | What it does |
|:---|---|
| `alert.pushover.token` | 🔑 Application token |
| `alert.pushover.user` | 👤 User or group key |

### 🟣 Webex

| Parameter | What it does |
|:---|---|
| `alert.webex.accessToken` | 🔑 Bot access token |
| `alert.webex.roomId` | 🚪 Room ID (optional) |
| `alert.webex.toPersonEmail` | ✉️ Person email (optional) |

### 🐙 GitHub

| Parameter | What it does |
|:---|---|
| `alert.github.token` | 🔑 Personal access token |
| `alert.github.owner` | 👤 Repository owner |
| `alert.github.repo` | 📦 Repository name |

```yaml
alert:
  github:
    token: "ghp_..."
    owner: "acme"
    repo: "infra"
```

### 🦊 GitLab

| Parameter | What it does |
|:---|---|
| `alert.gitlab.token` | 🔑 Personal access token |
| `alert.gitlab.projectId` | 🆔 Project ID |

```yaml
alert:
  gitlab:
    token: "glpat-..."
    projectId: "12345"
```

### 🦘 Gitea

| Parameter | What it does |
|:---|---|
| `alert.gitea.token` | 🔑 Access token |
| `alert.gitea.owner` | 👤 Repository owner |
| `alert.gitea.repo` | 📦 Repository name |

### 🧩 Zapier

| Parameter | What it does |
|:---|---|
| `alert.zapier.url` | 🔗 Zap webhook URL |
| `alert.zapier.token` | 🔑 Optional token |

### ⚡ n8n

| Parameter | What it does |
|:---|---|
| `alert.n8n.url` | 🔗 Workflow webhook URL |
| `alert.n8n.token` | 🔑 Optional auth header value |

### 🧙 IFTTT

| Parameter | What it does |
|:---|---|
| `alert.ifttt.key` | 🔑 Webhooks key |
| `alert.ifttt.event` | 🎯 Event name (default: `kwatch`) |

```yaml
alert:
  ifttt:
    key: "d3L..."
```

### 🗒️ Microsoft Teams Workflow

| Parameter | What it does |
|:---|---|
| `alert.teamsworkflow.webhook` | 🔗 Power Automate / Teams Workflow URL |

### 👑 Zulip

| Parameter | What it does |
|:---|---|
| `alert.zulip.email` | ✉️ Bot email |
| `alert.zulip.token` | 🔑 Bot API key |
| `alert.zulip.channel` | 📢 Channel/stream to post to |
| `alert.zulip.url` | 🔗 Server URL (default: `https://zulip.example.com/api/v1/messages`) |

### 🏠 HomeAssistant

| Parameter | What it does |
|:---|---|
| `alert.homeassistant.token` | 🔑 Long-lived access token |
| `alert.homeassistant.url` | 🔗 Server URL (default: `http://localhost:8123`) |
| `alert.homeassistant.service` | 🔧 Notification service (default: `notify`) |

### 🔆 Splunk

| Parameter | What it does |
|:---|---|
| `alert.splunk.url` | 🔗 HEC endpoint URL |
| `alert.splunk.token` | 🔑 HEC token |
| `alert.splunk.source` | 🏷️ Source name (optional) |
| `alert.splunk.sourcetype` | 🏷️ Source type (optional) |
| `alert.splunk.index` | 📚 Index name (optional) |
| `alert.splunk.host` | 🖥️ Host name (optional) |

```yaml
alert:
  splunk:
    url: "https://splunk.example.com:8088/services/collector/event"
    token: "Splunk-..."
```

### 🐕 Datadog

| Parameter | What it does |
|:---|---|
| `alert.datadog.apiKey` | 🔑 API key |
| `alert.datadog.site` | 🌍 Datadog site (default: `datadoghq.com`) |
| `alert.datadog.applicationKey` | 🔑 Optional application key |
| `alert.datadog.alertType` | 🏷️ Alert type (default: `error`) |
| `alert.datadog.tags` | 🏷️ Comma-separated tags |

### 📈 New Relic

| Parameter | What it does |
|:---|---|
| `alert.newrelic.apiKey` | 🔑 User API key |
| `alert.newrelic.accountId` | 🆔 Account ID |

```yaml
alert:
  newrelic:
    apiKey: "NRAK-..."
    accountId: "1234567"
```

### 📋 ClickUp

| Parameter | What it does |
|:---|---|
| `alert.clickup.token` | 🔑 Personal API token |
| `alert.clickup.listId` | 🆔 List ID to create tasks in |
| `alert.clickup.priority` | 🎚️ Optional task priority (1-4) |

```yaml
alert:
  clickup:
    token: "pk_..."
    listId: "901234567"
```

### 🔭 ilert

| Parameter | What it does |
|:---|---|
| `alert.ilert.integrationKey` | 🔑 Integration key |
| `alert.ilert.priority` | 🎚️ Priority (LOW/HIGH/CRITICAL, default: HIGH) |

### 🚨 Incident.io

| Parameter | What it does |
|:---|---|
| `alert.incidentio.url` | 🔗 Incident.io URL |
| `alert.incidentio.apiKey` | 🔑 Optional API key |

> 💡 Also accepted as `incident.io` in config.

### 📟 Squadcast

| Parameter | What it does |
|:---|---|
| `alert.squadcast.serviceKey` | 🔑 Service key |

### 🆘 SIGNL4

| Parameter | What it does |
|:---|---|
| `alert.signl4.teamSecret` | 🔑 Team secret |
| `alert.signl4.title` | ✏️ Custom title |
| `alert.signl4.user` | 👤 Optional alerting user |

### ✉️ Twilio

| Parameter | What it does |
|:---|---|
| `alert.twilio.accountSid` | 🔑 Account SID |
| `alert.twilio.authToken` | 🔑 Auth token |
| `alert.twilio.from` | 📤 Sender phone number |
| `alert.twilio.to` | 📥 Recipient phone number |

```yaml
alert:
  twilio:
    accountSid: "AC..."
    authToken: "..."
    from: "+12025550100"
    to: "+12025550101"
```

### 📱 Vonage

| Parameter | What it does |
|:---|---|
| `alert.vonage.apiKey` | 🔑 API key |
| `alert.vonage.apiSecret` | 🔑 API secret |
| `alert.vonage.from` | 📤 Sender name/number |
| `alert.vonage.to` | 📥 Recipient phone number |

### 📱 Plivo

| Parameter | What it does |
|:---|---|
| `alert.plivo.authId` | 🔑 Auth ID |
| `alert.plivo.authToken` | 🔑 Auth token |
| `alert.plivo.from` | 📤 Sender number |
| `alert.plivo.to` | 📥 Recipient phone number |

### 🐦 MessageBird

| Parameter | What it does |
|:---|---|
| `alert.messagebird.accessKey` | 🔑 Access key |
| `alert.messagebird.from` | 📤 Sender number |
| `alert.messagebird.to` | 📥 Recipient phone number |

### 🟡 Signal

| Parameter | What it does |
|:---|---|
| `alert.signal.number` | 📤 Sender phone number |
| `alert.signal.to` | 📥 Recipient phone number |
| `alert.signal.url` | 🔗 REST API URL (default: `http://localhost:8080`) |

### ✈️ SendGrid

| Parameter | What it does |
|:---|---|
| `alert.sendgrid.apiKey` | 🔑 API key |
| `alert.sendgrid.from` | 📤 From address |
| `alert.sendgrid.to` | 📥 Recipients (list of addresses) |
| `alert.sendgrid.subject` | ✏️ Email subject |

```yaml
alert:
  sendgrid:
    apiKey: "SG..."
    from: "kwatch@example.com"
    to:
      - "ops@example.com"
      - "oncall@example.com"
```

### ☁️ AWS SES

| Parameter | What it does |
|:---|---|
| `alert.ses.accessKeyId` | 🔑 AWS access key ID |
| `alert.ses.secretAccessKey` | 🔑 AWS secret access key |
| `alert.ses.region` | 🌍 AWS region (default: `us-east-1`) |
| `alert.ses.from` | 📤 Verified sender address |
| `alert.ses.to` | 📥 Recipients (comma-separated) |
| `alert.ses.subject` | ✏️ Email subject |

```yaml
alert:
  ses:
    accessKeyId: "AKIA..."
    secretAccessKey: "..."
    region: "us-east-1"
    from: "kwatch@example.com"
    to: "ops@example.com, oncall@example.com"
```

### 📣 AWS SNS

| Parameter | What it does |
|:---|---|
| `alert.sns.accessKeyId` | 🔑 AWS access key ID |
| `alert.sns.secretAccessKey` | 🔑 AWS secret access key |
| `alert.sns.region` | 🌍 AWS region (default: `us-east-1`) |
| `alert.sns.topicArn` | 📢 SNS topic ARN (or `targetArn`) |
| `alert.sns.subject` | ✏️ Optional subject (email subscriptions) |

```yaml
alert:
  sns:
    accessKeyId: "AKIA..."
    secretAccessKey: "..."
    region: "us-east-1"
    topicArn: "arn:aws:sns:us-east-1:123456789012:kwatch"
```

### 📋 Jira

| Parameter | What it does |
|:---|---|
| `alert.jira.url` | 🔗 Jira base URL |
| `alert.jira.user` | 👤 Email or username |
| `alert.jira.apiToken` | 🔑 API token |
| `alert.jira.projectKey` | 🆔 Project key |
| `alert.jira.issueType` | 🏷️ Issue type (default: `Task`) |

```yaml
alert:
  jira:
    url: "https://kwatch.atlassian.net"
    user: "ops@example.com"
    apiToken: "ATATT..."
    projectKey: "OPS"
```

### 🟩 WeCom (WeChat Work)

| Parameter | What it does |
|:---|---|
| `alert.wecom.webhook` | 🔗 Group robot webhook URL |

```yaml
alert:
  wecom:
    webhook: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
```

### 🆘 Splunk OnCall (VictorOps)

| Parameter | What it does |
|:---|---|
| `alert.splunkoncall.apiKey` | 🔑 API key |
| `alert.splunkoncall.routingKey` | 🔀 Routing key |
| `alert.splunkoncall.url` | 🔗 Optional endpoint override |

```yaml
alert:
  splunkoncall:
    apiKey: "..."
    routingKey: "everyone"
```

### ✉️ Mailgun

| Parameter | What it does |
|:---|---|
| `alert.mailgun.apiKey` | 🔑 API key |
| `alert.mailgun.domain` | 📦 Sending domain |
| `alert.mailgun.from` | 📤 From address |
| `alert.mailgun.to` | 📥 Recipients (comma-separated) |
| `alert.mailgun.subject` | ✏️ Email subject |

```yaml
alert:
  mailgun:
    apiKey: "key-..."
    domain: "mg.example.com"
    from: "kwatch@mg.example.com"
    to: "ops@example.com"
```

### ✉️ Resend

| Parameter | What it does |
|:---|---|
| `alert.resend.apiKey` | 🔑 API key |
| `alert.resend.from` | 📤 From address |
| `alert.resend.to` | 📥 Recipients (comma-separated) |
| `alert.resend.subject` | ✏️ Email subject |

```yaml
alert:
  resend:
    apiKey: "re_..."
    from: "kwatch@example.com"
    to: "ops@example.com, oncall@example.com"
```

### 🚨 GoAlert

| Parameter | What it does |
|:---|---|
| `alert.goalert.url` | 🔗 GoAlert URL (default: `https://goalert.example.com`) |
| `alert.goalert.token` | 🔑 API token |
| `alert.goalert.serviceId` | 🆔 Service ID |

```yaml
alert:
  goalert:
    token: "..."
    serviceId: "SVC123"
```

### 🚦 Alerta

| Parameter | What it does |
|:---|---|
| `alert.alerta.url` | 🔗 Alerta server URL |
| `alert.alerta.apiKey` | 🔑 API key |
| `alert.alerta.environment` | 🌍 Environment (default: `Production`) |
| `alert.alerta.service` | 🏷️ Service name (default: `kwatch`) |

```yaml
alert:
  alerta:
    url: "https://alerta.example.com"
    apiKey: "..."
```

### 🟩 Threema Gateway

| Parameter | What it does |
|:---|---|
| `alert.threema.gatewayId` | 🔑 Threema Gateway ID |
| `alert.threema.secret` | 🔑 Gateway secret |
| `alert.threema.to` | 📥 Recipient Threema ID |

### 💬 Flock

| Parameter | What it does |
|:---|---|
| `alert.flock.webhook` | 🔗 Incoming webhook URL |

### 🔵 Pushbullet

| Parameter | What it does |
|:---|---|
| `alert.pushbullet.accessToken` | 🔑 Access token |

### 📟 Sensu Go

| Parameter | What it does |
|:---|---|
| `alert.sensugo.url` | 🔗 Sensu Go API URL |
| `alert.sensugo.apiKey` | 🔑 API key |
| `alert.sensugo.namespace` | 🗂️ Namespace (default: `default`) |
| `alert.sensugo.entity` | 🖥️ Entity name (default: `kwatch`) |

```yaml
alert:
  sensugo:
    url: "http://sensu.example.com:8080"
    apiKey: "..."
```

### 🔗 Custom Webhook

| Parameter | What it does |
|:---|---|
| `alert.webhook.url` | 🔗 Webhook URL |
| `alert.webhook.headers` | 📋 Custom headers |
| `alert.webhook.basicAuth` | 🔐 Username + password |

---

## 🛠️ CLI commands

| Command | What it does |
|:---|---|
| `kwatch` | ▶️ Run the main monitor |
| `kwatch --version` | ℹ️ Print version |
| `kwatch lint` | ✅ Validate your config |
| `kwatch lint --strict` | ✅✅ Strict check (catches typos!) |
| `kwatch lint --check` | ✅✅✅ Validate + test provider credentials |
| `kwatch replay < events.jsonl` | 🎬 Replay past events to test |

---

## 🧹 Clean up

```shell
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/config.yaml
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/deploy.yaml
```

---

## 📖 Not a monitoring platform — and proud of it! 🎉

kwatch is **not** a metrics collector, dashboard, or observability backend.
No TSDB, no dashboards, no log storage, no query language.
kwatch is the **alarm** — your existing tools are the archive.

Need full observability? Pair kwatch with Prometheus + Grafana for metrics,
or Loki for logs. kwatch handles the one thing a dashboard cannot: telling
you something broke **right now**. ⏰

---

## 👍 Contribute & Support

+ ⭐ [Give us a star](https://github.com/abahmed/kwatch/stargazers) — it really helps!
+ 💡 [Suggest features](https://github.com/abahmed/kwatch/issues)
+ 🐛 [Report bugs](https://github.com/abahmed/kwatch/issues)

## 🚀 Who uses kwatch?

**kwatch** is trusted by:

[<img src="./assets/users/trella.png"/>](https://www.trella.app)
[<img src="./assets/users/ibec-systems.svg" width="50%"/>](https://ibecsystems.com/en#/)
[<img src="./assets/users/justwatch.png" width="50%"/>](https://www.justwatch.com/us/talent)

Want to add your company? [Open an issue!](https://github.com/abahmed/kwatch/issues)

## 💻 Contributors

<a href="https://github.com/abahmed/kwatch/graphs/contributors">
  <img src="https://contributors-img.firebaseapp.com/image?repo=abahmed/kwatch" />
</a>

## ⭐️ Stargazers

<img src="https://api.star-history.com/svg?repos=abahmed/kwatch&type=Date" alt="Stargazers over time" style="max-width: 100%">

## 👋 Get in touch

Questions? Suggestions? [Chat with us on Discord](https://discord.gg/kzJszdKmJ7) — we're friendly! 🎉

## ⚠️ License

kwatch is [MIT Licensed](LICENSE) — use it, fork it, share it! 🎊
