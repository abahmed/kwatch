# 🧠 How kwatch thinks

This page is for contributors and curious operators who want to understand what
happens after Kubernetes reports a problem. For installation, start with the
[interactive manager](./kwatch-sh.md); for settings, use the
[configuration reference](./configuration.md).

kwatch is not one of those tools that just forwards every Kubernetes event to your chat.
Events happen constantly in a cluster — most of them are harmless. kwatch connects the dots,
cuts the noise, and explains what broke and **why** in one plain message.

### Pod identity and replacements

Correlation uses a Kubernetes controller's owner UID when a Pod has an
`OwnerReference`. For an ownerless Pod, kwatch uses the Pod UID, so a new Pod
cannot inherit an unrelated incident. If an ownerless Pod is intentionally
recreated as the same logical unit, put this annotation on the Pod template:

```yaml
kwatch.abahmed.dev/lineage-id: payments-worker
```

`generateName`, labels, and matching specs are evidence shown in diagnosis,
not proof of replacement and never cause deduplication by themselves.

This page explains the ideas behind those alerts in simple English. For setup and every
option, see the [README](../README.md) and the [configuration reference](./configuration.md).

## 🧩 Capability controls

kwatch keeps a versioned capability catalog for `kwatch.sh`. Each capability
has a stable ID, explicit dependencies, and a lifecycle. The catalog is
informational; runtime behavior is controlled by each monitor's normal
configuration (`enabled`, thresholds, and targets), which keeps startup and
component wiring simple.

## Code architecture and naming

The Go code follows a one-way dependency flow:

```text
cmd/kwatch
    └── internal/app                 composition root
          ├── controller             informers, queues, graph wiring
          ├── handler → filter       detection and suppression
          │     └── observe          objects → observations, pod ownership
          ├── correlation             incident lifecycle and notifications
          ├── insight                 cause, impact, and change analysis
          ├── alert/*                 provider adapters and delivery
          └── state/startup/upgrader  persistence and integrations
```

Shared leaf packages (`model`, `event`, `graphcontext`, `constant`, and
`format`) contain data and pure helpers. They must not import orchestration,
providers, Kubernetes clients, or application composition code. The application
package is the only place that assembles concrete implementations and shared
clients. Domain packages receive interfaces or injected collaborators instead
of reaching into global state.

Naming follows Go conventions and the domain vocabulary already used by the
project:

- Constructors use `New<Type>`; optional wiring uses `Set<Type>`.
- Lifecycle methods use explicit verbs such as `Process`, `Resolve`,
  `Snapshot`, `Start`, `Stop`, and `Validate`.
- Files are lower-case and responsibility-oriented (`group_flush.go`,
  `graph_resources.go`, `payload_limits_test.go`).
- Public initialisms are consistent: `ID`, `UID`, `URL`, `HTTP`, `API`, `PVC`,
  and `JSON`.
- Tests use `Test<Type><Behavior>` and describe observable behavior rather than
  implementation order.

When a public name must change, keep a small compatibility wrapper and mark it
deprecated. Remove the wrapper only after all repository imports and supported
external call sites have migrated.

### Two doors into the correlation engine

Everything a detector or monitor has to say goes through one of two methods,
and nothing else:

- `Engine.Process(observation)` — "this is what I am looking at, and this is
  what I found." A `model.Observation` carries the subject and its owner as
  typed references (`model.ObjectRef`) plus the finding; turning it into the
  event the pipeline carries happens in exactly one place
  (`event.FromObservation`). Producers used to assemble that event themselves,
  in eight packages, and each copy forgot a different field: labels (which
  silently disabled every label-based silence rule for that producer's
  incidents), the pod UID (which the engine needs to tell a replacement pod
  from the original), the object's own name.
- `Engine.Resolve(subject, reason)` and `ResolveObserved(observation)` —
  "this recovered." The subject is a typed reference and an empty reason means
  "nothing is wrong with it any more", so a caller never spells out an
  incident key; six packages used to. Group, mass-failure and cross-namespace
  incidents are left alone by a subject resolve, because they speak for many
  subjects at once.

The `observe` package builds observations from Kubernetes objects —
`observe.Pod`, `Object`, `Node`, `Namespace`, `ClusterObject`, `Synthetic` —
so a subject's identity is read off the object instead of typed out at the
call site. It also owns `OwnerResolver`: one walk up the owner chain
(ReplicaSet → Deployment), shared by the pod pipeline, the kubelet and metrics
monitors and the startup baseline. Three private copies of that walk is how
one broken Deployment used to arrive as three unrelated alerts.

