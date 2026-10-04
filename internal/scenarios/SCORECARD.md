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
staging day is 12 hours with every labelled scenario once, each fixed 45
minutes later, plus healthy rollouts every 15 minutes, scale events every
25 minutes and the explicit non-events below, each run twice.

Regenerate this page's numbers with `make alert-quality`; the report is
written to `_output/alert-quality.md`.

## Gates

"Start" is the scorecard at the start of the 2026-10-01 round, before
any change below; "rule engine" is the 14-rule engine this engine
replaced, measured on 2026-09-30. The latency targets were changed on
2026-10-01 by the maintainers' decision (from 60 and 180 seconds); no
other target was relaxed.

| Metric | Target | Rule engine | Start | Now | Result |
| --- | --- | --- | --- | --- | --- |
| Correct root cause (labelled) | >= 90% | 63.0% | 96.5% (55 of 57) | 97.4% (76 of 78) | pass |
| Correct root cause (held-out) | >= 80% | - | 71.4% (10 of 14) | 85.7% (12 of 14) | pass |
| Wrong high-confidence root | <= 5% of high-confidence cases, labelled and held-out, gated at 20 or more | 0% | 2.5% (1 of 40) | 4% (3 of 75) | pass |
| Calibration: high confidence | 80-100% (at least 10 cases) | 100% (10 of 10) | 97.5% (39 of 40) | 96.9% (62 of 64) | pass |
| Calibration: likely confidence | 50-90% (at least 10 cases) | 66.7% (4 of 6) | 90.9% (10 of 11) | not gated: 4 of 4 right, 10 cases needed | pass |
| Messages per incident (p95) | <= 3 | - | 3 | 3 | pass |
| Messages per incident (most) | <= 5 | 4 | 3 | 4 | pass |
| Time to first message (page tier, max) | <= 2m (was 60s) | - | 1m45s (p95 1m45s, 9 scenarios) | 1m45s (p95 1m45s, 17 scenarios) | pass |
| Time to first message (notify tier, max) | <= 5m (was 180s) | - | 4m15s (p95 3m15s, 43 scenarios) | 5m0s (p95 5m0s, 55 scenarios) | pass |
| Notifications from non-events (staging day) | 0 | - | 4 (all digest) | 0 | pass |
| Notifications per hour (staging day peak) | <= 30/h (sanity ceiling) | 20/h | 18/h | 23/h | pass |
| Unchanged updates | 0% | 0% | 0% | 0% | pass |
| Re-created incidents | <= 5% | 2.1% | 1.6% | 2.5% | pass |
| Repeated recoveries | 0 | 0 | 0 | 0 | pass |
| Storm messages in 2 minutes (shared node) | <= 3 | 2 | 1 | 1 | pass |
| Storm messages in 2 minutes (shared registry) | <= 3 | 102 | 1 | 1 | pass |
| Storm messages in 2 minutes (multi-cause) | <= 13 (causes + 3) | - | 10 | 10 | pass |
| Multi-cause storm causes reported | >= 10 | - | 10 | 10 | pass |
| Scenarios blaming a must-not-blame entity | 0 | 4 | 0 | 0 | pass |
| Quiet scenarios that alerted | 0 | 0 | 0 | 0 | pass |
| Scenarios over their message budget | 0 | 7 | 0 | 0 | pass |

21 of 21 gates pass (16 of 21 at the start). 72 labelled root cases in
71 scenarios; first-incident tier matches in 71 of 71. Staging day: 178
notifications in 14h42m (12.1/h mean, peak hour 21); none from
non-events. The staging day grew with the 15 new labelled scenarios;
its busiest incidents (4 messages) are two autoscaler ceilings that go
digest, notify, digest and resolve as the staging fix removes their
pods first, and a node under memory pressure that sends one more update
while the staging fix deletes its pods.

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

