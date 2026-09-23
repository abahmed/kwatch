# Real-cluster regression scenarios

The scenario suite runs the real Kwatch container inside a disposable Kind
cluster. It installs Kwatch from the source manifests in `deploy/` with
`kubectl`; it does not use Helm or the interactive `kwatch.sh` manager.

The installer and Helm chart are separate from the semantic suite. Keeping
packaging paths out of it makes a failed incident, grouping, delivery, or
recovery test an actual Kwatch runtime failure.

## Local run

Install Docker, Kind, kubectl, Go, Bash, and curl. Then run:

```sh
make verify-scenarios
```

Useful filters are:

```sh
SCENARIO_REGEX=TestScenarioPodCrashLoop make verify-scenario
SCENARIO_FAMILY=workload make verify-scenario
KEEP_CLUSTER=true make verify-scenario
ARTIFACTS=/tmp/kwatch-e2e make verify-scenarios
make verify-negative-regressions
```

The script builds temporary images with `docker build --load`, loads them into
Kind, and removes them and the cluster after the run. Images are never pushed
or uploaded.

The manual `scenarios.yml` workflow resolves the latest `main` commit to an
immutable SHA before building and runs the complete scenario suite, including
the extended Kind cases. It accepts a scenario regex, family, shard, and
optional cluster retention for debugging. In compare mode it also accepts a
release tag or commit. The workflow runs that reported source and the latest
`main` in separate Kind clusters and writes one of `fixed_on_main`,
`still_failing`, `regression_on_main`, or `not_reproduced` to the artifacts.
Both image sets are built locally and removed after each cluster run.

## Architecture

Kind owns the real Kubernetes cluster. The Go tests use Kubernetes SIG's
`sigs.k8s.io/e2e-framework` for test lifecycle and client-go for Kubernetes
operations. Kwatch-specific helpers inspect audit logs, webhook requests,
health endpoints, metrics, persistence, and Lease leadership.

The source Deployment and CRD are the production manifests from `deploy/`.
`test/e2e/install/` only changes the candidate image, pull policy, and replica
count for the disposable cluster.

## Adding a scenario

1. Inspect `test/e2e/coverage/coverage.yaml` and choose a stable ID.
2. Confirm that the behavior is not already covered.
3. Add the smallest deterministic fixture under `test/e2e/fixtures/`.
4. Reuse the harness waiters and oracle helpers.
5. Add a Go test under `test/e2e/scenarios/`.
6. Assert expected behavior and forbidden behavior.
7. Repair the fault and assert recovery when recovery is meaningful.
8. Delete the scenario namespace and assert cleanup.
9. Link the GitHub issue in the test or coverage entry.
10. Update the coverage status.

Use watches, receiver notifications, or bounded polling. Do not add arbitrary
sleep calls. A scenario must have a context deadline and must not execute
commands supplied by a GitHub issue.

To reproduce a reported regression, copy only the minimum non-secret
configuration and resources into a new committed scenario. Replace external
images with the local workload image, review every resource, and never execute
issue content directly.

Issue input may be staged through the safe parser, but it is not a test
workflow or an execution mechanism. Only these marked blocks are accepted:

~~~markdown
<!-- kwatch-config -->
```yaml
app:
  clusterName: reproduced
```
<!-- kwatch-resources -->
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: reproduced
```
<!-- kwatch-expectation -->
The Pod should produce one incident and one recovery.
~~~

`test/e2e/issue` rejects commands, URLs, credentials, Secrets, privileged
resources, host mounts, and cluster-scoped RBAC. It rewrites namespaces and
workload images to the disposable local values. The sanitized output still
requires human review before it becomes a permanent scenario.

To prepare that output without executing anything, run:

```sh
go run ./cmd/kwatch-e2e-issue \
  --issue-file /path/to/issue-body.txt \
  --output-dir /tmp/kwatch-issue-123 \
  --namespace kwatch-issue-123 \
  --image kwatch-e2e-workload:test
```

This writes `config.yaml`, sanitized `resources.yaml`, `expectation.txt`, and
`metadata.json`. It does not create a cluster, pull an image, execute issue
commands, or apply the output. Review the files, copy only the required
declarative fixture into a permanent scenario, then add positive, negative,
recovery, and cleanup assertions.

Every supported Kwatch monitor must have coverage for relevant lifecycle
profiles: startup failure, delayed failure, one-shot failure, recurring
failure, simultaneous failures, grouping, shared-node impact, and recovery.

## Extended Kind coverage

The same manual workflow installs metrics-server in the disposable Kind
cluster before running scenarios that need APIs not present in a base Kind
cluster. Its release manifest is transiently downloaded, pinned by version and
SHA-256, and removed during cleanup. The image exists only in the disposable
Kind node; it is never pushed, saved, or uploaded. It covers:

- Metrics API and HPA failure;
- CSI `VolumeAttachment` failure;
- validating admission webhooks and policies;
- expired TLS certificates.

The workflow builds and loads temporary local images into Kind. It does not
push, upload, or retain those images. It collects resources, events, logs, and
test results. Use `keep_cluster=true` for debugging; the cluster and temporary
images are removed automatically otherwise.

## Diagnostics

Failed runs retain Kubernetes objects, events, Nodes, Leases, Kwatch logs,
audit records, health responses, metrics, receiver requests, and Kind node
logs under `ARTIFACTS`. Tokens and Secret data must never be written there.
