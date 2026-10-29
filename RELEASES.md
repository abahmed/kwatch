# Releasing kwatch

## Unreleased

### Editions

- These leave Community and are now Pro: the `/metrics` endpoint (Community
  answers 404 and logs one line at start), readiness probe failures,
  shared-factor causes ("node X explains N failures"), and the pod reasons
  PodPreempted, PodPreemptedRepeatedly, PodAdmissionRejected,
  PodKilledAtGrace and SchedulingGated (new capability core.pods.advanced).
  Pro is free during early access. Liveness and startup probe failures stay
  in Community.

- Community counts, in memory and only in the cluster, the Pro findings it
  does not show, and reports them in a startup notice and a "Pro checks"
  line of /status. Alerts carry one Pro line chosen by alert type, or a
  teaser when Pro found a likely cause. app.hideProHints hides the alert
  line, the teaser and the startup notice; /status still counts.

### Evidence (Pro)

- kwatch now runs the read-only look a message used to suggest and shows
  the result: pod state, logs (previous and current run, per container),
  events, memory use, node and the pods on it, volume claims, Secrets and
  ConfigMaps (key names only), Service and Ingress chains, webhooks,
  rollouts, control plane readiness, finalizers, images and pull secrets,
  certificates, quotas, autoscalers, Jobs and probes. Every applicable
  check runs in parallel when an incident opens and on re-investigation,
  under a 10-second budget with at most four API reads at once.
- A Pro message keeps only the steps that change the cluster; a read-only
  step stays when its read was not allowed or timed out. When the gathered
  evidence proves the cause, the message says so and the confidence is
  "confirmed". When a provider's length limit cuts a message, it cuts the
  proof and keeps the fix whole.
- The dashboard's issue page has an Evidence tab with one section per
  check, a log viewer, the related objects and the correlation (the
  failure signature and what it points to, the nearest change, the spread
  and what a rollout changed). `GET /api/incidents/{id}` adds `evidence`
  and `correlation`. The report is kept in memory only, not in the state
  file.
- New settings: `investigation.logs` (default true; false reads no log)
  and the Helm value `rbac.readyz` (default true). Memory and CPU use come
  from the kubelet stats kwatch already reads (nodes/stats), so no add-on
  such as metrics-server is needed. A read that is not allowed shows as
  such and never fails the investigation. Audit entries add `evidence`
  and `readSteps`.
- A Pro message for a workload whose container was killed at its memory
  limit suggests a new limit: half again above the observed peak (or the
  limit, when higher), rounded up to 64Mi, as a step that changes the
  cluster.
- Teams cards and email lay a Pro message out as cause, what kwatch found,
  the log quote and the fix; email adds an HTML part next to the plain
  text. The webhook, n8n and Zapier payloads add an `evidence` object
  (confirmed, checks, facts, quote, fix); every other field is unchanged.
- When a provider's limit cuts a Pro message, the log quote loses lines
  first, then the proof, then the quote; the cause and the fix stay.
- The dashboard shows when each check read its section.
- Community messages are unchanged, except on Discord (below).

### Cluster DNS

- The cluster DNS servers are found through the DNS Service
  (`kube-system/kube-dns`, `openshift-dns/dns-default`, or a kube-system
  Service named for CoreDNS or selecting `k8s-app=kube-dns`), then its
  pods and their owners of any kind. OpenShift's DaemonSet, RKE2's
  `rke2-coredns-rke2-coredns` and Helm installs with a release prefix are
  now recognised for DNS metrics and DNS outages; the coredns and kube-dns
  names remain the fallback.

### Delivery

