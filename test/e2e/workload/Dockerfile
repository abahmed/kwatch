# Same digest-pinned builder as the release Dockerfile; Dependabot's docker
# entry for this directory keeps the digest current.
FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build

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
