# ⚙️ Configuration reference

Every kwatch configuration option, monitor, and endpoint in one place. For the 60-second
install and a quick overview, see the [README](../README.md).

Sensitive values must reference a mounted file with the exact form
`${file:/absolute/path}`. kwatch rejects plain provider credentials, heartbeat
URLs, diagnostic tokens, and environment substitutions for those fields. The
interactive installer stores the files in a Kubernetes Secret.

> **The good news: you probably don't need this page.** Every option below has a safe
> default and works out of the box. Use this reference when you want to *change* something —
> fewer alerts, a different channel, a custom message — or when a term in an alert confuses
> you. After editing your `config.yaml`, run `kwatch lint` (add `--check` to also verify
> credentials for providers that support checks).

## 🗺️ Find what you need

| You want to... | Read |
| --- | --- |
| Choose where alerts go | [Channels](https://kwatch.dev/docs/channels) |
| Watch only some namespaces | [Namespace filters](#-filter-by-namespace) |
| Stop a known, intentional alert | [Silences](#-silences--stop-the-noise) |
| Deliver a problem more or less loudly | [Severity](#-severity) |
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
| `namespaces` | 🔽 Deliver only problems in these namespaces — or use `!kube-system` to deliver *everything except* it |
| `namespaceSelector` | 🏷️ Pick namespaces by K8s label selector (use *instead of* `namespaces`, not with it) |
| `reasons` | 🔽 Deliver only these reasons — or exclude some with `!` (e.g. `reasons: ["!Started"]`) |
| `runbooks` | 📚 Map a reason to a URL; it is added to the next steps of every matching problem |
| `severityByReason` / `severityByOwnerKind` | 🎯 Change how loudly a problem is delivered (see [Severity](#-severity)) |
| `maintenance.enabled` | ✅ Honor maintenance annotations (default: true) |
| `maintenance.annotation` | Annotation used to mark deliberate maintenance (default: `kwatch.io/maintenance`) |
| `maintenance.untilAnnotation` | Optional RFC3339 expiry annotation (default: `kwatch.io/maintenance-until`) |
| `ignore*` fields | 🔕 Deprecated filters (`ignoreContainerNames`, `ignorePodNames`, `ignoreContainerMessages`, `ignoreNodeReasons`, `ignoreNodeMessages`) — each becomes one silence rule; use `silences` below |

kwatch always watches every supported resource. `namespaces`, `namespaceSelector`,
`reasons` and `silences` only filter what is **delivered**: a problem is
delivered while at least one signal it explains is in scope and not silenced.
State lives on a small disk (a PVC), not in ConfigMaps.

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
# Only these reasons are delivered
reasons:
  - CrashLoopBackOff
  - ImagePullBackOff

# Or exclude some
reasons:
  - !Started
  - !Killing
```

#### 🔧 Maintenance mode

Annotate an object (or a namespace) when a deliberate change should not notify:

```yaml
maintenance:
  enabled: true
  annotation: kwatch.io/maintenance
  untilAnnotation: kwatch.io/maintenance-until
```

Use `kwatch.io/maintenance: "true"`, or set the until annotation to an RFC3339
time such as `2026-08-31T23:00:00Z`. A problem is not delivered when all of its
signals are on annotated objects, or on objects in annotated namespaces. A
problem that also has a signal on an unannotated object is still delivered.
Invalid expiry values are ignored rather than suppressing problems.

## 📱 App settings

Small but useful global options — mostly about **what alerts say** and how kwatch
talks to the outside world.

| Parameter | What it does |
|:---|---|
| `app.proxyURL` | 🔗 Proxy for outgoing HTTP requests |
| `app.clusterName` | 🏷️ Name shown in alerts so you know which cluster |
| `app.disableStartupMessage` | Silence the "kwatch is alive" welcome message |
| `app.logFormatter` | Log format: `text` (default) or `json` |
| `app.insecureSkipTLSVerify` | 🔓 Skip TLS verification on outbound HTTP (default: false) |
| `app.caBundlePath` | 📜 Path to a PEM CA bundle for outbound HTTP |

## 💓 Health checks

Endpoints kwatch serves so you can see it's alive, grab Prometheus metrics, and poke it for
tests.

| Parameter | What it does |
|:---|---|
| `healthCheck.enabled` | ✅ Health endpoints (default: true) |
| `healthCheck.port` | Port to serve health on (default: 8060) |
| `healthCheck.pprof` | 🔬 Go profiling endpoints (default: false) |
| `healthCheck.diagnostics` | 🩺 Extra endpoints: `/incidents`, `/test-alert`, `/deadletters` |
| `healthCheck.diagnosticsToken` | 🔑 Bearer token for diagnostics and pprof; use `${file:/absolute/path}` |

**Endpoints:**
- `GET /healthz` — ✅ Liveness
- `GET /readyz` — ✅ Readiness. Ready when the leader has restored its on-disk state,
  configured required sources, and synchronized required informer caches. Optional
  API absence remains degraded and does not fail readiness. An unrecoverable
  required startup or cache failure causes the active process to stop so
  Kubernetes can restart or replace it.
- `GET /availabilityz` — ✅ Deployment availability. A leader or standby
  participating in Lease election can pass the rolling-update probe.
- `GET /health` — JSON containing overall status, leadership, component states,
  and bounded degradation reasons.
- `GET /metrics` — 📊 Prometheus-format metrics (problems, notifications, queues, and informer activity). It does not require Prometheus to be installed.

Informer caches discard Kubernetes `managedFields` metadata at ingestion time
to reduce memory on apply-heavy clusters. Labels, annotations, spec, status,
resource versions, and deletion metadata remain intact for detection and graph
analysis.
- `GET /incidents` — 📋 All active problems (requires diagnostics and its token)
- `POST /test-alert` — 📤 Send a test alert (requires diagnostics and its token)
- `GET /deadletters` — 💀 Recent delivery failures (requires diagnostics and its token)

> **Know when alerts are being lost.** A notification a provider rejects is dead-lettered,
> counted in `kwatch_notifications_dropped_total`, and — with `diagnostics: true` — listed
> at `/deadletters`. Diagnostics are off by default because `/test-alert` accepts
> unauthenticated POSTs in test-only servers; production validation requires
> `diagnosticsToken` whenever diagnostics or pprof is enabled. Either way, alert on
> the counter: it is the difference between "no problems" and "no deliveries".

## 🔐 Kubernetes permissions and graceful degradation

The bundled manifests contain a read-only ClusterRole, derived from the access
declared by each source kwatch watches. kwatch never needs write access to monitored
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

Watched resources need only `get`, `list`, and `watch`. Optional sources expose
`unavailable` or `rbacDenied` when an API is not served or a permission is
missing; the controller continues with the remaining sources. Metrics API
evidence, Prometheus, a service mesh, and a cloud-provider API are not required
for core Kubernetes object monitoring.

Because kwatch always watches every supported resource, the ClusterRole covers
all of them. Installations requiring least privilege can remove rules for
resources they do not care about from their copied manifest; those sources
then report `rbacDenied` and are skipped.

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

The `/kubelet` health status reports `healthy`, `partial`, `unavailable`, or
`rbacDenied`, alongside Summary, cAdvisor, runtime, and node counts. The
`/controlplane`, `/security`, and `/informer` endpoints expose the same state
vocabulary and detailed last-error fields. `partial` means some built-in
capabilities are working; `rbacDenied` identifies an authorization gap rather
than a Kubernetes failure. Missing optional endpoints are visible without
becoming fabricated problems.

## 🔄 Upgrader

At startup kwatch quietly asks "is there a newer version?" and mentions it. Turn it off if
you manage updates yourself (e.g. you pin images).

| Parameter | What it does |
|:---|---|
| `upgrader.disableUpdateCheck` | 🔕 Don't check for new kwatch versions |

## 📡 Minimal adoption telemetry

Official builds send a small pseudonymous heartbeat so the project can estimate
adoption. The payload contains only a stable, randomly generated installation
ID (kept in kwatch's on-disk state) and the kwatch version. The API field is named
`cluster_uuid` for wire compatibility; it is not the Kubernetes cluster UID.
It is sent after startup and at most once per week to
`https://api.kwatch.dev/v1/telemetry/heartbeat`.

| Parameter | What it does |
|:---|:---|
| `telemetry.enabled` | ✅ Send adoption heartbeats (default: `true`) |

kwatch logs one line at startup naming the endpoint and effective setting, so
the heartbeat is visible from the logs without exposing the payload. Disable
it with `telemetry.enabled: false`. Development builds and
recognized CI environments do not send telemetry. The service should use this
data only for aggregate adoption counts and version planning; no feature-usage
or cluster inventory is collected. Telemetry failures never affect monitoring
startup or runtime.

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
`failureThreshold` consecutive checks before alerting and pass
`recoveryThreshold` consecutive checks before resolving.

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
| `activeProbeMonitor.recoveryThreshold` | ✅ Consecutive successes before resolving (default: 2) |
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

### 💓 Heartbeat Monitor (dead man's switch)

| Parameter | What it does |
|:---|---|
| `heartbeatMonitor.enabled` | Send pings to a health-check URL (default: false) |
| `heartbeatMonitor.interval` | ⏱️ Seconds between pings (default: 300) |
| `heartbeatMonitor.url` | 🔗 Secret-backed `${file:/absolute/path}` heartbeat URL |

If kwatch stops or crashes, the external monitor stops getting pings and pages you. 🔔

### 🎯 Severity

Every problem carries a severity, shown as the colour of its headline, and you control it.
Severity decides how loudly it is delivered: `critical` pages; `high`, `medium` and
`warning` notify; `low` and `info` go to the digest.

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
| `severityByReason` | Set severity per reason, checked first, before owner kind, e.g. `OOMKilled: "high"` |

Keys are used verbatim (`DaemonSet`, not `Daemonset`). Defaults come from the built-in rules for each reason.

### 🔇 Silences — stop the noise

In plain words: if a rule matches a signal, that signal is ignored. Build rules from anything on the signal: namespace, reason, pod
name pattern, container name, container message, Event message, or node.

```yaml
silences:
  - namespaces: ["kube-system", "monitoring"]
  - reasons: ["BackOff"]
  - podNamePatterns: ["my-fancy-pod-.*"]
```

Each rule can also filter by `containerNames`, `containerMessages`,
`eventMessages`, `nodeReasons`, and `nodeMessages` (message substrings). A
rule matches a signal only when every field it sets matches. A problem is
delivered while at least one signal it explains is in scope and not silenced,
so silencing a symptom never hides a root cause that affects other workloads.
Message matchers search the signal summary and its evidence, which includes
the Event message for Event-backed signals. The deprecated top-level `ignore*`
fields map onto these rules.

For example, suppress a noisy transient cache-sync error while keeping other
`CreateContainerConfigError` problems visible:

```yaml
silences:
  - eventMessages:
      - "failed to sync configmap cache"
```

The match is a case-sensitive substring of an attached Event message. It
silences the signal backed by that Event.

### 📝 Custom message templates

In plain words: if the default alert text isn't yours, write your own. `templates` maps a
reason to a Go `text/template` used by plain-text providers. The template receives
`.Message` and `.Text`; rich providers ignore it.

```yaml
templates:
  CrashLoopBackOff: "{{ .Text }} (see the runbook)"
```

### 📚 Runbooks

`runbooks` maps a reason to a URL that is added to the next steps of every problem with that reason.

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

In plain words: for every problem decision (announced, updated, resolved, skipped),
kwatch writes one structured JSON line — feed it to your log pipeline if you want a
searchable history of everything it decided. `kwatch-scorecard` reads this log.

| Parameter | What it does |
|:---|---|
| `auditLog.enabled` | Write one structured JSON entry per problem decision (default: true) |
| `auditLog.output` | Destination: `stdout` (default) or a file path |

File output is append-only. Configure rotation and retention in the container
runtime or log collector; kwatch does not rename or delete audit files.

### 📋 CRD — configuration overlay with automatic restart

In plain words: instead of editing the base config and restarting, you can store
non-sensitive configuration in a small custom resource. Provider settings,
heartbeat URLs, and diagnostic tokens are forbidden in `KwatchConfig`; they
remain in the mounted Secret. The overlay is applied at startup. When the `KwatchConfig` changes,
the watcher restarts kwatch so the complete configuration is rebuilt consistently. It is off in
the generic binary defaults when no CRD is installed. The Helm chart installs the CRD and enables
it by default; the interactive installer does the same. Manual deployments must install the CRD
before enabling it.

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
