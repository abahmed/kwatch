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

---

Previous: [Updates](./updates.md) | Next: [Recovering and the resolve hold](./resolve.md)
