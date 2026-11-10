# Reminders

Part of [The life of an incident](../incident-lifecycle.md).

Reminders say `still open` when nothing else changed (`reminderEvery` in
`lifecycle.go`).

- A page (or a held page) is said again once after `PageRemindAfter` (6h),
  then every `RemindEvery` (a week).
- An incident that only the digest or a roll-up carries (digest tier, or
  `Delivery.rolledUp`) is said again after `ChronicRemindEvery` (a day), then
  3 days later, then a week later, and weekly from then on ("still failing,
  for 3d now"); `chronicBackoff` in `reminder_backoff.go`. Any other message
  of the incident (a rise, a new cause, new objects, a reopening) starts the
  gaps over at a day. The count is saved with the incident, so a restart
  keeps the back-off. Without a reminder nobody would hear of it twice, but a
  low problem that changes nothing does not need one every day.
- Every other tier is said again every `RemindEvery`.

A flapping incident also gets its reminder when it is due.

## Ongoing digest problems

A digest lists a problem once, and the reminder above comes only
when the incident stays quiet. A digest-tier incident that a digest listed
and that is still failing and not acknowledged (open, or flapping with a
failure right now; one that recovers, or flaps and is healthy at the
moment, is left out, because "still failing" would be false:
`stillFailing` in `announce/ongoing.go`)
is therefore named again in the digests that go out, as an ongoing item
in the Problems list: `Service web (shop) is still failing, for two days
now.` Each ongoing item is listed in full at most once a day (UTC): in the
first digest of the day, or when it changed (flapping, its reason, how many
things fail, its cause). Other digests carry one line for the rest: `3 more
still failing, unchanged (web, db, cache +1)`. The audit entry of a digest
names every ongoing item. The list is capped like the rest (five, then
`+N more still failing`; `announce/ongoing_compact.go`).

An idle webhook (a fail-closed webhook whose backend is gone and that
refused nothing) is announced once, falls to the digest quietly and is not
reminded: its daily "still failing" is the digest line.
An ongoing problem that came back since its last listing also opens a
digest window on its own, but no sooner than `OngoingEvery` (6h) after
the last listing, so a quiet cluster still hears about a problem that
never goes away (`announce/ongoing.go`).

## Acknowledged incidents

A person acknowledges an incident by annotating its root, or one of its
members, with `kwatch.dev/ack` (see `ack.go`):

```text
kubectl annotate deploy/api kwatch.dev/ack="looking into it"
```

- While the annotation is there, `reminderDue` is false, so the incident
  neither reminds nor wakes the tick for a reminder. That covers the page
  reminder too: the page already happened. Everything that is news still
  goes out: a worse state, a changed cause, the resolve.
- `ackStep` (step 4 of `openSteps`, and `flappingNews`) tells the thread
  once, `Acknowledged on deployment/api: "looking into it"`, and, when the
  annotation is removed, once, `Acknowledgement removed`. Reminders then
  count from the removal. It waits while a material change is not yet
  told, so it never swallows news. kwatch does not name who set the
  annotation: an object records the tool that wrote it last, not a person.
- Editing the note changes nothing in the thread; only the appearance and
  the removal are told.
- A reopen inside `RepageWindow` is the same incident, so the
  acknowledgement still applies and is not told again; its one `failing
  again` update still goes out, because the resolve ended the thread. If
  the annotation was removed meanwhile, the removal is told after it.
- A new incident (after the window, or another mode) is not pre-acked by
  the old one. An annotation that was on the object before the incident
  opened is ignored for it (`staleAck` in `ack_stale.go`): no mention in
  the announcement, and it reminds, releases a held page and pages a
  flapper as usual. "Before" is the `Since` of the model's ack attribute,
  when kwatch first saw the annotation with its present note, so changing
  the note, or removing the annotation and setting it again, acknowledges
  the new incident. A restart rebuilds the model and would make every
  annotation look new, so the incident keeps the objects whose annotation
  it ignored (`AckIgnored` in the record) and goes on ignoring them until
  the annotation changes after the restore grace.
- An annotation written while the incident still settles is about it: the
  announcement says `It is already acknowledged (annotation on
  deployment/api)`, no separate update follows, and reminders are held.
- The note is a person's text: kwatch reads it as one line, removes
  credentials, bounds it to 200 bytes and only ever quotes it.
- `Incident.Ack` is persisted, so a restart does not tell it again. While a
  restored incident is inside the restore grace the model is still being
  rebuilt, so a missing annotation is not read as removed.
- For the dashboard the acknowledgement also records `Since`, when kwatch
  saw it appear or its note change, and `By`, the field manager that wrote
  the annotation (`kubectl-annotate`). The informer transform reads it from
  the managed fields before it trims them (the latest manager whose field
  set holds `f:kwatch.dev/ack`) and stamps it on the cached object, and the
  translator turns it into the `ack.by` attribute (`kube.AttrAckBy`,
  `ack_manager.go`). It is an attribute, not a change: an edit of an
  annotation is not a change to the object, and the change history feeds
  the root-cause reasoning, fix attempts and resolves. `By` is empty when
  the managed fields name no writer. Neither is told to the thread, and
  older records restore without them.

---

Previous: [Updates](./updates.md) | Next: [Recovering and the resolve hold](./resolve.md)