This mirrors the rule on the way out: `correlation/emit.go` is the only place
a notification leaves the engine.

Two vocabularies meet on an observation, and the distinction is deliberate:
`Subject.Kind` is kwatch's resource word (`pod`, `deployment`, lower case),
which incident keys and silence rules use, while `Owner.Kind` is the
Kubernetes Kind (`Deployment`), which is what an operator writes in
`severityByOwnerKind` and what the alert prints.

An incident carries the same two references (`Object` and `Owner`), decided
once when it is created. `Incident.Name` is display text — a bare workload
name for pods, `namespace/name` for objects, a whole sentence for a smart
group, and rewritten as replicas come and go — so nothing that has to answer
"is this the same thing?" reads it.

### Recovery is derived, not remembered

`internal/handler/reconcile.go` keeps, per watched object, the set of reasons
last reported for it. Each pass hands the reconciler everything currently
wrong with that object; whatever was in the last set and is not in this one is
resolved, and an object with nothing wrong resolves as a whole.

Every detector used to carry its own `else { resolve }` branch naming the
exact reasons it could produce. A reason added without a matching branch
opened an incident nothing could close, and a branch that resolved the whole
object closed incidents another detector was still reporting. Diffing makes
both mistakes unrepresentable.

Three paths keep their explicit handling, because for them recovery is not a
diff: node conditions (each has its own sustain timer, and resolving one has
to refresh the flag that suppresses the pods on that node), the pod pipeline
(a container's recovery is answered by `ResolveHealthyPodContainers`, which
checks that the other replicas are gone before closing anything), and the
event-driven producers, which have no object state to compare against.

---

## 🗺️ The journey of an alert

Every alert is born the same way. It takes a few steps from "something went wrong" to the
message in your chat:

1. **Something happens.** Watchers following pods, nodes, and events — plus a few
   "watchdogs" that check things on a timer (disk space, node pressure, stuck rollouts) —
   notice a problem and create an *event*.
2. **kwatch curates it.** Many events are dropped right away: a container stopped because it
   was scaled down (not a crash), a pod evicted during a scheduled node drain, or a
   transient blip that clears itself. Only real problems go forward.
