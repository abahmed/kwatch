# What shapes what people hear

Part of [The life of an incident](../incident-lifecycle.md).

## Coverage check

A backstop split over three files that `internal/pipeline/coverage/doc.go`
explains in one place: the memory and timings (`pipeline/coverage/watch.go`),
the engine step that hands back (`pipeline/coverage.go`) and the list of
covered workloads (`incident/coverage.go`). It exists because an incident
can be lost (resolved while its workload still fails, or absorbed under
another that closed), and then nobody would be told.

- Every coverage `Every` (5m) it looks at Deployments, StatefulSets and
  DaemonSets.
- It picks the ones that have been below their desired replicas for
  `coverage.After` (15m) and have none ready, or keep restarting. 15m is
  longer than `kube.BootWindow` (10m), so a node pool that is still booting is
  never mistaken for a lost incident.
- A workload is covered when a live incident with failing members speaks for
  it at any tier (`CoveredWorkloads`): as its root or the owner of a failing
  member, and, for notify tier or higher, through its impact. A digest or
  silent incident is quiet on purpose, so it covers. The exception is a digest
  incident that must speak: it has lasted past `kube.BootWindow` with a crash
  loop or nothing ready, and the tier it would have once `persistent` is above
  the digest (`escalationOwed`). `escalate` and the check share one test for
  that (`lastsPastBoot`), so the check fires only for incidents that were
  genuinely lost. The boot window is measured from the later of `Opened`
  and the first tick after a restore (`bootStart`): the pods of a restored
  incident boot again after a restart, so an 8 hour gap does not turn
  their boot noise into a lasting failure.
- If no live incident covers one and it has an active failure finding (not a
  configuration risk), the findings go back to the incident manager. They take
  the normal settle and announce path. Handing back findings an incident
  already holds changes nothing (`Apply` is idempotent).
- A workload just handed back is left alone for `coverage.Retry` (30m), so one
  the manager cannot cover is not retried every five minutes. Workloads that
  left the cluster are forgotten.
- It logs one info line from the `pipeline` component: "coverage check found
  failing workloads no incident covers; an incident was lost, opening one",
  with the workload names. Seeing it means a bug upstream.

## Other things that shape what people hear

- **Namespace outage hold** (`pipeline/announce/outage.go`, thresholds in
  `announce/outage_shared.go` and `announce/timings.go`).
  Announcements (notify tier or higher) of one namespace that have no cause are held for up to
  `outageHoldMax` (30s) so one message can name them all. Five incidents
  (`outageIncidents`) that opened within `outageWindow` (10m) are an outage;
  so are three (`outageMinShare`) when they are at least half the
  namespace's workloads. A smaller group is held only after one is already
  held. If a held incident that had paged resolves before it was told, its
  paging alert is closed (so does one the startup summary holds). Incidents
  released as not an outage are announced one by one and are no longer held;
  the one the hold already paged goes to chat only (`Decision.PagedAlready`),
  so the pagers are not told twice. A resolve of a held incident drops its
  announcement (`DropAnnouncement`): it stays "nobody heard of it".
- **Boot grace.** A node pool scaled up from zero starts every pod at once.
  For `kube.BootWindow` (10m) pods that are pending or not ready on a booting
  node are expected, not failures (`detectors/boot.go`). A digest-tier
  incident is promoted (`escalate` in `worsen.go`) only when it has lasted
  `BootWindow` and a crash keeps going or the workload is down.
- **Wake-up and scale-down.** A cluster put to sleep at night wakes up all
  at once: five workloads (`kube.WakeMinWorkloads`) set from 0 replicas to
  some, or three nodes (`WakeMinNodes`) joining, each start less than
  `WakeQuiet` (10m) after the one before (`inventory/kube/wake.go`). While
  it lasts, at most `WakeMax` (30m) from its first start, the pods it
  created get the boot grace above, so probe failures, pending pods and
  empty Services wait. A pod that crashes, restarts or cannot pull its image
  is never held, and what still fails when the wake-up ends is reported at
  once, with "the cluster was waking up" as context
  (`Facts.Wake`). A wake-up in which pods had startup warnings
  and recovered costs one line in the next digest ("Cluster waking up: 42
  workloads started between 06:51 and 07:03; 5 had brief startup failures,
  all recovered"); one without is not mentioned. The mirror image, five
  workloads set to 0 within `ScaleDownSpan` (15m), is a planned scale-down:
  those workloads raise no "scaled to 0 but still routed" finding, and a
  cordoned node that leaves is a drain (`NodeDraining`), not a loss.
- **Restart.** Incidents are saved and restored. For `restoreGrace` (10m) a
  restored incident without members does not recover, because detectors
  have not re-raised its findings yet (`grace.go`). A restart announces no
  incident, so when the grace is over one summary (`ListRestored` in
  `pipeline/announce/startup.go`) names the restored incidents that still fail
  and have not spoken for themselves since the restart, digest-tier ones
  after the louder ones, within the usual cap, so nothing restored is
  silent. On a
  cold start the first `StartupWindow` (2m) collects existing incidents into
  one startup summary; an active page still reaches the pagers at once.
  What a restart keeps: the incident records (with the worst stage reached,
  `stagePeak`, so a crash loop told once is not told again, and the
  announced route), the startup marker with its open roll-ups (also those
  of a cold start that stopped inside its window), and the part of the
  pending digest that no record holds (`DigestState`: reminder and `failing
  again` lines, resolves of listed incidents, and the listed incidents
  whose own first message must introduce them). Not kept: `sentTier`,
  `sentGrowth` and `lastMaterial` of `Delivery`, a fix attempt and its late
  flag, and the evidence investigation found (the next update may quote it
  again).
- **Digest.** An incident a digest listed has not been introduced, so its
  first message of its own (when it rises out of the digest tier) is an
  announcement, as for a startup summary or a roll-up. A decision the digest
  takes in and leaves out (an update with nothing to list, the resolve of a
  blip nobody heard of) is audited as `dropped`, not as carried by the
  digest.
- **Listings and resolves.** A roll-up or summary counts a member as having
  said it resolved only when that resolve reached chat. A resolve sent to
  the pagers alone, or to nobody (`unannounced`), leaves the roll-up to close
  with its own message once everything it named is over.
- **Saving.** A held announcement that expires or whose investigation
  result arrives is handed to delivery, so the loop saves the incident
  records at once (a crash then does not announce it again). The cold-start
  window end is a wake deadline. Fingerprints of the model are built only
  when the store will write them (`fingerprintInterval`) and at shutdown.
  An open incident keeps its investigation evidence however old; the cap
  bounds it. A panicking investigator is logged and its result dropped, and
  an investigator that ignores its budget stops holding a worker once the
  budget and a short grace have passed. Each such abandoned goroutine is
  counted per investigator kind (`InvestigationsAbandoned`); a kind with
  `maxAbandonedPerKind` (4) of them still running gets no new jobs until
  one returns, so a stuck read leaks a bounded number of goroutines.
  A material-change update held for its investigation is saved with the
  fingerprint from before it (`Record.HeldUpdate`), so a restart that loses
  it decides it again.

## Where the flags live

Delivery-facing state sits in two structs on `Incident`, defined in
`internal/incident/flags.go` and changed only through their methods:
`Delivery` (`MarkPaged`, `ClosePage`, `HoldAtNotify`, `MarkRolledUp`,
`RecordSent`, `MarkMaterial`) and `Pending` (`ScheduleReopenUpdate`,
`ClearReopen`, `MarkRevised`, `ClearRevised`). Decision reasons are the
typed `incident.Reason` constants listed in `AllReasons`.

---

Previous: [Examples](./examples.md)
