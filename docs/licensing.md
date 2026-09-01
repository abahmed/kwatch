# Licensing

## Current releases

Current releases are distributed under the Elastic License 2.0 in
[`LICENSE`](../LICENSE). It allows internal personal and commercial use,
including production use, while protecting the paid feature gates and
restricting hosted or managed-service redistribution.

## Historical releases

Kwatch releases published before this change remain available under the MIT
License. Those permissions are not revoked by a later license change; the
historical text is preserved in [`LICENSE-MIT`](../LICENSE-MIT).

## Product model

Kwatch uses one source-visible image and one feature-entitlement system:

- Community capabilities may be used inside a personal or commercial organization,
  including in production.
- Pro capabilities require a valid signed entitlement. A trial entitlement may
  activate Pro for a limited evaluation period.
- The entitlement can enable individual capabilities; it is not a replacement for
  the operator's configuration.
- The runtime does not need to contact a license server. License validation is
  local and expiry falls back safely to Community behavior.
- Users must not bypass protected license checks, remove required notices, provide
  Kwatch itself as a hosted or managed service, or use the Kwatch name to imply an
  unofficial distribution is official.

This is source-available software, not OSI-approved Open Source software. The
Elastic License 2.0 is not an OSI-approved Open Source license. Third-party
dependencies retain their own licenses and are listed in
[`third-party-notices.md`](./third-party-notices.md).
