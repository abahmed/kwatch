# Alert-quality scorecard

Alert-quality numbers of the root-cause engine (package
`internal/rootcause/explain`), measured by `make alert-quality`.
Targets are from [docs/production-goals.md](../../docs/production-goals.md).
`make alert-quality-gate` enforces every gate; it is part of `make verify`
and of the alert-quality workflow, so a change that misses a gate does not
merge.

Correct root is judged against each scenario's `expect.json`: the first
incident people hear about must end rooted at the expected entity (notify
and page incidents interrupt at once, so they are heard before digest and
silent ones, which wait for the digest), an expected `unknown` must state
no cause, and every further expected root needs its own incident.
Messages per incident, unchanged updates, re-created incidents (an
announced incident whose audit entry links a `previous` resolved
incident, or a resolved ID announced again) and repeated recoveries are
measured over the labelled scenarios and the staging day together. The
staging day holds every labelled scenario once, spaced 9 minutes apart
(so the day is 9 minutes longer for each scenario: 12 hours when the
library had 79 of them), each fixed 45 minutes later, plus healthy
rollouts every 15 minutes, scale events every 25 minutes and the
explicit non-events below, each run twice. A scenario that lasts more
than a day (a problem that is known needs two earlier days of history)
has no place in a day and is left out of it; the labelled scenarios
still include it.

Regenerate this page's current results with `make alert-quality`; the
report is written to `_output/alert-quality.md`. The "Current report"
section below is that report verbatim, and
`TestScorecardReportIsCurrent` reads it back inside the gate run and
fails when it has drifted from the engine, so a stale number cannot be
merged.

## Gate history

