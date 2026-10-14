# Production operations

This page describes the supported operating model for Kwatch. It is an
operations reference, not a promise of high availability.

## Operating model

Kwatch runs one replica with the `Recreate` strategy. State lives in a bbolt
file on a PVC mounted at `/var/lib/kwatch`. A Kubernetes Lease is used only as
a lock: it stops two processes from writing the volume at once. Only the Lease
holder claims the state file, with an epoch counter kept in the file itself,
so a process whose claim was superseded cannot overwrite newer state. There
is no second Pod and no Kwatch self-failover; if the Pod or its node fails,
Kubernetes restarts it and Kwatch resumes from the volume.

The lock matters even with one replica because two Kwatch processes can
still exist for a short time: a node cut off from the network keeps its old
Pod running while Kubernetes starts a new one elsewhere, and a Pod stuck in
`Terminating` or a manual restart can overlap the next one. The process that
loses the Lease stops sending and stops writing, so alerts are never sent
twice and the state file has one writer. Some names still say "leader"
(`KWATCH_LEADER_ELECTION_NAME`, the `<id>-leader` Lease, the
`*-leader-election` Role and the `kwatch_leadership_*` metrics); they are
kept so existing installs and dashboards keep working, and all refer to this
state lock.

### Node failure: the real monitoring gap

A crashed or unreachable node is the slowest case, because Kubernetes, not
Kwatch, decides when the Pod may start elsewhere:

1. **Node detection and eviction, about 70 to 80 seconds.** The node
   controller marks the node `NotReady` after roughly 40 seconds without a
   heartbeat, and the default `not-ready`/`unreachable` toleration of 300
   seconds is shortened by the chart's `defaultTolerations`
   (`tolerationSeconds: 30`), so the Pod is marked for deletion about 70 to
   80 seconds after the crash.
2. **Volume force-detach, up to about 6 minutes more.** A `ReadWriteOnce`
   volume stays attached to the dead node until the attach-detach controller
   gives up waiting for the kubelet to unmount it. That timeout is 6 minutes
   by default (`maxWaitForUnmountDuration`). Until the volume detaches, the
   replacement Pod stays in `ContainerCreating`.
3. **Startup**, then readiness within the targets above.

Plan for up to about 7 to 8 minutes without monitoring after a hard node
failure. An external dead man's switch (the heartbeat integration) is the
only signal during that window.

To shorten the gap once you know the node is really down (powered off, not
just partitioned), taint it out of service:

```sh
kubectl taint nodes <node> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute
```

Kubernetes then deletes the Pods on that node at once and detaches their
volumes without the 6-minute wait. Remove the taint after the node is
repaired or deleted. Never apply it to a node that might still be running:
it removes the volume protection described below.

### Volume requirements

The state volume must be **`ReadWriteOnce` block storage** (for example a
cloud disk such as EBS, Persistent Disk, or Azure Disk, or a local volume).
Do not use NFS, EFS, CephFS, Azure Files, or any `ReadWriteMany` class.

Fencing relies on the volume: the Lease and the epoch in the state file stop
an old process from writing after it lost the Lease, but only RWO attach
exclusivity guarantees that a process on a partitioned node cannot reach the
same file at all. With a shared file system two Pods on two nodes can open the
file at once, and bbolt file locks are not reliable over network file
systems, so the state file can be corrupted.

**The claim stays `Pending`.** With a `WaitForFirstConsumer` StorageClass
the claim is provisioned for the node the scheduler picked for the Pod. When
the class's CSI driver is registered on only some nodes (for example the
driver's DaemonSet does not run on an architecture or node group), a Pod
placed on a node without it never gets a volume, and the claim's events say
`no topology key found for node <name>`. Either keep kwatch on nodes the
driver serves, with a node affinity on the driver's own topology label (the
label exists only where the driver is registered; any CSI driver has one),
or use an `emptyDir`. With the chart:

```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: <the driver's topology key>   # e.g. topology.ebs.csi.aws.com/zone
              operator: Exists
```

`kubectl get csinodes -o yaml` lists each node's drivers and their topology
keys. Both installers handle this case before it fails: `helm install` and
`helm upgrade` look the driver up and add that affinity when
`persistence.pinToStorageDriverNodes` is true (the default; offline renders
such as `helm template` or Argo CD cannot look it up and take
`persistence.storageDriverTopologyKey` instead), and `kwatch.sh` checks the
driver's node coverage before applying and pins the Pod the same way. The
raw `deploy/deploy.yaml` cannot ask the cluster; add the affinity by hand.

