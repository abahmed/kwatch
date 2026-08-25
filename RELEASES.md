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
| RC (pre-release) | `v0.11.0-rc.1` | `rc` command. Takes the next number while a series is open; otherwise opens a new series using `bump` |
| Stable | `v0.11.0` | `stable` command, promotes the newest RC |
| Patch | `v0.11.1` | `patch` command, increments the highest stable tag |

Versions are always computed from existing tags — maintainers never type a version, they
only choose which part moves.

> **Tag hygiene:** only `-rc.<N>` pre-releases are recognised. The workflow matches
> `vX.Y.Z` for stable tags and `vX.Y.Z-rc.<N>` for RCs, exactly. A tag like `v1.0.0-beta.1`
> is ignored by both, so do not create other pre-release forms — the version computation
> will behave as if it does not exist.

> **Going to v2:** Go requires the module path to carry a `/v2` suffix from major 2 on. The
> workflow warns when it computes a major ≥ 2 while `go.mod` still lacks the suffix, but it
> does not block. Update `go.mod` and every internal import before shipping a v2, or
> `go install ...@v2.0.0` will fail.

## Cutting a release

Run the **Release** workflow (Actions → Release → Run workflow) and set:

| Input | Required | Meaning |
|---|---|---|
| `command` | yes | `rc`, `stable`, or `patch` |
| `bump` | no | `minor` (default), `major`, or `patch`. Only used by `rc` when it opens a new series |
| `target` | no | A commit sha/ref to tag. Defaults to `main` HEAD |
| `dry_run` | no | Compute the version and notes, then stop. Nothing is tagged or built |

Every run writes a **summary on the run page**: the version, whether it is a pre-release,
the tagged commit, the image tags Publish will push, the release URL, and the full notes.
You never need to read the logs to find out what was cut.

### `rc` — pre-release

1. **While a series is open** — an RC whose base version is not yet a stable tag — `rc`
   simply takes the next number: `v0.11.0-rc.4` → `v0.11.0-rc.5`. The `bump` input is
   ignored here, and the run logs a notice saying so.
2. **When no series is open** — the newest RC was already promoted (it counts as
   **consumed**), or there are no RC tags at all — `rc` opens a new series from the newest
   stable, and `bump` decides which part moves:

   | `bump` | From `v0.11.0` | Use for |
   |---|---|---|
   | `minor` (default) | `v0.12.0-rc.1` | new features |
   | `patch` | `v0.11.1-rc.1` | a fix you want to soak before shipping |
   | `major` | `v1.0.0-rc.1` | breaking changes |

3. Creates the tag and opens a GitHub Release marked **pre-release**.
4. Release notes compare against the **previous RC** while a series is open, so each RC
   lists only what is new since the last one. The first RC of a series compares against the
   newest stable.
5. `publish.yml` pushes `ghcr.io/abahmed/kwatch:v<X>.<Y>.<Z>-rc.<N>` only — **no `latest`**,
   and the in-app upgrader does not nag RC users.
6. Updates the **preview install block** in `README.md` (between the `rc-install` markers) to
   the new RC and pushes that commit to `main`. The chart, `deploy/deploy.yaml`, the chart
   README, and the **stable** install snippets are left alone — they stay on the latest
   stable, so a plain `kubectl apply` straight from `main` never ships a preview build.

Run `rc` as often as needed until the candidate stabilizes.

> **Why installing an RC takes one extra command.** `deploy/deploy.yaml` stays pinned to the
> latest **stable** image everywhere — on `main` and at every tag, RC tags included. The bump
> commit is also the commit that gets tagged, so a single commit cannot carry a stable image
> for `main` and an RC image for the tag. Breaking the pin would mean anyone copying
> `deploy/deploy.yaml` out of the repo browser silently installs a preview build, so the
> README's preview block instead tells users to apply the manifest and then
> `kubectl -n kwatch set image deployment/kwatch kwatch=ghcr.io/abahmed/kwatch:<rc tag>`.
>
> `stable` and `patch` need no such step: their bump commit **is** the tagged commit, so
> `deploy.yaml` at the tag already carries the released image and a plain `kubectl apply`
> is correct.
>
> If you ever change `rc` to bump `deploy/deploy.yaml`, drop the `set image` line from the
> README preview block in the same PR — and be clear that you are trading it for a `main`
> that ships preview builds to anyone who copies the manifest.

### `stable` — promote the latest RC

1. Verifies the newest RC points at the current `main` tip; otherwise it fails and you must
   cut a fresh RC first.
2. Bumps every stable pin to the new version, resets the **preview** block to
   *"No release candidate right now."* (the RC it promotes has just shipped), strips
   `🚧 Unreleased` banners from `README.md` and every `docs/*.md`, commits them to `main`,
   and pushes (`RELEASE_TOKEN` required; see below):
   `deploy/chart/Chart.yaml` (`version`, `appVersion`), `deploy/chart/README.md`,
   `deploy/deploy.yaml` (image tag), and the `README.md` stable install snippets
   (`helm install --version`, `/kwatch/vX.Y.Z/deploy/...`).
