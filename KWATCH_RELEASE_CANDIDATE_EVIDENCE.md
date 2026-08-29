# Kwatch Release-Candidate Evidence

## Source

- Branch: `kwatch-hardening`
- Source SHA: `1aaa0c796c124c964de4a0e3a7ea4fd6569ca074`
- Working tree: clean
- Build-time binary version defaults to `dev`; release workflows inject the version.

## Local verification

- `go build ./...`: passed
- `go vet ./...`: passed
- `go test ./...`: passed
- `go test -race ./...`: passed in the Phase 5 verification run
- `golangci-lint run`: passed
- `helm lint deploy/chart`: passed
- `helm template kwatch deploy/chart`: passed
- `bash deploy/chart/test_helm.sh`: passed

## Infrastructure-dependent checks not executed

The environment does not provide `kubectl`, `kind`, `docker`, `staticcheck`, `gosec`, or
`govulncheck`. Therefore Kubernetes-version integration, failure injection, scale/soak,
container inspection, SBOM/provenance/signing, and vulnerability scans remain unverified.

## Release authorization

No tag, image push, chart publication, GitHub release, or external-cluster mutation was
performed. Those actions require explicit authorization and a completed artifact-evidence
bundle.

## Configuration/version note

The source checkout intentionally reports `dev` unless built with release ldflags. The chart
and raw manifest currently carry their existing `0.10.5` references; release automation must
update and verify these pins for the intended release before publication.

