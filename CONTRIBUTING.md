# 🤝 Contributing to kwatch

Thank you for helping make Kubernetes easier to operate. Code, tests,
documentation, bug reports, and ideas are all welcome.

## 🌱 Choose a way to help

- 🐛 [Report a bug](https://github.com/abahmed/kwatch/issues)
- 💡 [Suggest an improvement](https://github.com/abahmed/kwatch/issues)
- 📚 Improve a guide or example
- 🧪 Add or improve tests
- 💻 Fix an issue or build a feature
- 💬 Ask in [Discord](https://discord.gg/kzJszdKmJ7)

For a large change, open an issue or discuss it in Discord before coding. This
helps us agree on the approach and avoids duplicated work. The complete
contributor guide, architecture tour, extension workflows, and documentation
guide live at [kwatch.dev/docs](https://kwatch.dev/docs).

## 🛠️ Local development

Install Go, `golangci-lint`, and ripgrep (`rg`) before running repository
checks. Helm is also required when changing the chart. Validation scripts fail
clearly if `rg` is unavailable because it is the repository's fast-search
dependency.

1. Fork and clone the repository.
2. Create a short-lived branch from `main`.
3. Read [AGENTS.md](./AGENTS.md) before changing Go code.
4. Make a focused change with tests and documentation.
5. Run the smallest relevant package checks while iterating:

```bash
make verify-fast PKGS="./internal/changed/package/..."
```

6. Run the full verification gate at the end of a coherent workstream:

```bash
make verify
```

The gate builds the binary, runs `go vet`, runs the tests, checks formatting and
line length, checks package boundaries, enforces coverage thresholds, and runs
`golangci-lint`. Use `make coverage-check` to run the coverage gate separately.
The coverage gate uses a race-enabled, repository-wide profile and requires at
least 75% aggregate statement coverage plus 70% for each core runtime package
listed in `scripts/check-coverage.sh`. Generated deep-copy code is the only
documented exclusion.

## 📐 Code and documentation rules

- Keep package dependencies moving downward; follow the package map in
  [AGENTS.md](./AGENTS.md).
- Inject clocks and I/O collaborators when behavior depends on time or a
  network/API call.
- Keep dependency direction explicit; use `make architecture-check` when
  adding or moving packages.
- Add focused tests for every behavior change, including success, failure,
  cancellation, and recovery paths where they exist.
- Keep docs task-focused: explain the goal, show a complete command, and use
  fake credentials.
- Explain Kubernetes terms the first time you use them.
- Use emojis when they help readers scan a page, not as decoration on every
  line.

## 📚 Where documentation belongs

- [README](./README.md): short product explanation and quick install.
- [AGENTS.md](./AGENTS.md): code and agent conventions for this repository.
- [kwatch.dev/docs](https://kwatch.dev/docs): canonical published tutorials,
  how-to guides, reference, architecture, operations, and contributor docs.
- `deploy/*-catalog.tsv`: generated metadata consumed by installers and the
  documentation synchronization workflow.
- `docs/`: source-tree release, legal, security, and offline references that
  are not duplicated as public technical pages.

When adding a feature, update code, tests, generated catalogs, and the
appropriate canonical website documentation through the separate reviewed
documentation synchronization workflow. The website repository is independent
and must not be overwritten from a code change.

## 🚧 Unreleased features

If a feature is merged before its release, mark the relevant documentation:

```markdown
> **🚧 Unreleased** — ships in `vX.Y.Z`. Not available in stable installs yet.
```

The release workflow removes these banners when the stable release ships.

## 📌 Version pins and releases

Do not update version numbers in normal feature pull requests. The release
workflow owns these locations:

- `deploy/deploy.yaml`;
- `deploy/chart/Chart.yaml`; and
- `deploy/chart/README.md`.

Keep install docs version-free. The repository README delegates stable and RC
selection to the interactive `kwatch.sh` manager; do not add release pins there.
Read [RELEASES.md](./RELEASES.md) before cutting a release.

The release workflow also publishes the configuration and feature catalogs used
by [`kwatch.sh`](https://kwatch.dev/kwatch.sh). Add new settings to the Go
configuration source and let the catalog command generate the installer data.

## Real-cluster regression scenarios

The real-cluster scenario guide is in
[`test/e2e/README.md`](test/e2e/README.md). It explains how to add a scenario,
update coverage, reproduce a safe GitHub issue, and run the manual Kind
workflow. The semantic suite uses source manifests and `kubectl`; installer
and Helm validation remain separate.

## 📤 Pull requests

Open pull requests against `main` and include:

- what changed;
- why it changed;
- tests or checks you ran; and
- any documentation or migration notes users need.

Keep commits and pull requests focused. Reviewers may ask for changes before
merging.

## 📜 Community and security

Follow the [Code of Conduct](./CODE_OF_CONDUCT.md). Report security problems
privately using [SECURITY.md](./SECURITY.md); do not publish credentials or
unpatched exploit details in a public issue.
