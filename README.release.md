<p align="center">
  <a href="https://kwatch.dev">
    <img src="./assets/logo.png" width="30%"/>
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

> **Kubernetes alerts that tell you what broke — and why.**
> kwatch monitors your cluster and sends one clear, self-explaining
> notification the moment something crashes — 60 seconds to install, no
> backend, no dashboards.

## Why kwatch

- **Self-explaining** — OOMKilled → "hit 512Mi limit; raise limits.memory";
  probe failures include which probe and port; exit codes decoded.
- **Low-noise** — incident grouping, cooldown, node→pod inhibition,
  storm digests, silences, and routing rules keep your channels clean.
- **GitOps-native** — config as a `KwatchConfig` CR, live-reloaded without
  a restart.
- Built on four promises: **never miss** · **never lie** · **never flood** ·
  **never silently die**.

## The smart alert (before → after)

| Raw kubectl | kwatch |
|---|---|
| `CrashLoopBackOff` | 🚨 OOMKilled (memory limit: 512Mi) — consider raising limits.memory · previous logs + events inline |
| `Error` | 🚨 HTTP probe failing on :8080/healthz (exit code 137) — container OOM-killed |

## ⚡️ 60-second install

### 📦 Install

#### ⎈ Using Helm

```shell
helm repo add kwatch https://kwatch.dev/charts
helm install [RELEASE_NAME] kwatch/kwatch --namespace kwatch --create-namespace --version 0.11.0
```

