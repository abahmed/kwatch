# ⬆️ Upgrading from v0.10.x to v1.0.0

Use this guide when you move a running kwatch v0.10.x install to v1.0.0. It
lists what you must change before the upgrade, what kwatch does on its first
start, and how to go back. It describes the difference between the v0.10.5
chart, manifests and configuration and the v1.0.0 ones; the release notes in
[`RELEASES.md`](../RELEASES.md) carry the full change list.

If you ran a `v1.0.0-rc.N` release, read
[Coming from a 1.0 release candidate](#coming-from-a-10-release-candidate)
as well. Metrics and diagnostic endpoints were added and changed during the
candidates, so they do not affect a v0.10.x install.

## What changes at a glance

| Area | v0.10.x | v1.0.0 |
| --- | --- | --- |
| State | In memory; lost on every restart | bbolt file on a PVC mounted at `/var/lib/kwatch` |
| Replicas | Deployment default strategy | One replica, `Recreate` strategy, a Lease as the state lock |
| Credentials | Inline in the config | `${file:/absolute/path}` references only |
| Endpoints | `/healthz`, `/health` | Adds `/readyz`, `/availabilityz`, `/metrics` and an opt-in `/status` |
| RBAC | One ClusterRole | Broader read access, Roles for the Lease, restart evidence and `kwatchconfigs` |
| Memory | 128Mi limit | 512Mi limit, `GOMEMLIMIT` at 90% of it |
| Custom config | None | `KwatchConfig` resource (`kwatch.abahmed.dev/v1beta1`) |

## Before you upgrade

1. **Move credentials to a Secret.** kwatch rejects plain provider
   credentials and heartbeat URLs. Put the values in files in a Secret and
   reference them as `${file:/config/<name>}`. With the chart, `config.alert`
   and `config.heartbeatMonitor.url` fail to render unless
   `configSecretName` is set; the Secret's `config.yaml` is then the whole
   base configuration. See the
   [chart README](../deploy/chart/README.md) and the
   [configuration reference](./configuration.md).
2. **Clean up your configuration.** Run `kwatch lint` (add `--strict` to
   treat unknown keys as errors) against the new `config.yaml`. See
   [Configuration](#configuration).
3. **Check your chart values.** See [Helm chart values](#helm-chart-values).
4. **Check storage.** The chart creates a 2Gi `ReadWriteOnce` claim by
   default. Make sure your cluster has a default StorageClass or set
   `persistence.storageClass`. See "Volume requirements" in
   [production operations](./production-operations.md).
5. **Check network access.** kwatch now reads node stats directly from each
   kubelet (the node `InternalIP`, port 10250 by default) instead of through
   the API server proxy. If you run your own NetworkPolicy, allow egress from
   kwatch to TCP 10250 on every node.
6. **Verify the release.** See [release integrity](./release-integrity.md).

## Configuration

These v0.10.x keys no longer exist. kwatch does not stop on an unknown key:
it logs one warning per key with its line and full path, so check the startup
log after the upgrade. `kwatch lint --strict` rejects them instead.

| v0.10.x key | In v1.0.0 |
| --- | --- |
| `pvcMonitor.*` | Not a configuration key |
| `nodeMonitor.*` | Not a configuration key |
| `maxRecentLogLines` | Not a configuration key |
| `ignoreFailedGracefulShutdown` | Not a configuration key |
| `ignoreLogPatterns` | Not a configuration key |
| `ignoreContainerNames`, `ignorePodNames`, `ignoreNodeReasons`, `ignoreNodeMessages` | Still read, but deprecated: each becomes one silence rule. Use `silences` |

The silence rules (`silences`) take `containerNames`, `containerMessages`,
`eventMessages`, `nodeReasons` and `nodeMessages`; see the
[configuration reference](./configuration.md).

Behaviour that now fails startup or `kwatch lint`, where v0.10.x ran:

- A `CONFIG_FILE` that is set but missing. Before, kwatch ran on defaults.
- A plain credential in a provider option or heartbeat URL (see above).
- `resyncSeconds` between 1 and 29. Use `0` or at least `30`.
- `app.logFormatter` other than `text` or `json`.
- An unparsable Go template in `templates`, a `runbooks` value that is not an
  absolute `http` or `https` URL, and an `alert.<provider>.fallback` that
  names a provider that is not configured.
- An invalid `app.proxyURL` or an unreadable `app.caBundlePath`.
- A route severity other than `critical`, `warning` or `info`.
- A blank entry in a silence field or an `ignore*` list.
- A boolean environment variable (`KWATCH_WATCH_SECRETS`,
  `KWATCH_CRD_ENABLED`, `KWATCH_TELEMETRY`) with a value other than
  `true`/`false`, `1`/`0`, `on`/`off` or `yes`/`no`.
- `alert.zulip.url` unset. It is required.

Other changes that can surprise you:

- `$${NAME}` now yields the literal text `${NAME}`. Before, it produced `$`
  followed by the value of `NAME`.
- Provider `title` and `text` options for chat providers no longer change
  anything, because kwatch writes the incident message. They are ignored with
  a startup warning; remove them.
- `alert.line` (LINE Notify) is ignored with a warning. The service shut down
  on 31 March 2025; use another provider.
- ntfy: `alert.ntfy.url` must be the server address; the topic goes in
  `alert.ntfy.topic`.
- Adoption telemetry is on by default in official builds. It sends an
  anonymous installation ID and the kwatch version. Turn it off with
  `telemetry.enabled: false` or `KWATCH_TELEMETRY=false`.

## Alert keys: close open alerts by hand

Provider deduplication keys and aliases are now `kwatch-<cluster>-<incident
key>`. An alert that v0.10.x opened under an old key is not found when its
incident resolves, so it is not closed automatically. Close open alerts in
Opsgenie, PagerDuty and other keyed providers by hand during the upgrade.
The `alertKey` sent by the webhook, Zapier and n8n providers and the
`X-Kwatch-Alert-Key` email header change value too; update any automation
that matches on them.

PagerDuty, Opsgenie and the other paging tools, and the issue trackers, no
longer receive the startup summary or plain notices; they receive incidents
only.

## Endpoints

kwatch serves these endpoints and no others:

| Endpoint | Notes |
| --- | --- |
| `/healthz` | Liveness. Existed in v0.10.x. |
| `/health` | Component state and reason codes. Existed in v0.10.x. |
| `/readyz` | New. Monitoring readiness. |
| `/availabilityz` | New. Used by the chart's Deployment probes. |
| `/metrics` | New. Prometheus metrics. |
| `/status` | New and off by default. Set `healthCheck.statusEndpoint: true`. |

The health server cannot be turned off in the chart: it fails to render when
`config.healthCheck.enabled` is false. The port is still `8060` by default.
The `/status` view shows object names and has no authentication, so reach it
with `kubectl port-forward` and never expose the port.

## Metrics

v0.10.x did not expose a `/metrics` endpoint, so there is nothing to rename
for dashboards you built against it. If you scrape the new endpoint, the
list of metrics is in the [configuration reference](./configuration.md).

## Upgrading the Deployment

The installer and `helm upgrade` (with your 0.10.x values file; the old
`service` block is ignored) upgrade in place. `deploy/deploy.yaml` now
selects pods by `app: kwatch` and `app.kubernetes.io/instance: kwatch`,
and a Deployment selector cannot change, so `kubectl apply` over a 0.10.x
install fails. Delete the Deployment first:

```sh
kubectl delete deployment kwatch -n kwatch
kubectl apply -f deploy/deploy.yaml
```

## RBAC and the CRD

The chart and `deploy/deploy.yaml` replace the old ClusterRole.

- The chart's `rbac.mode` is `least-privilege` by default: an explicit
  `list`/`watch` set for the kinds kwatch understands. Kinds outside it,
  custom resources included, show as `permission_denied` in `/health` and
  their detectors stay silent. To watch custom resources and kinds added
  later, opt back into the wildcard with `--set rbac.mode=full` (`list`
  and `watch` on every resource in every API group). The raw
  `deploy/deploy.yaml` manifest keeps the `full` role.
- `get` is granted only on `namespaces`, `nodes`, `nodes/stats`,
  `nodes/metrics` and `pods/log`. `nodes/proxy` is no longer granted.
- Secrets are watched by default, which lets the ServiceAccount read every
  Secret in the cluster (values are hashed and never stored). Set
  `watch.secrets: false` to remove all Secret permissions.
- Namespaced Roles add `get` on Pods in kwatch's own namespace (to explain a
  restart), Lease access limited by name to kwatch's own Lease, `get` on
  Leases in `kube-system`, and `list`/`watch` on `kwatchconfigs`.
- If you maintain your own RBAC, compare it with `deploy/deploy.yaml`
  instead of the v0.10.x rules.

The `KwatchConfig` custom resource
(`kwatchconfigs.kwatch.abahmed.dev`) is new in this line. Its stored version
is `v1beta1`; write new manifests with `apiVersion:
kwatch.abahmed.dev/v1beta1`. `v1alpha1` is still served but deprecated:
existing `v1alpha1` objects keep working and print a deprecation warning
when used. There is no conversion webhook, so switch the `apiVersion` in
your manifests when convenient. It lets you store non-sensitive
configuration in the cluster; provider settings and heartbeat URLs are
forbidden in it. When it changes, kwatch restarts to apply it. The CRD
carries `helm.sh/resource-policy: keep`, so Helm leaves it in place on
uninstall. Because the CRD is cluster-wide, only one release can own it: set
`crd.install: false` on every other release, and when you render offline
(`helm template`, Argo CD) or apply `deploy/crd.yaml` yourself. To run
without the overlay, set `config.crd.enabled: false`.

## State store and first start

v0.10.x kept state in memory. v1.0.0 keeps it in `state.db` on the volume, so
the first start behaves like a cold start:

- The chart creates the claim `<release>-data` (2Gi by default), with
  `helm.sh/resource-policy: keep` so it survives uninstall. The Pod stays
  `Pending` until the claim binds.
- There is no state to migrate. kwatch learns baselines from scratch and sends
  one startup summary. Problems that already exist when it starts are listed
  in that summary instead of being announced one by one to chat channels.
- A file from a later v1.0 build with another schema version is moved aside
  as `state.db.corrupt.<UTC time>` and a fresh store is created. The delivery
  outbox and provider thread IDs are carried over after a schema change; the
  newest two copies are kept. This shows as `storage_reset` on `/health`.
- The store is capped at 512 MiB of logical data on the default 2Gi volume.
  Past the cap the oldest history is dropped first; open incidents, baselines
  and the outbox are not.
- `persistence.emptyDir: true` works but loses state whenever the Pod is
  rescheduled, and `helm install` warns about it.

Delivery is at least once. Queued notifications are written to a persisted
outbox and re-sent after a restart, so an interrupted send can repeat.

See [production operations](./production-operations.md) for sizing,
retention and recovery.

## Helm chart values

The chart now rejects unknown keys at the root and in `rbac`, `watch`,
`persistence`, `networkPolicy`, `config.kubelet` and `config.watch`, so a
stale values file fails at `helm upgrade` instead of being ignored. Compare
your file with `values.yaml` before upgrading.

| Value | Change |
| --- | --- |
| `service.port` | Removed. The container and probe port come from `config.healthCheck.port` |
| `image.pullPolicy` | Default is now `IfNotPresent` (was `Always`) |
| `resources` | Limits are now 512Mi memory and 500m CPU; requests 128Mi and 100m. Raising memory above the default is covered by the sizing table in [production operations](./production-operations.md) |
| `config.alert.*` | Needs `configSecretName`; the Secret's `config.yaml` replaces `.Values.config` |
| `persistence.*` | New: `size`, `storageClass`, `existingClaim`, `emptyDir`, `emptyDirSizeLimit`, `pinToStorageDriverNodes`, `storageDriverTopologyKey`. `emptyDir` and `existingClaim` cannot both be set |
| `rbac.mode`, `watch.secrets` | New. See [RBAC and the CRD](#rbac-and-the-crd) |
| `crd.install` | New, default `true` |
| `networkPolicy.*` | New and off by default. With it on, `/health`, `/metrics` and `/status` accept only the release namespace, `networkPolicy.kubeletCIDRs` and `networkPolicy.healthIngressFrom` |
| `podAnnotations` | Defaults now carry `karpenter.sh/do-not-disrupt` and `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"`. Setting your own replaces them |
| `tolerations` | Merged with the new `defaultTolerations` (30-second `not-ready` and `unreachable` tolerations) |
| `terminationGracePeriodSeconds` | New, default 60 |

The Pod also runs with a `RuntimeDefault` seccomp profile, no privilege
escalation, all capabilities dropped and `fsGroup: 1000`. Namespaces that
enforce the `restricted` Pod Security level accept it.

Upgrade with:

```shell
helm repo update
helm upgrade kwatch kwatch/kwatch \
  --namespace kwatch \
  --values values.yaml
```

The chart's ClusterRole and ClusterRoleBinding are named
`<fullname>-<namespace>`; Helm replaces the old objects on upgrade.

## Verify

After the upgrade:

1. `kubectl -n kwatch get pods,pvc` shows the Pod `Running` and the claim
   `Bound`.
2. `/readyz` succeeds on the Pod's health port.
3. `/health` has no `permission_denied` or `kubelet_unreachable` entries you
   did not expect.
4. The startup log has no `not recognized` configuration warnings.
5. The first startup summary arrives in your channels.

## Rolling back

v0.10.x kept its state in memory and does not read `state.db`, so a rollback
loses only what v1.0.0 learned.

```shell
helm rollback kwatch --namespace kwatch
```

- The claim, the `KwatchConfig` CRD and any `KwatchConfig` objects are kept.
  Delete the claim yourself if you do not plan to upgrade again.
- Use the values file you used with v0.10.x, not the v1.0.0 one: the older
  chart does not know the `persistence`, `rbac` or `crd` values.
- Alerts opened under the new `kwatch-<cluster>-<key>` keys are not known to
  v0.10.x. Close any that stay open by hand.
- Between 1.0 builds, a state file with another schema version is moved aside
  and the outbox and thread IDs are carried over, so a rollback inside the
  1.0 line does not drop queued pages. Issue trackers save a resolved issue
  in a new format; an older 1.0 build reads it as an unknown issue ID, gets a
  404 and forgets it, with at most one failed call per issue.

## Coming from a 1.0 release candidate

If you already run a `v1.0.0-rc.N` build, these changes apply to you:

- **Endpoints.** The diagnostics endpoints are gone, including the incident
  list that was renamed from `/problems` to `/incidents` and then removed.
  Only the endpoints in [Endpoints](#endpoints) remain.
- **Metrics.** `kwatch_incidents_active` is now `kwatch_incidents_open`, and
  `kwatch_notifications_total` is now `kwatch_delivery_notifications_total`.
  The `kwatch_apiserver_*`, `kwatch_graph_*`, `kwatch_insight_*`,
  `kwatch_persistence_*`, `kwatch_queue_depth`, `kwatch_group_size` and
  several other internal metrics were removed. The full list of removed and
  new metrics is under "Breaking changes" in
  [`RELEASES.md`](../RELEASES.md); update dashboards and alert rules.
- **Feature IDs.** `problems.*` capability IDs are now `incidents.*`.
- **Chart.** `kwatchconfigs` list/watch moved from the ClusterRole to a
  namespaced Role in kwatch's namespace; copy that change if you maintain
  your own RBAC. The schema now rejects `resyncSeconds` from 1 to 29, an
  invalid `app.logFormatter`, negative heartbeat or active-probe numbers and
  an empty `networkPolicy.apiServerPorts`.
- **Config.** `activeProbeMonitor.recoveryThreshold` was removed.
  `KwatchConfig` cannot carry `healthCheck`.
- **State.** A state file written by an earlier candidate is moved aside at
  the first start if its schema version differs; open incidents are
  announced once more. Delete `state.db.corrupt.*` copies when you no longer
  need them.
