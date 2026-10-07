# The timings

Part of [The life of an incident](../incident-lifecycle.md).

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
| `kube.WakeQuiet` / `WakeMax` | 10m / 30m | a wake-up ends this long after its last start, or at most this long after its first |
| `kube.ScaleDownSpan` | 15m | workloads set to 0 this close together are one planned scale-down |
| announce `OngoingEvery` | 6h | an open digest problem that came back is listed again after this long, even when no other digest is due |

---

Previous: [Reopen and supersede](./reopen.md) | Next: [Examples](./examples.md)
