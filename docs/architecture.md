# How kwatch thinks

kwatch is not one of those tools that just forwards every Kubernetes event to your chat.
Events happen constantly in a cluster — most of them are harmless. kwatch connects the dots,
cuts the noise, and explains what broke and **why** in one plain message.

This page explains the ideas behind those alerts in simple English. For setup and every
option, see the [README](../README.md) and the [configuration reference](./configuration.md).

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
6. **kwatch decides how to say it.** Related incidents may be **grouped** into one
   notification, and duplicates are **suppressed** so you're not paged for the same crash
   five times. (Details in [Smart grouping](#smart-grouping).)
7. **It lands in your chat.** The message — logs, events, a runbook link if you set one —
   is delivered to your configured providers. If one provider fails, kwatch tries the next
   (routing, retries, and fallback are configurable).

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

When an incident fires, the engine walks the graph and answers three questions:

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
  (default), repeated restarts climb tiers (defaults `[3, 10, 50]` restarts) and later tiers
  are *notified again* with a higher severity — the first crash is a papercut, the 50th is a
  page.
- **Re-notify.** For long-lived incidents that stay broken for hours, kwatch can nudge you
  again on a timer (`correlation.renotify.intervalBySeverity`, e.g. every 60 minutes for
  `high`), up to `maxPerIncident` times (default 3) — so a quiet incident can't be forgotten,
  but you're not re-paged forever.
- **Node suppression.** If the problem is a *node* condition, alerts for pods on that node
  are suppressed while the node is actually down. The moment it recovers, suppression lifts
  **immediately** — it doesn't wait for the resolve hold-down to finish.

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
| `kwatch-state` | Cluster identity + upgrade bookkeeping |
| `kwatch-incidents` | Every active incident (so a restart doesn't forget what's broken) |
| `kwatch-baseline` | The pre-existing problems seen at startup |
| `kwatch-pvc` | Last-known disk usage for PVC monitoring |

The point: if kwatch restarts, is rescheduled, or its pod is recreated, it **resumes exactly
where it left off** — active incidents stay active, and it doesn't re-report everything as
brand new. That also enables the startup *baseline* check: kwatch snapshots the problems that
already existed when it boots, and `reportStartupBaseline` tells you about them once (so you
know what your cluster already looks like), without treating them as fresh crashes.

---

## ✨ The result

Put together, the flow is:

- **Crashes** become incidents with a **cause**, an **impact**, and a **what-changed** hint.
- **Floods** become **one** grouped, throttled, batch-resolvable alert.
- **Root causes** (a node, a ConfigMap, a disk) get **blamed explicitly**, and if 30% of a
  shared dependency is down you hear about the *dependency* — not a thousand symptoms.
- **Restarts and upgrades** lose nothing thanks to state in ConfigMaps.

That's why kwatch alerts read like a human explaining a problem, not a log file shouting.