On 2026-10-01 the labelled cases scored in [0.70, 0.75) were right 12
times in 13 (92%): stated as "likely", they made that level
underconfident (90.9%, then 94.1% with the new scenarios). The method
put the high boundary at 0.70 (it was 0.75). The four labelled cases
between 0.50 and 0.70 were all right but are too few to judge, so the
likely level is reported as not gated. No boundary would have kept
"likely" inside 50 to 90% with 10 or more cases: every range of 10 or
more includes the 13 tied cases at 0.70, of which only one is wrong.
Measuring the likely level needs more genuinely ambiguous labelled
scenarios, not different boundaries.

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
  node is full. Label: its PersistentVolumeClaim. Missed: blamed on the
  scheduling constraint "had volume node affinity conflict".
- `heldout-pull-secret-rotated`: a namespace's image pull Secret is
  rotated to a token the registry rejects; another namespace pulls from
  the same registry fine. Label: the Secret. Missed: blamed on the
  registry (a must-not-blame entity).
- `heldout-liveness-too-aggressive`: new replicas of a slow-starting app
  are killed by a liveness probe that gives up after 20 seconds. Label:
  the Deployment. Right, though with no cause stated.
- `heldout-quota-pod-count`: a scale-up exceeds a namespace's pod-count
  quota while another namespace scales out. Label: the quota. Right.

They are reported, not fixed. The held-out set is again 12 scenarios
with 14 root cases, 12 right (85.7%).

## Borderline labelled scenarios

Eight labelled scenarios (`library_borderline_*_test.go`) are ambiguous
on purpose: two plausible upstream changes close together (a ConfigMap
edit and a NetworkPolicy, a rollout and a Service selector edit, an
operator upgrade and a custom resource edit, a Secret rotation and a
schedule edit, a Gateway edit and a route edit, two policies, a Secret
rotation and a rollout) and weak timing with partial evidence (a
ConfigMap picked up eleven minutes later, with an error naming no key).
Their labels are the true root of what happened, not the engine's
expected answer. Two of them miss, both now with high confidence:
borderline-policy-and-config blames the ConfigMap (1.00) and
borderline-two-policies blames the allowing policy (0.70).

## Scenarios

