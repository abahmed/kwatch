# Production goals

This page states what kwatch must achieve in production. Operators use it to
know what to expect. Contributors use it as the bar for every change.

## Mission

kwatch runs as one pod in the cluster. It understands every resource and
sends one clear, correct, narrative message per real incident:

- what broke;
- why, with the evidence;
- the impact;
- the next step, with read-only `kubectl` suggestions. Anything that
  changes the cluster is labelled as a change.

There are no user commands or APIs. The only endpoints are `/healthz`,
`/readyz`, `/availabilityz`, `/health`, and `/metrics`.

## Alert-quality gates

CI measures these on synthetic replay and labelled scenarios with
`make alert-quality-gate`, part of `make verify`. Current numbers are in
[internal/scenarios/SCORECARD.md](../internal/scenarios/SCORECARD.md).

| Goal | Target |
| --- | --- |
| Correct root cause, labelled scenarios | at least 90% |
| Correct root cause, held-out scenarios | at least 80% |
| Wrong high-confidence root | at most 5% of high-confidence cases, labelled and held-out together, gated once there are at least 20 |
| Calibration, high confidence | right 80% to 100% of the time, gated at 10 or more cases |
| Calibration, likely confidence | right 50% to 90% of the time, gated at 10 or more cases |
| Messages per incident | p95 at most 3, most at most 5 |
| Time to first message, page tier | at most 2 minutes from the first failure observation |
| Time to first message, notify tier | at most 5 minutes from the first failure observation (aim: p95 at most 3 minutes) |
| Notifications caused by non-events | 0 (successful rollouts, scaling, drains within budget, Jobs completing, a healthy CronJob) |
| Notifications in the busiest hour | at most 30, a sanity ceiling |
| Unchanged updates | 0% |
| Re-created incidents | at most 5% |
| Repeated recoveries | 0 |
| Storm of 1,000 failing pods from one cause | at most 3 messages in 2 minutes |
| Storm of 1,000 failing pods from several causes | at most one message per cause plus 3 in 2 minutes |
| A healthy node with a crashing app | never blames the node |

Calibration is two-sided: overconfidence and underconfidence both fail
the gate. The confidence levels are bucket boundaries chosen once from
the labelled scenarios only (`scorecard.CalibratedHigh`) and committed as
constants; the scorecard prints what the method gives for the current
labelled set next to the committed value. Time to first message is
measured on the simulated clock of the replay, for each labelled
scenario that notifies or pages. The latency targets were set to two
minutes (page) and five minutes (notify) on 2026-10-01 by the
maintainers' decision; they leave room for the detectors' grace periods
plus the settle.

The held-out scenarios are written from descriptions of Kubernetes
failures and are never used to tune the engine. A held-out miss is
reported, not fixed by changing rules, weights, tiers or labels to fit
it; fixing the failure needs its own labelled scenario first. Held-out
results are scored and reported separately from the labelled set.

Once held-out scenarios have been looked at to find the generic gap
behind a miss, they are no longer held out. They move into the labelled
set under their own names, and the same number of fresh held-out
scenarios, for other failures, are written with their labels fixed
before they are first replayed. The first rotation, on 2026-10-01, is
recorded in the scorecard.

## Intelligence without feedback

kwatch does not learn from user feedback. It reasons from the cluster:

- Causes need evidence, and negative evidence counts against them.
- A set-cover explanation picks the smallest set of causes that explains
  all symptoms.
- Causes are re-verified continuously, and outcomes are confirmed before
  an incident resolves.
- Baselines and recurrence memory separate normal from new.
- Unknown never blames anything and never resolves an incident.

## Coverage

- Every listable Kubernetes resource, plus Gateway API and snapshots.
- Any CRD, through generic health and link handling.
- Every known failure mode maps to a health mode.

## Scale

- 5,000 pods and 500 nodes within the default 512Mi memory limit (the Go
  memory limit is set at 90% of it). Tests assert a live heap of at most
  250 MiB and a peak of at most 512 MiB; the measured live heap is about
  83 MiB.
- Decisions are fresh: at 5,000 pods, the p99 time from an observation
  being submitted to its decisions being applied is at most 2 seconds
  in-process. Production exports it as
  `kwatch_pipeline_decision_lag_seconds`.
- No I/O on the decision loop.
- The store is capped at 512 MiB of logical data (evidence 128 MiB) with
  30-day retention, and its file size is held to the same cap.

## Reliability

- One replica with a Lease lock. This is not HA. A restart gap of 1 to 2
  minutes is accepted.
- A warm restart in the middle of an incident announces it no second
  time, and queued outbox messages are sent once after a restart.
- No lost messages across restarts: delivery is at least once through a
  persisted outbox (at most 2048 jobs, none older than 24 hours). A rare
  duplicate is possible, because a send interrupted mid-request may repeat.
- Queued alerts are sent or dead-lettered at shutdown, within the 60-second
  grace period; whatever is still queued stays in the outbox for the next
  start.
- A stale writer is fenced.
- A corrupt record is skipped.
- A schema mismatch or an unreadable file causes a reset (the old
  file is kept as `state.db.corrupt`; no migration).

## Security

- Secrets are only ever seen as hashes.
- All evidence is redacted.
- RBAC is read-only, with a least-privilege mode.
- kwatch writes nothing to the cluster except its own Lease and
  SelfSubjectAccessReviews for its permission audit (not persisted).

## Messages

- Sentences are built dynamically from the facts of the case.
- One status emoji everywhere: 🔴 page, 🟠 notify, 🟡 low, ✅ resolved.
- No labels, sections, or links.
- The startup and upgrade notices are single sentences. Summaries and plain
  notices are not sent to PagerDuty, Opsgenie, GoAlert, Zenduty, Squadcast,
  SIGNL4, iLert, incident.io, GitLab, Gitea, GitHub, Jira or ClickUp, because
  nothing would close what they open. Splunk On-Call, Alerta, Sensu Go,
  Datadog, New Relic, SNS and Splunk HEC receive them as informational.

## Code quality

- One vocabulary: inventory, detection, rootcause, incident,
  notification/compose, delivery, driven by the pipeline.
- A `doc.go` in every package.
- Rules are data, not branching code.
- Small functions, within the limits in `AGENTS.md`.

## Out of scope for now

- User feedback and learning from it.
- LLMs.
- Alertmanager integration.
- Team routing and escalation.
- Digests and quiet hours.
- Ack and snooze inbox.
- Shadow mode.
- User APIs and CLI.
- Self-health incidents.