To get more details, please check [chart's configuration](https://github.com/abahmed/kwatch/blob/main/deploy/chart/README.md)

#### 🐙 Using kubectl

You need to get config template to add your configs

```shell
curl  -L https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/config.yaml -o config.yaml
```

Then edit `config.yaml` file and apply your configuration

```shell
kubectl apply -f config.yaml
```

To deploy **kwatch**, execute following command:

```shell
kubectl apply -f https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/deploy.yaml
```

## ⬆️ Upgrading from v0.10.x

Most changes in this release are additive and off-by-default. The
following WILL change behavior you may depend on:

### Alert formats changed — update anything that parses kwatch messages
- **Incident messages** now include a `Severity:` line and `Logs:` /
  `Events:` blocks on creation, and there are new update / stale /
  resolved / digest message shapes.
- **Node alerts** were a single plain string
  (`Node <name> is not ready: <reason> - <message>`). They are now full
  incidents with lifecycle messages (🚨 create → ✅ resolve), deduplication
  and cooldown — expect different text and fewer repeats.
- **PVC alerts** now use the stable reason `VolumeUsageHigh` with the
  live percentage in the hint (previously the percentage was part of the
  alert itself). Update any silences, routes, or webhook consumers that
  matched the old strings.

### kwatch is now low-noise by default
All monitors are enabled by default (pod, node, PVC, *deployment rollout*,
*job*, *cronjob*, *daemonset*, *HPA*). Storm/digest aggregation, severity
escalation, and node→pod inhibition are all on. `resolveHoldDown: 30s`
prevents flap resolution storms. Health check (`/healthz`, `/readyz`) is
also on. To restore the v0.10.x off-by-default behavior, explicitly set
each monitor, storm, escalation, and inhibition to `false`.

### Edge-triggered notification — no more cooldown/stale
kwatch previously used a cooldown timer (minimum gap between updates) and
a stale threshold (time before marking an incident stale). Both are
**removed** — kwatch now compares a notification signature on every event
and only sends when state, severity, or resolved-changed meaningfully.
Existing config files mentioning `correlation.cooldown` or
`correlation.staleThreshold` are silently ignored (the yaml parser ignores
unknown fields).

### kwatch restarts no longer re-alert existing incidents
kwatch now persists a baseline of already-broken workloads (in its state
ConfigMap) and suppresses re-alerts for them across restarts. If you
relied on restarting kwatch to re-page open issues, configure
`correlation.renotify` instead.

### Custom container args are now parsed
The binary accepts flags and subcommands (`--version`, `lint`, `replay`).
Unrecognized flags now fail at startup instead of being silently ignored —
review any custom `args:` in your Deployment. Standard klog flags
(`-v`, `-logtostderr`, …) remain supported.

### RBAC — required only for newly enabled features
The default path needs no new permissions. Apply the updated
chart/manifests BEFORE enabling any of: `leaderElection`
(coordination.k8s.io/leases), `jobMonitor` / `cronJobMonitor` (batch),
`hpaMonitor` (autoscaling), `tlsMonitor` (secrets — read-widening; see
the chart README for a namespace-scoped alternative), `crd.enabled`
(kwatch.abahmed.dev/kwatchconfigs + installing `deploy/crd.yaml`).

## What it catches

| Signal | Default | Details |
|--------|---------|---------|
| Pod crashes (CrashLoop, OOM, ImagePull, Error) | **on** | Container-state + previous logs + events inline |
| Pending pods (incl. `Unschedulable`) | **on** | Threshold: 300s |
| Node conditions (NotReady, Unknown, pressure) | **on** | Per-condition severity |
| PVC usage tiers (warn / critical) | **on** | Thresholds: 80% / 90% |
| Job failures & suspension | **on** | Reason: `JobFailed` / `JobSuspended` |
| Stuck rollouts (`ProgressDeadlineExceeded`) | **on** | Deployments only |
| DaemonSet unavailability | **on** | By DaemonSet |
| CronJob suspension / missed schedules | **on** | By CronJob |
| HPA pinned at max replicas | **on** | Sustain window configurable |
| TLS certificates expiring | off | Threshold in days |
| Node crash → pod inhibition | **on** | Controlled per-cluster |

All signals beyond TLS are enabled by default for low-noise zero-config.

## kwatch vs …

| | kwatch | DIY Prometheus + Alertmanager | Heavy SaaS |
|---|---|---|---|
| Install time | ~5 minutes | hours of YAML | agent + backend setup |
| Footprint | ~20 MB single binary | whole monitoring stack | per-node agents + cloud costs |
| Alert content | Self-explaining (hint + logs + events) | Rule-defined message | Depends on configuration |
| Data storage | None (stateless) | Prometheus TSDB | Full retention (costly) |
| Learning curve | One ConfigMap | PromQL + alert rules | Platform-specific DSL |

## Not a monitoring platform

kwatch is not a metrics collector, dashboard, or observability backend.
There is no metrics/TSDB storage, no dashboards, no log storage, and no
query language. kwatch is the alarm — your existing platform is the archive.

For full observability, pair kwatch with Prometheus + Grafana for metrics,
or Loki for logs. kwatch handles the one thing a dashboard cannot: telling
you something broke *right now*.

## ⚙️ Configuration

### 🔧 General

| Parameter                      | Description   |
|:-------------------------------|:-----------------------|
| `maxRecentLogLines`            | Max tail log lines fetched from API and displayed in alert message blocks (default: 50) |
| `resyncSeconds`                | Periodic informer resync interval in seconds (default: 0). 0 = event-driven only |
| `workers`                     | Number of concurrent reconcile workers (default: 1). Raise for large clusters. |
| `namespaces`                   | Optional list of namespaces that you want to watch or forbid, if it's not provided it will watch all namespaces. If you want to forbid a namespace, configure it with `!<namespace name>`. You can either set forbidden namespaces or allowed, not both. |
| `reasons`                      | Optional list of reasons that you want to watch or forbid, if it's not provided it will watch all reasons. If you want to forbid a reason, configure it with `!<reason>`. You can either set forbidden reasons or allowed, not both.                     |
| `ignoreFailedGracefulShutdown` | If set to true, containers which are forcefully killed during shutdown (as their graceful shutdown failed) are not reported as error (default: true) |
| `ignoreDisruptionTerminations` | If set to true, suppresses alerts for evicted/terminated pods during node drains (default: true) |
| `ignoreContainerNames`         | Optional list of container names to ignore (deprecated — use silences) |
| `ignorePodNames`               | Optional list of pod name regexp patterns to ignore    |
| `ignoreLogPatterns`            | Optional list of regexp patterns of logs to ignore (deprecated — use silences) |
| `ignoreContainerMessages`      | Optional list of container messages to ignore (deprecated — use silences) |
| `ignoreNodeReasons`            | Optional list of node condition reasons to ignore (deprecated — use silences) |
| `ignoreNodeMessages`           | Optional list of node condition messages to ignore (deprecated — use silences) |
| `runbooks`                     | Optional map of reason → URL appended to incident hint (e.g. `ImagePullBackOff: "https://wiki/registry-auth"`) |
| `llm.enabled`                  | Enable AI incident enrichment via self-hosted LLM sidecar (default: true) |
| `containerRestartThreshold`    | Alert when a container exceeds this many restarts without a detect/enrich match (0 = off) |
| `reportStartupBaseline`        | If true, emits a single informational notification at startup summarizing pre-existing issues suppressed by the baseline (default: false) |

#### Namespace filter

Use the `namespaces` option to restrict which namespaces are monitored:

```yaml
# Watch only these namespaces
namespaces:
  - default
  - kube-system
```

Prefix with `!` to exclude namespaces (cannot mix allow and forbid):

```yaml
# Watch all namespaces except those listed
namespaces:
  - !kube-system
  - !monitoring
```

#### Reason filter

Use the `reasons` option to filter by Kubernetes event reason:

```yaml
# Only alert on these reasons
reasons:
  - CrashLoopBackOff
  - ImagePullBackOff
```

Prefix with `!` to exclude reasons:

```yaml
# Alert on everything except these reasons
reasons:
  - !Started
  - !Killing
```

### 📱 App

| Parameter                     | Description                                 |
|:------------------------------|:------------------------------------------- |
| `app.proxyURL` | used in outgoing http(s) requests except Kubernetes requests to cluster optionally |
| `app.clusterName` | used in notifications to indicate which cluster has issue |
| `app.disableStartupMessage` | If set to true, welcome message will not be sent to notification channels |
| `app.logFormatter` | used for setting custom formatter when app prints logs: text, json (default: text) |
| `includeEvents` | Include Kubernetes events in alert messages (default: true) |
| `includeLogs` | Include container logs in alert messages (default: true) |


### 💓 Health Check

| Parameter                     | Description                                 |
|:------------------------------|:------------------------------------------- |
| `healthCheck.enabled` | If set to true, enables health check endpoints (default: true) |
| `healthCheck.port` | Port for health check endpoints (default: 8060) |
| `healthCheck.pprof` | Enable /debug/pprof/* endpoints (default: false) |
| `healthCheck.diagnostics` | Enable /incidents, /test-alert, and /deadletters endpoints (default: false) |

**Endpoints:**
- `GET /healthz` - Liveness probe (text/plain: "OK")
- `GET /readyz` - Readiness probe (text/plain: "OK")
- `GET /health` - Returns `{"status": "ok"}` (application/json)
- `GET /incidents` - Returns all active incidents as JSON (requires `healthCheck.diagnostics: true`)
- `POST /test-alert` - Sends a test alert through all configured providers (requires `healthCheck.diagnostics: true`)
- `GET /deadletters` - Returns recent delivery failures (last 100) as JSON (requires `healthCheck.diagnostics: true`)
- `GET /debug/pprof/` - Go pprof index (runtime profiling data, when enabled)
- `--version` flag - Prints version and exits


### 🔄 Upgrader

| Parameter                     | Description                                 |
|:------------------------------|:------------------------------------------- |
| `upgrader.disableUpdateCheck` | If set to true, does not check for and notify about kwatch updates |

### 💾 PVC Monitor

| Parameter                    | Description                                 |
|:-----------------------------|:------------------------------------------- |
| `pvcMonitor.enabled`         | to enable or disable this module (default: true) |
| `pvcMonitor.interval`        | the frequency (in minutes) to check pvc usage in the cluster  (default: 5) |
| `pvcMonitor.threshold`       | the percentage of accepted pvc usage (warn tier). if current usage exceeds this value, it will send a notification with normal severity (default: 80) |
| `pvcMonitor.criticalThreshold` | the percentage above which severity is "high" (default: 90) |
| `pvcMonitor.clearThreshold`    | the percentage below which a PVC alert is resolved (default: 75) |


### 🖥️ Node Monitor

| Parameter                    | Description                                 |
|:-----------------------------|:------------------------------------------- |
| `nodeMonitor.enabled`        | to enable or disable node monitoring (default: true) |

Node monitoring alerts on `NotReady` and `Unknown` conditions, plus `MemoryPressure`, `DiskPressure`, `PIDPressure`, and `NetworkUnavailable`. Node-specific suppression is available via `ignoreNodeReasons` and `ignoreNodeMessages`.

### 🚀 Rollout Monitor

| Parameter                       | Description                                                    |
|:--------------------------------|:-------------------------------------------------------------- |
| `rolloutMonitor.enabled`        | Watch Deployments for stuck rollouts (`ProgressDeadlineExceeded`) (default: true) |

### 🖥️ DaemonSet Monitor

| Parameter                       | Description                                                    |
|:--------------------------------|:-------------------------------------------------------------- |
| `daemonSetMonitor.enabled`      | Watch DaemonSets for unavailable pods (default: true)          |

Alerts when `status.numberUnavailable > 0`, resolves when all pods become available.

### 🧑‍💼 Job Monitor

| Parameter                  | Description                                                 |
|:---------------------------|:----------------------------------------------------------- |
| `jobMonitor.enabled`       | Watch Jobs for failures and suspension (default: true)       |

Alerts with `JobFailed` or `JobSuspended` reasons.

### ⏰ CronJob Monitor

| Parameter                       | Description                                                    |
|:--------------------------------|:-------------------------------------------------------------- |
| `cronJobMonitor.enabled`        | Watch CronJobs for suspension or missed schedules (default: true) |

Alerts when a CronJob is suspended (`spec.suspend: true`) or has not been scheduled within the last 24 hours.

### 📈 HPA Monitor

| Parameter                       | Description                                                    |
|:--------------------------------|:-------------------------------------------------------------- |
| `hpaMonitor.enabled`            | Watch HPAs for maxed-out replicas (default: true)               |
| `hpaMonitor.sustainedMinutes`   | Minutes an HPA must be maxed before alerting (default: 20)      |

Alerts with reason `HPAMaxedOut` when an HPA has scaled to its maximum replica count.

### 💓 Heartbeat Monitor (dead man's switch)

| Parameter                       | Description                                                    |
|:--------------------------------|:-------------------------------------------------------------- |
| `heartbeatMonitor.enabled`      | Send periodic pings to an external health-check endpoint (default: false) |
| `heartbeatMonitor.interval`     | Seconds between pings (default: 300)                            |
| `heartbeatMonitor.url`          | External URL to ping (e.g. Healthchecks.io)                     |

When enabled, kwatch sends a GET request to the configured URL every interval. If kwatch stops or crashes, the external monitor stops receiving pings and alerts.

### 🔒 TLS Certificate Monitor

| Parameter                       | Description                                                    |
|:--------------------------------|:-------------------------------------------------------------- |
| `tlsMonitor.enabled`            | Monitor TLS secret certificates for expiry (default: false)    |
| `tlsMonitor.threshold`          | Days before expiry to start alerting (default: 30)             |
| `tlsMonitor.criticalThreshold`  | Days before expiry for high severity (default: 3)              |

TLS is the only monitor off by default; enable it and grant RBAC to `secrets` if needed.

### ⏳ Pending Pod Threshold

| Parameter                       | Description                                                        |
|:--------------------------------|:------------------------------------------------------------------ |
| `pendingPodThreshold`           | Seconds a pod can stay in Pending before alerting (default: 300)   |

### 🎯 Severity

| Parameter                          | Description                                                        |
|:-----------------------------------|:------------------------------------------------------------------ |
| `severityByOwnerKind`              | Map owner kinds to severity levels, e.g. `{"StatefulSet": "high", "default": "normal"}` |

Built-in defaults: `StatefulSet` → `high`, all others → `normal`.

### 🔇 Silences

Suppress alerts matching namespace, reason, or pod name patterns:

```yaml
silences:
  - namespaces: ["kube-system", "monitoring"]
  - reasons: ["BackOff"]
  - podNamePatterns: ["my-fancy-pod-.*"]
```

### 🔄 Resync

| Parameter                | Description                                                           |
|:-------------------------|:--------------------------------------------------------------------- |
| `resyncSeconds`          | Periodic informer resync interval in seconds (0 = event-driven only)  |

### 📋 CRD Configuration

| Parameter                | Description                                                           |
|:-------------------------|:--------------------------------------------------------------------- |
| `crd.enabled`            | Watch KwatchConfig CRs for live config changes (default: false)       |

Hot-applied: `maxRecentLogLines`, `silences`, `severityByOwnerKind`. Restart-only fields are logged and skipped. CR deletion restores boot-time ConfigMap snapshot.

**Example KwatchConfig CR:**
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
  severityByOwnerKind:
    StatefulSet: "high"
```

### 🚫 Inhibition

| Parameter                          | Description                                                        |
|:-----------------------------------|:------------------------------------------------------------------ |
| `inhibition.nodeSuppressesPods`    | Suppress pod incidents on nodes with an active node incident (default: true) |

When a node-level incident (e.g. `NotReady`) is active, pod incidents on that same node are skipped to reduce noise during node outages.

### ⛈️ Storm / Digest

Aggregate rapidly-firing incidents into periodic digests to prevent alert storms.

| Parameter                          | Description                                                        |
|:-----------------------------------|:------------------------------------------------------------------ |
| `storm.enabled`                    | Enable digest aggregation (default: true)                           |
| `storm.threshold`                  | Max creates per window before digest mode activates (default: 10)   |
| `storm.windowMinutes`              | Sliding window (minutes) for rate tracking (default: 5)             |
| `storm.digestIntervalMinutes`      | How often (minutes) a digest summary is sent (default: 5)           |

When the create rate exceeds `threshold` within `windowMinutes`, new incidents are silently buffered and a single summary message is sent every `digestIntervalMinutes`.

### 📝 Templates

Override alert message formatting per incident reason using Go `text/template`:

```yaml
templates:
  CrashLoopBackOff: "{{.Incident.Name}} — {{.Action}} — {{.Incident.Hint}}"
```

Available template keys:
- `{{.Incident.Key}}`, `{{.Incident.Reason}}`, `{{.Incident.Name}}`, `{{.Incident.Namespace}}`, `{{.Incident.Hint}}`
- `{{.Action}}` — `create`, `update`, `stale`, `resolved`
- `{{.Message}}` — the default formatted message

### 🧠 Correlation

Incident grouping and lifecycle management. Events from the same owner/reason/container are grouped into incidents, with stale detection and auto-resolution.

| Parameter                          | Description                                                        |
|:-----------------------------------|:------------------------------------------------------------------ |
| `correlation.window`               | Time window (minutes) to keep incidents in memory (default: 10)    |
| `correlation.resolveHoldDown`      | Seconds to wait before sending a resolved notification (default: 30) |
| `correlation.lifecycleInterval`    | Interval (minutes) for lifecycle checks (default: 1)               |
| `correlation.startupQuiet`         | **Removed** — no longer supported. Previously a quiet period (seconds) after startup with no alerts (default: 30). In-cluster state restore via `buildSeenSet` replaces this feature. |
| `correlation.escalation.enabled`   | Escalate severity based on container restart count (default: true) |
| `correlation.escalation.tiers`     | Ordered restart thresholds, e.g. `[3, 10, 50]` → 3+ "high", 10+ "critical" |
| `correlation.renotify.maxPerIncident` | Max renotifications per incident (default: 3)                    |

⚠️ v0.10.x fields `cooldown` and `staleThreshold` are removed. Flat `renotify.interval` removed; use `renotify.intervalBySeverity["default"]` instead.

When Slack is configured with a bot token, incidents are sent as threaded messages: a root message on creation, with updates, stale, and resolved notifications as thread replies.

Noise filter automatically skips `Normal`/`Scheduled`/`Pulled`/`Pulling` events before correlation to reduce alert fatigue.

### 🤖 AI Enrichment

kwatch can optionally append a root-cause analysis to incident alerts using a
self-hosted LLM sidecar. Everything runs inside your cluster — no external API
call, no data leaves the cluster.

| Parameter               | Description                                                        |
|:------------------------|:-------------------------------------------------------------------|
| `llm.enabled`           | Enable AI enrichment via the built-in LLM sidecar (default: true) |

### 🔔 Alerts

#### Slack



If you want to enable Slack, provide either a webhook URL or a bot token with channel

**Webhook mode:**

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.slack.webhook`            | Slack webhook URL                           |
| `alert.slack.channel`            | Used by legacy webhooks to send messages to specific channel instead of default one |
| `alert.slack.title`              | Customized title in slack message           |
| `alert.slack.text`               | Customized text in slack message            |
| `alert.slack.compact`            | Single-line message instead of rich embed (`true`/`false`) |

**Bot Token mode:**

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.slack.token`              | Slack bot token (xoxb-...)                  |
| `alert.slack.channel`            | Channel to post to (e.g. #alerts)           |
| `alert.slack.title`              | Customized title in slack message           |
| `alert.slack.text`               | Customized text in slack message            |
| `alert.slack.compact`            | Single-line message instead of rich embed (`true`/`false`) |

#### Compact mode

Set `compact: true` to send a single-line message instead of the rich embed:

```yaml
alert:
  slack:
    webhook: "https://hooks.slack.com/..."
    compact: true
```

> **Incident mode** — When correlation is enabled and Slack is in bot token mode, alerts are sent as threaded conversations. A root message is created on the first occurrence, with updates, stale, and resolved notifications posted as thread replies. The incident message includes enriched fields: Owner Kind, Container Name, Restart Count, Severity, and Hint (e.g. "Memory pressure", "Registry/Auth").

#### Provider Routing & Retry

Each provider supports optional routing and retry:

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

When `routes` are configured, only matching incidents are delivered to that provider. When omitted, all incidents are delivered (default). Retry is configurable per provider with `maxAttempts` (default 1) and `delay` (default 1s).

**Fallback provider** — When `maxAttempts` is exhausted, a fallback provider can be called as a last resort. Configure with the `fallback` key:

```yaml
alert:
  slack:
    webhook: "<url>"
    fallback: "pagerduty"    # name of another provider entry
    retry:
      maxAttempts: 3
      delay: 5s
```

The fallback sends a single prefixed message (no further retry or fallback recursion).

#### Discord



If you want to enable Discord, provide the webhook with optional text and title

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.discord.webhook`          | Discord webhook URL                         |
| `alert.discord.title`            | Customized title in discord message         |
| `alert.discord.text`             | Customized text in discord message          |

#### Email



If you want to enable Email, provide the from and to emails with host and the port

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.email.from`               | From email                                  |
| `alert.email.password`           | From email Password                         |
| `alert.email.host`               | provide the host                            |
| `alert.email.port`               | provide the port                            |
| `alert.email.to`                 | the receiver email                          |

#### PagerDuty



If you want to enable PagerDuty, provide the integration key

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.pagerduty.integrationKey` | PagerDuty integration key [more info](https://support.pagerduty.com/docs/services-and-integrations) |

#### Telegram



If you want to enable Telegram, provide a valid token and the chat Id.

| Parameter                        | Description                                     |
|:---------------------------------|:------------------------------------------------|
| `alert.telegram.token`           | Telegram token                                  |
| `alert.telegram.chatId`          | Telegram chat id                                |

#### Microsoft Teams



If you want to enable Microsoft Teams, provide the channel webhook.

| Parameter                        | Description                                     |
|:---------------------------------|:------------------------------------------------|
| `alert.teams.webhook`            |  webhook Microsoft team                         |
| `alert.teams.title`              | Customized title in Microsoft teams message     |
| `alert.teams.text`               | Customized title in Microsoft teams message     |

#### Rocket Chat



If you want to enable Rocket Chat, provide the webhook with optional text

| Parameter                  | Description                            |
|:---------------------------|:---------------------------------------|
| `alert.rocketchat.webhook` | Rocket Chat webhook URL                |
| `alert.rocketchat.text`    | Customized text in rocket chat message |

#### Mattermost



If you want to enable Mattermost, provide the webhook with optional text and title

| Parameter                             | Description                               |
|:--------------------------------------|:----------------------------------------- |
| `alert.mattermost.webhook`            | Mattermost webhook URL                    |
| `alert.mattermost.title`              | Customized title in Mattermost message    |
| `alert.mattermost.text`               | Customized text in Mattermost message     |

#### Opsgenie



If you want to enable Opsgenie, provide the API key with optional text and title

| Parameter                             | Description                             |
|:--------------------------------------|:--------------------------------------- |
| `alert.opsgenie.apiKey`               | Opsgenie API Key                        |
| `alert.opsgenie.title`                | Customized title in Opsgenie message    |
| `alert.opsgenie.text`                 | Customized text in Opsgenie message     |

#### Matrix



If you want to enable Matrix, provide homeServer, accessToken and internalRoomID
with optional text and title

| Parameter                           | Description                            |
|:------------------------------------|:-------------------------------------- |
| `alert.matrix.homeServer`           | HomeServer URL                         |
| `alert.matrix.accessToken`          | Account access token                   |
| `alert.matrix.internalRoomID`       | Internal room ID                       |
| `alert.matrix.title`                | Customized title in message            |
| `alert.matrix.text`                 | Customized text in message             |

#### DingTalk

If you want to enable DingTalk, provide accessToken with optional secret and
title

| Parameter                           | Description                            |
|:------------------------------------|:-------------------------------------- |
| `alert.dingtalk.accessToken`        | Chat access token                      |
| `alert.dingtalk.secret`             | Optional secret used to sign requests  |
| `alert.dingtalk.title`              | Customized title in message            |

#### FeiShu


If you want to enable FeiShu, provide accessToken with optional secret and
title

| Parameter                | Description                 |
|:-------------------------|:----------------------------|
| `alert.feishu.webhook`   | FeiShu bot webhook URL      |
| `alert.feishu.title`     | Customized title in message |

#### Zenduty


If you want to enable Zenduty, provide IntegrationKey with optional alert type

| Parameter                      | Description                 |
|:-------------------------------|:----------------------------|
| `alert.zenduty.integrationKey` | Zenduty integration Key     |
| `alert.zenduty.alertType`      | Optional alert type of incident: critical, acknowledged, resolved, error, warning, info (default: critical) |

#### Google Chat



If you want to enable Google Chat, provide the webhook with optional text

| Parameter                  | Description                            |
|:---------------------------|:---------------------------------------|
| `alert.googlechat.webhook` | Google Chat webhook URL                |
| `alert.googlechat.text`    | Customized text in Google Chat message |

#### Custom webhook

If you want to enable custom webhook, provide url with optional headers and
basic auth

| Parameter                 | Description                     |
|:--------------------------|:--------------------------------|
| `alert.webhook.url`       | Webhook URL                     |
| `alert.webhook.headers`   | optional list of name and value |
| `alert.webhook.basicAuth` | optional username and password  |

### 🛠️ CLI

| Command                        | Description                                                       |
|:-------------------------------|:----------------------------------------------------------------- |
| `kwatch`                       | Run the main monitoring daemon                                    |
| `kwatch --version`             | Print version and exit                                            |
| `kwatch lint`                  | Validate config file and print errors to stderr (exit 1 on failure) |
| `kwatch lint --strict`         | Strict decode — rejects unknown YAML keys (typos, removed fields) |
| `kwatch lint --check`          | Validate config + verify provider credentials (pre-flight)        |
| `kwatch replay < events.jsonl` | Replay JSONL events from stdin through the alert pipeline         |

`kwatch replay` reads JSON lines from stdin in the following format:

```json
{"podName": "test-pod", "namespace": "default", "reason": "CrashLoopBackOff", "events": "Back-off restarting failed container"}
```

### ⚠️ Upgrading

#### v0.x → v0.(x+1): Silences consolidation

Deprecated `ignore*` fields (`ignoreContainerNames`, `ignoreLogPatterns`,
`ignoreContainerMessages`, `ignoreNodeReasons`, `ignoreNodeMessages`) still
work but emit a startup warning. Migrate to the unified `silences:` block:

```yaml
# Old (still works):
ignoreContainerNames: ["sidecar-proxy"]

# New:
silences:
  - containerNames: ["sidecar-proxy"]
```

`SilenceRule` now supports all the same fields: `containerNames`,
`logPatterns`, `containerMessages`, `nodeReasons`, `nodeMessages`.

#### Strict config validation

`kwatch lint --strict` re-decodes the config file with `yaml.KnownFields(true)`
to catch typos and removed keys. Add it to your CI pipeline:

```shell
kwatch lint --strict
```

#### Provider credential pre-flight

`kwatch lint --check` loads the config and calls `Verify()` on each provider
that supports it (Telegram: `getMe`, Discord: webhook GET, Slack: `auth.test`):

```shell
kwatch lint --check
```

#### Upgrader `--skip-upgrade` flag

Set the environment variable `SKIP_UPGRADE_CHECK=1` or the config field
`upgrader.disableUpdateCheck: true` to suppress kwatch's release-check on
startup (#438).

#### v0.(x+1): Phase 0 bug fixes

**`correlation.startupQuiet` removed** — The `startupQuiet` field is removed
from both the YAML config and the `KwatchConfig` CRD. The engine now relies on
`buildSeenSet` (in-cluster state restore) instead. Configs that include
`startupQuiet` will be silently ignored (permissive decode) or rejected by
`kwatch lint --strict`.

**Init container alerts improved** — Init container failures now use reason
`InitContainerError` instead of the container's underlying state reason
(e.g. `CrashLoopBackOff`). This affects routing rules, silences, and templates
that match on reason strings — update any such rules.

**OOMKilled alerts without memory limits** — When a container is OOMKilled but
has no memory limit set, the hint now says *"OOMKilled with no memory limit set
— node-level memory pressure; set/raise container memory limits"* instead of a
generic OOM message.

**Discord/Telegram use shared HTTP client** — Both providers now use the
kwatch shared HTTP client with proper timeout instead of a bare
`&http.Client{}`. No config change needed, but socket exhaustion on very large
clusters may decrease slightly.

**`/readyz` lags during startup** — The readiness endpoint returns 503 until
leader-elected tasks (informers synced, baseline built) complete. Previously it
returned 200 immediately. Add a `readinessProbe.initialDelaySeconds` if your
orchestrator probes `/readyz` before the leader is elected.

**`maxRecentLogLines: 0` now capped** — When `maxRecentLogLines` is set to 0
(or omitted), log fetch now defaults to a tail of 500 lines with a 1 MB
`LimitBytes` cap. Previously, 0 meant "unbounded" — the handler could fetch
megabytes of logs for a single incident.

### 📋 Guarantees

kwatch follows semver. Within the same major version:

- **Config schema** is additive-only. New optional fields may be added; existing
  fields will not be removed or change type without a major version bump.
- **Alert message format** is considered informative (not machine-parsable).
  Fields may be added to messages in minor releases.
- **`kwatch lint --strict`** catches unknown YAML keys, guarding against typos
  on new fields.
- **CRD shape** (`KwatchConfig`) follows the same additive policy.

### 📖 Recipes

#### Silence noisy sidecars (#82)

```yaml
silences:
  - containerNames: ["istio-proxy", "envoy"]
```

#### Silence all CrashLoopBackOff in CI namespaces (#140)

```yaml
silences:
  - namespaces: ["ci", "staging"]
    reasons: ["CrashLoopBackOff"]
```

### 🧹 Cleanup

```shell
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/config.yaml
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.11.0/deploy/deploy.yaml
```

## 👍 Contribute & Support

+ Add a [GitHub Star](https://github.com/abahmed/kwatch/stargazers)
+ [Suggest new features, ideas and optimizations](https://github.com/abahmed/kwatch/issues)
+ [Report issues](https://github.com/abahmed/kwatch/issues)

## 🚀 Who uses kwatch?

**kwatch** is being used by multiple entities including, but not limited to

[<img src="./assets/users/trella.png"/>](https://www.trella.app)
[<img src="./assets/users/ibec-systems.svg" width="50%"/>](https://ibecsystems.com/en#/)
[<img src="./assets/users/justwatch.png" width="50%"/>](https://www.justwatch.com/us/talent)

If you want to add your entity, [open issue](https://github.com/abahmed/kwatch/issues) to add it

## 💻 Contributors

<a href="https://github.com/abahmed/kwatch/graphs/contributors">
  <img src="https://contributors-img.firebaseapp.com/image?repo=abahmed/kwatch" />
</a>

## ⭐️ Stargazers

<img src="https://api.star-history.com/svg?repos=abahmed/kwatch&type=Date" alt="Stargazers over time" style="max-width: 100%">

## 👋 Get in touch

Feel free to chat with us on [Discord](https://discord.gg/kzJszdKmJ7) if you have questions, or suggestions

## ⚠️ License

kwatch is licensed under [MIT License](LICENSE)
