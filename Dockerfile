# Build on the native runner architecture and cross-compile from there.
# Without --platform=$BUILDPLATFORM this stage runs under QEMU for arm64,
# arm/v7 and arm/v6, which is what made a release build take over an hour.
# Go cross-compiles fine and CGO is off, so emulation buys nothing.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

ARG RELEASE_VERSION="dev"
ARG GIT_COMMIT="none"
ARG BUILD_DATE="unknown"

# Supplied by buildx for each requested platform.
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG SOURCE_DATE_EPOCH="0"

WORKDIR /build

# Dependencies change far less often than source, so keep them in their own
# layer. This RUN references no TARGET* args, so BuildKit runs it once and
# shares the result across all four platforms.
COPY go.mod go.sum /build/
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . /build/

# No -a: it forces a rebuild of every package including the standard library
# on every run, which makes Go's build cache useless. -installsuffix has done
# nothing since Go 1.10. -buildvcs=false keeps Go from needing git in the
# image; the version values come from ldflags anyway.
#
# This RUN does reference TARGET* args, so BuildKit runs it once per platform.
# The mounts are left at the default sharing=shared so those four runs happen
# concurrently; sharing=locked would serialize them. GOCACHE and GOMODCACHE
# are both safe for concurrent use by multiple processes.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    : "${TARGETOS:?empty - buildx did not supply TARGETOS}"; \
    : "${TARGETARCH:?empty - buildx did not supply TARGETARCH}"; \
    export CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH}"; \
    if [ "${TARGETARCH}" = "arm" ]; then export GOARM="${TARGETVARIANT#v}"; fi; \
    go build -trimpath -buildvcs=false \
      -ldflags "-X github.com/abahmed/kwatch/internal/version.version=${RELEASE_VERSION} -X github.com/abahmed/kwatch/internal/version.gitCommitID=${GIT_COMMIT} -X github.com/abahmed/kwatch/internal/version.buildDate=${BUILD_DATE}" \
      -o kwatch ./cmd/kwatch

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
LABEL maintainer="Abdelrahman Ahmed <a.ahmed1026@gmail.com>"

# Keep the release identity in the image itself. The publish workflow supplies
# these values from the exact tag checkout, rather than from the workflow's
# default branch context.
ARG RELEASE_VERSION="dev"
ARG GIT_COMMIT="none"
ARG BUILD_DATE="unknown"
LABEL org.opencontainers.image.source="https://github.com/abahmed/kwatch" \
      org.opencontainers.image.version="${RELEASE_VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.licenses="Elastic-2.0"

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 1000 kwatch
COPY --from=builder /build/kwatch /bin/kwatch
USER kwatch
ENTRYPOINT ["/bin/kwatch"]
