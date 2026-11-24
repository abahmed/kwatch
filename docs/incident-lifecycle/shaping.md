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
  Announcements (notify tier or higher) of one namespace that have no cause,
  or that blame their own workload (a Service in front of it does that:
  `Incident.Standalone`), are held for up to
  `outageHoldMax` (30s) so one message can name them all. Five incidents
  (`outageIncidents`) that opened within `outageWindow` (10m) are an outage;
  so are three (`outageMinShare`) when they are at least half the
  namespace's workloads. A smaller group is held only after one is already
  held. If a held incident that had paged resolves before it was told, its
  paging alert is closed (so does one the startup summary holds). Incidents
  released as not an outage are announced one by one and are no longer held;
  the one the hold already paged goes to chat only (`Decision.PagedAlready`),
  so the pagers are not told twice. A resolve of a held incident drops its
  announcement (`DropAnnouncement`): it stays "nobody heard of it". The last
  outage message of a namespace is remembered for `outageWindow`: a failure
  of that namespace a minute or two later, too few to be an outage alone, is
  held and sent as one more outage message that names the whole outage
  (`outage_recent.go`), and, if the outage paged and its alert is still
  open, every page-tier member of it, the first late one included, is held
  at notify (`newOutageHold` starts the hold as paged), so one alert stands
  for the whole outage.
- **Boot grace.** A node pool scaled up from zero starts every pod at once.
  For `kube.BootWindow` (10m) pods that are pending or not ready on a booting
  node are expected, not failures (`detectors/boot.go`). A digest-tier
  incident is promoted (`escalate` in `worsen.go`) only when it has lasted
  `BootWindow` and a crash keeps going or the workload is down.
