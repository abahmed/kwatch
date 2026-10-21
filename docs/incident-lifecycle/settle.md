# Finding and settling

Part of [The life of an incident](../incident-lifecycle.md).

Detectors raise findings. The incident manager attaches each finding to the
incident of its root cause, opening one in `Settling` when there is none. It
says nothing yet. It waits `Settle` (75s) so the findings of one failure
arrive as one message, and so a shared cause can surface and replace many
incidents with one.

A page is urgent, so it waits only `PageSettle` (15s). The exception is a
burst. `BurstIncidents` (3) is how many incidents may settle at once before a
page also waits the full 75s. The count (`settlingCount` in `lifecycle.go`)
takes incidents in `Settling` that have members and are at the `Notify`
tier or higher. A `Silent` or `Digest` incident never speaks on its own, so
it never counts toward a burst: a page that settles beside three digest
findings still goes out after `PageSettle`.

A settling incident does not wait for evidence that is still on its way.
Holding the announcement while a pod of the failing workload, or of a
considered cause, still has a finding inside its own wait was tried and
dropped: those waits are pending for almost every failure, so it delayed
nearly every first message by one settle (page latency included) and
changed roll-ups, for the sake of a handful of late cause revisions. The
settle above is the whole wait.

An incident that recovers while settling is dropped silently. When the
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

---

Previous: [The life of an incident](../incident-lifecycle.md) | Next: [Announce](./announce.md)
