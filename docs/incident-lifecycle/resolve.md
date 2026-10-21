# Recovering and the resolve hold

Part of [The life of an incident](../incident-lifecycle.md).

When the last member is gone the incident is `Recovering`. It resolves only
after the hold: `Hold` (3m) doubled for each recovery in the last
`FlapWindow`, at most `MaxHold` (30m), and multiplied by `ChronicFactor` when
it reopened within `ChronicWindow`. A workload that is still below its
desired replicas with failing pods does not resolve on the timer alone
(`stillBroken`), but only for `StillBrokenMax` (2h, the `RepageWindow`)
after the last member left. Then it resolves anyway, with a timer armed for
that moment: one pod no detector flags must not keep an incident and its
paging alert open for ever. The reason of that resolve does not say
`healthy`: it is `stopped tracking after 2h; coverage check continues`
(`StoppedTrackingPrefix`, `resolveReason`). A workload that is really down
is handed back by the coverage check, and a failure inside `RepageWindow`
reopens the same incident.

A failure inside the hold returns the incident to `Open` and the manager says
nothing. There is one exception. Each such failure is a recovery that counts
in `FlapWindow`. When the count reaches `FlapCycles` (3) the incident becomes
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
  [Updates](./updates.md).

## Resolves of one tick

Incidents that only a roll-up (or a namespace outage message) named never
spoke on their own, so they have no thread to close. When two or more of
them resolve in the same tick, the first resolve is one message that also
lists the others (`compose.Recovered`), and the other decisions are
recorded in the audit log as carried by it (`joint recovery`,
`jointRecoverable` and `splitRecoveries` in `pipeline/recoveries.go`).
They still count as delivered for the roll-up listing's own close.

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
