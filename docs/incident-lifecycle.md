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

## The stages, in order

Each page covers one stage; read them in this order.

1. [Finding and settling](./incident-lifecycle/settle.md): Findings become
   an incident, and the settle window that decides when it is real.
2. [Announce](./incident-lifecycle/announce.md): The first message: who is
   paged, how the workload's own normal is used, and progressing rollouts.
3. [Updates](./incident-lifecycle/updates.md): What an open incident says
   next, in the order the steps are tried.
4. [Reminders](./incident-lifecycle/reminders.md): When a still-open
   incident repeats itself.
5. [Recovering and the resolve hold](./incident-lifecycle/resolve.md): How
   an incident recovers and why it waits before it resolves.
6. [Reopen and supersede](./incident-lifecycle/reopen.md): What happens when
   a resolved incident fails again, or is replaced.
7. [The timings](./incident-lifecycle/timings.md): Every duration, each with
   its reason.
8. [Examples](./incident-lifecycle/examples.md): A crash loop, a flapping
   webhook, and a crash storm becoming one incident.
9. [What shapes what people hear](./incident-lifecycle/shaping.md): The
   coverage check, other things that shape messages, and where the flags
   live.
