## Contributing to kwatch

:tada: Anyone can contribute to kwatch. Newcomers are always welcome to contribute to kwatch, and we are happy to offer help to newcomers.
Before making changes, please first discuss the change you want to make through [Discord](https://discord.gg/kzJszdKmJ7)


### There are many ways to contribute:

+ [Suggest new features to be implemented](https://github.com/abahmed/kwatch/issues)
+ [Report issues](https://github.com/abahmed/kwatch/issues)
+ [Improve Documentation](https://github.com/abahmed/kwatch)
+ [Fix issues](https://github.com/abahmed/kwatch/issues)


### Code Contribution

If you wish to work on an issue, please comment on the issue that you want to work on it. This is to prevent duplicated efforts on the same issue.

Before you start coding, read [AGENTS.md](./AGENTS.md) — it documents the build/test gate,
package layout, naming conventions, and the checklist for adding a new monitored resource.


Contributions to kwatch should be made in the form of pull requests to the **main** branch. Each pull request will be reviewed by someone with permission to land patches. After reviewing the patch, it could be landed in the main branch or given feedback for changes.

### Documenting features

Feature documentation lives in the right place: the landing-page **README** gets a short
section and a link; the detail goes into a page under `docs/`
([configuration](./docs/configuration.md), [providers](./docs/providers.md),
[architecture](./docs/architecture.md)). Docs pages keep the **same banner convention** as
the README but never carry version pins.

- **Released features:** document them normally, no marker.
- **Unreleased features:** merge the code and the doc section, but mark the section with the
  official banner:

  ```
  > **🚧 Unreleased** — ships in `v0.12.0`. Not available in stable installs yet.
  ```

  The banner is stripped automatically when a stable release is cut (see `RELEASES.md` —
  from `README.md` and every `docs/*.md`), so the docs on `main` always tell users exactly
  what has shipped versus what is pending.

  When a whole milestone rewrite is still unreleased, a single top-of-file banner instead
  marks the entire README as documenting the dev build (e.g. `v0.11.0-rc`). Keep it
  version-free about the stable — a patch release during the window would otherwise make it
  stale. It uses the same `🚧 Unreleased` marker and is stripped the same way.

- **Never touch pinned version references in a feature/bug PR.** The stable `README.md`
  install snippets, the `deploy.yaml` image tag, `deploy/chart/Chart.yaml`, and
  `deploy/chart/README.md` always point at the **latest stable release** (e.g. `v0.10.5`).
  The preview block in `README.md` points at the **newest open release candidate**, or says
  there isn't one. The release workflow bumps all of them at release time. Keep `docs/`
  pages **free of version pins** — if a reference page needs an install command, link to the
  README instead. If a pinned snippet looks stale, bump it in a PR, but do not pre-stamp the
  next version (e.g. `v0.11.0`) before it ships.

- **Never delete the install markers in `README.md`.** Version pins live between HTML
  comments — `<!-- stable-install:start -->` … `:end` and `<!-- rc-install:start -->` …
  `:end` — and the release workflow only rewrites what is inside them. A pin moved outside a
  block silently stops being maintained: nothing fails, the snippet just goes stale and
  starts telling users to install a version that isn't current. If you add a version pin,
  put it inside the block for its channel. A marker name may repeat — `stable-install` wraps
  both the install section and the clean-up section, and both are rewritten together.

- All three release commands (`rc`, `stable`, `patch`) push a version-bump commit to
  protected `main`, so they need the `RELEASE_TOKEN` secret (see `RELEASES.md`). `rc` needs
  it too, because it maintains the preview block. Without the secret the workflow fails
  before tagging — add it before first use.

### Code of Conduct
We expect everyone to follow the [Code Of Conduct](./CODE_OF_CONDUCT.md)
