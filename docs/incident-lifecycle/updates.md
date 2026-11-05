# Updates

Part of [The life of an incident](../incident-lifecycle.md).

Updates come from four places (the same list is at the top of
`open_steps.go`):

1. `openSteps` (`open_steps.go`): the news of an `Open` incident, below.
2. `reopenUpdate` (`reopen.go`, called from `advance`): the `failing again`
   update of a reopened incident, before the steps.
3. The `recovering` and `flapping` handlers (`lifecycle.go`): the `flapping`
   update when recoveries reach `FlapCycles`, and what a flapping incident
   says (`flappingNews`). A flapping workload whose readiness check fails
   or whose Service has no ready endpoint is said to be "not serving" first
   (`notServingState`, compose); the state stays `Flapping`.
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
   The cause in the fingerprint (`causePart`, `cause_told.go`) is the one
   last told unless the cause now is new for the incident and has held for
   `CauseHold` (30s) over at least two evaluations. The first cause found
   while none was told is news at once (it is no flip). A cause told before
   is never news again by itself: a solver that blames a release, then a
   startup setting, and back, for the same failures sends nothing. No
   cause is the exception: it is never "told before", since what explains
   the incident then lives elsewhere (a called backend, say) and may have
   changed; it is news once it holds. Every message records the cause it
   shows, held or not, so a cause that changes with other news goes out
   with that news and is not sent again 30 s later. Up to 8 told causes
   are kept per incident (hashes, saved with it). Changes in the affected
   count, the confidence or the evidence checked never enter the
   fingerprint. A repeated Warning event on the root (`UnusualEvent.*`)
   does not enter it while the root fails on its own (a condition not
   ready, say): the repeat only confirms the failure, so neither it nor
   its window closing sends an update (`eventRestated`,
   `fingerprint.go`). Likewise pods that run but never get ready
   (`WorkloadNeverReady`) are no news once the incident said they are not
   ready (no endpoints, a failing readiness probe, containers not ready),
   unless the workload never became healthy since it was created.

**Falling to the digest** (`reassess.go`, called from `advance`). An
announced incident keeps the loudest tier it reached, so crash-loop members
that come and go never flip it. One change is not churn: every finding the
tier stood on is gone and only digest findings are left, as when a restart
brings an old `FailedGetScale` back as the clearer `HPATargetMissing`. A
`Notify` incident then falls to `Digest` (after the restore grace, and not
within `MaterialGap` of its last update) and owes its thread one `cause
revised` update, sent through step 2 above. A page falls the same way once
every failure that paged has cleared and only digest findings are left (a
pressure stall that outlived a crash loop): nothing new is paged, its open
alert closes with the resolve, and an idle webhook page keeps its own rule.
A held page never falls. The update and the resolve of such an
incident carry `Decision.Thread` and go to the thread people were told in
(`threadNews` in `digest.go`); reminders and other updates ride in the
digest, which lists the still-open incident as an ongoing item
(`announce/ongoing.go`). A real failure that returns raises the tier again,
told by the usual material change. A user severity override keeps its tier.

**A restart does not flip the tier.** While a restored incident's findings
come back inside the restore grace, its tier can move with the partial
view: a target not listed yet reads as missing, a member back before its
sibling reads alone. Such moves are neither logged as "incident tier
changed" nor recorded as journal transitions (`tierUnsettled`); where the
tier stands once the grace is over is. A detector that words the same
failure differently once every source is listed is not noted as a
recovery either.

**Cause swings are damped.** A finding whose cause returns to the root it
left less than `flipDamping` (2 minutes) ago stays where it is: the
solver swinging between two candidates for the same failure is not a
revision. A return to the finding's own object, or to a root that now
fails by itself, is. A finding that becomes its own root writes no
"X is now explained by X" note.

**A restart does not swallow a rise.** A restored incident adopts its
fingerprint during the restore grace so that an upgrade says nothing, but a
tier that rose since the record was saved (a page above all) stays news. The
adoption keeps the tier people last heard: the saved one (`restoredTier`)
until this run decides an update, then the tier of that update (`toldTier`).
A rise told inside the grace is therefore told once, not again one
`MaterialGap` later. The update says "It is now critical." (page) or "It is
now a warning." (notify) (`raisedSentences`).

**A partial view is not a rise.** Right after a restart some kinds have
not synced, and a detector may describe a failure in poorer words: an
autoscaler whose target Deployment is not listed yet shows the event
`FailedGetScale` (notify) instead of `HPATargetMissing` (digest). The
record saves the reasons of its failing members (`Record.Reasons`; an
older record falls back to `RootReasons`). Until all of them are back
inside the grace, the incident is provisional (`restore_reasons.go`): its
tier follows the members, never below the tier people were told, and a
material change is held. When the saved reasons return, the tier ratchets
again from what they give, and a rise on top of them is told at once, a
page included. A rise that still holds when the grace ends is told then
(`provisionalDeadline` wakes it). A page waiting only on other findings is
held too: before the saved reasons are back it rests on the same partial
view, and the hold lasts at most the grace. How a restored incident recovers is in
[Recovering and the resolve hold](./resolve.md#the-restore-grace).

**A withdrawn cause says nothing.** A `cause revised` that takes a known
cause away and finds none (`causeWithdrawn`, `open_steps.go`) is not sent
for a notify or lower incident: the evidence of the cause (an admission
error event, say) aged out while the failure went on, and nothing
observed contradicts what the thread says. The fingerprint adopts the new
root so the move does not come back as a material change, and a different
cause, a rise to page, or the resolve still speak. A page always sends
it.

**A tie with a listed cause says nothing.** An incident that only a
listing (a roll-up, the startup summary) announced, and that has sent
nothing of its own since, may be revised to a new cause whose rival is
the cause the listing stated (`hedgesTold`, `open_steps.go`). The update
would only say "two possible causes: X or what you were told", as the
incident's first message of its own: a new conversation, and later a
resolve of its own, for a solver that re-ranked two answers within the
margin (the told one often still scoring higher). Nothing observed
contradicts the listing, so for a notify or lower incident it is not
sent; the incident still moves to the new root and the fingerprint
adopts it, as for a withdrawn cause. It stays a listed item, so its
resolve can join the grouped "Recovered (or Deleted) at the same moment"
message. An incident with a thread of its own still hears the tie
there, and a page always sends it. The told roots are the incident's
root and its cause's root when the first revision since the last
message was owed (`Pending.KeepTold`), so a second move before the
update goes out does not make an unsent root look told. They are kept
in memory only: after a restart an owed revision is sent.

A reopened incident's `failing again` update comes before all of these. If
the reopened incident recovered before the update was due, it is sent when
the failure returns and the incident is `Open` again, not before.
Free text never changes the fingerprint, so counters and estimates are
never news.

---

Previous: [Announce](./announce.md) | Next: [Reminders](./reminders.md)
