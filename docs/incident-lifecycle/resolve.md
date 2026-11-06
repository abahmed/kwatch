# Recovering and the resolve hold

Part of [The life of an incident](../incident-lifecycle.md).

When the last member is gone the incident is `Recovering`. It resolves only
after the hold: `Hold` (3m) doubled for each recovery in the last
`FlapWindow`, at most `MaxHold` (30m), and multiplied by `ChronicFactor` when
it reopened within `ChronicWindow`. A workload that is still below its
desired replicas with failing pods, or that has a container that is
crashing, does not resolve on the timer alone (`stillBroken`, `crashing`),
but only for `StillBrokenMax` (2h, the `RepageWindow`) after the last member
left. A container counts as crashed when it is in CrashLoopBackOff or its
last run ended with a non-zero exit within the hold; a clean exit, such as a
normal init container end, does not hold an incident open. At the cap the
incident resolves anyway, with a timer armed for that moment: one pod no
detector flags must not keep an incident and its paging alert open for ever.
The reason of that resolve does not say `healthy`: it is `stopped tracking
after 2h; coverage check continues` (`StoppedTrackingPrefix`,
`resolveReason`), with `; container still crashing` added when only a
crashing container held it. A workload that is really down is handed back by
the coverage check, and a failure inside `RepageWindow` reopens the same
incident.

A failure inside the hold returns the incident to `Open` and the manager says
nothing. There is one exception. Each such failure is a recovery that counts
in `FlapWindow`, unless the workload's containers kept crashing while it
recovered (in back-off, or a run that ended with an error within the hold):
that quiet was a gap of the crash loop, as a container without a readiness
probe looks ready while it runs, so it counts no cycle and never makes the
incident flap (`crash_gap.go`). When the count reaches `FlapCycles` (3) the incident becomes
`Flapping` and sends one `flapping` update. After that a flapping incident
speaks only when the failure grows or a reminder is due, and it resolves
after `MaxHold` of quiet. It does not run the `revised` step: a resolve (or a
quiet supersede) drops any owed `cause revised` update, so it never appears
after a later reopen.

## The restore grace

A restart hides failures, it does not end them: detectors need a while to
re-raise their findings once the model is rebuilt. For `restoreGrace` (10m,
extended when the sources finish syncing) a restored incident therefore
follows these rules (`grace.go`, `restored_recovered.go`):

- A restored incident without members does not recover, and a recovering
  one does not resolve, on missing members: the hold keeps running.
- When its findings come back in the grace it returns to `Open` without
  counting a flap cycle, because the restart hid the failure, it did not
  stop. The same holds for a root kind kwatch cannot observe: its returning
  findings count no cycle either (`recovering` tests `verifiable` as well
  as `inGrace`).
- The exception is a node that was only ever failing as not ready. When the
  synced model shows the node `Ready` again and no member is back, the
  incident does not wait: it recovers at once, and its downtime ends when
  the node became ready, not when the grace ended (`recoveredWhileDown`).
  This holds only when every reason the incident ever had on its root is
  `NodeNotReady` or `NodeHeartbeatStale` (`readyEndsIt`): `Ready` is the
  recovery of those and of nothing else. A node under disk, memory or PID
  pressure, with a network or runtime fault, or with any other condition,
  can be `Ready` and still failing, so such an incident keeps the whole
  grace. A record from before the reasons were saved has none to read and
  keeps the grace too.
- A rise of tier since the record was saved is news inside the grace; see
  [Updates](./updates.md). Until the reasons saved with the record are all
  back, though, the tier is provisional (`restore_reasons.go`).
- A restored incident with no members after the grace is handed over to the
  announced incident that holds its root's failures or any of the entities
  that were its failing members when it was saved (`Record.Members`,
  `handover.go`). An incident that is still settling is never the target: it
  may never send a message, and the "Moved" close would point at nothing. The
  old incident waits until the holder is announced; if none ever is, it
  resolves on its own.
  The second covers a root that is not made again after a restart, such as
  a virtual or deleted one: its failures come back under another root, and
  the incident is closed as moved, not as healthy. One whose
  workload is still broken does not resolve as healthy
  (`workload_ready.go`). For a Service root, the pods behind it are those
  its endpoint slices name or, when no slice names any, those its own
  selector matches (`workloadsBehind`), so a missing slice cannot turn a
  still-failing Service into a "healthy" resolve.

