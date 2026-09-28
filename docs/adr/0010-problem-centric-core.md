# ADR 0010: Problem-centric notification core

## Status

Proposed — needs maintainer review before implementation starts.

## Context

A 12-hour staging replay of v1.0.0-rc.10 (`kwatch-scorecard`) measured:

| KPI | rc.10 |
| --- | --- |
| Notifications | 3,132 (266/h) |
| Messages per incident | 7.7 (p95 28) |
| Updates with no visible change | 39% |
| Re-created (flapping) incidents | 166 |
| Repeated "Recovered" messages | 78 |
| Messages without a cause | 34% |
| Causes naming the incident itself | 1,596 |

A synthetic storm (5,000 failing pods in 500 workloads) produces 500 creates
and 4,500 updates.

The causes are structural, not individual bugs:

1. Kwatch notifies per `(object, reason)` symptom. One node problem becomes
   dozens of pod, Service, HPA and Deployment messages.
2. Updates follow internal state (counts, confidence, pattern) rather than
   changes a person would care about.
3. There is no alert tiering: a missing HPA metric is delivered like an
   outage.
4. Resolve has no hysteresis, so flapping re-creates incidents.
5. Root cause is guessed from graph reachability after the notification
   decision, so it is often empty or circular.
6. Seven overlapping grouping and suppression layers each have gaps:
   reason-keyed groups, revived-incident bypass, the 2-minute mass-failure
   sustain, node inhibition that ignores pressure, cooldown, cascading
   suppression, and feedback bias.

## Decision

Replace the incident engine's decision path with five stages. Monitors, every
currently monitored resource and check, and all providers stay.

```
collectors ─▶ health state ─▶ problem builder ─▶ policy ─▶ writers ─▶ delivery
(monitors)    (per object)    (one per root)     (tiers)   (per case)  (providers)
```

1. **Health state.** Each monitored object has a current health record: a set
   of active signals with first-seen and last-seen times. Monitor observations
   and resolves only update this state. No notification decisions are made
   here.
2. **Problem builder.** Every 10 seconds, and on demand, signals are joined
   into problems using explicit causal rules, not graph reachability:
   - node → pods on that node → owning workloads → Services and Ingresses
     that select them;
   - CoreDNS, the API server or an admission webhook → the workloads whose
     failures match its signature;
   - a Deployment rollout (a `spec.template` change only) → new ReplicaSet
     pods;
   - a missing Secret, ConfigMap or PVC → the pods that reference it.

   A problem has exactly one root. Signals that cannot be attributed become
   single-object problems. A new problem waits for a settle window (60–90s,
   or immediately for page tier) so the first message already contains the
   grouped picture.
3. **Policy.** Each signal has a default tier: `page`, `notify`, `digest` or
   `silent`. A problem takes its highest member tier. Policy decides:
   - **create**: after the settle window;
   - **update**: only on a material change — new tier, root change, the
     affected workload count crossing 1→N or doubling, or new first-seen
     evidence (for example an OOM after crashes). Count increments never
     trigger an update;
   - **resolve**: after hysteresis (the root healthy for N minutes). A
     re-failure inside the hysteresis window reopens the same problem
     silently;
   - **renotify**: at most once per configured interval while unchanged.

   The decision state is persisted so a restart never repeats or re-creates a
   message.
4. **Writers.** One writer per problem kind composes the message from the
   problem's evidence: what broke, why (the root and its evidence), impact
   (affected workloads and Services), and the next step with real `kubectl`
   commands for this object. Sections without content are omitted. There is
   at most one status emoji. Every renderer uses the same structured problem,
   with plain, markdown and HTML variants.
5. **Delivery.** One conversation per problem: an edit in place plus a thread
   where the provider supports it, and a dedupe key for paging providers. The
   existing generation, retry and transport contracts are unchanged.

### Scalability requirements

- **Size target:** 5,000 pods and 500 nodes on one pod within 256–512 MiB.
  Memory grows with cluster size through informer transforms (unused fields
  and Secret data stripped). There are no full-object copies in health state.
- **Indexed lookups:** by node, owner, selector and reference. There are no
  scans of all incidents or all changes per event. The problem builder costs
  O(changed objects) per tick.
- **Incremental graph:** the dependency graph is updated incrementally. There
  is no hourly full rebuild that drops dynamic edges.
- **Bounded work:** queues, workers, per-node kubelet concurrency and log
  fetches are all bounded. Log fetches happen after a problem is created,
  never on the detection hot path.
- **API budget:** the API server QPS budget is explicit, and Lease renewal
  uses its own client (done in step 4).
- **Bounded state:** problem state is sized by active problems, not history.
- **Storm behaviour:** 1,000 pods failing at once must produce at most 3
  messages within 2 minutes (scale test gate).

### Acceptance (scorecard gates for cutover)

A replay of the rc.10 staging log and the scenario suite must reach:

| KPI | Target |
| --- | --- |
| Notifications per hour | ≤ 20 |
| Messages per problem | ≤ 3 |
| Unchanged updates | 0% |
| Re-created problems | ≤ 5% of problems |
| Repeated recoveries | 0 |
| Circular causes | 0 |
| Page or notify messages without a cause or a next step | ≤ 10% |

The redesigned core must also pass every `test/e2e` scenario and the reason
parity check. The storm test must stay within its message budget.

### Rollout

The new core is built behind `KWATCH_CORE=v2` next to the current engine and
fed by the same monitors. Delivery switches by flag. Once v2 beats rc.10 on
the scorecard and a staging run, the old engine, grouping layers, feedback
learning and pattern filler are removed (step 9) with release notes.

## Consequences

- Fewer, later-but-complete first messages. The page tier bypasses settling.
- Some configuration becomes obsolete: grouping windows, mass-failure
  thresholds and feedback settings. Removals are proposed individually
  before they are made.
- Persisted incident state gains a versioned problem-state format with a
  migration. Old incident state is read once and closed silently.
- Monitors keep their observation contract, so detection changes and core
  changes stay independently testable.
