# Repository documentation

The polished, user-facing guides live on [kwatch.dev](https://kwatch.dev/docs).
This directory contains documentation that should stay versioned with the
source tree: architecture, release behavior, security details, and offline
references for operators and contributors.

## Start here

- [Project README](../README.md) — product overview and quick installation.
- [`kwatch.sh` manager](./kwatch-sh.md) — installer and day-to-day manager.
- [Configuration reference](./configuration.md) — complete versioned settings
  reference.
- [Provider reference](./providers.md) — provider fields and examples.
- [Kubernetes coverage](./kubernetes-coverage.md) — watched resources and
  capabilities.

## Technical and operational reference

- [Architecture](./architecture.md) — runtime design and package boundaries.
- [Release integrity](./release-integrity.md) — image, manifest, and chart
  verification.
- [Licensing](./licensing.md) — project and dependency licensing.
- [Third-party notices](./third-party-notices.md) — bundled notices.
- [Trademarks](./trademarks.md) — name and logo usage.

## Which documentation should I change?

| Change | Primary location |
| --- | --- |
| Installation, onboarding, or troubleshooting | `kwatch.dev/docs` |
| A setting, provider, or runtime behavior | This repository and the site |
| Package design or contributor workflow | This repository |
| Release steps or security policy | This repository |

When a setting or behavior changes, update the versioned repository reference
first, then keep the matching site guide concise and user-oriented. Avoid
copying an entire reference page into both locations.
