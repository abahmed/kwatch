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

1. **What likely caused this?** It checks the incident's own tree first:
   - the **node** is in its dependencies → *"node worker-2 may be unhealthy"* — the 
     machine itself is probably the problem
   - the **owning workload** is unhealthy → *"owning Deployment orders-api is unhealthy"* —
     a bad rollout, not a random crash
   - a **ConfigMap, Secret, or PVC** it depends on → *"referenced Secret may have changed or
     is misconfigured"*

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
> moments ago. One message: *"referenced ConfigMap may have changed — updated 3m ago — 3
> pods affected, 1 service."* That's the whole story in one line.

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

Two behaviors keep this honest:

- **Cooldown after resolve.** When an incident resolves, kwatch arms a cooldown (the window,
  default 10 minutes). If the identical problem reappears inside that window, it's revived
  **silently** — no "resolved → crash → resolved → crash" ping-pong in your chat.
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
same five stages, in this order. Each stage can end the story.

| # | Stage | Question | If yes |
|:--|:--|:--|:--|
| 1 | **Baseline** | Was this already broken when kwatch started? | Stay quiet; it is not news. |
| 2 | **Attribution** | Is this a *symptom* of something kwatch already knows about? | Record it against the cause — count it, list it — and let the cause's alert speak for it. |
| 3 | **Cooldown** | Did this exact problem resolve a few minutes ago? | Revive silently; no "resolved → crash → resolved" ping-pong. |
| 4 | **Identity** | Which incident is this? | Update the existing one (or fold a crash loop into its canonical key) instead of opening a new one. |
| 5 | **Announcement** | Should it speak now? | Buffer it for its group, or send on the edge — only when something observable changed. |

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

Attribution runs *before* the cooldown check on purpose: a pod whose incident is cooling
down is still its owner's symptom and keeps being counted against it, instead of vanishing
into "cooldown".

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
| `kwatch-incidents` | Every active incident (so a restart doesn't forget what's broken), trimmed to the freshest that fit if the cluster is large enough to exceed a ConfigMap |
| `kwatch-baseline` | The pre-existing problems seen at startup |
| `kwatch-pvc` | Last-known disk usage for PVC monitoring |

State written by kwatch 0.10.x used a different layout; it is read and migrated on the first
start of a newer version, so an upgrade keeps its incident memory instead of re-announcing
everything already broken.

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
