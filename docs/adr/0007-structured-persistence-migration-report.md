# ADR 0007: Structured persistence migration reports

## Status

Superseded by ADR 0010 (2026-09-28).

The ConfigMap persistence manager, incident shards and `MigrationReport` were
removed together with `internal/persistence`. The bbolt store has a schema
version, and a file with another version is deleted and recreated: there is no
migration and no backup, and the reset is counted and shown on `/health`
(`storage_reset`). The principle that unsupported or malformed data is preserved and
reported instead of overwritten is retained.

## Decision

Persistence records every migration operation in a startup-cycle
`MigrationReport`; there is no last-result compatibility accessor. Reports
contain source and destination formats, bounded status, recoverability,
continuation safety, and safe operator context.

Missing schema metadata follows the legacy path. Supported older versions are
migrated. Current versions are left unchanged. Unsupported future versions and
malformed metadata are preserved and reported; they are never silently
overwritten.

Incident shard format v4 changes oversized incident storage: shards use a
checksum-based generation name and the manifest is the publication point.
Readers continue to accept format v3 fixed shard names. A rollback to a
pre-v4 binary requires a backup or reset of a v4 sharded incident snapshot.

## Consequences

Startup and health diagnostics can show the complete migration outcome rather
than only the last operation. Detailed storage errors remain in logs and are
not exposed through public diagnostic responses.