## Resolves of listed incidents

An incident that only a listing (the startup summary, the restored
summary, a roll-up) announced, and that never spoke on its own, is known
to people by what the listing named. The listing records that as the
incident's `Subject`, and its resolve carries `Decision.AsAnnounced`: it
names that subject even when the root moved since, adds "Last affected:"
with the root it ended on, and the audit entry adds `subject` beside
`root`. The message carries `ChatCarrier`: the startup and restored
summaries keep no thread for their items, so the resolve stands alone in
chat (`placement="standalone"` in the send log), while a roll-up's item
replies in the roll-up's thread (`placement="thread"`). A paging-only
message that no paging provider takes is logged as `result="carried"`
with the listing as `carrier`, not as `not routed`.

## What a resolve says

`compose.resolveNote` leads with how the subject ended ("payments in shop
is healthy again."), then who fixed it when the fixing change is known,
how long it was failing and what it was blamed on. When something changed
in the 15 minutes before it recovered (and after its failures began), it adds
"Changed just before it recovered: ..." with at most two changes, newest
first; the fixing change is not repeated there (`changed_before_recovery.go`).
When more than one pod
failed at the incident's widest it adds "Three pods were affected.": the
count is `Journal.Widest.Pods`, the most failing pods the incident held at
once (`radius.go`), recomputed with the impact and persisted, so it
survives the members leaving and a restart.

A resolve at the `StillBrokenMax` cap does not say the incident is healthy.
Its lead says what is still wrong: "X still has a container that keeps
crashing.", "X is still short of replicas." or, for a subject without
replicas, "X has not recovered.". Then it says "kwatch stopped tracking this
after 2h; it is still not healthy, so check it yourself." (`stoppedTracking`
in `compose/resolve.go`).

## Resolves of one tick

Incidents that only a roll-up (or a namespace outage message) named never
spoke on their own, so they have no thread to close. When two or more of
them resolve in the same tick, the first resolve is one message that also
lists the others (`compose.Recovered`), and the other decisions are
recorded in the audit log as carried by it (`joint recovery`,
`jointKind` and `splitRecoveries` in `pipeline/recoveries.go`).
They still count as delivered for the roll-up listing's own close.

Only resolves that ended the same way share a message (`endOf`). A
recovery is listed under "Recovered at the same moment". The end of a
deleted object or namespace did not recover, so such resolves share their
own message under "Deleted at the same moment" (`compose.Deleted`), routed
and recorded like the recoveries: the object the message speaks of (the
listed subject when a listing named it, else the root) must be the one
deleted, and a node, which "is gone" rather than deleted, is never one.
A restored incident whose root failed by itself (it has root reasons, so
it existed) and that a fully listed model no longer holds was deleted
while kwatch was down: its resolve says it was removed even when no
deletion dated after its opening was seen (`goneUnseen` in `ended.go`).
A root without a reason of its own, which may never have existed (a
Service an Ingress routes to), is not claimed removed. A resolved
incident keeps the code of why it was at its level (`keepLevel`), so the
dashboard still explains it.
A resolve at the `StillBrokenMax` cap, and one where only something else
was deleted (a member, say), fits neither heading and stays on its own.

Members whose page was held beside the one page that stood for their
listing (`PageHeld`, see [Announce](./announce.md)) never opened an alert
at the pagers, so they join the grouped recovery too: a namespace outage
whose members recover together is one "recovered" message, not one per
member. What never joins stays on its own: a page, an incident whose
alert is open at the pagers, one that spoke in a thread of its own since
the listing named it (`SpokeOnItsOwn`), a handover and a superseded
incident. A member that recovers alone gets its own message, as before.

---

Previous: [Reminders](./reminders.md) | Next: [Reopen and supersede](./reopen.md)
