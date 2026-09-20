# ADR 0007: Structured persistence migration reports

## Status

Accepted

## Decision

Persistence records every migration operation in a startup-cycle
`MigrationReport`; there is no last-result compatibility accessor. Reports
contain source and destination formats, bounded status, recoverability,
continuation safety, and safe operator context.

Missing schema metadata follows the legacy path. Supported older versions are
migrated. Current versions are left unchanged. Unsupported future versions and
malformed metadata are preserved and reported; they are never silently
overwritten.

## Consequences

Startup and health diagnostics can show the complete migration outcome rather
than only the last operation. Detailed storage errors remain in logs and are
not exposed through public diagnostic responses.
