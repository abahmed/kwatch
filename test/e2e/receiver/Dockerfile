FROM golang:1.27 AS build

WORKDIR /src
COPY . .
RUN GO111MODULE=off CGO_ENABLED=0 go build -trimpath \
	-o /out/kwatch-e2e-receiver .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kwatch-e2e-receiver /kwatch-e2e-receiver
ENTRYPOINT ["/kwatch-e2e-receiver"]