Initial operating targets are: readiness within 120 seconds after a healthy
startup and graceful shutdown within 45 seconds. These are SLO targets for
capacity planning and alerting, not guarantees. Queue limits and configured
worker counts remain the authority for memory and delivery capacity.

Use the published image digest for production deployments. Version tags and
chart versions must refer to the same release. Kwatch serves only `/healthz`,
`/readyz`, `/availabilityz`, `/health`, and `/metrics`; `/status` is added
only when `healthCheck.statusEndpoint` is true (default false, 404 otherwise).
It has no profiling endpoints to protect. `/status` is read-only and lists
object names (workloads, nodes, budgets, webhooks) to anyone who can reach the
health port, so do not expose that port outside the cluster; read it with
`kubectl port-forward`. With the NetworkPolicy on, the health port (including
`/metrics` and `/status`) is reachable only from pods in the release namespace,
from the node ranges in `networkPolicy.kubeletCIDRs`, and from the peers you
list in `networkPolicy.healthIngressFrom`.

The Helm chart includes an opt-in NetworkPolicy template. With its default
`networkPolicy.allowDefaults: true` it admits the health port to its own
namespace, to the `networkPolicy.kubeletCIDRs` nodes and to the scrapers in
`networkPolicy.healthIngressFrom` (a list of `namespaceSelector`,
`podSelector` or `ipBlock` peers, for example your Prometheus namespace). Most
network plugins always let a node reach its own pods, so kubelet probes work
without more; if yours enforces policy on node traffic, set `kubeletCIDRs`.
It allows egress to DNS, to TCP
`networkPolicy.apiServerPorts` (443 and 6443) on any address, which covers the
Kubernetes API server and HTTPS notification providers, and to the kubelet on
every node (`networkPolicy.kubeletPort`, default 10250) for node stats. Set
`networkPolicy.kubeletCIDRs` to your node address ranges to limit the kubelet
rule. Providers on other ports need their port in
`networkPolicy.extraEgressPorts`, for example SMTP (25, 465 or 587) for the
email provider, or a custom webhook or self-hosted server on a non-443 port.
It also opens TCP `networkPolicy.dnsMetricsPort` (default 9153) to the cluster
DNS pods in `kube-system` for the DNS request and SERVFAIL rates.
`networkPolicy.apiServerPorts` must list at least one port, because an empty
egress port list would admit all egress.

The defaults do not cover what your own configuration makes kwatch dial:
active probe targets (`http`, `tcp`, `dns`), every `autoServices` Service
and `autoDependencies` endpoint, the `app.proxyURL` port, and heartbeat or
provider URLs on a port other than 443. With the policy enabled they fail
(for example, active probes report every target down) until you open them
with `networkPolicy.extraEgressPorts`, a precise `networkPolicy.egress` rule,
or, when you accept unrestricted egress from the pod,
`networkPolicy.allowProbeEgressAll: true`. `helm install` prints a warning
when the policy is on and active probes or a proxy are configured. Add
`ingress` or `egress` rules for anything else. The policy is disabled by default: the API server address,
the kubelet port and the alert providers differ per cluster, a policy that
misses one silently stops monitoring or alerting, and existing installations
must not lose connectivity during an upgrade.

### Kubelet stats

Node, container and volume usage (`/stats/summary`, `/metrics/cadvisor`,
`/metrics`) is read directly from each node's kubelet over HTTPS, at the
node's `InternalIP` and the port in
`status.daemonEndpoints.kubeletEndpoint.Port` (default 10250). kwatch sends
its ServiceAccount token and verifies the kubelet serving certificate with
the cluster CA, which works when kubelet serving certificates are signed by
it (for example with `serverTLSBootstrap`). For kubelets with self-signed
serving certificates set `kubelet.insecureSkipVerify: true` in the mounted
configuration; it cannot be set from a KwatchConfig. RBAC grants `get` on
`nodes/stats` and `nodes/metrics`; `nodes/proxy` is not needed. The kwatch Pod
must reach every node on the kubelet port: allow it in any NetworkPolicy or
firewall in front of the nodes. A node whose kubelet cannot be reached only
loses usage attributes; readiness is not affected.

Reachability is visible as the optional `/health` component `kubelet-stats`:
`degraded` with `kubelet_unreachable` (no node answered),
`kubelet_partially_unreachable` (some nodes failed, or the round ran out of
time), `optional_permission_denied` (a kubelet answered 401 or 403), or
`source_not_configured`. Failed reads are counted by
`kwatch_kubelet_stats_failures_total` and logged at most once every 10
minutes, with one log line on recovery. Each round starts at the first node
the previous round did not reach, so a slow round never starves the same
nodes.

