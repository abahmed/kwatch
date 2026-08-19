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


Contributions to kwatch should be made in the form of pull requests to the **main** branch. Each pull request will be reviewed by someone with permission to land patches. After reviewing the patch, it could be landed in the main branch or given feedback for changes.

### Documenting features in the README

- **Released features:** document them normally, no marker.
- **Unreleased features:** merge the code and the README section, but mark the section with the official banner:

  ```
  > **🚧 Unreleased** — ships in `v0.12.0`. Not available in stable installs yet.
  ```

  The banner is stripped automatically when a stable release is cut (see `RELEASES.md`), so
  the README on `main` always tells users exactly what has shipped versus what is pending.

- **Never touch pinned version references in a feature/bug PR.** `README.md` install
  snippets, the `deploy.yaml` image tag, `deploy/chart/Chart.yaml`, and
  `deploy/chart/README.md` always point at the **latest released version** (e.g. `v0.10.5`);
  the `stable` workflow bumps them at release time. If a snippet looks stale, bump it in a
  PR, but do not pre-stamp the next version (e.g. `v0.11.0`) before it ships.

### Code of Conduct
We expect everyone to follow the [Code Of Conduct](./CODE_OF_CONDUCT.md)
