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
- [Real-cluster scenarios](../test/e2e/README.md) — regression scenarios and
  the contributor workflow.
- [Configuration catalog](../deploy/config-catalog.tsv) — generated settings
  metadata used by the installer and documentation pipeline.
- [Provider catalog](../deploy/provider-catalog.tsv) — generated provider
  metadata used by the installer and documentation pipeline.
- [Feature catalog](../deploy/feature-catalog.tsv) — generated capability
  metadata and dependency information.

## Technical and operational reference

- [How kwatch works](./architecture.md) — the pipeline, the four concepts,
  one incident from crash to message, watch modes, storage, endpoints and the
  single-replica Lease lock; the published architecture is maintained on
  `kwatch.dev`.
- [Contributor architecture guide](./contributor-architecture.md) — the
  package table, import direction rules and where new code belongs.
- Contributor guides, each five steps with a real example and test command:
  [add a detector](./contributing-detector.md),
  [add a propagation rule](./contributing-propagation-rule.md),
  [add a source or kind schema](./contributing-source.md),
  [add a message fact](./contributing-message-fact.md),
  [add a labelled scenario](./contributing-scenario.md).
- [Production operations](./production-operations.md) — readiness, restarts,
  recovery, outage handling, and release operations.
- [Configuration notes](./configuration.md) — offline configuration and
  endpoint notes; the published reference remains on `kwatch.dev`.
- [Provider notes](./providers.md) — offline provider behavior and transport
  notes; the published reference remains on `kwatch.dev`.
- [Kubernetes coverage](./kubernetes-coverage.md) — monitored resources and
  graceful degradation boundaries.
- [Permission matrix](./feature-permissions.md) — feature-to-RBAC guidance.
- [Production goals](./production-goals.md) — the quality, scale,
  reliability and security bar kwatch must meet.
- [Architecture ADRs](./adr/) — decisions behind runtime boundaries. ADR 0011
  (health, propagation and explanation) supersedes the reasoning, storage and
  message sections of ADR 0010; ADR 0010 supersedes ADRs 0002, 0004, 0006 and
  0007 and part of 0001; ADRs 0003, 0005, 0008 and 0009 remain in force.
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
