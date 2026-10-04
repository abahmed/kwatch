# ⚙️ Configuration reference

Every kwatch configuration option, monitor, and endpoint in one place. For the 60-second
install and a quick overview, see the [README](../README.md).

Sensitive values must reference a mounted file with the exact form
`${file:/absolute/path}`. kwatch rejects plain provider credentials, heartbeat
URLs, and environment substitutions for those fields. The
interactive installer stores the files in a Kubernetes Secret.

> **The good news: you probably don't need this page.** Every option below has a safe
> default and works out of the box. Use this reference when you want to *change* something —
> fewer alerts, a different channel, a custom message — or when a term in an alert confuses
> you. After editing your `config.yaml`, run `kwatch lint` (add `--check` to also verify
> credentials for providers that support checks).
>
> A key kwatch does not recognise — a typo or an option removed in this release — does
> not stop startup. kwatch logs one warning per such key, naming the line in your file
> and the full path of the key (for example `line 4: app.clusterNmae`), so check the
> startup log after an upgrade. `kwatch lint --strict` rejects them instead.
> `activeProbeMonitor.recoveryThreshold` was removed (a probe resolves on its first
> success) and is now reported this way.
> Provider options are checked the same way against the provider catalog: an
> unknown `alert.<provider>` key logs a warning with its path and, for an
> obvious typo, a suggestion (`alert.slack.webhok ...; did you mean
> "webhook"?`). `kwatch lint` also warns, without failing, when no alert
> provider is configured.

## 🗺️ Find what you need

