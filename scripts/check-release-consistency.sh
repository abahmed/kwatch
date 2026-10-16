#!/usr/bin/env bash
set -euo pipefail

chart_version=$(sed -n 's/^version: "\([^"]*\)"/\1/p' \
  deploy/chart/Chart.yaml)
chart_app_version=$(sed -n 's/^appVersion: "\([^"]*\)"/\1/p' \
  deploy/chart/Chart.yaml)
manifest_version=$(sed -n \
  's#^[[:space:]]*image: ghcr.io/abahmed/kwatch:\(.*\)$#\1#p' \
  deploy/deploy.yaml)

if [[ -z "$chart_version" || -z "$chart_app_version" ||
  -z "$manifest_version" ]]; then
  echo "release metadata is incomplete" >&2
  exit 1
fi
if [[ "$chart_app_version" != "v$chart_version" ]]; then
  echo "Chart.yaml version and appVersion disagree" >&2
  exit 1
fi
if [[ "$manifest_version" != "$chart_app_version" ]]; then
  echo "deploy/deploy.yaml image and Chart.yaml disagree" >&2
  exit 1
fi

# The licensetest tag turns on a test-only edition override. Release and
# publish images must never get it: the Dockerfile default stays empty and no
# release workflow passes GO_BUILD_TAGS.
if ! grep -q '^ARG GO_BUILD_TAGS=""$' Dockerfile; then
  echo "Dockerfile GO_BUILD_TAGS must default to empty" >&2
  exit 1
fi
if grep -rn 'GO_BUILD_TAGS\|licensetest' .github/workflows/publish.yml \
  .github/workflows/ci.yml; then
  echo "release workflows must not build with GO_BUILD_TAGS or licensetest" >&2
  exit 1
fi

echo "release metadata is consistent: $chart_app_version"
