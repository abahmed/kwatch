# Production operations

This page describes the supported operating model for Kwatch. It is an
operations reference, not a promise of high availability.

## Operating model

Kwatch runs two replicas by default with Kubernetes Lease leader election and a
`RollingUpdate` strategy. Exactly one Pod is active; the other is a standby.
Only the active Pod watches resources, delivers notifications, and writes
mutable state. A one-replica override is supported for very small clusters,
but it has no Kwatch self-failover.

Additional replicas are standby capacity, not monitoring workers. They increase
resource usage and takeover options but do not increase monitoring throughput.
Topology spreading is recommended when node failure protection matters.

Initial operating targets are: readiness within 120 seconds after a healthy
startup, normal leader takeover within 90 seconds, graceful shutdown within
45 seconds, and persistence restore within 30 seconds for normal state sizes.
These are SLO targets for capacity planning and alerting, not election protocol
guarantees. Queue limits and configured worker counts remain the authority for
memory and delivery capacity.

Operational CI covers one through five replicas and an explicit scale-up,
standby-readiness, PDB, standby-removal, leader-removal, scale-down, and
rollback scenario.

Use the published image digest for production deployments. Version tags and
chart versions must refer to the same release. Keep the default diagnostics
and profiling endpoints disabled unless they are required for a controlled
investigation and protected with a diagnostic token.

The Helm chart includes an opt-in NetworkPolicy template. When enabling it,
provide ingress rules for health clients and egress rules for the Kubernetes
API, DNS, and configured notification providers. The default is disabled so
existing installations do not lose connectivity during an upgrade.

## Health and readiness

- `/healthz` reports process liveness.
- `/readyz` reports whether required monitoring infrastructure is ready.
- `/availabilityz` reports whether the Pod is participating in election and can
  be safely retained during a Deployment rollout. It is the Kubernetes
  Deployment probe, not monitoring readiness.
- `/health` reports optional monitor degradation and safe reason codes.

An absent optional Kubernetes API is reported as degraded and does not create a
synthetic incident. A required informer or source that is not ready keeps the
application unready until it is available or the configuration disables that
pipeline.

## Shutdown and recovery

Kwatch gives workers and persistence savers a bounded shutdown window. On
termination, producers stop before the final incident snapshot is written.
If a dependency does not stop within its deadline, Kwatch records a shutdown
timeout and exits rather than waiting forever.

The deployment provides a 60-second termination grace period. Required
component failure or an internal stall makes the active Pod unready, stops its
active generation, fences persistence, releases leadership, and lets Kubernetes
restart it. Optional component failures remain visible as bounded degraded
health states and use bounded restart backoff; they do not create synthetic
incidents.

If the active Pod loses its Lease, the standby cancels the old active
generation and takes over. The new leader restores persisted state, waits for
required informer synchronization, and reconciles current Kubernetes state
before becoming ready. The monitoring gap is reported from the last persisted
liveness marker. Kubernetes Events that expired while Kwatch was unavailable
cannot be reconstructed.

Persistence ConfigMaps should be backed up according to the operator's normal
cluster backup policy. Before upgrading, verify that the backup includes the
Kwatch state ConfigMaps. Migration failures are reported through the health
diagnostics and must be resolved or rolled back using the release notes before
restarting the workload.

## Outages and capacity

Kubernetes API and provider outages are handled through bounded retries and
degraded status. Delivery queues are bounded; sustained saturation can drop
notifications, so operators should alert on queue saturation, terminal
delivery failures, and dead letters.

Size CPU, memory, worker counts, queue capacity, and resync intervals for the
cluster size and enabled monitors. The chart defaults are suitable for small
installations only; large clusters require load testing before increasing
worker counts.

The default RBAC profile is intentionally read-only but includes permissions
for optional monitors. Review the enabled monitor set and the generated RBAC
manifest before deployment, and remove optional permissions when maintaining a
custom least-privilege profile.

## Release verification

Release evidence includes a source mapping, image digest, checksums, and image
signature/provenance. Verify the release assets before upgrading and deploy the
image by digest when the platform supports it. Do not reuse a release tag for a
different image.

Kind and live-cluster validation belongs to CI or an operational milestone. A
local unit-test pass does not prove RBAC, API outage, provider outage, or
upgrade recovery behavior.

The `Operational validation` workflow runs a disposable Kind cluster on a
weekly schedule or by manual dispatch. It verifies chart installation,
effective ServiceAccount permissions, an older state-schema migration, health
probes, restart and Helm upgrade retention, a Pod event burst, and recovery
after restarting the disposable control-plane node. An API-only outage, a real
provider outage, and large-cluster capacity still require an operational
environment because Kind cannot reproduce every production network and scale
condition.

For a local run with Docker, Kind, kubectl, and Helm installed:

```sh
docker build -t kwatch:ci .
kind load docker-image kwatch:ci
KWATCH_LOAD_COUNT=100 make verify-operational
```

The security workflow separately runs `govulncheck`, verifies module content,
scans the built image with Trivy, and publishes a CycloneDX SBOM artifact.
