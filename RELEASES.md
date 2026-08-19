# Releasing kwatch

This document describes how kwatch is branched and released. Releases are cut with the
`.github/workflows/release.yml` workflow using `workflow_dispatch`. It creates the version
tag and a GitHub Release; `.github/workflows/publish.yml` then builds and pushes the
multi-arch container image to `ghcr.io/abahmed/kwatch`.

## Branch model

**One branch: `main`.** All changes land here via short-lived PR branches
(`feat/*`, `fix/*`, `refactor/*`). `main` is protected (review + green `check` required).

There are no release or develop branches. Every release — RC, stable, or patch — is a
**tag** on `main` history plus a GitHub Release.

## Versioning

Semantic versioning, `v`-prefixed:

| Stage | Example | How it is produced |
|---|---|---|
| RC (pre-release) | `v0.11.0-rc.1` | `rc` command, auto-incremented from the newest RC tag |
| Stable | `v0.11.0` | `stable` command, promotes the newest RC |
| Patch | `v0.11.1` | `patch` command, increments the highest stable tag |

Versions are always computed from existing tags — maintainers never type a version.

## Cutting a release

Run the **Release** workflow (Actions → Release → Run workflow), pick a `command`, and
optionally set `target` (a commit sha/ref; defaults to `main` HEAD).

### `rc` — pre-release

1. Computes the next `v<X>.<Y>.<Z>-rc.<N>` from the newest RC tag.
2. Creates the tag and opens a GitHub Release marked **pre-release**.
3. `publish.yml` pushes `ghcr.io/abahmed/kwatch:<X>.<Y>.<Z>-rc.<N>` only — **no `latest`**,
   and the in-app upgrader does not nag RC users.
4. Chart and README versions are untouched (they stay pinned to the latest released version,
   e.g. `v0.10.5`).

Run `rc` as often as needed until the candidate stabilizes.

### `stable` — promote the latest RC

1. Verifies the newest RC points at the current `main` tip; otherwise it fails and you must
   cut a fresh RC first.
2. Creates `v<X>.<Y>.<Z>`, opens a normal GitHub Release (gets `latest`).
3. Bumps every pinned version reference to the new version and commits them to `main`:
   `deploy/chart/Chart.yaml` (`version`, `appVersion`), `deploy/chart/README.md`,
   `deploy/deploy.yaml` (image tag), and the `README.md` install snippets
   (`helm install --version`, `/kwatch/vX.Y.Z/deploy/...`).

> **Pinned-version invariant:** on `main`, the chart version, `deploy.yaml` image tag, chart
> README, and README install snippets always point at the **latest released version** — what
> a visitor reads is what they can actually install. They are bumped by the `stable` command
> and never by feature PRs.

### `patch` — hotfix tagged on `main`

1. Merge the fix to `main`.
2. Run the workflow with `command: patch`. By default the tag points at `main` HEAD.
3. Computes `v<X>.<Y>.<Z+1>` from the highest stable tag, opens a normal release.

> **Hotfix isolation:** tagging `main` HEAD also bundles any unreleased minor work already
> merged. If the hotfix must ship exactly on top of the previous stable, set `target` to the
> hotfix commit sha (a detached one-off tag). For a strictly isolated hotfix line you can
> also create a throwaway branch locally, cherry-pick, and pass its sha as `target` — no
> persistent release branches are ever kept.

> Pushing the version-bump commit to the protected `main` branch requires a
> `RELEASE_TOKEN` secret (a maintainer classic PAT with `repo` scope, allowed to bypass
> branch protection). If the secret is absent the workflow still cuts the release and prints
> the manual `git push` command.

## RC → stable gates

An RC should not be promoted until all of these hold:

- [ ] RC has been published for at least **2 weeks** of soak (unless a critical fix is blocking).
- [ ] No open **critical** issues / known regressions against the RC.
- [ ] `check` workflow is green on `main` (lint, build, unit tests with `-race`, integration tests).
- [ ] `helm lint` + `test_helm.sh` pass for the released chart.
- [ ] Release notes reviewed (generated automatically from merged commit titles).
- [ ] README contains no `🚧 Unreleased` banners (stripped automatically on stable).

## README and unreleased features

Feature **code** merges to `main` immediately. A feature's **README section** also merges
immediately, but under a banner while unreleased:

```
> **🚧 Unreleased** — ships in `v0.12.0`. Not available in stable installs yet.
```

- Unreleased sections stay visible on `main` with the banner, so docs don't drift.
- The `stable` command strips every banner line and bumps the pinned version references
  (`helm install --version`, `/kwatch/vX.Y.Z/deploy/...`, `deploy.yaml` image, chart files)
  in the same commit.
- Maintainers never touch version numbers by hand; the pinned-version invariant and the
  banner convention are enforced by `CONTRIBUTING.md` and the gate checklist above.

## Releasing the Helm chart (manual)

The chart (`deploy/chart`) is published to the ArtifactHub **kwatch** repository. After a
stable release, package and upload it manually — always from `main` HEAD **after** the
`stable` workflow's version-bump commit, so `Chart.yaml` carries the released version:

```sh
# Update the artifacthub.io/changes annotation in Chart.yaml to describe this release first
helm lint deploy/chart
helm package deploy/chart   # uses Chart.yaml version, e.g. kwatch-0.11.0.tgz
# upload the resulting .tgz to the ArtifactHub kwatch repository
```

## Upgrader notes

The in-app upgrader compares the baked version against the latest **non-pre-release**
GitHub Release. The container image bakes the full tag (e.g. `v0.11.0-rc.1`), so RC images
never report "an update is available" for an RC. Keep this in mind: the baked version must
equal the release tag name (`v`-prefixed).