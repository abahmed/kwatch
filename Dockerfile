FROM golang:1.26-alpine AS builder
ARG RELEASE_VERSION="dev"
ARG GIT_COMMIT="none"
ARG BUILD_DATE="unknown"
LABEL maintainer="Abdelrahman Ahmed <a.ahmed1026@gmail.com>"

RUN apk update && \
    apk add git build-base && \
    rm -rf /var/cache/apk/* && \
    mkdir -p "/build"

WORKDIR /build
COPY go.mod go.sum /build/
RUN go mod download

COPY . /build/
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/abahmed/kwatch/internal/version.version=${RELEASE_VERSION} -X github.com/abahmed/kwatch/internal/version.gitCommitID=${GIT_COMMIT} -X github.com/abahmed/kwatch/internal/version.buildDate=${BUILD_DATE}" \
    -a -installsuffix cgo -o kwatch ./cmd/kwatch

FROM alpine:3.24
RUN apk add --update ca-certificates && \
    adduser -D -u 1000 kwatch && \
    rm -rf /var/cache/apk/*
COPY --from=builder /build/kwatch /bin/kwatch
USER kwatch
ENTRYPOINT ["/bin/kwatch"]
