# ADR 0011: Health, propagation and explanation core

## Status

Accepted (2026-09-30). Supersedes the reasoning-engine, storage-layout
and message sections of ADR 0010. The rest of ADR 0010 stays as history.
The production goals this design serves are in
[production-goals.md](../production-goals.md).

## Context

ADR 0010 introduced a problem-centric core. Its implementation (now
renamed to `inventory`, `detection`, `rootcause`, `incident`, `pipeline`,
`storage` and `notification`) delivers the pipeline shape, but a review
of the branch found structural limits:

- Root cause is 14 hand-written rules. Each scans the model its own way
  and adds its own constants. Most rule bugs found in review (a node with
  an unrelated warning blamed for an app crash, loose registry and network
  policy matches) came from this.
- One goroutine does model updates, detection, reasoning, log reads and
  full-bucket store writes. Slow I/O stalls decisions.
- Incident identity is the root's name, which forces adopt, supersede and
  numbered-ID logic when a cause is revised.
- A fixed list of kinds is watched. The coverage review against the
  Kubernetes API reference and source found 25 of 76 listable resources
  covered, 22 partially and 24 not at all; 63 of 149 known failure modes
  covered, 27 partially and 59 not at all.
- There is no persisted change history, baseline or timeline, so
  correlation with changes is limited to a two-hour memory window.

The goal is a core that understands the whole cluster, explains failures
with evidence, scales to 5,000 pods and 500 nodes in one pod, and that a
junior developer can read and extend.

## Decision

### Four concepts

| Concept | Meaning |
| --- | --- |
| Object | Anything with identity: every Kubernetes object plus virtual ones (registry, cluster DNS, zone, node pool, control-plane component). Identity is `group/kind/namespace/name`. |
| Link | A typed, directed relation with a failure-propagation direction: `owns`, `runs-on`, `uses`, `mounts`, `selects`, `routes-to`, `managed-by`, `served-by`, `admits`, `resolves-via`, `pulls`. |
| Health | Each object's computed state: `Healthy`, `Degraded`, `Failing` or `Unknown`, plus a failure mode (`ImagePull.Auth`, `MemoryPressure`, `NotReconciling`) and the time it started. |
| Change | A recorded edit to an object: what changed, who changed it, when, and its revision. |

Every core type maps to one of these. `Unknown` is a real state: an
object kwatch cannot observe (no permission, API not served, not synced)
is Unknown. Unknown never causes a blame and never allows a resolve.

### Understanding every resource

Three layers give every object health and links.

1. **Discovery and watch.** Every served resource that supports `list`
   and `watch` is discovered at start and again when CRDs or APIServices
   change. Each kind gets a watch mode that bounds memory:

   | Mode | Stored | Used for |
   | --- | --- | --- |
   | Full | typed object, trimmed | pods, nodes, workloads, storage, network |
   | Status | `metadata`, `status` and selected `spec` fields | CRDs and other kinds with status |
   | Metadata | name, labels, owner references, generation, finalizers | ConfigMaps, Leases, ServiceAccounts, RBAC |
   | Hashed | metadata plus a hash of data values | Secrets and ConfigMap data |

   Each kind has an object cap, and the whole inventory has a memory
   target. Users can opt kinds out. A kind that cannot be watched is
   listed with a bounded reason code in `/health`.
2. **Generic knowledge** for any kind, including CRDs kwatch has never
   seen: health from standard status conventions (conditions such as
   `Ready`, `Available`, `Degraded`, `Stalled`, `Synced`; phases;
   `observedGeneration` lagging `generation`; a deletion stuck on
   finalizers), links from owner references, label selectors, reference
   fields by convention (`*Ref`, `secretName`, `serviceAccountName`,
   `storageClassName`) and the managing controller from `managedFields`.
3. **Specialised schemas** for well-known kinds add precise failure modes,
   links and investigation. They are added over time; without one, a kind
   still takes part in root-cause chains through generic knowledge.

The resource and failure-mode coverage tables live in
`docs/kubernetes-coverage.md` and are kept complete: every listable kind
and every known condition, reason and Warning event maps to a watch mode
and a health mode, or is marked out of scope with a reason.

### Detection sets health

