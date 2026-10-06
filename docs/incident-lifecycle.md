# The life of an incident

A guide for contributors who touch `internal/incident`. It follows one
incident from the first finding to its last message. The code is in
`lifecycle.go` (the per-tick state machine), `open_steps.go` (what an open
incident may say) and `timings.go` (every duration, each with its reason).
The settings that can be changed are in [configuration](./configuration.md);
the package map is in [contributor architecture](./contributor-architecture.md).

The manager is ticked by the pipeline loop. Each tick it moves every live
incident one step and returns the decisions that deserve a message. A
decision is `Announce`, `Update` or `Resolve`; the pipeline gathers the facts
for it (`Decision.Facts`: output, evidence, recent changes, kind spellings)
and `internal/notification/compose` writes the text from the decision alone.

## The states

```text
 Settling -> Open -> Recovering -> Resolved
                \        |            |
                 \--> Flapping        +-- reopens (same incident) within
                                           RepageWindow
```

## 1. Finding and settling

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

## 2. Announce

When the settle is over the incident becomes `Open` (or `Flapping`, if the
same root has recovered `FlapCycles` times within `FlapWindow`) and the
decision is `Announce`. The tier decides the audience: page, notify or
digest (`policy.go`). A page that follows another page of the same failure
within `RepageWindow` is held at notify, so one flapping outage does not
page again and again (`isPage` and `reachedPaging` in `flags.go` say what
counts as a page).

## 3. Updates

Updates come from four places (the same list is at the top of
`open_steps.go`):

1. `openSteps` (`open_steps.go`): the news of an `Open` incident, below.
2. `reopenUpdate` (`reopen.go`, called from `advance`): the `failing again`
   update of a reopened incident, before the steps.
3. The `recovering` and `flapping` handlers (`lifecycle.go`): the `flapping`
   update when recoveries reach `FlapCycles`, and what a flapping incident
   says (`flappingNews`).
4. `escalate` (`worsen.go`, called from `advance`): raises a digest incident
   to notify once it has lasted past `kube.BootWindow`. It sends nothing
   itself; the next step sees the changed fingerprint.

While the incident is open, `openSteps` is read in this order, and the first
step that decides ends the tick, so one tick sends at most one update:

1. **evidence**: no members means it is recovering (or waiting for the
   restore grace after a restart); nothing is said.
2. **revised**: the cause changed. The update `cause revised` waits
   `ReviseSettle` (30s) so what joins the new cause is in the same message.
3. **attempt** (`fix_attempt.go`): someone changed the workload or its
   config: `fix attempt`, and `fix attempt still failing` once `FixWatch`
   (10m) later. Only for the `Notify` tier or higher, only while the incident
   has sent at most `attemptMessageBudget` (3) messages, and only when
   nothing else is pending: if the fingerprint differs from the last digest,
   the attempt steps aside for the material change. The attempt is recorded
   when it is seen (`observeAttempts`, in `Apply`), so a step that ends the
   tick earlier (a held material change, a pending revision, a reopen
   update) delays the update but never loses it: it goes out on the first
   later tick no earlier step ends.
4. **reminder**: nothing changed and a reminder is due: `still open`.
5. **material**: the fingerprint changed (a tier rise, a worse stage, a
   bigger impact, more failing pods). `MaterialGap` (10m) spaces two
   material-change updates. Only `materialStep` starts the gap (it sets
   `Delivery.lastMaterial`); an announcement, a reminder, a revision or a fix attempt
   does not. A tier rise skips the gap. While a change is held by the gap the
   tick ends here, so a fix-attempt update also waits, up to `MaterialGap`,
   until the material update goes out and the digest catches up (it is
   recorded meanwhile, see step 3).

A reopened incident's `failing again` update comes before all of these. If
the reopened incident recovered before the update was due, it is sent when
the failure returns and the incident is `Open` again, not before.
Free text never changes the fingerprint, so counters and estimates are
never news.

## 4. Reminders

Reminders say `still open` when nothing else changed (`reminderEvery` in
`lifecycle.go`).

- A page (or a held page) is said again once after `PageRemindAfter` (6h),
  then every `RemindEvery` (a week).
- An incident that only the digest or a roll-up carries (digest tier, or
  `Delivery.rolledUp`) is said again every `ChronicRemindEvery` (a day): "still
  failing, for 3d now". Without it nobody would hear of it twice.
- Every other tier is said again every `RemindEvery`.

A flapping incident also gets its reminder when it is due.

## 5. Recovering and the resolve hold

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

