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
takes incidents in `Settling` that have members and are not of the `Silent`
tier. A `Silent` incident never speaks, so it never counts toward a burst.

An incident that recovers while settling is dropped silently. A `Silent`
incident is never announced. One exception: a page that reached the pagers
while its announcement was held (startup summary, outage hold) and was then
restored as `Settling` after a restart. If it recovers while settling, its
pager alert would stay open for ever, so `settle` returns a `Resolve` with
`Decision.Unannounced` set. That resolve goes to the pagers alone, like the
one that closes a held page inside the summary or hold, and the incident is
still one nobody can reopen.

---

Previous: [The life of an incident](../incident-lifecycle.md) | Next: [Announce](./announce.md)