Detectors no longer emit loose findings. Each detector reads one object
and returns its health and failure modes with evidence. A finding is the
record of a health transition (raised, changed, cleared). Evidence text
is redacted when it is first recorded.

### Propagation table

How failure spreads is data, not code:

```
cause (kind: mode)        link       explains (effect modes)       prior
node: NotReady            runs-on    pod: Unreachable, Evicted     0.8
node: MemoryPressure      runs-on    pod: Evicted, OOMKilled       0.6
secret: Missing|Changed   uses       pod: ConfigError, CrashLoop   0.7
deployment: Rollout       owns       pod: CrashLoop, NotReady      0.7
registry: Unreachable     pulls      pod: ImagePull                0.7
pvc: Pending|Full         mounts     pod: Pending, WriteError      0.7
any: Failing              owns       owned: Failing, Degraded      0.6
controller: Failing       manages    managed: NotReconciling       0.7
```

Generic rows cover every kind. Specific rows cover the most common
failures precisely. A rule is a row plus a fixture test. Code is used
only where text must be parsed, such as scheduler messages.

### Explanation solver

For each connected area of failing objects:

1. **Candidates.** Walk links upstream, bounded by depth 6 and a fan-in
   limit with sampling. Keep only objects that are unhealthy or changed
   inside the causal window. Reachability alone is never a cause.
2. **Evidence.** Named scorers add or subtract points. Each has one file,
   one unit test and its weight in `rootcause/weights.go`:
   temporal order, coverage, exclusivity (siblings elsewhere are
   healthy), specificity (the error names the candidate), recent change,
   revision specificity, blast-radius overlap, shared dimension (node,
   zone, pool, registry, kernel), baseline deviation, recurrence, and
   data quality (Unknown lowers confidence and never raises it).
3. **Explain.** Choose the fewest causes that cover the failures (greedy
   set cover). Each cause has a confidence, alternatives above a floor
   and a reasoning trace. Nothing above the floor means the cause is
   unknown, stated plainly.

Set cover is what makes storms quiet: 1,000 failing pods on one node
produce one cause. It also keeps independent problems apart: two causes
produce two incidents.

The solver is a pure function of the inventory snapshot. It does no I/O
and reads no clock, so a test is a fixture in and an explanation out.

### Incremental evaluation

A health change marks the object and its dependents (reverse links)
dirty. Dirty objects are collected in short solve windows (2–5 seconds)
and solved together, so a storm is one solve. Results are cached per
connected area and invalidated by the links they used. When a cause
heals, its dependents are re-explained.

### Changes, correlation and timeline

- **Classification.** Rollout (`spec.template`), scale, image, config or
  Secret data (hash), RBAC, policy, selection-relevant labels, taints,
  CRD spec, node added or removed. Status and resync churn are not
  changes.
- **Attribution.** Actor from `managedFields`, GitOps annotations,
  revision numbers.
- **Change sets.** Changes close in time on related objects form one
  release that is evaluated as one cause.
- **Blast radius and outcome.** After a change set, kwatch watches its
  downstream objects for an effect window adapted to the workload's
  normal ready time and records `healthy`, `degraded` or `reverted`. A
  revert that recovers confirms causation.
- **Timeline.** Health transitions, changes, Events and kwatch's own
  decisions are timestamped events on one persisted timeline, ordered by
  API server time with a small skew tolerance.
- **Baselines.** Rolling per-workload statistics (restart rate, ready
  time, pending time, Job duration) are persisted and feed the baseline
  scorer.
- **Recurrence.** The same root and mode links to earlier incidents and
  how they were fixed.

### Incidents diff explanations

The incident manager compares each new explanation with the previous
one. Incident IDs are opaque and stable; the root is a field.

| Change in the explanation | Decision |
| --- | --- |
| new root | announce after settling (page tier settles fastest) |
| root changed | update: cause revised, same incident |
| impact crossed 1→N or doubled, new tier | update |
| root healthy for the hold | resolve, stating what fixed it |
| counters or wording only | nothing |

Hysteresis, flapping, recurrence, startup summary and scope behave as
today, now keyed by incident ID.

### Pipeline and workers

