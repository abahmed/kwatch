# Build on the native runner architecture and cross-compile from there.
# Without --platform=$BUILDPLATFORM this stage runs under QEMU for arm64,
# arm/v7 and arm/v6, which is what made a release build take over an hour.
# Go cross-compiles fine and CGO is off, so emulation buys nothing.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG RELEASE_VERSION="dev"
ARG GIT_COMMIT="none"
ARG BUILD_DATE="unknown"

# Supplied by buildx for each requested platform.
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

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
    export CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}"; \
    if [ "${TARGETARCH}" = "arm" ]; then export GOARM="${TARGETVARIANT#v}"; fi; \
    go build -trimpath -buildvcs=false \
      -ldflags "-X github.com/abahmed/kwatch/internal/version.version=${RELEASE_VERSION} -X github.com/abahmed/kwatch/internal/version.gitCommitID=${GIT_COMMIT} -X github.com/abahmed/kwatch/internal/version.buildDate=${BUILD_DATE}" \
      -o kwatch ./cmd/kwatch

FROM alpine:3.24
LABEL maintainer="Abdelrahman Ahmed <a.ahmed1026@gmail.com>"
RUN apk add --no-cache ca-certificates && \
    adduser -D -u 1000 kwatch
COPY --from=builder /build/kwatch /bin/kwatch
USER kwatch
ENTRYPOINT ["/bin/kwatch"]
