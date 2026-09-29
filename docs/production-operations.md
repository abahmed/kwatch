# Production operations

This page describes the supported operating model for Kwatch. It is an
operations reference, not a promise of high availability.

## Operating model

Kwatch runs one replica with the `Recreate` strategy. State lives in a bbolt
file on a PVC mounted at `/var/lib/kwatch`. A Kubernetes Lease is used only as
a lock: it stops two processes from writing the volume at once, and its
transition count fences the state file so a process that lost the Lease cannot
overwrite newer state. There is no standby Pod and no Kwatch self-failover; if
the Pod or its node fails, Kubernetes restarts it and Kwatch resumes from the
volume.

Initial operating targets are: readiness within 120 seconds after a healthy
startup and graceful shutdown within 45 seconds. These are SLO targets for
capacity planning and alerting, not guarantees. Queue limits and configured
worker counts remain the authority for memory and delivery capacity.

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
- `/availabilityz` reports whether the Pod is participating in the application
  lifecycle and is used for Deployment rollouts. It is not monitoring
  readiness.
- `/health` reports optional monitor degradation and safe reason codes.

An absent optional Kubernetes API is reported as degraded and does not create a
synthetic problem. `/readyz` succeeds only when the Pod holds the Lease, has
claimed the state file, every source finished its initial list, and delivery is
running.

## Shutdown and recovery

Kwatch gives its components a bounded shutdown window. On termination the
sources stop with the engine, the final problem snapshot is written, and the
state file is closed. If a dependency does not stop within its deadline,
Kwatch records a shutdown timeout and exits rather than waiting forever.

The deployment provides a 60-second termination grace period. A required
component failure (delivery or the core) or an internal stall makes the Pod
unready, cancels the active session, and lets Kubernetes restart it. Optional
component failures remain visible as bounded degraded health states and use
bounded restart backoff; they do not create synthetic problems.

After a restart the core restores open problems from the state file, so they
are not announced again, and waits ten minutes before it may recover a
restored problem so detectors can re-raise their signals. Once every source has
finished its initial list, Kwatch compares the saved object fingerprints with
the live cluster and records changes made while it was down, so root-cause
analysis can still find a rollout that happened during the gap. Kubernetes
Events that expired while Kwatch was unavailable cannot be reconstructed.

Back up the PVC according to the operator's normal volume backup policy. The
state file is created with owner-only permissions and can be removed to reset
Kwatch: it then starts cold and sends one startup summary. A state file written
by a newer Kwatch is not opened; roll forward, or reset the file, as described
in the release notes. Store failures are reported through health diagnostics.

## Outages and capacity

Kubernetes API and provider outages are handled through bounded retries and
degraded status. Delivery queues are bounded; sustained saturation coalesces or
summarises notifications and can drop them, so operators should alert on queue saturation, terminal
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
effective ServiceAccount permissions, health probes, restart and Helm upgrade
retention, a Pod event burst, and recovery
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
