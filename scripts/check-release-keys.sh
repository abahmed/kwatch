#!/usr/bin/env bash
# Fails when the binary would ship without a production license key
# (internal/license trustedKeys is empty). Run by the release and publish
# workflows and by `make release-keys-check`; never part of `make verify`,
# so local development works while the key is a placeholder.
set -euo pipefail

RELEASE=1 go test -count=1 -run '^TestReleaseTrustedKeys$' ./internal/license
echo "release trusted keys present"
