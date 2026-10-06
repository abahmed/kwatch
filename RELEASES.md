# Releasing kwatch

## Unreleased

### Highlights

- **Memory is easier to read, and the last slow growers are capped.** The
  self-health log line now shows `heapLiveMiB` (what the last collection
  kept, the number that tells a leak from garbage) and `heapGoalMiB`, and a
  new `self-health-model` line shows the model's sizes. A simulated day
  (`TestSoakHeapPlateaus`) shows the heap flat. The small growers found
  were capped: overflow-summary reasons, an incident's remembered cause
  findings, and the baseline sampler's warning counters.
- **A new core.** kwatch now builds an understanding of the whole cluster
  (workloads, nodes, volumes, networking, configuration) and keeps it current.
  A root-cause engine ranks the likely cause of each incident, and symptoms of
  the same cause are folded into one incident.
- **Narrative messages.** Every notification is one readable message: what is
  wrong, where, the likely cause, the impact and what to try next. The cluster
  name is included in every message.
- **Formatted messages.** Messages are short lines with the resource
  names in bold, pod text in code and the command in a code block, written
  in each provider's own markup (Slack mrkdwn, Markdown, Telegram HTML,
  Jira wiki); digests and roll-ups are scannable lists. SMS, push and pager
  providers get the same lines as plain text, and webhook payloads keep
  the plain `note` and add `markdown`.
