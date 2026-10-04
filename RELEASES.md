# Releasing kwatch

## Unreleased

### Highlights

- **A new core.** kwatch now builds an understanding of the whole cluster
  (workloads, nodes, volumes, networking, configuration) and keeps it current.
  A root-cause engine ranks the likely cause of each incident, and symptoms of
  the same cause are folded into one incident.
- **Narrative messages.** Every notification is one readable message: what is
  wrong, where, the likely cause, the impact and what to try next. The cluster
  name is included in every message.
- **Smaller operational surface.** One state file, a short list of endpoints
  and a smaller metric set.

### Breaking changes

- **Endpoints.** kwatch serves only `/healthz`, `/readyz`, `/availabilityz`,
  `/health` and `/metrics`. The diagnostics endpoints are gone. The
  incident list was first renamed from `/problems` to `/incidents` and then
  removed with the rest.
- **Metrics.** Removed: `kwatch_apiserver_latency_milliseconds`,
  `kwatch_apiserver_probe_errors_total`, `kwatch_baseline_size`,
  `kwatch_controlplane_probe_errors_total`, `kwatch_graph_edges`,
  `kwatch_graph_nodes`, `kwatch_graph_rebuild_latency_milliseconds`,
  `kwatch_graph_rebuilds_total`, `kwatch_group_size`,
  `kwatch_grouped_children_total`, `kwatch_informer_events_total`,
  `kwatch_informer_watch_errors_total`, `kwatch_insight_analyses_total`,
  `kwatch_insight_causes_total`, `kwatch_insight_reevaluations_total`,
  `kwatch_insight_rollout_suppressions_total`,
  `kwatch_lifecycle_duplicate_transitions_total`,
  `kwatch_notifications_dropped_total`, `kwatch_persistence_compactions_total`,
  `kwatch_persistence_last_success_timestamp_seconds`,
  `kwatch_persistence_migration_errors_total`,
  `kwatch_persistence_migrations_total`, `kwatch_persistence_omitted_total`,
  `kwatch_persistence_payload_bytes`, `kwatch_persistence_retries_total`,
  `kwatch_processing_latency_milliseconds`, `kwatch_queue_depth`,
  `kwatch_root_cause_suppressions_total` and
  `kwatch_startup_summaries_suppressed_total`.
  Renamed: `kwatch_incidents_active` is now `kwatch_incidents_open`, and
  `kwatch_notifications_total` is now `kwatch_delivery_notifications_total`.
  New: `kwatch_delivery_dropped_total` (notifications no provider accepted),
  `kwatch_delivery_pending_dropped_total`,
  `kwatch_delivery_budget_folded_total` (notifications folded into the
  overflow digest),
  `kwatch_delivery_digest_skipped_total`,
  `kwatch_delivery_outbox_dropped_total`,
  `kwatch_delivery_outbox_write_failures_total`,
  `kwatch_heartbeat_failures_total`,
  `kwatch_investigations_total{result}`, `kwatch_audit_dropped_total` and
  `kwatch_storage_corrupt_records_total`, `_evicted_total`, `_expired_total`,
  `_resets_total{reason}` and `_write_failures_total`. Update dashboards and
  alert rules.
- **Feature IDs.** The `problems.*` capability IDs are now `incidents.*`
  (`incidents.noise-control`, `incidents.flapping`,
  `incidents.startup-summary`, `incidents.state`,
  `incidents.downtime-changes`). Update anything that reads the feature
  catalog.
