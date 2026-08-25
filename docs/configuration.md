# Configuration reference

Every kwatch configuration option, monitor, and endpoint in one place. For the 60-second
install and a quick overview, see the [README](../README.md).

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
| `resyncSeconds` | How often to re-scan everything for problems. `0` = only react to live events (recommended). |
| `workers` | How many checks to run in parallel (default: 1, raise for big clusters) |
| `namespaces` | 🔽 Watch only these namespaces — or use `!kube-system` to watch *everything except* it |
| `namespaceSelector` | 🏷️ Pick namespaces by K8s label selector (use *instead of* `namespaces`, not with it) |
| `reasons` | 🔽 Only alert on these event reasons — or exclude some with `!` (e.g. `reasons: ["!Started"]`) |
| `ignoreFailedGracefulShutdown` | ✅ Skip containers stopped by a clean/graceful shutdown (default: true — keep it) |
| `ignoreDisruptionTerminations` | ✅ Skip pods evicted during node drains (default: true — keep it) |
| `runbooks` | 📚 Add a link to your runbook for each error reason, so every alert comes with help attached |
| `containerRestartThreshold` | Alert if a container restarts this many times (default: 0, off) |
| `reportStartupBaseline` | 📋 Send one startup summary of pre-existing issues (default: true) |
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
| `includeEvents` | 📋 Include K8s events in alerts (default: true) |
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
- `GET /readyz` — ✅ Readiness
- `GET /health` — `{"status": "ok"}`
- `GET /metrics` — 📊 Prometheus metrics (incidents, notifications, baseline)
- `GET /incidents` — 📋 All active incidents (requires `diagnostics: true`)
- `POST /test-alert` — 📤 Send a test alert (requires `diagnostics: true`)
- `GET /deadletters` — 💀 Recent delivery failures (requires `diagnostics: true`)

## 🔄 Upgrader

At startup kwatch quietly asks "is there a newer version?" and mentions it. Turn it off if
you manage updates yourself (e.g. you pin images).

| Parameter | What it does |
|:---|---|
| `upgrader.disableUpdateCheck` | 🔕 Don't check for new kwatch versions |

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
| `rolloutMonitor.sustainedMinutes` | ⏱️ Minutes of unavailability before alerting (default: 2) |

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
| `controlPlaneMonitor.enabled` | 🏛️ Watch for broken control-plane components (default: true) |

Detects container issues (CrashLoopBackOff, Error, OOMKilled, etc.) in control-plane pods (kube-apiserver, kube-scheduler, kube-controller-manager, etcd, kube-proxy, coredns). Runs a dedicated sweep at startup to catch pre-existing failures.

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

Periodically computes the ratio of pod resource requests vs node allocatable for CPU and memory. Data is purely in-memory — no TSDB or persistent storage needed.

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

Alerts with `ContainersNotReady` when a Running pod's Ready condition stays false for 60 seconds. Container-level failures (crashes, waits, non-zero terminations) are handled by the container pipeline, so this fires for otherwise-healthy containers whose pod never becomes ready.

### 🎯 Severity

Every alert carries a color — **normal** or **high** — and you control the coloring. It
drives urgent channels, escalation, and re-notification.

| Parameter | What it does |
|:---|---|
| `severityByOwnerKind` | Set severity per resource type, e.g. `StatefulSet: "high"` |
| `severityByReason` | Set severity per event reason, checked before owner kind, e.g. `OOMKilled: "high"` |

Defaults: `StatefulSet` → 🔴 high, everything else → 🟡 normal

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
| `correlation.escalation.tiers` | 📊 Restart thresholds: `[3, 10, 50]` |
| `correlation.renotify.intervalBySeverity` | 🔔 Re-alert interval per severity (e.g. `high: 60`), `default` key as fallback; unset = off |
| `correlation.renotify.maxPerIncident` | 🔔 Max re-alerts per incident (default: 3) |

### 🧹 Smart Grouping — coalesce duplicate notifications

In plain words: **many pods failing the same way = one alert**, not one per pod. Events
that share a root dimension are collected over a short window and summarized into a single
notification, then re-notified on a gentle cooldown instead of on every event.

| Parameter | What it does |
|:---|---|
| `smartGrouping.windowSeconds` | ⏱ Grouping window in seconds (default: 60). Set to 0 to disable. |

kwatch groups related incidents by the dimension that best captures each failure type's root cause. For example, OOMKilled and probe failures group by owner+namespace, node conditions group by node (not pod errors on the same node), image pull errors group by image (or globally for rate limits), and CrashLoopBackOff with a matching log signature bridges across owners. Each group notification shows affected pods, owners, nodes, or images depending on scope, with overflow counting above 1,000 entries. After a group notification is sent, the same condition is not silently repeated on every event: re-notifications are throttled by a cooldown (4× the grouping window, clamped between 5 and 30 minutes). While the underlying members keep failing, the group resumes with a periodic UPDATE after the cooldown lapses; it resolves (and stops notifying) once the members clear. This prevents per-event flooding while still surfacing ongoing incidents.

### 📝 Audit log

In plain words: for every incident *transition* (created, updated, resolved, skipped),
kwatch writes one structured JSON line — feed it to your log pipeline if you want a
searchable history of everything it decided.

| Parameter | What it does |
|:---|---|
| `auditLog.enabled` | Write one structured JSON entry per incident transition (default: true) |
| `auditLog.output` | Destination: `stdout` (default) or a file path |

Emits `create`/`update`/`resolved`/`skip` entries (with `incidentKey`, `namespace`, `reason`, `severity`, `count`, `duration`, `skipReason`) to stdout or a file for feeding into your log pipeline.

### 📋 CRD — live config changes

In plain words: instead of editing the ConfigMap and restarting, you can push config changes
live with a small custom resource. Off by default — turn it on only if you manage kwatch via
kubectl and want hot reloads.

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

