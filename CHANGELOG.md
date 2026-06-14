# Changelog

## v0.11.0 (2026-06-15)

### Major changes

- **Edge-triggered notifications** — alerts fire only when an incident starts
  (`create`) or resolves (`resolved`); ongoing persistence is silent. No more
  periodic `update`/`stale` noise. Enable `correlation.renotify` for opt-in
  reminders.
- **Incident grain = `(namespace, owner, reason)`** — all affected pods and
  containers are members of one incident instead of fanning out per container.
  One alert per root cause.
- **Sensible defaults** — all monitors ON (pod, node, PVC, pending-pod,
  rollout, job, cronjob, daemonset, HPA), storm digest ON (floods collapse to
  a summary), inhibition ON (node failure suppresses pod fan-out), escalation
  ON (severity rises with restart count), `resolveHoldDown: 30s` (flap
  dampening), `maxRecentLogLines: 50`, `resyncSeconds: 600`. Health endpoint
  ON with liveness/readiness probes. `/incidents` and `/test-alert` gated
  behind `healthCheck.diagnostics` (default false).
- **Least-privilege RBAC** — `secrets` read gated behind `tlsMonitor.enabled`;
  `coordination.k8s.io/leases` gated behind `replicaCount>1`;
  `kwatchconfigs` gated behind `crd.enabled`.
- **Async notification delivery** — slow/hung providers no longer block
  detection or other providers. Per-provider buffered channels with
  non-blocking send.
- **Self-metrics `/metrics`** — Prometheus-format counters for incidents,
  notifications, active incidents, and baseline size. No external dependency.

### New features

- Per-pod startup baseline — suppresses exactly the pods broken at startup;
  new pods alert immediately; a healthy sibling never clears a broken sibling.
- Resolve hold-down with recurrence-correct re-create (`StatePendingResolve` +
  `correlation.resolveHoldDown`).
- `DisruptionFilter` — ignores terminating/evicted/scale-down pods
  (`ignoreDisruptionTerminations`, default true).
- `ContainerMessageFilter` — ignore by container status message substring
  (`ignoreContainerMessages`).
- `namespaceSelector` — watch namespaces by Kubernetes label selector.
- `includeEvents`/`includeLogs` — toggle events/logs sections per-provider.
- Env-var interpolation in config (`${VAR}`, braced-only; preserves literal `$`).
- Outbound TLS/CA/proxy — `insecureSkipTLSVerify`, `caBundlePath`, `proxyURL`
  on the shared HTTP client.
- Discord embed field-splitting — logs split into `(n/N)` fields, ≤1024 each,
  25-field cap.
- Compact Slack mode (`compact: true`) — single-line messages.
- Per-reason runbooks — `runbooks` config maps reason→URL appended to hint.
- `PeakResources` tracking — shows max concurrent affected pods in resolved
  messages.
- Centralized message truncation (`maxMessageLen: 4096`) to prevent provider
  limit failures.

### Bug fixes

- Node-condition stable key + per-condition resolve (no create/resolved flap).
- `MarkResolved` idempotency guard (no repeated `resolved`).
- Stale node inhibition — node condition resolve immediately clears the
  inhibition flag (BUG-I).
- ClearSeen dead code removed (BUG-II).
- k8s client proxy fix — `http.ProxyURL(nil)` → `url.Parse` (BUG-III).
- Slack `threadMap` memory leak — delete on ActionResolved (BUG-IV).
- Container log-leak (ContainerStatusUnknown skips log fetch).
- Config file not found → graceful defaults (no crash-loop).
- Teams card rendering — golden test confirms pod name + namespace + reason.

### Breaking changes

- Alert text formats changed (incident grouping, edge-triggering).
- No restart re-alert (per-pod baseline suppresses pre-existing breakage).
- `ignoreDisruptionTerminations` defaults **true** (terminating/evicted pods
  no longer alert).
- Only `${VAR}` (braced) interpolated in config; bare `$` preserved.
- `/incidents` and `/test-alert` now behind `healthCheck.diagnostics: true`.
- RBAC least-privilege — secrets/leases/kwatchconfigs are now gated; update
  your ClusterRole if you use TLS/leader-election/CRD features.
- `correlation.cooldown` and `correlation.staleThreshold` removed
  (edge-triggering replaces both). `renotify.interval` deprecated in favor of
  `renotify.intervalBySeverity["default"]`.

### Closed issues

Closes #64, #41, #335, #51, #194, #84, #124, #419, #410, #85, #65, #175, #126.
Refs #324 (node monitoring supported).