- Discord incidents are an embed again: the headline as the title, the
  cause, proof, log quote and fix in the description, colour by status,
  and kwatch, the cluster and the time in the footer. The embed stays
  within Discord's limits (description 4096, embed 6000) by cutting the
  proof, never the fix, and without one field per section, which broke
  long messages before (#194, #416). Mentions are neutralized, thread_id
  still works, and plain notices stay message content of at most 2000
  characters. This changes Discord output in both editions.

### Detection

- A node that has not been Ready since it joined is now given the ten-minute
  boot window before it is reported as lost, and the report says "has not been
  Ready since it joined N minutes ago". A node that was Ready and then lost
  keeps the 90-second rule. A stale kubelet heartbeat on a new node waits the
  same way.

- A Service that sits behind an Ingress, route, LoadBalancer or NodePort and
  selects labels that no pod or workload template in its namespace carries is
  now reported as soon as it is two minutes old, quoting its selector. A
  workload scaled to zero still counts as a match and stays quiet.

- A Service whose selected pods are all Pending now gets its own "no ready
  backends" finding instead of staying silent.

- A pod that is shutting down on purpose no longer counts as an unready
  Service backend, so a long graceful shutdown no longer reports degraded
  backends.

- The kubelet notices InvalidDiskCapacity, Rebooted, DNSConfigForming and
  CheckLimitsForResolvConf are now informational (digest only), and a kubelet
  note older than the moment its node turned Ready is ignored.

- A Job pod that fails while the Job still has retries left (less than half of
  its backoff limit spent) is no longer announced; it is reported once half
  the retries are gone or the Job fails.

- A container killed for memory is now reported from its third restart instead
  of its fifth.

- Scaled-to-zero, Service backend, webhook backend and init-wait findings now
  wait until EndpointSlices are watched, instead of reading a missing source
  as "no backends".

- Pods the kubelet rejected for AppArmor, an unsupported pod feature,
  pod-level resources or a volume attachment limit are now named as admission
  rejections; pods ended by a node shutdown are no longer reported as
  admission failures.

- A pending kubelet-serving certificate request and an unallocated device
  claim now count their wait from the object's creation time, so a restart of
  kwatch no longer restarts the wait or resolves and reopens the finding.

- Failed pods and terminated containers now date themselves from when the
  container ended and drop to the digest after fifteen minutes; any failed
  Job, not only a CronJob run, drops to the digest after a day.

- Removed reasons and modes that nothing raised (ContainerCannotRun, Killed,
  DeploymentProgressingFalse, DeploymentAvailableFalse,
  StatefulSetConditionFailure, DaemonSetConditionFailure, ResourceHigh,
  ResourceCritical) and the rollout condition checks registered for
  StatefulSets and DaemonSets, which have no such conditions.

- A scaled-to-zero workload that is routed to turns informational exactly one
  day after the scale-down, without waiting for another change.

- NetworkUnavailable on a node younger than the boot window is held back until
  the node is ten minutes old.

- A controller's "cannot create pods" event now stays current for one more
  window while the controller is still short of pods, so the finding no longer
  clears between its backoff retries.

- A routed scale-to-zero finding now dates itself from the cluster's own
  record of the scale-down, and says "since before kwatch started" when it
  cannot be dated.

- A failed pod of a DaemonSet on a node that is leaving (deleting, cordoned,
  tainted for removal, not ready, gone or just joined) is no longer reported.

- A detector panic no longer clears the findings of the entity it was working
  on: that detector's earlier findings are kept while the other detectors'
  fresh results still apply, and the entity is checked again within a minute.

- A custom resource's failing condition now quotes the controller's message.
  Failing=True and Succeeded=False are reported once they last two minutes and
  Ready=Unknown once it lasts five; generic, custom and route/listener
  condition ranking share one table.

- Node conditions that a node problem reporter sets (such as
  ReadonlyFilesystem or KernelDeadlock) now reach the digest after they have
  held for the node-condition grace. An unknown True condition with no message
  stays evidence only.

- A DaemonSet that keeps reporting misscheduled nodes now appears in the
  digest.

- Warning events whose reason is shown by a pod's or node's own state
  (BackOff, Failed, Unhealthy and others) are no longer hidden when a custom
  resource's controller reports the same reason; the endpoint controller's
  failures are no longer hidden either.

- An expired TLS Secret is called unused only when Ingresses and pods are both
  watched, and the wording says "no pod or Ingress mounts or references it";
  otherwise it is reported as expired without a claim about use.

- Findings that hold only inside a time window (a pod preemption, a
  PodDisruptionBudget overlap flag) are checked again when the window ends
  instead of lingering.

- Finding summaries no longer predict ("pods may be evicted", "will fail on
  its next restart", "a finalizer is likely blocking it"); they state what is
  true now, and configuration risks state the configuration only.

- The "exec format error" finding states the node's architecture and quotes
  the error instead of claiming the image has no build for it.

- A webhook configuration that calls two Services with two failure policies is
  now judged per Service: a missing backend behind an Ignore webhook is a
  digest line even when another webhook of the same configuration fails
  closed, and a dead backend behind a fail-closed webhook is no longer hidden
  by an earlier dead one behind an Ignore webhook.

- Events the kubelet records against a container (back-off, probe failures,
  kills, image pull failures) are no longer reported as unusual events on top
  of the failure the container's own state already shows, so a failing pod's
  incident closes when the pod recovers instead of staying open with a "cause
  now known" message about an event kwatch already reads.

- A detector that panics now keeps only its own earlier findings for that
  object: the findings of the other detectors are applied as usual, so one bug
  no longer freezes everything kwatch knows about the object.

- Removed two failure modes that nothing raised (CannotRun, ConditionFailure).

- A DaemonSet with exactly one misscheduled node now says "1 node runs its
  pod" instead of "1 nodes run its pod".

- An expired TLS Secret whose users cannot be told yet (Ingresses or pods not
  yet watched) is now a warning that only says it expired, not a critical
  outage; it is critical only when a pod or Ingress is known to use it.

- A custom resource or other object reporting Ready=Unknown is now reported
  after five minutes (the time a controller gets to observe a change) instead
  of two, unless another failing condition is reported with it.

- A node condition that is True but has no message and is not a type a node
  problem reporter is known to set no longer adds a digest line on its own; it
  stays as evidence inside the incident of a real failure of the node.

### Root cause

- A stuck-deleting finalizer is no longer blamed on a controller that only has
  a similar name. A workload is named as the handler only when the owner of
  the finalizer entry, a label or annotation key, container arguments, a
  ServiceAccount role that writes the API group, or a webhook of the group
  ties it to it. A workload named like the controller is mentioned as
  unconfirmed.

- Messages for a finalizer nothing handles now say "no controller in the
  cluster is known to handle it (API group ...)" instead of "no running
  controller found".

- Stuck-deleting messages quote the held object's own detach error, or the
  latest Warning event on the object, when a controller outside kwatch's view
  says why it cannot finish.

- The controller of a finalizer is searched cluster-wide, including for
  cluster-scoped objects.

- A finalizer handler that appears or changes after the objects were first
  explained now updates the explanation.

- Causes that are not objects kwatch watches, such as a finalizer, now carry
  the time their first failure began, so a late takeover of an older incident
  is refused.

- A rival cause that scores higher than the stated one is now always told as a
  rival.

- Roles and bindings are now read in a compact form (API groups they may
  write, bound ServiceAccounts) instead of metadata only.

- When several workloads in one message have all their replicas failing, each
  "Checked:" comparison now names its workload ("all 6 replicas fail, on 3
  nodes (api)") instead of repeating the same words.

- Fixed: a Secret, ConfigMap or other object that was changed after the pods
  began to fail (a fix attempt) is no longer blamed for the failure. Each
  failure is dated by its own start, a node's own conditions are still exempt,
  and an object edited after the failure is no longer listed as "healthy and
  unchanged".

- Fixed: a RoleBinding or ClusterRoleBinding that was changed
  or deleted is blamed for a "forbidden" error only when it binds the
  ServiceAccount the error names (or the pod's own account); a binding of
  another account is no longer blamed. A deleted binding keeps the accounts it
  bound on its tombstone for this. A binding whose subjects are unknown stays
  a suspect.

- Fixed: a recent change is no longer proof on its own. A Role or RoleBinding
  that was only created is never blamed for a "forbidden" error, and an RBAC
  change is not blamed when the error names a different ServiceAccount than
  the pod's own; a NetworkPolicy is blamed only when the failure is a
  connection error; an edit of a Secret or ConfigMap is capped below "likely"
  when the failures quote a defect of the program's own code (a nil
  dereference, an index out of range) and nothing names the edit; and a defect
  in the program's own code now counts against a config, policy, permission or
  node cause.

- Fixed: a probe that checks a port the container does not declare is blamed
  only when the probe cannot connect (a readiness failure, or a liveness kill
  with a refused connection). An app that crashes for its own reasons is no
  longer told to change a probe port that works.

- Fixed: a workload blamed for its own setting (a probe port, a memory limit,
  a startup budget) no longer gains confidence from its own pods failing, and
  its own name in its own log lines no longer counts as an error that
  "mentions it". Such a cause now reaches "likely" or "high" on its own
  evidence.

- Fixed: a Secret is blamed for a failed image pull only when the pod pulls
  with it (imagePullSecrets) and the registry refused the login. An
  environment Secret edited before a wrong image tag is no longer blamed for
  "manifest unknown".

- Fixed: a Service of type ExternalName named in a connection error is no
  longer called a Service "with no ready endpoints"; it has none by design.

- Fixed: a caller whose own error is a defect of its code (a nil dereference,
  an index out of range) is no longer chained to a failing database or Service
  just because its configuration names it.

- Fixed: a node shared by three failing workloads is no longer suspected when
  their errors are defects of their own code.

- Fixed: an object named like a common word (a ConfigMap called config,
  default or data) is no longer matched by prose that holds the word, and when
  a refusal names one LimitRange of a namespace the other LimitRanges are not
  blamed.

- A failing node agent (a log shipper, a proxy, a network plugin) is no longer
  blamed for unrelated crashes beside it. It is blamed only when most workload
  pods on its node fail together and each failing workload runs a healthy
  replica elsewhere. A storage-driver error ("driver name X not found in the
  list of registered CSI drivers") blames only an agent that is given that
  driver's name, and a DaemonSet that registers the driver but has no pod on
  the node is named as the cause.

- A crash-looping node agent is now blamed from its first crash, without
  waiting three minutes for its pod to turn unready, and the node it runs on
  is no longer a second root for the same fault.

- An external endpoint is no longer called unreachable when kwatch's own probe
  connects to it, or when only the pods on one node fail while the same
  workload calls it fine from other nodes (the node is then the suspect).

- Pods that name no image pull Secret, beside healthy workloads that pull from
  the same registry with one, are now blamed for their own configuration
  instead of the registry.

- A crashing controller is now found as the root of its custom resources
  whatever its field manager is: the workload whose ServiceAccount may update
  the resource's API group (read from Roles and bindings) is the controller,
  and it is recognised as down as soon as its pods crash, not minutes later.
  Gateway API routes are linked to their gateway controller the same way.

- A refused create that says its ServiceAccount was not found now names the
  missing ServiceAccount as the cause, in one incident with the ReplicaSet and
  the Deployment.

- A pull Secret that changed up to a day before a new pod's pull was refused
  (401, 403) is now named as the cause, and the registry's own answer is
  quoted.

- One workload that fills the disk of one node, is evicted, and fills the next
  node is now one incident rooted at the workload, not one incident per node.

- "N of M dependents fail" now counts every dependent of the cause, not a
  sample of 32.

- A pod that is refused with its own ServiceAccount named as the user, where
  no role binding names that account (directly or through a group of
  accounts), is now explained by that: the account is bound to nothing. With
  the bindings not watched, or one changed in the window, the cause stays
  unknown.

- The first error line of a crashed container's previous log now reaches the
  root-cause engine even when the container left a termination message that
  says nothing, so a crash whose log names the Service or endpoint it could
  not reach is explained by it.

- Connection URLs in a workload's arguments or environment no longer leak
  password or user-name fragments into the model or replay fixtures; only the
  host is read.

- An RBAC binding that names every account (or a whole namespace's accounts)
  is now treated as binding the refused account, and a binding whose subject
  list was cut at 32 entries is treated as unknown instead of as binding
  nobody, so the right RBAC change is no longer dropped or falsely cleared.

- A failing node agent is now recognised as the CSI driver's plugin when the
  driver name only appears in the kubelet socket path (registration path,
  socket variable, plugin directory) or in the DaemonSet's arguments.

- A workload that fills the disks of several nodes is no longer described as
  having "failing workloads that all depend on it" or as having "crossed its
  eviction threshold"; the message now says it fills the nodes' disks and pods
  on them are evicted.

- Long Warning event messages keep their end when shortened, so a stuck
  deletion still shows the reason the controller gave.

- A Service named in the configuration is no longer blamed as "missing" when
  the lookup got no answer (EAI_AGAIN, server misbehaving) or cluster DNS is
  failing, so cluster DNS is named instead of a two-cause hedge.

- A workload failing on its own is no longer shown as the rival of a
  shared-node or chain cause, and an unprocessed edit is no longer blamed for
  the controller not reconciling it (apiserver throttling), so those incidents
  blame the right entity.

### Incidents and paging

- **A namespace outage is one message even when the workloads have a
  Service.** Eight crash-looping Deployments that each sit behind a Service
  were announced and paged one by one, because a Service made each blame
  itself. They are now held and named in one namespace message.

- **A page of another team is never held.** Two unrelated user-facing outages
  that began in the same moment were paged once, because the second looked
  like part of the first. A roll-up pages the first page-tier incident of each
  namespace and holds only the others of that namespace.

- **A restart no longer swallows a rise to page.** An incident held by the
  restored-incidents summary, or one whose fingerprint was adopted during the
  restore grace, that rose to the page tier was never paged. The rise is paged
  at once and told.

- **A roll-up no longer says "all N problems resolved" while a problem lives
  on.** An incident that was closed as moved, or whose recurrence was still
  failing, counted as resolved for the roll-up that named it. The roll-up now
  closes only when the incident that took over, or the recurrence of a
  forgotten one, is over too.

- **A node outage names every node it merged.** A merged node outage reached
  the owners and namespaces of one node only. It now reaches the pods on every
  merged node.

- **A zone that fails minutes after its first node is one outage, one page.**
  When the zone or pool fails as a whole, the incident that paged for the
  first node takes the zone as its root and says its cause was revised.
  Before, a second incident was announced and paged, and the first alert was
  closed as moved while the outage went on.

- **An outage message holds the pages it did not send.** The other page-tier
  incidents of a namespace outage kept the page tier, so their reminders paged
  the pagers for an alert nobody opened. They are held at notify, and a held
  page no longer repeats itself as a "material change" a few seconds later.

- **Held updates widen the route of the resolve.** An update that was held
  (startup summary, outage hold) and rose to a page did not record its
  severity and reasons, so the resolve could miss the pager route. It now
  does.

- **A page is not delayed by digest incidents.** Digest-tier incidents
  settling beside a page counted toward a burst, so the page waited 75 seconds
  instead of 15.

- **A flapping page keeps an open alert.** A page that resolved and failed
  again left the pagers with no open alert after the first resolve. When it
  becomes a flapper, or comes back for the third time inside two hours, one
  update goes to the pagers at the page tier and reopens the alert; the
  resolve of a stable recovery closes it. The held page is not released into a
  second page while the alert is open. A flapper that someone acknowledged
  does not page.

- **One incident panicking costs one incident.** A panic while advancing one
  incident, or in one collecting step of the announcer (digest, listings,
  roll-up), dropped every decision of the tick. Each incident and each step is
  contained on its own, counted and logged, and the others still speak.

- **"Failing again" survives a restart.** A reopened flapping incident that
  restarted before its "failing again" update went out never sent it. It is
  sent, and an owed one is not repeated after a flapping update that already
  said it.

- **A forgotten outage leaves no group memory.** The roots a grouped node
  outage merged stayed in memory after the incident was forgotten.

- **A moved roll-up member closes its chat thread.** A roll-up member that
  spoke on its own and was then closed as moved closed for the pagers only,
  leaving its chat thread open. It is closed in chat too; a member that never
  spoke still closes for the pagers alone.

- **A node that recovered while kwatch was down resolves at once.** After a
  restart its incident waited for the restore grace and the hold (13 minutes)
  to resolve, and said it was down for that long. It resolves as soon as the
  sources synced, and its downtime ends when the node became ready.

- **Late failures of an outage join it.** The sixth and seventh workload of a
  namespace outage, failing a minute or two after its message, were announced
  alone. They are held and sent as one more outage message naming the whole
  outage, and are held at notify when the outage paged.

- **A page that moves to a workload says the node recovered.** When a node
  page was re-rooted to a workload because the node came back, the update gave
  no word about the node. It now says "Node n1 has recovered; the workload
  still has open failures". The incident keeps its page tier.

- **The restart summary does not wait ten minutes for loud problems.**
  Restored incidents at notify or page whose findings are back are named one
  settle after the sources synced; the rest, and the quiet ones, are named
  when the restore grace ends.

- **A severity override cannot undo a held repeat page.** An override that
  made a failure critical paged again on a repeat of a page that had just
  resolved. The hold stands.

- **"Moved" waits for the message it points to.** The incidents closed as
  moved said so half a minute before the incident that took over sent its
  "cause revised" update. They close together, the update first.

- **The digest audit entry names every incident.** The audit entry of a digest
  worded 20 incidents and hid the rest. It words 100 and ends with "and N
  more: id, id, ..." naming the others.

- **A flickering failure keeps one incident ID.** A probe that failed for a
  few seconds, over and over, opened a new incident every time. The silent
  incident of the same root is revived under its own ID. The docs now say the
  timeline keeps 7 days or 32 MiB, whichever is reached first.

- **A re-ranked cause does not move an old incident.** An incident announced
  more than ten minutes ago stays rooted where it is when the engine finds a
  new cause that only ties with its root, or one that an alternative of the
  same kind explains as well. Before, a peer that edged ahead every few hours
  moved the incident and told it again each time.

- Fixed a restart that told a rise to the page tier twice: the second,
  identical "material change" update ten minutes later is gone.

- A page that keeps coming back no longer opens a second alert beside the one
  already open for the same outage, and it honours an acknowledgement
  annotation; the once-per-incident rule now survives a restart.

- A node under disk, memory or other pressure that is Ready again after a
  restart no longer resolves early and reopens; only a node that was just not
  ready or had a stale heartbeat resolves at once.

- A late page-tier failure of a namespace outage that already paged is held at
  notify from the first one, so the outage has one alert.

- A quota used up with nothing refused, and CPU throttling while nothing is
  affected, no longer produce a digest line.

-  After a restart, a problem that was announced before and whose findings
  only come back after the restore grace (for example a Deployment whose new
  pod is created minutes after kwatch starts and is held while the cluster
  wakes up) is now named in a restored-problems message when it is back,
  instead of staying silent for days.

### Messages and providers

- Fixed: text a pod or an event wrote is now quoted byte for byte in every
  message. A double quote, a backtick, a run of spaces or a final full stop
  inside a quoted error no longer cuts it short or changes it in Slack,
  Discord, Mattermost, Teams, Telegram, Matrix, Jira and the other rich chats;
  text that a code span cannot hold exactly goes in a code block.

- Fixed: a change to a Deployment, StatefulSet or other pod template no longer
  shows secret values. An environment variable shows its old and new value
  only when its name is on a short list (host, port, url, level, env, region,
  enabled, timeout, replicas, mode, addr) and the value is a plain word; every
  other literal, and every command or argument that follows a flag, says
  "changed". Query strings of probe paths are hidden too.

- Fixed: a Secret or ConfigMap that was edited shortly before the failure is
  no longer said to "not exist"; the message says it "changed shortly before".
  A missing or deleted one still says it does not exist.

- Fixed: durations and semicolons inside quoted text are left exactly as the
  pod or the event wrote them; kwatch only rewrites its own words.

- Fixed: "Fixed by rollout 15 after N minutes" now measures to the moment the
  failures cleared, not to the end of the resolve hold, and every time in a
  message names its zone ("10:03 UTC"), including the change window, the
  serving certificate expiry and "same as Tue 14:02 UTC".

- Fixed: the update that says someone changed the workload, or its config,
  names the workload in its short text, so a provider that shows only the
  short text (an Opsgenie alert title, an SMS) still says what it is about.

- Fixed: redaction now also removes PGPASSWORD-style names, pass= and PASS:,
  curl -u user:password, redis-cli -a, sshpass -p, docker login -p, Azure
  AccountKey and shared access signatures, X-Amz-Signature, X-Goog-Signature,
  session ids, docker config auth values, PGP private key blocks, Stripe
  restricted keys, Grafana, Vault, DigitalOcean and age tokens, a bare token
  before the @ of a web address, passwords that contain commas or semicolons,
  Digest response values and "the password is X" prose.

- Fixed: chat messages no longer delete underscores or stars from names in
  italic or bold text (Slack and Google Chat write such text plain), keep runs
  of spaces in code spans, and carry backticks in code blocks instead of
  replacing them with a look-alike character.

- Changed: private addresses (10.x, 172.16-31.x, 192.168.x, 100.64.0.0/10,
  169.254.0.0/16, fc00::/7, fe80::) now stay visible in log lines as they
  already did in events and termination messages, which is what the evidence
  policy in the docs says and what the owner annotation after an address
  needs.

- Fixed: the Microsoft Teams card shows the incident title as escaped text, so
  a title taken from the cluster cannot become a link or a mention.

- Fixed: ilert sends notify and low tier incidents at priority LOW; only a
  page-tier incident gets the configured priority, so a warning no longer
  pages.

- Fixed: closing an Opsgenie alert that Opsgenie does not know is retried for
  a few seconds and then taken as done, instead of failing for five minutes
  and holding up the alerts behind it.

- Fixed: a failure signature is no longer printed between quotes as lower-case
  normalised words that no pod wrote; the cause now keeps the error one of the
  failing pods wrote (`SharedError`) and the message quotes it as written,
  without credentials.

- Fixed: Telegram messages neutralize @username in text a pod wrote, so a log
  line cannot notify a person or a channel.

- Fixed: redaction treats 100.64.0.0/10 and 169.254.0.0/16 as private
  addresses and redacts "the password is X" prose.

- Fixed: an external endpoint whose probe error says "no such host" is no
  longer described as not accepting connections; it is worded as a name that
  does not resolve, with the error quoted as written.

- Fixed: a fact about a container (memory used against its limit, "its last
  run lasted ...") is no longer said as "It" under a node or Secret lead; the
  message names the container. A fact about a claim no longer drifts onto a
  workload whose quoted crash comes before it ("It is used by ..." now ends
  the usage sentence).

- Fixed: "no memory limit" is only said for memory evictions, a node that was
  only under memory or disk pressure now resolves as "no longer low on
  memory/disk space" instead of "is ready again", and a cause that is the
  workload's own pod ("keeps crashing because its own pod is not ready") is
  dropped.

- Fixed: kubectl `--previous` is only suggested for a container that
  restarted; readiness-only and never-restarted containers get describe pod
  plus current logs. Pages about the cluster DNS, scheduler, controller
  manager, API server and etcd now start with a first step.

- Fixed: affected lists name every namespace when there are several ("a in
  billing; b and c in media"), a network-policy lead names the caller without
  a capital and no longer lists it as affected, the same quoted error is shown
  once, baseline sentences use each figure's own verdict once, and a container
  recovering with its pod is told once.

- Fixed: an outage of several nodes says "They report ..." and "the nodes may
  be down"; its updates say "Nodes n1-n5 are spreading to ..."; a zone's
  updates speak for the zone and never say "Node a1 is spreading to node a1".

- Fixed: a reminder for a condition that fails nothing (quota used up,
  throttled but ready) says "still the case, for one day now" instead of
  "still failing".

- Fixed: the digest closes with "It is not urgent ..." for one problem, says
  nothing when only resolved ones are listed, and says "one more still
  failing"; "kwatch" stays lowercase at the start of a sentence.

- Fixed: "The error mentions it" is only said next to the quoted error;
  otherwise it is left out.

- Fixed: the "Quoted:" and "Quoted readiness failure:" labels are gone.

- Fixed: the rollback hint says what the command does ("Rolling back to
  revision N, the one before this release, undoes it") instead of promising it
  fixes the failure.

- Fixed: a LimitRange change shows only the bounds that changed.

- Fixed: the namespace outage message ends with a full stop.

- Fixed: removed repeated or empty-information sentences: the rival that
  restated the lead, a service listed again after its Service, the last chain
  hop repeated, "The pod-security changed.", a workload named "storage"
  capitalised, one workload named twice in an update.

- Fixed: an update that only raised the tier says "It is now critical."

- Fixed: messages speak of kwatch in the third person ("kwatch can't see ...",
  "kwatch will write again ...").

- Fixed: removed "work in flight was cut off", "so the restarts are a
  symptom", and the load balancer event label now says "failed to deploy"
  (what happened), not "can't deploy" (a claim about the state).

- Fixed: a release whose image cannot be pulled starts with `kubectl describe
  pod` (the events) instead of `rollout status`.

- Fixed: durations the message writes are in words ("ten seconds", "created
  two hours ago", "three minutes after the release", "30-second grace
  period"); a whole rate reads "12 requests a second".

- Fixed: the node controller's own taints are no longer listed as something
  that "changed before it broke".

- Fixed: a resolve no longer counts the time a node's Warning event stayed in
  its window as time the node was failing.

- Fixed: a Service that selects a typo keeps the clause that says
  no pod has those labels ("selects app=apii, but no pod or workload template
  in shop has those labels").

- Fixed (detection-b): the webhook consequence states what was seen ("It fails
  closed (failurePolicy Fail); no request has been rejected so far.") instead
  of forecasting that any create is rejected.

- An edit of a hidden environment variable or command-line value is no longer
  dropped from "Only change in revision N", so a release whose only edit was
  FOO=1 to FOO=2 still says "env FOO changed".

- A config map or Secret key change reads "Key app.yaml changed." instead of
  "changed to changed".

- An environment value that merely starts with "from " is hidden like any
  other literal; references to Secrets and ConfigMaps are told apart by
  origin.

- Command words after "password", "token", "secret", "key" or "auth", long
  random words, and probe path segments that look like tokens are hidden in
  change descriptions.

- A quote mark hidden inside a word can no longer slip a password past the
  redactor, and no message field can carry a stray quote marker.

- Jira shows a quote that contains braces in a {noformat} block, byte for
  byte, instead of dropping the braces.

- "PASS: TestName" Go test output, pass_through=true and proxy_pass=... are no
  longer redacted; a quoted curl -u 'user:pw' / --user "user:pw" and a quoted
  password cut off before its closing quote are now redacted to the end of the
  line.

- An Opsgenie alert whose close is answered 404 is retried in the background
  for up to 5 minutes without holding the provider queue, then given up with
  an info log and kwatch_delivery_resolves_lost_total. EXPECTED GOLDEN CHANGES
  (not regenerated): scenarios whose revision edits include only hidden
  env/arg values now gain an "Only change in revision N: env X changed" line;
  ConfigMap/Secret key-change notes (previously "changed to changed" in stale
  goldens) read "Key K changed."; Jira rendering of brace quotes (no golden);
  redaction goldens containing PASS:/pass_through unlikely.

- Fixed: the configuration risks quoted beside a failure now state facts only
  ("It runs a single replica.", "Its image has no fixed tag.", "Its containers
  have no memory limit, so nothing caps their memory use.", "It has no
  readiness probe, so Kubernetes sends it traffic while the container runs.")
  instead of "so this is downtime" or "what runs may have changed". A test
  scans every sentence the message writers can say for forecasting words, like
  the one the detectors have.

- Fixed: a quotation shown in its own code block no longer leaves a stray line
  that starts with ". Every error mentions it." or "; its recent events
  include ..."; the words after the block start a sentence of their own.

- Fixed: when a cause revision moves an incident from a node to a workload,
  its resolve now describes the workload ("cart in shop is healthy again. It
  was failing for ten minutes.") and counts that workload's own failing time,
  instead of saying the node is ready again after the whole incident's twelve
  minutes. A page for a node no longer makes the workload it moved to "down".

- Fixed: a digest says "still open", not "still failing" ("one more still
  open, unchanged (...)", "+2 more still open", "... — still open since
  17:14"), for ongoing problems that are standing conditions or configuration
  advice; a reminder of an advice-only incident no longer says "still
  failing".

### Runtime and performance

- Fixed a startup slowdown on large clusters: with the flight recorder on, the
  initial sync of a 20,000-pod cluster no longer copies the whole recording
  queue on every event, and the change history no longer copies its ring on
  every change once full.

- Fixed incident acknowledgement lookups, the downtime fingerprint snapshot,
  two boot-grace checks and the namespace-label scope check copying every
  attribute of an object to read one; error-line signatures are now remembered
  (bounded), so the decision loop allocates less per tick.

- Fixed a wall-clock step (NTP correction, resumed VM) being able to fake or
  hide a stalled component: stall detection now uses elapsed monotonic time.

- A panic while trimming an informer object, or in a crash-log, control-plane
  probe, model pruning, flight recorder, storage, history or audit worker, is
  now contained: it is counted (`kwatch_panics_recovered_total`, new
  `background` stage) and logged, and the process keeps running instead of
  restarting. The object that failed to trim is cached as an identity-only
  stub that never holds Secret data, and the resource shows `handler_panics`
  on `/health`.

- Fixed a KwatchConfig deleted while its informer was down staying applied
  until the next restart: kwatch now notices the missing object after the
  informer is rebuilt.

- Fixed `/health` showing `handler_panics` as the generic `component_failed`.

- A required kind that is forbidden or not served now shows on `/health` with
  its reason while kwatch waits to start, instead of staying silent until the
  first sync; readiness is unchanged.

- `/health` now degrades a synced kind whose watch has kept failing for two
  minutes (`cache_sync_failed`) until it recovers; readiness is unchanged.

- Fixed the translator describing the previous object twice on every update.

- Active probes now run at most 32 at a time per round.

- Announcements whose investigation found no free slot are now held up to the
  output wait and retried as slots free, and a storm of new incidents gives
  the investigation slots to the loudest tier first.

- The order in which entities waiting for a synced kind are re-evaluated is
  now deterministic.

- In least-privilege installs, kinds the API refuses for permission are
  retried less and less often (at most every four hours), and the log has one
  line per discovery pass listing them instead of one line per kind every 30
  minutes.

- Storage write failures are now logged for the first failing batch and then
  every 30th, with a line when writes recover, instead of every second; the
  history writer stops for good when the state file is claimed by a newer
  leader, like the incident writer.

- Documented how long readiness waits for optional kinds (two minutes, not
  configurable).

- Fixed the `kubelet-stats` health component degrading on node churn: a node's
  kubelet now counts as unreachable only after two failed polls in a row, and
  never while the node is under ten minutes old or cordoned.

### Editions, permissions, configuration and chart

- The least-privilege chart rules now include PriorityClasses, RuntimeClasses,
  IngressClasses and ReplicationControllers, so a missing PriorityClass,
  RuntimeClass or IngressClass is detected again; after the first discovery
  kwatch logs one warning naming the kinds it cannot watch and shows a
  degraded `coverage` note on /health.

- A Community install whose configuration sets Pro features is now told once,
  at its first start, which settings are ignored, and the start logs a real
  warning.

- A license key with a space or newline no longer stops kwatch from starting:
  whitespace is stripped and a key that is still unusable is reported as an
  invalid license.

- A mistyped, expired, revoked or conflicting license key now logs one warning
  per reason, shows a `license` component on /health and sends a "Pro is
  disabled: reason" notice, even when Pro never ran.

- Community now keeps the incidents it announced across a restart, so their
  resolves are still sent and a pager alert is not left open; when the
  provider cap leaves a paging service inactive kwatch warns, and the provider
  Community keeps is chosen pager first, then chat, then email.

- The license, the adoption telemetry and the update check always verify TLS;
  `app.insecureSkipTLSVerify` applies to alert providers only.

- The README Helm quick-start installs no provider; kwatch now warns at start
  when no provider is active and the chart notes say so.

- Adoption telemetry uses its own ID, separate from the license cluster ID
  (installs get a new telemetry ID once).

- Pro cluster check-ins (the 30-minute ping) are removed. `clusterCheckins`
  and `KWATCH_CLUSTER_CHECKINS` are accepted, ignored and logged once as
  deprecated. Pro only renews its license daily; a revocation applies at
  the next renewal. Use `heartbeatMonitor.url` to learn when kwatch stops.

- The Pro lease and clock guard are mirrored to the
  `kwatch-license-instance` Secret, so offline grace survives `emptyDir`.

- An invalid KwatchConfig at startup now falls back to the last one that
  applied cleanly (kept in the state file), and a notice is sent once per
  refused spec.

- The permission audit checks the license instance Secret when a license key
  is set.

- `kwatch replay-export` and `kwatch version --json` read the effective
  edition from the running installation's state file, so they work on Pro.

- A KwatchConfig may now set only the fields of the CRD schema; license,
  status endpoint, Secret watching and other settings are refused. It can turn
  telemetry, check-ins and the update check off but never back on, and cannot
  add probe targets that name loopback, link-local or metadata addresses.

- The license clock guard recovers when a wrong clock step is corrected and
  the server signs a newer lease.

- The kubelet client no longer follows redirects; the update notice accepts
  only a strict version tag and compares pre-releases correctly.

- `kwatch lint` and `kwatch test-alert` use the providers the edition actually
  runs.

- A wrong boolean in KWATCH_TELEMETRY, KWATCH_WATCH_SECRETS,
  KWATCH_CRD_ENABLED or KWATCH_CLUSTER_CHECKINS no longer stops kwatch: it
  logs a warning and uses the safe side. A license key written as text in the
  configuration file is warned about.

- A Pro start no longer logs the Community warnings for a second while the
  license is settling.

- During an edition rebuild the health port keeps answering liveness until the
  new generation takes it over, so a slow rebuild no longer risks a liveness
  restart.

- The chart gains `priorityClassName`, `imagePullSecrets`, `extraVolumes` and
  `extraVolumeMounts`, rolls the pod when an existing license Secret changes
  (where Helm can read it), and renders the license instance Secret and its
  Role only when a license key is configured (`license.instanceSecret` forces
  them).

- The sizing table now says what the non-pod objects add.

- The KwatchConfig API types encode an explicit `enabled: false`.

- The chart and manifest no longer set `karpenter.sh/do-not-disrupt` by
  default, so Karpenter can consolidate kwatch's node; opt in with
  `podAnnotations`, and see the docs for PodDisruptionBudget guidance.

- The feature catalog says Community detects nodes that are not ready, and Pro
  adds pressure and draining.

- The startup log says why the Community edition keeps the Secret watch.

- `kwatch lint` and `kwatch test-alert` now have tests proving they apply the
  Community provider cap.

- `kwatch lint` and `kwatch test-alert` read the state file once per run
  instead of copying it several times, and `version --json` and
  `replay-export` use an injected clock.

- The error-line normaliser memo is no longer a process-wide global; each
  detection tracker owns its own.

- Probe targets added by a KwatchConfig are refused at connect time when their
  name resolves to a loopback, link-local (cloud metadata), unspecified or
  multicast address; in-cluster private addresses such as Service ClusterIPs
  are allowed, and anything else can be listed in the mounted configuration.

- `make alert-quality-gate` now also runs the check that the scorecard gates
  see root-cause regressions.

- When the last good KwatchConfig cannot be read at startup (lock timeout,
  unreadable file, invalid kept spec), kwatch logs the reason once at Info
  level.

### Alert-quality gates

- The alert-quality scorecard can now see root-cause regressions: it fails on
  any labelled scenario that states a wrong root at likely confidence (0.50)
  or higher, and on any scenario whose first message names a wrong root even
  if a later message corrects it.

- The scorecard judges every message of a scenario, reports the time to the
  first message that states the right root, and counts a right root found with
  a stated cause apart from one reached by falling back to the failing entity
  with no cause.

- A scenario's must-not-blame list now also covers the rival of a hedged cause
  and any message sentence that names an entity as the cause.

- A per-scenario ratchet (`internal/scenarios/testdata/ratchet.txt`) fails
  when a labelled scenario that passed stops passing, or states its cause at a
  lower confidence level.

- A mutation check switches the specificity scorer, every veto and the
  counterfactual checks off and fails unless a scorecard gate notices.

- Advisory states (a quota used exactly, pods throttled but ready, a budget
  that allows no disruption) are part of the staging day's non-events, so
  "advisory findings never produce a message of their own" is measured.

- The scenario harness models a kind kwatch cannot list as never synced,
  judges independent roots in any order, dates incidents listed in a roll-up
  by the roll-up, and the advisory documentation now says one thing
  everywhere.

- 38 adversarial scenarios (look-alike names, unrelated recent changes,
  independent faults sharing a node, states that must stay quiet) joined the
  labelled library as permanent regression tests.
## v1.0.0 (2026-10-19)

### Highlights

- **The `KwatchConfig` API is now `kwatch.abahmed.dev/v1beta1`.** The CRD
  serves both `v1beta1` (stored) and `v1alpha1` (deprecated) with the same
  schema, so existing `v1alpha1` objects keep working without any change and
  are read as `v1beta1`. Update the `apiVersion` in your manifests when
  convenient; `kubectl` prints a deprecation warning for `v1alpha1`.

- **Messages quote the latest evidence.** When a finding's summary or
  evidence changed after its incident had formed (a restart count, the
  last run), the incident kept the first copy and a later update showed
  stale lines. The incident now replaces its copy in place. Counts, times
  and ids alone do not count as new, and a refresh never sends a message
  on its own.

- **The cluster model uses half the memory per pod.** Attributes,
  relations and graph edges are stored in compact slices instead of nested
  maps, so 20,000 realistic pods take about 520 MiB in total (was about
  880 MiB). The Memory section of the operations guide and the chart now
  carry a pods-to-memory sizing table; the default 512Mi suits about 8,000
  pods.

- **A restart no longer reads the nodes already cordoned as a fresh
  drain.** Nodes that are leaving when kwatch starts are the starting
  state, not a displacement, so they no longer hold re-failures for up to
  15 minutes. A node a restored node incident stood on, gone by the first
  look, counts as removed then. The node check also reads three
  attributes in bulk instead of copying every node.
- **A provider that panics no longer takes kwatch down.** A panicking
  send counts as a failed attempt, and a job that fails three sends in a
  row is given up, also across restarts. A panic in any optional
  component restarts that component with a backoff.
- **The delivery outbox writer, dynamic watches and the CRD watcher come
  back after a panic.** Each is restarted with the same one second to one
  minute backoff; an outbox batch is never lost to a panic. After three
  failures in a row /health shows `delivery-outbox` as degraded
  (`persistence_write_failed`) or the CRD watcher as failing, and a dead
  dynamic watch reports its kind as not synced until it is rebuilt.
- **A node that returns and stays down pages again.** A repeated page is
  still held for a flap, but one that keeps failing for the page settle
  plus the base hold pages once more.
- **Nodes lost together are one incident and one page.** Nodes, zones and
  pools failing in one settle window share an incident that says how many
  nodes are down; a roll-up of pages pages once.
- **Evicted pods left over after node pressure clears are history.** An
  evicted pod no cause explains, whose workload runs all its replicas,
  opens no incident.
- **A node suspected of being the shared cause takes its workloads back
  when it fails.** They rejoin its incident instead of being announced
  again beside it, and its recovery says the node is ready again.
- **The scorecard gates pages per outage, incidents left open after a
  fold, and recoveries said while members still fail.**
- **A joint recovery message that fails to compose no longer loses
  resolves.** When the one message for several recoveries cannot be
  written, the lead goes alone and each other resolve is sent on its own.
- **A pod that cannot mount a volume because the CSI driver is not
  registered on its node now points at the node plugin.** The mount or
  attach failure quoting "not found in the list of registered CSI
  drivers" joins the failing node-agent pod on that node, so one
  incident names the plugin instead of two unrelated problems.
- **A node whose kubelet reports an x509 certificate error says so.** The
  node message now quotes the certificate error itself ("Its kubelet
  reports \"x509: certificate has expired...\"") instead of cutting it
  off behind a long prefix.
- **A lost node reaches the teams whose pods run on it, from the first
  message.** A node has no owner and no namespace, so owner- and
  namespace-routed providers heard about it only on a later update, or
  never. An incident rooted at a cluster-scoped object (node, zone) now
  routes by the owners and namespaces of its members and of the pods on
  its nodes, from the first announcement; the resolve uses the same route.
- **A zone failure counts its nodes, not their findings.** "Four nodes have
  not reported" for two lost nodes counted a node's heartbeat and
  not-ready findings separately; the lead now counts distinct nodes.
- **A broken node agent is quoted in the node message.** When the node-level
  incident's members include a crash-looping kube-system container, its own
  last error is quoted ("Pod aws-node-x in kube-system fails with ...").
- **CoreDNS down without kwatch's DNS probe is one incident.** The
  coredns/kube-dns Deployment that runs but never gets ready now folds under
  the cluster DNS outage the clients' name-resolution errors show, instead of
  announcing ten minutes later as a separate incident.
- **A dial to a Service's ClusterIP is not an "external endpoint".** Addresses
  that are a Service's ClusterIP or a pod IP in the model are in the cluster;
  the message names the Service that has no endpoints.
- **Spread and worsening updates name workloads, not pods.** "spreading to
  billing-app0 and billing-app1 in billing and five others" replaces lists of
  pod and container names, with replicas of one workload counted once.

- **A rollout stuck on a ReadWriteOnce volume names what holds it.** A
  "Multi-Attach error" is now its own failure, held for two minutes so a
  normal rolling update that detaches in time stays quiet. A persistent
  one reads "api can't start: its volume pvc-data is still attached to
  node n1 (used by pod api-6d, terminating)" and blames that pod, or the
  node when only the attachment is left. Pods that finished or are
  themselves still waiting for the volume are not named as holders.
- **A claim Pending because its provisioner is down names the provisioner.**
  The claim's StorageClass provisioner is matched to the Deployment or
  StatefulSet that serves it: by the full provisioner name in its
  arguments, name or image, else by whole words of its name and images
  (no vendor names built in; generic words such as "csi" or "driver" do
  not count, and built-in provisioners match nothing). A controller
  scaled to zero counts as having no ready pods. It is the cause only
  when one workload matches, or all that match are down; when none is
  found the note says kwatch sees no workload in the cluster serving it
  and quotes the provisioning failure.
- **A claim with no storage class and no default class says so.** "Claim
  data-0 sets no storage class and the cluster has no default one" is
  raised only when no StorageClass is the default and no free static
  volume fits, and never for a claim that selects its volume by labels.
- **A restart no longer makes the cause look late.** Right after kwatch
  starts, some findings are stamped with the time kwatch first saw them
  while pods keep their cluster times. Mixing the two used to read as "it
  began after the failures" and could name the restart as when the cause
  began. That comparison is now skipped, and a start that is only the
  time kwatch noticed is not shown as when the problem began.
- **A container the kubelet refuses to configure is no longer called
  "missing something".** "Its configuration references something
  missing" is said only when the pod really references an object that
  does not exist. A rejected `runAsNonRoot`, a bad `subPath` or an invalid
  environment variable name reads "cannot start: the kubelet rejected its
  configuration", with the kubelet's own message quoted.
- **A host port taken on every node is explained.** A pod Pending with
  "didn't have free ports for the requested pod ports" now names, for the
  closest node of each pool, the pod that already holds the same host
  port.
- **One workload names a failing node agent only by its own error.** A
  single victim is blamed on the agent when its error names the agent's
  job (the network plugin, a CSI driver). A lone application pod beside
  a failing agent is not blamed on it without such words.
- **A registry that is rate limiting pulls is named as such.** The note
  says "is rate limiting pulls" (and "cannot be reached" for an
  unreachable registry) instead of the generic "refuses the image pulls".
- **A node that stays cordoned no longer hides repeat failures.** Only
  a node that starts leaving (or is removed) counts as a displacement, so
  a failure that comes back on a node cordoned for days is "failing
  again" instead of being held and opened as a new incident.
- **The liveness advice is kept when the why-now sentence says the
  numbers.** The startupProbe or longer-delay remedy was lost whenever the
  joined sentence carried the liveness numbers.
- **Restored health marks live their 24 hours.** The first inventory prune
  no longer drops marks of an entity that was not observed yet.
- **"Who scaled it" looks back as far as the model keeps changes (24
  hours),** not 7 days it never held.
- **A digest problem that comes back is listed again after its notes age
  out,** and the per-incident digest bookkeeping of resolved incidents is
  released.
- **The flight recorder no longer skips an observation after a failed
  write.**
- **Jira code spans escape wiki markup.** `*`, `_`, `[~user]` and the
  rest of the wiki markup inside `{{...}}` are escaped, so a quoted log
  line is no longer formatted or turned into a mention. The unused
  `escapeWiki` helper is removed; the Jira dialect does the escaping.
- **SNS subjects keep their words.** Accented Latin letters and
  typographic punctuation are transliterated to ASCII, other characters
  are dropped, and the gaps collapse, instead of "shop    and.". A
  configured subject is cleaned the same way.
- **Feishu and DingTalk render the note in their own markup.** The note
  was sent as raw Markdown, so code spans arrived as literal `*x*` and
  backticks. Feishu cards get inline code and code blocks; DingTalk, whose
  markdown has no code, gets escaped text and block quotes. Feishu `<at>`
  tags and DingTalk `@phone`/`@all` text are broken so kwatch text can
  never mention anyone.
- **Webex and DingTalk cut messages to their size limit.** Webex markdown
  is held to 7439 bytes and the DingTalk request body to 20000 bytes
  (assumed from the platform documentation), cut by whole blocks so no
  code fence is left open.
- **Quoted cluster text cannot mass-mention on any chat provider.** The
  shared neutralizer now also breaks Slack's `<!here>`, `<!channel>` and
  `<!subteam^...>` (which Mattermost and Rocket.Chat webhooks expand too),
  and Flock sends neutralized text. Every chat provider's contract test
  has a `mentions` case with `@channel @everyone <!here> @all` and fails
  on any live mention in the request.
- **Every alert provider has a contract test against a golden request.**
  Seven canonical messages (page open, notify update, resolve, digest,
  markdown with code, unicode, near the size limit) are sent through each
  provider to a recording server and the exact method, path, query, pinned
  headers (secrets redacted) and normalized body are compared with a
  committed golden under testdata/contract, with API-shape checks for keys,
  severity, resolve and size limits. Goldens change only with a scoped
  -update. `make provider-smoke` is an opt-in smoke test of Slack,
  PagerDuty, Opsgenie, Teams and webhook against real accounts, skipped
  without its KWATCH_SMOKE_* variables.
- **Short application output is no longer dropped from size-limited
  messages.** Discord (and any provider using the shared rich renderer with
  a limit) omitted the output block when the output was under 40 bytes.
- **Splunk On-Call alerts post to the right URL.** The REST path was built
  as routing key then API key; it is now API key then routing key.
- **The Secret access that comes by default is now stated.** With
  `watch.secrets` on (the default) kwatch's ServiceAccount can read every
  Secret in the cluster; kwatch keeps only hashes. Helm prints a warning
  after install, kwatch logs one INFO line at startup while Secrets are
  watched, and the README and docs say it plainly. Behaviour is unchanged:
  `watch.secrets=false` removes all Secret access (`rbac.mode=least-privilege`
  alone does not).
- **Every reason Kubernetes emits is classified, and the rest is counted.**
  A checked-in snapshot of the container waiting and terminated reasons,
  pod reasons, kubelet, scheduler and controller Warning events and built-in
  condition types of Kubernetes 1.37 (internal/detection/upstream) is held
  against the reason registry by a test, so a reason kwatch ignores fails CI.
  The gaps it found are closed: about 60 Warning event reasons (kubelet
  start and node events, controller, storage and load balancer failures), the
  RegistryUnavailable and PreCreateHookError waiting reasons, and the
  ModifyVolumeError claim condition. Warning events that were only digest
  material after three repeats are now findings of their own severity.
- **The Kubernetes reason corpus is generated from the source, not typed.**
  `cmd/upstreamreasons` reads the kubernetes/kubernetes tag that matches
  k8s.io/api (v1.37.1) with go/ast and writes the event, waiting, pod and HPA
  condition reasons; `make upstream-check` fails when the committed files
  drift from the tag and `make upstream-reasons` regenerates them (both need
  network; `make verify` stays offline). Against the old hand-written list it
  dropped reasons Kubernetes never emits (FailedGetMetrics, FailedConvertHPA,
  HostPortConflict, FailedCallingWebhook and others), renamed
  WarnAlreadyMountedVolume to the real AlreadyMountedVolume, and added the
  admission, certificate, resize, Job, CronJob and CRI image reasons it had
  missed. FailedUpdateScale stays as an HPA condition reason; its event is
  FailedRescale.
- **A container waiting in an unknown state is never assumed healthy.** A
  waiting reason that is neither a known failure nor known progress
  (ContainerCreating, PodInitializing) is reported after two minutes as a
  warning that quotes the reason and message. An unknown terminated reason
  with a non-zero exit was already an Error finding.
- **`/status` shows what kwatch could not explain.** "Unexplained (last 7
  days)" counts every occurrence of a Warning event reason without a
  registry entry, and incidents that stayed "cause not clear", with the top
  reasons, failure modes and a redacted sample. Counts are bounded, kept in
  the state file, and exported as
  `kwatch_unexplained_total{kind=event|incident}`. Display only; the
  alerting for events without a registry entry (three repeats, digest) is
  unchanged.
- **Fewer notifications at the busiest hour.** A "cause revised" update that
  only takes a known cause back (its evidence aged out, nothing new was
  found) is no longer sent for notify and lower incidents, and incidents that
  only a roll-up named and that resolve in the same moment get one message
  listing them all. The staging-day peak falls from 29 to 25 notifications an
  hour; every other gate is unchanged.
- **More exit codes are named.** A container that ends with 132, 134, 135
  or 136 is now reported as an illegal instruction, SIGABRT, a bus error or
  an arithmetic error (like 139 already was), and a crash loop with one of
  them is named after it. Exit code 255 is explained as a status of -1 or out
  of range. No change for the codes named before.
- **Probe changes show real timings.** A probe that leaves its period,
  timeout or failure threshold unset is now shown with the Kubernetes
  defaults (every 10s, timeout 1s, 3 failures) instead of "every 0s, timeout
  0s, 0 failures" in "changed the readinessProbe from ... to ...".
- **Real incidents become regression tests.** Kwatch now keeps a bounded,
  sanitized flight recording of what its sources observed (48 hours, 64 MiB
  at most, names hashed, no messages or log text, no Secret detail).
  `kwatch replay-export --since 2h --out incident.jsonl` writes a window of
  it in the format the scenario library loads, and the scorecard reports a
  separate "recorded" category that is scored but never tuned against.
  See "Capturing an incident" in docs/production-operations.md.
- **`/status` shows whether the named cause was the fix.** Per rule, "cause
  confirmed by the fix in N of M resolved incidents (last 7 days)": did
  recovery follow a change to the cause, to something else, or to nothing.
  Display only; it never changes a confidence.
- **A node replacement no longer says "failing again".** When a node is
  drained, terminated or lost and its pods start again elsewhere, a workload
  with a recently resolved incident is held for the normal start time and
  does not reopen it. If it still fails after that, it is reported as a new
  incident; a failure on settled infrastructure still reopens the old one.
- **Failures name the exact place to fix.** When the cause is a setting, the
  message adds one line from the object's own fields: "Fix in:
  deployment/api → container app → resources.limits.memory (256Mi)". It
  covers memory and CPU limits, probe timing and ports, the image, the
  environment variable or Secret reference that is missing, replica and
  autoscaler limits, a disruption budget, a route's backend, a claim's size
  and class, and the tolerations or node selector a pod lacks. A Secret is
  named, never its value; a cause that is not a setting (a node down) adds
  nothing.
- **A failure reads as one joined fact.** When a container is killed at its
  limit, or by a liveness probe, right after a release, one sentence gives
  the numbers and the time: "it was killed at its 256Mi memory limit; it
  used 255Mi at peak in the last 24 hours; it has failed since 10:05, 3m
  after the 10:02 release (revision 14)". It states only what happened.
- **Failures say how they happened, in numbers kwatch saw.** A crash loop or
  restart storm adds how long the last run lasted and how many restarts came
  in how long since the pod was created; an OOM kill says when memory jumped
  between its last two readings before the kill (for example "180Mi to 512Mi
  within 40s"). Pods that wait to start and Jobs that run long are quoted
  beside the workload's usual wait or run time.
- **Refusals quote the refusing object.** When a quota or a LimitRange
  refuses pods, the message gives its exact line from the API server's own
  text: "ResourceQuota team-a: requests.memory 31.5Gi of 32Gi used; this pod
  asks 1Gi" or "LimitRange limits: max memory 1Gi per container; a container
  asks 2Gi (limit)".
- **Changes survive restarts.** Edits to workloads, config, services,
  policy and nodes are saved with their actor, fields and time, and the last
  day is reloaded at start, so a change made at 18:00 is still named when a
  workload fails the next morning, not reported as "changed while kwatch was
  down". A message also says "Between 10:01:40 (last healthy) and 10:02:10
  (first failure): ..." when both times are known and something changed in
  between. The last healthy time of each workload is saved too.
- **Findings and notes state facts, never predictions.** Memory, ephemeral
  storage and etcd size read "uses 92% of its memory limit (470Mi of 512Mi)"
  and "etcd database is 1.9 GiB of its 2 GiB quota" instead of saying what
  might be killed or refused; a fail-closed webhook says "any create it
  matches is rejected" and a still-present acknowledgement says "reminders
  are held". Two near-limit alerts are gone: a quota close to its limit
  opens nothing (only a refused create does), and an expiring certificate is
  reported once it has expired. A guard test fails on predictive wording in
  any message.
- **Restarts do not repeat old problems.** The restart summary names the
  notify and page problems that still fail and only counts the
  lower-priority ones ("and 19 lower-priority problems, in the next
  digest"). The record of what a digest listed in full today is kept in the
  state file, so a nightly restart does not make every ongoing problem new
  again.
- **One stuck deletion is one incident.** Objects held by the same finalizer
  share one root: a workload that wrote only some of them is no longer
  blamed for those, and a restored incident rooted at such a workload is
  handed over to the finalizer's incident instead of resolving as healthy.
- **A pod moved by a lost node is not a failure.** Probe failures of a young
  pod that has not restarted, on a node that just joined, are the node's
  lifecycle and open nothing.
- **The digest audit entry shows what was rendered.** It carries the line
  count and the grouped, unchanged and wake-up lines.
- **`/status` is opt-in.** The read-only `/status` view (open incidents with
  object names, no authentication) is served only with
  `healthCheck.statusEndpoint: true`; by default it answers 404. Read it with
  `kubectl port-forward`. `KwatchConfig` cannot set it.
- **Findings that predict are context, not alerts.** Version skew, a large
  etcd database and deprecated API use appear only as evidence inside a real
  incident (certificates close to expiry and quotas near their limit are not
  reported at all, see Upgrade notes); kwatch reports failures that
  happened. The same holds for container memory, CPU
  or ephemeral storage close to a limit, node filesystems or inodes filling
  up, many objects in etcd, a budget that selects nothing or overlaps
  without a refused eviction, a Job running long, and a missing reference
  of a pod that still runs. An Ingress without an address is reported only
  with evidence: a controller warning, a lost address, or peers of its class
  that have one.
- **Messages are more exact.** The resolve duration counts until the failure
  cleared, not the resolve hold. Deleted subjects say "was deleted" or "was
  deleted with its namespace". A resolve names the subject that was
  announced. A first deploy that never worked reads "has never worked since
  it was created". A workload no longer "spreads" to its own pods and
  containers. Containers that never started or were evicted get next steps.
  Builds that are not releases no longer announce an upgrade.
- **Delivery and providers.** Email uses the application CA bundle and
  `insecureSkipTLSVerify`; the HTTP proxy does not apply to SMTP. An Opsgenie
  close that gets 404 early is retried for up to five minutes. GitHub, Gitea,
  GitLab and Jira search for their key marker before creating an issue
  (ClickUp does not). Every provider has an explicit payload limit (Rocket.Chat
  is now 4000, Teams workflow 28000). Matrix `M_LIMIT_EXCEEDED` is retried.
  Failed outbox writes back off. A proxy URL that cannot be parsed disables
  the proxy. Heartbeat and telemetry requests do not follow redirects.
- **Permissions and RBAC.** The permission audit has a per-check timeout and
  reports checks it could not run as unchecked. The kube-system Lease Role is
  limited to `kube-scheduler` and `kube-controller-manager`.
- **Detection and engine.** An informer handler panic degrades health as
  `handler_panics`, and panics in detection and the pipeline are contained. Node
  pressure must be sustained before it is a finding. The cold-start summary
  waits for required sources. An OOM kill is told apart as by the node or by
  the container's own limit. Missing references are ignored while a namespace
  terminates. Name matching is word-bounded. New causes: missing
  StorageClass, APIService without a Service, ephemeral-storage limit. A call
  error naming an IP is resolved to its Service.
- **Fewer confident wrong causes.** The error has to fit the cause: a
  connection timeout points at a policy, Service or node, not at a
  ConfigMap edit unless the edited key names an address, and a parse error
  points at the configuration. A rollout and a configuration change that
  split the same pods no longer both claim that split. A failing Job or
  CronJob run can blame a ConfigMap or Secret changed up to a day before
  it, but not before its last success. Two causes that stay close are both
  named at "likely" instead of one at "high". A liveness probe that kills
  a slow-starting app is stated as the cause.

- **Node churn no longer flaps the digest.** A node-level DaemonSet pod
  (aws-node, kube-proxy, the CSI node agent) that fails on a node that is
  leaving, not ready, gone or younger than the boot window is that node's
  lifecycle: no `FailedDaemonPod` or unavailable finding is raised for it,
  and it is raised after all if the pod still fails once the node has
  settled. A pressure stall now has to last five minutes before it is a
  finding, so a short spike while a node starts its pods stays quiet. A
  digest-tier problem that opened twice in six hours waits the full
  stability hold before it resolves, so recoveries that do not last are no
  longer announced as resolved and "failing again". The node scan
  behind this now runs only when a node changed, not on every update and
  tick, which saves about 1.8 ms each at 2000 nodes.
- **Expired, unused TLS Secrets are one digest line, not an outage.** Ten
  leftover Secrets no longer read as ten lines: the digest says "9 expired
  TLS Secrets that nothing references: a/x, b/y, c/z +6", in the new and the
  ongoing sections and again, as "unchanged", on the days they did not
  change. "Clients are rejecting it now" is said only when a pod, an Ingress
  or an account references the Secret; one referenced by nothing says so
  ("expired on <date> and nothing in the cluster references it") and waits
  for the digest, and restored chat incidents for such Secrets fall to the
  digest. The audit log still names each one.
- **The next step names who uses an expired Secret.** For an Ingress it is
  `kubectl get ingress <name>` to see the hosts; for a Secret nothing
  references it is a read-only check that nothing names it. Pods keep the
  "list the keys" step.
- **A fail-closed webhook with no backend pages only when a request is
  refused.** A webhook with failurePolicy Fail whose
  Service is missing or has no ready pods, such as one an uninstalled
  operator left behind, now sends one notification ("Validating webhook x is
  failing because service y does not exist. No request has been rejected
  yet; any create it matches is rejected."), then waits for the digest as an
  ongoing item, with no page and no "still open" reminders. It pages as
  before once kwatch sees a request refused because of it: a `failed calling
  webhook "<name>"` event on a controller, or the API server's metrics
  counting its calls that failed closed. A restored page that has no such
  evidence drops to a notification, but one that saw a refusal before the
  restart keeps its page (the sighting is saved with the incident).
  failurePolicy Ignore is unchanged.
- **Ongoing problems are listed in full once a day.** A digest lists each
  ongoing problem in full in the first digest of the day that names it, or
  when it changed (flapping, its reason, how many things fail, its cause).
  Other digests show only the changed ones plus one line: "3 more still
  failing, unchanged (web, db, cache +1)". The audit log still lists them all.
- **Objects stuck deleting for good are one digest line that names the
  cause.** Objects held by the same finalizer are one incident. When the
  controller that handles the finalizer (same namespace, and its pods carry
  the finalizer's API group, or it is the field manager of the object) is
  scaled to zero or has no ready replica, the message says so: "Six objects
  in shop have been stuck deleting since Dec 4: their finalizer
  example.com/cleanup is never removed; the controller that handles it,
  deployment shop/widget-ctl, is scaled to 0." With no such controller it
  says "no running controller found for it". A deletion stuck for more than
  a day that nothing live depends on waits for the digest; one that blocks a
  claim a pod uses, or a child that is not being deleted, still notifies.
  A stuck VolumeAttachment is named by its volume, node and driver, not its
  hash, and one whose volume no longer exists is a digest line; restored
  chat incidents for such leftovers fall to the digest.
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
  open incidents that wait for the digest or a roll-up are counted daily
  (listed in full once a day, then as an "unchanged" line; see "Ongoing
  problems are listed in full once a day").
- **Recurring noise after a resolve is reported.** A digest-tier incident that
  recurs and resolves before its digest goes out is listed as resolved; periodic
  noise is listed at least once a day.
- **Digest audit entries carry content.** The digest's audit entry holds the
  opened, resolved and not-ready counts and the first 20 incidents as "id: title".
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
  pods get no grace.
- **Webhooks that cannot block go to the digest.** An admission webhook whose
  backend is missing or has no ready pods is an informational digest item when
  every webhook in its configuration has failurePolicy Ignore (requests skip
  it); with Fail, it notifies once, and pages only after a request was
  refused (see the leftover-webhook item above).
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
  skips ExternalName Services and honours `kwatch.dev/skip-probe: "true"`;
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

- **No second "resolved" for an outage the thread never heard was back.** An
  incident that reopened and recovered before its "failing again" update went
  out used to edit the resolve message a second time. It now closes quietly
  and can still reopen.

- **Private addresses in crash logs, with their owner.** Quoted crash-log
  and error lines no longer hide private IPs as `[private-address]`. When
  kwatch knows the address, it adds the Service, Pod or Node that owns it
  (`dial tcp 10.0.3.4:5432 (db/postgres): connection refused`).
  Credentials are still redacted.

- **OOM kills by the node, and CPU throttling behind failing probes.** A
  container killed far below its own memory limit while the node reports a
  system OOM or memory pressure is now said to be killed by the node: the
  message names the node and its biggest memory users instead of advising a
  higher limit. A failing liveness, readiness or startup probe on a container
  that is CPU-throttled at least half the time says so (`Liveness timed out
  while the container was CPU-throttled 72% of the time (limit 200m)`). Both
  use readings kwatch already collects; no new permissions.

- **Why only some replicas fail.** When some replicas of one workload fail and
  others are healthy, kwatch compares them and says what cleanly separates the
  two groups: the zone they run in, an injected container only the failing pods
  carry, or the image digest a mutable tag resolved to (`Same tag
  registry/api:latest, but the failing pods run sha256:9f1c0a2b44d1, the
  healthy ones sha256:aaaa1111bbbb`). It only speaks when two or more pods fail
  and at least one is healthy, and it adjusts the confidence of the cause
  accordingly. Uses pod and node data kwatch already reads.

- **A "what is wrong right now" page.** `GET /status` on the health port lists
  the open incidents (tier, root, age, one-line cause), control plane state, upgrade readiness, zone health and what kwatch cannot
  see (kinds it is forbidden to read, unreachable kubelets). Text by default,
  JSON with `?format=json`. It is read-only, needs no new permissions and
  shows object names to anyone who can reach the health port.
- **Upgrade readiness.** `/status` and, at most once a day and only when it
  changed, the digest list what is true now that matters for an upgrade:
  deprecated APIs in use with the release that removes them, kubelets outside
  the supported skew, PodDisruptionBudgets that currently allow 0
  disruptions, and `Fail` admission webhooks whose backend is unhealthy.
- **Zone health.** `/status` shows, per availability zone, the nodes not ready
  and the failing pods on them. It says failures are all in one zone only
  when the cluster has two or more zones and every other zone is clean.
- **Pods on different config versions.** When a ConfigMap or Secret changes,
  pods that read it through environment variables or a `subPath` mount keep
  the old content until their containers restart. A failure now says when the
  object changed, which keys (names only, never values) and who changed it,
  and whether the pods started since fail while the older ones are healthy, or
  the reverse. Plain volume mounts, which update in place, are never called
  stale.
- **Acknowledge an incident with an annotation.** `kubectl annotate
  deploy/api kwatch.dev/ack="looking into it"` posts one update in the
  incident's thread and stops its reminders, including the page reminder,
  until the annotation is removed (which is told once too). Resolves and real
  news still go out. A reopen inside the repage window stays acknowledged; a
  new incident is not pre-acknowledged but says when the annotation is still
  on the object. The note is quoted, bounded and stripped of credentials.
- **Route incidents by owner.** A provider route can list `owners`. An
  incident's owner is the `kwatch.dev/owner` label or annotation of its root,
  else of what owns the root, else of its namespace; with none it goes to the
  routes that ask for no owner. Provider routes did not look at labels before.
- **A cluster waking up is not a storm.** When five or more workloads start
  from zero replicas, or three or more nodes join, within minutes of each
  other, the pods they create are not reported for startup noise (probe
  failures, pending, empty Services) until the wake-up is over, and a
  crash loop, an OOM or anything still failing then is announced as before,
  with "the cluster was waking up" as context. The next digest carries one
  line: "Cluster waking up: 42 workloads started between 06:51 and 07:03;
  5 had brief startup failures, all recovered." Workloads set to zero
  together in a planned scale-down no longer raise "scaled to 0 but still
  routed".
- **Digests keep naming problems that stay open.** A digest-tier problem an
  earlier digest listed (an event that returns every quarter hour, say) is
  listed again in each following digest as `Service web (shop) — still
  failing: FailedDeployModel ×14 since 17:14`, and opens a digest of its
  own if it returns and none was sent for six hours.
- **The state file stays small and starts fast.** The write-only timeline
  is kept 7 days and at most 32 MiB (it was 30 days, up to the whole 512 MiB
  cap, which left a 650 MB file and a five minute page check on every
  restart). The compactor now gives free pages back by rewriting the file
  while running, reads the file front to back before checking it, and logs
  `state compaction pass` with the file size, free bytes and bucket sizes
  (also `kwatch_storage_file_bytes`). A file from an earlier release is
  trimmed and shrunk by the first pass after the upgrade.
- **Stuck deletions name what holds them.** A terminating Namespace now
  quotes the messages of its own deletion conditions
  (`NamespaceContentRemaining`, `NamespaceFinalizersRemaining`,
  `NamespaceDeletionContentFailure`, `NamespaceDeletionDiscoveryFailure`)
  and counts the time from the deletion request, not from when kwatch first
  saw it. Claims, volumes and workloads held by a finalizer are reported
  with the finalizer names (a claim also names the pods still using it),
  and a pod stuck terminating lists its finalizers. A pod stuck on an
  unreachable node is still explained by the node's incident.
- **A controller that stopped renewing its Lease is reported even when its
  pod is failing.** The finding names the holder pod and its state
  (`CrashLoopBackOff`, `Pending`, not ready) and joins that pod's incident,
  so the Lease is no longer silent while the holder crash-loops. kwatch's
  own leader Lease is left out of the scan.
- **A CronJob that skips runs says how many and why.** With
  `concurrencyPolicy: Forbid`, the finding now counts the runs skipped
  since the last one started, names the Job that is still running and the
  state of its pod ("skipped 5 scheduled runs: job nightly-report-28812340
  from 02:00 is still running (pod stuck in ImagePullBackOff)").
- **A call to a Service that does not exist is named as the cause.** A
  failing workload that calls such a Service gets it named, quoting the
  failed lookup: "api
  calls paymnts:8080, but there is no Service paymnts in shop. Quoted:
  "dial tcp: lookup paymnts.shop.svc.cluster.local: no such host". A
  Service named payments exists." It applies only to a workload that is
  failing (crashing, not ready, or quoting the failed lookup), never to a
  healthy workload whose configuration names the Service, and it waits
  until Services are listed. The Service it suggests is only one a couple
  of edits away.
- **Readiness flapping is reported.** Pods of a workload that switch
  between ready and not ready at least six times in ten minutes without
  restarting are one finding ("has pods that went ready to unready 9 times in the last 10
  min. Service api's endpoints keep changing. Quoted readiness failure:
  ..."). A pod's first Ready, restarting pods, booting node pools and the
  young pods of a rollout are not counted, and a workload that never
  flaps is called unusual.
- **Full volumes say who they hurt, and inodes count.** A claim over
  85% used (95% critical) now names its size and the workloads that
  mount it ("It is at 98% of 20 GiB. It is used by postgres and
  reporter."), and a claim whose inodes are nearly gone is a finding of
  its own (`VolumeInodesHigh`, mode `VolumeFull.Inodes`) even with bytes
  free. When a container on a nearly full claim crashes with "No space
  left on device", the claim is the cause and the application's own line
  is quoted. Current state only: no prediction, no new settings.
- **Rejected registry logins name the Secret.** When pulls from one
  registry fail with an authentication error, the one incident now says
  which image pull Secret the failing pods use, its type and when it
  last changed ("It pulls with Secret regcred in ci
  (kubernetes.io/dockerconfigjson, last changed 92 days ago), and the
  registry answers "unauthorized: authentication required"."), or that
  the pods name none. Pods whose pull Secret does not exist blame that
  Secret instead of the registry. Only the Secret's name, type and
  change time are read, never its content; rate limits stay separate.
- **A clearer cause no longer stays silent.** When every finding an
  announced notify incident stood on is replaced by digest findings (a
  restart turns an old `FailedGetScale` into the clearer
  `HPATargetMissing`), the incident falls to the digest tier and its thread
  hears the new cause once; a page is never lowered. The thread still gets
  its resolve, the digest lists the open incident as ongoing, and the
  restart summary now also names restored digest-tier incidents.
- **A port that matches nothing is reported as a fact.** A Service whose
  `targetPort` names a port none of its pods declare (it gets no endpoints),
  or sends traffic to a number its pods do not declare while connections
  to it are refused right now, says what it sends to and what the pods
  listen on. An Ingress rule whose backend port, a number or a name, the
  Service does not have names the rule and the ports the Service has. A
  declared port is only information: a number with nothing failing stays
  quiet.
- **A pod held in Init says what it waits for.** An init container that runs
  far longer than this workload's own earlier init runs (three minutes when
  there is no history) is reported as stuck, with the Service its command,
  arguments or environment name, whether that Service has no ready
  endpoints, and a link to that Service's own incident. The last lines the
  container wrote are quoted as written, with credentials redacted; a
  Service the output names is stated too. A healthy dependency is not
  blamed.
- **No configuration advice in the digest or `/status`.** kwatch reports
  what is happening, not what could happen. The digest no longer has a
  "Configuration risks" section or "risks on N workloads" in its header, and
  `/status` no longer has a configuration risks section (the
  `configurationRisks` key is gone from its JSON). A single replica, a missing
  probe or memory limit and a mutable image tag are still quoted in a failure
  of that workload when they show what it cost. Workloads whose pods run but
  never become ready are still named in the digest. The advisory detectors
  for "every replica on one node", "privileged container" and "pods on mixed
  config versions" are removed; the root-cause check for config versions
  stays.
- **A liveness probe that kills a slow start is named.** When a container is
  restarted by "Liveness probe failed" and never became ready before each
  kill, the message says what liveness allows and what the workload's own
  starts took: "Liveness gives api 40s (10s delay + 3 × 10s) but api normally
  needs about 75s to become ready (last 5 starts), so it is killed before it
  finishes starting. A startupProbe or a longer delay would let it start."
  The starts come from the workload's learned baseline (a new `start_seconds`
  metric: container start to ready, bounded like the others), else from its
  ready pods. Without any history kwatch says only that the container was not
  ready when each kill came. A kill of a container that had time to start
  (it ran longer than a usual start) never gets this explanation, and a
  liveness kill is no longer blamed on a startup or readiness probe budget.
- **Restarts that a failing dependency causes through liveness join its
  incident.** When pods are killed by a liveness probe that runs the same
  check as their readiness probe, and a Service they are configured to call
  has no ready endpoints or failing pods behind it, the restarts become
  members of the dependency's incident: "db went down at 10:02; 12 pods of
  api and worker were then restarted by their liveness probe (same check as
  readiness), so the restarts are a symptom." Both only explain restarts that
  are happening; a probe that checks something else, or a healthy dependency,
  leaves each workload its own incident.
- **A LoadBalancer Service that never gets an address says why.** A
  Service of type LoadBalancer with no address after its type was set now
  quotes the newest event about it ("has been waiting 40 min for a load
  balancer: "Failed to build model: ... subnets not found""), or says no
  controller has acted on it (naming its load balancer class when it has
  one). One that an Ingress routes to, or that had an address and lost it,
  notifies; a brand-new one waits for the digest. Its FailedDeployModel and
  FailedBuildModel events are quoted there instead of becoming a second
  finding.
- **A webhook whose TLS handshake fails is blamed with the API server's own
  words.** When creates fail with `failed calling webhook ...: x509: ...`,
  the webhook configuration is the cause and the x509 error is quoted:
  "validating webhook image-policy rejects calls: "x509: certificate signed
  by unknown authority"". When the Secret its pods mount holds a
  certificate that has already expired, the message adds when. A
  fail-closed webhook pages as before; kwatch reads only the public
  expiry it already recorded, never the certificates' contents.
- A NotReady node now says why in its own words. The note quotes the
  Ready condition's message, picking the nested fault out of a long
  runtime network message ("container runtime network not ready: ...
  cni plugin not initialized"), and adds the other node conditions that
  are True and the node's recent Warning events (ImageGCFailed,
  Rebooted, SystemOOM), each quoted. When the kubelet stopped reporting
  the note says kwatch cannot see why from inside the cluster and the
  node may be down or unreachable. The incident's root does not change.
- A pod held by where its volume lives now names the volume. The
  scheduling explanation reads: claim data-0 (volume pv-1) can only
  attach where topology.kubernetes.io/zone=eu-west-1b; the only node
  there, n2, has only 100m CPU free of the 1 CPU it needs. A local
  volume tied to a node says it lives on that node, which is gone or
  NotReady.
- **Wrong CPU architecture is named.** A container that crashes with
  "exec format error" on a node of a known `kubernetes.io/arch` is reported
  as an image that does not run there ("crashes on arm64 node n1 ..."), with
  the quoted error line and, when replicas of the same workload run fine on
  nodes of another architecture, how many.
- **Out of pod IPs is one incident with the numbers.** Pods stuck in
  ContainerCreating on nodes that ran out of addresses read "14 pods can't
  start on 3 nodes: no free pod IPs", with the kubelet's own sandbox error
  quoted; nodes of one zone are one incident.
- **Preemption names the preemptor.** A pod the scheduler preempted, while
  its workload is still short of replicas, says who preempted it, both
  priorities and when. Many victims of one preemptor, and the replacements
  that cannot be scheduled, are one incident rooted at the preemptor's
  workload.
- **Hard kills at the end of the grace period.** A pod terminated by a
  rollout, scale-down or drain whose container did not stop within its
  grace period and was killed (exit 137) is reported, with the failed
  preStop hook quoted when there is one. It waits for the digest unless the
  workload also fails; pods on a lost node are never reported.
- **Limits above a node's capacity are no longer a finding.** Pod limits
  adding up to more than the node has are normal for burstable pods, so the
  "Pod limits exceed node capacity" finding (`NodeResourceHigh` and
  `NodeResourceCritical`) is gone. The memory commitment is reported only
  while the node is short of memory (pressure, evictions or a kernel OOM),
  as the reason pods inside their own limit were killed.
- **Restored incidents are auditable.** The audit entry of the "restored
  incidents" summary lists the incidents it covers as "id: root tier=T
  reasons=A,B" (the first 50, with the full count in `opened`), and each
  restore logs one INFO line with the totals by tier.
- **A known problem no longer hides a workload with nothing ready.** A
  failure explained by another root, such as a node pool, joins that
  root's incident. When the pool's problem was already known the incident
  went to the digest, and it spoke up past the boot window only if the
  root itself was the workload with nothing ready. It now speaks when any
  workload it holds has no ready replica after the boot window, as it
  already did for one rooted at the workload.
- **A replaced node's first sandbox failures no longer page.** When a
  node is terminated and a new one joins, the replacement pods can fail
  to set up their network sandbox for a minute or two while the new
  node's network plugin starts. A sandbox or network-not-ready event on a
  pod whose node joined less than the boot window ago is now held, as it
  already was while a whole pool boots, and is quoted again if the pod
  still cannot start once the node is old. An exhausted address pool is
  never held.
- **A crash loop that swings between two causes stays one incident.** A
  pod that runs, ends with Error, then waits in CrashLoopBackOff can be
  blamed on the workload, then on something it depends on, then on the
  workload again. Each swing closed one incident as superseded or moved
  its root, and the next one opened a fresh incident with a new first
  message and, for a page, a new page, every few minutes for as long as
  the loop lasted. A failure that goes back to the root it just left now
  stays with the incident that took over, for as long as that incident
  is open and for up to the repage window (2h) after the swing. A
  simulated two-hour crash loop (`crash-loop-for-hours`) that produced 21
  messages now produces 2. An incident is also no longer closed as
  superseded by one that has already resolved. A Service that only the
  configuration names no longer outranks the error the crashing container
  quotes, unless that error names the Service. The memory of where an
  incident moved survives a restart. A container waiting in back-off, or
  that crashed within the hold, no longer counts as healthy, so a loop
  with long back-offs does not resolve between crashes. An outage split
  over two incidents pages once. An incident closed as superseded edits
  its thread: "Moved: this is now part of the warehouse crash in staging."
- **Ongoing digest lines read like the incident.** A problem an earlier
  digest listed and that is still open said "still failing: HPATargetMissing
  since Oct 3 22:20 (ongoing)" in later digests, after "is still failing,
  for four days now" in the first. It now always uses the incident's own
  words, never a reason code.
- **"Since" is when the problem began.** A Service stuck deleting since April
  said "since Oct 6", the day kwatch first saw it, and a reopened incident
  said the day it reopened. The start is now the earlier of the incident's
  opening and its first finding's own time, in the ongoing lines and in
  reminders ("for two days now").
- **A node resolve says what ended.** A node that was under CPU or memory
  pressure said "is ready again", though it was never NotReady. It now says
  "is no longer under CPU pressure"; "ready again" is kept for a node that
  was down.
- **A restart posts no burst.** The first time a restored incident falls to
  the digest tier after a restart, its thread gets no post: the restored
  summary and the next digest list it as still failing. A problem found
  after the restart that began long before it (a Secret stuck deleting for
  months) joins the restored summary instead of opening a chat thread.
- **A digest counts only what opened in it.** "opened" in the audit entry of
  a digest counted old problems it listed again; those are now `ongoing`.
- **Every decision has a delivery line.** A decision carried by a digest or
  the restored summary, or sent to no provider (a paging-only message with
  no pager), now logs a `provider send` line with `provider="none"`,
  `result="carried"` or `"not routed"` and its placement, so the audit log
  and the delivery log can be matched. Startup-summary and roll-up carriers
  are not logged one by one.

- **kwatch reports what has failed, not what might.** A Job that runs long, a
  deprecated API still in use, a disruption budget shared by pods that no one
  tried to evict, a ConfigMap or Secret edited under pods that only read it at
  start, a node whose disk or inodes are filling and a container near its
  limits are advisory findings: no message of their own, evidence inside a real
  incident, and a line in `/status` (deprecated APIs and version skew are
  still listed there as upgrade blockers). An Ingress with no address is a
  finding only when its controller warned, it lost an address it had, or its
  peers of the same class are addressed and it is not.
- **A cause that has not failed anything is silent.** A volume nearly full
  (`VolumeUsageHigh`, `VolumeInodesHigh`), an autoscaler at its maximum
  (`HPAMaxedOut`) and any other node condition that is True are still named
  as the cause when pods fail, but an incident made only of them sends no
  message, not even a digest line. It is announced, with the finding as the
  cause, the moment a real failure joins it. The "autoscaler at its maximum
  for 30 minutes" notification (`HPAStuckAfter`) is removed. An expiring
  certificate is no longer a digest line; the expired one is still reported.
- **Changes made to cluster-wide settings are found, and blamed.** A Pod
  Security label on a namespace, a LimitRange, a webhook configuration, a
  ServiceAccount's image pull Secrets and a StorageClass are recorded as
  changes, and also when they were edited while kwatch was down. The creation
  of a ConfigMap, Secret or ServiceAccount is never named as the change that
  broke a pod.
- **More built-in causes.** A pod refused by Pod Security Admission names the
  namespace whose enforce label did it; one refused by a LimitRange names the
  LimitRange; one denied by a ValidatingAdmissionPolicy names the policy. A
  node that is struggling (slow kubelet pod relist, container runtime errors,
  tasks stalled on CPU, memory or disk, evictions, or an extra node condition
  that is True) is blamed for the pods that go unready, fail to start or
  crash on it. A failing DaemonSet that runs on every node is a node agent
  outside kube-system too. A throttling API server or slow reads explain
  controllers that stopped reconciling and autoscalers that cannot scale. A
  ResourceClaim no device could be allocated for is the cause of the pod that
  waits for it.
- **A scale is not a release.** A change of only the replica count reads "the
  replicas were scaled from 2 to 5 at 10:04" or "the 10:04 scale-up", not
  "the 10:04 release changed the replicas".
- **Upgrade note.** The persisted downtime fingerprints now include the new
  kinds above; the first start after the upgrade reports no change for them.
  No state schema change and no reset.
- **Ongoing and reminder lines say what fails.** A digest line or reminder
  uses the incident's own wording and cause ("is stuck deleting: finalizers
  ... hold it", "targets Deployment x, which does not exist"), then "still
  failing, for ... now", counted from when the problem started. A stuck
  VolumeAttachment is named by its volume and node everywhere, never by its
  hash. Ages over eight weeks read in months and years; dates before this
  year carry the year; a few stuck objects are named in the lead. A
  flapping update followed at once by an identical "failing again" is
  recorded once. A component degradation is logged with its name and reason.
- **An upgrade or rollback no longer drops queued pages and thread ids.**
  When the state file has another schema version, the delivery outbox and
  the provider thread ids are copied into the fresh file, so queued pages
  and resolves are still sent and Slack and Jira threads continue. The old
  file is kept as `state.db.corrupt.<time>`, and the newest two copies stay
  instead of one being overwritten. An undecodable outbox record is deleted
  with a log line instead of lingering.
- **The health port is no longer open to every pod.** With the chart
  NetworkPolicy on, `/health`, `/metrics` and `/status` accept only the
  release namespace, `networkPolicy.kubeletCIDRs` and the peers listed in
  the new `networkPolicy.healthIngressFrom` (for example a Prometheus
  namespace). `/status` stays off unless `healthCheck.statusEndpoint` is
  true.
- **A webhook with broken credentials fails startup.** A malformed
  `headers` entry, a header HTTP cannot carry, or a `basicAuth` with only a
  username or only a password is now an error instead of being ignored. An
  `http://` webhook with credentials logs a warning.
- **Bad outbound proxy or CA bundle settings fail startup.** An invalid
  `app.proxyURL` or an unreadable `app.caBundlePath` used to be logged and
  skipped. A proxy URL with credentials must now be a `${file:/path}`
  reference.
- **Delivery edge cases.** A 2xx answer whose body drain fails counts as
  sent (no duplicate page). A header value with a newline is dead-lettered at
  once instead of retried for 24 hours. A job stops retrying inline after two
  minutes of waiting and is requeued, so a long backoff cannot hold a queued
  resolve. A config type error no longer prints the first characters of a
  secret read from a file.

### Editions: Community and Pro

kwatch now ships in two editions from the same image. The first release
after `v1.0.0-rc.10` is source available under ELv2 (see
[`docs/licensing.md`](docs/licensing.md)). Without a license key kwatch runs
the **Community** edition; a valid key (`license.key` or
`license.existingSecret` in the chart, `KWATCH_LICENSE_KEY` otherwise)
enables **Pro**, free during early access at
https://kwatch.dev/signup?src=upgrade. A license that lapses or cannot be
renewed falls back to Community in place, without restarting the pod.

- **Community keeps pod and node alerts.** Pod failures, scheduling
  problems, OOM kills, crash loops and restarts, node readiness and
  pressure, root-cause analysis, noise control, namespace scope and the
  RBAC audit run in both editions.
- **What moved to Pro (detectors).** Workload rollouts, Jobs and CronJobs,
  PodDisruptionBudgets and HorizontalPodAutoscalers; storage and volume
  usage; Services, Ingresses and NetworkPolicies; admission webhooks and
  policies; TLS certificates; ResourceQuotas and LimitRanges; custom
  resource conditions; control plane checks; pod readiness; node usage from
  kubelet stats; configuration risks; unusual Warning events; and active
  probes. After an upgrade these stop reporting in Community.
- **What moved to Pro (everything else).** Shared-factor, change and impact
  analysis and log investigation; flapping detection; the startup summary,
  the on-disk incident state and the report of changes made while kwatch
  was down; severity overrides, maintenance windows, runbooks, the reason
  filter and namespace selector; routing, native threads and custom
  templates; the audit log file output; the heartbeat monitor; the JSON
  `/status` format; and more than one replica.
- **Community sends to one provider.** When several are configured,
  `app.communityProvider` names the one to keep (a name that is not
  configured is warned about); without it the first by name is kept.
  `/status`, the startup log and `kwatch lint` show it, for example
  `Community sends to: slack (inactive: pagerduty)`.
- **Community starts from a baseline.** The failures that exist when kwatch
  syncs are not announced and nothing is restored from disk. A baseline
  incident that gets worse (a higher severity, or more failing members than
  at start) is announced once and behaves like any other incident after.
- **You are told what stopped running.** The startup log, `/status`
  ("Pro detectors not running: N (workloads, storage, certificates, ...)")
  and `kwatch lint` list the Pro detector families Community does not run.
  The first time a cluster runs Community after an earlier version or Pro,
  kwatch also sends one notification to the active provider that explains
  this and links to free Pro. A fresh install gets none, and it is not
  repeated across restarts (the state key is `edition.last`).
- **Chart.** The `kwatch-license-instance` Secret is always rendered empty
  and kept on uninstall; kwatch writes the instance key into it and `helm
  upgrade` leaves that data alone. The NetworkPolicy opens TCP 443 for the
  license server only when `license.key` or `license.existingSecret` is
  set.

### Breaking changes

- **Endpoints.** kwatch serves only `/healthz`, `/readyz`, `/availabilityz`,
  `/health`, `/status` and `/metrics`. The diagnostics endpoints are gone. The
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

- **The unused audit bucket is gone from the state file.** The bbolt
  `audit` bucket held nothing kwatch read and the documented 30-day audit
  retention never applied to the decision log. An older state file that still
  has the bucket opens normally and the bucket is dropped at start. The
  decision log is unchanged: `auditLog.output` stdout or a file, rotated at
  10 MiB to one `<path>.1` backup.

- **Expiring TLS certificates are no longer reported.** kwatch used to
  raise `TLSCertExpiringSoon` (mode `Cert.Expiring`) for a certificate close
  to its end date. That was a prediction, so it is gone; a certificate is
  reported once it has expired (`TLSCertExpired`), as before. The same goes
  for `ResourceQuotaNearLimit`: only a refused create opens an incident.
  Incidents saved by an older version with either old reason still restore
  and resolve.

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
  `https://api.kwatch.dev/telemetry/heartbeat`, shortly after startup and
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
- **Configuration risks, as the cost of a failure.** A new detector notes a
  workload with no readiness probe, no memory limit, an image tag that can
  change (`latest` or none) or a single replica. These are advisory: they
  never open an incident or appear in the digest, and when a failure of that
  workload shows what the risk cost, the message says so ("It runs a single
  replica, so this is downtime, not degradation.").
- **An overcommitted node explains kills on it.** A node that is short of
  memory and whose pods' memory limits add up to more than 150% of its
  memory becomes a cause for OOM kills and evictions of pods that stayed
  within their own limit. Limits above capacity alone are never reported.
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
  object; a valid `kwatch.dev/maintenance-until` in the past ends the hold
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
  its own". A stale Lease is reported
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
  pages once a request was refused because of it. A denial reason that
  mentions a timeout is no longer read as a failed call.
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
- **No more volume fill forecasts.** kwatch no longer predicts that a volume
  will fill ("full in about 3 hours"), and the `VolumeFillingUp` finding and
  mode are gone. A claim over 85% used (95% critical) is still reported, as
  are claims out of inodes. Rules that match `VolumeFillingUp` should use
  `VolumeFull`.
- **Cause updates say what changed.** An update that follows a revised cause
  now leads with "Cause now known:" (or "Cause changed:" when it replaces a
  cause already told) instead of "is the revised cause".

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
> `vX.Y.Z` for stable tags and `vX.Y.Z-rc.<N>` for RCs, exactly. A tag like `v1.0.0-beta.1` or `v1.0.0-rc1`
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
