# Announce

Part of [The life of an incident](../incident-lifecycle.md).

When the settle is over the incident becomes `Open` (or `Flapping`, if the
same root has recovered `FlapCycles` times within `FlapWindow`) and the
decision is `Announce`. The tier decides the audience: page, notify or
digest (`policy.go`). A page that follows another page of the same failure
within `RepageWindow` is held at notify, so one flapping outage does not
page again and again (`isPage` and `reachedPaging` in `flags.go` say what
counts as a page). After the announcement the tier only falls when
nothing but digest findings is left, never from a page (see
[Updates](./updates.md)).

## Who is paged: user impact

A page interrupts someone at any hour, so it needs a critical member and
one of the rules in `pageRules` (`page_rules.go`). Each rule is a way users
feel the failure:

1. **Users lose traffic.** A Service that users reach from outside has no
   ready backend: an Ingress or Gateway route sends traffic to it, or it is
   a LoadBalancer or NodePort Service (`traffic-lost`,
   `exposed-service-lost`; `trafficLost` in `graph.go`).
2. **A user-facing workload has no replica left.** Every replica of a
   Deployment, StatefulSet or DaemonSet that such a Service selects is not
   ready (`last-replica-down`; `servingDown` in `serving.go`). A workload
   that only other workloads call, behind a ClusterIP Service, is internal:
   when it fails, the workloads that need it show the user impact and are
   judged instead, and the incident notifies.
3. **A cluster-critical component is down.** Cluster DNS, the API server and
   etcd, the scheduler, the controller manager, a fail-closed admission
   webhook that blocks every create, or a node, which takes its capacity and
   its network plugin with it (`node-lost`).

Everything else that is critical notifies; what is only worth knowing waits
for the digest. The rules live in code and are listed here, so change both
together. A repeat of a page that just resolved is held at notify.

## Behaviour against the workload's own normal

The inventory keeps a rolling baseline for each workload (its top owner)
over the last seven days (`inventory/baseline*.go`, stored in the
`baselines` bucket): restarts per hour, how long its pods take to become
ready, the largest container memory per hour, and Warning events per hour
by reason. It uses robust statistics (the median and the median absolute
deviation, or the p95) and judges nothing until a statistic has ten
samples, so a new workload is never judged. A new pod template starts the
memory statistic again, because new code has new memory; the restart and
readiness statistics carry on so a bad release is judged against what came
before it. Baselines describe the past only; kwatch never forecasts.

Detectors compare current behaviour with that normal and set
`Finding.Normal`:

- `Unusual`: far above the normal range. The message quotes it ("That is
  unusual for this workload: restarts 12×/h vs a usual 0.1×/h"), and
  restarts from a workload that never restarts are never written off as
  known or routine (`unusual.go`).
- `Usual`: inside the normal range, such as a worker that always restarts
  twice an hour. Only three findings may be excused this way: the plain
  "keeps restarting" finding, memory near (but below the critical level of)
  its limit, and a pod still starting inside its workload's usual readiness
  time. They go to the digest with the baseline shown. A crash loop, an OOM
  kill, a failing start or a Critical finding is never excused, and once the
  workload has been down past the boot window (`persistent`) nothing is.

## Rollouts that are progressing

While a Deployment's Progressing condition says its rollout is under way
(`ReplicaSetUpdated`, `NewReplicaSetCreated`, `FoundNewReplicaSet`) or a
StatefulSet rolling update moved within `DefaultRolloutStuck`, the
"replicas not ready" finding is held (`detectors/rollout_hold.go`), and a
young pod's readiness-probe failures wait for its start budget. Nothing
else is held: a new pod that crashes or cannot start has container
findings of its own, a stalled rollout is `ProgressDeadlineExceeded` (or
the StatefulSet's own stuck finding), and a pod that stays unready past its
start budget is reported as before. A Deployment's hold ends at the latest
`rolloutHoldMax` (30m) after its newest ReplicaSet was created. Release
watch (`release_watch.go`) still compares the new revision's restarts with
the old one's, and the node boot grace (`boot.go`) is separate: both can
apply to one pod.

---

Previous: [Finding and settling](./settle.md) | Next: [Updates](./updates.md)
