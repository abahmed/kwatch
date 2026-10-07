# Updates

Part of [The life of an incident](../incident-lifecycle.md).

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

**Falling to the digest** (`reassess.go`, called from `advance`). An
announced incident keeps the loudest tier it reached, so crash-loop members
that come and go never flip it. One change is not churn: every finding the
tier stood on is gone and only digest findings are left, as when a restart
brings an old `FailedGetScale` back as the clearer `HPATargetMissing`. A
`Notify` incident then falls to `Digest` (after the restore grace, and not
within `MaterialGap` of its last update) and owes its thread one `cause
revised` update, sent through step 2 above. A page never falls: it is not
un-paged and nothing new is paged. The update and the resolve of such an
incident carry `Decision.Thread` and go to the thread people were told in
(`threadNews` in `digest.go`); reminders and other updates ride in the
digest, which lists the still-open incident as an ongoing item
(`announce/ongoing.go`). A real failure that returns raises the tier again,
told by the usual material change. A user severity override keeps its tier.

A reopened incident's `failing again` update comes before all of these. If
the reopened incident recovered before the update was due, it is sent when
the failure returns and the incident is `Open` again, not before.
Free text never changes the fingerprint, so counters and estimates are
never news.

---

Previous: [Announce](./announce.md) | Next: [Reminders](./reminders.md)