"Start" is the scorecard at the start of the 2026-10-01 round, before
any change below; "rule engine" is the 14-rule engine this engine
replaced, measured on 2026-09-30. The latency targets were changed on
2026-10-01 by the maintainers' decision (from 60 and 180 seconds); no
other target was relaxed. The current value of every gate is in
[Current report](#current-report).

| Metric | Target | Rule engine | Start |
| --- | --- | --- | --- |
| Correct root cause (labelled) | >= 90% | 63.0% | 96.5% (55 of 57) |
| Correct root cause (held-out) | >= 80% | - | 71.4% (10 of 14) |
| Wrong high-confidence root | <= 5% of high-confidence cases, labelled and held-out, gated at 20 or more | 0% | 2.5% (1 of 40) |
| Calibration: high confidence | 80-100% (at least 10 cases) | 100% (10 of 10) | 97.5% (39 of 40) |
| Calibration: likely confidence | 50-95% (at least 10 cases) | 66.7% (4 of 6) | 90.9% (10 of 11) |
| Messages per incident (p95) | <= 3 | - | 3 |
| Messages per incident (most) | <= 5 | 4 | 3 |
| Time to first message (page tier, max) | <= 2m (was 60s) | - | 1m45s (p95 1m45s, 9 scenarios) |
| Time to first message (notify tier, max) | <= 5m (was 180s) | - | 4m15s (p95 3m15s, 43 scenarios) |
| Time to first message (boot-held, max) | <= 15m (boot window + 5m) | - | - |
| Notifications from non-events (staging day) | 0 | - | 4 (all digest) |
| Notifications per hour (staging day peak) | <= 30/h (sanity ceiling) | 20/h | 18/h |
| Unchanged updates | 0% | 0% | 0% |
| Re-created incidents | <= 5% | 2.1% | 1.6% |
| Repeated recoveries | 0 | 0 | 0 |
| Storm messages in 2 minutes (shared node) | <= 3 | 2 | 1 |
| Storm messages in 2 minutes (shared registry) | <= 3 | 102 | 1 |
| Storm messages in 2 minutes (multi-cause) | <= 13 (causes + 3) | - | 10 |
| Multi-cause storm causes reported | >= 10 | - | 10 |
| Scenarios blaming a must-not-blame entity | 0 | 4 | 0 |
| Quiet scenarios that alerted | 0 | 0 | 0 |
| Scenarios over their message budget | 0 | 7 | 0 |

The `boot-endpoints-still-down` scenario is held on purpose by the node
boot grace, so it is measured by its own gate (see Time to first
message). Its third message (a "pods run but are not ready" update that
restated the Service already announced with no endpoints) is gone: that
reason after a Service-no-endpoints finding is one story, not a new
problem.

Three measurements moved when the library grew from 133 to 228
scenarios, none by changing a target:

- The time to first message no longer counts from the moment replicas
  began to run different builds of one tag. They are healthy side by
  side until one fails (`same-tag-new-digest` measured 9m15s from the
  drift, 1m15s from the first crash).
- The `autoscaling-limit` row's prior stays at 0.65. An unreleased
  change had raised it to 0.70 so its three scenarios (right every
  time, 0.001 to 0.004 under the boundary through the hop decay) left
  the likely level; that was done to move a gate, not on evidence, and
  with it the likely level was unmeasured (8 cases). A ceiling with
  timing-out pods cannot rule out other saturation, so "likely" is the
  honest word for four right cases of four.
- A kubelet probe failure names the address the kubelet probes, never
  one the pod calls, so the explainer no longer reads it as an external
  endpoint that several workloads share.

Two earlier misses were closed on 2026-10-05, without changing a target:

- Messages per incident (most) was 6 on the staging day: the node that
  pages and comes back twice (`page-flaps-reopens`) sent settled,
  healthy, failing again, healthy, failing again, and then healthy when
  the staging fix landed. An incident that has re-opened once now waits
  four times its hold before it resolves (two doublings, capped at
  MaxHold; one doubling left 12 minutes against a 13-minute gap between
  its failures), so the second return lands inside the open incident
  and adds no message. The scenario sends 3 alone. A notify-tier
  incident that fails again within two hours is now one incident too
  ("failing again, 2nd time in two hours") instead of a new one each
  time.
- The staging-day peak was 32/h: the library grew from 79 to 119
  scenarios while the day stayed 12 hours, so the same alerts per
  scenario arrived 50% closer together. The day now grows with the
  library at the original spacing of 9 minutes per scenario, so the
  density is measured the same way whatever the library size. The
  ceiling stays 30/h; the peak is 24/h.
- The staging-day peak was 29/h, with 679 notifications: 290 of them
  resolves, and 13 "cause revised" updates that only took back a cause
  whose evidence had aged out. Two rules cut it to 25/h and 646
  notifications: a revision that finds no cause for a known one says
  nothing (`docs/incident-lifecycle/updates.md`), and resolves of one
  tick that only a roll-up named are one message
  (`docs/incident-lifecycle/resolve.md`).

The 2026-11 round (root cause and correlation) gained headroom
without moving a target:

- The slowest notify-tier first message was 5m0s against the 5m gate: a
  workload short of ready replicas under an autoscaler at its ceiling
  waited the full `DefaultUnavailable` (5m) for a cause the ceiling
  already named. It now waits `CeilingUnavailable` (3m): the notify
  maximum is 4m15s, the p95 3m15s.
- The staging-day peak fell from 28/h to 26/h: two workloads that begin
  failing within `CoOnsetWindow` (30s) on one healthy node are one
  incident rooted at the node at once, where they were two incidents and
  a "cause revised" update.
- Messages per incident (p95 3, most 5) did not move. The five-message
  incident exists on the staging day only (the labelled scenarios top
  out at four) and none of this round's work adds a message to a
  scenario.
- Accuracy rose from 98.8% to 98.9% on 440 to 443 labelled root cases;
  the new scenarios add no wrong root.
- `TestMissingEvidenceNeverStatesAWrongRoot` (part of
  `make alert-quality-gate`) replays every labelled scenario with logs
  denied, without metrics and with Secrets and ConfigMaps hidden. The
  all three add no wrong root. When a Secret or ConfigMap the failing
  pods read is unreadable, a cause that blames no concrete fault (the
  workload's rollout, its bare failure, a registry refusing a login) is
  held below the confidence floor and the message says the cause is
  unknown, naming what kwatch cannot see. `toleratedWithoutEvidence` is
  empty and must stay empty.

Freshness and memory are gated by tests rather than this replay:

| Metric | Target | Now | Test |
| --- | --- | --- | --- |
| Decision lag p99 at 5,000 pods, in-process | <= 2s | 28 ms (skipped under -race and -short) | `TestEngineDecisionLagAt5000Pods` (internal/pipeline) |
| Live heap at 5,000 pods and 500 nodes | <= 250 MiB | 80.8 MiB | `internal/app/memory_budget_test.go` |
| Peak heap held at 5,000 pods and 500 nodes | <= 512 MiB | 168.7 MiB | `internal/app/memory_budget_test.go` |
| Re-announcements after a warm restart mid-incident | 0 | 0 | `TestWarmRestartMidIncidentAnnouncesOnce` |
| Queued outbox messages resent after a restart | exactly once | once each | `TestOutboxRestartResendsQueuedMessagesOnce` (internal/delivery) |

The decision lag is measured on the wall clock: a burst of observations
of a 5,000-pod cluster goes through `Submit` and one engine step (apply,
detect, solve, incident decisions). It does not include API server or
informer delay. In production the same span is exported as the histogram
`kwatch_pipeline_decision_lag_seconds`. The test skips under `-race`,
whose instrumentation measures the race detector rather than the engine,
and under `-short`; the normal test run enforces it.

## What changed on 2026-10-11

Every engine fix below started from a new labelled look-alike scenario
(different names, objects and numbers). No weight was tuned against a
held-out scenario. The calibration boundary was recomputed with
`scorecard.CalibratedHigh` over the labelled results only: it is still
0.70, so `rootcause.High`, `explain.ConfidenceHigh` and
`scorecard.HighConfidence` are unchanged and
`TestConfidenceThresholdsAgree` still holds.

- **A rollout and a config edit are one split of the pods.** When a
  ConfigMap or Secret that pods read at start is edited between two
  revisions, the new revision's pods are exactly those that read the
  new content. `scoreRevision` and `scoreConfigVersion` each rewarded
  that same partition (+0.15), so a rollout out-scored the edit
  whatever the error said. Both rewards are now dropped when the two
  share the partition (`config_coupling.go`). A new error-class scorer
  (`scorer_errorclass.go`, a table) reads what the errors describe: a
  parse error fits a ConfigMap or Secret, a nil dereference or index
  error fits a workload's code, and a refused or timed-out connection
  fits a policy, Service, node or endpoint and counts against a
  ConfigMap or Secret unless the changed key names an address
  (`*_HOST`, `URL`, `PORT`, `DSN`, ...). Look-alikes in both
  directions: `coupled-config-rejected-value` and
  `coupled-image-nil-pointer`.
- **A NetworkPolicy now has dependents.** `scoreCoverage` found no pods
  behind a policy, so the policy never earned the +0.20 a ConfigMap
  earned for the same pods. Its dependents are the pods it selects.
  With the error class this fixes `borderline-policy-and-config`
  (the call goes to an address the model does not hold, so
  `policy-blocks-call` cannot fire; the timeout does not fit a
  ConfigMap). The label was not changed.
- **`borderline-two-policies` was relabelled.** NetworkPolicies are
  additive allow-lists: the first policy that selects pods for egress
  isolates them, and the second (DNS only) widened what is allowed.
  The database port was denied from `allow-cache` on, so that policy
  is the root of what happened. The old label named the later policy
  because the failures followed it. The engine already said
  `allow-cache`.
- **A Job run reads what changed hours before it.** The causal window
  (30 minutes) hid a Secret rotated two hours before a CronJob run.
  For a failing Job or CronJob, a ConfigMap or Secret it references
  now reaches back to the run's start less 24 hours
  (`ExposureWindow`), but not before the CronJob last succeeded: a run
  that read the object after the edit and worked shows it harmless.
  Two rows (`run-secret-missing-or-changed`,
  `run-configmap-missing-or-changed`) let a Job be the effect, since
  no pod need be in view. Look-alikes: `cron-secret-edited-hours-before`
  (blames the Secret) and `cron-secret-edited-then-succeeded` (does
  not). Pods and Deployments keep the 30-minute window: their first
  use of a config is not known from the model.
- **Rivals compare raw scores, and a rival caps the score.** Scores
  are summed and clamped to 1, so a changed object one hop away
  reached 1.00 and `incident/cause_rival.go` compared clamped values.
  `explain.Cause.Raw` carries the unclamped sum and rivals are judged
  on it. An alternative that the chosen cause covers still counts as a
  rival when it has a change of its own (a rollout is not just an
  effect of the config edit). A cause with a rival is worded "two
  possible causes" and its score is now capped at `RivalCap` (0.69),
  so the measured level matches the stated one. The timing, change and
  outcome scorers read one fact (something changed first); together
  they count at most `CorrelatedCap` (0.20).
- **Open evidence is measured.** Three twin pairs
  (`coupled-silent-*`) have the same observations and different
  truths: a new image and an edited ConfigMap or Secret land together
  and the pods exit with an error that names neither. Whatever the
  engine says is right for one half and wrong for the other, and it
  now says "two possible causes" at the likely level (0.69). Without
  such cases the likely level was 18 of 18 right after the fixes
  above, which is underconfidence, not calibration; with them it is
  23 of 26 (88.5%).
- **Liveness kills whose event names no container.** A Kubernetes
  `Unhealthy` event without a container field path is attached to the
  pod, so a killed container found no liveness event and was reported
  as a plain crash loop with no cause. The pod's events now count
  (`container_liveness.go`). Look-alike:
  `liveness-scale-out-too-tight`.
- **Judge: a real node incident next to a workload incident.** In
  `ambiguous-rollout-every-node` and `ambiguous-oom-near-limit` the
  engine names the workload correctly, but a genuine node-pressure
  incident is announced first. The label may now list `unrelated`
  roots (`node//n1`): those incidents are skipped when the first root
  is judged, still count against the message budget and may not be
  blamed. Nothing is suppressed in the engine.
- **Missing-reference grace removed.** An earlier round of this series
  delayed `Missing` findings by 30 seconds; the incident settling
  window already absorbs an object applied next to its pod, and the
  delay moved `registry-login-missing-secret` to a generic lead. A
  terminating namespace is still skipped (`namespace-delete-crashing-pods`).
- **Later settling is the node-pressure window.** `ambiguous-rollout-*`
  (6 minutes later), `two-independent-problems` and `cause-revised`
  (1 minute later) settle later because a node pressure condition must
  hold one minute (`DefaultNodePressure`) counted from the condition's
  own transition time; `ambiguous-rollout-*` used a transition time
  in the future of the first list, so its old timing was an artefact.
  The window delays a node-pressure finding only; the workload
  incident of the same scenario is announced at the same time as before.

## What changed on 2026-10-01

Every engine fix below started from a new labelled look-alike scenario
(different names, objects and numbers) that captures the generic
pattern. No engine code, weight or tier was tuned against a held-out
scenario.

- **A full volume blames its claim.** A crash that says "no space left
  on device" (ENOSPC in its usual spellings) gives the claim the pod
  mounts the pseudo mode `VolumeFull`, so the `claim-not-usable` row
  blames the claim: the size a person sets and resizes. A
  PersistentVolume named by a Bound claim is never "missing", even when
  kwatch does not list volumes: the binding proves it exists.
  Look-alikes: `claim-full-with-volume` (volume listed),
  `claim-full-unlisted-volume` (volume not in view).
- **Webhook timeouts root at the configuration.** A failed create that
  names a webhook in the API server's words (`failed calling webhook
  "x"`) and says the call failed (`context deadline exceeded`, a client
  timeout, refused) gives that configuration the pseudo mode
  `Webhook.Timeout` or `Webhook.CallFailed`, read through the webhook
  names now stored on each configuration. An incident rooted at a
  fail-closed configuration that rejects creates pages under the
  `admission-blocked` rule, though its endpoints are ready and it has no
  finding of its own. Look-alikes: `mutating-webhook-slow-backend`,
  `validating-webhook-deadline`.
- **A route whose backend Service is missing roots at the Service.** A
  Gateway API route that sends traffic to a Service that does not exist
  is reported at once, as an Ingress already was (reason
  `IngressBackendNotFound`, now also used for routes), and
  `routed-missing` also explains a route that only reports a failed
  condition. A routed Service that does not exist pages under
  `traffic-lost`. Look-alikes: `route-backend-renamed`,
  `route-canary-backend-missing`.
- **An HPA at its maximum under load is the cause.** An autoscaler at
  maxReplicas whose metric wants more (`Scaling.MaxedOut`) explains the
  pods of its target that time out their probes (`autoscaling-limit`)
  and the target's missing replicas (`autoscaling-limit-unavailable`):
  "has reached its autoscaling limit". Look-alikes:
  `autoscaler-ceiling-cpu`, `autoscaler-ceiling-requests`.
- **Drains within their envelope are silent.** An incident rooted at a
  cordoned or departing node is Silent while nothing it disrupted fails
  (no digest, no recovery message). It notifies only when the drain
  exceeds its envelope: pods unready long after, or a budget blocking
  evictions. A budget whose expected pods are all healthy while it
  allows no eviction is too strict for its replica count, which is
  conclusive, so it is reported after one minute instead of ten.
- **Storms on a shared error.** `external-database-storm` (six
  Deployments in two namespaces refused by `db.example.com:5432`) is
  one incident rooted at `external-endpoint//db.example.com:5432` in one
  message; `external-database-one-workload` is the control that still
  blames its own workload; `shared-panic-storm` (four workloads panic
  with "license check failed for tenant <N>") is one incident rooted at
  the shared error.
- **Message wording.** Updates lead with the incident's subject and name
  it once ("pg-operator ... now has no ready replicas (0 of 1). It is
  spreading to ..."); a changed Secret reads "after the 10:01 change to
  secret orders-db. Key password changed." instead of "does not exist";
  a full claim "is out of space"; a webhook "times out on every call";
  a blocked drain names its node. Every message golden was regenerated
  and each diff reviewed.

### Time to first message

The gates are on the slowest scenario of each tier; the p95 is reported
next to it. Measured from the first failure observation: the first log
entry about an entity the incident explains, at or after the earliest
`Since` of its findings. Cold-start scenarios are left out, because they
measure the initial sync rather than detection.

A scenario labelled `bootHeld` in its `expect.json` has its first message
held on purpose: its failure is present from the start of a node boot, and
the product holds failures for `kube.BootWindow` (10 minutes) from node
creation, plus the 75-second settle. Such a scenario is left out of the
page and notify gates above and measured by "Time to first message
(boot-held)", whose target is `kube.BootWindow` plus 5 minutes, measured
the same way from the failure onset. Label a scenario only when the boot
grace is what delays its message; never to dodge another gate. Today only
`boot-endpoints-still-down` is labelled.

| Notify path | Start | Now | What dominated |
| --- | --- | --- | --- |
| budget-blocks-drain | not measured (announced as a digest, then revised) | 2m15s | the 10-minute budget grace; a budget too strict for its replica count is now conclusive after 1 minute |
| probe-port-mismatch | 4m15s | 1m15s | the 3-minute pod not-ready grace; the fixture's probe events did not name their container, so the probe detector (3 failed probes) never saw them. Real kubelet events name it |
| borderline-init-after-rotation | 4m15s | at most 1m15s | measurement: the fixture dated the rollout's new pod an hour back, so the failure seemed to start at the log's start |
| operator and route conditions, Pending pods (8 scenarios) | 3m15s | 3m15s | the 2-minute custom-resource and pod-Pending graces plus the 75-second settle; not shortened: an operator passing through a failed state and a pod waiting for an autoscaled node are not conclusive |

| autoscaler at its maximum (3 scenarios) | 3m15s (as an update) | 5m0s | the digest: an autoscaler at its maximum is low priority on its own and waits for the half-hourly digest; its first interrupting message is the promotion when the Deployment falls short of replicas, which the scorecard now counts as the first message instead of an update |

The notify p95 is now 5m0s, at the target: the three autoscaler
scenarios dominate it, and before them eight scenarios wait out a
2-minute grace that only a conclusive signal could shorten.
Page incidents take 15 seconds to 1m45s; the slowest are control-plane
components, whose detectors sustain unavailability for a minute and a
half.

- **Predictions are advice, and a cause that has not failed anything is
  silent (batch F2).** kwatch reports failures that have happened.
  `JobRunningLong`, `DeprecatedAPIInUse` and the other predictive
  findings are advisory, so `job-running-long`,
  `deprecated-api-still-called` and `upgrade-readiness-digest` are now
  quiet scenarios (`/status` still lists the deprecated APIs and version
  skew, and `TestReadinessNamesOnlyWhatIsTrueNow` reads them as
  advisory findings). A volume nearly full (`VolumeUsageHigh`,
  `VolumeInodesHigh`), an autoscaler at its maximum (`HPAMaxedOut`) and
  an extra node condition are still causes, but an incident made only of
  them is `Silent` (`causeOnlyReasons` in `incident/policy.go`): it is
  announced, with the finding as the cause, when a real failure joins
  it. `volume-nearly-full-quiet` became a quiet scenario and
  `hpa-at-max-healthy-quiet` is its autoscaler look-alike; `hpa-at-max`,
  `autoscaler-ceiling-*` and `claim-full-*` still announce the cause when
  pods fail. The old "autoscaler stuck at its maximum for 30 minutes
  notifies" rule (`HPAStuckAfter`) is gone for the same reason, and
  `hpa-resolve-while-workload-broken` now expects only the Deployment.
  `webhook-tls-serving-cert-expired` changed order, not substance: the
  expiring-soon finding is advisory, so the Secret no longer has an
  incident to be told "expired" about before the webhook fails. It opens
  at expiry and settles like any incident, so the paging webhook
  incident is heard first and the Secret second (both are labelled
  roots).
- **More built-in causes (batch F2).** Look-alikes first, then rows:
  `pod-security-enforce-label-added` (`pod-security-rejects`: a
  namespace with an enforce label and a "violates PodSecurity" refusal),
  `limit-range-max-lowered` (`limit-range-rejects`),
  `admission-policy-denies-pods` (`admission-policy-denies`: the denial
  names a ValidatingAdmissionPolicy), `node-condition-true-pods-unready`
  (`node-degraded`: a slow kubelet relist, runtime errors, a stalled
  node, evictions or any other node condition that is True),
  `node-agent-outside-kube-system` (`node-agent-failing` now also takes
  a DaemonSet of any namespace with a pod on every node),
  `apiserver-throttling-stalls-controllers` (`apiserver-unavailable`
  now also takes throttling and slow reads, and autoscalers whose scale
  calls fail), `resourceclaim-unallocated-pod-pending`
  (`resource-claim-unallocated`). Labelled scenarios for built-in
  detectors that had none: `csr-denied`,
  `flowschema-dangling-priority-level`, `pdb-overlap-eviction-refused`
  and `statefulset-rollout-stuck`.
  Two cases have no labelled scenario by design. A slow kubelet pod
  relist is reported only after five minutes, and so is an approved but
  never issued certificate request, which with the notify settle is
  beyond the five-minute first-message gate; their rows and
  detectors are covered by fixtures and unit tests. The staging-day
  peak went from 25/h to 30/h (target 30/h) because twelve scenarios joined
  the day; the day grows by one spacing per scenario, so this is the
  spread of the new scenarios, not a new kind of noise.

## Calibration

Calibration is two-sided. A level right less often than its band is
overconfident; one right more often than its band (a "likely" cause that
is always right) is underconfident and hides certainty readers could
use. A level is gated only once it has at least 10 cases: below that,
one case moves its accuracy by more than ten points.

The levels are bucket boundaries over the engine's score, computed once
from the labelled results and committed as constants
(`rootcause.High`, `explain.ConfidenceHigh`, `scorecard.HighConfidence`,
kept equal by `TestConfidenceThresholdsAgree`). The method,
`scorecard.CalibratedHigh`, is isotonic in spirit: scores are grouped in
0.05 ranges; walking down from the top, the high boundary is the lower
edge of the lowest range that has at least 10 cases and is right at
least 80% of the time, while it and everything above it together stay
at 80% or more. The report prints what the method gives for the current
labelled set next to the committed value.

Two gates (`calibration_boundary_test.go`) fail the build when the
committed boundary drifts from the labelled set. "Calibration:
high-confidence boundary" fails when the computed boundary sits more than
0.05 above `rootcause.High` (scores stated plainly in a range that is
right too rarely: overconfidence) or more than 0.15 below it
(underconfidence, wider because it states nothing wrong and because the
boundary already stands at 0.55: 19 labelled cases in [0.55, 0.60) are
all right), or when no range qualifies. "Calibration: precision at the
high boundary" fails when the labelled cases scored at or above
`rootcause.High` are right less than 92% of the time, the accuracy the
boundary was calibrated at, once there are at least 10 of them.

On 2026-10-01 the labelled cases scored in [0.70, 0.75) were right 12
times in 13 (92%): stated as "likely", they made that level
underconfident (90.9%, then 94.1% with the new scenarios). The method
put the high boundary at 0.70 (it was 0.75). The four labelled cases
between 0.50 and 0.70 were all right but are too few to judge, so the
likely level is reported as not gated. No boundary would have kept
"likely" inside 50 to 90% with 10 or more cases: every range of 10 or
more includes the 13 tied cases at 0.70, of which only one is wrong.
Twelve `ambiguous-*` scenarios (`library_ambiguous_test.go`) were added,
as pairs labelled with the root a person would conclude from the whole
story. Of them, five land in the likely band: an init container with
nothing edited (right), a CronJob run that failed after its Secret was
rotated (blamed on the CronJob: wrong) and with nothing edited (right),
and an OOM kill on an overcommitted node under pressure at 92% of the
container's limit (blamed on the node: wrong) and at 51% (right). The
rest sit elsewhere: the init container after a Secret rotation is high
and right; an image bump and a ConfigMap edit a minute apart are both
blamed on the image at high (right for one, wrong for the config-parse
panic); a rollout during node pressure names the node without a cause
(right when only its replica fails, wrong when every replica fails); a
ceiling over a dependency that stopped answering names the dependency
(right), and one over a healthy dependency names the autoscaler (right).
The rate is near the top of its band, since the autoscaler and
healthy-dependency ceilings are right at 0.70. The current rate of each
level is in [Current report](#current-report).

## Non-events

The staging day runs each of these twice. A notification is attributed
to a non-event when its incident's root, a finding it explains or its
impact is a non-event object (the `staging` and `calm-*` namespaces, and
nodes and zones named `bg-*` or `calm-*`).

- A successful rollout: new pods ready within 25 seconds, old ones go.
- Scale up from three to five replicas and back twenty minutes later.
- A node drain within its budget: cordon, evictions through the eviction
  API with replacements ready within 20 seconds, then the node is
  deleted. Silent since 2026-10-01; before, each drain sent a digest
  ("Node is cordoned for maintenance") and its recovery.
- Pod churn from five one-off Jobs that complete and are deleted.
- A healthy CronJob running every 30 minutes for three hours, on time,
  deleted before its next run.

The background churn of the staging day (rollouts every 15 minutes and
scale events every 25 minutes) is attributed the same way.

## Recorded set

The recorded set (`testdata/recorded`, found by listing the directory)
holds incidents the flight recorder captured in real clusters and
`kwatch replay-export` wrote out, each with a label written before its
first replay. It follows the held-out rule: scored and reported on its own
("Recorded scenarios" below), never tuned against, and a miss is reported,
not fixed to fit the file. It is not part of the labelled accuracy, the
calibration or the wrong-high gate. Its accuracy gate is informational
until ten recorded cases exist; no gate fails on it. The one example now
(`recorded-bad-rollout`) is a synthetic scenario run through the recorder,
sanitizer and export to prove the format; replace it as real captures
arrive. See "Capturing an incident" in `docs/production-operations.md`.

## Held-out set

The held-out set (`testdata/heldout`, generators in `heldout_*_test.go`)
measures accuracy on failures the engine was not built against. Its
rule:

- Each scenario is written from a description of a Kubernetes failure,
  and its label (the root a person who knows Kubernetes would name) is
  fixed before the scenario is first replayed.
- A held-out miss is reported, never fixed by changing rules, weights,
  tiers or the label to fit these files. Fixing a missed failure needs
  its own labelled scenario first.
- It is scored and reported on its own: it is not part of the labelled
  accuracy, the calibration bands or the staging day. Only its correct
  root rate is gated; blame, tier and budget misses are listed but not
  gated.
- Once held-out scenarios have been looked at to find the gap behind a
  miss, they are "seen": they move into the labelled set and are
  replaced by fresh held-out scenarios for other failures.

### Rotation of 2026-10-01

Four held-out misses were looked at to find their generic gaps: a full
PVC blamed on its PersistentVolume, a webhook timing out with ready
endpoints reported on its Service, an HTTPRoute with a missing backend
blamed on the route, and an HPA at its maximum reported without a
cause. Each gap was fixed through new labelled look-alikes (above), and
the four scenarios became labelled under their own names:
`heldout-pvc-full` is now `pvc-full`, `heldout-webhook-timeout` is
`webhook-timeout`, `heldout-route-missing-backend` is
`route-missing-backend` and `heldout-hpa-at-max` is `hpa-at-max`. Their
labels did not change. One fixture detail of `webhook-timeout` was
corrected when it moved: its EndpointSlice published no port, which read
as a Service targeting a port its pods do not expose, contradicting the
failure described (ready endpoints, an overloaded backend).

Four fresh held-out scenarios replaced them, written and labelled before
they were first replayed:

- `heldout-statefulset-zone-conflict`: a restarted StatefulSet broker
  cannot be scheduled because its zonal volume is in a zone whose only
  node is full. Label: its PersistentVolumeClaim. Right in the current
  report (high 0.95); it first missed, blamed on the scheduling
  constraint "had volume node affinity conflict".
- `heldout-pull-secret-rotated`: a namespace's image pull Secret is
  rotated to a token the registry rejects; another namespace pulls from
  the same registry fine. Label: the Secret. It first missed, blamed on
  the registry (a must-not-blame entity); the current report roots it at
  the Secret.
- `heldout-liveness-too-aggressive`: new replicas of a slow-starting app
  are killed by a liveness probe that gives up after 20 seconds. Label:
  the Deployment. Right, now with the cause named.
- `heldout-quota-pod-count`: a scale-up exceeds a namespace's pod-count
  quota while another namespace scales out. Label: the quota. Right.

They are reported, not fixed. The held-out set is again 12 scenarios
with 14 root cases, all 14 right in the current report.

## Borderline labelled scenarios

Eight labelled scenarios (`library_borderline_*_test.go`) are ambiguous
on purpose: two plausible upstream changes close together (a ConfigMap
edit and a NetworkPolicy, a rollout and a Service selector edit, an
operator upgrade and a custom resource edit, a Secret rotation and a
schedule edit, a Gateway edit and a route edit, two policies, a Secret
rotation and a rollout) and weak timing with partial evidence (a
ConfigMap picked up eleven minutes later, with an error naming no key).
Their labels are the true root of what happened, not the engine's
expected answer. In the current report all eight are right; earlier
rounds missed borderline-policy-and-config (blaming the ConfigMap) and
borderline-two-policies (blaming the allowing policy).

## Current report

The report below is generated by `make alert-quality`; it is committed
here verbatim and checked by `TestScorecardGates`.

<!-- BEGIN GENERATED REPORT (make alert-quality) -->
## Gates

| Metric | Target | Current | Result |
| --- | --- | --- | --- |
| Correct root cause (labelled) | >= 90% | 99.4% | pass |
| Correct root cause (held-out) | >= 80% | 100% | pass |
| Wrong high-confidence root | <= 5% (at least 20 cases) | 0% (0 of 454 high-confidence cases, labelled and held-out) | pass |
| Wrong root stated at likely confidence or higher (labelled) | <= 0 | 0 | pass |
| Scenarios whose first message stated a wrong root | <= 0 | 0 | pass |
| Calibration: high confidence | 80-100% (at least 10 cases) | 100.0% (441 of 441 right) | pass |
| Calibration: likely confidence | 50-95% (at least 10 cases) | 91.7% (55 of 60 right) | pass |
| Calibration: high-confidence boundary | 0.55-0.75 (committed 0.70) | 0.55 | pass |
| Calibration: precision at the high boundary | >= 92% (at least 10 cases) | 100.0% (441 of 441 right) | pass |
| Messages per incident (p95) | <= 3 | 3 | pass |
| Messages per incident (most) | <= 5 | 5 | pass |
| Time to first message (page tier) | max <= 2m0s | p95 1m45s, max 1m45s (69 scenarios) | pass |
| Time to first message (notify tier) | max <= 5m0s | p95 4m15s, max 5m0s (621 scenarios) | pass |
| Time to first message (boot-held) | max <= 15m0s | p95 12m0s, max 12m0s (7 scenarios) | pass |
| Time to first message (episode-held) | max <= 15m0s | p95 11m15s, max 11m15s (1 scenarios) | pass |
| Notifications from non-events (staging day) | <= 0 | 0 | pass |
| Notifications per hour (staging day peak) | <= 30/h | 25/h | pass |
| Unchanged updates | <= 0% | 0% | pass |
| Re-created incidents | <= 5% | 1.9% | pass |
| Repeated recoveries | <= 0 | 0 | pass |
| Storm messages in 2 minutes (shared node) | <= 3 | 1 | pass |
| Storm messages in 2 minutes (shared registry) | <= 3 | 1 | pass |
| Storm messages in 2 minutes (multi-cause) | <= 13 | 6 | pass |
| Multi-cause storm causes reported | >= 10 | 10 | pass |
| Scenarios blaming a must-not-blame entity | <= 0 | 0 | pass |
| Quiet scenarios that alerted | <= 0 | 0 | pass |
| Scenarios over their message budget | <= 0 | 0 | pass |
| Scenarios paging more than once per outage | <= 0 | 0 | pass |
| Incidents left open after their members moved | <= 0 | 0 | pass |
| Recoveries said while members still fail | <= 0 | 0 | pass |
| Scenarios that passed and no longer do (ratchet) | <= 0 | 0 | pass |

31 of 31 gates pass. 828 labelled root cases in 878 scenarios; first-incident tier matches in 878 of 878 scenarios.
Staging day: 1824 notifications in 130h44m0s (14.0/h mean, peak hour 25); 0 from non-events, 0 of them notify or page.
Storm shared node: 1000 pods, 2 messages in total, 1 in the loudest 2 minutes.
Storm shared registry: 1000 pods, 1 messages in total, 1 in the loudest 2 minutes.
Storm multi-cause: 1000 pods from 10 causes, 10 of them reported, 9 messages in total, 6 in the loudest 2 minutes.
Slowest first messages: scale-down-moved-pod-never-starts page 12m0s, etcd-path-intermittent-during-maintenance notify 11m15s, node-join-never-ready page 10m15s, young-node-persists notify 10m0s, drain-beside-stuck-replacement page 10m0s.
Calibration: the labelled cases put high confidence at 0.55 (committed 0.70).
Right roots: 507 with a stated cause, 288 by falling back to the failing entity with no stated cause, 28 by stating no cause as labelled. Wrong roots stated at likely confidence or higher: 0 (5 hedges on ambiguous labels tolerated). Wrong root stated in some message: 18 scenarios, 10 of them labelled as legitimate revisions.
Time to first correct root: p95 4m15s, max 47m0s over 752 scenarios; slowest: sporadic-probe-timeouts-no-event 47m0s, sporadic-connection-reset-no-cause 42m15s, deployment-paused-with-pending-edit 31m15s, hpa-metrics-outlive-recovery 25m30s, log-missing-service 21m15s. Never stated: coupled-silent-image-fault, coupled-silent-image-fault-secret, coupled-silent-image-fault-state, adv-two-causes-stable, adv-two-causes-six-hours.

## Scenarios

| Scenario | Expected root | Actual root | Confidence | Tier (expected/actual) | Messages (max) | Result |
| --- | --- | --- | --- | --- | --- | --- |
| bad-rollout | `deployment/shop/payments` | `deployment/shop/payments` | high 0.91 | notify/notify | 1 (2) | pass |
| release-gitops-managed | `deployment/shop/orders` | `deployment/shop/orders` | high 0.91 | notify/notify | 1 (2) | pass |
| release-helm-managed | `deployment/shop/billing` | `deployment/shop/billing` | high 0.91 | notify/notify | 1 (2) | pass |
| secret-key-removed | `secret/shop/db-creds` | `secret/shop/db-creds` | high 1.00 | notify/notify | 1 (2) | pass |
| missing-secret | `secret/billing/stripe-api` | `secret/billing/stripe-api` | likely 0.69 | notify/notify | 1 (2) | pass |
| configmap-change | `configmap/web/frontend-config` | `configmap/web/frontend-config` | high 1.00 | notify/notify | 1 (2) | pass |
| config-run-as-non-root-rejected | `deployment/billing/ledger` | `deployment/billing/ledger` | high 1.00 | notify/notify | 1 (2) | pass |
| statefulset-release-rollback | `statefulset/streaming/queue` | `statefulset/streaming/queue` | high 0.77 | notify/notify | 2 (2) | pass |
| statefulset-release-previous-unseen | `statefulset/streaming/queue` | `statefulset/streaming/queue` | high 0.83 | notify/notify | 2 (2) | pass |
| daemonset-gitops-release | `daemonset/ops/agent` | `daemonset/ops/agent` | high 0.83 | notify/notify | 1 (2) | pass |
| release-after-completed-rollout | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.83 | notify/notify | 1 (2) | pass |
| statefulset-release-previous-fails | `statefulset/streaming/queue` | `statefulset/streaming/queue` | high 0.91 | notify/notify | 2 (2) | pass |
| networkpolicy-stops-covering-workload | `networkpolicy/shop/orders-egress` | `networkpolicy/shop/orders-egress` | high 0.80 | notify/notify | 1 (2) | pass |
| networkpolicy-selector-flips-back | quiet | - | - | -/digest | 0 (0) | pass |
| pod-restart-keeps-links | quiet | - | - | -/- | 0 (0) | pass |
| rollout-drops-service-dependency | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (3) | pass |
| rollout-drops-service-dependency-and-back | quiet | - | - | -/- | 0 (0) | pass |
| node-memory-pressure-eviction | `node//n1` | `node//n1` | high 1.00 | notify/notify | 2 (2) | pass |
| node-lost-notready | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (2) | pass |
| healthy-node-app-crash | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| cordoned-node-app-crash | `deployment/shop/worker` | `deployment/shop/worker (no cause)` | none | notify/notify | 1 (2) | pass |
| zone-failure | `zone//zone-b` | `zone//zone-b` | likely 0.69 | page/page | 2 (2) | pass |
| zone-outage-healthy-node-crash | `zone//zone-a`, `deployment/shop/api` | `zone//zone-a`, `deployment/shop/api (no cause)` | high 1.00 | page/page | 2 (4) | pass |
| coredns-down | `cluster-dns//cluster-dns` | `cluster-dns//cluster-dns` | high 1.00 | page/page | 1 (2) | pass |
| webhook-no-endpoints | `validatingwebhookconfiguration//policy-validator` | `validatingwebhookconfiguration//policy-validator` | high 1.00 | page/page | 2 (2) | pass |
| webhook-shared-backend | `service/policy/policy-svc`, `service/mesh-system/mesh-webhook` | `service/policy/policy-svc`, `service/mesh-system/mesh-webhook` | high 0.95 | notify/notify | 2 (2) | pass |
| quota-exhausted | `resourcequota/analytics/compute-quota` | `resourcequota/analytics/compute-quota` | high 1.00 | notify/notify | 1 (2) | pass |
| quota-forbids-pods | `resourcequota/analytics/zero-pods` | `resourcequota/analytics/zero-pods` | high 1.00 | notify/notify | 1 (2) | pass |
| metrics-apiservice-down | `apiservice//v1beta1.metrics.k8s.io` | `apiservice//v1beta1.metrics.k8s.io` | high 1.00 | notify/notify | 2 (2) | pass |
| registry-auth-failure | `registry//registry.corp.example` | `registry//registry.corp.example` | high 1.00 | notify/notify | 1 (2) | pass |
| image-typo | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.91 | notify/notify | 1 (2) | pass |
| networkpolicy-change | `networkpolicy/payments/restrict-egress` | `networkpolicy/payments/restrict-egress` | high 1.00 | notify/notify | 1 (2) | pass |
| operator-cr-stuck | `postgrescluster.postgres.example.com/data/orders-db` | `postgrescluster.postgres.example.com/data/orders-db` | high 0.70 | notify/notify | 1 (2) | pass |
| scheduler-insufficient-memory | `scheduling//Insufficient memory` | `scheduling//Insufficient memory` | high 1.00 | notify/notify | 1 (2) | pass |
| pvc-pending-immediate | `persistentvolumeclaim/data/orders-db` | `persistentvolumeclaim/data/orders-db` | high 0.90 | notify/notify | 1 (2) | pass |
| pvc-wffc-no-consumer | quiet | - | - | -/- | 0 (0) | pass |
| cronjob-invalid-schedule | `cronjob/reports/nightly-report` | `cronjob/reports/nightly-report` | high 0.70 | notify/notify | 1 (1) | pass |
| oom-limit-too-low | `deployment/shop/cart` | `deployment/shop/cart` | high 1.00 | notify/notify | 1 (2) | pass |
| flapping-workload | `deployment/shop/search` | `deployment/shop/search (no cause)` | none | notify/notify | 3 (3) | pass |
| two-independent-problems | `node//n1`, `deployment/shop/checkout` | `node//n1`, `deployment/shop/checkout` | high 1.00 | notify/notify | 2 (4) | pass |
| cause-revised | `node//n3` | `node//n3` | high 1.00 | notify/notify | 2 (3) | pass |
| cold-start-preexisting | `secret/billing/invoicer-creds`, `deployment/shop/api` | `secret/billing/invoicer-creds`, `deployment/shop/api (no cause)` | high 0.85 | notify/notify | 1 (1) | pass |
| start-mid-outage-missing-secret | `secret/billing/invoicer-creds` | `secret/billing/invoicer-creds` | high 0.85 | notify/notify | 1 (1) | pass |
| scheduler-down | `scheduler//kube-scheduler` | `scheduler//kube-scheduler` | high 1.00 | page/page | 2 (2) | pass |
| controller-manager-down | `controller-manager//kube-controller-manager` | `controller-manager//kube-controller-manager` | high 1.00 | page/page | 2 (2) | pass |
| etcd-down | `etcd//etcd` | `etcd//etcd` | high 0.90 | page/page | 1 (2) | pass |
| rbac-binding-removed | `rolebinding/shop/api-config-reader` | `rolebinding/shop/api-config-reader` | high 0.90 | notify/notify | 1 (2) | pass |
| rbac-change-unrelated-crash | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| service-selector-change | `service/shop/payments` | `service/shop/payments` | high 0.70 | notify/notify | 1 (2) | pass |
| service-backends-crash | `deployment/shop/payments` | `deployment/shop/payments` | high 1.00 | notify/notify | 1 (2) | pass |
| ingress-backend-missing | `service/shop/web-v2` | `service/shop/web-v2` | high 0.75 | page/page | 1 (2) | pass |
| ingress-tls-secret-missing | `secret/shop/web-tls` | `secret/shop/web-tls` | high 0.75 | notify/notify | 1 (2) | pass |
| route-not-accepted | `httproute.gateway.networking.k8s.io/shop/web` | `httproute.gateway.networking.k8s.io/shop/web` | high 0.70 | notify/notify | 1 (2) | pass |
| service-target-port-name-missing | `service/shop/api` | `service/shop/api` | high 0.70 | notify/notify | 1 (2) | pass |
| service-target-port-undeclared | quiet | - | - | -/- | 0 (0) | pass |
| ingress-backend-port-missing | `ingress/shop/shop` | `ingress/shop/shop` | high 0.70 | page/page | 1 (2) | pass |
| init-waits-on-broken-service | `deployment/shop/db` | `deployment/shop/db` | likely 0.55 | notify/notify | 2 (3) | pass |
| init-waits-on-healthy-service | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| init-finishes-slowly | quiet | - | - | -/- | 0 (0) | pass |
| certificate-expired | `secret/shop/bank-client-tls` | `secret/shop/bank-client-tls` | high 1.00 | notify/notify | 1 (2) | pass |
| certificate-expiring-app-crash | `deployment/shop/payments` | `deployment/shop/payments (no cause)` | none | notify/notify | 1 (2) | pass |
| certificate-expired-unused | `secret/shop/old-operator-tls` | `secret/shop/old-operator-tls (no cause)` | none | digest/digest | 1 (2) | pass |
| certificate-expired-ingress | `secret/shop/shop-tls` | `secret/shop/shop-tls (no cause)` | none | notify/notify | 1 (2) | pass |
| spot-node-removed | `node//n3` | `node//n3` | high 1.00 | notify/notify | 1 (2) | pass |
| node-removed-earlier-scale-up | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 1.00 | notify/notify | 1 (2) | pass |
| budget-blocks-drain | `poddisruptionbudget/shop/ledger` | `poddisruptionbudget/shop/ledger` | high 0.90 | notify/notify | 1 (2) | pass |
| single-replica-node-loss | `node//n1` | `node//n1` | high 1.00 | page/page | 3 (3) | pass |
| operator-cr-owned-failing | `kafkacluster.kafka.example.com/data/events` | `kafkacluster.kafka.example.com/data/events` | high 1.00 | notify/notify | 1 (2) | pass |
| operator-down-crs-stuck | `deployment/db-operators/pg-operator` | `deployment/db-operators/pg-operator` | high 0.85 | notify/notify | 2 (2) | pass |
| operator-cluster-cr-owned-failing | `messagebus.bus.example.com//events` | `messagebus.bus.example.com//events` | high 0.83 | notify/notify | 1 (1) | pass |
| probe-port-mismatch | `deployment/shop/api` | `deployment/shop/api` | high 0.99 | notify/notify | 1 (2) | pass |
| startup-budget-too-short | `deployment/shop/api` | `deployment/shop/api` | high 0.88 | notify/notify | 1 (2) | pass |
| init-container-fails | `deployment/shop/orders` | `deployment/shop/orders` | high 0.80 | notify/notify | 1 (2) | pass |
| sidecar-crash-loops | `deployment/shop/payments` | `deployment/shop/payments` | high 0.90 | notify/notify | 2 (2) | pass |
| liveness-kill-loop | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | notify/notify | 1 (2) | pass |
| liveness-single-kill | quiet | - | - | -/- | 0 (0) | pass |
| sidecar-starts-slowly | quiet | - | - | -/- | 0 (0) | pass |
| restart-rule-exit-code | quiet | - | - | -/- | 0 (0) | pass |
| restart-rule-other-code | `deployment/jobs/worker` | `deployment/jobs/worker (no cause)` | none | notify/notify | 1 (2) | pass |
| crash-loop-short-backoff | `deployment/shop/reports` | `deployment/shop/reports (no cause)` | none | notify/notify | 1 (2) | pass |
| borderline-policy-and-config | `networkpolicy/payments/restrict-egress` | `networkpolicy/payments/restrict-egress` | high 1.00 | notify/notify | 1 (2) | pass |
| borderline-selector-and-rollout | `service/shop/checkout` | `service/shop/checkout` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-operator-upgrade-and-edit | `postgrescluster.postgres.example.com/data/orders-db` | `postgrescluster.postgres.example.com/data/orders-db` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-schedule-and-secret | `cronjob/reports/nightly-report` | `cronjob/reports/nightly-report` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-late-config-reload | `configmap/batch/worker-config` | `configmap/batch/worker-config` | high 1.00 | notify/notify | 1 (2) | pass |
| borderline-route-and-gateway-edit | `httproute.gateway.networking.k8s.io/shop/web` | `httproute.gateway.networking.k8s.io/shop/web` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-two-policies | `networkpolicy/payments/allow-cache` | `networkpolicy/payments/allow-cache` | likely 0.69 | notify/notify | 1 (2) | pass |
| borderline-init-after-rotation | `secret/shop/orders-db` | `secret/shop/orders-db` | high 1.00 | notify/notify | 1 (2) | pass |
| claim-full-with-volume | `persistentvolumeclaim/observability/chunks` | `persistentvolumeclaim/observability/chunks` | high 0.85 | notify/notify | 1 (2) | pass |
| claim-full-unlisted-volume | `persistentvolumeclaim/messaging/broker-log` | `persistentvolumeclaim/messaging/broker-log` | high 0.70 | notify/notify | 1 (2) | pass |
| pvc-full | `persistentvolumeclaim/inventory/pgdata` | `persistentvolumeclaim/inventory/pgdata` | high 0.70 | notify/notify | 1 (2) | pass |
| claim-pins-pod | `persistentvolumeclaim/streaming/data-kafka-0` | `persistentvolumeclaim/streaming/data-kafka-0` | high 0.80 | notify/notify | 1 (2) | pass |
| multi-attach-held-by-old-pod | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 1 (2) | pass |
| multi-attach-clears-quickly | quiet | - | - | -/- | 0 (0) | pass |
| provisioner-controller-down | `deployment/kube-system/ebs-csi-controller` | `deployment/kube-system/ebs-csi-controller` | high 1.00 | notify/notify | 1 (2) | pass |
| provisioner-no-workload-found | `persistentvolumeclaim/data/orders-db` | `persistentvolumeclaim/data/orders-db (no cause)` | none | notify/notify | 1 (2) | pass |
| claim-no-storage-class | `persistentvolumeclaim/data/cache-0` | `persistentvolumeclaim/data/cache-0` | high 1.00 | notify/notify | 1 (2) | pass |
| provisioner-controller-scaled-to-zero | `deployment/kube-system/ebs-csi-controller` | `deployment/kube-system/ebs-csi-controller` | high 0.95 | notify/notify | 1 (2) | pass |
| provisioner-look-alike-workload | `persistentvolumeclaim/data/orders-db` | `persistentvolumeclaim/data/orders-db (no cause)` | none | notify/notify | 1 (2) | pass |
| volume-inodes-full | `persistentvolumeclaim/mail/spool` | `persistentvolumeclaim/mail/spool` | high 0.95 | notify/notify | 1 (2) | pass |
| volume-full-names-users | `persistentvolumeclaim/inventory/pgdata` | `persistentvolumeclaim/inventory/pgdata` | high 0.80 | notify/notify | 1 (2) | pass |
| volume-nearly-full-quiet | quiet | - | - | -/- | 0 (0) | pass |
| registry-login-expired | `registry//registry.corp.example` | `registry//registry.corp.example` | high 1.00 | notify/notify | 1 (2) | pass |
| registry-login-missing-secret | `secret/shop/regcred` | `secret/shop/regcred` | high 0.78 | notify/notify | 1 (2) | pass |
| registry-no-pull-secret | `registry//registry.corp.example` | `registry//registry.corp.example` | high 1.00 | notify/notify | 1 (2) | pass |
| registry-rate-limited | `registry//registry.corp.example` | `registry//registry.corp.example` | high 1.00 | notify/notify | 1 (2) | pass |
| registry-private-image-missing-secret | `secret/shop/regcred` | `secret/shop/regcred` | likely 0.63 | notify/notify | 1 (2) | pass |
| mutating-webhook-slow-backend | `mutatingwebhookconfiguration//mesh-injector` | `mutatingwebhookconfiguration//mesh-injector` | high 1.00 | page/page | 1 (2) | pass |
| validating-webhook-deadline | `validatingwebhookconfiguration//label-guard` | `validatingwebhookconfiguration//label-guard` | high 0.95 | page/page | 1 (2) | pass |
| webhook-timeout | `validatingwebhookconfiguration//image-policy` | `validatingwebhookconfiguration//image-policy` | high 1.00 | page/page | 1 (2) | pass |
| webhook-denies-pod | `validatingwebhookconfiguration//image-policy` | `validatingwebhookconfiguration//image-policy` | high 0.95 | notify/notify | 1 (2) | pass |
| webhook-idle-backend-gone | `validatingwebhookconfiguration//widget-operator` | `validatingwebhookconfiguration//widget-operator (no cause)` | none | notify/notify | 1 (1) | pass |
| webhook-idle-then-rejects | `validatingwebhookconfiguration//widget-operator` | `validatingwebhookconfiguration//widget-operator` | high 1.00 | page/page | 2 (3) | pass |
| webhook-tls-unknown-authority | `validatingwebhookconfiguration//image-policy` | `validatingwebhookconfiguration//image-policy` | high 1.00 | page/page | 1 (2) | pass |
| webhook-tls-serving-cert-expired | `validatingwebhookconfiguration//image-policy`, `secret/policy-system/policy-tls` | `validatingwebhookconfiguration//image-policy`, `secret/policy-system/policy-tls (no cause)` | high 1.00 | page/page | 2 (3) | pass |
| webhook-closed-on-pods-backend-gone | `validatingwebhookconfiguration//guard-pods` | `validatingwebhookconfiguration//guard-pods (no cause)` | none | notify/notify | 1 (2) | pass |
| webhook-closed-on-gone-crds-backend-gone | `validatingwebhookconfiguration//widget-guard` | `validatingwebhookconfiguration//widget-guard (no cause)` | none | digest/digest | 1 (2) | pass |
| webhook-ignore-on-pods-backend-gone | `validatingwebhookconfiguration//guard-pods` | `validatingwebhookconfiguration//guard-pods (no cause)` | none | digest/digest | 1 (2) | pass |
| pod-security-enforce-label-added | `namespace//reports` | `namespace//reports` | high 0.80 | notify/notify | 1 (2) | pass |
| limit-range-max-lowered | `limitrange/billing/container-bounds` | `limitrange/billing/container-bounds` | high 1.00 | notify/notify | 1 (2) | pass |
| limit-range-memory-max-lowered | `limitrange/shop/limits` | `limitrange/shop/limits` | high 1.00 | notify/notify | 1 (2) | pass |
| admission-policy-denies-pods | `validatingadmissionpolicy//require-team.example.com` | `validatingadmissionpolicy//require-team.example.com` | high 1.00 | notify/notify | 1 (2) | pass |
| node-condition-true-pods-unready | `node//n1` | `node//n1` | high 1.00 | notify/notify | 1 (2) | pass |
| node-agent-outside-kube-system | `daemonset/observability/logship` | `daemonset/observability/logship` | high 1.00 | notify/notify | 2 (2) | pass |
| node-agent-beside-lone-workload | `daemonset/observability/logship`, `deployment/shop/cart` | `daemonset/observability/logship (no cause)`, `deployment/shop/cart (no cause)` | none | notify/notify | 4 (4) | pass |
| node-agent-healthy-app-crashes | `deployment/shop/cart` | `deployment/shop/cart (no cause)` | none | notify/notify | 1 (2) | pass |
| csi-node-plugin-failing | `daemonset/kube-system/ebs-csi-node` | `daemonset/kube-system/ebs-csi-node` | possible 0.45 | notify/notify | 2 (3) | pass |
| kubelet-cert-expired | `node//n1` | `node//n1 (no cause)` | none | page/page | 1 (3) | pass |
| apiserver-throttling-stalls-controllers | `apiserver//kube-apiserver` | `apiserver//kube-apiserver` | high 0.75 | notify/notify | 2 (2) | pass |
| csr-denied | `certificatesigningrequest//csr-w7k2p` | `certificatesigningrequest//csr-w7k2p (no cause)` | none | notify/notify | 1 (2) | pass |
| flowschema-dangling-priority-level | `flowschema//batch-jobs` | `flowschema//batch-jobs (no cause)` | none | notify/notify | 1 (2) | pass |
| resourceclaim-unallocated-pod-pending | `resourceclaim/ml/gpu-claim` | `resourceclaim/ml/gpu-claim` | likely 0.69 | notify/notify | 2 (2) | pass |
| pdb-overlap-eviction-refused | `poddisruptionbudget/shop/cart-min`, `poddisruptionbudget/shop/cart-max` | `poddisruptionbudget/shop/cart-min (no cause)`, `poddisruptionbudget/shop/cart-max (no cause)` | none | notify/notify | 1 (2) | pass |
| statefulset-rollout-stuck | `statefulset/streaming/queue` | `statefulset/streaming/queue` | high 0.76 | notify/notify | 2 (2) | pass |
| loadbalancer-subnets-not-found | `service/shop/web` | `service/shop/web (no cause)` | none | notify/notify | 1 (3) | pass |
| loadbalancer-no-controller | `service/shop/api` | `service/shop/api (no cause)` | none | digest/digest | 0 (2) | pass |
| route-backend-renamed | `service/storefront/search-v3` | `service/storefront/search-v3` | high 0.90 | page/page | 1 (2) | pass |
| route-canary-backend-missing | `service/storefront/checkout-canary` | `service/storefront/checkout-canary` | high 0.90 | page/page | 1 (2) | pass |
| route-missing-backend | `service/shop/checkout-v2` | `service/shop/checkout-v2` | high 0.90 | page/page | 1 (2) | pass |
| autoscaler-ceiling-cpu | `horizontalpodautoscaler/orders/worker` | `horizontalpodautoscaler/orders/worker` | likely 0.70 | notify/notify | 1 (2) | pass |
| autoscaler-ceiling-requests | `horizontalpodautoscaler/search/api` | `horizontalpodautoscaler/search/api` | likely 0.70 | notify/notify | 1 (2) | pass |
| hpa-at-max | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web` | likely 0.70 | notify/notify | 1 (2) | pass |
| hpa-at-max-healthy-quiet | quiet | - | - | -/- | 0 (0) | pass |
| external-database-storm | `external-endpoint//db.example.com:5432` | `external-endpoint//db.example.com:5432` | high 1.00 | notify/notify | 1 (3) | pass |
| external-database-one-workload | `deployment/reports/builder` | `deployment/reports/builder (no cause)` | none | notify/notify | 1 (2) | pass |
| shared-panic-storm | `failure-signature//panic: license check failed for tenant #` | `failure-signature//panic: license check failed for tenant #` | high 0.75 | notify/notify | 1 (3) | pass |
| stack-frame-not-endpoint | `deployment/orders/api`, `deployment/billing/ledger` | `deployment/orders/api (no cause)`, `deployment/billing/ledger (no cause)` | none | notify/notify | 1 (4) | pass |
| redis-down-storm | `failure-signature//dial tcp <ip>:6379: connect: connection refused` | `failure-signature//dial tcp <ip>:6379: connect: connection refused` | high 0.75 | notify/notify | 1 (3) | pass |
| different-errors-stay-apart | `deployment/shop/cart`, `deployment/shop/search`, `deployment/billing/ledger` | `deployment/shop/cart (no cause)`, `deployment/shop/search (no cause)`, `deployment/billing/ledger (no cause)` | none | notify/notify | 1 (6) | pass |
| redis-storm-logs-only | `failure-signature//unhandled exception. stackexchange.redis.redisconnectionexception: it was not po` | `failure-signature//unhandled exception. stackexchange.redis.redisconnectionexception: it was not po` | high 0.75 | notify/notify | 1 (3) | pass |
| redis-storm-logs-late | `failure-signature//unhandled exception. stackexchange.redis.redisconnectionexception: it was not po` | `failure-signature//unhandled exception. stackexchange.redis.redisconnectionexception: it was not po` | high 0.75 | notify/notify | 1 (1) | pass |
| cronjob-last-run-failed | `cronjob/reports/weekly-export` | `cronjob/reports/weekly-export` | likely 0.55 | digest/digest | 0 (2) | pass |
| cron-runs-keep-failing | `cronjob/ops/stats-rollup` | `cronjob/ops/stats-rollup` | likely 0.69 | notify/notify | 3 (3) | pass |
| cache-unreachable-at-start | `deployment/web/api` | `deployment/web/api` | likely 0.69 | notify/notify | 1 (3) | pass |
| cache-retry-healthy-quiet | quiet | - | - | -/- | 0 (0) | pass |
| shared-node-suspected | `node//n1` | `node//n1` | possible 0.49 | notify/notify | 1 (2) | pass |
| separate-nodes-not-suspected | `deployment/shop/orders`, `deployment/shop/payments`, `deployment/shop/cart` | `deployment/shop/orders (no cause)`, `deployment/shop/payments (no cause)`, `deployment/shop/cart (no cause)` | none | notify/notify | 3 (4) | pass |
| co-onset-pair-shares-node | `node//n1` | `node//n1` | possible 0.49 | notify/notify | 1 (2) | pass |
| adv-co-onset-pair-minutes-apart | `deployment/shop/orders`, `deployment/shop/cart` | `deployment/shop/orders (no cause)`, `deployment/shop/cart (no cause)` | none | notify/notify | 2 (4) | pass |
| adv-co-onset-separate-nodes | `deployment/shop/orders`, `deployment/shop/cart` | `deployment/shop/orders (no cause)`, `deployment/shop/cart (no cause)` | none | notify/notify | 2 (4) | pass |
| dependency-refuses-connections | `external-endpoint//db.example.com:5432` | `external-endpoint//db.example.com:5432` | high 0.85 | notify/notify | 1 (2) | pass |
| dependency-answers | `deployment/orders/api` | `deployment/orders/api (no cause)` | none | notify/notify | 1 (2) | pass |
| dependency-host-from-configmap | `external-endpoint//db.example.com:5432` | `external-endpoint//db.example.com:5432` | high 0.85 | notify/notify | 1 (2) | pass |
| dependency-host-in-credential-key | `deployment/orders/api` | `deployment/orders/api (no cause)` | none | notify/notify | 1 (2) | pass |
| healthy-pods-error-burst | `deployment/shop/orders` | `deployment/shop/orders (no cause)` | none | notify/notify | 2 (2) | pass |
| healthy-pods-steady-errors | quiet | - | - | -/- | 0 (0) | pass |
| healthy-pods-small-burst | quiet | - | - | -/- | 0 (0) | pass |
| healthy-pods-burst-without-baseline | quiet | - | - | -/- | 0 (0) | pass |
| user-endpoints-named | `deployment/shop/payments` | `deployment/shop/payments` | high 1.00 | page/page | 1 (2) | pass |
| change-blast-overlap | `configmap/shop/app-defaults` | `configmap/shop/app-defaults` | high 1.00 | notify/notify | 1 (4) | pass |
| resolve-cause-recovered | `external-endpoint//db.example.com:5432` | `external-endpoint//db.example.com:5432` | high 0.85 | notify/notify | 2 (2) | pass |
| resolve-cause-still-present | `external-endpoint//db.example.com:5432` | `external-endpoint//db.example.com:5432` | high 0.85 | notify/notify | 1 (1) | pass |
| pool-boot-quiet | quiet | - | - | -/- | 0 (0) | pass |
| pool-boot-broken | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| boot-endpoints-quiet | quiet | - | - | -/- | 0 (0) | pass |
| boot-endpoints-still-down | `deployment/shop/api` | `deployment/shop/api` | likely 0.69 | notify/notify | 2 (2) | pass |
| release-regression | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.70 | notify/notify | 2 (2) | pass |
| release-steady | quiet | - | - | -/- | 0 (0) | pass |
| metrics-api-hpas | `deployment/kube-system/metrics-server` | `deployment/kube-system/metrics-server` | likely 0.69 | notify/notify | 2 (2) | pass |
| one-hpa-bad-target | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web (no cause)` | none | digest/digest | 0 (2) | pass |
| node-ready-blips | quiet | - | - | -/- | 0 (0) | pass |
| node-stays-down | `node//n1` | `node//n1` | high 0.85 | page/page | 1 (2) | pass |
| node-cni-not-ready | `node//n1` | `node//n1` | high 0.85 | page/page | 1 (2) | pass |
| node-lost-pinned-tenants | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (3) | pass |
| node-lost-after-mount-race | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (3) | pass |
| unknown-cause-recent-change | `unknown` | `deployment/shop/reports (no cause)` | none | notify/notify | 1 (2) | pass |
| unknown-cause-old-change | `unknown` | `deployment/shop/reports (no cause)` | none | notify/notify | 1 (2) | pass |
| fix-attempt-works | `deployment/shop/payments` | `deployment/shop/payments` | high 1.00 | notify/notify | 3 (4) | pass |
| fix-attempt-fails | `deployment/shop/payments` | `deployment/shop/payments` | high 1.00 | notify/notify | 4 (5) | pass |
| service-backend-flip | `deployment/shop/cart` | `deployment/shop/cart` | high 0.85 | notify/notify | 2 (2) | pass |
| webhook-flaps-hourly | `validatingwebhookconfiguration//image-policy` | `validatingwebhookconfiguration//image-policy` | high 0.95 | page/page | 1 (3) | pass |
| unschedulable-quantified | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 0.85 | notify/notify | 1 (2) | pass |
| unschedulable-taint | `scheduling//had untolerated taint` | `scheduling//had untolerated taint` | high 0.85 | notify/notify | 1 (2) | pass |
| fit-cpu-near-miss | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 0.85 | notify/notify | 1 (2) | pass |
| fit-taint-mismatch | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 0.85 | notify/notify | 1 (2) | pass |
| fit-volume-zone-no-nodes | `persistentvolumeclaim/analytics/data-etl` | `persistentvolumeclaim/analytics/data-etl` | high 0.95 | notify/notify | 1 (2) | pass |
| fit-autoscaler-scaling-up | quiet | - | - | -/- | 0 (0) | pass |
| fit-autoscaler-cannot-scale | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 0.85 | notify/notify | 1 (2) | pass |
| fit-volume-zone-node-full | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 0.85 | notify/notify | 1 (2) | pass |
| fit-local-volume-node-gone | `persistentvolumeclaim/analytics/data-etl` | `persistentvolumeclaim/analytics/data-etl` | high 0.95 | notify/notify | 1 (2) | pass |
| fit-host-port-taken | `scheduling//didn't have free ports for the requested pod ports` | `scheduling//didn't have free ports for the requested pod ports` | high 1.00 | notify/notify | 1 (2) | pass |
| ready-never | `deployment/shop/cert-controller` | `deployment/shop/cert-controller (no cause)` | none | notify/notify | 1 (3) | pass |
| ready-never-since-before-start | `deployment/shop/cert-controller` | `deployment/shop/cert-controller (no cause)` | none | notify/notify | 1 (3) | pass |
| hpa-target-missing | `horizontalpodautoscaler/istio-system/istiod` | `horizontalpodautoscaler/istio-system/istiod (no cause)` | none | digest/digest | 1 (2) | pass |
| ready-never-service | `deployment/external-secrets/cert-controller` | `deployment/external-secrets/cert-controller` | likely 0.55 | notify/notify | 1 (3) | pass |
| known-never-ready-past-boot | `deployment/external-secrets/cert-controller` | `deployment/external-secrets/cert-controller (no cause)` | none | notify/notify | 6 (8) | pass |
| readiness-error-logged | `deployment/shop/catalog` | `deployment/shop/catalog (no cause)` | none | notify/notify | 1 (3) | pass |
| readiness-outage-shared-database | `external-endpoint//10.0.9.40:5432` | `external-endpoint//10.0.9.40:5432` | high 0.95 | notify/notify | 1 (3) | pass |
| readiness-outage-node-out-of-addresses | `external-endpoint//10.0.9.40:5432`, `node//n1` | `external-endpoint//10.0.9.40:5432`, `node//n1` | high 0.80 | notify/notify | 2 (4) | pass |
| readiness-outage-member-leaves | `external-endpoint//10.0.9.40:5432`, `deployment/shop/payments`, `deployment/shop/reports` | `external-endpoint//10.0.9.40:5432`, `deployment/shop/payments (no cause)`, `deployment/shop/reports (no cause)` | high 0.80 | notify/notify | 2 (6) | pass |
| kind-only-joins-address | `external-endpoint//10.0.7.20:6379` | `external-endpoint//10.0.7.20:6379` | high 0.80 | notify/notify | 1 (2) | pass |
| kind-only-other-kind | `external-endpoint//10.0.7.20:6379`, `deployment/shop/pricing`, `deployment/shop/tracking` | `external-endpoint//10.0.7.20:6379`, `deployment/shop/pricing (no cause)`, `deployment/shop/tracking (no cause)` | high 1.00 | notify/notify | 1 (4) | pass |
| young-node-transient | quiet | - | - | -/- | 0 (0) | pass |
| young-node-persists | `failure-signature//dial tcp <ip>:6379: connect: connection refused` | `failure-signature//dial tcp <ip>:6379: connect: connection refused` | high 0.85 | notify/notify | 2 (2) | pass |
| young-and-old-nodes-fail | `external-endpoint//10.0.7.20:6379` | `external-endpoint//10.0.7.20:6379` | high 1.00 | notify/notify | 2 (2) | pass |
| log-missing-service | `deployment/shop/reports` | `deployment/shop/reports (no cause)` | none | digest/digest | 1 (2) | pass |
| log-missing-at-start-only | quiet | - | - | -/- | 0 (0) | pass |
| log-missing-service-exists | quiet | - | - | -/- | 0 (0) | pass |
| node-stall-clears-pods-still-unready | `node//n1`, `deployment/shop/api`, `deployment/shop/web`, `deployment/shop/worker` | `node//n1`, `deployment/shop/api (no cause)`, `deployment/shop/web (no cause)`, `deployment/shop/worker (no cause)` | possible 0.49 | notify/notify | 4 (6) | pass |
| start-error-release-liveness-137 | `deployment/shop/orders` | `deployment/shop/orders` | high 0.91 | notify/notify | 1 (2) | pass |
| start-error-release-liveness-143 | `deployment/shop/orders` | `deployment/shop/orders` | high 0.91 | notify/notify | 1 (2) | pass |
| exec-format-error-no-release | `deployment/shop/reports` | `deployment/shop/reports (no cause)` | none | notify/notify | 1 (2) | pass |
| crash-loop-ready-between-crashes | `deployment/shop/ledger` | `deployment/shop/ledger (no cause)` | none | notify/notify | 1 (2) | pass |
| readiness-answers-on-undeclared-port | `deployment/infra/cert-agent` | `deployment/infra/cert-agent (no cause)` | none | notify/notify | 1 (2) | pass |
| scaled-to-zero-with-release | `deployment/shop/api` | `deployment/shop/api` | high 0.70 | notify/notify | 1 (2) | pass |
| custom-condition-detail | `issuer.certs.example.com/certs/public` | `issuer.certs.example.com/certs/public (no cause)` | none | notify/notify | 1 (2) | pass |
| custom-generic-with-event | `issuer.certs.example.com/certs/internal` | `issuer.certs.example.com/certs/internal (no cause)` | none | notify/notify | 1 (2) | pass |
| own-volume-wait-at-start | quiet | - | - | -/- | 0 (0) | pass |
| own-volume-wait-too-long | `deployment/kwatch/kwatch` | `deployment/kwatch/kwatch (no cause)` | none | notify/notify | 2 (2) | pass |
| namespace-outage-groups | `deployment/shop/app0`, `deployment/shop/app1`, `deployment/shop/app2`, `deployment/shop/app3`, `deployment/shop/app4`, `deployment/shop/app5`, `deployment/shop/app6`, `deployment/shop/app7` | `deployment/shop/app0 (no cause)`, `deployment/shop/app1 (no cause)`, `deployment/shop/app2 (no cause)`, `deployment/shop/app3 (no cause)`, `deployment/shop/app4 (no cause)`, `deployment/shop/app5 (no cause)`, `deployment/shop/app6 (no cause)`, `deployment/shop/app7 (no cause)` | none | notify/notify | 1 (1) | pass |
| namespace-two-failures-stay-separate | `deployment/shop/app0`, `deployment/shop/app1` | `deployment/shop/app0 (no cause)`, `deployment/shop/app1 (no cause)` | none | notify/notify | 2 (4) | pass |
| page-flaps-reopens | `node//n1` | `node//n1` | high 0.85 | page/page | 4 (5) | pass |
| long-page-reminder | `node//n1` | `node//n1` | high 0.85 | page/page | 2 (3) | pass |
| digest-never-paged-resolve | `horizontalpodautoscaler/shop/orders` | `horizontalpodautoscaler/shop/orders (no cause)` | none | digest/digest | 2 (3) | pass |
| rollup-chronic-worsens | `deployment/shop/worker`, `deployment/shop/api` | `deployment/shop/worker (no cause)`, `deployment/shop/api (no cause)` | none | notify/notify | 2 (6) | pass |
| unusual-event-recurs | `service/shop/web` | `service/shop/web (no cause)` | none | digest/digest | 1 (4) | pass |
| moved-root-returns | `configmap/shop/settings` | `configmap/shop/settings` | high 1.00 | notify/notify | 3 (4) | pass |
| job-running-long | quiet | - | - | -/- | 0 (0) | pass |
| job-running-usual | quiet | - | - | -/- | 0 (0) | pass |
| scaled-to-zero-routed | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| scaled-to-zero-unrouted | quiet | - | - | -/- | 0 (0) | pass |
| scaled-to-zero-blue-green | quiet | - | - | -/- | 0 (0) | pass |
| scaled-to-zero-parked | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | digest/digest | 0 (2) | pass |
| first-rollout-never-healthy | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | notify/notify | 2 (3) | pass |
| oom-steady-climb | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| oom-by-node | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| oom-at-own-limit-during-node-oom | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| oom-by-overcommitted-node | `node//n1` | `node//n1` | likely 0.52 | notify/notify | 2 (3) | pass |
| limits-overcommitted-quiet | quiet | - | - | -/- | 0 (0) | pass |
| oom-limit-on-pressured-node | `deployment/billing/ledger`, `node//n3` | `deployment/billing/ledger (no cause)`, `node//n3 (no cause)` | none | notify/notify | 3 (5) | pass |
| oom-under-limit-on-pressured-node | `node//n3` | `node//n3` | likely 0.52 | notify/notify | 2 (3) | pass |
| namespace-delete-crashing-pods | `deployment/preview-4/mailer` | `deployment/preview-4/mailer (no cause)` | none | notify/notify | 1 (3) | pass |
| new-namespace-crash-before-root-ca | `deployment/team-new/api` | `deployment/team-new/api` | high 0.83 | notify/notify | 1 (2) | pass |
| short-name-quoted-in-error | `configmap/pipeline/api` | `configmap/pipeline/api` | high 1.00 | notify/notify | 1 (3) | pass |
| claim-names-missing-storage-class | `storageclass//nvme-archive` | `storageclass//nvme-archive` | high 1.00 | notify/notify | 1 (2) | pass |
| apiservice-backend-service-missing | `service/reports/report-adapter` | `service/reports/report-adapter` | high 0.95 | notify/notify | 1 (2) | pass |
| pod-evicted-over-ephemeral-limit | `deployment/render/thumbnailer` | `deployment/render/thumbnailer` | high 0.93 | notify/notify | 1 (3) | pass |
| policy-blocks-call-by-address | `networkpolicy/shop/dns-only-egress` | `networkpolicy/shop/dns-only-egress` | high 1.00 | notify/notify | 1 (2) | pass |
| liveness-timeout-cpu-throttled | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | notify/notify | 1 (2) | pass |
| kwatch-network-restricted | `kwatch//kwatch` | `kwatch//kwatch (no cause)` | none | digest/digest | 1 (2) | pass |
| kubelet-unreachable-window | `node//n1` | `node//n1 (no cause)` | none | digest/digest | 1 (2) | pass |
| crashloop-after-announce | `deployment/shop/pay` | `deployment/shop/pay (no cause)` | none | notify/notify | 2 (2) | pass |
| notify-flap-reopens | `deployment/shop/mailer` | `deployment/shop/mailer (no cause)` | none | notify/notify | 3 (4) | pass |
| cause-flip-stable | `deployment/shop/draftor` | `deployment/shop/draftor` | high 0.85 | notify/notify | 2 (3) | pass |
| known-boot-still-demotes | `deployment/shop/sync` | `deployment/shop/sync (no cause)` | none | notify/notify | 5 (5) | pass |
| known-crashloop-past-boot | `deployment/shop/sync` | `deployment/shop/sync (no cause)` | none | notify/notify | 6 (7) | pass |
| hpa-resolve-while-workload-broken | `deployment/shop/warehouse` | `deployment/shop/warehouse (no cause)` | none | notify/notify | 1 (2) | pass |
| crash-loop-during-node-replacement | `deployment/data/warehouse` | `deployment/data/warehouse (no cause)` | none | notify/notify | 6 (6) | pass |
| metrics-blip-short | quiet | - | - | -/- | 0 (0) | pass |
| metrics-blip-long | `apiservice//v1beta1.metrics.k8s.io` | `apiservice//v1beta1.metrics.k8s.io` | high 0.95 | digest/digest | 0 (3) | pass |
| digest-flap-reopens | `horizontalpodautoscaler/shop/orders` | `horizontalpodautoscaler/shop/orders (no cause)` | none | digest/digest | 2 (3) | pass |
| deprecated-api-still-called | quiet | - | - | -/- | 0 (0) | pass |
| upgrade-readiness-digest | quiet | - | - | -/- | 0 (0) | pass |
| apiserver-writes-slow | `apiserver//kube-apiserver` | `apiserver//kube-apiserver (no cause)` | none | digest/digest | 1 (2) | pass |
| slow-webhook-slows-api | `validatingwebhookconfiguration//policy-check` | `validatingwebhookconfiguration//policy-check` | high 0.85 | notify/notify | 1 (2) | pass |
| slow-webhook-blocks-deploys | `validatingwebhookconfiguration//policy-check` | `validatingwebhookconfiguration//policy-check` | high 1.00 | page/page | 2 (2) | pass |
| redis-down-linked-by-error | `deployment/shop/redis` | `deployment/shop/redis` | high 0.95 | notify/notify | 1 (2) | pass |
| ingress-service-no-endpoints-chain | `deployment/shop/cache` | `deployment/shop/cache` | high 0.80 | page/page | 1 (2) | pass |
| networkpolicy-blocks-dependency | `networkpolicy/shop/deny-all` | `networkpolicy/shop/deny-all` | high 1.00 | notify/notify | 1 (2) | pass |
| chain-postgres-api-ingress | `deployment/shop/postgres` | `deployment/shop/postgres` | high 0.80 | page/page | 2 (2) | pass |
| chain-wrong-time-order | `deployment/shop/api`, `deployment/shop/postgres` | `deployment/shop/api`, `deployment/shop/postgres` | high 0.85 | page/page | 2 (5) | pass |
| chain-capped-at-three-steps | `deployment/shop/postgres`, `service/shop/web` | `deployment/shop/postgres`, `service/shop/web` | high 0.80 | notify/notify | 5 (6) | pass |
| chain-timeline-line | `deployment/shop/postgres` | `deployment/shop/postgres` | high 0.80 | page/page | 3 (3) | pass |
| serving-last-replica-down | `deployment/shop/front` | `deployment/shop/front` | high 0.85 | page/page | 1 (2) | pass |
| internal-only-down | `deployment/shop/front` | `deployment/shop/front` | high 0.85 | notify/notify | 1 (2) | pass |
| rollout-healthy-slow | quiet | - | - | -/- | 0 (0) | pass |
| rollout-stalled | `deployment/shop/ledger` | `deployment/shop/ledger` | high 0.86 | notify/notify | 2 (3) | pass |
| stable-restarts-spike | `deployment/shop/orders` | `deployment/shop/orders (no cause)` | none | notify/notify | 1 (3) | pass |
| batch-restarts-normal | `deployment/shop/batch-worker` | `deployment/shop/batch-worker (no cause)` | none | digest/digest | 1 (2) | pass |
| config-breaks-image-runs-elsewhere | `configmap/web/frontend-config` | `configmap/web/frontend-config` | high 1.00 | notify/notify | 1 (2) | pass |
| node-local-healthy-replicas | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (2) | pass |
| bad-image-everywhere | `deployment/shop/checkout` | `deployment/shop/checkout` | high 1.00 | notify/notify | 1 (2) | pass |
| same-tag-new-digest | `deployment/shop/api` | `deployment/shop/api` | high 0.88 | notify/notify | 1 (3) | pass |
| same-tag-mixed-digests-healthy | quiet | - | - | -/- | 0 (0) | pass |
| zone-separates-replicas | `deployment/shop/api` | `deployment/shop/api` | high 0.72 | notify/notify | 1 (3) | pass |
| bad-config-value-change | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.96 | notify/notify | 1 (2) | pass |
| memory-limit-lowered-revision | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.96 | notify/notify | 1 (2) | pass |
| image-and-env-change-ranking | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.96 | notify/notify | 1 (2) | pass |
| ack-annotation | `deployment/shop/payments` | `deployment/shop/payments` | high 0.91 | notify/notify | 3 (3) | pass |
| ack-at-announce | `deployment/shop/payments` | `deployment/shop/payments` | high 0.91 | notify/notify | 1 (2) | pass |
| ack-stale-ignored | `deployment/shop/payments` | `deployment/shop/payments` | high 0.91 | notify/notify | 2 (2) | pass |
| pod-replaced-not-the-fix | `unknown` | `deployment/shop/reports (no cause)` | none | notify/notify | 2 (2) | pass |
| owner-routed | `deployment/shop/payments` | `deployment/shop/payments` | high 0.91 | notify/notify | 1 (2) | pass |
| config-edited-new-pods-fail | `configmap/shop/api-config` | `configmap/shop/api-config` | high 1.00 | notify/notify | 1 (2) | pass |
| config-edited-plain-mount | `configmap/shop/api-config` | `configmap/shop/api-config` | high 0.91 | notify/notify | 1 (2) | pass |
| config-versions-mixed-digest | quiet | - | - | -/- | 0 (0) | pass |
| wake-up-blips-held | quiet | - | - | -/- | 0 (0) | pass |
| wake-up-crash-loop | `deployment/shop/wake-app2` | `deployment/shop/wake-app2 (no cause)` | none | notify/notify | 1 (3) | pass |
| small-scale-up-node-lost | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (2) | pass |
| planned-scale-down-quiet | quiet | - | - | -/- | 0 (0) | pass |
| digest-ongoing | `service/kube-system/ingress-internal` | `service/kube-system/ingress-internal (no cause)` | none | digest/digest | 3 (3) | pass |
| exec-format-error-arm64 | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| crash-on-arm64-node-other-error | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| exec-format-error-release-retitle | `deployment/shop/reports` | `deployment/shop/reports` | high 0.82 | notify/notify | 1 (3) | pass |
| replicas-fail-only-on-arm64 | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| replicas-fail-on-mixed-nodes | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| pod-ips-exhausted | `zone//zone-a` | `zone//zone-a` | high 1.00 | notify/notify | 1 (3) | pass |
| preempted-by-higher-priority | `deployment/batch/importer` | `deployment/batch/importer` | high 1.00 | notify/notify | 1 (4) | pass |
| preempted-and-rescheduled | quiet | - | - | -/- | 0 (0) | pass |
| rollout-hard-kills | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 1 (3) | pass |
| rollout-hard-kills-healthy | quiet | - | - | -/digest | 0 (0) | pass |
| rollout-clean-stops | quiet | - | - | -/- | 0 (0) | pass |
| readiness-flapping | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| startup-readiness-churn | quiet | - | - | -/- | 0 (0) | pass |
| missing-service-lookup | `service/shop/paymnts` | `service/shop/paymnts` | high 1.00 | notify/notify | 1 (2) | pass |
| missing-service-unused | quiet | - | - | -/- | 0 (0) | pass |
| liveness-slow-start | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 2 (2) | pass |
| liveness-never-ready | `deployment/shop/api` | `deployment/shop/api` | likely 0.65 | notify/notify | 1 (2) | pass |
| liveness-scale-out-too-tight | `deployment/media/transcoder` | `deployment/media/transcoder` | high 1.00 | notify/notify | 2 (2) | pass |
| liveness-kill-after-start | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| liveness-cascade | `deployment/shop/db` | `deployment/shop/db` | likely 0.55 | notify/notify | 1 (2) | pass |
| liveness-cascade-healthy-dependency | `deployment/shop/api` | `deployment/shop/api` | likely 0.60 | notify/notify | 1 (2) | pass |
| namespace-stuck-terminating | `namespace//old-shop` | `namespace//old-shop (no cause)` | none | notify/notify | 1 (2) | pass |
| claim-stuck-deleting | `persistentvolumeclaim/shop/data` | `persistentvolumeclaim/shop/data (no cause)` | none | notify/notify | 1 (2) | pass |
| lease-holder-crash-loops | `deployment/kube-system/ccm` | `deployment/kube-system/ccm` | high 0.80 | notify/notify | 2 (2) | pass |
| lease-holder-gone | quiet | - | - | -/- | 0 (0) | pass |
| lease-taken-once-healthy-holder | quiet | - | - | -/- | 0 (0) | pass |
| cronjob-blocked-by-stuck-run | `cronjob/reports/nightly-report` | `cronjob/reports/nightly-report (no cause)` | none | notify/notify | 1 (2) | pass |
| pod-terminating-node-lost | `node//n1` | `node//n1` | high 0.95 | page/page | 2 (2) | pass |
| finalizer-controller-scaled-down | `deployment/widget-system/widget-controller` | `deployment/widget-system/widget-controller` | high 0.75 | digest/digest | 0 (2) | pass |
| finalizer-controller-running | `finalizer/widget-system/widgets.example.com/cleanup` | `finalizer/widget-system/widgets.example.com/cleanup` | high 0.70 | digest/digest | 0 (2) | pass |
| finalizer-controller-scaled-down-fresh | `deployment/widget-system/widget-controller` | `deployment/widget-system/widget-controller` | high 0.75 | notify/notify | 1 (2) | pass |
| attachment-volume-gone | `volumeattachment//csi-0a1b2c`, `volumeattachment//csi-3d4e5f` | `volumeattachment//csi-0a1b2c (no cause)`, `volumeattachment//csi-3d4e5f (no cause)` | none | digest/digest | 0 (2) | pass |
| attachment-node-gone | `volumeattachment//csi-6a7b8c`, `volumeattachment//csi-9d8e7f` | `volumeattachment//csi-6a7b8c (no cause)`, `volumeattachment//csi-9d8e7f (no cause)` | none | notify/notify | 1 (2) | pass |
| finalizer-lookalike-controllers | `finalizer/arc-system/actions.github.com/cleanup-protection` | `finalizer/arc-system/actions.github.com/cleanup-protection` | high 0.70 | digest/digest | 1 (2) | pass |
| finalizer-lookalike-no-rbac-view | `finalizer/arc-system/actions.github.com/cleanup-protection` | `finalizer/arc-system/actions.github.com/cleanup-protection` | high 0.70 | digest/digest | 1 (2) | pass |
| finalizer-controller-granted-the-group | `deployment/arc-system/gha-runner-scale-set-controller` | `deployment/arc-system/gha-runner-scale-set-controller` | high 0.75 | digest/digest | 1 (2) | pass |
| finalizer-owned-by-a-workload | `deployment/tools/gha-finalizer-reaper` | `deployment/tools/gha-finalizer-reaper` | high 0.75 | digest/digest | 1 (2) | pass |
| service-stuck-deleting-warning | `service/kube-addons/ingress-internal` | `service/kube-addons/ingress-internal (no cause)` | none | digest/digest | 1 (2) | pass |
| scale-failure-replaced | `horizontalpodautoscaler/staging/thirdparty` | `horizontalpodautoscaler/staging/thirdparty (no cause)` | none | notify/notify | 2 (3) | pass |
| node-replace-sandbox-blip | quiet | - | - | -/- | 0 (0) | pass |
| node-replace-sandbox-stuck | `node//n3` | `node//n3` | high 1.00 | page/page | 2 (2) | pass |
| unknown-waiting-reason | `deployment/shop/ledger` | `deployment/shop/ledger (no cause)` | none | notify/notify | 1 (2) | pass |
| registry-unavailable | `deployment/shop/invoices` | `deployment/shop/invoices (no cause)` | none | notify/notify | 1 (2) | pass |
| pod-cgroup-create-fails | `deployment/shop/search` | `deployment/shop/search (no cause)` | none | notify/notify | 1 (2) | pass |
| crash-loop-for-hours | `service/staging/regional` | `service/staging/regional` | high 0.85 | notify/notify | 1 (3) | pass |
| crash-quotes-other-error | `deployment/staging/warehouse` | `deployment/staging/warehouse (no cause)` | none | notify/notify | 1 (2) | pass |
| daemonset-node-replaced | quiet | - | - | -/- | 0 (0) | pass |
| daemonset-settled-node-fails | `daemonset/kube-system/aws-node` | `daemonset/kube-system/aws-node (no cause)` | none | notify/notify | 1 (2) | pass |
| pressure-spike-fresh-node | quiet | - | - | -/- | 0 (0) | pass |
| pressure-sustained-settled-node | `node//n1` | `node//n1 (no cause)` | none | digest/digest | 0 (2) | pass |
| spot-node-reclaimed-pods-move-quietly | quiet | - | - | -/- | 0 (0) | pass |
| spot-reclaim-after-notify | `deployment/shop/mailer` | `deployment/shop/mailer (no cause)` | none | notify/notify | 2 (2) | pass |
| ambiguous-init-secret-edited | `secret/shop/orders-db` | `secret/shop/orders-db` | high 0.85 | notify/notify | 1 (3) | pass |
| ambiguous-init-no-change | `deployment/shop/orders` | `deployment/shop/orders` | high 0.80 | notify/notify | 1 (3) | pass |
| ambiguous-cron-secret-edited | `secret/reports/export-creds` | `secret/reports/export-creds` | high 0.80 | notify/notify | 1 (2) | pass |
| ambiguous-cron-no-change | `cronjob/reports/nightly-export` | `cronjob/reports/nightly-export` | likely 0.55 | notify/notify | 1 (2) | pass |
| ambiguous-oom-near-limit | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 3 (3) | pass |
| ambiguous-oom-half-limit | `node//n1` | `node//n1` | likely 0.52 | notify/notify | 2 (3) | pass |
| ambiguous-ceiling-slow-dependency | `external-endpoint//db.example.com:5432` | `external-endpoint//db.example.com:5432 (no cause)` | none | notify/notify | 2 (2) | pass |
| ambiguous-ceiling-dependency-fine | `horizontalpodautoscaler/orders/worker` | `horizontalpodautoscaler/orders/worker` | likely 0.70 | notify/notify | 1 (2) | pass |
| ambiguous-image-and-config-image | `deployment/shop/checkout` | `deployment/shop/checkout` | high 1.00 | notify/notify | 1 (2) | pass |
| ambiguous-image-and-config-config | `configmap/shop/checkout-config` | `configmap/shop/checkout-config` | high 1.00 | notify/notify | 1 (2) | pass |
| ambiguous-rollout-one-pressured-node | `node//n1` | `node//n1 (no cause)` | none | notify/notify | 2 (3) | pass |
| ambiguous-rollout-every-node | `deployment/shop/payments` | `deployment/shop/payments` | high 1.00 | notify/notify | 2 (3) | pass |
| coupled-config-rejected-value | `configmap/billing/ledger-config` | `configmap/billing/ledger-config` | high 1.00 | notify/notify | 1 (2) | pass |
| coupled-image-nil-pointer | `deployment/stock/sync` | `deployment/stock/sync` | high 1.00 | notify/notify | 1 (2) | pass |
| cron-secret-edited-hours-before | `secret/billing/sync-key` | `secret/billing/sync-key` | high 0.80 | notify/notify | 1 (2) | pass |
| cron-secret-edited-then-succeeded | `cronjob/billing/invoice-sync` | `cronjob/billing/invoice-sync` | likely 0.55 | notify/notify | 1 (2) | pass |
| coupled-silent-image-fault | `deployment/mail/intake` | `configmap/mail/intake-config` | likely 0.69 | notify/notify | 1 (2) | pass |
| coupled-silent-config-fault | `configmap/post/intake-config` | `configmap/post/intake-config` | likely 0.69 | notify/notify | 1 (2) | pass |
| coupled-silent-image-fault-secret | `deployment/mail/relay` | `secret/mail/relay-secret` | likely 0.69 | notify/notify | 1 (2) | pass |
| coupled-silent-secret-fault-secret | `secret/post/relay-secret` | `secret/post/relay-secret` | likely 0.69 | notify/notify | 1 (2) | pass |
| coupled-silent-image-fault-state | `deployment/mail/ledger` | `configmap/mail/ledger-config` | likely 0.69 | notify/notify | 1 (2) | pass |
| coupled-silent-config-fault-state | `configmap/post/ledger-config` | `configmap/post/ledger-config` | likely 0.69 | notify/notify | 1 (2) | pass |
| node-lost-owner-routed | `node//n1` | `node//n1` | high 1.00 | page/page | 3 (4) | pass |
| cni-agent-crash-quoted | `node//n1` | `node//n1` | high 1.00 | notify/notify | 2 (4) | pass |
| coredns-down-no-probe | `cluster-dns//cluster-dns` | `cluster-dns//cluster-dns` | high 0.90 | notify/notify | 1 (3) | pass |
| dependency-clusterip-dial | `deployment/data/postgres` | `deployment/data/postgres` | high 1.00 | notify/notify | 1 (6) | pass |
| node-blip-then-down | `node//n1` | `node//n1 (no cause)` | none | page/page | 4 (5) | pass |
| nodes-lost-same-zone | `node//n1` | `node//n1 (no cause)` | none | page/page | 2 (6) | pass |
| nodes-lost-three-zones | `node//n1` | `node//n1 (no cause)` | none | page/page | 3 (6) | pass |
| node-pressure-evicted-linger | `node//n1` | `node//n1` | high 1.00 | notify/notify | 2 (2) | pass |
| node-late-cause-after-crashes | `node//n1` | `node//n1` | high 1.00 | page/page | 3 (4) | pass |
| node-join-slow-ready | quiet | - | - | -/- | 0 (0) | pass |
| node-join-never-ready | `node//n3` | `node//n3 (no cause)` | none | page/page | 1 (2) | pass |
| ingress-to-typo-selector | `service/shop/api` | `service/shop/api (no cause)` | none | page/page | 1 (2) | pass |
| ingress-to-scaled-to-zero-service | quiet | - | - | -/- | 0 (0) | pass |
| job-retry-succeeds | quiet | - | - | -/- | 0 (0) | pass |
| job-retries-all-fail | `job/batch/report` | `job/batch/report (no cause)` | none | notify/notify | 2 (3) | pass |
| node-join-kubelet-notices | quiet | - | - | -/- | 0 (0) | pass |
| pod-dns-search-line-notice | quiet | - | - | -/digest | 0 (0) | pass |
| chronic-never-ready-deployment | `deployment/shop/cert-controller` | `deployment/shop/cert-controller (no cause)` | none | notify/notify | 1 (3) | pass |
| webhook-backend-namespace-gone | `service/policy-gone/policy-svc` | `service/policy-gone/policy-svc` | high 0.95 | notify/notify | 1 (2) | pass |
| webhook-ignore-no-endpoints | `validatingwebhookconfiguration//widget-operator` | `validatingwebhookconfiguration//widget-operator (no cause)` | none | digest/digest | 1 (2) | pass |
| webhook-ignore-beside-closed | `validatingwebhookconfiguration//mixed-policy` | `validatingwebhookconfiguration//mixed-policy (no cause)` | none | digest/digest | 1 (2) | pass |
| node-readonly-filesystem | `node//n1` | `node//n1 (no cause)` | none | digest/digest | 1 (2) | pass |
| daemonset-misscheduled-stays | `daemonset/monitoring/agent` | `daemonset/monitoring/agent (no cause)` | none | digest/digest | 1 (2) | pass |
| custom-task-succeeded-false | `taskrun.tekton.example.com/ci/build-image` | `taskrun.tekton.example.com/ci/build-image (no cause)` | none | notify/notify | 1 (2) | pass |
| custom-controller-failed-events | `order.acme.example.com/infra/web-tls` | `order.acme.example.com/infra/web-tls (no cause)` | none | digest/digest | 2 (2) | pass |
| custom-healthy-warning-stream | quiet | - | - | -/- | 0 (0) | pass |
| custom-warnings-then-recovery | `order.acme.example.com/infra/web-tls` | `order.acme.example.com/infra/web-tls (no cause)` | none | digest/digest | 0 (2) | pass |
| namespace-outage-with-services | `deployment/shop/app0`, `deployment/shop/app1`, `deployment/shop/app2`, `deployment/shop/app3`, `deployment/shop/app4`, `deployment/shop/app5` | `deployment/shop/app0`, `deployment/shop/app1`, `deployment/shop/app2`, `deployment/shop/app3`, `deployment/shop/app4`, `deployment/shop/app5` | likely 0.55 | notify/notify | 1 (2) | pass |
| namespace-outage-pages-once | `deployment/shop/app0`, `deployment/shop/app1`, `deployment/shop/app2`, `deployment/shop/app3`, `deployment/shop/app4`, `deployment/shop/app5` | `deployment/shop/app0`, `deployment/shop/app1`, `deployment/shop/app2`, `deployment/shop/app3`, `deployment/shop/app4`, `deployment/shop/app5` | likely 0.55 | page/page | 9 (10) | pass |
| two-teams-two-pages | `deployment/billing/api`, `deployment/shop/api` | `deployment/billing/api (no cause)`, `deployment/shop/api (no cause)` | none | page/page | 3 (4) | pass |
| zone-nodes-lost-staggered | `zone//zone-a` | `zone//zone-a` | likely 0.69 | page/page | 3 (4) | pass |
| node-recovers-workload-stays-down | `deployment/shop/cart` | `deployment/shop/cart (no cause)` | none | page/page | 3 (3) | pass |
| endpoint-answers-probe-three-workloads | `unknown` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| node-egress-external-endpoint | `node//n3` | `node//n3` | possible 0.49 | notify/notify | 1 (3) | pass |
| node-local-network-fault | `node//n3` | `node//n3` | possible 0.49 | notify/notify | 1 (3) | pass |
| node-local-unmodelled-address | `node//n3` | `node//n3` | possible 0.49 | notify/notify | 1 (3) | pass |
| kube-proxy-one-node-crash | `daemonset/kube-system/kube-proxy` | `daemonset/kube-system/kube-proxy` | high 0.70 | notify/notify | 4 (4) | pass |
| node-agent-with-healthy-peers | `deployment/shop/api`, `deployment/shop/web` | `deployment/shop/api (no cause)`, `deployment/shop/web (no cause)` | none | notify/notify | 1 (5) | pass |
| node-agent-crash-csi-error-names-other-driver | `unknown` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (4) | pass |
| csi-node-plugin-missing-on-node | `daemonset/kube-system/ebs-csi-node` | `daemonset/kube-system/ebs-csi-node` | possible 0.45 | notify/notify | 1 (3) | pass |
| cr-controller-crash-manager-is-deployment-name | `deployment/gadget-system/gadget-controller` | `deployment/gadget-system/gadget-controller` | high 0.85 | notify/notify | 2 (3) | pass |
| cr-controller-crash-default-manager | `deployment/gadget-system/gadget-controller` | `deployment/gadget-system/gadget-controller` | high 0.85 | notify/notify | 2 (3) | pass |
| cr-single-resource-default-manager | `deployment/gadget-system/gadget-controller` | `deployment/gadget-system/gadget-controller` | high 0.78 | notify/notify | 1 (3) | pass |
| disk-hog-moves-nodes | `deployment/media/ingest` | `deployment/media/ingest` | likely 0.68 | notify/notify | 2 (4) | pass |
| service-account-deleted-before-scale-out | `serviceaccount/shop/api` | `serviceaccount/shop/api` | high 1.00 | notify/notify | 1 (2) | pass |
| pull-secret-rotated-hours-before-scale-out | `secret/payments/pull-creds` | `secret/payments/pull-creds` | high 0.73 | notify/notify | 1 (2) | pass |
| pull-without-secret-beside-logins | `deployment/labs/newa`, `deployment/labs/newb` | `deployment/labs/newa`, `deployment/labs/newb` | high 1.00 | notify/notify | 1 (4) | pass |
| secret-edited-after-code-crash | `deployment/shop/orders` | `deployment/shop/orders (no cause)` | none | notify/notify | 3 (4) | pass |
| secret-rotated-before-code-crash | `deployment/shop/orders` | `deployment/shop/orders (no cause)` | none | notify/notify | 1 (4) | pass |
| image-typo-after-secret-edit | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.76 | notify/notify | 1 (2) | pass |
| policy-created-before-code-crash | `deployment/shop/orders` | `deployment/shop/orders (no cause)` | none | notify/notify | 1 (4) | pass |
| role-created-before-forbidden-crash | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (4) | pass |
| binding-of-another-account-removed | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| undeclared-probe-port-code-crash | `deployment/shop/orders` | `deployment/shop/orders (no cause)` | none | notify/notify | 1 (4) | pass |
| adv-rollout-then-node-lost | `node//n2` | `node//n2` | high 1.00 | page/page | 2 (3) | pass |
| adv-oom-two-victims | `deployment/billing/ledger`, `node//n1` | `deployment/billing/ledger (no cause)`, `node//n1` | none | notify/notify | 3 (5) | pass |
| adv-clean-drain-quiet | quiet | - | - | -/- | 0 (0) | pass |
| adv-pdb-zero-quiet | quiet | - | - | -/- | 0 (0) | pass |
| adv-bad-config-and-stuck-drain | `configmap/billing/invoicer-config`, `poddisruptionbudget/shop/ledger` | `configmap/billing/invoicer-config`, `poddisruptionbudget/shop/ledger` | high 1.00 | notify/notify | 1 (5) | pass |
| adv-bad-config-clean-drain | `configmap/billing/invoicer-config` | `configmap/billing/invoicer-config` | high 1.00 | notify/notify | 1 (2) | pass |
| adv-rbac-two-bindings | `rolebinding/shop/api-config-reader` | `rolebinding/shop/api-config-reader` | high 0.90 | notify/notify | 1 (2) | pass |
| adv-forbidden-partner-api | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-similar-name-configmap | `deployment/payments/payments` | `deployment/payments/payments (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-two-causes-stable | `unknown` | `configmap/shop/checkout-flags` | likely 0.69 | notify/notify | 1 (2) | pass |
| adv-configmap-recreated-blip | quiet | - | - | -/- | 0 (0) | pass |
| adv-dns-nxdomain-probed | `external-endpoint//sandbox.partner-gateway.example:443` | `external-endpoint//sandbox.partner-gateway.example:443` | high 1.00 | notify/notify | 1 (2) | pass |
| adv-dns-nxdomain-unprobed | `deployment/shop/checkout` | `deployment/shop/checkout (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-dns-nxdomain-two-namespaces | `deployment/shop/checkout`, `deployment/billing/invoicer` | `deployment/shop/checkout (no cause)`, `deployment/billing/invoicer (no cause)` | none | notify/notify | 1 (4) | pass |
| adv-init-wait-selector-edited | `service/shop/queue` | `service/shop/queue` | high 0.85 | notify/notify | 2 (3) | pass |
| adv-webhook-timeout-six-namespaces | `validatingwebhookconfiguration//image-policy` | `validatingwebhookconfiguration//image-policy` | high 1.00 | page/page | 1 (2) | pass |
| adv-same-error-different-targets | `deployment/shop/db-a`, `service/billing/db-b` | `deployment/shop/db-a`, `service/billing/db-b` | high 0.85 | notify/notify | 1 (4) | pass |
| adv-hpa-max-throttled | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web` | high 0.75 | notify/notify | 2 (2) | pass |
| adv-quota-after-hpa-scale-up | `resourcequota/batch/pod-cap` | `resourcequota/batch/pod-cap` | high 1.00 | notify/notify | 1 (2) | pass |
| adv-statefulset-zone-without-nodes | `persistentvolumeclaim/streaming/data-kafka-2` | `persistentvolumeclaim/streaming/data-kafka-2` | high 1.00 | notify/notify | 1 (2) | pass |
| adv-readiness-flap-under-load | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| adv-node-lost-and-app-crash | `deployment/shop/api`, `node//n2` | `deployment/shop/api`, `node//n2` | high 0.79 | notify/notify | 3 (6) | pass |
| adv-lookalike-redis | `deployment/shop/cart` | `deployment/shop/cart (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-silent-exit-no-evidence | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-exit137-not-oom | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-quota-full-no-refusal-quiet | quiet | - | - | -/- | 0 (0) | pass |
| adv-secret-rotation-healthy-quiet | quiet | - | - | -/- | 0 (0) | pass |
| adv-same-node-independent-faults | `deployment/billing/ledger`, `deployment/shop/search` | `deployment/billing/ledger (no cause)`, `deployment/shop/search (no cause)` | none | notify/notify | 1 (4) | pass |
| adv-clusterrolebinding-deleted | `clusterrolebinding//ops-pod-reader` | `clusterrolebinding//ops-pod-reader` | high 0.90 | notify/notify | 1 (2) | pass |
| adv-two-registries-independent | `registry//registry.corp.example`, `registry//registry-1.docker.io` | `registry//registry.corp.example`, `registry//registry-1.docker.io` | likely 0.65 | notify/notify | 1 (4) | pass |
| adv-quota-full-no-refusal-two-days | quiet | - | - | -/- | 0 (0) | pass |
| adv-secret-rotated-but-invisible | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-throttled-but-healthy-quiet | quiet | - | - | -/- | 0 (0) | pass |
| adv-rollout-in-flight-node-lost | `node//n2` | `node//n2` | high 1.00 | page/page | 2 (4) | pass |
| adv-node-blip-then-bad-rollout | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 3 (3) | pass |
| adv-two-causes-six-hours | `unknown` | `configmap/shop/checkout-flags` | likely 0.69 | notify/notify | 1 (3) | pass |
| adv-netpol-unrelated-crash | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| adv-dependency-rolled-unrelated-panic | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| called-backend-not-ready-timeout | `deployment/shop/ledger` | `deployment/shop/ledger` | high 1.00 | notify/notify | 2 (3) | pass |
| called-backend-not-ready-refused | `deployment/shop/ledger` | `deployment/shop/ledger` | high 1.00 | notify/notify | 2 (3) | pass |
| attachments-detach-error-disk-gone | `volumeattachment//csi-7a7a7a`, `volumeattachment//csi-8b8b8b` | `volumeattachment//csi-7a7a7a (no cause)`, `volumeattachment//csi-8b8b8b (no cause)` | none | notify/notify | 1 (2) | pass |
| lb-delete-resource-in-use | `service/shop/web` | `service/shop/web (no cause)` | none | digest/digest | 0 (2) | pass |
| lb-delete-throttled-once | quiet | - | - | -/- | 0 (0) | pass |
| liveness-kill-http-500 | `deployment/shop/catalog` | `deployment/shop/catalog (no cause)` | none | notify/notify | 1 (2) | pass |
| liveness-kills-slow-start-no-startup-probe | `deployment/shop/reports` | `deployment/shop/reports` | high 0.90 | notify/notify | 1 (2) | pass |
| liveness-refused-while-app-crashes | `deployment/shop/notifier` | `deployment/shop/notifier (no cause)` | none | notify/notify | 1 (2) | pass |
| release-old-revision-rescheduled | quiet | - | - | -/- | 0 (0) | pass |
| sandbox-first-attempt-retried | quiet | - | - | -/- | 0 (0) | pass |
| young-node-address-warm-up | quiet | - | - | -/- | 0 (0) | pass |
| scale-up-backend-starting | quiet | - | - | -/- | 0 (0) | pass |
| operator-leftovers-grouped | `deployment/policy-system/policy-controller`, `deployment/policy-system/trace-operator` | `deployment/policy-system/policy-controller (no cause)`, `deployment/policy-system/trace-operator (no cause)` | none | digest/digest | 1 (2) | pass |
| old-leftovers-age-out | quiet | - | - | -/- | 0 (0) | pass |
| chronic-issuer-lowered | `issuer.certs.example.com/certs/letsencrypt` | `issuer.certs.example.com/certs/letsencrypt (no cause)` | none | digest/digest | 1 (2) | pass |
| fresh-issuer-notifies | `issuer.certs.example.com/certs/letsencrypt` | `issuer.certs.example.com/certs/letsencrypt (no cause)` | none | notify/notify | 1 (2) | pass |
| old-leftover-joins-late-group | `validatingwebhookconfiguration//policy-resource`, `deployment/policy-system/policy-controller` | `validatingwebhookconfiguration//policy-resource (no cause)`, `deployment/policy-system/policy-controller (no cause)` | none | notify/notify | 2 (2) | pass |
| scale-down-moved-pod-held | quiet | - | - | -/- | 0 (0) | pass |
| scale-down-moved-pod-never-starts | `node//n1` | `node//n1` | likely 0.65 | page/page | 1 (2) | pass |
| scale-down-crash-loop-pages | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | page/page | 1 (2) | pass |
| scale-down-unrelated-outage-told | `deployment/shop/pay` | `deployment/shop/pay (no cause)` | none | notify/notify | 1 (2) | pass |
| scale-down-moved-pod-pull-fails | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | page/page | 1 (2) | pass |
| hpa-no-ready-pods-joins-workload | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | notify/notify | 1 (2) | pass |
| hpa-metrics-beside-serving | `deployment/shop/web`, `horizontalpodautoscaler/shop/web` | `deployment/shop/web (no cause)`, `horizontalpodautoscaler/shop/web (no cause)` | none | notify/notify | 1 (3) | pass |
| hpa-metrics-outlive-recovery | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web (no cause)` | none | notify/notify | 3 (3) | pass |
| config-change-long-crash-loop | `configmap/web/frontend-config` | `configmap/web/frontend-config` | high 1.00 | notify/notify | 1 (3) | pass |
| secret-key-removed-hours-earlier | `secret/shop/db-creds` | `secret/shop/db-creds` | high 1.00 | notify/notify | 1 (2) | pass |
| secret-key-never-present | `secret/shop/db-creds` | `secret/shop/db-creds` | high 0.85 | notify/notify | 1 (2) | pass |
| secret-edited-hours-earlier-own-crash | `deployment/shop/orders` | `deployment/shop/orders (no cause)` | none | notify/notify | 1 (2) | pass |
| pull-secret-absent-pods-healthy | quiet | - | - | -/- | 0 (0) | pass |
| pull-secret-absent-slow-start | quiet | - | - | -/- | 0 (0) | pass |
| pull-secret-absent-own-crash | `deployment/shop/reports` | `deployment/shop/reports (no cause)` | none | notify/notify | 1 (2) | pass |
| cron-run-failed-then-recovered | `cronjob/batch/rollup` | `cronjob/batch/rollup` | high 0.70 | notify/notify | 3 (3) | pass |
| job-helper-container-finished | quiet | - | - | -/- | 0 (0) | pass |
| job-create-refused-once | quiet | - | - | -/- | 0 (0) | pass |
| cronjob-suspended-on-purpose | quiet | - | - | -/- | 0 (0) | pass |
| job-created-suspended | quiet | - | - | -/- | 0 (0) | pass |
| statefulset-ordered-scale-up | quiet | - | - | -/- | 0 (0) | pass |
| statefulset-scale-up-stalls | `statefulset/streaming/queue` | `statefulset/streaming/queue (no cause)` | none | notify/notify | 1 (2) | pass |
| cold-start-shared-start-error | `failure-signature//stackexchange.redis.redisconnectionexception: it was not possible to connect to` | `failure-signature//stackexchange.redis.redisconnectionexception: it was not possible to connect to` | high 0.70 | notify/notify | 2 (3) | pass |
| node-stall-from-capped-container | quiet | - | - | -/- | 0 (0) | pass |
| node-stall-from-busy-node | `node//n1` | `node//n1 (no cause)` | none | digest/digest | 0 (2) | pass |
| kubelet-pull-qps-exceeded | `node//n3` | `node//n3` | high 1.00 | notify/notify | 1 (2) | pass |
| release-previous-version-fails | `deployment/shop/orders` | `deployment/shop/orders` | high 0.87 | notify/notify | 1 (2) | pass |
| scaled-to-zero-then-released | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| probe-refused-then-http-500 | `deployment/external-secrets/cert-controller` | `deployment/external-secrets/cert-controller (no cause)` | none | notify/notify | 1 (3) | pass |
| crash-loop-before-node-drain | `deployment/shop/report` | `deployment/shop/report (no cause)` | none | notify/notify | 1 (3) | pass |
| crash-everywhere-node-pressure | `deployment/shop/worker` | `deployment/shop/worker (no cause)` | none | notify/notify | 1 (4) | pass |
| helm-upgrade-failed | `helmrelease/shop/payments` | `helmrelease/shop/payments (no cause)` | none | notify/notify | 1 (2) | pass |
| helm-pending-upgrade-stuck | `helmrelease/shop/payments` | `helmrelease/shop/payments (no cause)` | none | notify/notify | 1 (2) | pass |
| helm-upgrade-deployed-quiet | quiet | - | - | -/- | 0 (0) | pass |
| helm-pending-upgrade-short-quiet | quiet | - | - | -/- | 0 (0) | pass |
| oom-flat-at-limit-sizing | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| oom-without-usage-history | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| liveness-timeout-at-cpu-limit | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | notify/notify | 1 (2) | pass |
| liveness-timeout-under-cpu-limit | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | notify/notify | 1 (2) | pass |
| oom-at-limitrange-cap | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (3) | pass |
| release-blamed-possible | `deployment/shop/checkout` | `deployment/shop/checkout` | likely 0.69 | notify/notify | 1 (2) | pass |
| release-previous-never-ready | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.82 | notify/notify | 1 (2) | pass |
| service-stuck-deleting-old-quiet | quiet | - | - | -/digest | 0 (0) | pass |
| release-source-linked | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.96 | notify/notify | 1 (2) | pass |
| release-source-invalid | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.96 | notify/notify | 1 (2) | pass |
| helm-upgrade-breaks-workload | `helmrelease/shop/payments` | `helmrelease/shop/payments` | high 1.00 | notify/notify | 1 (3) | pass |
| helm-upgrade-fails-beside-other-crash | `helmrelease/shop/payments`, `deployment/shop/orders` | `helmrelease/shop/payments (no cause)`, `deployment/shop/orders` | none | notify/notify | 2 (4) | pass |
| helm-upgrade-fails-after-crash | `deployment/shop/payments`, `helmrelease/shop/payments` | `deployment/shop/payments`, `helmrelease/shop/payments (no cause)` | high 0.98 | notify/notify | 2 (4) | pass |
| helm-old-failure-healthy | quiet | - | - | -/- | 0 (0) | pass |
| helm-old-failure-workload-down | `deployment/shop/payments`, `helmrelease/shop/payments` | `deployment/shop/payments`, `helmrelease/shop/payments (no cause)` | high 0.98 | notify/notify | 2 (4) | pass |
| drain-beside-stuck-replacement | `node//n3` | `node//n3` | high 1.00 | page/page | 1 (2) | pass |
| drain-evictions-find-no-room | `node//n3` | `node//n3` | high 1.00 | notify/notify | 2 (2) | pass |
| sched-node-selector-no-match | `scheduling//didn't match Pod's node affinity/selector` | `scheduling//didn't match Pod's node affinity/selector` | high 1.00 | notify/notify | 1 (2) | pass |
| sched-anti-affinity-pending | `scheduling//didn't match pod anti-affinity rules` | `scheduling//didn't match pod anti-affinity rules` | high 1.00 | notify/notify | 1 (2) | pass |
| release-command-not-found | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| release-read-only-root-filesystem | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| hpa-target-no-cpu-request | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web (no cause)` | none | digest/digest | 0 (2) | pass |
| node-pid-pressure | `node//n1` | `node//n1` | high 1.00 | notify/notify | 1 (2) | pass |
| node-pleg-not-healthy | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (2) | pass |
| node-max-pods-reached | `scheduling//Too many pods` | `scheduling//Too many pods` | high 0.85 | notify/notify | 1 (2) | pass |
| node-no-execute-taint-evicts | quiet | - | - | -/- | 0 (0) | pass |
| node-no-execute-taint-tolerated | quiet | - | - | -/- | 0 (0) | pass |
| node-cni-ip-conflict | `node//n2` | `node//n2` | high 1.00 | notify/notify | 2 (2) | pass |
| node-clock-skew-cert-rejected | `node//n3` | `node//n3` | high 1.00 | page/page | 2 (2) | pass |
| pvc-volume-resize-failed | `persistentvolumeclaim/inventory/pgdata` | `persistentvolumeclaim/inventory/pgdata` | likely 0.53 | notify/notify | 2 (3) | pass |
| pv-released-blocks-claim | `persistentvolume//pv-db-0` | `persistentvolume//pv-db-0` | high 1.00 | notify/notify | 1 (2) | pass |
| cron-active-deadline-exceeded | `cronjob/ops/stats-rollup` | `cronjob/ops/stats-rollup` | high 0.70 | notify/notify | 1 (3) | pass |
| pod-stuck-terminating-finalizer | `finalizer/streaming/storage.example.com/volume-detach` | `finalizer/streaming/storage.example.com/volume-detach` | high 0.85 | notify/notify | 1 (2) | pass |
| release-post-start-hook-fails | `deployment/shop/api` | `deployment/shop/api` | high 0.77 | notify/notify | 2 (2) | pass |
| release-invalid-image-name | `deployment/shop/api` | `deployment/shop/api` | high 0.77 | notify/notify | 1 (2) | pass |
| release-image-pull-deadline | `deployment/shop/etl` | `deployment/shop/etl` | high 0.77 | notify/notify | 1 (2) | pass |
| release-seccomp-profile-missing | `deployment/shop/api` | `deployment/shop/api` | high 0.77 | notify/notify | 1 (2) | pass |
| release-pull-secret-wrong-type | `secret/shop/regcred` | `secret/shop/regcred` | high 1.00 | notify/notify | 1 (2) | pass |
| release-priority-class-missing | `priorityclass//batch-critical` | `priorityclass//batch-critical` | high 0.75 | notify/notify | 1 (2) | pass |
| deployment-paused-with-pending-edit | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | digest/digest | 1 (2) | pass |
| deployment-paused-no-edit | quiet | - | - | -/- | 0 (0) | pass |
| release-configmap-volume-missing | `configmap/shop/app-config` | `configmap/shop/app-config` | high 0.85 | notify/notify | 1 (2) | pass |
| release-node-name-missing | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| network-packet-loss-to-node | `unknown` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| etcd-database-space-exceeded | `etcd//etcd` | `etcd//etcd (no cause)` | none | page/page | 1 (2) | pass |
| ingress-class-unknown | `ingress/shop/shop` | `ingress/shop/shop (no cause)` | none | notify/notify | 1 (2) | pass |
| gateway-class-missing | `gateway.gateway.networking.k8s.io/shop/public` | `gateway.gateway.networking.k8s.io/shop/public (no cause)` | none | notify/notify | 1 (2) | pass |
| crd-conversion-webhook-gone | `customresourcedefinition//widgets.acme.example` | `customresourcedefinition//widgets.acme.example` | high 1.00 | notify/notify | 1 (2) | pass |
| release-scheduler-name-not-running | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| release-pvc-not-found | `persistentvolumeclaim/shop/data-0` | `persistentvolumeclaim/shop/data-0` | high 0.85 | notify/notify | 1 (2) | pass |
| release-probe-exec-binary-missing | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-probe-scheme-mismatch | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-script-not-executable | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-exit-139-segfault | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 1 (2) | pass |
| release-exit-zero-crash-loop | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-sidecar-port-conflict | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-bind-low-port-non-root | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-subpath-not-a-directory | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| release-memory-emptydir-oom | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-token-automount-disabled | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-legacy-token-secret-missing | `secret/shop/api-token` | `secret/shop/api-token` | high 0.85 | notify/notify | 1 (2) | pass |
| release-sysctl-forbidden | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| release-host-network-cluster-dns | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-registry-unknown-authority | `registry//registry.corp.example` | `registry//registry.corp.example` | high 0.90 | notify/notify | 1 (2) | pass |
| hostnetwork-port-taken-on-node | `node//n1` | `node//n1` | high 0.79 | notify/notify | 1 (3) | pass |
| hostpath-directory-missing-on-node | `node//n3` | `node//n3` | high 0.79 | notify/notify | 1 (2) | pass |
| node-runtime-not-ready-after-upgrade | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (2) | pass |
| upgrade-in-tree-volume-plugin-removed | `deployment/shop/ledger` | `deployment/shop/ledger (no cause)` | none | notify/notify | 1 (2) | pass |
| node-kubelet-upgrade-restarts-all-pods | quiet | - | - | -/- | 0 (0) | pass |
| kubelet-admission-out-of-cpu-pileup | `node//n2` | `node//n2` | high 0.95 | notify/notify | 1 (2) | pass |
| kubelet-admission-single-reject-replaced | quiet | - | - | -/- | 0 (0) | pass |
| node-image-gc-failed-disk-pressure | `node//n1` | `node//n1` | high 1.00 | notify/notify | 1 (2) | pass |
| node-conntrack-table-full | `node//n2` | `node//n2 (no cause)` | none | notify/notify | 1 (3) | pass |
| node-max-map-count-too-low | `node//n3` | `node//n3` | high 0.79 | notify/notify | 1 (2) | pass |
| networkpolicy-blocks-apiserver-access | `networkpolicy/shop/dns-only-egress` | `networkpolicy/shop/dns-only-egress` | high 1.00 | notify/notify | 1 (2) | pass |
| node-pause-image-pull-fails | `node//n4` | `node//n4` | possible 0.49 | notify/notify | 1 (3) | pass |
| nfs-server-unreachable-three-namespaces | `unknown` | `deployment/billing/invoices (no cause)` | none | notify/notify | 1 (3) | pass |
| coredns-corefile-edit-breaks-dns | `configmap/kube-system/coredns` | `configmap/kube-system/coredns` | high 1.00 | page/page | 2 (2) | pass |
| node-local-dns-down-on-two-nodes | `daemonset/kube-system/node-local-dns` | `daemonset/kube-system/node-local-dns` | high 1.00 | notify/notify | 1 (3) | pass |
| dns-storm-from-looping-workload | `deployment/kube-system/coredns` | `deployment/kube-system/coredns` | high 1.00 | page/page | 4 (4) | pass |
| kube-proxy-not-syncing-one-node | `daemonset/kube-system/kube-proxy` | `daemonset/kube-system/kube-proxy` | high 1.00 | notify/notify | 2 (3) | pass |
| runaway-cronjob-fills-nodes | `cronjob/batch/fanout` | `cronjob/batch/fanout (no cause)` | none | notify/notify | 2 (3) | pass |
| preemption-chain-across-namespaces | `deployment/ml/trainer` | `deployment/ml/trainer` | high 1.00 | notify/notify | 2 (4) | pass |
| control-plane-certificate-expired | `apiserver//kube-apiserver` | `apiserver//kube-apiserver (no cause)` | none | page/page | 1 (2) | pass |
| namespace-stuck-on-unavailable-apiservice | `apiservice//v1beta1.metrics.k8s.io` | `apiservice//v1beta1.metrics.k8s.io` | likely 0.60 | notify/notify | 3 (3) | pass |
| pvc-rwx-class-supports-only-rwo | `persistentvolumeclaim/shop/shared-uploads` | `persistentvolumeclaim/shop/shared-uploads` | high 1.00 | notify/notify | 1 (2) | pass |
| node-max-volume-count-reached | `scheduling//exceed max volume count` | `scheduling//exceed max volume count` | high 0.85 | notify/notify | 1 (2) | pass |
| hpa-flapping-scale-up-down | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web` | high 0.80 | digest/digest | 0 (2) | pass |
| hpa-single-scale-up-quiet | quiet | - | - | -/- | 0 (0) | pass |
| deployment-replicas-reset-by-gitops | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | digest/digest | 0 (2) | pass |
| statefulset-governing-service-missing | `service/streaming/queue` | `service/streaming/queue` | high 1.00 | notify/notify | 1 (2) | pass |
| node-kernel-task-hung | `node//n2` | `node//n2` | high 1.00 | notify/notify | 1 (2) | pass |
| node-xfs-shutdown | `node//n3` | `node//n3` | high 1.00 | notify/notify | 1 (3) | pass |
| node-hardware-error-before-loss | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (2) | pass |
| node-kubelet-restarts-repeatedly | `node//n1` | `node//n1` | high 1.00 | notify/notify | 1 (2) | pass |
| node-kubelet-single-restart-quiet | quiet | - | - | -/- | 0 (0) | pass |
| node-unregister-netdevice-stuck | `node//n3` | `node//n3 (no cause)` | none | notify/notify | 1 (3) | pass |
| node-corrupt-image-layer | `node//n2` | `node//n2` | high 0.79 | notify/notify | 1 (2) | pass |
| daemonset-pod-pending-node-full | `node//n4` | `node//n4` | high 0.75 | notify/notify | 1 (2) | pass |
| daemonset-pod-pending-briefly-quiet | quiet | - | - | -/- | 0 (0) | pass |
| daemonset-failed-pod-recreated-loop | `node//n2` | `node//n2` | high 0.75 | notify/notify | 1 (2) | pass |
| pdb-selector-matches-no-pods | `poddisruptionbudget/shop/search-pdb` | `poddisruptionbudget/shop/search-pdb (no cause)` | none | notify/notify | 1 (2) | pass |
| pdb-percentage-on-unmanaged-pods | `poddisruptionbudget/shop/queue-pdb` | `poddisruptionbudget/shop/queue-pdb (no cause)` | none | digest/digest | 0 (2) | pass |
| job-max-failed-indexes-exceeded | `job/batch/import-shards` | `job/batch/import-shards` | high 0.85 | notify/notify | 1 (2) | pass |
| job-pod-failure-policy-fail-job | `job/batch/migrate` | `job/batch/migrate` | high 0.85 | notify/notify | 1 (2) | pass |
| job-failed-then-ttl-deleted | `job/batch/rebuild-index` | `job/batch/rebuild-index` | high 0.85 | notify/notify | 1 (2) | pass |
| job-completed-then-ttl-deleted-quiet | quiet | - | - | -/- | 0 (0) | pass |
| cronjob-unknown-time-zone | `cronjob/reports/nightly-digest` | `cronjob/reports/nightly-digest (no cause)` | none | notify/notify | 2 (2) | pass |
| pod-security-refuses-statefulset-and-job | `namespace//streaming` | `namespace//streaming` | high 1.00 | notify/notify | 1 (3) | pass |
| statefulset-claim-refused-by-storage-quota | `resourcequota/streaming/storage-quota` | `resourcequota/streaming/storage-quota` | high 1.00 | notify/notify | 1 (2) | pass |
| quota-requires-requests-and-limits | `resourcequota/shop/compute-quota` | `resourcequota/shop/compute-quota` | high 1.00 | notify/notify | 1 (2) | pass |
| quota-scoped-to-priority-class-exhausted | `resourcequota/shop/high-priority-pods` | `resourcequota/shop/high-priority-pods` | high 1.00 | notify/notify | 1 (2) | pass |
| limit-range-ratio-refuses-rollout | `limitrange/shop/limits` | `limitrange/shop/limits` | high 0.80 | notify/notify | 1 (2) | pass |
| pod-security-latest-version-three-namespaces | `namespace//team-a`, `namespace//team-b`, `namespace//team-c` | `namespace//team-a`, `namespace//team-b`, `namespace//team-c` | high 0.80 | notify/notify | 1 (4) | pass |
| admission-policy-param-deleted-denies-all | `validatingadmissionpolicy//require-owner.example.com` | `validatingadmissionpolicy//require-owner.example.com` | high 1.00 | notify/notify | 1 (3) | pass |
| ephemeral-volume-claim-refused-by-quota | `resourcequota/shop/storage-quota` | `resourcequota/shop/storage-quota (no cause)` | none | notify/notify | 2 (2) | pass |
| deployment-replicaset-create-refused-by-quota | `resourcequota/shop/object-counts` | `resourcequota/shop/object-counts (no cause)` | none | notify/notify | 2 (2) | pass |
| loadbalancer-service-no-eligible-nodes | `unknown` | `service/shop/web (no cause)` | none | digest/digest | 0 (2) | pass |
| node-pod-cidr-not-available | `node//n4` | `node//n4` | high 1.00 | notify/notify | 1 (3) | pass |
| pvc-lost-volume-deleted | `persistentvolumeclaim/shop/data` | `persistentvolumeclaim/shop/data` | high 0.80 | notify/notify | 1 (2) | pass |
| pvc-static-volume-too-small | `persistentvolumeclaim/shop/data-nfs` | `persistentvolumeclaim/shop/data-nfs` | high 0.90 | notify/notify | 1 (2) | pass |
| node-lost-rwo-volume-multi-attach | `node//n1` | `node//n1` | high 0.89 | page/page | 2 (3) | pass |
| node-shutdown-terminates-bare-pod | `node//n1` | `node//n1` | likely 0.60 | notify/notify | 2 (2) | pass |
| node-shutdown-pods-replaced-quiet | quiet | - | - | -/- | 0 (0) | pass |
| pod-resize-deferred-no-room | `node//n1` | `node//n1` | high 0.75 | notify/notify | 1 (2) | pass |
| pod-scheduling-gate-never-removed | `deployment/shop/worker` | `deployment/shop/worker (no cause)` | none | notify/notify | 1 (2) | pass |
| pod-scheduling-gate-removed-quickly-quiet | quiet | - | - | -/- | 0 (0) | pass |
| release-readiness-gate-never-set | `deployment/shop/api` | `deployment/shop/api` | high 0.86 | notify/notify | 1 (2) | pass |
| device-plugin-down-gpu-unavailable | `daemonset/kube-system/nvidia-device-plugin` | `daemonset/kube-system/nvidia-device-plugin (no cause)` | none | notify/notify | 2 (3) | pass |
| hpa-duplicate-for-one-deployment | `horizontalpodautoscaler/shop/web-extra` | `horizontalpodautoscaler/shop/web-extra (no cause)` | none | digest/digest | 0 (2) | pass |
| hpa-container-metric-name-missing | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web (no cause)` | none | digest/digest | 0 (2) | pass |
| statefulset-ordinal-name-taken-by-bare-pod | `pod/data/db-1` | `pod/data/db-1 (no cause)` | none | digest/digest | 0 (2) | pass |
| headless-service-hides-not-ready-peers | `service/streaming/queue` | `service/streaming/queue` | high 1.00 | notify/notify | 1 (2) | pass |
| coredns-cluster-role-edited | `clusterrole//system:coredns` | `clusterrole//system:coredns` | high 1.00 | page/page | 1 (2) | pass |
| pod-cannot-reach-own-service-address | `unknown` | `deployment/shop/ledger (no cause)` | none | notify/notify | 1 (2) | pass |
| failed-mount-not-registered-clears-quiet | quiet | - | - | -/- | 0 (0) | pass |
| pod-dns-nameserver-limit-notice | quiet | - | - | -/digest | 0 (0) | pass |
| sporadic-probe-timeouts-no-event | `unknown` | `deployment/shop/payments (no cause)` | none | digest/digest | 0 (2) | pass |
| rollout-read-write-once-pod-conflict | `deployment/shop/db` | `deployment/shop/db` | high 1.00 | notify/notify | 1 (2) | pass |
| pvc-larger-than-any-node-capacity | `persistentvolumeclaim/shop/scratch` | `persistentvolumeclaim/shop/scratch` | high 1.00 | notify/notify | 1 (2) | pass |
| pod-pending-claim-being-deleted | `persistentvolumeclaim/shop/pgdata` | `persistentvolumeclaim/shop/pgdata` | high 0.95 | notify/notify | 1 (2) | pass |
| pv-terminating-while-claim-bound | `persistentvolumeclaim/shop/pgdata` | `persistentvolumeclaim/shop/pgdata` | high 0.75 | digest/digest | 1 (2) | pass |
| release-pod-affinity-target-scaled-to-zero | `deployment/shop/cache` | `deployment/shop/cache` | high 0.95 | notify/notify | 1 (2) | pass |
| pod-priority-class-preemption-never | `priorityclass//batch-nopreempt` | `priorityclass//batch-nopreempt` | high 0.80 | notify/notify | 1 (2) | pass |
| kubelet-preempts-pods-for-critical-agent | `unknown` | `deployment/billing/payments (no cause)` | none | notify/notify | 2 (3) | pass |
| kubelet-topology-affinity-error-pileup | `node//n2` | `node//n2` | high 0.93 | notify/notify | 1 (3) | pass |
| kubelet-rejects-pod-node-shutting-down-quiet | quiet | - | - | -/- | 0 (0) | pass |
| node-runtime-class-handler-missing | `node//n3` | `node//n3` | high 0.79 | notify/notify | 1 (2) | pass |
| release-user-namespace-volume-unsupported | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| release-image-volume-not-found | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| workload-hits-pod-pids-limit | `deployment/shop/worker` | `deployment/shop/worker (no cause)` | none | notify/notify | 1 (2) | pass |
| node-hugepages-allocatable-lost | `node//n2` | `node//n2` | likely 0.60 | notify/notify | 1 (2) | pass |
| node-invalid-disk-capacity | `node//n1` | `node//n1 (no cause)` | none | digest/digest | 0 (2) | pass |
| node-allocatable-enforcement-failed | `node//n1` | `node//n1 (no cause)` | none | digest/digest | 1 (2) | pass |
| dra-driver-down-prepare-fails | `daemonset/kube-system/gpu-dra-driver` | `daemonset/kube-system/gpu-dra-driver (no cause)` | none | notify/notify | 1 (2) | pass |
| pod-resize-error-in-progress | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | digest/digest | 0 (2) | pass |
| node-swap-notices-healthy-pods-quiet | quiet | - | - | -/digest | 0 (0) | pass |
| release-pod-level-memory-limit-oom | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-sidecar-startup-probe-fails | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| job-success-policy-terminates-other-pods-quiet | quiet | - | - | -/- | 0 (0) | pass |
| job-replacement-waits-for-terminating-pod | `node//n1` | `node//n1` | high 0.80 | page/page | 2 (3) | pass |
| job-suspended-while-running-quiet | quiet | - | - | -/- | 0 (0) | pass |
| ephemeral-debug-container-fails-quiet | quiet | - | - | -/- | 0 (0) | pass |
| release-grpc-liveness-probe-tls-server | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-crash-loop-reduced-backoff | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| all-nodes-lost-evictions-halted | `node//n1` | `node//n1 (no cause)` | none | page/page | 2 (2) | pass |
| evicted-pods-linger-after-recovery-quiet | quiet | - | - | -/- | 0 (0) | pass |
| scheduler-leader-lost-after-etcd-slow | `etcd//etcd` | `etcd//etcd (no cause)` | none | page/page | 2 (2) | pass |
| old-nodes-lose-network-together | `node//n1` | `node//n1 (no cause)` | none | page/page | 3 (3) | pass |
| node-kubelet-status-updates-lag | `node//n1` | `node//n1` | high 0.90 | notify/notify | 1 (2) | pass |
| memory-hog-moves-and-takes-down-nodes | `deployment/batch/ingest` | `deployment/batch/ingest` | high 0.98 | page/page | 3 (4) | pass |
| all-replicas-on-one-node-lost | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (3) | pass |
| daemonset-without-limits-system-oom-cycle | `daemonset/observability/logship` | `daemonset/observability/logship (no cause)` | none | notify/notify | 1 (3) | pass |
| api-endpoint-change-nodes-blip-together-quiet | quiet | - | - | -/- | 0 (0) | pass |
| node-cni-agent-saturated-by-connections | `daemonset/kube-system/cni-agent` | `daemonset/kube-system/cni-agent` | high 1.00 | notify/notify | 2 (3) | pass |
| daemonset-selector-matches-no-node-after-upgrade | `daemonset/kube-system/net-agent` | `daemonset/kube-system/net-agent` | high 1.00 | notify/notify | 1 (3) | pass |
| coredns-memory-limit-oom-loop | `deployment/kube-system/coredns` | `deployment/kube-system/coredns` | high 1.00 | page/page | 2 (3) | pass |
| terminating-pods-hold-addresses | `finalizer/jobs/jobs.example.com/cleanup` | `finalizer/jobs/jobs.example.com/cleanup` | high 0.88 | notify/notify | 1 (3) | pass |
| webhook-backend-evicted-cannot-return | `validatingwebhookconfiguration//policy-guard` | `validatingwebhookconfiguration//policy-guard` | high 1.00 | page/page | 2 (3) | pass |
| daemonset-rollout-overloads-api-then-dns | `apiserver//kube-apiserver` | `apiserver//kube-apiserver (no cause)` | none | notify/notify | 5 (5) | pass |
| crd-instances-grow-unbounded-list-times-out | `unknown` | `deployment/auth/dex-operator (no cause)` | none | notify/notify | 1 (2) | pass |
| etcd-restarted-by-own-liveness-probe | `etcd//etcd` | `etcd//etcd` | high 1.00 | page/page | 2 (3) | pass |
| endpoint-slice-lists-pods-that-are-gone | `unknown` | `deployment/billing/invoices (no cause)` | none | notify/notify | 2 (3) | pass |
| drain-long-grace-period-within-grace-quiet | quiet | - | - | -/- | 0 (0) | pass |
| secret-controller-scaled-down-new-pods-wait | `unknown` | `deployment/billing/invoices (no cause)` | none | notify/notify | 1 (3) | pass |
| statefulset-recreated-restarts-all-pods | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 1 (3) | pass |
| one-replica-takes-all-connections-oom | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| cronjob-every-minute-fills-api-server-memory | `pod/kube-system/kube-apiserver-n1` | `pod/kube-system/kube-apiserver-n1` | likely 0.50 | notify/notify | 4 (4) | pass |
| etcd-path-intermittent-during-maintenance | `etcd//etcd` | `etcd//etcd (no cause)` | none | notify/notify | 1 (2) | pass |
| release-server-binds-loopback | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 2 (2) | pass |
| release-server-binds-other-ip-family | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| service-protocol-udp-for-tcp-app | `service/shop/api` | `service/shop/api` | high 1.00 | notify/notify | 1 (2) | pass |
| kube-dns-service-recreated-new-cluster-ip | `service/kube-system/kube-dns` | `service/kube-system/kube-dns` | high 1.00 | notify/notify | 1 (3) | pass |
| kube-dns-service-selector-matches-nothing | `service/kube-system/kube-dns` | `service/kube-system/kube-dns` | high 1.00 | page/page | 2 (3) | pass |
| coredns-upstream-resolver-unreachable | `deployment/kube-system/coredns` | `deployment/kube-system/coredns` | high 0.70 | notify/notify | 1 (3) | pass |
| upgrade-overwrites-coredns-stub-domain | `configmap/kube-system/coredns` | `configmap/kube-system/coredns` | high 1.00 | notify/notify | 1 (3) | pass |
| statefulset-fsgroup-recursive-mount-timeout | `statefulset/streaming/queue` | `statefulset/streaming/queue (no cause)` | none | notify/notify | 1 (2) | pass |
| node-runtime-container-name-reserved | `node//n3` | `node//n3` | high 0.79 | notify/notify | 1 (2) | pass |
| bootstrap-token-expired-nodes-cannot-join | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 1.00 | notify/notify | 1 (2) | pass |
| node-left-cordoned-after-maintenance | `node//n3` | `node//n3` | high 0.85 | notify/notify | 1 (2) | pass |
| bare-pod-evicted-never-replaced | `pod/shop/cache-standalone` | `pod/shop/cache-standalone (no cause)` | none | notify/notify | 1 (3) | pass |
| bare-pod-restart-never-failed | `pod/batch/migrate-once` | `pod/batch/migrate-once (no cause)` | none | notify/notify | 1 (2) | pass |
| drain-waits-on-pod-without-owner | `unknown` | `pod/ops/debug-shell (no cause)` | none | digest/digest | 1 (2) | pass |
| metrics-server-cannot-verify-kubelet-certificate | `unknown` | `horizontalpodautoscaler/shop/web (no cause)` | none | digest/digest | 0 (2) | pass |
| kubelet-serving-csr-pending-metrics-fail | `certificatesigningrequest//csr-4k8x2` | `certificatesigningrequest//csr-4k8x2 (no cause)` | none | digest/digest | 0 (2) | pass |
| page-cache-memory-pressure-evicts-reader | `deployment/media/reader` | `deployment/media/reader` | high 0.73 | notify/notify | 1 (2) | pass |
| evictions-in-succession-disk-threshold | `node//n1` | `node//n1` | high 1.00 | notify/notify | 2 (3) | pass |
| node-pressure-condition-flaps | `node//n1` | `node//n1 (no cause)` | none | digest/digest | 1 (2) | pass |
| release-exec-probe-hangs-pid-pressure | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 2 (2) | pass |
| statefulset-rollout-wedged-after-template-fix | `statefulset/streaming/queue` | `statefulset/streaming/queue` | likely 0.67 | notify/notify | 4 (4) | pass |
| node-disk-full-by-deleted-open-file | `node//n1` | `node//n1` | high 1.00 | notify/notify | 1 (2) | pass |
| crash-looping-pods-exhaust-node-inodes | `deployment/batch/flapper` | `deployment/batch/flapper (no cause)` | none | notify/notify | 2 (3) | pass |
| claim-terminating-held-by-completed-job-pod | `persistentvolumeclaim/batch/results` | `persistentvolumeclaim/batch/results (no cause)` | none | notify/notify | 1 (2) | pass |
| pvc-selected-node-removed-before-provisioning | `persistentvolumeclaim/shop/data` | `persistentvolumeclaim/shop/data` | high 1.00 | notify/notify | 1 (2) | pass |
| cronjob-pull-failures-fill-pod-capacity | `cronjob/ops/sync-feed` | `cronjob/ops/sync-feed (no cause)` | none | notify/notify | 3 (3) | pass |
| job-on-failure-restarts-in-place-forever | `job/batch/import` | `job/batch/import` | high 0.85 | notify/notify | 1 (2) | pass |
| crash-looping-statefulset-blocks-drain | `statefulset/streaming/queue` | `statefulset/streaming/queue (no cause)` | none | notify/notify | 2 (2) | pass |
| release-volume-hides-image-files | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-writes-into-configmap-volume | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| service-external-name-is-an-ip-address | `service/shop/db` | `service/shop/db` | high 0.95 | notify/notify | 1 (2) | pass |
| registry-removes-tags-shared-by-workloads | `registry//registry.example.com` | `registry//registry.example.com` | high 1.00 | notify/notify | 1 (3) | pass |
| daemonset-host-root-mount-not-shared | `daemonset/monitoring/host-agent` | `daemonset/monitoring/host-agent (no cause)` | none | notify/notify | 1 (2) | pass |
| release-post-start-hook-hangs | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| statefulset-ordinal-refuses-restart-after-drain | `statefulset/streaming/queue` | `statefulset/streaming/queue (no cause)` | none | notify/notify | 1 (2) | pass |
| coredns-pod-removed-lookups-stall-briefly | `deployment/kube-system/coredns` | `deployment/kube-system/coredns` | high 0.80 | notify/notify | 2 (2) | pass |
| sporadic-connection-reset-no-cause | `unknown` | `deployment/shop/api (no cause)` | none | digest/digest | 0 (2) | pass |
| node-memory-starved-by-low-requests | `node//n1` | `node//n1` | high 1.00 | page/page | 3 (3) | pass |
| probe-failure-on-terminating-pod-quiet | quiet | - | - | -/- | 0 (0) | pass |
| pods-bound-to-node-that-no-longer-exists | `node//n9` | `node//n9` | high 0.95 | notify/notify | 1 (2) | pass |
| spread-skew-after-rollout-quiet | quiet | - | - | -/- | 0 (0) | pass |
| control-plane-failover-nodes-unknown-then-recover | `unknown` | `node//n1 (no cause)` | none | page/page | 3 (3) | pass |
| etcd-wal-corrupt-after-reboot | `etcd//etcd` | `etcd//etcd` | high 1.00 | page/page | 2 (3) | pass |
| in-cluster-nfs-server-restarts-consumers-hang | `unknown` | `deployment/billing/invoices (no cause)` | none | notify/notify | 1 (3) | pass |
| upgrade-removes-api-version-controller-crashes | `deployment/ingress-system/legacy-controller` | `deployment/ingress-system/legacy-controller (no cause)` | none | notify/notify | 1 (2) | pass |
| webhook-v1-defaults-fail-closed-after-upgrade | `validatingwebhookconfiguration//legacy-labels` | `validatingwebhookconfiguration//legacy-labels` | high 1.00 | page/page | 1 (3) | pass |
| webhook-certificate-common-name-only-after-upgrade | `validatingwebhookconfiguration//cert-policy` | `validatingwebhookconfiguration//cert-policy` | high 1.00 | page/page | 1 (3) | pass |
| upgrade-enforces-exec-probe-timeout | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| upgrade-changes-topology-spread-counting | `deployment/shop/api` | `deployment/shop/api` | high 0.95 | notify/notify | 1 (2) | pass |
| controller-manager-static-pod-probe-port-removed | `deployment/kube-system/kube-controller-manager` | `deployment/kube-system/kube-controller-manager (no cause)` | none | notify/notify | 1 (3) | pass |
| scheduler-config-names-removed-plugin | `scheduler//kube-scheduler` | `scheduler//kube-scheduler` | high 1.00 | page/page | 3 (3) | pass |
| upgrade-without-cloud-controller-nodes-uninitialized | `scheduling//had untolerated taint` | `scheduling//had untolerated taint` | high 1.00 | notify/notify | 2 (3) | pass |
| upgrade-garbage-collects-cross-namespace-owner-ref | quiet | - | - | -/- | 0 (0) | pass |
| upgrade-addon-image-repository-not-mirrored | `deployment/kube-system/coredns` | `deployment/kube-system/coredns` | high 0.77 | notify/notify | 1 (2) | pass |
| storageclass-provisioner-secret-missing | `storageclass//secure-fast` | `storageclass//secure-fast` | high 1.00 | notify/notify | 1 (2) | pass |
| storageclass-parameter-rejected-claims-pending | `storageclass//fast-bad` | `storageclass//fast-bad` | high 1.00 | notify/notify | 1 (3) | pass |
| pvc-request-below-driver-minimum-size | `persistentvolumeclaim/shop/cache-data` | `persistentvolumeclaim/shop/cache-data` | high 0.90 | notify/notify | 1 (2) | pass |
| pvc-restore-from-snapshot-not-ready | `persistentvolumeclaim/shop/restore-data` | `persistentvolumeclaim/shop/restore-data` | high 0.90 | notify/notify | 1 (2) | pass |
| volume-snapshot-two-default-snapshot-classes | `volumesnapshot.snapshot.storage.k8s.io/shop/nightly` | `volumesnapshot.snapshot.storage.k8s.io/shop/nightly (no cause)` | none | digest/digest | 0 (2) | pass |
| volume-snapshot-content-creation-failed | `volumesnapshotcontent.snapshot.storage.k8s.io//snapcontent-3f2a` | `volumesnapshotcontent.snapshot.storage.k8s.io//snapcontent-3f2a (no cause)` | none | notify/notify | 1 (2) | pass |
| snapshot-controller-scaled-to-zero | quiet | - | - | -/- | 0 (0) | pass |
| volume-snapshot-delete-blocked-by-restoring-claim | `persistentvolumeclaim/shop/restore-data` | `persistentvolumeclaim/shop/restore-data` | high 0.80 | digest/digest | 1 (2) | pass |
| volume-snapshot-content-missing | `volumesnapshotcontent.snapshot.storage.k8s.io//imported-content` | `volumesnapshotcontent.snapshot.storage.k8s.io//imported-content` | high 0.95 | notify/notify | 1 (2) | pass |
| csi-attacher-down-pods-wait-for-attach | `deployment/kube-system/ebs-csi-controller` | `deployment/kube-system/ebs-csi-controller` | high 1.00 | notify/notify | 1 (3) | pass |
| csi-driver-object-missing-pod-waits | `csidriver//hostpath.csi.example.com` | `csidriver//hostpath.csi.example.com` | high 0.85 | notify/notify | 1 (2) | pass |
| pv-filesystem-mismatch-pod-waits | `persistentvolume//pv-restored` | `persistentvolume//pv-restored (no cause)` | none | notify/notify | 2 (2) | pass |
| pvc-volume-health-inaccessible | `persistentvolumeclaim/inventory/pgdata` | `persistentvolumeclaim/inventory/pgdata (no cause)` | none | notify/notify | 1 (2) | pass |
| selectorless-service-without-endpoints | `service/shop/external-db` | `service/shop/external-db` | high 0.75 | notify/notify | 1 (2) | pass |
| endpointslice-writes-refused-by-quota | `resourcequota/shop/object-counts` | `resourcequota/shop/object-counts (no cause)` | none | notify/notify | 1 (3) | pass |
| service-ipv6-family-pods-ipv4-only | `service/shop/api` | `service/shop/api` | high 0.95 | notify/notify | 1 (2) | pass |
| node-cni-teardown-fails-pods-terminating | `node//n2` | `node//n2` | possible 0.49 | notify/notify | 1 (3) | pass |
| node-cni-config-invalid-new-pods-fail | `node//n3` | `node//n3` | high 0.79 | notify/notify | 1 (2) | pass |
| node-image-lacks-cni-plugin-binary | `node//n4` | `node//n4` | possible 0.49 | notify/notify | 1 (3) | pass |
| rollout-surge-pod-no-node-free-of-replica | `deployment/shop/search` | `deployment/shop/search` | high 0.87 | notify/notify | 1 (2) | pass |
| release-init-container-cpu-request-too-large | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| job-fails-backoff-limit-from-node-drains | `job/batch/nightly` | `job/batch/nightly (no cause)` | none | notify/notify | 1 (3) | pass |
| cronjob-replace-policy-kills-every-run | `cronjob/reports/slow-export` | `cronjob/reports/slow-export (no cause)` | none | digest/digest | 1 (2) | pass |
| job-never-completes-sidecar-keeps-running | `job/batch/report-gen` | `job/batch/report-gen` | high 0.85 | notify/notify | 1 (2) | pass |
| rollout-readiness-needs-leadership-deadlock | `deployment/shop/api` | `deployment/shop/api` | high 0.90 | notify/notify | 2 (2) | pass |
| rollout-callers-see-refused-connections | `unknown` | `deployment/shop/web (no cause)` | none | notify/notify | 1 (2) | pass |
| encryption-provider-down-secret-reads-fail | `unknown` | `deployment/billing/invoices (no cause)` | none | notify/notify | 1 (3) | pass |
| encryption-key-dropped-old-secrets-unreadable | `unknown` | `deployment/shop/orders (no cause)` | none | notify/notify | 1 (2) | pass |
| authorization-webhook-down-forbidden-everywhere | `unknown` | `deployment/billing/operator (no cause)` | none | notify/notify | 1 (3) | pass |
| service-account-recreated-running-pods-unauthorized | `serviceaccount/shop/api` | `serviceaccount/shop/api` | high 1.00 | notify/notify | 1 (2) | pass |
| operator-role-lacks-lease-permission | `role/shop/operator-role` | `role/shop/operator-role` | high 0.70 | notify/notify | 1 (2) | pass |
| aggregated-cluster-role-edit-reverted | `clusterrole//view-extended` | `clusterrole//view-extended` | high 0.85 | notify/notify | 1 (2) | pass |
| networkpolicy-blocks-api-server-to-webhook-backend | `networkpolicy/policy-system/default-deny` | `networkpolicy/policy-system/default-deny` | high 1.00 | page/page | 1 (3) | pass |
| networkpolicy-isolates-metrics-server | `networkpolicy/kube-system/isolate-metrics` | `networkpolicy/kube-system/isolate-metrics` | high 1.00 | notify/notify | 2 (2) | pass |
| csr-approved-but-never-issued | `certificatesigningrequest//csr-9d4k1` | `certificatesigningrequest//csr-9d4k1 (no cause)` | none | notify/notify | 1 (2) | pass |
| admission-policy-expression-error-denies-all | `validatingadmissionpolicy//require-team-label` | `validatingadmissionpolicy//require-team-label` | high 1.00 | notify/notify | 1 (3) | pass |
| release-drops-all-capabilities-chown-fails | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| release-run-as-user-without-passwd-entry | `deployment/shop/api` | `deployment/shop/api` | high 0.91 | notify/notify | 1 (2) | pass |
| dropped-oom-event-crash-loop | `deployment/shop/api` | `deployment/shop/api` | high 0.73 | notify/notify | 1 (2) | pass |
| event-ttl-expired-before-kwatch-start | `deployment/billing/worker` | `deployment/billing/worker (no cause)` | none | notify/notify | 1 (1) | pass |
| resync-replays-resolved-warnings | quiet | - | - | -/- | 0 (0) | pass |
| kwatch-restart-mid-incident-no-second-page | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (1) | pass |
| kwatch-restart-during-node-loss | `node//worker-3` | `node//worker-3` | high 1.00 | page/page | 2 (2) | pass |
| clock-skew-node-events-predate-the-edit | `configmap/shop/api-config` | `configmap/shop/api-config` | high 1.00 | notify/notify | 1 (1) | pass |
| clock-skew-future-events-secret-missing | `secret/media/encoder-creds` | `secret/media/encoder-creds` | high 0.85 | notify/notify | 1 (1) | pass |
| event-before-pod-object-missing-secret | `secret/shop/checkout-creds` | `secret/shop/checkout-creds` | high 0.85 | notify/notify | 1 (1) | pass |
| event-before-owner-replicaset-cached | `deployment/reports/gen` | `deployment/reports/gen` | high 0.83 | notify/notify | 1 (1) | pass |
| rbac-hides-events-status-suffices | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 1 (1) | pass |
| rbac-hides-nodes-pods-unknown-phase | `unknown` | `statefulset/data/pg (no cause)` | none | notify/notify | 1 (1) | pass |
| rbac-hides-configmaps-event-names-it | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (1) | pass |
| rbac-hides-pod-logs-dependency-refused | `unknown`, `deployment/shop/redis` | `deployment/shop/api (no cause)`, `deployment/shop/redis (no cause)` | none | notify/notify | 1 (2) | pass |
| watch-gap-crash-and-recover-quiet | quiet | - | - | -/- | 0 (0) | pass |
| watch-gap-hides-rollout-start | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| watch-gap-node-flaps-quiet | quiet | - | - | -/- | 0 (0) | pass |
| late-unschedulable-event-after-pending-seen | `scheduling//had untolerated taint` | `scheduling//had untolerated taint` | high 0.85 | notify/notify | 1 (1) | pass |
| dropped-kill-events-liveness-kills | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (1) | pass |
| aggregated-event-count-jump | `deployment/shop/web` | `deployment/shop/web (no cause)` | none | notify/notify | 1 (1) | pass |
| stale-cache-pod-already-deleted | `unknown` | `replicaset/shop/api-4dffe2cd (no cause)` | none | notify/notify | 2 (2) | pass |
| one-zone-pods-cannot-reach-database | `zone//zone-b` | `zone//zone-b` | likely 0.58 | notify/notify | 1 (2) | pass |
| one-zone-image-pull-fails | `zone//zone-c` | `zone//zone-c` | high 0.73 | notify/notify | 1 (2) | pass |
| one-pool-new-taint-pods-pending | `scheduling//had untolerated taint` | `scheduling//had untolerated taint` | high 0.85 | notify/notify | 2 (2) | pass |
| one-pool-disk-pressure-evictions | `nodepool//pool-logs` | `nodepool//pool-logs` | high 1.00 | notify/notify | 1 (2) | pass |
| one-replica-corrupt-image-cache-node | `node//node-c` | `node//node-c` | high 0.79 | notify/notify | 1 (1) | pass |
| one-replica-oom-hot-shard | `deployment/shop/cache` | `deployment/shop/cache` | high 0.73 | notify/notify | 1 (1) | pass |
| some-replicas-old-config-subpath | `configmap/shop/api-config` | `configmap/shop/api-config` | high 0.80 | notify/notify | 1 (1) | pass |
| one-path-large-responses-time-out | quiet | - | - | -/- | 0 (0) | pass |
| one-path-slow-dependency-health-ok | quiet | - | - | -/- | 0 (0) | pass |
| intermittent-resets-from-terminating-endpoint | quiet | - | - | -/- | 0 (0) | pass |
| intermittent-one-coredns-replica-unsynced | `deployment/kube-system/coredns` | `deployment/kube-system/coredns (no cause)` | none | notify/notify | 1 (2) | pass |
| canary-five-percent-bad-image | `deployment/shop/api-canary` | `deployment/shop/api-canary (no cause)` | none | notify/notify | 1 (1) | pass |
| tenant-default-deny-breaks-dns | `networkpolicy/tenant-b/default-deny-all` | `networkpolicy/tenant-b/default-deny-all` | high 1.00 | notify/notify | 1 (2) | pass |
| tenant-quota-forbids-storageclass | `resourcequota/tenant-c/storage-caps` | `resourcequota/tenant-c/storage-caps` | high 1.00 | notify/notify | 1 (2) | pass |
| zone-spread-leaves-replicas-pending | `scheduling//didn't match pod topology spread constraints` | `scheduling//didn't match pod topology spread constraints` | high 1.00 | notify/notify | 1 (2) | pass |
| one-pool-runtimeclass-handler-missing | `nodepool//pool-b` | `nodepool//pool-b` | high 0.82 | notify/notify | 1 (2) | pass |
| statefulset-one-ordinal-data-corrupt | `statefulset/data/db` | `statefulset/data/db (no cause)` | none | notify/notify | 1 (1) | pass |
| udp-only-blocked-by-policy | quiet | - | - | -/- | 0 (0) | pass |
| readiness-gate-unmet-on-some-pods | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (1) | pass |
| local-traffic-policy-unhealthy-nodes-quiet | quiet | - | - | -/digest | 0 (0) | pass |
| shared-postgres-down-three-namespaces | `statefulset/data/pg` | `statefulset/data/pg` | high 0.80 | notify/notify | 1 (2) | pass |
| shared-cache-no-endpoints-cross-namespace | `unknown` | `deployment/jobs/render (no cause)` | none | notify/notify | 1 (1) | pass |
| ingress-controller-down-four-namespaces | `deployment/ingress-nginx/controller` | `deployment/ingress-nginx/controller (no cause)` | none | page/page | 1 (2) | pass |
| shared-nfs-server-down-mount-timeouts | `deployment/storage/nfs-server` | `deployment/storage/nfs-server` | likely 0.57 | notify/notify | 3 (3) | pass |
| shared-egress-proxy-down | `deployment/egress/proxy` | `deployment/egress/proxy` | high 1.00 | notify/notify | 1 (2) | pass |
| shared-auth-service-jwks-unreachable | `deployment/identity/auth` | `deployment/identity/auth (no cause)` | none | notify/notify | 2 (2) | pass |
| shared-cache-rolling-restart-brief-errors-quiet | quiet | - | - | -/- | 0 (0) | pass |
| shared-database-failover-digest | quiet | - | - | -/- | 0 (0) | pass |
| clusterissuer-secret-deleted-certificates-stalled | `clusterissuer.certs.example.com//internal-ca` | `clusterissuer.certs.example.com//internal-ca (no cause)` | none | notify/notify | 3 (3) | pass |
| clusterissuer-ca-secret-annotation-expired | `clusterissuer.certs.example.com//internal-ca` | `clusterissuer.certs.example.com//internal-ca (no cause)` | none | notify/notify | 3 (3) | pass |
| shared-configmap-bad-json-three-namespaces | `failure-signature//unexpected end of json input reading platform/shared-flags key flags.json` | `failure-signature//unexpected end of json input reading platform/shared-flags key flags.json` | high 0.85 | notify/notify | 1 (1) | pass |
| corefile-forwarder-edit-external-lookups-fail | `configmap/kube-system/coredns` | `configmap/kube-system/coredns` | high 1.00 | notify/notify | 1 (1) | pass |
| crd-conversion-webhook-no-endpoints | `deployment/crd-system/gadget-conv` | `deployment/crd-system/gadget-conv (no cause)` | none | notify/notify | 2 (2) | pass |
| secretstore-credential-secret-missing-externalsecrets-fail | `unknown` | `deployment/billing/api (no cause)` | none | notify/notify | 2 (2) | pass |
| mutating-webhook-injects-missing-image | `mutatingwebhookconfiguration//sidecar-injector` | `mutatingwebhookconfiguration//sidecar-injector` | high 1.00 | notify/notify | 1 (2) | pass |
| storageclass-recreated-wrong-parameters | `storageclass//fast-ssd` | `storageclass//fast-ssd` | high 1.00 | notify/notify | 2 (3) | pass |
| priorityclass-deleted-replicasets-refused | `priorityclass//team-high` | `priorityclass//team-high` | high 0.90 | notify/notify | 1 (1) | pass |
| clusterrole-edit-agents-forbidden-three-namespaces | `clusterrole//app-discovery` | `clusterrole//app-discovery` | high 1.00 | notify/notify | 1 (1) | pass |
| apiservice-cabundle-mismatch-discovery-fails | `apiservice//v1alpha1.custom.example.com` | `apiservice//v1alpha1.custom.example.com` | high 0.85 | notify/notify | 1 (1) | pass |
| crd-version-removed-operators-log-no-match | `failure-signature//no matches for kind "widget" in version "example.com/v#beta#"` | `failure-signature//no matches for kind "widget" in version "example.com/v#beta#"` | high 0.70 | notify/notify | 1 (1) | pass |
| wording-oom-limit-too-low | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 1 (1) | pass |
| wording-image-typo | `deployment/billing/worker` | `deployment/billing/worker` | high 0.91 | notify/notify | 1 (1) | pass |
| wording-bad-rollout-config-key | `deployment/shop/api` | `deployment/shop/api` | high 0.88 | notify/notify | 1 (1) | pass |
| wording-probe-port-mismatch | `deployment/shop/web` | `deployment/shop/web` | high 0.83 | notify/notify | 1 (1) | pass |
| wording-certificate-expired-handshake | `secret/shop/bank-client-tls` | `secret/shop/bank-client-tls` | high 1.00 | notify/notify | 1 (1) | pass |
| wording-first-message-ongoing-chronic | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| wording-quota-forbids-pods | `resourcequota/shop/compute` | `resourcequota/shop/compute` | high 1.00 | notify/notify | 1 (1) | pass |
| wording-registry-rate-limited | `registry//docker.io` | `registry//docker.io` | high 1.00 | notify/notify | 1 (1) | pass |

## Held-out scenarios

16 held-out root cases in 14 scenarios, 16 right. Never used to tune the engine.

| Scenario | Expected root | Actual root | Confidence | Tier (expected/actual) | Messages (max) | Result |
| --- | --- | --- | --- | --- | --- | --- |
| heldout-configmap-key-missing | `configmap/catalog/api-settings` | `configmap/catalog/api-settings` | high 1.00 | notify/notify | 1 (2) | pass |
| heldout-image-tag-missing | `deployment/storefront/backend` | `deployment/storefront/backend` | high 0.91 | notify/notify | 1 (2) | pass |
| heldout-sidecar-crash | `deployment/orders/api` | `deployment/orders/api` | high 0.91 | notify/notify | 1 (2) | pass |
| heldout-node-disk-pressure | `node//n2` | `node//n2` | high 1.00 | notify/notify | 1 (2) | pass |
| heldout-namespace-dns-blocked | `networkpolicy/orders/default-deny-egress` | `networkpolicy/orders/default-deny-egress` | high 1.00 | notify/notify | 1 (2) | pass |
| heldout-quota-one-namespace | `resourcequota/ml/team-quota` | `resourcequota/ml/team-quota` | high 1.00 | notify/notify | 1 (2) | pass |
| heldout-cronjob-failing | `cronjob/billing/invoice-export` | `cronjob/billing/invoice-export` | likely 0.69 | notify/notify | 2 (2) | pass |
| heldout-mixed-storm | `node//n4`, `secret/finance/payout-keys`, `deployment/search/indexer` | `node//n4`, `secret/finance/payout-keys`, `deployment/search/indexer` | high 1.00 | page/page | 4 (8) | pass |
| heldout-statefulset-zone-conflict | `persistentvolumeclaim/streaming/data-kafka-2` | `persistentvolumeclaim/streaming/data-kafka-2` | high 0.95 | notify/notify | 1 (2) | pass |
| heldout-quota-pod-count | `resourcequota/ci/object-counts` | `resourcequota/ci/object-counts` | high 1.00 | notify/notify | 1 (2) | pass |
| heldout-pull-secret-rotated | `secret/payments/pull-creds` | `secret/payments/pull-creds` | high 0.80 | notify/notify | 1 (2) | pass |
| heldout-liveness-too-aggressive | `deployment/reports/renderer` | `deployment/reports/renderer` | high 1.00 | notify/notify | 2 (2) | pass |
| heldout-jdbc-refused-healthy-db | `deployment/sales/orders` | `deployment/sales/orders` | likely 0.69 | notify/notify | 1 (2) | pass |
| heldout-queue-timeout-backend-down | `deployment/pipeline/queue` | `deployment/pipeline/queue` | high 0.95 | notify/notify | 1 (2) | pass |

## Recorded scenarios

1 recorded root cases in 1 scenarios, 1 right. Captured from real clusters; never used to tune the engine.
Recorded accuracy gate: not gated, 1 of 10 recorded cases needed (1 of 1 root cases right so far).

| Scenario | Expected root | Actual root | Confidence | Tier (expected/actual) | Messages (max) | Result |
| --- | --- | --- | --- | --- | --- | --- |
| recorded-bad-rollout | `deployment/ns-dacf0cd1f0/name-468f1de69a` | `deployment/ns-dacf0cd1f0/name-468f1de69a` | high 0.86 | notify/notify | 1 (2) | pass |
<!-- END GENERATED REPORT (make alert-quality) -->
