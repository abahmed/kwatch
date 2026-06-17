#!/usr/bin/env sh
set -eu

DIR="$(dirname "$0")"
IMAGE="${IMAGE:-ghcr.io/abahmed/kwatch-llm}"
TAG="${TAG:-$(cat "$DIR/VERSION")}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"

docker buildx build --platform "$PLATFORMS" -f "$DIR/Dockerfile" \
  --build-arg MODEL_VERSION="$(cat "$DIR/VERSION")" \
  -t "$IMAGE:$TAG" ${PUSH:+--push} "$DIR"

echo "built $IMAGE:$TAG"
