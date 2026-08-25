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

1. Computes the next `v<X>.<Y>.<Z>-rc.<N>` from the newest RC tag. An RC whose base version
   already exists as a stable tag counts as **consumed** (it was promoted), so the next RC
   branches from the newest stable instead — e.g. after promoting `v0.11.0`, the next RC is
   `v0.12.0-rc.1`, never `v0.11.0-rc.<N+1>`.
2. Creates the tag and opens a GitHub Release marked **pre-release**.
3. `publish.yml` pushes `ghcr.io/abahmed/kwatch:v<X>.<Y>.<Z>-rc.<N>` only — **no `latest`**,
   and the in-app upgrader does not nag RC users.
4. Chart and README versions are untouched (they stay pinned to the latest released version,
   e.g. `v0.10.5`).

Run `rc` as often as needed until the candidate stabilizes.

### `stable` — promote the latest RC

1. Verifies the newest RC points at the current `main` tip; otherwise it fails and you must
   cut a fresh RC first.
2. Bumps every pinned version reference to the new version, strips `🚧 Unreleased` banners
   from `README.md` and every `docs/*.md`, commits them to `main`, and pushes
   (`RELEASE_TOKEN` required; see below):
   `deploy/chart/Chart.yaml` (`version`, `appVersion`), `deploy/chart/README.md`,
   `deploy/deploy.yaml` (image tag), and the `README.md` install snippets
   (`helm install --version`, `/kwatch/vX.Y.Z/deploy/...`).
3. Creates the `v<X>.<Y>.<Z>` tag **on that bump commit** and opens a normal GitHub Release
   (gets `latest`). Because the tag commit carries the bumped files, the raw
   `/kwatch/vX.Y.Z/deploy/...` refs and the chart at the tag match the released version.

> **Pinned-version invariant:** on `main`, the chart version, `deploy.yaml` image tag, chart
> README, and README install snippets always point at the **latest released version** — what
> a visitor reads is what they can actually install. They are bumped by the `stable` and
> `patch` commands and never by feature PRs. The `docs/` reference pages carry **no version
> pins**; a reference page that needs an install command links to the README instead.

### `patch` — hotfix tagged on `main`

1. Merge the fix to `main`.
2. Run the workflow with `command: patch`. By default it computes `v<X>.<Y>.<Z+1>` from the
   highest stable tag.
3. Bumps the pinned references to the patch version and pushes to `main` (same commit/step
   as `stable`, banners are **not** stripped — the next minor's features are still pending).
4. Creates the tag on that bump commit and opens a normal release.

> **Hotfix isolation:** tagging `main` HEAD also bundles any unreleased minor work already
> merged. If the hotfix must ship exactly on top of the previous stable, set `target` to the
> hotfix commit sha (a detached one-off tag). For a strictly isolated hotfix line you can
> also create a throwaway branch locally, cherry-pick, and pass its sha as `target` — no
> persistent release branches are ever kept. Note that the bump still lands on `main`, so a
> `target`-based tag's files stay at the previous version by design.

> The version-bump commit is pushed to the protected `main` branch, so `stable` and `patch`
> require a **`RELEASE_TOKEN`** secret (a maintainer classic PAT with `repo` scope, allowed
> to bypass branch protection). If the secret is missing or the push fails, the workflow
> stops **before** tagging and prints the manual `git push` command to run.

## RC → stable gates

An RC should not be promoted until all of these hold:

- [ ] RC has been published for at least **2 weeks** of soak (unless a critical fix is blocking).
- [ ] No open **critical** issues / known regressions against the RC.
- [ ] `check` workflow is green on `main` (lint, build, unit tests with `-race`, integration tests).
- [ ] `helm lint` + `test_helm.sh` pass for the released chart.
- [ ] Release notes reviewed (generated automatically from merged commit titles).
- [ ] README and `docs/` contain no `🚧 Unreleased` banners (stripped automatically on stable).

## README, docs, and unreleased features

Feature **code** merges to `main` immediately. A feature's **README section** also merges
immediately, but under a banner while unreleased:

```
> **🚧 Unreleased** — ships in `v0.12.0`. Not available in stable installs yet.
```

- Unreleased sections stay visible on `main` with the banner, so docs don't drift.
- When a whole milestone rewrite is unreleased (e.g. the current `v0.11.0-rc` build), one
  top-of-file banner marks the entire README as documenting the dev build. Same `🚧 Unreleased`
  marker, stripped the same way.
- The `stable` command strips every banner line from `README.md` and `docs/*.md` and bumps
  the pinned version references (`helm install --version`, `/kwatch/vX.Y.Z/deploy/...`,
  `deploy.yaml` image, chart files) in the same commit.
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

The container image bakes the full version string (the release tag name, `v`-prefixed). The
in-app upgrader only runs on **stable and patch** images: it compares the baked version
against the latest **non-pre-release** GitHub Release and notifies on a newer one, recording
the notified version in a ConfigMap so users are nudged once. **RC builds skip the check
entirely** (`CheckUpdates` returns early when the baked version contains `-rc`) — RC users
opted into the dev channel and are never nagged toward stable. Keep this in mind: the baked
version must equal the release tag name (`v`-prefixed), or the equality comparison in the
upgrader never matches.