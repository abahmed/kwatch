# Kwatch Release-Candidate Evidence

## Source

- Branch: `kwatch-hardening`
- Source SHA: `0d0457842628e9d3ba7db3cd97f394860ed6006a`
- Working tree: clean
- Build-time binary version defaults to `dev`; release workflows inject the version.

## Local verification

- `go build ./...`: passed
- `go vet ./...`: passed
- `go test ./...`: passed
- `go test -race ./...`: passed on the current source
- `go test -tags=integration ./internal/integration`: passed using the repository's fake-client
  integration harness (cluster-backed scenarios remain unverified).
- `go test -race -tags=integration ./internal/integration/...`: passed.
- Local `BenchmarkProcess` baseline (Apple M4 virtual, 2s): 1,164 ns/op, 1,912 B/op,
  11 allocs/op. This is an engine microbenchmark, not a cluster-scale capacity claim.
- LLM-removal search: no implementation, dependency, configuration, or deployment references
  were found; matches are limited to audit-plan wording and unrelated `image pull` test names.
- Local release metadata: chart `version: 0.10.5`, chart `appVersion: v0.10.5`, and raw
  manifest image `ghcr.io/abahmed/kwatch:v0.10.5`; the binary defaults to `dev` unless release
  ldflags are supplied.
- `golangci-lint run`: passed
- `helm lint deploy/chart`: passed
- `helm template kwatch deploy/chart`: passed
- `bash deploy/chart/test_helm.sh`: passed
- `staticcheck ./...`: passed
- `gosec ./...`: remaining findings are the intentional informer `G104` cases; the
  configuration-controlled paths in `internal/config/load_config.go` are explicitly
  documented with scoped `nosec` annotations.
- `govulncheck ./...`: no vulnerabilities reachable from imported packages. One vulnerability
  remains in a required-but-unreachable module dependency; it is not called by Kwatch.
- Additional security hardening: audit-log creation is restricted to owner-only (`0600`),
  heartbeat response-body close errors are surfaced, restart-count rendering clamps values
  before narrowing to `int32`, operational randomness uses `crypto/rand`, and the GitHub
  client no longer pulls the deprecated `x/crypto/openpgp` implementation.

## Infrastructure-dependent checks not executed

The environment does not provide `kubectl`, `kind`, or `docker`. Therefore Kubernetes-version
integration, failure injection, scale/soak, container inspection, SBOM/provenance/signing,
and cluster-backed release checks remain unverified.

## Release authorization

No tag, image push, chart publication, GitHub release, or external-cluster mutation was
performed. Those actions require explicit authorization and a completed artifact-evidence
bundle.

## Configuration/version note

The source checkout intentionally reports `dev` unless built with release ldflags. The chart
and raw manifest currently carry their existing `0.10.5` references; release automation must
update and verify these pins for the intended release before publication.