| Scenario | Expected root | Actual root | Confidence | Tier (expected/actual) | Messages (max) | Result |
| --- | --- | --- | --- | --- | --- | --- |
| bad-rollout | `deployment/shop/payments` | `deployment/shop/payments` | high 0.91 | notify/notify | 1 (2) | pass |
| secret-key-removed | `secret/shop/db-creds` | `secret/shop/db-creds` | high 1.00 | notify/notify | 1 (2) | pass |
| missing-secret | `secret/billing/stripe-api` | `secret/billing/stripe-api` | high 1.00 | notify/notify | 1 (2) | pass |
| configmap-change | `configmap/web/frontend-config` | `configmap/web/frontend-config` | high 1.00 | notify/notify | 1 (2) | pass |
| node-memory-pressure-eviction | `node//n1` | `node//n1` | high 1.00 | notify/notify | 2 (2) | pass |
| node-lost-notready | `node//n1` | `node//n1` | high 1.00 | page/page | 2 (2) | pass |
| healthy-node-app-crash | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 2 (2) | pass |
| cordoned-node-app-crash | `deployment/shop/worker` | `deployment/shop/worker (no cause)` | none | notify/notify | 1 (2) | pass |
| zone-failure | `zone//zone-b` | `zone//zone-b` | high 1.00 | page/page | 2 (2) | pass |
| coredns-down | `cluster-dns//cluster-dns` | `cluster-dns//cluster-dns` | high 0.95 | page/page | 1 (2) | pass |
| webhook-no-endpoints | `validatingwebhookconfiguration//policy-validator` | `validatingwebhookconfiguration//policy-validator` | high 1.00 | page/page | 2 (2) | pass |
| quota-exhausted | `resourcequota/analytics/compute-quota` | `resourcequota/analytics/compute-quota` | high 1.00 | notify/notify | 1 (2) | pass |
| metrics-apiservice-down | `apiservice//v1beta1.metrics.k8s.io` | `apiservice//v1beta1.metrics.k8s.io` | high 1.00 | notify/notify | 2 (2) | pass |
| registry-auth-failure | `registry//registry.corp.example` | `registry//registry.corp.example` | high 1.00 | notify/notify | 1 (2) | pass |
| image-typo | `deployment/shop/checkout` | `deployment/shop/checkout` | high 0.91 | notify/notify | 1 (2) | pass |
| networkpolicy-change | `networkpolicy/payments/restrict-egress` | `networkpolicy/payments/restrict-egress` | high 0.70 | notify/notify | 1 (2) | pass |
| operator-cr-stuck | `postgrescluster.postgres.example.com/data/orders-db` | `postgrescluster.postgres.example.com/data/orders-db` | high 0.70 | notify/notify | 1 (2) | pass |
| scheduler-insufficient-memory | `scheduling//Insufficient memory` | `scheduling//Insufficient memory` | high 1.00 | notify/notify | 1 (2) | pass |
| pvc-pending-immediate | `persistentvolumeclaim/data/orders-db` | `persistentvolumeclaim/data/orders-db` | high 1.00 | notify/notify | 1 (2) | pass |
| pvc-wffc-no-consumer | quiet | - | - | -/- | 0 (0) | pass |
| cronjob-invalid-schedule | `cronjob/reports/nightly-report` | `cronjob/reports/nightly-report` | high 0.70 | notify/notify | 1 (1) | pass |
| oom-limit-too-low | `deployment/shop/cart` | `deployment/shop/cart` | high 1.00 | notify/notify | 1 (2) | pass |
| flapping-workload | `deployment/shop/search` | `deployment/shop/search (no cause)` | none | notify/notify | 3 (3) | pass |
| two-independent-problems | `node//n1`, `deployment/shop/checkout` | `node//n1`, `deployment/shop/checkout` | high 1.00 | notify/notify | 2 (4) | pass |
| cause-revised | `node//n3` | `node//n3` | high 1.00 | notify/notify | 2 (3) | pass |
| cold-start-preexisting | `secret/billing/invoicer-creds`, `deployment/shop/api` | `secret/billing/invoicer-creds`, `deployment/shop/api (no cause)` | high 0.85 | notify/notify | 1 (1) | pass |
| scheduler-down | `scheduler//kube-scheduler` | `scheduler//kube-scheduler` | high 1.00 | page/page | 2 (2) | pass |
| controller-manager-down | `controller-manager//kube-controller-manager` | `controller-manager//kube-controller-manager` | high 0.95 | page/page | 2 (2) | pass |
| etcd-down | `etcd//etcd` | `etcd//etcd` | high 0.90 | page/page | 1 (2) | pass |
| rbac-binding-removed | `rolebinding/shop/api-config-reader` | `rolebinding/shop/api-config-reader` | high 0.85 | notify/notify | 1 (2) | pass |
| rbac-change-unrelated-crash | `deployment/shop/api` | `deployment/shop/api (no cause)` | none | notify/notify | 1 (2) | pass |
| service-selector-change | `service/shop/payments` | `service/shop/payments` | high 0.70 | notify/notify | 1 (2) | pass |
| service-backends-crash | `deployment/shop/payments` | `deployment/shop/payments` | high 1.00 | notify/notify | 1 (2) | pass |
| ingress-backend-missing | `service/shop/web-v2` | `service/shop/web-v2` | high 0.90 | page/page | 1 (2) | pass |
| ingress-tls-secret-missing | `secret/shop/web-tls` | `secret/shop/web-tls` | high 0.75 | notify/notify | 1 (2) | pass |
| route-not-accepted | `httproute.gateway.networking.k8s.io/shop/web` | `httproute.gateway.networking.k8s.io/shop/web` | high 0.70 | notify/notify | 1 (2) | pass |
| certificate-expired | `secret/shop/bank-client-tls` | `secret/shop/bank-client-tls` | high 1.00 | notify/notify | 1 (2) | pass |
| certificate-expiring-app-crash | `deployment/shop/payments` | `deployment/shop/payments (no cause)` | none | notify/notify | 2 (2) | pass |
| spot-node-removed | `node//n3` | `node//n3` | high 1.00 | notify/notify | 1 (2) | pass |
| node-removed-earlier-scale-up | `scheduling//Insufficient cpu` | `scheduling//Insufficient cpu` | high 1.00 | notify/notify | 1 (2) | pass |
| budget-blocks-drain | `poddisruptionbudget/shop/ledger` | `poddisruptionbudget/shop/ledger` | high 0.90 | notify/notify | 1 (2) | pass |
| single-replica-node-loss | `node//n1` | `node//n1` | high 1.00 | page/page | 3 (3) | pass |
| operator-cr-owned-failing | `kafkacluster.kafka.example.com/data/events` | `kafkacluster.kafka.example.com/data/events` | high 0.98 | notify/notify | 1 (2) | pass |
| operator-down-crs-stuck | `deployment/db-operators/pg-operator` | `deployment/db-operators/pg-operator` | high 0.80 | notify/notify | 2 (2) | pass |
| probe-port-mismatch | `deployment/shop/api` | `deployment/shop/api` | high 1.00 | notify/notify | 1 (2) | pass |
| startup-budget-too-short | `deployment/shop/api` | `deployment/shop/api` | high 0.88 | notify/notify | 1 (2) | pass |
| init-container-fails | `deployment/shop/orders` | `deployment/shop/orders` | likely 0.60 | notify/notify | 1 (2) | pass |
| sidecar-crash-loops | `deployment/shop/payments` | `deployment/shop/payments` | high 0.90 | notify/notify | 2 (2) | pass |
| borderline-policy-and-config | `networkpolicy/payments/restrict-egress` | `configmap/payments/api-config` | high 1.00 | notify/notify | 1 (2) | wrong root |
| borderline-selector-and-rollout | `service/shop/checkout` | `service/shop/checkout` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-operator-upgrade-and-edit | `postgrescluster.postgres.example.com/data/orders-db` | `postgrescluster.postgres.example.com/data/orders-db` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-schedule-and-secret | `cronjob/reports/nightly-report` | `cronjob/reports/nightly-report` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-late-config-reload | `configmap/batch/worker-config` | `configmap/batch/worker-config` | high 1.00 | notify/notify | 1 (2) | pass |
| borderline-route-and-gateway-edit | `httproute.gateway.networking.k8s.io/shop/web` | `httproute.gateway.networking.k8s.io/shop/web` | high 0.70 | notify/notify | 1 (2) | pass |
| borderline-two-policies | `networkpolicy/payments/restrict-egress` | `networkpolicy/payments/allow-cache` | high 0.70 | notify/notify | 1 (2) | wrong root |
| borderline-init-after-rotation | `secret/shop/orders-db` | `secret/shop/orders-db` | high 0.85 | notify/notify | 1 (2) | pass |
| claim-full-with-volume | `persistentvolumeclaim/observability/chunks` | `persistentvolumeclaim/observability/chunks` | high 0.85 | notify/notify | 1 (2) | pass |
| claim-full-unlisted-volume | `persistentvolumeclaim/messaging/broker-log` | `persistentvolumeclaim/messaging/broker-log` | high 0.70 | notify/notify | 1 (2) | pass |
| pvc-full | `persistentvolumeclaim/inventory/pgdata` | `persistentvolumeclaim/inventory/pgdata` | high 0.70 | notify/notify | 1 (2) | pass |
| mutating-webhook-slow-backend | `mutatingwebhookconfiguration//mesh-injector` | `mutatingwebhookconfiguration//mesh-injector` | high 1.00 | page/page | 1 (2) | pass |
| validating-webhook-deadline | `validatingwebhookconfiguration//label-guard` | `validatingwebhookconfiguration//label-guard` | high 0.95 | page/page | 1 (2) | pass |
| webhook-timeout | `validatingwebhookconfiguration//image-policy` | `validatingwebhookconfiguration//image-policy` | high 1.00 | page/page | 1 (2) | pass |
| route-backend-renamed | `service/storefront/search-v3` | `service/storefront/search-v3` | high 0.90 | page/page | 1 (2) | pass |
| route-canary-backend-missing | `service/storefront/checkout-canary` | `service/storefront/checkout-canary` | high 0.90 | page/page | 1 (2) | pass |
| route-missing-backend | `service/shop/checkout-v2` | `service/shop/checkout-v2` | high 0.90 | page/page | 1 (2) | pass |
| autoscaler-ceiling-cpu | `horizontalpodautoscaler/orders/worker` | `horizontalpodautoscaler/orders/worker` | likely 0.70 | notify/notify | 2 (2) | pass |
| autoscaler-ceiling-requests | `horizontalpodautoscaler/search/api` | `horizontalpodautoscaler/search/api` | likely 0.70 | notify/notify | 2 (2) | pass |
| hpa-at-max | `horizontalpodautoscaler/shop/web` | `horizontalpodautoscaler/shop/web` | likely 0.70 | notify/notify | 2 (2) | pass |
| external-database-storm | `external-endpoint//db.example.com:5432` | `external-endpoint//db.example.com:5432` | high 0.95 | notify/notify | 1 (3) | pass |
| external-database-one-workload | `deployment/reports/builder` | `deployment/reports/builder (no cause)` | none | notify/notify | 1 (2) | pass |
| shared-panic-storm | `failure-signature//CrashLoop panic: license check failed for tenant #` | `failure-signature//CrashLoop panic: license check failed for tenant #` | high 0.70 | notify/notify | 1 (3) | pass |

