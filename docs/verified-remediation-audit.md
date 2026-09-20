# Verified remediation audit

This note records the implementation baseline for the verified remediation pass.

## Confirmed fixes

- Mutable persistence, incident, group, baseline, feedback, and engine restore
  is deferred until the process owns a leadership epoch; standby Pods only
  construct immutable dependencies and serve health/election.
- Required persistence writers are registered explicitly for each active runtime
  instead of relying on a hard-coded writer count.
- Controller namespace-selector resolution receives the application lifecycle
  context in production composition.
- Migration results identify their operation phase (`restore`, `migrate`,
  `recover`, `write`, or `initialize`) so restore and migration entries cannot
  be confused.
- Saver final writes use bounded contexts detached from canceled component
  contexts and remain fenced by the persistence gate.
- Change-history saves treat an empty snapshot as a successful no-op.
- Incident notifications remain queued while delivery generations are being
  replaced.
- Provider payload policy and fallback lookup use stable catalog identities.
- Non-positive payload budgets cannot bypass direct truncation helpers.
- Status watcher shutdown paths use bounded contexts instead of unbounded
  background waits.
- Delivery reconfiguration is handled by a loop rather than recursive runtime
  calls.

## Preserved behavior

Leader callback state, dynamic watcher generations, provider fallback-cycle
handling, migration reporting, provider response handling, and incident
algorithms were not redesigned without a failing regression test. Persisted
formats, provider identities, aliases, metrics, audit reasons, and external
configuration remain compatibility boundaries.

## External validation

Kind, Docker, kubectl, live provider outage tests, RBAC `can-i`, load tests,
rolling upgrade, rollback, and real SMTP validation remain CI or operational
environment checks. They must not be reported as locally passed when the tools
are unavailable.
