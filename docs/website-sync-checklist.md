# Website synchronization checklist

The documentation site in `/Users/macos/kwatch.dev` is a separate repository.
Review and commit its changes independently; never copy over unrelated dirty
work. Use this checklist when synchronizing code-derived behavior from Kwatch.

## Review subjects

- Architecture and package ownership.
- One-leader, standby replicas, Lease failover, and single-replica limits.
- Adding monitors, providers, filters, and RCA behavior.
- Incident lifecycle, grouping, recovery, and monitoring gaps.
- Persistence migration, backup, restore, and rollback.
- Health, readiness, liveness, diagnostics, and watcher operations.
- Provider outage recovery, queue limits, and delivery semantics.
- RBAC, deployment security, topology, and resource sizing.
- Release checksums, signatures, provenance, SBOMs, upgrades, and rollback.

## Synchronization procedure

1. Inspect the website worktree and preserve unrelated changes.
2. Compare code-owned catalogs, manifests, and operations guidance with the
   website reference pages.
3. Update the appropriate tutorial, how-to, reference, explanation, or
   operations page rather than duplicating a second source of truth.
4. Verify links, generated provider/configuration references, and versioned
   release notes in the website repository.
5. Run that repository's documentation and build checks.
6. Commit the website changes separately and record the code commit they mirror.

The code repository does not claim website synchronization is complete until
that independent review and commit have happened.