### CPU and probes

The default CPU request is 100m with a 500m limit, so a burst of events
(for example a nightly node scale-up) is not throttled. The liveness and
readiness probes wait 3 seconds for an answer and tolerate 3 failures in a
row.

### Node autoscalers

kwatch holds one state file under one lock, so evicting its Pod to repack
or scale down nodes costs a restart and a gap in monitoring. The manifest
and the chart's default `podAnnotations` set the plain annotations
`karpenter.sh/do-not-disrupt: "true"` and
`cluster-autoscaler.kubernetes.io/safe-to-evict: "false"`. They only ask a
node autoscaler to leave the Pod alone, and they do nothing where none reads
them; kwatch itself uses only the Kubernetes API and the kubelet.

### Memory

The default memory limit is 512Mi. kwatch sets the Go soft memory limit
(`GOMEMLIMIT`) to 90% of the container limit, read from the
`KWATCH_MEMORY_LIMIT` downward-API variable, so the garbage collector works
harder before the kernel would kill the Pod. An explicit `GOMEMLIMIT` always
wins. A model of 5,000 simple pods and 500 nodes measured about 50 MiB of
live heap (`TestMemoryBudgetLargeCluster`, which fails above 65 MiB of live
heap or 200 MiB of peak heap).

Size the limit from the number of pods. Measured with realistic pods (three
containers, 45 environment variables, six volumes, probes, managed fields;
`TestMemoryBudgetTwentyThousandPods`, run with `KWATCH_MEMORY_20K=1`), kwatch
holds about 26 KB of live heap per pod: about 8 KB in the informer cache and
18 KB in the model, which keeps one entity per pod and per container. Plan
for twice the live heap, so the garbage collector has room and a burst of
events does not reach the limit.

| Pods | Live heap (about) | Memory request and limit |
|------|-------------------|--------------------------|
| up to 2,000 | 60 MiB | `512Mi` (chart default) |
| 5,000 | 140 MiB | `512Mi` (chart default) |
| 10,000 | 270 MiB | `768Mi` |
| 20,000 | 520 MiB | `1Gi` (`1280Mi` for pods with larger specs) |

The default 512Mi supports about 8,000 realistic pods; beyond that, raise
`resources.requests.memory` and `resources.limits.memory` together. Pods
with many containers, large environment lists or many volumes weigh more,
so measure with the `self-health` heap line and the Go heap metrics rather
than trusting the table at the edges.
Every 30
minutes kwatch logs a `self-health` line with the Go heap in use, the
number of goroutines and the sizes of its bounded in-memory maps, so slow
growth shows in the logs before the limit does.

The chart takes the container and probe port from `config.healthCheck.port`
(default 8060) and refuses to render with `config.healthCheck.enabled: false`,
because the probes need the health server. With `configSecretName`, keep
`config.healthCheck.port` in the chart values equal to the port in the
Secret. The Secret's `config.yaml` replaces the chart's `config` (they are
not merged). The chart restarts the Pod on a Secret change only when it can
read the Secret through Helm `lookup`, which needs a live cluster; offline
renders (`helm template`, `--dry-run`, Argo CD) cannot see the Secret, so
restart the Deployment yourself after editing it.

## Health and readiness

- `/healthz` reports process liveness.
- `/readyz` reports whether required monitoring infrastructure is ready.
- `/availabilityz` reports whether the Pod is participating in the application
  lifecycle and is used for Deployment rollouts. It is not monitoring
  readiness.
- `/health` reports optional monitor degradation and safe reason codes.
- `/status` (opt-in, `healthCheck.statusEndpoint`) shows open incidents.

