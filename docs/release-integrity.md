# 🔐 Release integrity

Use this guide when you need to verify that an image or binary came from the
published kwatch release and was not changed on the way to your cluster.

Every published container release includes a source commit, image digest, checksums,
and a release manifest. The image is signed with Cosign using GitHub Actions
OIDC; kwatch does not connect to Sigstore at runtime.

## Pin the image

Use the immutable digest from the GitHub Release instead of a mutable tag:

```shell
docker pull ghcr.io/abahmed/kwatch@sha256:<digest>
```

The release manifest records the relationship between the version tag, source commit,
image digest, and (for stable releases) Helm package checksum.

## Verify an image

Install Cosign, then verify the digest from the release:

```shell
cosign verify \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --certificate-identity-regexp='^https://github.com/abahmed/kwatch/.github/workflows/publish.yml@' \
  ghcr.io/abahmed/kwatch@sha256:<digest>
```

The command should be run against the exact digest, not `latest` or another mutable tag.

## Verify checksums

Download `SHA256SUMS` and the release files from the matching GitHub Release, then run:

```shell
sha256sum -c kwatch-vX.Y.Z-SHA256SUMS
```

The release manifest's `source.commit` must match the commit shown by the GitHub tag,
and its `image.digest` must match the digest used for the Cosign verification.

## Verify the running binary

The image embeds its version and source commit:

```shell
docker run --rm ghcr.io/abahmed/kwatch@sha256:<digest> version --json
```

The returned `version` and `commit` should match the release tag and manifest.
