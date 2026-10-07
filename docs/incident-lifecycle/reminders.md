# Reminders

Part of [The life of an incident](../incident-lifecycle.md).

Reminders say `still open` when nothing else changed (`reminderEvery` in
`lifecycle.go`).

- A page (or a held page) is said again once after `PageRemindAfter` (6h),
  then every `RemindEvery` (a week).
- An incident that only the digest or a roll-up carries (digest tier, or
  `Delivery.rolledUp`) is said again every `ChronicRemindEvery` (a day): "still
  failing, for 3d now". Without it nobody would hear of it twice.
- Every other tier is said again every `RemindEvery`.

A flapping incident also gets its reminder when it is due.

## Ongoing digest problems

A digest lists a problem once, and the daily reminder above comes only
when the incident stays quiet. A digest-tier incident that a digest listed
and that is still open (also flapping or recovering) and not acknowledged
is therefore named again in every digest that goes out, as an ongoing item
in the Problems list: `Service web (shop) — still failing:
FailedDeployModel ×14 since 17:14`. The count is the Kubernetes event
count since the incident opened, and is left out when kwatch has none. It
is capped like the rest of the list (five, then `+N more still failing`).
An ongoing problem that came back since its last listing also opens a
digest window on its own, but no sooner than `OngoingEvery` (6h) after
the last listing, so a quiet cluster still hears about a problem that
never goes away (`announce/ongoing.go`).

## Acknowledged incidents

A person acknowledges an incident by annotating its root, or one of its
members, with `kwatch.io/ack` (see `ack.go`):

```text
kubectl annotate deploy/api kwatch.io/ack="looking into it"
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
  the old one. If the annotation is still on the object when it is
  announced, the announcement says `The ack annotation is still present on
  deployment/api, so I won't remind`, and reminders are held for it too.
- The note is a person's text: kwatch reads it as one line, removes
  credentials, bounds it to 200 bytes and only ever quotes it.
- `Incident.Ack` is persisted, so a restart does not tell it again. While a
  restored incident is inside the restore grace the model is still being
  rebuilt, so a missing annotation is not read as removed.

---

Previous: [Updates](./updates.md) | Next: [Recovering and the resolve hold](./resolve.md)