- **Shared dependency removed**
  (`rootcause/explain/virtual_removed_service.go`). A job that deletes
  a shared cache or database (its Service and pods) breaks every
  workload that uses it within minutes, and each says so in its own
  client's words. When at least `RemovedDependencyMinWorkloads` (3)
  workloads begin to fail within `RemovedDependencyWindow` (5m) of the
  deletion, on either side, of a Service their own errors or log lines
  name (`cache.data:6379`, `cache.data.svc.cluster.local`), the
  `dependency-removed` row roots them all at that Service: one incident,
  one root message ("Service cache in data was removed at 10:15 UTC; 27
  workloads that use it are not ready"), at most one page, one recovery.
  Members join the open incident as they fail. A workload that also fails
  for a reason of its own keeps its own incident, and a Service that only
  a configuration names never outranks the one the errors name. Fewer
  users, or a deletion long before they failed, fall back to the missing
  Service rows.
- **Leftovers of a removed controller** (`incident/leftover.go`,
  `incident/leftover_link.go`). What a removed or stopped controller left
  behind is one Low incident rooted at that controller, not one per
  object. A detector marks what it sees (a webhook calling an absent
  backend, the expired certificate named after it); `linkLeftovers` then
  links the low findings of other detectors that name the same
  controller: an autoscaler whose target is gone, a budget that selects
  nothing by the gone workload's labels, a Helm release stuck
  uninstalling, an expired certificate named after it (same anchor or
  `kube.NamedAfter`, in the anchor's namespace), and objects stuck on a
  finalizer of the API domain its webhooks serve. The root is the best
  anchor: the workload, else its Service, else its release. Workloads of
  one namespace parked at zero replicas while routes still reach them are
  one Low incident for the namespace that lists them. A live failure is
  never a leftover. An old group stays Low when a younger object stuck
  on the same controller joins it, after a restart too: its age is that
  of its oldest leftover (`chronicAge`), and a leftover that flips
  between Info and Warning is not a rise (`rose`).
- **Young-node network setup** (`detectors/event_young_node.go`,
  `incident/young_node_page.go`). Pods on a node younger than
  `kube.BootWindow` that cannot set up their sandbox are held and dropped
  if they start; the node's own sandbox finding is held the same way.
  What still fails when the node is no longer young is one incident for
  the node (its network plugin is not ready), not one per workload. A
  pod network setup failure on a node younger than `kube.YoungNodeWindow`
  never pages (level `young-node-network`).
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
  cordoned node that leaves is a drain (`NodeDraining`), not a loss. Once
  the scale-up that followed brought at least five of its workloads back
  and `kube.LeftBehindGrace` (15m) has passed since that wake-up was
  over, a routed workload it left at zero is told as usual
  (`kube.LeftBehind`).
- **Scheduled operations.** kwatch keeps a short history of the mass
  scale-downs (five workloads set to 0 and/or three nodes that stopped
  reporting or were removed, each within `kube.ScaleDownSpan` of the one
  before) and mass scale-ups (wake-ups) it saw: at most
  `plannedMaxOps` (32) of them, for at most five days, saved with the
  digest state (`DigestState.Planned`) so a restart keeps it
  (`incident/planned_ops.go`). One that begins within `PlannedTolerance`
  (30m) of the time of day of the same kind of operation on at least
  `PlannedMinDays` (2) of the previous `PlannedDays` (3) days is
  scheduled: a cluster put to sleep every evening and woken every
  morning. Nothing is predicted: only operations seen are compared, so
  the first days (and a schedule that moves by more than the tolerance,
  such as a local-time schedule on the day clocks change) are treated as
  before. While a scheduled scale-down's effect lasts, until
  `PlannedGrace` (15m) after the scale-up that followed it was over, or
  24h when none follows, an incident whose every failure it explains
  settles without a message (`incident/planned_hold.go`): a node it
  switched off or removed, a workload it scaled to zero or what depends
  on one (the incident's root or cause), or a scaling side effect
  (`scaleSideModes`) on one of its workloads or on pods of its nodes.
  One that clears meanwhile closes silently; what still fails when the
  effect ends did not come back and is told as usual. A crash or any
  other failure the operation does not explain is told at once. The
  morning's wake-up hold is unchanged. Each scheduled operation costs one
  low-priority digest line, instead of the wake-up line for a scale-up:
  "Scheduled scale-down at 18:02 UTC (seen at this time on 3 of the last
  3 days): 27 workloads, 4 nodes." The dashboard shows a held incident
  as held while the cluster scales (`Timers.ScalingHeld`). Community
  keeps the history only while kwatch runs.
- **Restart.** Incidents are saved and restored. Until its model is
  rebuilt (the sync, a 2m settle, its kinds synced; at most `restoreGrace`,
  10m) a restored incident without members does not recover, because
  detectors have not re-raised its findings yet (`grace.go`). A restart announces no
  incident, so when the grace is over (`WarmAt`) one message
  (`ListRestored` in `pipeline/announce/restored_list.go`,
  `sendRestart` in `restart_message.go`) is the whole restart: the gap
  report the engine held for it (`HoldGap`), the restored incidents that
  are news, and the count of the ongoing ones told before and unchanged.
  News is a restored incident at the notify tier or higher whose findings
  still call for it (`currentNews`) that nobody was told about
  (`toldBefore`) or whose live tier (`Incident.LiveTier`) is above the
  tier it was told at, plus the problems held after the restart
  (`holdWarm`). Told and unchanged ones, and the digest-bound ones
  (`Incident.DigestBound`), are only counted; the message's tier comes
  from the news alone. With no gap and no news nothing is sent and the
  audit log alone records the restart (`CarrierQuietRestart`). Findings
  back after the grace are named only when they are news
  (`listLateRestored`). A restored incident that only a listing named and
  that never spoke on its own (`SpokeOnItsOwn`) is owed one
  introduction: at the end of the grace it is queued like a roll-up's
  (`noteRestoredIntros` in `restored_intro.go`, `queueRestoredIntros` in
  `pipeline/introduce.go`), sent once its investigation is ready,
  threaded under the listing, and never paging by itself; once sent, the
  startup marker records it with its told fingerprint
  (`StartupState.Spoke`, `NoteSpoke`) until the incident is over, so a
  later restart does not introduce it again. The incident record keeps
  the decided introduction too (`Marks.Introduced`), so an incident is
  introduced at most once in its whole life, also after it resolved and
  reopened under the same ID. A told, unchanged incident the restart only
  counted was not named by it and is owed nothing (`restoredNews`). The
  audit entry names the news first, then the counted ones. On a
  cold start the first `StartupWindow` (2m) collects existing incidents into
  one startup summary, and an active page still reaches the pagers at once
  (Pro; Community announces each still-failing one after the startup
  window).
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
- **A node removed while kwatch was down** counts as a displacement only
  for incidents rooted at that node: the saved record keeps no members and
  no list of the nodes an incident's pods ran on, so the replaced pods of a
  workload incident are not held as node churn after a restart.
- **Digest.** An incident a digest listed has not been introduced, so its
  first message of its own (when it rises out of the digest tier) is an
  announcement, as for a startup summary or a roll-up. A decision the digest
  takes in and leaves out (an update with nothing to list, the resolve of a
  blip nobody heard of) is audited as `dropped`, not as carried by the
  digest. The audit entry words the first `MaxAuditItems` (100) incidents of
  a digest and ends with "and N more: id, id, ..." naming the others by ID
  (up to `MaxHiddenIDs`).
- **Introducing listed incidents.** A roll-up or a namespace outage only
  names its members: it says they fail, not why. Each member whose
  investigation is in within `outputWait` gets one first message of its
  own, threaded under the listing and written as an announcement, with the
  cause, the evidence and the confirmation the checks give
  (`incident.ReasonIntroduced`, `pipeline/introduce.go`). It goes to chat
  only, never to the pagers (so it opens no alert a later resolve would
  close), never twice (`Marks.Introduced`, saved with the record), not
  for a member that already spoke on its own, and never with the
  fingerprint the incident's last own message already told
  (`Marks.SaidDigest`, `Manager.NoteSaid`): that would be the same
  message again. A member a namespace outage still holds waits for that listing to
  go out, within the same bound. Without it, a failure that just goes on (a crash
  loop) never changes enough to send an update, and its cause would stay
  untold. A member without an investigation, or whose investigation is not
  in time, is not introduced.
- **Confirmation follows the cause.** An investigation starts when the
  incident opens, often before its cause is known, and confirmation needs a
  cause or a failing root. A message whose stored investigation confirmed
  nothing concludes again over the same reads for the incident as it is
  then (`RootInvestigator.Recorrelate`), so a cause found later is
  confirmed by what was read: an OOM-killed run, say.
- **Listings and resolves.** A roll-up or summary counts a member as having
  said it resolved only when that resolve reached chat. A resolve sent to
  the pagers alone, or to nobody (`unannounced`), leaves the roll-up to close
  with its own message once everything it named is over.
- **A panic costs one step.** The manager advances each incident on its own
  and the announcer runs each collecting step (startup summary, digest,
  outage hold, roll-up, listings) on its own, contained: a step that panics
  takes nothing, the decisions of the tick go on to the next step and are
  delivered, and the panic is counted (`kwatch_panics_recovered_total`).
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

## The journal: what the dashboard keeps

`Incident.Journal` (`journal.go`) is the incident's own record of its life,
for the dashboard. Nothing that decides a message reads it: no fingerprint,
tier or timer does, so it never changes what anyone hears. Every list is
bounded and, but for the timers, persisted with the record; an older record
restores with an empty journal.

- **Transitions** (50 newest): each state change (opened, announced,
  recovering, failing again, flapping, reopened, resolved) and each tier
  change of a live, announced incident (tier raised / lowered, from and
  to). `observe` compares state and tier with the last observation, at the
  end of every Apply for the incidents it changed and after every tick; a
  restart is no transition.
- **Breaks** (2 × 10 containers, `breaks.go`): the failing containers of
  the incident's pods (ready, restarts, state and last exit reasons, exit
  code, image, requests, limits and usage) when it opened, and again,
  replacing the second, each time it got worse: its tier rose or it
  reopened.
- **Changes** (12, `linked_changes.go`): the changes `ChangesBetween`
  finds around the root from `RecentWindow` before the onset until now,
  looked for once a minute while the incident is live and not silent, so
  they outlive the two-hour change history. `SafeChange` keeps at most 6
  fields per change, redacts and cuts values to 160 bytes, and names the
  data of a Secret or ConfigMap by its key only.
- **ErrorKey**: the same-error key (`SameErrorKey`: mode and cause rule),
  stored when the incident is announced so a later cause does not regroup
  it.
- **Level**: the code of why the incident is at its tier (`tierWhy` beside
  `tier`, plus the overrides, the ratchet and lowering; `level.go`).
  `AlertLevelText` turns it into plain words.
- **Timers**: the settle, the resolve hold and the next deadline as of the
  latest tick, noted with the deadlines in `nextWake`. `WhyOf` turns them
  into "Looks better; closes after 3m healthy, 1m20s so far". Not
  persisted.

## Where the flags live

Delivery-facing state sits in two structs on `Incident`, defined in
`internal/incident/flags.go` and changed only through their methods:
`Delivery` (`MarkPaged`, `ClosePage`, `HoldAtNotify`, `MarkRolledUp`,
`RecordSent`, `MarkMaterial`) and `Pending` (`ScheduleReopenUpdate`,
`ClearReopen`, `MarkRevised`, `ClearRevised`). Decision reasons are the
typed `incident.Reason` constants listed in `AllReasons`.

---

Previous: [Examples](./examples.md)
