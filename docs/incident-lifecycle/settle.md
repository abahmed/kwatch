# Finding and settling

Part of [The life of an incident](../incident-lifecycle.md).

Detectors raise findings. The incident manager attaches each finding to the
incident of its root cause, opening one in `Settling` when there is none. It
says nothing yet. It waits `Settle` (75s) so the findings of one failure
arrive as one message, and so a shared cause can surface and replace many
incidents with one.

A page is urgent, so it waits only `PageSettle` (15s). The exceptions are a
failure kwatch found already there when it started (`foundAtStart`: opened
by its first look, with a member older than kwatch), which may be clearing
already and waits the full 75s, and a burst. `BurstIncidents` (3) is how many incidents may settle at once before a
page also waits the full 75s. The count (`settlingCount` in `lifecycle.go`)
takes incidents in `Settling` that have members and are at the `Notify`
tier or higher. A `Silent` or `Digest` incident never speaks on its own, so
it never counts toward a burst: a page that settles beside three digest
findings still goes out after `PageSettle`.

An incident also waits while the cluster scales (`scale_wave.go`,
`scale_hold.go`). At least 3 nodes, or a fifth of the most nodes seen if
that is more, added or removed within 10 minutes make a scaling wave; it
is over when no node was added or removed for 5 minutes. An incident of
the `Notify` tier or higher opened during the wave whose failures are all
scaling side effects (pods pending, unschedulable, creating, pulling
images, evicted, lost or not ready; workloads unavailable; Services
without endpoints; a node that is removed, being removed or joined during
the wave) settles until 5 minutes after the wave is over, at most 30
minutes from its opening. A notification is held as well as a page: told
at once, it would turn into a page a minute later. A crash loop, an OOM
kill, a node that fails without being taken out, or a cause that blames a
change other than a node's coming or going ends the hold at once. Its
timers say "Paging held: cluster is scaling".

A settling incident does not wait for evidence that is still on its way.
Holding the announcement while a pod of the failing workload, or of a
considered cause, still has a finding inside its own wait was tried and
dropped: those waits are pending for almost every failure, so it delayed
nearly every first message by one settle (page latency included) and
changed roll-ups, for the sake of a handful of late cause revisions. The
settle above is the whole wait.

An incident that recovers while settling is closed without a message and
logged ("incident resolved", reason `settled before alerting`); it is
marked untold (`Incident.Untold`), so the dashboard shows it as settled
before alerting and counts it as no page and no notification. One whose
failures all moved to another incident before anyone was told (a pod
listed before its ReplicaSet's owner at startup opens under the
ReplicaSet, then moves to the Deployment) was a duplicate: it is logged
(`moved before alerting`) and dropped. When the
same root fails again within `FlapWindow`, that incident is revived (`revive`
in `revive.go`): same ID, settling again, with the flap cycles, occurrences
and the earlier blip in its history, as a recurrence has them. A probe that
flickers every few seconds, as a waking cluster does, is therefore one
incident ID and one message, not a new ID each time. One that people were
told about is never revived; it reopens or recurs by the rules below. A `Silent`
incident is never announced until its tier rises: a drain inside its
envelope, and an incident made only of cause-only findings (`causeOnly` in
`policy.go`: a volume nearly full, an autoscaler at its maximum) are silent, and so is an incident made only of states that are not failures (`advisoryState`: CPU throttling, a quota used up that nothing was refused by) while no traffic is lost and nothing is left without a ready replica: no message of its own, not even a digest line. They speak when a real failure joins them, at the
next tick, as a digest incident does when it is promoted. One exception: a page that reached the pagers
while its announcement was held (startup summary, outage hold) and was then
restored as `Settling` after a restart. If it recovers while settling, its
pager alert would stay open for ever, so `settle` returns a `Resolve` with
`Decision.Unannounced` set. That resolve goes to the pagers alone, like the
one that closes a held page inside the summary or hold, and the incident is
still one nobody can reopen.

Not every finding earns a line of its own. A node condition that is True
and that kwatch has no detector for (`nodeConditionFindings`) is a digest
line when the reporter gave a message, quoted as written, or when its type is
one node-problem-detector sets (`problemNodeConditions`: `KernelDeadlock`,
`ReadonlyFilesystem` and the like). A True condition with neither is
advisory: it joins the incident of a real failure of the same node as
evidence and never becomes a digest line, because "Node condition X is True"
alone tells nobody anything. An expired TLS certificate is critical only when
a pod or Ingress is known to use it; with its users unknown (Ingresses or
pods not yet watched) it is a warning that says only that it expired.

## The problem title

Every incident gets a stable problem title when it opens, before any
message is sent (`problem.go`, written by the pipeline with
`compose.Writer.ProblemTitle`): the lead of its announcement, without the
cluster tag, at most 140 characters, clipped on a word. It is written
again when the open problem changes materially (another root, cause,
failure mode or set of reasons), never from an update, a recovery or a
move, and a resolved incident keeps the last one. It is persisted
(`Record.Problem`); an older record gets one when its findings are back.
The dashboard names incidents by it, so an incident nobody was told about
has a title too; the root's key is only the last resort. A Community
title says what fails and never the cause.

---

Previous: [The life of an incident](../incident-lifecycle.md) | Next: [Announce](./announce.md)
