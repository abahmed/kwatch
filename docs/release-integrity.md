# 🔐 Release integrity

Use this guide when you need to verify that an image or binary came from the
published kwatch release and was not changed on the way to your cluster.

Every published release includes a source commit, image digest, release SBOMs,
checksums, a signed checksum manifest, and a release manifest. The image and
checksum manifest are signed with Cosign using GitHub Actions OIDC; kwatch does
not connect to Sigstore at runtime.

## Use the release image

Use the published version tag for installation and normal operation:

```shell
docker pull ghcr.io/abahmed/kwatch:vX.Y.Z
```

The release manifest records the relationship between the version tag, source commit,
image digest, and (for stable releases) Helm package checksum. The digest is release
evidence for verification, not a separate operational image tag.

## Verify an image

Install Cosign, then verify the digest recorded in the release evidence:

```shell
cosign verify \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --certificate-identity-regexp='^https://github.com/abahmed/kwatch/.github/workflows/publish.yml@' \
  ghcr.io/abahmed/kwatch@sha256:<digest>
```

The command verifies the image behind the version tag; do not use the `sha256-...`
provenance tag as an operational image.

## Verify checksums

Download `SHA256SUMS` and the release files from the matching GitHub Release, then run:

```shell
sha256sum -c kwatch-vX.Y.Z-SHA256SUMS
```

The release manifest's `source.commit` must match the commit shown by the GitHub tag,
and its `image.digest` must match the digest used for the Cosign verification.

## Verify release assets

Download the release assets, then verify the signed checksum manifest. This
authenticates the checksums for the source archive, Helm chart, SBOMs, and other
release evidence:

```shell
cosign verify-blob \
  --bundle kwatch-vX.Y.Z-SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --certificate-identity-regexp='^https://github.com/abahmed/kwatch/.github/workflows/publish.yml@' \
  kwatch-vX.Y.Z-SHA256SUMS

sha256sum -c kwatch-vX.Y.Z-SHA256SUMS
```

The release includes CycloneDX SBOMs for the source tree and the exact image
digest. Validate that the SBOM files named by the release manifest are present
and contain `bomFormat: CycloneDX` before using them for inventory or policy.

## Verify image provenance

GitHub's attestation service records the workflow and source identity used to
build the published image. Verify it against the repository and digest:

```shell
gh attestation verify \
  oci://ghcr.io/abahmed/kwatch@sha256:<digest> \
  --repo abahmed/kwatch \
  --signer-workflow abahmed/kwatch/.github/workflows/publish.yml \
  --predicate-type https://slsa.dev/provenance/v1
```

## Verify the running binary

The image embeds its version and source commit:

```shell
docker run --rm ghcr.io/abahmed/kwatch:vX.Y.Z version --json
```

The returned `version` and `commit` should match the release tag and manifest.