- **Provider options.** These options no longer change anything, because
  incident messages are written by kwatch: `title` and `text` for Slack,
  Discord, Mattermost, Opsgenie and Matrix; `text` for Teams, Rocket.Chat and
  Google Chat. They are ignored, and kwatch logs a deprecation warning at
  startup if one is still set. Remove them from your configuration. Teams
  `title` still overrides the card title. These options now apply to plain
  operator messages only: the `subject` of SES, SendGrid, Mailgun and Resend
  (incident emails use the message's short summary as the subject), and the
  `title` of n8n and Zapier.
- **Alert keys include the cluster.** Provider deduplication keys and aliases
  are now `kwatch-<cluster>-<incident key>`, so two clusters that report the
  same incident no longer share one alert. Opsgenie alert aliases use this
  form.
- **Splunk On-Call.** Severity now follows the incident: info and plain notices
  send INFO, warning sends WARNING, critical sends CRITICAL, and resolution
  sends RECOVERY.
- **Datadog.** `alertType` is no longer defaulted. Unset, each event carries
  the incident's own severity. Set it to force one type.
- **Issue trackers.** Issue titles are the incident's short summary, and the
  body includes the cluster name.

### Upgrade notes

- **RBAC mode.** The chart value `rbac.mode` is `full` by default. `full`
  grants read-only `list` and `watch` on every resource in every API group so
  any built-in or custom kind can be understood, including CRDs installed
  later, and `get` only on namespaces, nodes, `nodes/stats`, `nodes/metrics`
  and `pods/log`.
  Both modes add a Role with `get` on pods in kwatch's own namespace, used to
  explain why the previous kwatch Pod restarted. Secret values are hashed and
  never stored. Choose `least-privilege` for an explicit `list`/`watch` set;
  kinds outside it are reported as permission denied in `/health` and custom
  resources are not covered.
- **Kubelet stats.** Node stats and metrics are read directly from each
  kubelet (node `InternalIP`, kubelet port, default 10250) instead of
  through the API server proxy. RBAC now grants `get` on `nodes/stats` and
  `nodes/metrics`, and `nodes/proxy` is no longer granted. The kwatch Pod
  must reach every node on the kubelet port. Kubelet serving certificates
  are verified with the cluster CA; set `kubelet.insecureSkipVerify: true`
  only for self-signed kubelet certificates.
- **Secrets switch.** The chart value and configuration field
  `watch.secrets` (default `true`) can turn Secret watching off; the chart
  then grants no Secret permission in either RBAC mode.
- **Lease RBAC.** `get` and `update` are limited to kwatch's own Lease by
  name.
- **Memory.** The default memory limit is 512Mi and `GOMEMLIMIT` is set to
  90% of it (`KWATCH_MEMORY_LIMIT` replaces the `GOMEMLIMIT` downward-API
  variable).
- **CLI.** The `replay` subcommand is removed; `kwatch lint` remains.
- **Configuration.** `activeProbeMonitor.recoveryThreshold` is removed (it
  was never applied: a probe resolves on its first success) and is now
  reported as an unknown key. `app.logFormatter: json` now switches logs to
  JSON lines. Unknown keys are reported with their full path and line.
- **State store reset.** There is no migration. At the first start the old
  `state.db` is deleted and a fresh store is created (reported as
  `storage_reset` on `/health`). Open incidents are announced once again as
  new, and a startup summary is sent. No backup of the old file is kept.
- **Opsgenie and other keyed providers.** Because the alert key format
  changed (it now includes the cluster), alerts opened by the old version
  will not be closed automatically when their incidents resolve. Close them
  by hand during the upgrade.
- **Storage.** The state store is capped at 512 MiB of logical data
  (evidence 128 MiB), on the default 2Gi volume. Past the cap the oldest
  history is dropped first; open incidents, baselines, fingerprints, state,
  threads and the outbox are never evicted. The cap also covers the file
  size: a file more than a quarter past it is rewritten at the next start when
  the volume has room. Baselines are dropped 7 days after their workload is
  gone. No backups are written. With `persistence.emptyDir`, the chart passes
  the `sizeLimit` as `KWATCH_VOLUME_LIMIT` so the rewrite checks the real
  limit.
- **NetworkPolicy.** If you run your own NetworkPolicy, allow egress from
  kwatch to TCP 10250 on every node. The chart's opt-in policy now does this
  (`networkPolicy.kubeletPort`, `networkPolicy.kubeletCIDRs`).
- **Delivery.** Delivery is at least once. Queued notifications are written
  to a persisted outbox (at most 2048 jobs, none older than 24 hours) and
  re-sent after a restart, so a restart no longer loses them. A send that was
  interrupted mid-request may repeat. Outbox problems show as
  `persistence_restore_failed` on `/health` and in the new outbox metrics.
- **Incident IDs.** IDs now look like `inc-20261001-7f3a-0007`: the UTC day,
  a 4-character random nonce kept with the store, and a sequence number. The
  nonce keeps IDs unique after a state reset or on a new empty volume, where
  the sequence starts over.
- **Telemetry.** Adoption telemetry is on by default in official builds. It
  sends exactly an anonymous installation ID (a salted hash of the
  `kube-system` namespace UID, formatted as a UUID) and the kwatch version to
  `https://api.kwatch.dev/v1/telemetry/heartbeat`, shortly after startup and
  then at most once every 7 days. Disable it with `telemetry.enabled: false`
  or `KWATCH_TELEMETRY=false`. See the configuration reference for the full
  description.
- **Health reasons.** `/health` reasons come from a fixed vocabulary that now
  includes `component_stalled`, `heartbeat_failed`, `storage_reset`,
  `storage_over_cap`, `permission_denied`, `optional_permission_denied`,
  `api_unavailable`, `cache_sync_pending` and `persistence_restore_failed`;
  the `coverage` object adds `disabled_by_config`.
- **Alert-quality gates.** The scorecard now has a calibration gate: stated
  high confidence must be right 80% to 100% of the time and likely
  confidence 50% to 90%, once a level has at least 10 labelled cases.
- **Alert-quality targets.** The scorecard gates realistic production
  targets: correct root at least 90% on labelled and 80% on a new held-out
  set that is never used for tuning, wrong high-confidence root at most 5%
  (gated at 20 or more cases), messages per incident p95 at most 3 and most
  at most 5 (was most at most 3), time to first message at most 60 seconds
  for page and 180 seconds for notify, no notification from healthy
  rollouts, scaling, drains, Jobs or CronJobs, and a busiest-hour ceiling of
  30 (was 20). `kwatch-scorecard` gains `-max-messages-per-incident-p95`
  (default 3); `-max-messages-per-incident` now defaults to 5 and
  `-max-peak-per-hour` to 30. The new histogram
  `kwatch_pipeline_decision_lag_seconds` measures the time from an
  observation being submitted to its decisions being applied.
- **Chart: state volume placement.** When the StorageClass's CSI driver is
  registered on only some nodes, `helm install` and `helm upgrade` now add a
  node affinity on the driver's topology label so the state claim can bind
  where the Pod lands (`persistence.pinToStorageDriverNodes`, default
  `true`; `persistence.storageDriverTopologyKey` sets the label for offline
  renders). Before, the Pod could stay `Pending` with the claim reporting
  `no topology key found for node`.
- **Chart.** `tolerations` are now merged with `defaultTolerations`; an entry
  with the same key and effect wins (an entry without an effect covers every
  effect of its key), so a `not-ready` `NoSchedule` toleration keeps the
  default 30-second `NoExecute` one. The container and probe port now come
  from `config.healthCheck.port`; the `service.port` value is removed, and
  rendering fails if `config.healthCheck.enabled` is false. The opt-in
  NetworkPolicy now admits the health port and allows DNS and API server
  egress by default (`networkPolicy.allowDefaults`,
  `networkPolicy.apiServerPorts`). The `emptyDir` volume gets a `sizeLimit`
  (`persistence.emptyDirSizeLimit`, default 2Gi), and installing with
  `persistence.emptyDir=true` prints a warning, because state is lost when the
  Pod is rescheduled.
- **Hourly budget.** `alert.<provider>.hourlyBudget` (integer >= 0, default
  60, `0` unlimited) sets how many new conversations a provider announces per
  hour; the rest are folded into one overflow digest. A negative or
  non-integer value fails startup.
- **Delivery retries.** Transient failures are retried with the provider's
  backoff for up to 24 hours through the outbox. `/health` reports
  `provider-<name>` components with `provider_unavailable`,
  `provider_rejected` or `provider_rate_limited`. A provider that is
  configured but cannot be constructed now fails startup. An incident that
  opens and resolves before announcement is sent as one "opened and
  resolved" message.
- **Roots and paging keys.** External endpoints and shared errors can be
  reported as roots. Paging dedup keys are built from root and mode, so they
  survive a state store reset.
- **New metrics.** `kwatch_delivery_queue_depth{provider}`,
  `kwatch_delivery_outbox_depth`, `kwatch_delivery_deferred_total`,
  `kwatch_tracker_untracked_total` and `kwatch_kubelet_stats_failures_total`;
  `/health` adds the `kubelet-stats` and `config-overlay` components.
- **Heartbeat.** The heartbeat monitor pings only while monitoring is ready.
  A leader that is still starting or is shutting down after a failure no
  longer keeps the external dead man's switch quiet.
- **Shutdown.** The leader drains queued alerts before saving provider thread
  IDs, so threads created by the last alerts survive a restart. After a lost
  Lease nothing more is sent. The drain also stops early, a few seconds
  before the Lease could expire, and the alerts still queued are
  dead-lettered rather than sent; their outbox records stay for the next
  session.

#### Alert keys and providers

- **Open alerts are not resolved after the upgrade.** Keys are now
  `kwatch-<cluster>-<key>`. An alert opened under an old key is not found
  when its incident resolves, so close it by hand. The `alertKey` sent by
  webhook, Zapier and n8n and the `X-Kwatch-Alert-Key` email header change
  value too; update any automation that matches on them.
- **Startup and plain notices.** PagerDuty, Opsgenie, Squadcast, GoAlert,
  Zenduty, incident.io, iLert and SIGNL4 (paging), and GitHub, GitLab, Gitea,
  Jira and ClickUp (issue trackers) no longer receive the startup summary or
  plain notices. They only receive incidents, so nothing is opened that cannot
  be resolved.

- **Zulip needs a `url`.** `alert.zulip.url` is now required. The old
  default sent the bot email and API key to `api.zulip.com`, which is not a
  shared Zulip host. Example hosts (`example.com`, `example.org`,
  `example.net`) are rejected.
- **Unused reasons removed.** These reason names were defined but nothing
  raised them, so no routing or silence rule could ever have matched an
  incident: `BackOff`, `ControlPlaneComponentFailure`,
  `CrashLoopHighFrequency`, `DaemonSetConditionFailure`,
  `StatefulSetConditionFailure`, `HPAScalingLimited`, `InformerLag`,
  `InformerWatchInterrupted`, `Killed`, `Killing`, `KubeletNotReady`,
  `KubeletReady`, `NodeAffinity`, `OOMRepeating`, `Preempting`,
  `PreExistingAtStartup`, `PreStopHookError`, `ProbeError`, `Pulled`,
  `ReplicaSetUpdated`, `Scheduled`, `SharedDependencyFailure`, `Started`,
  `TestAlert` and `TooManyReplicas`.

#### Configuration

- **Route severities are validated.** An unknown value such as `high` now
  fails startup. Use `critical`, `warning` or `info`.
- **Unknown or removed keys.** A key kwatch does not know, including options
  removed in this release, logs a warning instead of being ignored silently.
- **An invalid KwatchConfig no longer crash-loops kwatch.** An invalid edit
  is rejected without a restart and logged with `config_overlay_invalid`. An
  overlay that is invalid at startup is ignored: kwatch runs on the mounted
  configuration and `/health` shows `config-overlay` degraded.
- **Secret watching stays off.** A KwatchConfig can no longer turn Secret
  watching back on when `KWATCH_WATCH_SECRETS=false`.
- **Missing config file fails startup.** When `CONFIG_FILE` is set but the
  file does not exist, kwatch now refuses to start instead of running with
  defaults and no providers.
- **Blank matchers are rejected.** An empty string in a silence field or an
  `ignore*` list, such as `podNamePatterns: [""]`, is a validation error.
- **Provider option typos.** An unknown `alert.<provider>` key logs a warning
  with its path and a suggestion for an obvious typo
  (`alert.slack.webhok`). `kwatch lint` reports "no alert providers
  configured" as a warning, not an error.
- **KwatchConfig schema.** The CRD no longer offers `app.proxyURL`,
  `app.insecureSkipTLSVerify`, `app.caBundlePath` and `auditLog.output`,
  which an overlay was never allowed to set.

#### Messages

- **One status emoji.** Every message, including the startup message and the
  upgrade notice, starts with exactly one status emoji: 🔴 page, 🟠 notify,
  🟡 low, ✅ resolved. The startup and upgrade notices are single sentences
  without links.
- **Updates after a restart.** Open incidents restored from the state file may
  send one update after the upgrade.
- **Spreading updates.** An "It's spreading" update names every object that
  became affected since the last delivered message, not only the latest
  batch.

#### Chart and operations

- **Stricter values.** The chart rejects unknown keys at the root and in
  `rbac`, `watch`, `persistence`, `networkPolicy`, `config.kubelet` and
  `config.watch`, a health port outside 1-65535, and setting both
  `persistence.emptyDir` and `persistence.existingClaim`. Check your values
  file before upgrading.
- **ClusterRole names.** The ClusterRole and ClusterRoleBinding are now named
  `<fullname>-<namespace>` and carry chart labels, so releases with the same
  name in different namespaces no longer collide. Helm replaces the old
  objects on upgrade.
- **CRD overlay with `configSecretName`.** The chart passes
  `config.crd.enabled` as `KWATCH_CRD_ENABLED`, so installs that bring their
  own Secret keep the KwatchConfig overlay.
- **NetworkPolicy ports.** New `networkPolicy.extraEgressPorts` opens
  provider ports other than 443, such as SMTP 587 for email.
- **ServiceAccount.** It no longer carries a config checksum annotation.
- **Kubelet stats health.** Reachability is reported as the optional
  `/health` component `kubelet-stats` (`kubelet_unreachable`,
  `kubelet_partially_unreachable`) and counted by the new
  `kwatch_kubelet_stats_failures_total`. Each round starts where the last
  one stopped, so the same nodes are not skipped every time.
- **Bounded informer memory.** The Event informer lists only Warning events.
  A dynamic kind over its object cap caches the extra objects only as small
  stubs, and `/health` coverage counts kinds by reason, including the new
  `object_cap_reached`.
- **Node failure guidance.** The production guide documents the real
  failover gap after a node crash (about 7 to 8 minutes), the
  `ReadWriteOnce` block storage requirement, and the
  `node.kubernetes.io/out-of-service` taint.
- **CI and release gates (contributors).** `make ci` is the one required
  CI gate and `make verify` its local form; both run the tests once with
  `-race`. `make scorecard` and `make scorecard-gate` are now
  `make alert-quality` and `make alert-quality-gate`. The `CI`, `E2E` and
  `Security` workflows replace `Check`, `Alert quality`, `Real-cluster
  scenarios` and `Operational validation`; required status checks are now
  `Verify`, `Image`, `Chart lifecycle`, `Go dependency scan` and
  `Analyze (actions)` (import `.github/rulesets/main-branch.json`). A
  release now requires green CI, security and full E2E runs on the tagged
  commit, and the image is scanned before it is pushed.
- **Readiness follows required sources.** `/readyz` now fails while a
  required source (Pods, Nodes) is unavailable, such as after its RBAC
  permission is revoked, and recovers with it. A source counts as synced
  only after kwatch processed its whole initial list.
- **Downtime changes of slow kinds.** A kind that has not finished its first
  list when downtime changes are reconstructed keeps its stored baseline and
  is compared once it syncs, instead of losing its baseline.
- **Provider errors.** DingTalk, Feishu, WeCom and IFTTT errors reported in
  a successful HTTP response are now permanent (not retried for a day),
  except the providers' rate-limit codes. Alerta alerts use the stable
  deduplication key as their resource. Matrix no longer drops a plain
  message whose text repeats an earlier one.
- **Delivery reconfiguration.** A configuration reload keeps each provider's
  hourly budget, outage backoff and pending overflow digest instead of
  resetting them. A fallback leading into a cycle is no longer disabled;
  only the provider that closes the cycle loses its fallback.
- **Detection fixes.** A slowly filling volume is no longer reported as
  full within hours; a lost PVC with a resize error stays Critical; a
  restored recovering incident no longer sends an empty update; crash log
  excerpts keep the end of the log, where the crash message is.
- **Webhooks share their backend as root.** Several admission webhooks
  that call one Service with no ready endpoints, or one Service that does
  not exist, are now one incident rooted at that Service instead of one
  incident per webhook configuration. A single webhook keeps its own root.
- **Tier never drops while open.** An announced incident keeps the loudest
  tier it reached; a crash loop whose critical member clears for a moment
  no longer flips between page and notify with a message per flip.
- **Zones and node pools lead their incidents.** An incident caused by a
  zone or node pool failing as a whole now opens with the place and its
  nodes ("Node pool dev-arm-np is failing as a whole: two nodes have not
  reported for about two minutes") instead of whichever pod was worst.
- **Skipped resource types are named.** The startup log line about kinds
  left out by the watch budget now lists them.
- **Problems found together are one message.** Two or more announcements
  made in the same pass go as one roll-up that names them, like the startup
  summary; each problem gets its own message when it changes or resolves,
  page-tier ones still reach the paging tools on their own, and the roll-up
  closes once all it listed have resolved.
- **A strained node pool is not a failing one.** A zone or node pool fails as
  a whole only when two or more of its nodes have a failing finding (not
  ready, under pressure, network unavailable). Nodes with a CPU stall or high
  usage no longer make the pool take over the workload incidents on them and
  hand them back minutes later.
- **A late cause does not take over an announced incident.** A cause that
  began more than ten minutes after an incident was opened did not cause
  what people were already told about: the incident keeps its root and the
  new cause explains only the new failures. A zone or pool counts as having
  begun when its first node broke. A node pool that fails this morning no
  longer takes over a deployment that has been unavailable for a week.
- **Every crash loop gets a reason or the facts.** "I couldn't find an
  outside cause" is gone. The engine records which objects upstream of a
  failure it checked and found healthy and unchanged, and the message says
  so: "Its node, image and configuration are healthy and unchanged, so
  nothing outside it explains this." The quoted crash output follows.
- **What failing workloads share is suspected.** When three or more
  workloads start failing within ten minutes and nothing upstream shows a
  fault, the node, image, ConfigMap, Secret or ServiceAccount they all share
  is named as a possible cause, never above "possibly", and any cause with
  evidence of its own outranks it. It is suspected only when most of what
  depends on it is failing.
- **Dependencies outside the cluster are modelled and probed.** Environment
  values that name a URL or a `host:port` (only host and port are kept; user
  names, passwords and paths never leave the value) become
  `external-endpoint` entities the pods call. With
  `activeProbeMonitor.autoDependencies: true` kwatch dials them from its own
  Pod, and a dependency that refuses connections is named as the cause of
  the pods that call it, even for a single workload.
- **Configuration risks, in the digest and as the cost of a failure.** A new
  detector reports a workload with no readiness probe, no memory limit, an
  image tag that can change (`latest` or none), a single replica, every
  replica on one node, or a privileged container. These are advisory: they
  never open an incident on their own, they wait for the digest, and when a
  failure of that workload shows what the risk cost, the message says so
  ("It runs a single replica, so this is downtime, not degradation.").
- **An overcommitted node explains kills on it.** A node whose pods' memory
  limits add up to more than 150% of its memory is reported (digest tier)
  and becomes a cause for OOM kills and evictions of pods that stayed within
  their own limit.
- **Unknown Warning events are not silent.** A Warning event kwatch has no
  detector for, repeated three times in a quarter hour on one object, becomes
  an informational finding that quotes the event text, so a new failure
  type is seen before a detector exists for it.
- **Changes say why they were made.** The `kubernetes.io/change-cause`
  annotation of a blamed change is quoted: `recorded as "bump payments to
  2.3 for the refund fix"`.
- **A failing node agent explains the pods beside it.** A kube-system
  DaemonSet pod (the CNI, kube-proxy, a CSI node plugin) that fails on a
  node is named as the cause when two or more workloads on that node lose
  their readiness or network at the same time.
- **The same tag, different builds.** A workload whose running pods report
  different image IDs for the same image reference is reported
  (`ImageDigestDrift`), and the drift explains why some replicas fail and
  others do not.
- **DaemonSet gaps name their nodes.** A DaemonSet below its desired count
  lists the nodes whose pod is not ready and the taints on them.
- **Controllers that run but do not work.** kwatch reads every leader Lease
  outside node heartbeats every two minutes; a Lease its running holder has
  not renewed for three durations is reported (`LeaseStale`). A Lease whose
  holder is gone is left alone, since an uninstalled controller leaves one.
- **Updates carry current evidence.** A material change whose evidence is
  older than two minutes is investigated again before the update is sent,
  so the quoted output is what the application says now.
- **HTTPS probes watch the certificate.** A probed HTTPS target records the
  expiry of the certificate it served, and the certificate check covers it.
- **Recurrences name their shape.** When every time people heard about
  followed the same kind of cause, the message says so: "This is the third
  time this week, each time after a rollout."
- **A claim can pin a pod.** When the scheduler rejects a pod for a volume
  node affinity conflict, the claim bound to a volume in a zone with no
  node for the pod is the cause (`claim-pins-pod`), not the scheduler.
- **Removal taints are drains.** A node tainted for scale-down or marked
  out of service is treated like a cordoned node: its disruption is the
  maintenance itself, and only what fails because of it is reported.
- **The audit log says what was ruled out.** Each entry carries
  `considered`: the other causes the solver weighed, best first, as
  `root (row, confidence)`.
- **API server and cluster DNS metrics.** The prober reads the API server's
  own `/metrics` and the cluster DNS pods' metrics port, both part of
  Kubernetes itself, and reports an API server answering 5% or more of
  requests with server errors (`APIServerErrors`) or a cluster DNS failing
  10% or more of lookups with SERVFAIL (`CoreDNSServfail`). A cluster
  without those endpoints loses only these findings.
- **Configuration risks reach the digest.** A risk the detector finds is
  named once in the next low-priority digest ("Risk: orders runs a single
  replica"); it never costs a message of its own.
- **Kubelet evidence.** The kubelet's pod lifecycle relist time and its
  evictions are read from its own metrics: a kubelet that takes over a
  second to list its pods is reported (`NodePLEGSlow`), and evictions in
  progress go to the digest (`NodeEvicting`).
- **Idle objects in the digest.** A Service that has selected no pod for a
  day (`ServiceUnused`) and a bound claim no pod has mounted for a day
  (`ClaimUnused`) wait for the digest.
- **The watch budget favours noisy kinds.** Over the resource-type budget,
  kinds whose objects had Warning events in the last hour are watched
  before kinds nothing has complained about, instead of alphabetical order.
- **Known problems go to the digest after a day.** A problem people have
  heard about twice, the first time a day ago or more, no longer interrupts
  on its next recurrence; the digest keeps counting it. A page stays a page.
- **Regular rhythms are recognised and stated.** An incident that recurs at
  even intervals within a day is treated as known, and says so: "It fails
  every 40 minutes or so; this is the third time in a day."
- **A weekly "still open" reminder.** An announced incident that stays open
  gets one update a week: "payments in shop is still down, for two weeks now."
- **The resolve message names the cause.** "... it was failing for 42 minutes
  because node n3 was low on memory." A workload blamed on itself names none.
- **Replacement grace for pods on fresh nodes.** A pod younger than ten
  minutes on a node younger than ten minutes gets five extra minutes before
  `ContainersNotReady` or workload unavailability is reported, so a scale-up
  or consolidation is not announced as an outage.
- **A dissolved group ends instead of changing subject.** When the members of
  an announced incident disperse to several roots, the incident recovers and
  resolves on its own and the workloads are announced anew (as one roll-up).
  Its conversation no longer continues under whichever member left last.
  Members that all leave for one root still move the conversation there.
- **Audit log records every decision.** Decisions the digest, a roll-up or
  the startup summary carry are written to the audit log when they are made,
  with `delivery: digest`, `roll-up` or `startup summary`; announcements that
  go only to paging tools carry `delivery: paging`.
- **Low-priority digest.** Digest-tier incidents (an autoscaler at its
  maximum, a budget that selects nothing, a throttled container, learned
  routines) are no longer announced one by one. One 🟡 digest every 30
  minutes lists what opened and what resolved; an incident that gets worse
  is announced at once. Paging tools and issue trackers never receive it.
- **"The second time this week" counts heard times.** A blip that recovered
  before it was announced still counts for flapping and routines, but is
  no longer called an earlier time in the next message.
- **One message when incidents merge.** When another incident takes over
  an incident's failures (a node pool explaining a dozen crashing
  Deployments), the absorbed incident's close now goes only to paging tools
  and issue trackers, which hold an alert under its key; chat channels read
  about those failures in the merged incident's update instead of one
  "cause revised" message per workload.
- **Bursts settle longer.** Three or more incidents settling at the same
  time take the 75-second settle even at page tier, so a shared cause can
  surface and be announced once instead of each incident paging and then
  being revised seconds later.
- **Startup summary holds pages too.** At a cold start, pre-existing
  page-tier problems are listed in the startup summary instead of being
  announced one by one to chat channels; paging tools and issue trackers,
  which never receive summaries, still get each of those announcements on
  its own.
- **Fewer wrong roots.** A zone or node pool is no longer blamed for a
  crash on a healthy node; stack-trace locations such as `Pool.java:512`
  are no longer read as external endpoints; one pod's lookup of a
  mistyped external host no longer blames cluster DNS; only the webhook
  named in an admission error can be blamed for it, and an `Ignore`
  webhook whose call failed cannot; "no space left on device" blames a
  claim only with evidence for that claim; image tags are no longer read
  as HTTP status codes; a named quota must match; generic errors such as
  "connection reset by peer" alone no longer form a shared-error root.
  A cause that starts failing after its effect is now found as well.
- **Fewer false findings.** Gateway routes to non-Service backends and
  AWS ALB `use-annotation` actions are not reported as missing Services;
  completed pods and init containers no longer count toward node
  overcommit; pod ephemeral storage is compared only when every container
  has a limit; HPA metric failures wait the two-minute condition grace;
  a recovered load balancer clears its sync-failure warning; finished or
  terminating pods no longer block a drain; and one reason raises one
  finding per object.
- **Changes made while kwatch was down.** Objects deleted or created while
  kwatch was not running are now recorded as changes, dated at the last
  snapshot, like edits already were. A deleted ConfigMap or Secret can
  therefore be named as the cause of a failure found after a restart.
  Creations are reported only once a snapshot has covered that kind, so an
  upgrade does not announce every existing object as new.
- **"The cause is not clear yet."** When kwatch considered an outside
  cause and rejected it, and the failing object has no accepted cause, the
  message says so instead of staying silent about the cause.
- **Message text.** Quantities such as `500m` or `12h` in names, versions
  and resource limits are no longer rewritten as durations.
- **`kwatch lint` is stricter.** It now rejects a provider whose required
  options are empty after expansion, an out-of-range `healthCheck.port`,
  an invalid probe URL or TCP address, severity override keys that differ
  only in case, and unknown arguments (`lint --stirct` used to pass). It
  constructs providers as startup does, so a config that startup refuses
  fails lint; `--check` still contacts providers.
- **Maintenance annotations.** Only `true`, `1`, `yes` or `on` hold an
  object; a valid `kwatch.io/maintenance-until` in the past ends the hold
  even when the annotation is still set.
- **Redaction.** Credentials in URLs are redacted up to the last `@` of the
  user info; camelCase keys (`secretAccessKey`, `privateKey`),
  `passphrase`, `pwd`, `credentials`, cookies, `--password X` and `-pX`
  are covered; plain words after `secret:` or `token:` such as "not found"
  or "expired" are no longer removed.
- **KwatchConfig.** A deletion delivered after a watch gap is now seen, and
  a watcher that fails to start after a late CRD install retries with
  backoff instead of staying dead.
- **Matrix.** `@room` is neutralized like other broadcast mentions, and
  repeated plain notices are no longer deduplicated by the homeserver.
- **Audit log.** A failed rotation (for example a full volume) no longer
  stops the audit log; entries keep going to the current file and the
  error is logged.
- **Rate limits in response bodies.** DingTalk, Feishu and WeCom frequency
  limits reported in a 2xx body are classified as rate limits, so the
  provider is paced rather than marked unavailable.
- **Contributors.** `make verify` now runs the decision-latency and memory
  budget tests without the race detector (`make verify-latency`), so the
  2-second p99 target is enforced. The e2e issue sanitizer uses an
  allowlist of namespaced workload kinds and removes `command` and `args`.

This document describes how kwatch is branched and released. Releases are cut with the
`.github/workflows/release.yml` workflow using `workflow_dispatch`. It creates the version
tag and a GitHub Release; `.github/workflows/publish.yml` then builds and pushes the
multi-arch container image to `ghcr.io/abahmed/kwatch`, publishes the stable Helm
chart, and syncs release metadata to `kwatch.dev`.

## Branch model

**One branch: `main`.** All changes land here via short-lived PR branches
(`feat/*`, `fix/*`, `refactor/*`). `main` is protected (review + green `check` required).

There are no release or develop branches. Every release — RC, stable, or patch — is a
**tag** on `main` history plus a GitHub Release.

## Versioning

Semantic versioning, `v`-prefixed:

| Stage | Example | How it is produced |
|---|---|---|
| RC (pre-release) | `v0.11.0-rc.1` | `rc` command. Takes the next number while a series is open; otherwise opens a new series using `bump` |
| Stable | `v0.11.0` | `stable` command, promotes the newest RC |
| Patch | `v0.11.1` | `patch` command, increments the highest stable tag |

Versions are always computed from existing tags — maintainers never type a version, they
only choose which part moves.

> **Tag hygiene:** only `-rc.<N>` pre-releases are recognised. The workflow matches
> `vX.Y.Z` for stable tags and `vX.Y.Z-rc.<N>` for RCs, exactly. A tag like `v1.0.0-rc.1`
> is ignored by both, so do not create other pre-release forms — the version computation
> will behave as if it does not exist.

> **Going to v2:** Go requires the module path to carry a `/v2` suffix from major 2 on. The
> workflow warns when it computes a major ≥ 2 while `go.mod` still lacks the suffix, but it
> does not block. Update `go.mod` and every internal import before shipping a v2, or
> `go install ...@v2.0.0` will fail.

## Cutting a release

Run the **Release** workflow (Actions → Release → Run workflow) and set:

| Input | Required | Meaning |
|---|---|---|
| `command` | yes | `rc`, `stable`, or `patch` |
| `bump` | no | `minor` (default), `major`, or `patch`. Only used by `rc` when it opens a new series |
| `new_series` | no | For `rc` only. Abandon an open RC series and start a new one from the latest stable |
| `target` | no | A commit sha/ref to tag. Defaults to `main` HEAD |
| `dry_run` | no | Compute the version and notes, then stop. Nothing is tagged or built |

Every run writes a **summary on the run page**: the version, whether it is a pre-release,
the tagged commit, the image tags Publish will push, the release URL, and the full notes.
You never need to read the logs to find out what was cut.

### `rc` — pre-release

1. **While a series is open** — an RC whose base version is not yet a stable tag — `rc`
   simply takes the next number: `v0.11.0-rc.4` → `v0.11.0-rc.5`. The `bump` input is
   ignored here, and the run logs a notice saying so.
2. **When no series is open** — the newest RC was already promoted (it counts as
   **consumed**), or there are no RC tags at all — `rc` opens a new series from the newest
   stable, and `bump` decides which part moves:

   | `bump` | From `v0.11.0` | Use for |
   |---|---|---|
   | `minor` (default) | `v0.12.0-rc.1` | new features |
   | `patch` | `v0.11.1-rc.1` | a fix you want to soak before shipping |
   | `major` | `v1.0.0-rc.1` | breaking changes |

If an old RC series should be abandoned before promotion, set `new_series: true`.
Then choose the desired `bump`; `bump: major` starts `v1.0.0-rc.1` from the latest
stable even when an older `v0.11.0-rc.*` series is still open. Use this only when
the older series is intentionally no longer the release target.

3. Creates the tag and opens a GitHub Release marked **pre-release**.
4. Release notes compare against the **previous RC** while a series is open, so each RC
   lists only what is new since the last one. The first RC of a series compares against the
   newest stable.
5. `publish.yml` pushes `ghcr.io/abahmed/kwatch:v<X>.<Y>.<Z>-rc.<N>` only — **no `latest`**,
   and the in-app upgrader does not nag RC users.
6. Regenerates the configuration, feature, and provider catalogs and pushes any
   resulting release metadata commit to `main`. The README install instructions
   are version-free. After publishing, `publish.yml` updates the preview version
   shown on `kwatch.dev` and deploys the site automatically.
7. Adds a second commit — **not pushed to `main`** — that pins `deploy/deploy.yaml` to the RC
   image, and puts the tag on it. So `kubectl apply` against the RC tag installs the
   candidate, while `deploy/deploy.yaml` on `main` still points at the latest stable. See the
   note below.

Run `rc` as often as needed until the candidate stabilizes.

> **Why an rc makes two commits.** `deploy/deploy.yaml` has to say two different things at
> once: on `main` it must pin the latest **stable** image, so that copying it out of the repo
> browser can never install a preview build; at the RC tag it must pin the **RC** image, so
> that `kubectl apply` against the tag actually installs the candidate. One commit cannot do
> both, so `rc` makes two:
>
> | Commit | Contains | Pushed to `main` | Tagged |
> |---|---|---|---|
> | 1 | Release catalogs and metadata, when changed | ✅ | — |
> | 2 | `deploy/deploy.yaml` image → the new RC | ❌ never | ✅ |
>
> The second commit exists only on the tag. That is why an RC tag sits exactly one commit
> ahead of `main`, and why the `stable` guard compares by reachability instead of by sha.
>
> `stable` and `patch` make a single commit — pushed and tagged — because they have no such
> conflict: the released manifest and `main`'s manifest are the same thing.

### `stable` — promote the latest RC

1. Verifies `main` carries **nothing the newest RC does not already have**
   (`git rev-list --count <rc>..origin/main` must be 0); otherwise it fails and you must cut
   a fresh RC first. This is a reachability check, not sha equality — an RC tag is one commit
   ahead of `main` on purpose (see the note under `rc`), so comparing shas would always fail.
2. Bumps the stable manifest and chart versions, strips `🚧 Unreleased` banners
   from `README.md` and every `docs/*.md`, commits them to `main`, and pushes
   (`RELEASE_TOKEN` required; see below):
   `deploy/chart/Chart.yaml` (`version`, `appVersion`), `deploy/chart/README.md`,
   and `deploy/deploy.yaml` (image tag).
3. Creates the `v<X>.<Y>.<Z>` tag **on that bump commit** and opens a normal GitHub Release
   (gets `latest`). Because the tag commit carries the bumped files, the raw
   `/kwatch/vX.Y.Z/deploy/...` refs and the chart at the tag match the released version.
4. `publish.yml` updates the stable version shown on `kwatch.dev`, clears the preview
   version, builds the site, and triggers one Render deployment.

> **Pinned-version invariant:** on `main`, the chart version, `deploy.yaml` image tag,
> and chart README always point at the latest stable release. The README install
> command deliberately resolves the current stable release through `kwatch.sh`, while
> the interactive manager offers a published RC. Every manifest pin is bumped by the
> release workflow and never by feature PRs. The `docs/` reference pages carry no
> version pins; a reference page that needs an install command links to the manager.

### `patch` — hotfix tagged on `main`

1. Merge the fix to `main`.
2. Run the workflow with `command: patch`. It computes `v<X>.<Y>.<Z+1>` from the highest
   stable tag. The `bump` input does not apply — `patch` always moves the patch number.
3. Bumps the pinned references to the patch version and pushes to `main` (same commit/step
   as `stable`, banners are **not** stripped — the next minor's features are still pending).
4. Creates the tag on that bump commit and opens a normal release.

> **A patch blocks a pending promotion.** If an RC is still waiting to be promoted, `patch`
> prints a warning: its bump commit moves `main` past the RC tag, so `stable` will refuse
> until you cut a fresh RC. Cut the RC again after the patch, then promote.

> **Hotfix isolation:** tagging `main` HEAD also bundles any unreleased minor work already
> merged. If the hotfix must ship exactly on top of the previous stable, set `target` to the
> hotfix commit sha (a detached one-off tag). For a strictly isolated hotfix line you can
> also create a throwaway branch locally, cherry-pick, and pass its sha as `target` — no
> persistent release branches are ever kept. When `target` is set, the version bump is made
> on that commit and carried by the tag, but it is **not** pushed to `main` — pushing it
> would rewrite `main`'s pinned versions backwards. Update `main` by hand if it should
> carry the new version.

> The version-bump commit is pushed to the protected `main` branch, so **all three commands**
> require a **`RELEASE_TOKEN`** secret (a maintainer classic PAT with `repo` scope, allowed
> to bypass branch protection). `rc` needs it too, since it may push regenerated catalogs.
> If the secret is missing or the push fails, the workflow stops **before** tagging and
> prints the manual `git push` command to run.
>
> `RELEASE_TOKEN` is also what makes the image get built. A GitHub Release created with the
> default `GITHUB_TOKEN` does **not** trigger other workflows, so `publish.yml` would never
> run and the release would ship with no container image.

### Previewing a release

Set `dry_run: true` to work out the version and the release notes and then stop. Nothing is
tagged, no release is opened, no image is built. The job is titled **Preview** and the run
summary shows the version it would cut, the image tags it would push, and the full notes.

Use it whenever you are unsure which version a command will produce — for example before a
`rc` that opens a new series, where the answer depends on `bump`.

## RC → stable gates

An RC should not be promoted until all of these hold:

- [ ] RC has been published for at least **2 weeks** of soak (unless a critical fix is blocking).
- [ ] No open **critical** issues / known regressions against the RC.
- [ ] `CI`, `Security and supply chain` and a full `E2E` run are green on the RC commit (the release workflow enforces this).
- [ ] `helm lint` + `test_helm.sh` pass for the released chart.
- [ ] Release notes reviewed (generated automatically from merged commit titles).
- [ ] README and `docs/` contain no `🚧 Unreleased` banners (stripped automatically on stable).

## README, docs, and unreleased features

Feature **code** merges to `main` immediately. A feature's **README section** also merges
immediately, but under a banner while unreleased:

```
> **🚧 Unreleased** — ships in `v0.12.0`. Not available in stable installs yet.
```

- Unreleased sections stay visible on `main` with the banner, so docs don't drift.
- When a whole milestone rewrite is unreleased (e.g. the current `v0.11.0-rc` build), one
  top-of-file banner marks the entire README as documenting the dev build. Same `🚧 Unreleased`
  marker, stripped the same way.
- The `stable` command strips every banner line from `README.md` and `docs/*.md` and bumps
  the pinned manifest and chart references in the same commit.
- Maintainers never touch version numbers by hand; the pinned-version invariant and the
  banner convention are enforced by `CONTRIBUTING.md` and the gate checklist above.

### Version-free install instructions

The repository README intentionally does not pin a stable or preview version. The
interactive `kwatch.sh` manager resolves the latest stable release by default and
offers a published RC during installation or upgrade. Release-specific image and
chart pins remain in the manifests and chart metadata maintained by the workflow.

## Releasing the Helm chart

Publishing is automatic. The `publish_helm_chart` job in
`.github/workflows/publish.yml` runs from the stable GitHub Release tag, verifies that
`deploy/chart/Chart.yaml` has the same version, runs `helm lint`, packages the chart, and
updates `static/charts/index.yaml` in the `abahmed/kwatch.dev` repository.

The following `update_website` job updates `src/data/releases.json`, builds the Docusaurus
site, pushes the metadata commit, and sends one Render deploy hook. RC releases skip the
Helm chart job but still update the preview version on the site. Docusaurus serves the
generated files under `static/charts` at `https://kwatch.dev/charts`.

Two repository secrets are required in `abahmed/kwatch`:

- `RELEASE_TOKEN`: a token with permission to push the protected `main` branch, create
  tags/releases, trigger `publish.yml`, and **Contents: Read and write** access to
  `abahmed/kwatch.dev`. It must not be replaced by the default `GITHUB_TOKEN`.
- `RENDER_DEPLOY_HOOK_URL`: the secret Deploy Hook URL copied from the `kwatch.dev` Render
  service settings. The workflow sends one `POST` after pushing the website metadata.

Disable Render's normal **Auto-Deploy** for the `kwatch.dev` service when using this hook;
otherwise the same website push can start two deploys.

If the token is missing or cannot push to `kwatch.dev`, the chart job fails without changing
the website repository. The container image job and GitHub Release are separate jobs, so the
release remains inspectable and the publish workflow can be safely re-run after fixing the
token. The final verification job fails the release workflow if the image, website sync, or
public chart (for stable releases) is not successful.

## Upgrader notes

The container image bakes the full version string (the release tag name, `v`-prefixed). The
in-app upgrader only runs on **stable and patch** images: it compares the baked version
against the latest **non-pre-release** GitHub Release and notifies on a newer one, recording
the notified version in the on-disk state store so users are nudged once. **RC builds skip the check
entirely** (`CheckUpdates` returns early when the baked version contains `-rc`) — RC users
opted into the dev channel and are never nagged toward stable. Keep this in mind: the baked
version must equal the release tag name (`v`-prefixed), or the equality comparison in the
upgrader never matches.