The decision loop is single-threaded and does no I/O:
`inventory → detection → rootcause → incident → compose`. Slow work runs
on bounded workers with deadlines and feeds results back as
observations: investigation (logs, scheduler text, node top pods),
storage writes (batched about once a second) and delivery.

### Storage

One bbolt file with a bucket per data class: incidents, changes,
baselines, evidence, timeline, audit, fingerprints, state, threads and the
delivery outbox. A bounded compactor off the decision loop enforces
retention (30 days for history) and a total cap of 512 MiB of logical data,
of which evidence may use 128 MiB. Only history is evicted, oldest first.
Baselines are dropped 7 days after their workload is gone. The file is
larger than its logical data, so the same cap also applies to its physical
size: past the cap plus a quarter, the compactor lowers its target by the
excess, and the next start rewrites the file when the volume has room (an
`emptyDir` reports its real limit through `KWATCH_VOLUME_LIMIT`). A schema
mismatch or an unreadable file deletes the file and starts fresh, with no
backup and no migration; a corrupt record is skipped and reported. The
claim epoch fences stale writers. Delivery is at least once: queued jobs
persist in the outbox (at most 2048, none older than 24 hours) and are
re-queued after a restart, and a send interrupted mid-request may repeat.

### Messages

The composer turns an incident's facts into a short note that reads as
if an engineer wrote it: a lead sentence with what broke and why, the
one or two facts that prove the cause, who is affected, and one useful
read-only `kubectl` suggestion or labelled fix. There are no labels,
sections or links, and exactly one status emoji: 🔴 page, 🟠 notify,
🟡 low, ✅ resolved. Confidence shows in the wording. Providers with
short limits get the lead sentence. Escaping and redaction happen once.

### Accuracy without feedback

Accuracy comes from the design and is proven by tests:

- labelled record/replay scenarios for every propagation row, including
  negative, multi-cause, storm and flapping cases;
- a synthetic storm of 1,000 pods in 500 workloads;
- CI gates on correct root (≥90%), wrong high-confidence roots (≤2%),
  confidence calibration, messages per incident (≤3), notifications per
  hour (≤20) and the storm (≤3 messages in 2 minutes);
- a regression corpus: every wrong verdict found becomes a permanent
  replay test.

Record and replay are test tooling, not user commands.

### Readability

- One job per package; the data flow is one arrow.
- Data over code: propagation table, weights and watch modes are plain
  tables.
- Pure core, I/O at the edges. No reflection, code generation or global
  registries in the core.
- Functions at most 50 lines, cognitive complexity at most 15 and nesting
  at most 3 in the core packages, enforced by `funlen`, `gocognit`,
  `nestif` and `revive` in `.golangci.yml`.
- Every package has a `doc.go` saying what it does, what comes in and
  what goes out. Every exported name has a plain-English comment.
- Guides show how to add a detector, a propagation row, a source, a kind
  schema and a message fact, each with an example test.

## Build order

1. Readability linters; record/replay; labelled scenarios; synthetic
   storm; scorecard gates; baseline numbers for the current core.
2. Decision loop without I/O; storage by data class with reset and
   compaction; typed identity; stable incident IDs.
3. Discovery of every resource, watch modes, health model, generic
   health and links, Unknown handling, RBAC, coverage tables.
4. Change classification, change sets, outcomes, timeline, baselines,
   recurrence.
5. Propagation table, candidate walk, scorers, solver, incremental
   evaluation, incident diffing, investigation, remaining rules.
6. Narrative composer.
7. Replace the old rule engine only when the scorecard shows the new one
   is better.

Every step passes `make verify` before the next starts.

## Consequences

- Adding a data source, a kind or a failure pattern touches one place.
- Root causes in operator-managed stacks work without dedicated code.
- The default ClusterRole becomes read-only on all resources. A
  least-privilege mode keeps a narrow role; the cost shows as Unknown
  kinds in `/health`.
- The on-disk format changes. No stable release exists, so the store is
  deleted and recreated instead of migrated.
- Out of scope for now: feedback, language-model explanations,
  Alertmanager integration, team routing and escalation, digests and
  quiet hours, chat actions, shadow mode, user APIs and commands, and
  self-health incidents.
