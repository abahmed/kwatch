FROM golang:alpine AS builder
ARG RELEASE_VERSION="nothing"
LABEL maintainer="Abdelrahman Ahmed <a.ahmed1026@gmail.com>"

RUN apk update && \
    apk add git build-base && \
    rm -rf /var/cache/apk/* && \
    mkdir -p "/build"

WORKDIR /build
COPY go.mod go.sum /build/
RUN go mod download

COPY . /build/
RUN sed -i 's/dev/'"${RELEASE_VERSION}"'/g' internal/version/version.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o kwatch ./cmd/kwatch

FROM alpine:latest
RUN apk add --update ca-certificates && \
    adduser -D -u 1000 kwatch && \
    rm -rf /var/cache/apk/*
COPY --from=builder /build/kwatch /bin/kwatch
USER kwatch
ENTRYPOINT ["/bin/kwatch"]