3. **An incident is created.** A problem's **first** sighting becomes an incident — a record
   with a stable identity. From now on, every new sighting of that *same problem* updates
   the same incident instead of creating a new one. (Details in [Incident lifecycle](#incident-lifecycle).)
4. **kwatch makes it explain itself.** An *insight engine* looks at the incident through a
   map of your cluster and adds a plain-English **cause** ("owning Deployment is unhealthy",
   "the node may be down") and **impact** ("5 pods, affecting 2 services"). (Details in
   [Context-aware intelligence](#context-aware-intelligence).)
5. **kwatch looks at what changed.** If the thing that broke was just updated (a ConfigMap
   edited minutes ago), kwatch says so — that's your smoking gun.
6. **kwatch decides whether to say it.** Before anything is sent, every event walks the
   same five checks in the same order — is it pre-existing, is it a *symptom* of something
   already known, is it cooling down, which incident is it, and should it speak now or wait
   for its group. (Details in [How kwatch decides whether to speak](#how-kwatch-decides-whether-to-speak).)
7. **It lands in your chat.** The message — logs, events, a runbook link if you set one —
   is delivered to your configured providers. If one provider fails, kwatch tries the next
   (routing, retries, and fallback are configurable). Every notification, whatever triggered
   it, leaves through one door: it is audited, diagnosed and delivered the same way whether
   it came from a live event, a timer, a group flush or a mass failure clearing.
   Deliveries to one provider are **paced** (a couple of seconds apart), because forty
   incidents opening in one minute used to become forty requests and earn a rate-limit that
   delayed everything queued behind them. If a burst is big enough to saturate the queue
   anyway, what could not be sent individually arrives as **one digest** naming the most
   common reasons — a storm reads as a line, not a wall, and nothing disappears silently
   into the dead-letter queue where nobody would see it.

Think of kwatch as a detective: it doesn't just shout "it broke!" — it investigates, names
the suspect, and tells you who else might be hurt.

---

## 🧠 Context-aware intelligence

### The dependency graph

To explain *why* things fail, kwatch first builds a **map of your cluster** in memory: a
*dependency graph*. It reads your cluster with the Kubernetes informers (the same cache
kubectl and controllers use) and draws edges between related objects:

- each **pod** is attached to the **node** it runs on
- each **pod** is attached to the thing that owns it — its Deployment, StatefulSet,
  DaemonSet, or Job
- each **pod** is attached to the **ConfigMaps** and **Secrets** it mounts, and the **PVCs**
  it uses
- the graph also knows what's *downstream*: which **Services** point at those pods, and
  which **Ingresses** point at those Services

So kwatch sees the whole family tree of your workloads, not just isolated pods. The graph is
built at startup, refreshed on a timer, and patched as pods come and go — all automatic, no
config needed.

### What the insight engine tells you

When an incident fires, the engine walks the graph and answers three questions. The answers
travel with **every** notification — a fresh incident, a grouped one, a re-notification, an
escalation — and Slack renders them as a **🧠 Diagnosis** block under the alert's fields. If
you are on a large cluster and diagnoses come back empty, check `kwatch_graph_nodes` and
`kwatch_graph_edges` on `/metrics`: an empty graph explains nothing.

1. **What likely caused this?** Some reasons are their own explanation and are answered
   before the graph is consulted: a throttled container is at its **CPU limit**, a container
   near its **memory limit** is about to be OOM-killed, an HPA reporting
   `FailedGetResourceMetric` has no **metrics-server** data, a `NodePressureStall` is a node
   under pressure. Blaming a mounted ConfigMap for any of those was a guess dressed as a
   diagnosis. For everything else it checks the incident's own tree:
   - the **node** is in its dependencies → *"node worker-2 may be unhealthy"* — the 
     machine itself is probably the problem
   - the **owning workload** is unhealthy → *"owning Deployment orders-api is unhealthy"* —
     a bad rollout, not a random crash
   - a **ConfigMap or Secret** it depends on **that was updated in the last 15 minutes** →
     *"referenced Secret my-app/db-creds changed shortly before this incident"*. Merely
     depending on one is not evidence — every pod mounts `kube-root-ca.crt` — so an unchanged
     ConfigMap or Secret is never blamed, here or in the root-cause walk below
   - a **PVC** it depends on → *"referenced PVC may be unavailable"*

   If none of the obvious suspects pan out, it walks the dependency chain **backward to the
   deepest root** (a node, a persistent volume, a storage class, a ConfigMap, a Secret, or a
   service account) and blames that. The deeper the resource, the more likely it's the real
   cause.

2. **What's the impact (blast radius)?** It counts what else would be affected if the
   failing thing keeps failing — walking *downstream*: *"12 pods on this node, affecting 3
   services"*, or *"6 pods reference this PVC, affecting 2 services, 1 ingress"*.

3. **What changed recently?** It checks its change tracker for updates to the same resource
   in the last few minutes. A Secret that changed 3 minutes before every pod started
   CrashLooping is almost certainly the culprit.

### 📨 What an alert looks like

Every notification is built from one **report** — the same structure whatever the provider —
and rendered top-down in the order a person reads:

```
🟠 Pod not ready — dev/api · Deployment · ContainersNotReady · high
pod stopped being ready 2m ago

🧠 Diagnosis
• Why: node ip-10-0-81-7 may be unhealthy (node_failure)
• Impact: affects service api
• Changed recently: deployment dev/api updated 3m ago

💡 check readiness probe and recent logs
  pod api-584ddc9849-gjwjp · image api:1.2.0 · node ip-10-0-81-7 · 2m · dev

🔍 Events — from api-584ddc9849-gjwjp
  Aug 25 23:52:21  FailedScheduling  0/5 nodes are available …
```

1. **Headline** — a plain-English label for the reason (*Pod not ready*, *Container keeps
   crashing*, *Rollout stuck*), then what it happened to. The raw reason code stays on the line
   for searching.
2. **What** — one sentence of current state.
3. **Diagnosis** — why, what else it touches, and what changed just before. Only when the graph
   has something to say; a quiet alert stays quiet.
4. **Hint** — anything actionable not already shown above. Fragments that repeat the headline
   or the state are dropped.
5. **Meta** — pod, image, node, restarts, count, age, cluster — one grey line. Image
   references lose their registry and node names their domain; both tripled the line length and
   said nothing a reader needed.
6. **Evidence** — events and logs, timestamped `Aug 25 23:52:21`, labelled with the pod they
   came from when the incident covers more than one.

**Groups are named the way you'd say them.** *"3 pods of Deployment dev/api"*,
*"12 pods on node ip-10-0-81-7 across 6 workloads"*, *"6 workloads in dev: accounts,
api, fleet …"*, *"api, readify — same error: connection refused:5432"*. The
reason, the count and the age have their own places in the message, so the name never repeats
them.

**Impact names things.** *"affects service api"*, not *"affects 1 service(s)"*. Services
and ingresses are listed by name (up to four, then *"+N more"*); everything else is counted.

**"What changed" is only what could be a cause.** For a failing pod that means its owner's
rollout (*"Deployment dev/api was updated 2m before this incident — likely a rollout"*) or an
edit to a ConfigMap or Secret it mounts. The pod's own creation is not listed — that *is* the
incident — and unrelated churn elsewhere in the namespace is ignored.

> **Example — the ConfigMap that started it all.** Three pods in `my-app` all start
> CrashLooping at once. Without context, that's three unrelated alerts. kwatch sees all
> three pods mount `my-app/config.yaml`, looks back, and finds that ConfigMap was updated
> moments ago. One message: *"referenced ConfigMap my-app/config.yaml changed shortly before
> this incident — updated 3m ago — 3 pods affected, 1 service."* That's the whole story in
> one line.

---

## 📊 Mass failure detection

Sometimes the cause isn't one resource — it's a shared dependency failing at once, and
suddenly *everyone* is down. kwatch periodically scans all active incidents for a common
thread (a node, ConfigMap, Secret, or PVC).

If **30% or more** of the dependents that share that thread are in failure (with a minimum of
3), kwatch fires a **mass failure** alert. The percentage is *dynamic*: it's recomputed per
dependency from the current graph, so a big node doesn't need every single pod to be down
before you're told — 30% is usually already a shivering dip.

Mass failures are treated just like any incident: kwatch explains the *shared* root cause and
the recent changes to it, sends you one alert instead of hundreds, and **auto-resolves** the
moment the underlying incidents clear.

> **Example — the node that tipped.** `worker-5` loses its network. 40 pods are scheduled
> there. Instead of 40 separate pod alerts, kwatch fires *one* mass-failure alert:
> *"23 pods on node worker-5 are failing — 30% threshold — check the node."* You go look at
> the machine. When the node recovers and pods restart, the mass failure resolves itself.

---

## 🔄 Incident lifecycle

An incident is kwatch's way of saying *"this exact problem is happening right now."* Keeping
one stable record per problem is what stops alert storms.

- **CREATE** — the problem is seen for the first time. kwatch notifies.
- **UPDATE** — the same problem happens again (the same pod, same symptom, same container).
  kwatch refreshes the incident's data (logs, restart count, timeline) and, in most cases,
  stays **quiet** — nobody wants "CrashLoopBackOff" paged every single event.
- **RESOLVE** — the problem stops. kwatch waits for a short *hold-down*
  (`correlation.resolveHoldDown`, default 5 minutes) so brief blips that recover on their own
  never show a fake "resolved" — then it sends the "✅ resolved" message.
- **SKIP** — an event that doesn't deserve a notification right now (already reported, still
  in a cooldown, silenced, part of a group).

**How kwatch knows a problem stopped.** Two things can close an incident, and they are not
the same claim. An *observed* recovery is the detector saying the condition is gone. The other
is silence: nothing has re-reported the problem for a whole `correlation.window`. Silence is
only trustworthy because periodic informer resyncs (`resyncSeconds`, default 300) re-deliver
every object and re-run every detector, so anything still broken re-reports itself inside the
window. That relationship is load-bearing — if `resyncSeconds` is 0 or longer than the window,
nothing re-confirms an incident and kwatch would close problems that are still happening, so
it warns about that combination at startup.

Two safeguards sit on top of silence:

- kwatch asks the informer cache whether the object is **still there**. An incident whose Pods,
  Deployment or Node still exist is not closed on silence alone — it is held for up to four
  windows first, because a workload wedged on a failed rollout stops producing events without
  ever recovering.
- That grace is only for incidents about **state**. An incident kwatch could only ever learn
  from a point-in-time Kubernetes Event — a `FailedMount`, a `FailedScheduling`, an autoscaler
  that could not add nodes — gets none of it: the object still existing says nothing about
  whether the event will happen again, so it closes on silence like anything unreported.
- A resolve that came from silence rather than observation says so, in the message: *"closed
  after no further reports; the underlying problem was not observed to recover."* An operator
  reading "✅ resolved" should not have to guess which of the two it was.

Two behaviors keep this honest:

- **Cooldown after resolve.** When an incident resolves, kwatch arms a cooldown (the window,
  default 10 minutes). If the identical problem reappears inside that window the incident is
  revived **silently** — the same incident, the same thread, counters and evidence still
  updated, but no "resolved → crash → resolved → crash" ping-pong in your chat. The recurrence
  is never dropped: whatever comes next, an escalation or the eventual resolve, speaks on the
  incident that was already open.
- **Escalation.** When a crash keeps repeating, kwatch raises its hand. With escalation on
  (default), repeated restarts climb tiers (defaults `[3, 10]` restarts) and each crossing is
  *notified again* with a higher severity — the first crash is a papercut, the third is
  high, the tenth is a page.
- **Re-notify.** For long-lived incidents that stay broken for hours, kwatch can nudge you
  again on a timer (`correlation.renotify.intervalBySeverity`, e.g. every 60 minutes for
  `high`), up to `maxPerIncident` times (default 3) — so a quiet incident can't be forgotten,
  but you're not re-paged forever.
- **Node suppression.** If the problem is a *node* condition, alerts for pods on that node
  are suppressed while the node is actually down. The moment it recovers, suppression lifts
  **immediately** — it doesn't wait for the resolve hold-down to finish.

### How kwatch decides whether to speak

Every event — a crashing container, a node condition, a stuck rollout — goes through the
same four stages, in this order. Each stage can end the story.

| # | Stage | Question | If yes |
|:--|:--|:--|:--|
| 1 | **Baseline** | Was this already broken when kwatch started? | Stay quiet; it is not news. |
| 2 | **Attribution** | Is this a *symptom* of something kwatch already knows about? | Record it against the cause — count it, list it — and let the cause's alert speak for it. |
| 3 | **Identity** | Which incident is this? | Update the existing one (or fold a crash loop into its canonical key) instead of opening a new one. Inside the post-resolve cooldown that update is silent; with no incident left to revive, the cooldown suppresses a new one. |
| 4 | **Announcement** | Should it speak now? | Buffer it for its group, or send on the edge — only when something observable changed. |

Attribution recognises three kinds of cause, checked from broadest to narrowest:

1. **A node condition.** The pod's node is NotReady or under pressure — the pod is the
   node's symptom. (A pod with no node yet, when any node is down, is attributed to the most
   constrained one: it probably cannot schedule because of it.)
2. **A shared dependency.** A [mass failure](#mass-failure-detection) already covers the
   ConfigMap, Secret or node this resource depends on. The symptom is kept — it counts
   toward the blast radius, and if it is still broken when the mass failure clears it is
   announced then — but stays silent while the bigger alert is open.
3. **The owning workload.** The pod's own Deployment or Job already has an incident (a
   stuck rollout, a failed job). The pod is folded into it.

Attribution runs *before* identity on purpose: a pod whose incident is cooling down is still
its owner's symptom and keeps being counted against it, instead of vanishing into
"cooldown".

Every one of these decisions is written to the [audit log](configuration.md#audit-log) —
once per incident, not once per poll — so "why didn't kwatch tell me?" always has an answer.

---

## 🧹 Smart grouping

Some failures are actually *one* failure wearing many masks. When 20 pods of the same
Deployment crash with the same symptom, that's one incident, not twenty.

Smart grouping works on a short **grouping window** (default 60 seconds): events that arrive
together and share the same root dimension are **buffered**, and when the window closes,
kwatch sends a **single notification** that summarizes the whole group.

kwatch picks the dimension that best captures each failure type:

| Failure type | Grouped by | Example |
|:--|:--|:--|
| OOMKilled, probe failures, crashed containers | owner + namespace | "3 pods of Deployment `orders-api`" |
| Node conditions (NotReady, pressure) | node | "everything on `worker-5`" |
| Image pull errors | image (or globally for rate limits/registry timeouts) | "image blocked in `production`" |
| CrashLoopBackOff with the same log signature | the log pattern (across pods!) | "same panic text in 8 different pods" |

Each group notification lists the affected pods, owners, nodes, or images (whatever fits the
scope), and if more than **1,000** entries pile up, the rest are folded into an overflow
counter ("+37 more").

Groups don't spam either:

- After the first group alert, the **same group** can only re-notify after a cooldown
  (`4×` the grouping window, clamped between **5 and 30 minutes**) — and then as an UPDATE,
  not a brand-new alert.
- When every member of the group recovers, kwatch **batch-resolves**: one "all 12 pods
  recovered" message instead of twelve.

> **Example — the bad deploy.** You ship `v2` and it panics on startup. 12 replicas
> CrashLoop with the exact same log signature. Over a minute you *would* have received 12+
> alerts. kwatch delivers one: *"5 pods of Deployment api in production crash-looping — same
> panic in all logs — here are the logs."* When you roll back and the pods recover, it sends
> one "all recovered" message. Total: 2 notifications for a 12-pod outage.

---

## 💾 Crash-safe state: no database, nothing to lose

kwatch keeps its state in **plain ConfigMaps** in its own namespace — there is no database,
no volume, no backend to run. It writes the things it can't afford to forget:

| ConfigMap | What it holds |
|:--|:--|
| `kwatch-state` | Cluster identity, upgrade bookkeeping, and a `last-seen` liveness stamp |
| `kwatch-baseline` | The pre-existing problems seen at startup |
| `kwatch-incidents` | Active and recently resolved incidents, trimmed to the freshest entries that fit if the cluster is large enough to exceed a ConfigMap |
| `kwatch-groups` | Smart groups that speak for related incidents |
| `kwatch-threads` | Provider conversation or thread IDs associated with incidents |
| `kwatch-engine` | Correlation engine state needed to resume lifecycle decisions |
| `kwatch-pvc` | Last-known disk usage for PVC monitoring |
| `kwatch-changes` | Recent resource-change history used for diagnosis |
| `kwatch-rca` | Persisted root-cause analysis state |
| `kwatch-telemetry` | Kubelet telemetry snapshots used to resume telemetry baselines |

State written by kwatch 0.10.x used a different layout; it is read and migrated on the first
start of a newer version, so an upgrade keeps its incident memory instead of re-announcing
everything already broken.

Incidents alone were not enough to resume cleanly. A group is the notification channel for its
members, so a restart that remembered the members but forgot the group gave you forty separate
green ticks for one recovery; and a restart that forgot the thread id posted the "✅ resolved"
as a new top-level message, leaving the original alert above it with no reply. Both are
persisted with the incidents, and thread ids are only restored for incidents that actually came
back — otherwise a recurrence next week would thread under a message nobody is reading.

The point: if kwatch restarts, is rescheduled, or its pod is recreated, it **resumes exactly
where it left off** — active incidents stay active, and it doesn't re-report everything as
brand new. That also enables the startup *baseline* check: kwatch snapshots the problems that
already existed when it boots, and `reportStartupBaseline` tells you about them once (so you
know what your cluster already looks like), without treating them as fresh crashes.

### 🕳️ Knowing when nobody was watching

kwatch runs as a single replica, so it shares fate with the cluster it reports on: when nodes
go away, kwatch goes away too — precisely when the gap matters most. Silence is ambiguous, and
"nothing was wrong" and "nothing was looking" should not look the same in your chat channel.

So kwatch stamps a `last-seen` timestamp into `kwatch-state` once a minute, riding the same
tick that snapshots incidents rather than adding another write. On the next start it compares
that stamp with the current time. A gap longer than **5 minutes** is reported alongside the
startup message:

```
🎉 kwatch@v0.11.0 just started!
⚠️ No monitoring for 52m before this start — anything that broke in that window went unreported.
```

Ordinary restarts, rollouts and pod moves take less than that and stay quiet. A first-ever run
has no stamp to compare against and says nothing.

Gap detection tells you the window existed; it cannot tell you what happened inside it. Treat
it as a prompt to look, not a report.

---

## ✨ The result

Put together, the flow is:

- **Crashes** become incidents with a **cause**, an **impact**, and a **what-changed** hint.
- **Floods** become **one** grouped, throttled, batch-resolvable alert.
- **Root causes** (a node, a ConfigMap, a disk) get **blamed explicitly**, and if 30% of a
  shared dependency is down you hear about the *dependency* — not a thousand symptoms.
- **Restarts and upgrades** lose nothing thanks to state in ConfigMaps.

That's why kwatch alerts read like a human explaining a problem, not a log file shouting.
