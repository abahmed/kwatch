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

During the restore grace after a restart (see below) a recovering incident
does not resolve on missing members, and when its findings come back in that
grace it returns to `Open` without counting a flap cycle: the restart hid the
failure, it did not stop. The same holds for a root kind kwatch cannot
observe: its returning findings count no cycle either (`recovering` tests
`verifiable` as well as `inGrace`).

---

Previous: [Reminders](./reminders.md) | Next: [Reopen and supersede](./reopen.md)
