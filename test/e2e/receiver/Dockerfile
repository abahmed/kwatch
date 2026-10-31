# Same digest-pinned builder as the release Dockerfile; Dependabot's docker
# entry for this directory keeps the digest current.
FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build

WORKDIR /src
COPY . .
RUN GO111MODULE=off CGO_ENABLED=0 go build -trimpath \
	-o /out/kwatch-e2e-receiver .

# distroless publishes no versioned tags for this variant, only the moving
# nonroot tag. Dependabot's docker entry for this directory proposes digest
# updates once a digest is recorded here.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kwatch-e2e-receiver /kwatch-e2e-receiver
ENTRYPOINT ["/kwatch-e2e-receiver"]