- **Wording follows confidence.** A high-confidence cause is stated ("X is
  down because Y"), a likely one says "likely because", a possible one says
  it might be related, and two causes that score almost alike are told as
  "two possible causes". A closing "Checked:" line lists up to three
  comparisons that ruled causes in or out.
- **Smaller operational surface.** One state file, a short list of endpoints
  and a smaller metric set.
- **Lease terminology.** Logs and docs call the Lease a state lock, not leader
  election: kwatch runs one replica and the Lease only guards its state volume.
- **Unschedulable pods, quantified.** When the scheduler reports
  "Insufficient cpu" or "Insufficient memory", the finding carries what the pod
  needs and the most any schedulable node has free.
- **Unschedulable pods, node by node.** For a pod no node takes, kwatch checks
  each node against the pod's CPU, memory, ephemeral storage, pod count and
  extended resources, taints and tolerations, node selector and required node
  affinity, volume topology, and (best effort) pod affinity, anti-affinity and
  topology spread. The message names the best node of each node pool and what
  stops it, and states a change that would fit, for example "tolerating
  gpu=true:NoSchedule would fit it on gpu-1". It reads the cluster
  autoscaler's and Karpenter's events (`TriggeredScaleUp`, `NotTriggerScaleUp`,
  `Nominated`) and quotes them: a pod the autoscaler is adding a node for is
  held for up to 15 minutes, one it says it cannot help is reported with its
  words. kwatch now also watches those three Normal events.
- **A crash that names a Service is linked to it.** When a crashing pod's
  error quotes a Service of the cluster ("dial tcp redis:6379: connection
  refused"), kwatch links the pod to that Service. If the Service has no ready
  endpoints, or its workload is failing or was just changed (scaled to zero),
  the incident is rooted there, and the message quotes the caller's line and
  says since when the Service has had none. Pods also record the Services their
  environment names, so the dependency graph reaches from Ingress and routes to
  Service, EndpointSlice, pod and the Services it calls.
- **A NetworkPolicy that blocks a dependency is named.** kwatch checks the
  policies against the calls a failing pod makes to a Service (selectors,
  namespaces, ports, address ranges and default-deny). A policy created or
  edited just before that blocks such a call is the root: "orders in shop
  can't reach postgres:5432: network policy deny-all (created 10:05 by bob)
  blocks the call". Anything kwatch cannot read, such as a named port, counts
  as allowed, so only a denial that can be shown is reported.
- **A cascade is one incident.** A failure that travels, such as postgres
  OOM-killed, then the API that calls it not ready, then the Service in front
  of the API with no endpoints, is one incident rooted at the first failure.
  Each step needs evidence (an error naming the Service, a Service with no
  ready endpoint, pods that are not ready and call it) and must begin after
  the one before it, and a step that is healthy ends the chain. A chain
  follows at most three steps from the cause, trusting each a tenth less; the
  message ends with "Chain: postgres (10:00) → api (10:01) → service api
  (10:01)." and names any failure past the limit, which is reported on its
  own.
- **A zero quota that refuses pods is named.** A ResourceQuota that allows
  none of a resource stays quiet until a controller is refused by that very
  quota; then it is reported and is the root of the missing pods.
- **Metrics API failures grouped.** Autoscalers in two or more workloads that
  cannot read resource metrics are one incident rooted at the metrics API, or
  at the failing metrics-server Deployment behind it; one autoscaler with a bad
  target stays its own incident.
- **Deprecated API calls are reported.** When the API server has seen a request
  for an API version a Kubernetes release will remove (for example
  `policy/v1beta1` poddisruptionbudgets), the digest lists it with the release
  that removes it, the next upgrade when it is close, and "calls fail" when
  the cluster is already past it. The metric is per API, not per caller, so
  the message says where to look; it clears when the API server restarts.
- **Control-plane health from the API server's own metrics.** kwatch reads
  how long the API server's writes and reads took since its last round (99th
  percentile, from the histogram) and reports writes slower than one second
  for five minutes as a digest line naming the slowest verb and resource, and
  the admission webhook that is slow, if one is. A webhook that takes seconds
  per call is reported on its own configuration and is the root of the slow
  API; one that fails over a fifth of its calls while failing closed is a
  warning. Requests refused by priority and fairness, an etcd database near
  its default 2 GiB limit and one resource with over 100,000 stored objects
  are reported too. Rounds that reach another API server process, or one that
  restarted, give no number.
- **Node flaps stay quiet.** A node that goes NotReady and Ready again within
  two minutes keeps its reported episode instead of starting a new one, and
  NotReady blips shorter than the threshold never add up to a finding.
- **Smarter messages.** An incident with no cause now lists the latest
  changes in its namespace ("In the last 30 minutes in shop: bob changed
  config map app-config at 21:10"); a fix attempt while an incident is open
  is one thread update ("Rollout 15 of api started at 21:30 (alice);
  watching."), with one "Still failing 10 minutes after rollout 15." and a
  resolve that reads "Fixed by rollout 15 (alice) after 18 minutes."; a
  recurrence says how it ended last time; roll-ups and digests list
  traffic-losing and larger incidents first.
- **Fewer repeated stories.** Re-blaming the same failures on another object
  of one workload (pod, Service, Deployment) is not a "cause revised"
  update, and chronic flappers hold their resolve longer.
- **Resolves reach only providers that heard the announcement.** An incident
  that only a digest, roll-up or startup summary carried (for example one
  "superseded by revised cause") no longer sends a resolve to paging and
  issue-tracker providers that never opened an alert for it; chat still gets
  ordinary resolves.
- **Pages once per outage, reminds at 6 hours.** An open page-tier incident is
  said again once after 6 hours as a thread update (then weekly). A page that
  returns within 2 hours of resolving does not page again (see "A returning
  page is the same incident").
- **Chronic incidents are not forgotten.** An incident named in a roll-up or
  the startup summary gets its own update when the failing pods triple, and
  open incidents that wait for the digest or a roll-up are listed again daily
  with how long they have failed.
- **Recurring noise after a resolve is reported.** A digest-tier incident that
  recurs and resolves before its digest goes out is listed as resolved; periodic
  noise is listed at least once a day.
- **Digest audit entries carry content.** The digest's audit entry holds the
  opened, resolved and risk counts and the first 20 incidents as "id: title".
- **A returning page is the same incident.** A page-tier failure that comes
  back within 2 hours of resolving re-opens its own incident instead of
  starting a new one: same Slack thread, same paging alert key, one update
  ("api is failing again: 4th time in two hours."), no new page, and the
  update goes to chat only; the first page stays the one page of that outage.
  After 2 hours it is a fresh incident again.
- **Recurring unusual events are reported again.** A Warning event kwatch has
  no detector for that comes back after its finding cleared is reported again,
  also when each sighting is a new Event with a count of one, and its finding
  clears when the last event ages out of the 15-minute window.
- **Node-replaced fixes are stricter.** A node incident counts as "node
  replaced" only when the node had a real failure (not just a drain) and the
  node that joined shares its node pool or zone.
- **Custom kinds named as declared.** Custom resources keep the spelling
  their CRD declares ("DatadogAgent datadog") without a global registry; the
  spelling is an entity attribute.
- **Roll-up members reply under the roll-up.** With a Slack bot token, an
  incident announced inside a roll-up message posts its updates and resolve in
  that message's thread, and the roll-up is edited to resolved (✅) once every
  member has resolved. The mapping is saved with the other thread state.
- **Shared crash errors group.** Three or more workloads whose last
  termination message is the same error, once addresses, ports, IDs, numbers
  and timestamps are normalised, and whose failures began within 30 minutes of
  each other, are one incident. The message quotes one line verbatim and says
  how many workloads share it. Bare lines such as "Error", "exit status 1" or
  "Killed" never group.
- **Workloads that run but never become ready.** A Deployment or StatefulSet
  whose pods run without restarting and stay unready for over ten minutes is
  reported with its ready count, how long, and the kubelet's readiness probe
  failure quoted. It notifies when no replica is ready and goes to the digest
  when some are.
- **Autoscalers pointing at nothing.** An HPA whose target Deployment,
  StatefulSet or ReplicaSet does not exist is a digest finding ("HPA
  istio-system/istiod targets Deployment istiod, which does not exist.").
- **Kubelets kwatch cannot reach.** A Ready node whose kubelet stats read
  failed three or more times in six hours is a digest finding saying its node
  metrics are missing.
- **OOM kills say what the container used.** kwatch keeps each container's
  memory peak of the last 24 hours and the run before its last restart, and
  an OOM message says "It was killed at its 512Mi memory limit; it used 610Mi
  at peak in the last 24 hours", "Its memory rose steadily from 200Mi to
  512Mi over three hours before the kill" or "It hit its 256Mi memory limit
  within 30 seconds of starting", with the step "Raise the memory limit above
  the observed peak, or find what uses the memory". Without usage history
  nothing is added.
- **Jobs that run far longer than usual.** A CronJob's Job that has run more
  than 30 minutes and over three times the longest of its last three or more
  successful runs notifies ("It has run 2h10m; recent runs took 8-12m"),
  unless its activeDeadlineSeconds ends it first.
- **Scaled to zero while still routed.** A Deployment or StatefulSet at zero
  replicas that a Service behind an Ingress, an HTTPRoute, a LoadBalancer or
  a NodePort still serves notifies, naming who scaled it (the field manager,
  as recorded) and when. A workload an autoscaler may scale to zero
  (minReplicas 0) is left alone.
- **First rollouts say they never worked.** A Deployment with one ReplicaSet,
  never available since it was created, leads with "api in shop has never
  become healthy since it was created 12 minutes ago."
- **Dependency probes say how they failed.** A failed dependency probe now
  says "did not answer within 3 seconds", "refused the connection" or "name
  does not resolve". When every one of three or more probed dependencies
  fails in the same round, kwatch reports one digest finding ("kwatch could
  not reach any of its 5 probed dependencies; its own network may be
  restricted") instead of blaming each of them.
- **Workloads are judged against their own normal.** kwatch learns, per
  workload, how often it restarts, how long its pods take to become ready,
  how much memory it peaks at and which Warning events it gets, over the
  last seven days. Restarts from a workload that never restarts say so
  ("restarts 12×/h vs a usual 0.1×/h"), and a worker that always restarts
  twice an hour goes to the digest with that shown. Crash loops and
  not-ready workloads are never excused by history.
- **Pages need user impact.** An incident pages only when a Service users
  reach has no ready backend, the last replica of a user-facing workload is
  down, or a cluster-critical component is down; a failing internal-only
  workload notifies.
- **Healthy rollouts stay quiet.** While a Deployment or StatefulSet
  rollout is progressing inside its deadline, the transient "replicas not
  ready" finding is held; a rollout that stalls or crashes is reported at
  once.
- **Readable digest lines.** Autoscalers with a missing target, kubelets kwatch
  cannot reach and workloads whose pods run but never become ready read as
  plain clauses in the digest, and never-ready system workloads are listed.
- **Quieter watch budget.** The skipped resource types are logged once and
  again only when the list changes, and the default type budget is 300 (was
  200) so common CRDs fit on a mid-size cluster.
- **Namespace outages in one message.** When five or more workloads of one
  namespace (or at least half of it, three minimum) fail within ten minutes
  with no shared cause, kwatch sends one message ("shop: 12 of 15 workloads
  failing since 10:02") with the worst first and what they share, such as a
  node or a recent change. Each workload's updates and its resolve thread
  under it, and it is paged once at most.
- **Crash causes from the previous log.** A container killed by its liveness
  probe leaves an empty termination message, so its pods could not be grouped
  by cause. kwatch now reads the first error line of the previous run's log
  (about 50 lines, once per restart, at most 20 reads per round, credentials
  redacted) and quotes it, so pods that crash on the same line become one
  incident. It needs the `pods/log` permission kwatch already uses.
- **A CronJob's failed last run.** A CronJob whose latest run failed and has
  not succeeded since is reported in the digest ("Last run failed 3 days ago
  (BackoffLimitExceeded); the next run is at ..."), and clears when a later
  run succeeds or the failed Job is deleted. A failed run stays a warning for
  its first day and turns informational after that. Suspended CronJobs are
  skipped.
- **Quiet node scale-ups.** Node memory overcommit and pressure-stall
  advisories wait until a node is 10 minutes old, and a Service or admission
  webhook without ready endpoints waits while every not-ready pod behind it is
  still starting (on a booting pool, or younger than 10 minutes, with no
  restarts). Anything still wrong after that is reported at once; crash-looping
  pods get no grace. The node "pod limits exceed capacity" advisory waits the
  same way.
- **Webhooks that cannot block go to the digest.** An admission webhook whose
  backend is missing or has no ready pods is an informational digest item when
  every webhook in its configuration has failurePolicy Ignore (requests skip
  it); with Fail, it still notifies because it blocks matching requests.
- **Roomier default CPU limit and probe timeouts.** The default CPU limit is
  500m (request stays 100m) and the liveness and readiness probes wait 3
  seconds, so an event storm no longer fails kwatch's own readiness check.
- **One log line per delivery.** Every provider send logs `provider send` with
  the provider, incident key, kind, placement (root, thread or edit), ok or
  error, and the retry count; never the message body or a URL.
- **Persistent crash loops are not routine.** A crash loop on a workload that
  also fails at the same time every day is no longer kept in the digest once it
  outlasts the boot window; it is announced like any other. An incident that
  reaches only the digest no longer counts as covering a workload that is down.
- **Pinned autoscalers are not maxed out.** An autoscaler whose minimum equals
  its maximum is no longer reported as wanting more replicas.
- **Metrics blips stay quiet.** An autoscaler must fail to read metrics for ten
  minutes before it is reported, so a metrics-server restart opens nothing.
- **Fix attempts are not lost.** A rollout or config edit seen while a held
  material change, a pending revision or a reopen update ended the tick was
  never reported; it is now recorded when seen and sent on the first later
  tick that has nothing earlier to say.
- **Reopens stay in the Slack thread.** A resolve that may reopen keeps its
  thread for 2 hours; a "failing again" update replies in it and the root goes
  back to the failing status, instead of posting a new top-level message. The
  kept thread is saved with the other thread state.
- **Quiet incidents are not "lost".** A digest or silent incident now covers
  its workload, so the coverage backstop no longer hands it back every 30
  minutes; only a digest incident that is crash-looping or has nothing ready
  past the boot window, and should already have been raised, is handed back.
- **Fewer false "scaled to 0" alerts.** A Deployment at zero replicas is
  reported only while its Service has no ready endpoints at all, so a
  blue/green twin behind the same selector stays quiet; an autoscaler that
  reports `ScalingActive=False` with reason `ScalingDisabled` also counts as a
  deliberate zero.
- **Crash logs reach every container.** A container whose previous log is
  gone is marked as read; other read errors back off for a few rounds, and
  each round starts where the last one stopped, so a few failing containers
  cannot use up the read budget.
- **Fix: error signatures.** Error signatures treat numbered names as one
  error across replicas (worker7 and worker3 both read worker#) but keep the
  name, and named hosts keep their service port, so a Postgres refusal on
  db.example.com:5432 stays apart from Redis on another host. HTTP status
  codes, IPv6 addresses and pod-name hashes are normalised more precisely.
- **Kubelet reachability.** "Cannot reach the kubelet" needs failures spread
  over at least 30 minutes and clears as soon as the kubelet answers again.
- **Network advisory no longer hides outages.** "Kwatch's own network may be
  restricted" is shown only when no pod using the probed dependencies is
  failing; otherwise each dependency is reported.
- **Smaller fixes.** DNS failures other than "no such host" read "DNS lookup
  failed"; a liveness-kill loop needs a probe event near the last kill; a full
  Slack thread map evicts threads kept for a reopen before open ones.
- **Autoscaler-safe pod.** The manifest and the chart's default
  `podAnnotations` carry `karpenter.sh/do-not-disrupt` and
  `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"` (plain
  annotations, inert without such an autoscaler).
- **Self-health log.** Every 30 minutes kwatch logs its Go heap, goroutines
  and the sizes of its bounded in-memory maps.
- **Holds close their paging alerts.** A page that the startup summary held
  and that resolved inside the window now closes its paging alert, and an
  outage-hold announcement released as "not an outage" is no longer held (a
  restart does not announce it again) nor sent to the pagers a second time.
- **Bounded "still broken" hold.** A recovered incident whose workload stays
  short of replicas with a failing pod resolves after 2 hours at most.
- **Only what was heard can reopen.** An announcement that was held and
  dropped, or out of scope, no longer reads "failing again"; a digest-listed
  incident that returns within 2 hours is the same incident, listed as
  recurring in the next digest.
- **Fewer stray updates.** A resolve (also a quiet supersede) drops an owed
  "cause revised", a quiet supersede closes the listings it was in, a restart
  no longer adds a flap cycle, and the restored-incident listing skips
  incidents that already spoke for themselves.
- **A restored incident never ends as healthy while it still fails.** When
  its failures are now explained by another incident (a Service with no
  endpoints, now blamed on its crashing Deployment), it closes as "moved to
  the incident for deployment X" in its old thread, and its alert closes with
  it. A Service whose workload is still short and crashing is held like one.
- **Dropped digest decisions say why.** The audit entry of a decision the
  digest leaves out carries `deliveryNote` ("digest-only: ...").
- **Fallbacks follow paging rules.** A fallback provider no longer takes a
  paging-only message or a resolve that skips paging.
- **CronJob time zones work.** The binary embeds the IANA zone database, so a
  CronJob with `spec.timeZone` is no longer reported as "UnknownTimeZone".
- **Credential redaction covers more shapes.** Authorization headers of any
  scheme, escaped JSON keys, Kubernetes env name/value pairs, Slack and
  Discord webhook paths, Telegram bot tokens, `user:pass@tcp(host)` DSNs,
  `apikey <value>`, and Google OAuth, npm, SendGrid and AWS secret keys.
- **The state file is never deleted silently.** A schema mismatch or a
  structurally corrupt file is renamed to `state.db.corrupt` (one copy) before
  a fresh store starts; a locked or unknown error fails startup instead.
- **Paging providers skip informational messages.** Splunk On-Call, Alerta,
  Sensu Go, New Relic and Datadog no longer send the startup summary or plain
  notices, and Splunk On-Call no longer logs its routing key.
- **Discord rate limits reach delivery.** The Discord client no longer sleeps
  through a 429 on its own, so delivery's rate limiting handles it.
- **Opsgenie and issue trackers are sturdier.** An Opsgenie update right after
  the create retries a brief 404; closing an issue that was deleted forgets it;
  log output can no longer break out of an issue's code fence.
- **Kubelet verification warning.** With `kubelet.insecureSkipVerify: true`,
  kwatch logs a startup warning that a compromised node could capture its
  ServiceAccount token.
- **Audit log and election hardening.** A failed audit-file reopen after
  rotation is bounded and retried; a late leader-election event can no longer
  overwrite the leader status.
- **Stale usage readings age out.** Volume, filesystem, pressure and memory
  readings from a kubelet are cleared when a pod or volume leaves the node's
  summary, and a node's readings are cleared after three failed polls in a
  row, so a deleted pod's volume no longer stays "nearly full".
- **Warning events belong to one object.** Events carry the UID of the object
  they are about. A re-created StatefulSet pod no longer inherits the old
  pod's FailedMount, and a pod re-created under the same name during a watch
  gap is treated as deleted and created again. Events of one reason from
  separate Event objects add their counts.
- **Resyncs are skipped.** A periodic informer resync of an unchanged object
  produces no observations; the kube informers' memory also drops the
  kubectl last-applied copy.
- **Opt-in dependency probes are safer.** Probed external dependencies are
  not dialed when their name resolves to a loopback, link-local, private or
  carrier-grade address ("blocked: private address", not a failure), and an
  endpoint no pod calls any more is retired after three rounds so its
  incident resolves.
- **Lease scan is complete.** Leases are read in pages instead of the first
  500, and a deleted Lease is removed after a complete scan.
- **Bounded condition names.** Custom resource condition types and reasons are
  redacted and cut to 64 and 128 bytes. The Secret informer skips Helm
  release Secrets (type `helm.sh/release.v1`).
- **Fewer false findings from normal change.** Image drift waits out a
  rolling update and five minutes after the newest build started; a missing
  Secret or ConfigMap is ignored for finished and deleting pods and is only a
  warning ("will fail on its next restart") for a pod that runs and is ready;
  a new Ingress gets 10 minutes for its TLS Secret and 2 minutes for its
  backend Service; a VolumeAttachment error must last 2 minutes; memory near
  its limit and node error rates must last a few minutes; an expired
  certificate is critical only when something references it; a full
  ResourceQuota is informational until a create is refused for exceeding it;
  a Lease is stale only after 5 minutes; a not-ready container's readiness
  finding no longer flaps when the kubelet slows its events.
- **Better counts.** CronJob repeated failure no longer counts skipped
  schedule slots (Forbid, missed runs); node memory overcommit leaves out init
  containers and is reported once, not twice; "no memory limit" counts
  containers per template, not per replica; a pinned autoscaler without
  minReplicas is not "maxed out".
- **Removed the port-mismatch check.** EndpointSlices copy a numeric
  targetPort verbatim, so the check could only fire during propagation lag.
- **Findings survive a blind kubelet.** Disk, inode and volume usage findings
  are held while the node's kubelet cannot be read, so an incident does not
  resolve as healthy while the disk may still be full.
- **A late crash-log line regroups the incident.** A new error line on a
  finding is a change, so incidents are grouped again by what the log says.
- **Plain cordon wording.** A cordon (and its automatic unschedulable taint)
  is "cordoned", not "being removed"; a failed scale-target read is a target
  problem, not a metrics one.
- **Shared-cause grouping and wording.** A shared error is found by counting
  workloads, not pods; webhook refusals and "no endpoints" are no longer
  called timeouts; rollback hints use the Deployment's own revision history;
  plain-word notes say "ingresses", not "ingresss".

- **No cause from a shared factor alone.** A candidate cause that only
  shares a factor with the failures (and is neither unhealthy nor changed)
  no longer explains them.
- **Kubelet Node events count again.** Warning events the kubelet posts about
  a Node (SystemOOM, EvictionThresholdMet, ImageGCFailed) and about static
  pods were dropped as "about another object"; they now reach the node and pod
  rules. A Warning event without a count counts as one.
- **Node readings survive one failed poll.** A failed kubelet read no longer
  wipes the node's disk, inode, pressure and network readings at once, nor
  right after the kubelet answers again; optional metrics endpoints get the
  same three-poll grace.
- **OOM evidence kept while a container crash-loops.** The 24 hour memory peak
  and previous-run figures stay on a container that has left the kubelet
  summary, so the "used 610Mi at peak" advice survives the back-off.
- **Stale crash-log line cleared.** The "last error line" of an earlier run is
  removed when the next run has no previous log, or the container last exited
  cleanly.
- **Fewer wrong readings and probes.** Moved pods and volumes keep their
  reading when the old node polls; readings of nodes that left the cluster are
  removed; API server and DNS rates are only computed between samples of the
  same process or pod; automatic Service probing dials the first TCP port,
  skips ExternalName Services and honours `kwatch.io/skip-probe: "true"`;
  scheduler reasons keep dots ("Insufficient nvidia.com/gpu"); a deleting pod
  is no longer described as a config edit; change sets stop growing at 15
  minutes.
- **Config and chart schema match the binary.** `resyncSeconds` must be 0 or at
  least 30, `app.logFormatter` text or json, and the heartbeat and active-probe
  numbers cannot be negative, in the chart's `values.schema.json` and in the
  `KwatchConfig` CRD, so a bad value fails `helm install` or the `kubectl
  apply` instead of crash-looping the Pod. A `KwatchConfig` can no longer
  carry `healthCheck`, which could disable or move the health server the
  probes depend on.
- **Safer chart defaults.** The NetworkPolicy opens the cluster DNS metrics
  port, refuses an empty `apiServerPorts` (which would open all egress) and has
  `allowProbeEgressAll` for active probes; `helm install` warns when probes or a
  proxy need egress the policy blocks. The `kwatchconfigs` list/watch grant is a
  namespaced Role. A second release no longer fails on the shared CRD.
- **Safer scripts and workflows.** The Kind scripts refuse to run unless the
  kubectl context is a `kind-*` cluster and use their own kubeconfig for
  diagnostics; artifact redaction replaces unsafe lines instead of deleting
  files; the release workflow bumps `main` only after the contents verify and
  the tag exists.
- **Configuration checks.** `kwatch lint` and startup validate message
  templates, runbook URLs and `fallback` names. `$${NAME}` keeps a literal
  `${NAME}` in config values. `SKIP_UPGRADE_CHECK` is parsed like the other
  boolean variables, `CI=false` no longer counts as CI, and an unreadable
  `KWATCH_MEMORY_LIMIT` fails startup.
- **Delivery follows the conversation.** A resolve and every later message
  of an incident go to the providers that received its announcement, even for
  providers routed by reason or namespace. Startup summaries, roll-ups,
  outage messages and digests reach a routed provider when one of the
  problems they name matches. Route reasons compare ignoring case, and
  paging-only or skip-paging jobs held before start or during a
  reconfiguration are now filtered like live ones.
- **Pages are not folded.** Page-tier messages and the resolves of paged
  incidents are exempt from the hourly budget. A chat fallback no longer
  swallows a pager's resolve, and a pager is no fallback for notices or
  summaries. Resolves and pages go ahead of routine jobs in a provider queue.
- **Backoff.** A 429 without `Retry-After` backs off exponentially with
  jitter, and `Retry-After` on a 503 is honoured. Provider HTTP failures are
  a typed `transport.StatusError`; Opsgenie and the issue trackers detect a
  404 by status instead of by message text.
- **Redaction gaps closed.** camelCase keys (`authToken`, `refreshToken`,
  `apiSecret`, `signingKey` and the like), npm `_authToken=`, URL passwords
  with an empty user (`redis://:pass@host`) or containing `?` or `#` are
  redacted. Quoted pod text in messages loses credentials only; private
  addresses stay.
- **Message wording and limits.** Quoted pod text is no longer rewritten
  (kind spelling, tense, plurals); an incident that ended at the
  still-broken cap no longer claims health; a few empty-phrase and
  "1 workloads" slips are fixed. ntfy, Webex, Zulip, Google Chat, Matrix
  and others get a message size cap that never leaves a code fence open.
- **Delivery housekeeping.** Delivery logs scrub URLs everywhere, the
  outbox writer keeps writing removals while delivery drains, and an overflow
  summary a provider rejects for good is dropped once instead of re-queued.
- **Pages survive restarts and holds.** A page held in the startup summary
  or an outage hold that is restored after a restart and recovers before
  it settles again now closes its pager alert (a paging-only resolve). A
  held incident that rises to the page tier is paged then, and a roll-up
  no longer pages an incident an outage hold already paged.
- **Quiet supersedes leave nothing behind.** An incident resolved without
  a message no longer leaves held announcements in the startup summary,
  digest, outage hold or investigation hold, so no thread opens for it
  later.
- **Restart accuracy.** Boot noise after a long gap (a nightly scale-down)
  no longer escalates a restored digest incident: the boot window runs
  from the restart. The worst stage reached, the first announcement's route
  (`AnnouncedRoute`), the pending digest lines and the roll-ups of an
  unfinished cold start are kept across restarts. A digest-listed incident
  that later escalates is introduced in full, and a roll-up closes even when
  a member's resolve reached nobody.
- **Engine hardening.** Held announcements that are released are saved at
  once, the cold-start window end wakes the loop, an investigator that
  panics or ignores its budget no longer takes down or pins the pool, model
  fingerprints are built only when the store writes them, and an open
  incident keeps its investigation evidence.
- **Scorecard and replay.** The scorecard counts the messages people
  received, not the member lines a digest or roll-up carries. The high
  calibration level promises 80%, as the method demands. Replay logs hide
  the change cause and the UID and origin of notes. The audit log no longer
  repeats a failing rotation on every entry.

- **Review fixes.** Smaller corrections and tuning found in review:
  - Fix: early kubelet events for static pods are kept when the pod first
    appears; notes tied to an old object are dropped when its UID changes.
  - Fix: PIN, pincode and passcode values, and a bare `key=` with a
    credential-looking value, are redacted.
  - Fix: a Datadog site with stray spaces is trimmed, and one containing
    `@` is rejected by lint and by the provider.
  - Fix: a recurring incident whose opening page is lost is no longer
    counted as paged because of the previous run's alert.
  - Fix: an error line that differs only by addresses, ids or counts on
    each restart no longer re-announces the finding as changed.
  - Fix: the own-network check counts only recent restarts or not-ready
    pods as failing callers, not lifetime restarts.
  - Fix: kubelet memory history is no longer republished for containers
    that have left the cluster.
  - Fix: image drift is hidden only during an unfinished rollout, not by
    unavailable replicas.
  - Change: a disk or inode finding held while the kubelet is unreachable
    ends after 30 minutes and says when its reading went stale.
  - Change: the Lease staleness limit follows the Lease scan period.
  - Fix: headless Services are no longer probed by automatic Service
    probing.
  - Fix: a change linking two change sets no longer merges them past the
    15-minute span cap.
  - Change: a StatefulSet rollout held at a partition is reported only when
    its pods are failing.
  - Change: node memory overcommit needs 5 minutes at 150% to be raised and
    holds until the share falls below 140%.
  - Change: container memory-high uses the kubelet's RSS when available and
    says whether it measured RSS or working set.
  - Fix: docs/kubernetes-coverage.md lists only the detectors that raise
    each failure mode.
  - Fix: the route recorded for an incident widens with every update that
    is sent, so a resolve after a restart still reaches a pager route
    matched only by a later update.
  - Fix: a restored held page whose pager alert is open is re-announced to
    chat only; it no longer pages again.
  - Fix: config keys that count tokens (`maxTokens=4096`, `numTokens`,
    `max_tokens`) with a number of up to seven digits are no longer redacted;
    password, secret, key and credential keys, and `authToken`-style keys
    with a longer number, still are.
  - Fix: a stuck investigator can leak at most four goroutines per kind;
    further jobs for that kind are skipped until one returns.
  - Fix: an incident is no longer treated as paged when every pager
    permanently rejects its messages.
  - Fix: in a full delivery queue a resolve never displaces a page
    announcement; lost resolves are logged at error and counted in
    `kwatch_delivery_resolves_lost_total`.
  - Fix: a resolve for a tracker issue with no mapping logs a warning.
  - Fix: a new incident no longer shares the alert key of a live, revised
    incident on the same root and mode.
  - Fix: a pre-upgrade held incident record restores as not yet told, and a
    material-change update held for its investigation survives a restart.
  - Fix: the noise scorecard counts a decision once when it reached both
    pagers and chat.
  - Docs: a page to a down pager is deliberately not redirected to chat.
  - Fix: a lost update to a pager clears the paged flag no more; only a lost
    opening message does, so a restart cannot leave an open alert unclosed.
  - Fix: a Jira or ClickUp issue whose resolve only commented (no close
    setting) stays mapped, so a recurrence comments on it, not a duplicate.
  - Fix: an Ingress is judged again when its IngressClass appears or goes
    away, so a class listed after the Ingress no longer leaves a false
    `IngressClassMissing` open.
  - Logs: opening the state file reports its size, schema version and how
    long it took; any startup step slower than five seconds (page check,
    rewrite, lease check, claim) is logged by name, and so is the number of
    incidents restored.
  - Fix: a workload that has been at zero replicas for a day or more (by the
    API server's record of its latest spec write, or the scale-down kwatch
    saw) and is still routed to goes to the digest, not as a notification.
  - Fix: an event about a Deployment, StatefulSet, DaemonSet or Pod that is
    fully ready now, and was ready before the event was written or before
    kwatch first saw it ready, no longer opens an incident, so replayed
    events from before a restart do not announce a recovered failure.
  - Fix: a known workload whose pods have not been ready since before this
    run started escalates at once instead of waiting a boot window after
    every restart of kwatch.
  - Fix: the audit entry of a roll-up, startup summary or digest has the
    action `summary`, no root, and lists the incidents it carries.
  - Fix: an object judged while a kind it needs is not synced yet (a webhook
    whose Service list had not arrived, a PodDisruptionBudget before the
    pods) is judged again when the kind syncs. A webhook calling a Service
    that does not exist was left unreported after a start, so restored
    incidents about it resolved as healthy while the problem remained.
  - Fix: a VolumeAttachment that is being deleted now says so, so a detach
    that never finishes is found; one whose node and PV are gone reads
    "is stuck deleting; its node and PV no longer exist" and goes to the
    digest, with only the node gone it stays a warning.
  - A CronJob with three or more scheduled runs since its last success (or
    since it was created) is listed in the digest as `CronJobNoRecentSuccess`:
    "Has not succeeded since 2024-05-20 (29 scheduled runs)". Suspended and
    repeatedly failing CronJobs keep their own findings.
  - Root cause now compares a failure with healthy twins elsewhere, and
    keeps what it compared as "what was checked": the same image running
    fine in another workload, a ConfigMap or Secret that healthy workloads
    use too, the other replicas of the workload (failing on one node while
    the rest run elsewhere, or failing on every node), the healthy pods of
    other workloads on the node, and a previous revision that ran healthy
    before the change. Each is a small bounded weight; none decides alone.
- **What a change touched, and what the new revision changed.** Every
  recorded change now carries a field-level summary for workloads (image, env
  values, command, probes, resources, volumes), config keys (names only for
  Secrets), Services, Ingresses and routes, autoscalers, budgets, network
  policies and node labels, with the actor and time; values are redacted and
  bounded. When only a new revision fails, the message names what it changed
  from the healthy one, likeliest culprit first ("Only change in revision 14:
  memory limit 512Mi → 256Mi (by alice); pods OOMKilled since."), and every
  message lists the other nearby changes made before the failure, ranked by
  closeness to the failing object (itself, the config it uses, its Service or
  Ingress, its node, then its namespace).

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
- **Configurations that used to start and now fail.** `kwatch lint` and
  startup report these as errors:
  - a Go template in `templates` or `alert.<provider>.templates` that does not
    parse (it used to be skipped and logged);
  - a `runbooks` value that is not an absolute `http` or `https` URL;
  - `alert.<provider>.fallback` naming a provider that is not configured;
  - `alert.datadog.site` that is not a bare host name (`datadoghq.com`,
    `us3.datadoghq.com`; no scheme, path or port), where the provider used to
    fail to start without a clear error;
  - a `KWATCH_MEMORY_LIMIT` that is not a positive size;
  - `KWATCH_WATCH_SECRETS`, `KWATCH_CRD_ENABLED` and `KWATCH_TELEMETRY` with a
    value other than a boolean (`true`/`false`, `1`/`0`, `on`/`off`,
    `yes`/`no`). `SKIP_UPGRADE_CHECK` is parsed the same way, but an invalid
    value is logged and ignored.
- **`$${NAME}` is a literal.** In configuration values `$${NAME}` now yields
  the text `${NAME}`. Before, it produced `$` followed by the value of
  `NAME`. A password that holds a `$` right before a `${NAME}` reference must
  be rewritten.
- **`KwatchConfig` cannot carry `healthCheck`.** An overlay with
  `healthCheck.enabled` or `healthCheck.port` is rejected and logged as
  `config_overlay_invalid`; the mounted configuration keeps running.
- **Chart and CRD schema are stricter.** `helm install` and `kubectl apply`
  reject `resyncSeconds` from 1 to 29, an `app.logFormatter` other than `text`
  or `json`, negative heartbeat or active-probe numbers, and an empty
  `networkPolicy.apiServerPorts`.

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
  `state.db` is renamed to `state.db.corrupt` (one copy; a later reset
  replaces it) and a fresh store is created (reported as `storage_reset` on
  `/health`). Open incidents are announced once again as new, and a startup
  summary is sent. Delete `state.db.corrupt` when you no longer need it; kwatch
  removes it at once when it is larger than a quarter of the volume limit.
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

- **Pushover receipts and priority.** Only the announcement of a page-tier
  incident can use `priority: 2` (emergency); a lower tier is sent at 1.
  Updates, summaries, digests, startup and plain operator messages are normal
  priority (before, an operator message could be an emergency). The receipt
  of an emergency is saved with the thread state. A resolve cancels that
  receipt first and only then sends the normal-priority "resolved" push, so a
  failed cancel is retried without a second "resolved".
- **ntfy publishes to the server root.** kwatch posts JSON to the server URL
  with the topic in the body, as ntfy documents. `alert.ntfy.url` must be the
  server address and the topic goes in `alert.ntfy.topic`.
- **LINE Notify is removed.** LINE shut the service down on 31 March 2025,
  so the provider could never deliver. An `alert.line` section no longer
  fails startup: kwatch ignores it and logs a warning. Remove it and use
  another provider.
- **ClickUp `closeStatus`.** New and optional. Set it to a status of the list
  (for example `complete`) and a resolve moves the task there after its
  comment. Task descriptions are sent as Markdown. `reopenStatus` moves a
  closed task back when the incident fails again.
- **Jira `closeTransition` is opt-in.** Unset (the default), a resolve only
  comments, as before. Set it (for example `Done`) to also move the issue
  through that workflow transition; a project without it is logged and left
  as it is. `reopenTransition` reopens the issue when the incident fails
  again. Jira and ClickUp reopen only when both the close and the reopen name
  are set; otherwise the mapping is forgotten at resolve and a recurrence
  opens a new issue, as before. Jira wiki markup in the Note is escaped
  without touching JSON-like braces and brackets.
- **Datadog cluster tag and `site`.** Every event now carries
  `cluster:<clusterName>` unless `tags` already has a `cluster:` tag. `site`
  must be a bare host name (`datadoghq.com`, `datadoghq.eu`,
  `us3.datadoghq.com`); anything else fails validation.
- **GitHub rate limits are retried.** A 403 whose message says the primary or
  secondary rate limit was hit is retried with backoff (a wait named in the
  message is honoured, up to 15 minutes) instead of being dropped.
- **Structured receivers see more.** The custom webhook, n8n, Zapier and
  Splunk providers now also receive the closes of paging-only incidents
  (`resolved: true` for a key they may never have seen) and the JSON gains
  optional fields (`opens`, `pagingOnly`, `skipPaging`, `carrier`,
  `reopenWithinSeconds`). Existing keys are unchanged, so tolerant parsers
  keep working; a receiver with a strict schema or one that deduplicates on
  keys must accept them.
- **Issue mapping format.** Issue trackers save a resolved issue as its id, a
  separator and the end of its reopen window. After a rollback to an older
  version the old code reads that as an issue id, gets a 404 from the tracker
  and forgets it, so it heals itself with at most one failed call per issue.
- **Slack threads through the reopen window.** A resolved incident keeps its
  Slack thread for 2 hours, so a recurrence replies in it. Roots saved by
  older versions have no recorded root marker; kwatch never edits those, and
  only new roots are edited to show the status.
- **Other provider fixes.** SMTP 552 (mailbox full) is retried like a 4xx
  reply; Alerta escapes the cluster name in its resource; Matrix breaks
  `@user:server` mentions as well as `@room`.

#### Configuration

- **Invalid environment booleans fail startup.** `KWATCH_WATCH_SECRETS`,
  `KWATCH_CRD_ENABLED` and `KWATCH_TELEMETRY` accept `true`/`false`, `1`/`0`,
  `on`/`off` and `yes`/`no`; any other value (for example `flase`) is an error.
- **`resyncSeconds` and `app.logFormatter` are validated.** `resyncSeconds`
  must be `0` or at least 30, and `app.logFormatter` must be `text` or `json`.
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
- **`kwatchconfigs` RBAC is a namespaced Role.** The list/watch grant on
  `kwatchconfigs` moved from the ClusterRole to a Role in kwatch's own
  namespace, because the overlay is read from there only. Custom RBAC that
  relied on the cluster-wide grant needs the same change.
- **`crd.install`.** New chart value (default `true`). The `KwatchConfig` CRD
  is shared and Helm keeps it on uninstall, so a second release no longer
  fails on it: it leaves a CRD owned by another release alone. Set
  `crd.install=false` on all releases but one, or when you apply
  `deploy/crd.yaml` yourself. `config.crd.enabled` still turns the live
  overlay on or off.
- **Pod defaults.** The default `podAnnotations` carry
  `karpenter.sh/do-not-disrupt: "true"` and
  `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"`; setting
  `podAnnotations` replaces them, so repeat them if you want to keep them.
  The default CPU limit is 500m (request 100m) and the liveness and readiness
  probes wait 3 seconds before timing out.
- **NetworkPolicy additions.** The opt-in policy opens the cluster DNS metrics
  port (`networkPolicy.dnsMetricsPort`, default 9153), and
  `networkPolicy.allowProbeEgressAll` (default `false`) admits all egress for
  active probes. `helm install` warns when probes or a proxy need egress the
  policy blocks.
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
  release now requires green CI and security runs on the tagged commit,
  plus a full E2E run for stable and patch releases (an rc skips E2E),
  and the image is scanned before it is pushed.
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
- **A node that failing workloads share is suspected.** When three or more
  workloads on one node start failing within ten minutes, most pods on that
  node fail, and nothing upstream shows a fault, the node is named as a
  possible cause, never above "possibly", and any cause with evidence of its
  own outranks it. An unchanged shared image, ConfigMap, Secret or
  ServiceAccount is no longer suspected: that is weak evidence, and bad
  builds are covered by rollout, image digest drift and the shared failure
  signature.
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
  annotation of a blamed change is quoted: `recorded as "bump api to
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
  gets one update a week: "api in shop is still down, for two weeks now."
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
- **Message wording and noise.** A configuration risk never leads a
  message, changes its tier or counts as a new failure, and CPU throttling
  is always low priority ("api in shop is throttled on CPU 75% of the time").
  A failure that only blames itself no longer says "because it is failing on
  its own". The digest lists configuration risks by type, with up to three
  example workloads each, and skips `kube-system`, `kube-public`,
  `kube-node-lease` and kwatch's own namespace. A stale Lease is reported
  only while its holder pod is Running and Ready, so it no longer repeats a
  crash loop. Custom resources keep their declared kind spelling
  ("DatadogAgent datadog").
- **Liveness kills read as crash loops.** A container whose last exit is
  SIGTERM (143), with a recent liveness `Unhealthy` event and at least three
  restarts, is reported as `LivenessKilled` (mode `CrashLoop.Liveness`): "keeps
  being killed by its liveness probe". Every crash-loop row matches it,
  including the probe port mismatch cause. One kill followed by a healthy
  container stays quiet.
- **Webhook denial versus outage.** An admission webhook that answers and
  denies a request is a policy rejection (`Webhook.Denied`): it blames the
  webhook configuration only for the workload it denied and notifies, even
  when the webhook fails closed. A webhook that times out or cannot be called
  still pages. A denial reason that mentions a timeout is no longer read as a
  failed call.
- **Quiet scale-up.** A node pool is booting while at least half of its
  nodes are under ten minutes old. Pods on it that are pending, creating or
  not ready, workload unavailability caused only by such pods, readiness and
  startup probe failures, and sandbox or network-not-ready events are held
  back until the boot is over, then reported normally if still failing. Crash
  loops, OOM kills, image pull errors and configuration errors are never held
  back. A booting pool or zone is not blamed for its young nodes failing.
- **Release watch.** For 15 minutes after a Deployment rollout, kwatch compares
  the new revision's container restarts per pod-hour with the workload's
  rate before the rollout. A new revision that restarts at least three times
  as often, with at least three restarts, is reported as a release regression
  on the Deployment. A first deploy, a rollout past the window and a revision
  already crash-looping are not reported. There is nothing to configure.

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
- [ ] `CI`, `Security and supply chain` and a full `E2E` run are green on the RC commit (the release workflow enforces E2E when promoting to stable; an rc itself needs only CI and security).
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
