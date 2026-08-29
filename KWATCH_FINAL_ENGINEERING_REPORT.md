# Kwatch Final Engineering Report

## Scope

This report covers the `kwatch-hardening` branch at source SHA
`37925d7254a9ed5fed36021c0bafb332d182f582`. It records repository-local evidence only;
it is not a production-release authorization.

## Phase status

| Phase | Status | Evidence / limitation |
|---|---|---|
| 0 Baseline | Complete | Baseline and architecture claims recorded. |
| 1 Core correctness | Complete | Graph/controller hardening merged and verified. |
| 2 Lifecycle/correlation/persistence | Complete | Determinism, lifecycle boundaries, and restore behavior verified. |
| 3 Detection | Complete | Nil-safety and monitor boundary fixes verified. |
| 4 Delivery/security/packaging | Complete | HTTP, delivery, diagnostics, and chart checks passed. |
| 5 Self-observability | Complete | Stable metrics behavior verified. |
| 6 Capability assessment | Complete | Existing intelligence retained; no parallel LLM subsystem introduced. |
| 7 Integration/failure injection | Partial | Fake-client integration and race suites pass; live-cluster failure injection unavailable. |
| 8 Performance/stability | Partial | Engine microbenchmark recorded; cluster-scale/soak measurements unavailable. |
| 9 Final audit | Complete (local scope) | Repository search, dependency, security, and documentation review completed. |
| 10 Release readiness | Blocked | Artifact publication and cluster-backed evidence require authorization and infrastructure. |

## Verification

Passed locally:

- `go build ./...`
- `go vet ./...`
- `go test ./...`
- `go test -race ./...`
- `go test -tags=integration ./internal/integration`
- `go test -race -tags=integration ./internal/integration/...`
- `golangci-lint run`
- `staticcheck ./...`
- `govulncheck ./...` (no vulnerabilities reachable from imported packages)
- `helm lint deploy/chart`
- `helm template kwatch deploy/chart`
- `bash deploy/chart/test_helm.sh`

The local `BenchmarkProcess` baseline is 1,164 ns/op, 1,912 B/op, and 11 allocs/op on an
Apple M4 virtual CPU. It is not a cluster-capacity claim.

## Security and known limitations

Operational randomness uses `crypto/rand`; the GitHub updater uses a maintained client and
does not pull the deprecated `x/crypto/openpgp` implementation. Remaining `gosec` findings
are informer event-handler return values intentionally ignored by controller wiring.

Unavailable or unauthorized checks include Kubernetes-version compatibility, API failure
injection, informer reconnect testing, scale/soak, container inspection, SBOM/provenance/
signing, and published-artifact install/rollback. HA remains unsupported and must not be
implied by this report.

## Capability status

Deduplication, blast-radius attribution, incident intelligence, maintenance awareness, and
change correlation are implemented and covered by repository tests. Their production-scale
limits and cluster failure behavior remain unverified until Phase 7/8 infrastructure is
available.

## Release decision

No tag, image push, chart publication, GitHub release, or external-cluster mutation was
performed. Release actions remain pending explicit maintainer authorization and completion
of the blocked evidence items above.