During the restore grace after a restart (see below) a recovering incident
does not resolve on missing members, and when its findings come back in that
grace it returns to `Open` without counting a flap cycle: the restart hid the
failure, it did not stop. The same holds for a root kind kwatch cannot
observe: its returning findings count no cycle either (`recovering` tests
`verifiable` as well as `inGrace`).

## 6. Reopen and supersede

A resolved incident is reopened as the same incident (same ID, same
Slack thread) when all of these hold (`reopenable` in `reopen.go`):

- people were told: a message of its own reached someone (`Incident.CanReopen`
  reads the delivery record `sentMark.told`, not the settle time). An
  announcement that was held and dropped (resolved inside a startup summary,
  outage hold or digest window) or dropped as out of scope was never heard;
- its tier was at least `Notify`, or it is a held page (`Delivery.pageHeld`),
  or it is a digest-tier incident that a digest listed;
- it was not superseded;
- it resolved within `RepageWindow` (2h);
- the new finding's mode matches. `modes.go` keeps the set of every mode the
  incident's members ever had, and the finding must match one of them. A
  finding or incident with no mode matches any.

On reopen the tier becomes `Notify` (a digest-tier incident stays at the
digest) and a page is marked `pageHeld` (`HoldAtNotify`), so the
incident does not page again. A digest incident's `failing again` update is
not sent on its own: the next digest lists it as recurring (and drops the
resolve line it may still hold for it). It keeps the page's reminders. One `failing
again` update is sent after `ReviseSettle`, so the members that return are in
the same message. If the reopened incident recovers before that update is
due, the update is sent when the failure returns and the incident is `Open`
again (never while `Recovering`). After the window, or for another mode, the failure is a new
incident that remembers the old one. A reopen also forgets the fix attempt
of the earlier occurrence (`Attempt`, the unsent attempt and its late flag).

How paging state works now:

- `Delivery.OpenAtPagers()` (`MarkPaged` / `ClosePage`) means the alert is open at the paging and issue-tracker
  providers. Any message of the incident's own that is not a resolve (an
  announcement or an update, `scopePaging` in `pipeline/deliver.go`) sets it.
  The one exception is the `failing again` update of a reopened page
  (`failingAgainOfPage`: reason `failing again` and `Delivery.pageHeld`). It
  goes to chat only (`SkipPaging`) and does not set it. The original page
  is the one page of that outage; its resolve closed the alert, and
  `Paged` stays false after it. A later reminder or material update of the
  reopened incident, if the outage lasts, reaches the pagers as a notify
  and opens a new alert then.
- A page held in the startup summary or an outage hold is paged on its own
  when it is announced. If it was held at notify and its update later
  reaches the page tier, the hold pages it then (`risesToPage`) and marks it
  paged, so the page is not lost; an outage hold pages the namespace once.
  A listing (roll-up, outage message) never pages an incident whose alert is
  already open (`pagedAlready`).
- A quiet supersede (a resolve without a message) purges what is still held
  for that incident: the announcer's hold, the startup summary, the digest
  and the outage hold (`TakeQuietResolves` returns the IDs). A held
  announcement is never sent for an incident that was resolved quietly.
- The route of the incident (`Incident.AnnouncedRoute`: namespaces,
  finding reasons, severity) is recorded when the incident is announced and
  widened by every update that is sent (union of namespaces and reasons,
  highest severity), and persisted. Delivery routes the resolve with it
  whole after the members are gone, so a pager route that only a later
  update matched still hears the resolve, also after a restart.
- A restored held page whose alert is open at the pagers (`Paged`) is
  announced again to chat only (`Decision.PagedAlready`) and stays paged.
- A new incident on a root and mode whose alert key a live, revised incident
  still holds gets a numbered key suffix (`freeAlertKey`), so the two never
  update or resolve each other's alert.
- A resolve closes it, and the resolve goes to the pagers only when `OpenAtPagers()`
  was set. A resolve for an alert nobody opened is not sent to them.
- A page to a pager that is down is not redirected to chat. Chat already
  gets its own copy of the message, and a chat fallback cannot open or close
  an alert by key, so the pager keeps retrying the opening and the resolve
  until it takes them (`mustReachPrimary` in `delivery/delivery.go`). If the
  pager stays down, nobody is paged; the health of the provider says so.
- In a full provider queue a resolve takes the place of the newest routine
  job, never of another resolve or of a page announcement. A resolve that is
  dead-lettered, expires or is dropped is logged at error with its alert key
  and counted in `kwatch_delivery_resolves_lost_total`: its alert may stay
  open. A resolve sent to an issue tracker that has no issue mapped for the
  key logs a warning with the key.
- Delivery marks an incident paged when the message is handed over; if every
  pager then gives up on its messages for good, delivery tells the pipeline
  (`PageObserver`) and the flag is cleared, so a later resolve is not sent
  to a pager that never held the alert. A job still queued or retrying keeps
  the flag.
- A record restored from an older version that was announced counts as paged,
  because sending a resolve is the safe mistake.
- `isPage` (page tier or `pageHeld`) and `reachedPaging` (`isPage` or
  `OpenAtPagers()`) in `flags.go` keep a page's resolve from being dropped or demoted.

Slack and the reopen thread. A resolve of an incident that may reopen
(`Incident.CanReopen`: announced, notify tier or held page, not superseded)
carries `Message.ReopenWithin` (`RepageWindow`). The Slack provider keeps
the conversation after that resolve until the window has passed, instead of
forgetting it. A `failing again` update inside the window is a reply in the
old thread, and the root message is edited back from the resolved marker to
the failing one; the next resolve edits it to resolved again. After the
window the conversation is dropped (a later failure is a new incident with
a new root), and the kept thread, with its end time, is saved with the other
threads, so a restart does not break it. The kept threads count toward the
same size bound as the open ones. A resolve that cannot reopen, or a
roll-up member's, is forgotten at once.

When a cause revision moves every member of an announced incident to another
announced incident, the old one is resolved as `superseded`. If its failures
are the same ones people were just told about and it never paged, that
resolve is not sent.

## The timings

All durations live in `internal/incident/timings.go` (and, for the pipeline,
`internal/pipeline/announce/timings.go` and `internal/pipeline/coverage/watch.go`);
read the header there for how they relate.

| Name | Value | Why |
| --- | --- | --- |
| `DefaultSettle` / `DefaultPageSettle` | 75s / 15s | one message per failure; pages are urgent |
| `DefaultReviseSettle` | 30s | what follows a new cause arrives together |
| `BurstIncidents` | 3 | this many announceable incidents settling together wait the full settle |
| `DefaultHold` / `DefaultMaxHold` | 3m / 30m | healthy long enough to trust; cap of the doubling |
| `StillBrokenMax` | 2h | the extra hold of a workload still short of replicas ends here |
| `DefaultFlapWindow`, `DefaultFlapCycles` | 30m, 3 | how far back recoveries count as flaps; how many make a flapper |
| `ChronicWindow`, `ChronicOccurrences`, `ChronicFactor` | 1h, 2, x4 | a fast flapper must not say "healthy" and "failing" in turn |
| `RepageWindow` | 2h | the same outage must not page twice, and reopens, not restarts |
| `PageRemindAfter` | 6h | a long outage does not go quiet |
| `RemindEvery` / `ChronicRemindEvery` | 7d / 24h | still-open reminders |
| `MaterialGap` | 10m | at most one material update per 10 minutes |
| `FixWatch` | 10m | when to say a fix attempt did not work |
| `HPAStuckAfter` | 30m | an autoscaler at its maximum this long is out of headroom |
| `RecentWindow` | 30m | how far back "what changed" looks |
| `DigestReportEvery` | 24h | at least one digest a day lists a periodic recurrence |
| `DefaultRemember` | 7d | resolved incidents are kept this long ("3rd time this week") |
| `KnownAfter`, `RhythmWindow` | 24h | a problem heard for a day waits for the digest |
| `routineWindow`, `recurrenceWeek` | 45m, 7d | same time of day counts as a routine; window of "the third time this week" |
| pipeline `restoreGrace` | 10m | restored incidents wait this long for findings to return |
| announce `StartupWindow` | 2m | cold-start announcements are collected into one summary |
| announce `outageHoldMax` (`announce/timings.go`) | 30s | a namespace outage waits this long for its siblings |
| announce `outageWindow` | 10m | incidents opened this close are one outage |
| announce `outageIncidents` / `outageMinShare` | 5 / 3 | incidents that make an outage alone / when at least half the namespace |
| coverage `Every` / `After` / `Retry` | 5m / 15m / 30m | coverage check cadence, wait before a workload is believed lost, quiet time after a hand-back |
| explain `SignatureWindow` / `SignatureMinWorkloads` | 30m / 3 | failures sharing one error began this close; this many workloads |
| `kube.BootWindow` | 10m | a node pool still booting is not an incident; `coverage.After` is longer |

## Example: a crash loop

1. 10:00:00 a pod of `api` starts crash-looping; a finding opens an
   incident (`Settling`).
2. 10:01:15 the settle is over: `Announce`, notify tier, one message.
3. 10:03 a second pod fails and the crash-loop stage is reached: the
   fingerprint changed, so `Update` (`material change`).
4. 10:08 someone edits the Deployment: `Update` (`fix attempt`).
5. 10:12 the pods are ready. The incident is `Recovering`; the hold is 3m.
6. 10:15 still healthy: `Resolve` (`healthy for 3m0s`), crediting the fix.

## Example: a flapping webhook

A validating webhook fails every ~13 minutes.

1. First failure: announced after the settle, at page tier if the page rule
   for rejecting webhooks matches.
2. It recovers and the hold (3m) passes: `Resolve`.
3. It fails again 10 minutes later, inside `RepageWindow`: the same incident
   reopens at notify (a held page), one `failing again` update.
4. Each recovery now counts as a cycle; the hold doubles, and as the webhook
   reopened within `ChronicWindow` it is multiplied by `ChronicFactor`, so
   the hold outlasts the gap between failures and "healthy again" stops
   alternating with "failing again".
5. After `FlapCycles` (3) cycles it is `Flapping`: one note, then silence
   unless it grows, until it is stable for `MaxHold` and resolves.

## How a crash storm with one shared error becomes one incident

Ten workloads crash with the same line, `license key rejected for tenant
4812`, each in its own pod.

1. Where the line comes from. The container's termination message is used
   first (`AttrLastMessage`). If it is empty, the crash-log round
   (`inventory/kube/crash_log_round.go`, every 30s, at most 20 reads) reads
   the previous run's log and stores the first error line that is not a stack
   frame as `AttrLastErrorLine` (`FirstErrorLine`).
2. `detectors/container.go` puts that text in the finding as `error`
   evidence, quoted as it is.
3. `explain` reads only that `error` evidence (`signatureOf`). It runs
   `format.Signature`, which lowercases and replaces times and IDs with
   placeholders. A word made only of hex letters and digits becomes `#`;
   any other word keeps its letters and only its digit runs become `#`
   (`worker7` reads `worker#`). IP literals become `<ip>` and keep a
   service port below 32768; a named host keeps its service port. A
   three-digit HTTP status after `http` or `status` is kept. The line is
   kept whole, so a leading `ERROR` stays, as `error`. Every replica and
   workload gives `error license key rejected for tenant #`.