3. Creates the `v<X>.<Y>.<Z>` tag **on that bump commit** and opens a normal GitHub Release
   (gets `latest`). Because the tag commit carries the bumped files, the raw
   `/kwatch/vX.Y.Z/deploy/...` refs and the chart at the tag match the released version.

> **Pinned-version invariant:** on `main`, the chart version, `deploy.yaml` image tag, chart
> README, and the **stable** README install snippets always point at the latest **stable**
> release. The **preview** README block always points at the newest open RC, or says there
> isn't one. What a visitor reads is what they can actually install, on either channel.
> Every pin is bumped by the release workflow and never by feature PRs. The `docs/`
> reference pages carry **no version pins**; a reference page that needs an install command
> links to the README instead.

### `patch` — hotfix tagged on `main`

1. Merge the fix to `main`.
2. Run the workflow with `command: patch`. It computes `v<X>.<Y>.<Z+1>` from the highest
   stable tag. The `bump` input does not apply — `patch` always moves the patch number.
3. Bumps the pinned references to the patch version and pushes to `main` (same commit/step
   as `stable`, banners are **not** stripped — the next minor's features are still pending).
4. Creates the tag on that bump commit and opens a normal release.

> **A patch blocks a pending promotion.** If an RC is still waiting to be promoted, `patch`
> prints a warning: its bump commit moves `main` past the RC tag, so `stable` will refuse
> until you cut a fresh RC. Cut the RC again after the patch, then promote.

> **Hotfix isolation:** tagging `main` HEAD also bundles any unreleased minor work already
> merged. If the hotfix must ship exactly on top of the previous stable, set `target` to the
> hotfix commit sha (a detached one-off tag). For a strictly isolated hotfix line you can
> also create a throwaway branch locally, cherry-pick, and pass its sha as `target` — no
> persistent release branches are ever kept. When `target` is set, the version bump is made
> on that commit and carried by the tag, but it is **not** pushed to `main` — pushing it
> would rewrite `main`'s pinned versions backwards. Update `main` by hand if it should
> carry the new version.

> The version-bump commit is pushed to the protected `main` branch, so **all three commands**
> require a **`RELEASE_TOKEN`** secret (a maintainer classic PAT with `repo` scope, allowed
> to bypass branch protection). `rc` needs it too, since it now maintains the preview block.
> If the secret is missing or the push fails, the workflow stops **before** tagging and
> prints the manual `git push` command to run.
>
> `RELEASE_TOKEN` is also what makes the image get built. A GitHub Release created with the
> default `GITHUB_TOKEN` does **not** trigger other workflows, so `publish.yml` would never
> run and the release would ship with no container image.

### Previewing a release

Set `dry_run: true` to work out the version and the release notes and then stop. Nothing is
tagged, no release is opened, no image is built. The job is titled **Preview** and the run
summary shows the version it would cut, the image tags it would push, and the full notes.

Use it whenever you are unsure which version a command will produce — for example before a
`rc` that opens a new series, where the answer depends on `bump`.

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

### Install blocks in `README.md`

Version pins live inside HTML comment markers, and the workflow only rewrites what is
between them:

| Marker | Holds | Bumped by |
|---|---|---|
| `<!-- stable-install:start -->` … `:end` | Helm + kubectl install snippets, and the clean-up commands | `stable`, `patch` |
| `<!-- rc-install:start -->` … `:end` | The collapsed preview section | `rc`; reset to a placeholder by `stable` |

Three rules follow from this:

- **Do not delete the markers.** They are how the workflow finds what to rewrite. A pin
  outside them silently stops being maintained — nothing fails, it just goes stale.
- **A marker name may repeat.** `stable-install` wraps two regions (install and clean-up)
  and both are rewritten in one pass.
- **New version pin? Put it inside a block.** If it belongs to the stable channel it goes in
  a `stable-install` region; if it documents the preview it goes in the `rc-install` one.

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

Nothing in this repository publishes the chart. `Chart.yaml` is the source of truth for the
version; the copy users install comes from ArtifactHub and only updates when someone runs
the steps above.

## Upgrader notes

The container image bakes the full version string (the release tag name, `v`-prefixed). The
in-app upgrader only runs on **stable and patch** images: it compares the baked version
against the latest **non-pre-release** GitHub Release and notifies on a newer one, recording
the notified version in a ConfigMap so users are nudged once. **RC builds skip the check
entirely** (`CheckUpdates` returns early when the baked version contains `-rc`) — RC users
opted into the dev channel and are never nagged toward stable. Keep this in mind: the baked
version must equal the release tag name (`v`-prefixed), or the equality comparison in the
upgrader never matches.
