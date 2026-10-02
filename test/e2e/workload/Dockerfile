# Same digest-pinned builder as the release Dockerfile; Dependabot's docker
# entry for this directory keeps the digest current.
FROM golang:1.27-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build

WORKDIR /src
COPY . .
RUN GO111MODULE=off CGO_ENABLED=0 go build -trimpath \
	-o /out/kwatch-e2e-workload .

# distroless publishes no versioned tags for this variant, only the moving
# nonroot tag. Dependabot's docker entry for this directory proposes digest
# updates once a digest is recorded here.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kwatch-e2e-workload /kwatch-e2e-workload
ENTRYPOINT ["/kwatch-e2e-workload"]