4. `normalizeSignature` drops a signature that is on the generic list
   (`error`, `panic`, `exit status #` and so on), shorter than 16
   characters, under 3 words, or made only of generic network words. It cuts
   the rest to 80 characters.
5. The text, with the failing mode family in front (`signatureName`), names a
   virtual `failure-signature` entity. Each failing pod is linked to it
   (`LinkSharesError`).
6. The `shared-failure-signature` row proposes that entity as the cause once
   `SignatureMinWorkloads` (3) workloads that began failing within
   `SignatureWindow` (30m) share it (`applySignatureWindow`). It is the
   weakest row (prior 0.55), so a named endpoint or a better cause wins.
   The incident manager then attaches every finding to the incident rooted
   at the signature. The per-workload incidents that were settling are
   replaced by it, and one message says the shared error is the cause. The
   full settle (75s) helps: in a burst even a page waits for it.
7. The message (`compose/calls.go:signatureLead`, `compose/proof.go`) says
   `api in shop and nine other workloads keep crashing with the same
   error.` and then quotes one pod's own line: `It fails with "ERROR license
   key rejected for tenant 4812".` The quoted tenant number is that pod's,
   not the normalised `#`. With no other workload it says `api in shop
   keeps crashing with an error other workloads share`.

Worked example, from the first crash to the message:

```text
10:00:00  pod api-1 exits; log line "ERROR license key rejected for tenant 4812"
10:00:30  crash-log round stores it as last.error.line
10:00:31  finding "error" evidence; signature "CrashLoop error license key rejected for tenant #"
10:01:15  api-1 alone has settled and is announced as its own incident
10:03:00  api-2 and api-3 fail with tenant 5190 and 7731: same signature, 3 workloads
10:03:00  shared-failure-signature row proposes the signature entity as the cause
10:04:15  settle ends (75s): one announce, rooted at the signature; api-1's
          incident is re-rooted under it with the two new ones
```

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
- **Restart.** Incidents are saved and restored. For `restoreGrace` (10m) a
  restored incident without members does not recover, because detectors
  have not re-raised its findings yet (`grace.go`). A restart announces no
  incident, so when the grace is over one summary (`ListRestored` in
  `pipeline/announce/startup.go`) names the restored incidents that still fail
  and have not spoken for themselves since the restart. On a
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