`/health` reasons come from a fixed vocabulary; raw errors stay in the logs.
The ones an operator meets most often are `component_stalled` (a required
component stopped reporting progress), `component_failed` and
`component_stopped`, `permission_denied` and `optional_permission_denied`,
`api_unavailable` and `optional_api_unavailable`, `cache_sync_pending`,
`cache_sync_timeout` and `cache_sync_failed`, `watcher_failed`,
`persistence_restore_failed`, `storage_reset`, `storage_over_cap`,
`heartbeat_failed`, `config_overlay_invalid`, and, in the `coverage` object, `disabled_by_config`,
`sync_timeout` and `budget_exceeded`. The
[configuration reference](./configuration.md#-health-reasons) lists all of
them with their meaning.

An absent optional Kubernetes API is reported as degraded and does not create a
synthetic incident. `/readyz` succeeds only when the Pod holds the Lease, has
claimed the state file, every source finished its initial list, and delivery is
running. Readiness is withdrawn again while a required source (Pods, Nodes) is
unavailable, for example after its permission is revoked, and returns when it
recovers; this does not restart the Pod. A source counts as listed only once
kwatch has processed every object of its initial list.

## Shutdown and recovery

Kwatch gives its components a bounded shutdown window. On termination
readiness drops at once, the sources stop with the engine, the final incident
snapshot is written, queued alerts are sent (or dead-lettered once the drain
deadline passes, keeping their outbox records for the next start), provider thread IDs from those last sends are saved, and the
state file is closed. If the Lease was lost instead, nothing more is sent:
queued alerts are dead-lettered, because this Pod no longer holds the Lease
and must not send; their outbox records stay, so the next session sends them. If a dependency does not stop within its deadline,
Kwatch records a shutdown timeout and exits rather than waiting forever.

The deployment provides a 60-second termination grace period. A required
component failure (delivery or the pipeline) or an internal stall makes the
Pod unready immediately, before any shutdown work, then cancels the active
session and lets Kubernetes restart it. The heartbeat monitor pings only
while monitoring is ready, so an external dead man's switch notices a Pod
that is up but not monitoring.
Optional component failures remain visible as bounded degraded health states and use
bounded restart backoff; they do not create synthetic incidents.

After a restart the pipeline restores open incidents from the state file, so
they are not announced again, and waits ten minutes before it may recover a
restored incident so detectors can re-raise their findings. Once every source
has finished its initial list, Kwatch compares the saved object fingerprints with
the live cluster and records changes made while it was down, so root-cause
analysis can still find a rollout that happened during the gap. Kubernetes
Events that expired while Kwatch was unavailable cannot be reconstructed.

The state file is created with owner-only permissions and can be removed to
reset Kwatch: it then starts cold and sends one startup summary. Store failures are
reported through health diagnostics.

## Storage

The state file `/var/lib/kwatch/state.db` holds one bucket per data class:
incidents, changes, baselines, evidence, timeline, object
fingerprints, lifecycle state, provider thread IDs, the delivery outbox, and
the flight recording.
Every write,
including compaction, first checks the store's claim epoch, so a process that
lost the Lease cannot write or delete anything.

**Schema changes reset the store.** There are no migrations. When the file
has another schema version (older or newer) or bbolt cannot read it, Kwatch
renames it to `state.db.corrupt.<UTC time>` (the newest two copies are kept,
older ones are deleted; a copy larger than a quarter of the volume limit is
deleted at once), creates a fresh store, and continues: it starts cold and
sends one startup summary. After a schema change (not for an unreadable
file) the delivery outbox and the thread IDs are copied into the fresh file,
so queued pages and resolves are still sent and chat and ticket threads
continue. Their records carry their own version: a record in a layout the new
release does not know is dropped and counted by the outbox drop metric.
Everything else is rebuilt from the cluster, so open incidents are
re-announced once. The log line `state store reset` shows `carriedRecords`. A locked or otherwise unknown open error does not reset; startup
fails instead. The event is logged with the old
version and reason and appears on `/health` as the degraded `state-store`
component with reason `storage_reset`; it does not affect readiness. The
`kwatch_storage_resets_total` metric counts it by reason (`schema_mismatch`,
`unreadable`).
A file locked by another process or on an inaccessible volume is not reset;
startup fails and is retried by Kubernetes.

A single record that cannot be decoded is skipped, counted
(`kwatch_storage_corrupt_records_total`), and logged once per bucket per
start. It never stops startup. An undecodable outbox record can never be sent,
so it is deleted at start and logged with its key.

**Retention.** A background compactor runs every 15 minutes in the active
session, off the decision path, in transactions of at most 500 entries. It
stops within one transaction on shutdown or loss of the state lock.

| Data | Retention |
| --- | --- |
| Changes | 7 days after the entry time, and at most 8 MiB |
| Timeline | 7 days after the entry time, and at most 32 MiB |
| Flight recording | 48 hours, and at most 64 MiB (see "Capturing an incident") |
| Evidence | 30 days, and at most 128 MiB |
| Resolved incidents | 7 days after resolution |
| Baselines | Dropped 7 days after their workload is gone (checked every 10 minutes) |
| Delivery outbox | Until a provider accepts the job; at most 2048 jobs, none older than 24 hours at restore |
| Open incidents, fingerprints, state, threads | No expiry |

**Size cap.** The logical size of the store (the bytes of keys and values) is
capped at 512 MiB by default; evidence may use at most 128 MiB of it. Over the
cap, the compactor deletes the oldest entries first across evidence,
timeline, changes, and resolved incidents, but always keeps at least a quarter
of the cap for history. Open incidents, baselines, fingerprints, state,
threads and the outbox are never evicted; if they alone keep the store over
the cap, `/health` shows the degraded `state-store-size` component with reason
`storage_over_cap`, and monitoring continues. Retention and caps are fixed
defaults in this release. Retention deletes are counted by
`kwatch_storage_expired_total` and size-cap deletes by
`kwatch_storage_evicted_total`.

**Physical size.** Every compactor pass logs `state compaction pass` with
`fileBytes`, `freeBytes` (free pages inside the file), `logicalBytes` and the
live bytes of the timeline, evidence and incidents, and publishes the file
size and free pages as `kwatch_storage_file_bytes` and
`kwatch_storage_free_bytes`. When free pages are at least 64 MiB and half of
the file, the same pass rewrites the file while running: it copies the live
data to `state.db.compact` and renames it over the original, which pauses
writes (never the decision loop) for about a second per 50 MiB of live data
and is counted by `kwatch_storage_rewrites_total`. It needs the same free
space as the startup copy and is skipped without it. A file left by an older
release is trimmed and rewritten by the first pass after the upgrade. At open
the whole file is read once, front to back, before its pages are checked, so
a slow network volume is not read page by page at random.

bbolt reuses freed space but never shrinks its file, so the
file is larger than the live data, and the volume fills with file bytes, not
logical bytes. The cap therefore also applies to the file: when the file is
clearly past the cap, the compactor lowers its logical target by the excess so
the freed pages are reused and the file stops growing. When the file is more
than a quarter past the cap at startup, Kwatch copies the live data into
`state.db.compact` and renames it over the original before it claims the
store. The copy runs only when the volume has free space for the live data
plus a 64 MiB margin; otherwise it is skipped and logged, and the original file
is used. If the copy fails, the original file is also used unchanged. No
backup of the old file is kept. The default 2Gi volume holds the file, which
is rewritten once it is more than a quarter past the cap, and the temporary
copy.

**emptyDir volumes.** The kubelet evicts a Pod whose `emptyDir` passes its
`sizeLimit`, and the file system reports the node's disk, not that limit. With
`persistence.emptyDir=true` the chart sets `sizeLimit` from
`persistence.emptyDirSizeLimit` (default 2Gi) and passes the same value as
`KWATCH_VOLUME_LIMIT`, so the free-space check for the startup copy uses the
real limit. An `emptyDir` loses all state when the Pod is rescheduled; use it
only for evaluation.

## Capturing an incident

Kwatch always keeps a flight recording: the observations its sources hand
the pipeline (what the scenario library replays), sanitized when written,
in the `flight` bucket of the state file. It is a ring: batches older than
48 hours go, and past 64 MiB the compactor drops the oldest first, so a
busy cluster keeps fewer hours, never more bytes. There is no setting; it
is part of the state file's 512 MiB logical cap and only ever evicted
first. A repeat of an unchanged state is skipped (and recorded again every
10 minutes), and batches are compressed, so a typical cluster keeps the
whole 48 hours in far less than the cap. Recording never delays the
pipeline: a full queue (8192 entries) drops the oldest and a failed write
is dropped, both logged at `-v=2`. Memory is bounded by that queue and
20,000 remembered states, about 10 MiB at most.

**What is in it.** Names, namespaces, UIDs, actors, GitOps applications,
event sources, images and label values are salted hash pseudonyms
(`pod-` style names such as `name-468f1de69a`), the same everywhere, so
owners, selectors and rollouts still line up. Reason codes, numbers,
booleans and times are kept. Messages, log lines, error text, change
causes and anything with spaces are dropped, so a recording has no quoted
log or event text; detectors that read a crash log line or an event
message see nothing in a replay of it. A Secret keeps only numbers,
booleans and times; its key names, types and digests are hashed. The salt
is random, created once and kept in the state file; it is never exported.
The state file itself still holds real names (incidents, changes), so
treat it as before; the exported file is the one to share.

**Export a window.** The state file is locked while kwatch runs, so the
command reads a copy of it, taken next to the output file (it needs free
space for it, up to the file size, and removes it after):

```sh
kubectl exec -n kwatch deploy/kwatch -- \
  kwatch replay-export --since 2h --out /var/lib/kwatch/incident.jsonl
kubectl cp kwatch/<pod>:/var/lib/kwatch/incident.jsonl ./incident.jsonl
```

`--since` and `--until` take a duration back from now (`2h`, `30m`) or an
RFC 3339 time; `--state` names another state file (default
`$KWATCH_DATA_DIR/state.db`). The log starts at `--since`, with the latest
state of every object seen before it written first, so a replay begins
from the cluster as it was. A copy taken while kwatch writes can fail to
open; run the command again. The export is the log format of
`internal/replay` and the scenarios, nothing else.

**Turn it into a scenario.**

1. Copy the file to `internal/scenarios/testdata/recorded/<name>.jsonl`.
2. Write `<name>.expect.json` next to it, before replaying it, in the
   format of `docs/contributing-scenario.md`: the `root` that a person who
   knows the cluster would blame, written with the pseudonyms the log uses
   (look the entity up in the log: `grep -m1 '"kind":"deployment"'`), its
   `tier`, a `maxMessages` budget and `mustNotBlame`.
3. Run `make alert-quality`. The case appears under "Recorded scenarios",
   scored and never tuned against: a miss is reported, not fixed by
   changing rules to fit the file. The recorded accuracy gate is
   informational until ten recorded cases exist.

**Cause self-check.** When an incident resolves, kwatch notes whether
recovery followed a change to the cause it named (the cause entity changed
or recovered before the members recovered), a change to something else
nearby, or no change. Counts are kept per rule for 7 days (at most 48
rules, a few kilobytes) in the state file, and `/status` shows, per rule,
"cause confirmed by the fix in 3 of 4 resolved incidents (last 7 days)"
(`causeChecks` in the JSON). It is display only: it never changes a
confidence, a tier or a message. Incidents superseded by a revised cause
are not counted.

## Delivery guarantee

Delivery is at least once, not exactly once. A delivery is written to the
persisted outbox when a provider queue accepts it, and removed when the
provider (or its fallback) accepted it or it was dead-lettered. After a
restart or crash the next session re-queues what is left, oldest first,
before any new work. The outbox keeps at most 2048 jobs and drops a job older
than 24 hours at restore, so a message is not lost across a restart within
those bounds. A send that was interrupted mid-request may repeat after the
restart, because Kwatch cannot know whether the provider received it. If the
outbox cannot be read at startup, delivery still works but without crash
protection, and `/health` shows the degraded `delivery-outbox` component with
reason `persistence_restore_failed`. Watch
`kwatch_delivery_outbox_dropped_total`,
`kwatch_delivery_outbox_write_failures_total` and
`kwatch_delivery_dropped_total`.

Startup summaries and plain notices (startup, upgrade, test) are information:
paging providers (PagerDuty, Opsgenie, Squadcast, GoAlert, Zenduty,
incident.io, iLert, SIGNL4) and issue trackers (GitHub, GitLab, Gitea, Jira,
ClickUp) do not receive them, because nothing would close the alert or issue
they open. Splunk On-Call, Alerta, Sensu Go, Datadog, New Relic, SNS and
Splunk HEC receive them as informational.

## After a crash

The state file is a transactional database, so a kill (SIGKILL, OOM, node
loss) at any moment leaves a file that opens at the next start. What is
guaranteed, and tested by killing a real process in the middle of each write:

- **Incident records, change store, digest state.** Every write is one
  transaction: it is there whole or not at all. A write that finished before
  the kill is there. Open incidents come back open; at most the last moment
  of changes before the kill is lost.
- **Online file rewrite.** The rewrite copies the live data to
  `<state file>.compact` and renames the copy over the file. A kill during
  the copy leaves the original whole. The leftover copy is never read: the
  next rewrite deletes it and starts again. A kill after the rename leaves
  the new, whole file.
- **Messages.** A crash between a provider accepting a message and the outbox
  forgetting it sends that message again once at the next start, with the
  same conversation key and alert key, so a thread or a paging alert takes it
  as the same one. It is not marked as a repeat, and a second restart sends
  nothing. A message queued but not yet sent is sent after the restart.

A file that cannot be read at all is replaced by an empty one (see
[Storage](#storage)); that is a reset, not a crash outcome.

## Security and data sent out

Kwatch serves no diagnostic endpoints and writes nothing to the cluster except
its own Lease and SelfSubjectAccessReviews for its permission audit (not
persisted). Secret values are only hashed, and evidence is redacted before
it is stored or rendered. Provider credentials come from mounted files, never
from plain configuration.

**Adoption telemetry is on by default.** An official build sends one request
to `https://api.kwatch.dev/v1/telemetry/heartbeat` shortly after startup and
then at most once every 7 days. The body is exactly
`{"cluster_uuid": "<installation ID>", "kwatch_version": "<version>"}`.
The installation ID is a UUID derived from a salted SHA-256 hash of the
`kube-system` namespace UID (a random UUID if that cannot be read) and kept in
the state file; the UID itself is never sent. No cluster name, resource, configuration, log line or
incident is sent. Failures are retried after 1, 5, 15, 30 and then 60 minutes,
each attempt times out after 3 seconds, and a failure never affects
monitoring. The request goes through the configured outbound client
(`app.proxyURL`, `app.caBundlePath`), so egress policies and proxies apply to
it like any other outbound call. To turn it off, set `telemetry.enabled:
false` or the environment variable `KWATCH_TELEMETRY` to `false`; development
builds and CI environments never send it. Startup logs one line naming the
endpoint and whether telemetry is enabled or skipped, and
`kwatch_telemetry_*` metrics count attempts, successes, retries and failures.
Air-gapped clusters should disable it, or the attempts will fail and retry.

## Outages and capacity

Kubernetes API and provider outages are handled through bounded retries and
degraded status. Delivery queues are bounded; sustained saturation coalesces or
summarises notifications and can drop them, so operators should alert on queue saturation, terminal
delivery failures, and dead letters.

## RBAC mode and watch coverage

The chart value `rbac.mode` selects the cluster read access:

- `full` (default, and the raw `deploy/deploy.yaml` manifest) grants
  `list` and `watch` on every resource in every API group, so the dynamic
  source can watch any built-in or custom kind, including CRDs installed
  later. `get` is granted only on namespaces, nodes, `nodes/stats`,
  `nodes/metrics` and `pods/log`; a wildcard `get` would also match `pods/exec` and similar
  subresources. Secret values are covered by the grant but are hashed in
  the informer transform and never stored. The only other grants are the
  audit's `selfsubjectaccessreviews` create, the `/readyz` and `/metrics`
  non-resource URL reads, `get` on `nodes` (restart evidence and kubelet
  addresses), `get` on the scheduler and controller-manager Leases in
  `kube-system`, and in kwatch's own namespace the Lease writes and `get`
  on pods (restart evidence for the previous kwatch Pod).
- `least-privilege` grants an explicit `list`/`watch` set derived from
  `kube.SourceAccess()` (typed sources, audited dynamic kinds, CRDs,
  Leases, ControllerRevisions and RBAC objects) plus the optional Gateway
  API and volume snapshot groups. Kinds outside that set are not watched:
  they appear as permission-denied in `/health` coverage, and detectors that
  need them stay silent. Custom resources are not covered. Both modes can
  read Secrets (hashed) unless `watch.secrets` is false.

Set the chart value `watch.secrets: false` (or `watch.secrets: false` in
the configuration) to run without any Secret access. The chart then grants
no Secret permission in either mode; because RBAC cannot remove one
resource from a wildcard, `rbac.mode=full` uses the explicit
least-privilege list in that case and custom kinds outside it are not
watched. kwatch starts no Secret watch, `/health` coverage lists `secrets`
with the reason `disabled_by_config` without degrading health, and checks
that need Secrets (missing Secret references, certificate expiry) report
that they cannot verify instead of guessing. The chart also passes
`KWATCH_WATCH_SECRETS=false`, so the setting holds when `config.yaml` comes
from your own Secret.

In both modes the state lock Role grants `get` and `update` only on
kwatch's own Lease (`resourceNames`); `create` cannot be limited by name in
Kubernetes RBAC and is granted on Leases in kwatch's namespace.

Architecture tests keep the least-privilege rules equal to `SourceAccess()`
and assert that `full` is read-only.

`/health` publishes a bounded `coverage` object: watches by mode (`full`,
`hashed`, `status`, `metadata`), the number of kinds skipped by the watch
budget, the unavailable count, at most 50 unavailable kinds with a reason
(`permission_denied`, `api_unavailable`, `sync_timeout`,
`budget_exceeded`, `object_cap_reached`, `disabled_by_config`), a `reasons`
count per reason, and whether the last discovery was complete. Dynamic kinds
that are skipped, unavailable, or over their object cap are listed too.

A dynamic kind over its object cap stays watched, but objects beyond the cap
are cached only as a small stub (name, namespace, UID, resource version), so
informer memory stays bounded. Stubs are never observed; when room frees up,
the object's next update brings it in in full. The Event informer lists only
`type=Warning` events, the only Events kwatch uses.
Coverage never affects readiness. `CoverageCatalog()` lists every built-in
kind of Kubernetes v1.35/v1.36, the Gateway API and snapshot groups with the
mode the plan must assign, and a test fails if a kind is dropped.

Size CPU, memory, worker counts, queue capacity, and resync intervals for the
cluster size and enabled monitors. The chart defaults are suitable for small
installations only; large clusters require load testing before increasing
worker counts.

## Release verification

Release evidence includes a source mapping, image digest, checksums, and image
signature/provenance. Verify the release assets before upgrading and deploy the
image by digest when the platform supports it. Do not reuse a release tag for a
different image.

Kind and live-cluster validation belongs to CI or an operational milestone. A
local unit-test pass does not prove RBAC, API outage, provider outage, or
upgrade recovery behavior.

The `E2E` workflow (`.github/workflows/e2e.yml`) runs nightly on `main`, on
manual dispatch, and on pull requests labelled `e2e`. Its operational job runs
a disposable Kind cluster for each combination of `rbac.mode` (`full`,
`least-privilege`) and `watch.secrets` (`true`, `false`). It verifies chart
installation, effective ServiceAccount permissions, health probes and
metrics, Lease handover, restart, Helm upgrade and rollback retention, a Pod
event burst, and recovery after restarting the disposable control-plane node.
Its scenarios job runs the semantic suite under `test/e2e`. An API-only
outage, a real provider outage, and large-cluster capacity still require an
operational environment because Kind cannot reproduce every production
network and scale condition.

For a local run with Docker, Kind, kubectl, and Helm installed:

```sh
docker build -t kwatch:ci .
kind create cluster --name kwatch-ops
KWATCH_RBAC_MODE=least-privilege KWATCH_WATCH_SECRETS=false \
  KWATCH_LOAD_COUNT=100 make verify-operational
```

The script loads the image into the cluster named by `KIND_CLUSTER_NAME`
(default `kwatch-ops`).

### CI and release gates

| Workflow | When | What it gates |
|:--|:--|:--|
| `CI` (`ci.yml`) | every pull request and push to `main` | `make ci`; the image build, `version --json` smoke test, Trivy scan and cross-compile; the Kind CRD lifecycle when `deploy/` or `api/` change |
| `Security and supply chain` (`security.yml`) | pull requests, `main`, weekly | `govulncheck` and module verification; SBOMs on `main` and weekly; a weekly Trivy re-scan of the latest released image digest |
| `E2E` (`e2e.yml`) | nightly, `e2e` label, dispatch | Kind scenarios and the operational matrix |
| `CodeQL Advanced` (`codeql.yml`) | pull requests and `main` (workflows), weekly (Go) | static analysis |
| `OpenSSF Scorecard` (`scorecard.yml`) | `main`, weekly | repository security posture |

Required checks for pull requests to `main` are `Verify`, `Image`,
`Chart lifecycle`, `Go dependency scan` and `Analyze (actions)`.
`Image` and `Chart lifecycle` are skipped when their inputs did not
change, and a skipped job counts as passing. The ruleset is kept in
`.github/rulesets/main-branch.json`; apply it under Settings → Rules →
Rulesets → New ruleset → Import a ruleset, or with
`gh api -X POST repos/OWNER/REPO/rulesets --input
.github/rulesets/main-branch.json`. Remove the old `Check` required
status check when you apply it.

The `Release` workflow refuses to tag a commit unless `CI` and `Security and
supply chain` succeeded on `main` for that exact commit. Stable and patch
releases also need a full `E2E` run (run name `E2E full`, the nightly run or
an unfiltered dispatch) on that commit; an rc pre-release skips E2E, so the
E2E gate applies when the rc is promoted to stable.
`Publish` then builds the release image, scans it with Trivy before anything
is pushed, pushes the multi-platform image, moves `latest` for stable
releases, and signs and attests the digest.

Trivy fails on HIGH and CRITICAL findings. Unfixed findings in base OS
packages are ignored; Go module findings are not. Any other exception goes
in `.trivyignore` with an expiry, following
[vulnerability exceptions](./vulnerability-exceptions.md).
