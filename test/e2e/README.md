# Real-cluster regression scenarios

The scenario suite runs the real Kwatch container inside a disposable Kind
cluster. It installs Kwatch from the source manifests in `deploy/` with
`kubectl`; it does not use Helm or the interactive `kwatch.sh` manager.

The installer and Helm chart are separate from the semantic suite. Keeping
packaging paths out of it makes a failed incident, grouping, delivery, or
recovery test an actual Kwatch runtime failure.

## Writing a scenario

A scenario is a short story: create something broken, say what Kwatch must
announce, fix it, say what must happen next. Use `inNamespace` (or
`onCluster` for nodes, webhooks and other cluster-wide objects). It creates
a namespace, cleans it up and stops the test on any error, so the scenario
has no error handling:

```go
func TestScenarioResolution(t *testing.T) {
	inNamespace(t, "lifecycle.resolution", func(s *Scenario) {
		s.CreateDeployment("recovery", "healthy",
			withConfigMapEnv("settings"))
		s.ExpectIncident("settings", "ProjectedConfigMapMissing", 0)

		s.FixMissingConfigMap("recovery", "settings")
		s.ExpectResolved("recovery")
	})
}
```

Every helper is a method on `*Scenario` or a plain builder that returns a
Kubernetes object, so a scenario never passes a context, client or
namespace. Helpers that create things sit in `*_build_test.go`, named after
what they build: `pods_build_test.go`, `workloads_build_test.go`,
`config_build_test.go`, `node_build_test.go`, `cluster_build_test.go` and
`kwatch_build_test.go`. Builders are named after the object
(`crashingPod`, `failingJob`, `suspendedCronJob`) and `Create*` methods
create it (`s.CreatePod(pod)`, `s.CreateDeployment(name, mode, options...)`).

Rules:

1. Copy a nearby scenario; keep the `TestScenario<Behavior>` name and the
   scenario ID in `coverage/coverage.yaml`.
2. Wait only with the `Expect*` helpers and `timing_test.go`. Never write a
   raw `time.Minute` wait. Pass the detector's sustain time (see
   `internal/detection/detectors`) to `ExpectIncident`.
3. Name the object Kwatch blames, not the object you created. An incident is
   rooted at the root cause: a missing Secret, the Service behind an
   Ingress or an APIService can be the root rather than the Pod you made.
4. Do not read the audit log or webhook payload directly. Use the helpers in
   `scenarios/scenario_test.go` and `harness/`.
5. A state Kwatch deliberately ignores, or one Kind cannot reproduce, is not
   an E2E scenario. Test it with a replay scenario in `internal/scenarios`.
6. Pick how the scenario shares the cluster:
   - `inNamespace` runs it at the same time as other scenarios. Use it when
     everything it creates lives in its own namespace.
   - `inNamespaceAlone` runs it while nothing else runs. Use it when it
     stops a node, deletes or restarts Kwatch, changes the webhook
     receiver, or breaks something cluster-wide such as an APIService or
     an admission webhook.
   - `onCluster` also runs alone, for scenarios without a namespace.
   When unsure, use `inNamespaceAlone`: it is slower but never flaky
   because of another scenario.
7. If the scenario takes more than a few minutes, set `minutes:` on its
   entry in `coverage/coverage.yaml`, so CI can spread slow scenarios over
   the shards. Every CI run prints the measured time of each scenario.
8. Keep a scenario under about 30 lines. Move object building into the
   matching `*_build_test.go` file as a builder plus a `Create*` method, and
   reuse `workloadContainer`, `workloadPod` and `deployment` instead of
   writing a new container or Deployment.

## Local run

Install Docker, Kind, kubectl, Go, Bash, and curl. Then run:

```sh
make verify-scenarios
```

Useful filters are:

```sh
SCENARIO_REGEX=TestScenarioPodCrashLoop make verify-scenarios
SCENARIO_FAMILY=workload make verify-scenarios
KEEP_CLUSTER=true make verify-scenarios
ARTIFACTS=/tmp/kwatch-e2e make verify-scenarios
make verify-negative-regressions
```

The script builds temporary images with `docker build --load`, loads them into
Kind, and removes them and the cluster after the run. Images are never pushed
or uploaded.

The `e2e.yml` workflow (nightly, manual, or on PRs labelled `e2e`) resolves the
latest `main` commit to an immutable SHA before building and runs the complete
scenario suite, including the extended Kind cases. A full run is split over
four Kind clusters that run in parallel. Tests are dealt to the clusters
longest first using the `minutes:` in `coverage/coverage.yaml`, so every
cluster gets about the same work. Inside a cluster the scenarios that must
run alone go first, then the rest run side by side (six at a time; set
`SCENARIO_PARALLEL` to change it). A run with a scenario, family, shard or
compare filter uses one cluster. It accepts a scenario regex,
family, shard, and optional cluster retention for debugging. In compare mode it
also accepts a release tag or commit. The workflow runs that reported source
and the latest `main` in separate Kind clusters and writes one of
`fixed_on_main`, `still_failing`, `regression_on_main`, or `not_reproduced` to
the artifacts. Both image sets are built locally and removed after each cluster
run.

## Architecture

Kind owns the real Kubernetes cluster. The Go tests use Kubernetes SIG's
`sigs.k8s.io/e2e-framework` for test lifecycle and client-go for Kubernetes
operations. Kwatch-specific helpers inspect audit logs, webhook requests,
health endpoints, metrics, persistence, and the Lease holder.

The source Deployment and CRD are the production manifests from `deploy/`.
`scripts/test-kind-scenarios.sh` substitutes only the candidate image and
pull policy before applying the Deployment to the disposable cluster. Kwatch
runs as one replica.

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

`test/e2e/issue` accepts only an allowlist of namespaced workload fixture
kinds: Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, CronJob,
Service, ConfigMap, Ingress, HorizontalPodAutoscaler, PodDisruptionBudget,
PersistentVolumeClaim, NetworkPolicy, ServiceAccount and HTTPRoute. Every
other kind is rejected, including CRDs, webhook configurations, StorageClass,
PriorityClass, APIService, Roles and Secrets. It also rejects URLs,
credentials, privileged containers, host ports, host namespaces, host mounts,
added capabilities, `runAsUser: 0` and service account tokens. It removes
`command` and `args` from every container, because the image is replaced and
issue commands must never run, and rewrites namespaces and workload images to
the disposable local values. The sanitized output still requires human review
before it becomes a permanent scenario.

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

## Root-cause assertions

Failure scenarios assert the incident root, not only a reason. A scenario
passes a `harness.RootExpectation` (same shape as the `expect.json` files in
`internal/scenarios/testdata`) to `assertRoot`, which reads Kwatch's audit log
and requires the expected root (`kind/namespace/name`, group roots such as
`registry//host` or `node//name`), the expected tier, at most `maxMessages`
for that incident, optionally `maxTotalMessages` for the whole scope, and no
incident rooted at any `mustNotBlame` entity. A trailing `*` on a name
matches a prefix. The evaluation logic is unit-tested without a cluster in
`harness/rootcause_test.go`. Use a reason-only wait only when the root is the
failing object itself and nothing else could be blamed.

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