| You want to... | Read |
| --- | --- |
| Choose where alerts go | [Channels](https://kwatch.dev/docs/channels) |
| Watch only some namespaces | [Namespace filters](#-filter-by-namespace) |
| Stop a known, intentional alert | [Silences](#-silences--stop-the-noise) |
| Deliver an incident more or less loudly | [Severity](#-severity) |
| Add a fix link to an alert | [Runbooks](#-custom-message-templates) |
| Store credentials safely | [Secret-backed credentials](#-secret-backed-credentials-are-required) |

### 🧭 A safe way to change settings

1. Change one setting at a time.
2. Run `kwatch lint`.
3. Apply the configuration and watch the Pod rollout.
4. Send a test alert if you changed a provider.

The sections below are grouped by the problem you want to solve. You do not
need to read the full page from top to bottom.

## 🔧 General

Decide **what** to watch and **how often**. These are the knobs most people change first —
narrowing the watch list and turning off the noisy reasons.

| Parameter | What it does |
|:---|---|
| `resyncSeconds` | How often informers re-list everything (default: 300). It is a safety net for lost watch events, not the detection path; `0` turns it off |
| `namespaces` | 🔽 Deliver only incidents in these namespaces — or use `!kube-system` to deliver *everything except* it |
| `namespaceSelector` | 🏷️ Pick namespaces by K8s label selector (use *instead of* `namespaces`, not with it) |
| `reasons` | 🔽 Deliver only these reasons — or exclude some with `!` (e.g. `reasons: ["!Started"]`) |
| `runbooks` | 📚 Map a reason to a URL; it is added to the next steps of every matching incident |
| `severityByReason` / `severityByOwnerKind` | 🎯 Change how loudly an incident is delivered (see [Severity](#-severity)). Keys match case-insensitively |
| `maintenance.enabled` | ✅ Honor maintenance annotations (default: true) |
| `maintenance.annotation` | Annotation used to mark deliberate maintenance (default: `kwatch.io/maintenance`) |
| `maintenance.untilAnnotation` | Optional RFC3339 expiry annotation (default: `kwatch.io/maintenance-until`) |
| `ignore*` fields | 🔕 Deprecated filters (`ignoreContainerNames`, `ignorePodNames`, `ignoreContainerMessages`, `ignoreNodeReasons`, `ignoreNodeMessages`) — each becomes one silence rule; use `silences` below |

kwatch always watches every supported resource. `namespaces`, `namespaceSelector`,
`reasons` and `silences` only filter what is **delivered**: an incident is
delivered while at least one finding it explains is in scope and not silenced.
State lives on a small disk (a PVC), not in ConfigMaps.

#### 🔽 Filter by namespace

```yaml
# Watch only these namespaces
namespaces:
  - default
  - production

# Or exclude some (can't mix both).
# Quote every "!" entry: a bare !name is a YAML tag, not a string.
namespaces:
  - "!kube-system"
  - "!monitoring"
```

#### 🔽 Filter by reason

```yaml
# Only these reasons are delivered
reasons:
  - CrashLoopBackOff
  - ImagePullBackOff

# Or exclude some
reasons:
  - "!Started"
  - "!Killing"
```

#### 🔧 Maintenance mode

Annotate an object (or a namespace) when a deliberate change should not notify:

```yaml
maintenance:
  enabled: true
  annotation: kwatch.io/maintenance
  untilAnnotation: kwatch.io/maintenance-until
```

Use `kwatch.io/maintenance: "true"` (`true`, `1`, `yes` or `on`, case
insensitive; any other value such as `false`, `0`, `no` or `off` does not hold),
or set the until annotation to an RFC3339 time such as `2026-08-31T23:00:00Z`.
When both are set, the until time bounds the hold: after it passes the object
is no longer held. An incident is not delivered when all of its
findings are on annotated objects, or on objects in annotated namespaces. An
incident that also has a finding on an unannotated object is still delivered.
Invalid expiry values are ignored rather than suppressing incidents.

## 📱 App settings

Small but useful global options — mostly about **what alerts say** and how kwatch
talks to the outside world.

| Parameter | What it does |
|:---|---|
| `app.proxyURL` | 🔗 Proxy for outgoing HTTP requests |
| `app.clusterName` | 🏷️ Name shown in alerts so you know which cluster |
| `app.disableStartupMessage` | Silence the "kwatch is alive" welcome message |
| `app.logFormatter` | Log format: `text` (default) or `json`. `json` writes one JSON object per log line to stderr |
| `app.insecureSkipTLSVerify` | 🔓 Skip TLS verification on outbound HTTP (default: false) |
| `app.caBundlePath` | 📜 Path to a PEM CA bundle for outbound HTTP |

The startup message and the upgrade notice are each a single plain sentence
with one status emoji and no links, for example `🟡 kwatch v1.2.0 started.` and
`🟡 kwatch v1.3.0 is available; this cluster runs v1.2.0.`. A restart after an
internal failure, or after a gap in monitoring, uses 🟠 and says which period
went unwatched. When there are problems that were already there at startup,
one startup summary lists them, page-tier problems included; the paging
tools and issue trackers below receive each page-tier announcement on its
own instead, since they never receive summaries. Summaries and plain
notices are information,
not incidents: PagerDuty, Opsgenie, Squadcast, GoAlert, Zenduty, incident.io,
iLert and SIGNL4 (paging) and GitHub, GitLab, Gitea, Jira and ClickUp (issue
trackers) do not receive them, because nothing would ever close the alert or
issue they would open. Splunk On-Call, Alerta, Sensu Go, Datadog, New Relic,
SNS and Splunk HEC receive them as informational.

## 🔌 Kubelet and watched resources

| Parameter | What it does |
|:---|---|
| `kubelet.insecureSkipVerify` | 🔓 Skip kubelet serving certificate verification for direct node stats reads (default: false). Set it only when kubelet serving certificates are self-signed rather than signed by the cluster CA. Mounted configuration only; a KwatchConfig cannot set it |
| `watch.secrets` | 🔐 Watch Secrets (values are hashed, never stored; default: true). `false` stops the Secret watch, lists Secret as `disabled_by_config` in `/health` coverage and makes checks that need Secrets report that they cannot verify. The Helm value `watch.secrets` also removes Secret RBAC |

kwatch reads node stats directly from each kubelet over HTTPS (node
`InternalIP`, kubelet port, default 10250), so the kwatch Pod must reach
every node on that port. It needs `get` on `nodes/stats` and `nodes/metrics`;
`nodes/proxy` is not used or granted. If you use a NetworkPolicy, allow egress
to TCP 10250 on the nodes (the chart's opt-in policy does this through
`networkPolicy.kubeletPort` and `networkPolicy.kubeletCIDRs`).

## 💓 Health checks

Endpoints kwatch serves so you can see it's alive and grab Prometheus metrics. These five
are the only HTTP endpoints; there are no diagnostic, profiling, or test-alert endpoints.

| Parameter | What it does |
|:---|---|
| `healthCheck.enabled` | ✅ Health endpoints (default: true) |
| `healthCheck.port` | Port to serve health on (default: 8060) |

**Endpoints:**
- `GET /healthz` — ✅ Liveness
- `GET /readyz` — ✅ Readiness. Ready when the leader has restored its on-disk state,
  configured required sources, and synchronized required informer caches. Optional
  API absence remains degraded and does not fail readiness. An unrecoverable
  required startup or cache failure causes the active process to stop so
  Kubernetes can restart or replace it.
- `GET /availabilityz` — ✅ Deployment availability. A leader, or a Pod
  still waiting for the Lease, can pass the rolling-update probe.
- `GET /health` — JSON containing overall status, leadership, component states,
  and bounded degradation reasons.
- `GET /metrics` — 📊 Prometheus-format metrics. It does not require Prometheus to be
  installed. Every series has a production writer; labels are a fixed, bounded set.

| Metric | Type | Labels | Meaning |
|:---|:---|:---|:---|
| `kwatch_incidents_total` | counter | `action` = `announce`, `update`, `resolve` | Incident lifecycle decisions |
| `kwatch_incidents_open` | gauge | none | Incidents not yet resolved |
| `kwatch_investigations_total` | counter | `result` = `done`, `skipped`, `late`, `timeout` | Announcement investigations (log excerpts and similar evidence) by outcome |
| `kwatch_delivery_notifications_total` | counter | none | Notification attempts |
| `kwatch_delivery_dropped_total` | counter | none | Notifications no provider accepted: dead-lettered, failed with no fallback left, dropped by a full queue, or dropped before delivery started |
| `kwatch_delivery_retries_total` | counter | none | Delivery retry attempts |
| `kwatch_delivery_terminal_errors_total` | counter | none | Deliveries that failed with no fallback left |
| `kwatch_delivery_dead_letters_total` | counter | none | Dead-lettered deliveries |
| `kwatch_delivery_queue_saturated_total` | counter | none | Delivery queue saturation events |
| `kwatch_delivery_pending_dropped_total` | counter | none | Notifications dropped before delivery started (pending queue full, or shutdown before start) |
| `kwatch_delivery_budget_folded_total` | counter | none | Notifications folded into the overflow digest by the hourly budget |
| `kwatch_delivery_digest_skipped_total` | counter | none | Overflow digests not sent because the provider skips plain messages |
| `kwatch_delivery_queue_depth` | gauge | `provider` = configured provider name | Jobs waiting in that provider's delivery queue |
| `kwatch_delivery_outbox_depth` | gauge | none | Persisted delivery jobs not yet delivered or given up |
| `kwatch_delivery_deferred_total` | counter | none | Jobs left in the outbox at shutdown for the next session |
| `kwatch_tracker_untracked_total` | counter | none | Issue creates whose response did not name the new issue, so the issue cannot be closed on resolve |
| `kwatch_pipeline_decision_lag_seconds` | histogram | none | Time from an observation batch being submitted until its decisions are applied |
| `kwatch_delivery_outbox_dropped_total` | counter | none | Persisted delivery jobs dropped by the outbox size or age bound |
| `kwatch_delivery_outbox_write_failures_total` | counter | none | Failed writes of the persisted delivery outbox |
| `kwatch_audit_dropped_total` | counter | none | Audit entries dropped because the audit queue was full |
| `kwatch_heartbeat_failures_total` | counter | none | Heartbeat pings that failed or were rejected |
| `kwatch_kubelet_stats_failures_total` | counter | none | Kubelet stats summary reads that failed, one per node per round |
| `kwatch_storage_resets_total` | counter | `reason` = `schema_mismatch`, `unreadable` | State files deleted and recreated at open |
| `kwatch_storage_corrupt_records_total` | counter | none | Stored values skipped because they did not decode |
| `kwatch_storage_expired_total` | counter | none | Stored entries deleted by retention |
| `kwatch_storage_evicted_total` | counter | none | Stored entries deleted to meet the size cap |
| `kwatch_storage_write_failures_total` | counter | none | Pipeline storage batches with at least one failed write |
| `kwatch_informer_handler_panics_total` | counter | none | Recovered informer handler panics |
| `kwatch_optional_api_unavailable_total` | counter | none | Optional APIs missing at watcher setup |
| `kwatch_watcher_syncs_total`, `kwatch_watcher_sync_failures_total` | counter | none | Dynamic watcher cache syncs and failures |
| `kwatch_component_degradations_total` | counter | none | Optional component degradations |
| `kwatch_component_stalls_total`, `kwatch_component_unexpected_stops_total` | counter | none | Required component stalls and unexpected stops |
| `kwatch_shutdown_timeouts_total` | counter | none | Component shutdown timeouts |
| `kwatch_source_unavailable_total` | counter | none | Required monitor sources unavailable |
| `kwatch_leadership_acquisitions_total`, `kwatch_leadership_losses_total`, `kwatch_leader_takeovers_total` | counter | none | Leader election transitions |
| `kwatch_telemetry_attempts_total`, `kwatch_telemetry_successes_total`, `kwatch_telemetry_retries_total` | counter | none | Adoption telemetry |
| `kwatch_telemetry_failures_total` | counter | `reason` = `state_read`, `invalid_identity`, `network`, `http_status`, `state_write` | Adoption telemetry failures |
| `kwatch_rendered_details_omitted_total` | counter | none | Provider detail sections omitted by bounds |
| `kwatch_redacted_values_total` | counter | none | Sensitive values redacted before rendering |

The process also exposes the standard Go runtime and process collectors.

Informer caches trim Kubernetes `managedFields` metadata at ingestion time to
reduce memory on apply-heavy clusters: only the entry of the most recent
writer is kept (plus the most recent spec writer when that is a different
entry), without its field sets. kwatch uses it to name who made a change
(for example `argocd-controller` or `kubectl-client-side-apply`) and to link
an object to the manager that owns its spec. Labels, annotations, spec,
status, resource versions, and deletion metadata remain intact.

> **Know when alerts are being lost.** A notification that no provider accepted is
> counted in `kwatch_delivery_dropped_total`. Alert on the counter: it is the difference
> between "no incidents" and "no deliveries". Queued deliveries are also kept in a
> persisted outbox (at most 2048 jobs, none older than 24 hours); the two
> `kwatch_delivery_outbox_*` counters show when it drops a job or cannot write.

### 📬 Delivery behaviour

- **Retries.** A transient failure (network error, timeout, 5xx, rate
  limit) is retried with the provider's backoff for up to 24 hours through
  the persisted outbox, then dropped and counted. A permanent rejection is
  not retried.
- **Provider health.** Each provider reports a `provider-<name>` component
  on `/health` with `provider_unavailable`, `provider_rejected` or
  `provider_rate_limited`. The `kubelet-stats` and `config-overlay`
  components report their own reasons (see below).
- **Hourly budget.** Each provider announces at most
  `alert.<provider>.hourlyBudget` new conversations per hour (integer >= 0,
  default 60, `0` means unlimited). Announcements over the budget, and the
  later updates and resolve of a folded conversation, are summarized in one
  overflow digest instead of one message each. Updates and resolves of
  announced conversations are never counted. There is no global default;
  set it per provider.
- **Digest wording.** The digest says how many notifications were folded,
  counted per reason (the most common reasons first, the rest as
  "+N other kinds"), so a channel never receives a bare update for an
  incident it was not told about. Providers that skip plain messages skip
  the digest (`kwatch_delivery_digest_skipped_total`).
- **Opened and resolved.** An incident that opens and resolves before its
  announcement is sent is delivered as one combined "opened and resolved"
  message instead of two.
- **Broken provider config fails startup.** A provider that is configured
  but cannot be constructed makes kwatch refuse to start (and
  `kwatch lint` fail) instead of running with alerts going nowhere.
- **Roots.** Failures of an external endpoint several workloads depend on,
  and errors shared by several workloads, are reported as their own root
  cause, with the affected workloads folded into one incident.
- **Paging keys.** Paging providers (and Alerta) use a stable
  deduplication key made of the root and the mode (`kwatch-<cluster>-`
  followed by root and mode), so the same problem maps to the same alert
  after a state store reset and is resolved rather than duplicated.

### 🩺 Health reasons

`/health` reports each component with a state and a reason from a fixed
vocabulary; raw errors stay in the logs. A reason that is not in this list is
reported as `component_failed`.

| Reason | Meaning |
|:---|---|
| `component_failed` | A component failed for a reason that has no more specific code |
| `component_stalled` | A required component stopped reporting progress |
| `component_stopped` | A component stopped unexpectedly |
| `timeout`, `canceled` | A component call hit its deadline, or was canceled |
| `cache_sync_pending`, `cache_sync_timeout`, `cache_sync_failed` | An informer cache is still syncing, did not sync in time, or failed to sync |
| `source_not_configured` | A source was not configured before it was needed |
| `permission_denied` | A required permission is missing |
| `optional_permission_denied` | An optional permission is missing; readiness is not affected |
| `api_unavailable` | A required API is not available |
| `optional_api_unavailable` | An optional API (for example Gateway API or the `KwatchConfig` CRD) is not installed |
| `watcher_failed` | A dynamic watcher failed |
| `persistence_restore_failed` | Saved state could not be read back, for example the delivery outbox |
| `storage_reset` | The state file had another schema version or was unreadable and was deleted and recreated |
| `storage_over_cap` | Data that is never evicted keeps the state store over its size cap |
| `provider_unavailable` | The `provider-<name>` component could not reach its provider (transient failures; the job stays queued) |
| `provider_rejected` | The `provider-<name>` component's provider refused a message permanently, for example a revoked token |
| `provider_rate_limited` | The `provider-<name>` component's provider is rate limiting kwatch |
| `heartbeat_failed` | A heartbeat ping failed or was rejected |
| `kubelet_unreachable`, `kubelet_partially_unreachable` | The `kubelet-stats` component could reach no node's kubelet, or only some of them, in the last stats round; only usage attributes are affected |
| `config_overlay_invalid` | The `KwatchConfig` overlay is invalid; kwatch runs on the mounted configuration (`config-overlay` component), or the watcher rejected an edit without restarting (`crd-watcher` component) |
| `leadership_lost`, `shutdown` | Why leadership ended |

The vocabulary also allows `source_configuration_failed`, `discovery_failed`,
`persistence_write_failed`, `provider_shutdown_timeout` and `rate_limited`.

The `coverage` object in `/health` lists resource kinds that are not watched,
each with one of `permission_denied`, `api_unavailable`, `sync_timeout`,
`budget_exceeded`, `object_cap_reached` or `disabled_by_config` (for
example Secrets with `watch.secrets: false`). Its `reasons` field counts
kinds per reason, so the counts stay complete when the kind list is cut at
50.

## 🔐 Kubernetes permissions and graceful degradation

kwatch never needs write access to monitored workloads. The chart value
`rbac.mode` selects how the read-only ClusterRole is built:

| Mode | What it grants |
|:---|---|
| `full` (default, and the raw `deploy/deploy.yaml`) | `list` and `watch` on every resource in every API group, so any built-in kind and any CRD, including ones installed later, can be watched |
| `least-privilege` | An explicit `list`/`watch` set for the kinds kwatch understands. Kinds outside it are reported as `permission_denied` in `/health` coverage, detectors that need them stay silent, and custom resources are not covered |

In both modes:

- `get` is granted only where kwatch reads a single object: `namespaces`,
  `nodes`, `nodes/stats`, `nodes/metrics` and `pods/log`. There is no wildcard
  `get`, which would also match subresources such as `pods/exec`.
- `get` on `pods` is granted only in kwatch's own namespace, to explain why
  the previous kwatch Pod restarted.
- `get` and `update` on Leases are limited by name to kwatch's own Lease;
  `create` is granted on Leases in kwatch's namespace because RBAC cannot
  limit it by name.
- `create` on `selfsubjectaccessreviews` lets kwatch audit its own
  permissions.
- Secrets are covered by the grant but their values are hashed in the
  informer transform and never stored.

Set `watch.secrets: false` (chart value and configuration field) to run
without any Secret permission. kwatch then starts no Secret watch, lists
`secrets` as `disabled_by_config` in the `/health` coverage without degrading
health, and checks that need Secrets (missing Secret references, certificate
expiry) report that they cannot verify. Because RBAC cannot remove one
resource from a wildcard, the chart uses the explicit least-privilege list in
that case, even in `full` mode.

Optional sources are reported as `optional_api_unavailable` or
`optional_permission_denied` when an API is not served or a permission is
missing; kwatch continues with the remaining
sources. Metrics API evidence, Prometheus, a service mesh, and a
cloud-provider API are not required for core Kubernetes object monitoring.
See [Production operations](production-operations.md#rbac-mode-and-watch-coverage)
for the full list.

The interactive `kwatch.sh` manager downloads the matching
`deploy/feature-catalog.tsv` from the installed release and caches it in a
separate ConfigMap. Run `kwatch.sh features` (or choose **Show capabilities**)
to see the available IDs, dependencies, and plain-language descriptions;
the catalog is informational and does not change runtime behavior.

Guided notification setup uses `deploy/provider-catalog.tsv`. It covers every
supported provider and documented provider field, and marks every credential
field as Secret-backed. It can also describe provider-specific choice groups,
conditional fields, and at-least-one destination groups (for example, one
authentication method requiring a channel). The installer interprets those
relationships generically, so adding a new provider does not require
provider-specific shell logic. Provider credentials never appear in the
general configuration catalog.

Missing optional endpoints and permissions are visible on `/health` as
degraded components or in its `coverage` object, without becoming fabricated
incidents.

## 🔄 Upgrader

At startup kwatch quietly asks "is there a newer version?" and mentions it. Turn it off if
you manage updates yourself (e.g. you pin images).

| Parameter | What it does |
|:---|---|
| `upgrader.disableUpdateCheck` | 🔕 Don't check for new kwatch versions |

## 📡 Minimal adoption telemetry

Telemetry is **on by default** in official builds. kwatch sends a small
pseudonymous heartbeat so the project can estimate adoption.

**What is sent.** One HTTPS `POST` with a JSON body of exactly two fields:

```json
{"cluster_uuid": "<installation ID>", "kwatch_version": "<version>"}
```

The installation ID is a UUID derived once from a salted SHA-256 hash of the
`kube-system` namespace UID, so the UID itself never leaves the cluster; if
that namespace cannot be read, kwatch uses a random UUID instead. It is kept in
kwatch's state file. The field is named `cluster_uuid` for wire
compatibility. No cluster name, namespace, resource name, node count,
configuration, provider setting, log line or incident is sent. As with any
HTTPS request, the receiving service sees the connection's source address and
the `User-Agent` `kwatch-telemetry/1`.

**Where and how often.** To `https://api.kwatch.dev/v1/telemetry/heartbeat`,
from the active replica, using kwatch's configured outbound HTTP client
(`app.proxyURL`, `app.caBundlePath`). The first heartbeat is sent shortly after
startup, then at most once every 7 days (the last successful send is stored in
the state file). A failed attempt is retried after 1, 5, 15, 30 and then 60
minutes. Each attempt times out after 3 seconds, and a failure never affects
monitoring.

| Parameter | What it does |
|:---|---|
| `telemetry.enabled` | ✅ Send adoption heartbeats (default: `true`) |

**How to disable it.** Set `telemetry.enabled: false` in the configuration, or
set the environment variable `KWATCH_TELEMETRY` to `false`, `0`, `off` or `no`.
Development builds, recognized CI environments (`CI` is set) and installs
without a state store do not send telemetry. At startup kwatch logs one line
naming the endpoint and either that it is enabled or why it is skipped
(`disabled`, `disabled_env`, `dev_build`, `ci_environment`). Outcomes are
counted by `kwatch_telemetry_attempts_total`, `_successes_total`,
`_retries_total` and `kwatch_telemetry_failures_total{reason}`.

---

## 📊 Monitors

kwatch always watches every supported resource; there are no per-resource monitor
switches. Two optional checks have their own settings: active probes and the
heartbeat.

### 🌐 Active Probes

Active probes are opt-in and target only endpoints explicitly listed by the
operator by default. Set `autoServices: true` to probe every advertised
Service port from inside the kwatch Pod (TCP for all ports, plus HTTP for
ports whose name starts with `http`). A target must fail
`failureThreshold` consecutive checks before alerting; it resolves on the
first successful check.

Explicit `http`, `tcp`, and `dns` targets are the recommended low-noise mode.
`autoServices` is opt-in and probes every advertised Service port; it uses
paginated Kubernetes API lists so large clusters are not fetched as one large
response.

| Parameter | What it does |
|:---|---|
| `activeProbeMonitor.enabled` | ✅ Run configured application probes (default: false) |
| `activeProbeMonitor.intervalSeconds` | ⏱️ Seconds between probe rounds (default: 30) |
| `activeProbeMonitor.timeoutSeconds` | ⏱️ Timeout for each probe (default: 5) |
| `activeProbeMonitor.failureThreshold` | 🔁 Consecutive failures before alerting (default: 3) |
| `activeProbeMonitor.autoServices` | 🔗 Probe discoverable Service ports automatically (default: false) |
| `activeProbeMonitor.excludeNamespaces` | 🚫 Namespaces `autoServices` skips entirely |
| `activeProbeMonitor.http` | 🌐 Explicit HTTP targets with optional status and latency limits |
| `activeProbeMonitor.tcp` | 🔌 Explicit TCP targets |
| `activeProbeMonitor.dns` | 🔎 Explicit DNS targets |

> ⚠️ **`autoServices` and NetworkPolicy.** Auto-probing opens a real TCP
> connection to every Service port in scope, from kwatch's own pod. In a
> namespace with default-deny ingress that does not admit kwatch, every
> Service there is reported as `ActiveProbeFailure` while being perfectly
> healthy. Two escape hatches: list the namespace in
> `activeProbeMonitor.excludeNamespaces`, or annotate the individual Service
> with `kwatch.io/skip-probe: "true"`. Explicitly configured `http`, `tcp` and
> `dns` targets are never filtered by either — you asked for those by name.

```yaml
activeProbeMonitor:
  enabled: true
  intervalSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3
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

### 💓 Heartbeat Monitor (dead man's switch)

| Parameter | What it does |
|:---|---|
| `heartbeatMonitor.enabled` | Send pings to a health-check URL (default: false) |
| `heartbeatMonitor.interval` | ⏱️ Seconds between pings (default: 300) |
| `heartbeatMonitor.url` | 🔗 Secret-backed `${file:/absolute/path}` heartbeat URL |

If kwatch stops or crashes, the external monitor stops getting pings and pages you. 🔔

### 🎯 Severity

Every incident carries a severity, shown as the colour of its headline, and you control it.
Severity decides how loudly it is delivered. The values `severityByReason` and
`severityByOwnerKind` accept are `critical`, `high`, `medium`, `warning` and
`normal`, compared case-insensitively; any other value fails startup.

| Severity | Delivery | Headline |
|:--|:--|:--|
| `critical` | Pages | 🔴 |
| `high` | Notifies | 🟠 |
| `medium` / `warning` | Notifies | 🟠 |
| `normal` | Goes to the digest: one 🟡 message every 30 minutes lists the low-priority problems that opened or resolved | 🟡 |

Without an override, kwatch derives the tier itself: an incident with a critical finding
pages only when it also matches a page rule (for example a lost node, an unavailable
API server or cluster DNS, or an Ingress that lost its backends); otherwise it notifies.
An incident made only of informational or digest-only findings goes to the
digest: it is not announced on its own, one digest message every 30 minutes
lists what opened and what resolved, and it is announced at once if it gets
worse. Paging tools and issue trackers never receive digests.

Every message starts with exactly one status emoji and no other: 🔴 page,
🟠 notify, 🟡 low, ✅ resolved. Red is reserved for incidents that page.

| Parameter | What it does |
|:---|---|
| `severityByOwnerKind` | Set severity per resource type, e.g. `StatefulSet: "high"` |
| `severityByReason` | Set severity per reason, checked first, before owner kind, e.g. `OOMKilled: "high"` |

Keys match case-insensitively (`DaemonSet`, `daemonset` and `DAEMONSET` are the same
key). Defaults come from the built-in rules for each reason. An override replaces the
built-in tier of an incident; when several of an incident's findings match, the loudest
tier wins.

### 🔇 Silences — stop the noise

In plain words: if a rule matches a finding, that finding is ignored. Build rules from anything on the finding: namespace, reason, pod
name pattern, container name, container message, Event message, or node.

```yaml
silences:
  - namespaces: ["kube-system", "monitoring"]
  - reasons: ["BackOff"]
  - podNamePatterns: ["my-fancy-pod-.*"]
```

Each rule can also filter by `containerNames`, `containerMessages`,
`eventMessages`, `nodeReasons`, and `nodeMessages` (message substrings). A
rule matches a finding only when every field it sets matches. An incident is
delivered while at least one finding it explains is in scope and not silenced,
so silencing a symptom never hides a root cause that affects other workloads.
Message matchers search the finding summary and its evidence, which includes
the Event message for Event-backed findings. The deprecated top-level `ignore*`
fields map onto these rules. A rule must set at least one field: an empty rule
(`- {}`) would silence everything, so kwatch rejects it at startup.

For example, suppress a noisy transient cache-sync error while keeping other
`CreateContainerConfigError` incidents visible:

```yaml
silences:
  - eventMessages:
      - "failed to sync configmap cache"
```

The match is a case-sensitive substring of an attached Event message. It
silences the finding backed by that Event.

### 📝 Custom message templates

In plain words: if the default alert text isn't yours, write your own. `templates` maps a
reason to a Go `text/template` used by plain-text providers. The template receives
`.Message` and `.Text`; rich providers ignore it.

```yaml
templates:
  CrashLoopBackOff: "{{ .Text }} (see the runbook)"
```

### 📚 Runbooks

`runbooks` maps a reason to a URL that is added to the next steps of every incident with that reason.

```yaml
runbooks:
  OOMKilled: https://wiki.example.com/runbooks/oom
```

### 🔐 Secret-backed credentials are required

The base config is often a ConfigMap. ConfigMaps are readable by anyone with `get` access to
the namespace and are not encrypted at rest by default, so a provider credential written there
is a credential anyone in the namespace can take.

Put `config.yaml` and each credential in the same Kubernetes Secret. For
example, `config.yaml` contains only the reference:

```yaml
alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
```

Create the Secret without putting the credential on a command line or in a
manifest:

```bash
kubectl -n kwatch create secret generic kwatch-config \
  --from-file=config.yaml \
  --from-file=slack-webhook
```

Mount that Secret at `/config` and set `CONFIG_FILE=/config/config.yaml`.
When `CONFIG_FILE` is set but the file does not exist (for example the Secret
has no `config.yaml` key), kwatch refuses to start and logs the path, instead
of running with defaults and no providers. Without `CONFIG_FILE` kwatch uses
the built-in defaults.
`${VAR}` remains available for non-sensitive settings only. Sensitive settings
accept absolute `${file:/path}` references exclusively.

If a token has ever been committed to a ConfigMap, rotate it — it should be treated as
disclosed, not merely moved.

### 🛡️ Kubernetes operational hardening

The shipped manifests and chart run kwatch as a non-root user with a read-only
root filesystem, disabled privilege escalation, all Linux capabilities dropped,
and the `RuntimeDefault` seccomp profile. The namespace manifest also requests
the Kubernetes `restricted` Pod Security profile. State is written to a PVC mounted at `/var/lib/kwatch`; kwatch runs as a single
replica and its Lease is only a lock. The ClusterRole remains read-only.

Keep the Secret protected with least-privilege RBAC, enable encryption at rest
for Secrets in the API server/etcd, and rotate provider credentials if access
is suspected. Rotating an external Secret should be followed by a deployment
rollout restart so the mounted files are refreshed.

The last two controls are cluster-operator responsibilities, not application
settings. For a self-managed control plane, configure an
`EncryptionConfiguration` with a newly generated key on the API server, enable
the API server's encryption-provider flag, restart the API server safely, and
verify that Secret reads still work. For a managed Kubernetes service, use its
provider-specific "encryption at rest" setting. Never commit the encryption
config or key to this repository. After installing kwatch, verify the namespace
labels and workload posture with your cluster's policy tooling, then rotate
provider credentials after any suspected exposure.

### 📝 Audit log

In plain words: for every incident decision (announced, updated, resolved, skipped),
kwatch writes one structured JSON line — feed it to your log pipeline if you want a
searchable history of everything it decided. `kwatch-scorecard` reads this log.

| Parameter | What it does |
|:---|---|
| `auditLog.enabled` | Write one structured JSON entry per incident decision (default: true) |
| `auditLog.output` | Destination: `stdout` (default) or a file path |

Every decision is recorded when it is made, including the ones people hear
through another message: `delivery` is `digest` or `startup summary` when the
decision was held for that message, `paging` when only paging tools and issue
trackers receive it, and absent for a message of its own.

File output is append-only. Configure rotation and retention in the container
runtime or log collector; kwatch does not rename or delete audit files.

### 📋 CRD — configuration overlay with automatic restart

In plain words: instead of editing the base config and restarting, you can store
non-sensitive configuration in a small custom resource. Provider settings
and heartbeat URLs are forbidden in `KwatchConfig`; they
remain in the mounted Secret. The overlay is applied at startup. When the `KwatchConfig` changes,
the watcher restarts kwatch so the complete configuration is rebuilt consistently. It is off in
the generic binary defaults when no CRD is installed. The Helm chart installs the CRD and enables
it by default; the interactive installer does the same. Manual deployments must install the CRD
before enabling it.

The chart passes `config.crd.enabled` to the container as `KWATCH_CRD_ENABLED`
(`true` or `false`), which overrides `crd.enabled` in the mounted file, so the
setting also holds when you bring your own Secret with `configSecretName`.

An invalid overlay never crash-loops kwatch. The watcher checks an edited
`KwatchConfig` against the mounted configuration before it restarts: an
invalid edit is rejected, logged with reason `config_overlay_invalid`, and
kwatch keeps running on its current configuration until the next edit. If the
overlay is already invalid at startup, kwatch logs the error, runs on the
mounted configuration alone, and `/health` shows the degraded
`config-overlay` component with reason `config_overlay_invalid`; readiness is
not affected. An overlay cannot turn Secret watching back on when
`KWATCH_WATCH_SECRETS=false`.

| Parameter | What it does |
|:---|---|
| `crd.enabled` | Watch `KwatchConfig` CRs and restart kwatch when the overlay changes (default: false for the binary; true in Helm and the interactive installer) |

```yaml
apiVersion: kwatch.abahmed.dev/v1alpha1
kind: KwatchConfig
metadata:
  name: kwatch-config
  namespace: kwatch
spec:
  resyncSeconds: 600
  silences:
    - namespaces: ["kube-system"]
```
