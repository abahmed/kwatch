# Repository documentation

The canonical source for published user, operator, architecture, and
contributor documentation is the [`kwatch.dev` repository](https://github.com/abahmed/kwatch.dev)
and its [documentation site](https://kwatch.dev/docs). This repository owns
the code-derived generators and the rules that keep generated references in
sync with runtime behavior.

Do not create a second manually maintained copy of public configuration,
feature, provider, CLI, RBAC, metrics, health, or architecture documentation
here. Root-level repository entry points may link to the website, and
implementation-only notes may remain beside code when they are not published.

## Start here

- [Project README](../README.md) — product overview and quick installation.
- [`kwatch.sh` manager](./kwatch-sh.md) — installer and day-to-day manager.
- [Configuration catalog](../deploy/config-catalog.tsv) — generated settings
  metadata used by the installer and documentation pipeline.
- [Provider catalog](../deploy/provider-catalog.tsv) — generated provider
  metadata used by the installer and documentation pipeline.
- [Feature catalog](../deploy/feature-catalog.tsv) — generated capability
  metadata and dependency information.

## Technical and operational reference

- [Architecture notes](./architecture.md) — repository-local architecture
  notes; the published architecture is maintained on `kwatch.dev`.
- [Contributor architecture guide](./contributor-architecture.md) — package
  ownership and extension workflows for source-tree contributors.
- [Architecture ADRs](./adr/) — accepted decisions behind runtime boundaries
  and compatibility seams.
- [Release integrity](./release-integrity.md) — image, manifest, and chart
  verification.
- [Licensing](./licensing.md) — project and dependency licensing.
- [Third-party notices](./third-party-notices.md) — bundled notices.
- [Trademarks](./trademarks.md) — name and logo usage.

## Which documentation should I change?

| Change | Primary location |
| --- | --- |
| Installation, onboarding, troubleshooting, or operations | `kwatch.dev/docs` |
| A setting, provider, feature, or runtime behavior | Code plus generated site reference |
| Published architecture or contributor workflow | `kwatch.dev/docs` |
| Agent rules and code-local workflow | `AGENTS.md` and root `CONTRIBUTING.md` |
| Release integrity and source-tree legal references | This repository |

When a setting or behavior changes, update the code and its tests first, then
regenerate the site reference and update the relevant website tutorial,
how-to, explanation, or runbook. Avoid copying an entire reference page into
both repositories.
