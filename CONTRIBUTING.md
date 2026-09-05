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
helps us agree on the approach and avoids duplicated work.

## 🛠️ Local development

1. Fork and clone the repository.
2. Create a short-lived branch from `main`.
3. Read [AGENTS.md](./AGENTS.md) before changing Go code.
4. Make a focused change with tests and documentation.
5. Run the full verification gate:

```bash
make verify
```

The gate builds the binary, runs `go vet`, runs the tests, checks formatting and
line length, and runs `golangci-lint`.

## 📐 Code and documentation rules

- Keep package dependencies moving downward; follow the package map in
  [AGENTS.md](./AGENTS.md).
- Inject clocks and I/O collaborators when behavior depends on time or a
  network/API call.
- Add focused tests for behavior changes.
- Keep docs task-focused: explain the goal, show a complete command, and use
  fake credentials.
- Explain Kubernetes terms the first time you use them.
- Use emojis when they help readers scan a page, not as decoration on every
  line.

## 📚 Where documentation belongs

- [README](./README.md): short product explanation and quick install.
- [`docs/configuration.md`](./docs/configuration.md): every setting and monitor.
- [`docs/providers.md`](./docs/providers.md): every alert provider.
- [`docs/architecture.md`](./docs/architecture.md): design and runtime behavior.
- [`docs/kwatch-sh.md`](./docs/kwatch-sh.md): interactive manager behavior.
- [kwatch.dev](https://kwatch.dev): beginner guides and the public docs site.

When adding a feature, update the short README section and the detailed
reference. Keep the two repositories consistent.

## 🚧 Unreleased features

If a feature is merged before its release, mark the relevant documentation:

```markdown
> **🚧 Unreleased** — ships in `vX.Y.Z`. Not available in stable installs yet.
```

The release workflow removes these banners when the stable release ships.

## 📌 Version pins and releases

Do not update version numbers in normal feature pull requests. The release
workflow owns these locations:

- `README.md` blocks between `stable-install` markers;
- the preview block between `rc-install` markers;
- `deploy/deploy.yaml`;
- `deploy/chart/Chart.yaml`; and
- `deploy/chart/README.md`.

Keep install docs version-free when possible. Never delete the README markers;
the workflow uses them to update stable and preview commands automatically.
Read [RELEASES.md](./RELEASES.md) before cutting a release.

The release workflow also publishes the configuration and feature catalogs used
by [`kwatch.sh`](https://kwatch.dev/kwatch.sh). Add new settings to the Go
configuration source and let the catalog command generate the installer data.

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