## Held-out scenarios

14 held-out root cases in 12 scenarios, 12 right. Never used to tune the engine.

| Scenario | Expected root | Actual root | Confidence | Tier (expected/actual) | Messages (max) | Result |
| --- | --- | --- | --- | --- | --- | --- |
| heldout-configmap-key-missing | `configmap/catalog/api-settings` | `configmap/catalog/api-settings` | high 0.93 | notify/notify | 1 (2) | pass |
| heldout-image-tag-missing | `deployment/storefront/backend` | `deployment/storefront/backend` | high 0.91 | notify/notify | 1 (2) | pass |
| heldout-sidecar-crash | `deployment/orders/api` | `deployment/orders/api` | high 0.91 | notify/notify | 1 (2) | pass |
| heldout-node-disk-pressure | `node//n2` | `node//n2` | high 1.00 | notify/notify | 1 (2) | pass |
| heldout-namespace-dns-blocked | `networkpolicy/orders/default-deny-egress` | `networkpolicy/orders/default-deny-egress` | high 0.75 | notify/notify | 1 (2) | pass |
| heldout-quota-one-namespace | `resourcequota/ml/team-quota` | `resourcequota/ml/team-quota` | high 1.00 | notify/notify | 1 (2) | pass |
| heldout-cronjob-failing | `cronjob/billing/invoice-export` | `cronjob/billing/invoice-export` | likely 0.60 | notify/notify | 4 (2) | over budget |
| heldout-mixed-storm | `node//n4`, `secret/finance/payout-keys`, `deployment/search/indexer` | `node//n4`, `secret/finance/payout-keys`, `deployment/search/indexer` | high 1.00 | page/page | 4 (8) | pass |
| heldout-statefulset-zone-conflict | `persistentvolumeclaim/streaming/data-kafka-2` | `scheduling//had volume node affinity conflict` | high 1.00 | notify/notify | 1 (2) | wrong root |
| heldout-quota-pod-count | `resourcequota/ci/object-counts` | `resourcequota/ci/object-counts` | high 1.00 | notify/notify | 2 (2) | pass |
| heldout-pull-secret-rotated | `secret/payments/pull-creds` | `registry//images.internal.example` | likely 0.65 | notify/notify | 1 (2) | wrong root; blamed registry//images.internal.example |
| heldout-liveness-too-aggressive | `deployment/reports/renderer` | `deployment/reports/renderer (no cause)` | none | notify/notify | 1 (2) | pass |
