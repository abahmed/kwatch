<p align="center">
  <a href="https://kwatch.dev">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="./assets/logo-dark.svg">
      <img src="./assets/logo-light.svg" width="260" alt="kwatch">
    </picture>
  </a>
</p>

<h1 align="center">Fifty alerts. One cause.</h1>

<p align="center">
  kwatch runs inside your Kubernetes cluster, traces every failure back to
  what started it, and sends one message: what broke, why, who changed it,
  and what to do next.
</p>

<p align="center">
  <a href="https://github.com/abahmed/kwatch/releases"><img alt="Release" src="https://img.shields.io/github/v/release/abahmed/kwatch?sort=semver"></a>
  <a href="https://github.com/abahmed/kwatch/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/abahmed/kwatch/actions/workflows/ci.yml/badge.svg"></a>
  <a href="./LICENSE"><img alt="License: ELv2, source available" src="https://img.shields.io/badge/license-ELv2%20(source%20available)-blue"></a>
  <a href="https://artifacthub.io/packages/helm/kwatch/kwatch"><img alt="Artifact Hub" src="https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kwatch"></a>
</p>

<p align="center">
  <a href="https://kwatch.dev">kwatch.dev</a> ·
  <a href="https://docs.kwatch.dev">Docs</a> ·
  <a href="https://kwatch.dev/pro">Pro, free during early access</a> ·
  <a href="https://discord.gg/kzJszdKmJ7">Discord</a>
</p>

```bash
/bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

Paste one line, answer a few questions. Run it again any time to upgrade,
change settings or manage your license.

---

## One incident instead of fifty alerts

It is 03:07. A release went out, pods are crash looping, a Service has no
endpoints, and two other apps fail their health checks. Every alert is true.
None of them says why.

kwatch sends this instead (an illustrative message):

```text
kwatch · 1 incident                                    ROOT CAUSE FOUND

checkout-api is crash-looping because a ConfigMap changed 4 minutes ago.

Why     DB_HOST points to a host that doesn't resolve
Who     deploy-bot, via Argo CD
Impact  payments, cart
Next    Roll back checkout-config
```

When it is fixed, you hear about it once, and then it stays quiet.

Messages look like this in your channel. This one is real kwatch output:

```text
🔴 api is down in shop (prod-eu-1)
after the 14:02 release of api:2.3. alice
released it; the previous image was
api:2.2. Only pods of the new
revision fail. Service api and
ingress storefront can't serve traffic.
checkout is affected as well. Rolling
back fixes it (changes the cluster):
kubectl rollout undo deployment/api
-n shop --to-revision=13
```

## Why kwatch

- **Root cause first.** kwatch follows a failure upstream through
  everything it depends on and names the one cause: a bad release, a full
  node, an expired certificate, a database outside the cluster. Symptoms
  join one incident.
- **Quiet by default.** No repeats every five minutes and no flapping
  storms. kwatch waits for failures to settle, merges what belongs together
  and speaks when there is news. Successful rollouts, scaling and node
  drains never wake anyone.
- **Who changed what.** Rollouts, config edits and image updates sit on a
  timeline with who made them, so a message can say "after alice's 14:02
  release".
- **The next step.** A message ends with the command to run: read the crash
  log, describe the node, or roll back.
- **50+ channels.** Slack, Microsoft Teams, Discord, PagerDuty, Opsgenie,
  email, Jira, webhooks and more. Paging tools get a stable key, so the same
  problem updates the same page and closes it when it is fixed.
  [All channels](https://kwatch.dev/docs/channels).
- **Your data stays in your cluster.** One pod and a small volume for state.
  There is no kwatch cloud reading your logs.

## How it works

1. **Watch.** kwatch keeps a live map of your cluster: pods, workloads,
   nodes, volumes, configs, services and certificates, how they connect,
   and every change made to them, with who made it.
2. **Trace.** When something fails, it follows the links upstream and keeps
   only suspects that are broken or changed just before. It prefers the
   cause with stronger evidence over the one that covers more failures.
3. **Tell.** One message per cause, written for people. It is updated only
   when something new is true, and closed once when it is resolved.

An example trace, from symptom to cause:

```text
Pod checkout-api-7f9c          CrashLoopBackOff
  Deployment checkout-api      healthy before 03:03
    Node worker-3              healthy
      ConfigMap checkout-config  changed 03:03   <- cause
```

An incident over time:

```text
03:07  Opened: checkout-api crash-looping, cause found
03:11  Update: cart-svc now affected too
03:16  Fix detected: checkout-config rolled back
03:21  Resolved after 14 minutes
```

## What it catches

| | |
| --- | --- |
| **Pods** | Crash loops, OOM kills, image pulls, Pending |
| **Nodes** | NotReady, pressure, draining |
| **Workloads** | Stuck rollouts, failed Jobs, missed CronJobs |
| **Storage** | Pending claims, attach failures, full volumes |
| **Network** | Services with no endpoints, Ingress, policies |
| **Certificates** | Expiring and invalid TLS |
| **Control plane** | API server, etcd, DNS, webhooks |
| **Custom resources** | Failing conditions on CRDs |

Built in, no rules to write. Community covers pod failures and NotReady
nodes; Pro covers everything. The full list is in the
[coverage guide](./docs/kubernetes-coverage.md).

## Install

**Installer (recommended).** It checks your cluster before it changes
anything, asks where alerts should go, stores credentials in a Kubernetes
Secret, installs kwatch and checks that it is running.

```bash
/bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

**Helm.** For GitOps or if you manage the cluster with Helm:

```bash
helm repo add kwatch https://kwatch.dev/charts
helm repo update
helm upgrade --install kwatch kwatch/kwatch \
  --namespace kwatch --create-namespace
```

The chart defaults to `rbac.mode: least-privilege`, an explicit read set for
the kinds kwatch watches. To also watch custom resources and kinds added
later, add `--set rbac.mode=full`. See the
[chart guide](./deploy/chart/README.md) for the full install, values and
the Secret-based configuration, and [kwatch.sh](./docs/kwatch-sh.md) for what the installer does.

## Editions

One binary, two editions. Community needs no account and no key. Pro is free
during early access.

| | Community | Pro |
| --- | --- | --- |
| Detection | Pod crash loops, OOM kills and Pending pods; node NotReady | Adds node pressure and draining, readiness, rollouts, Jobs and CronJobs, PDBs, HPAs, storage, network, admission, certificates, quota, custom resources, control plane, probes and unusual events |
| Root cause | One-line headline | Full proof and confidence, shared-factor suspects, recent changes and who made them, affected services, log excerpts |
| Channels | 1 provider (pick with `app.communityProvider`), namespace filters and silences | All providers, routing by scope, threads, text templates, audit log |
| Noise control | Dedup and settling, state in memory | Flapping detection, startup summary, state kept on disk across restarts, changes made while kwatch was down |
| Operations | RBAC audit, secret redaction, health, metrics, text `/status` | Severity overrides, maintenance holds, runbook links, heartbeat monitor, `/status` JSON, flight recorder and `replay-export`, standby replicas |
| Account | None | Email only |

When a Pro license ends, kwatch falls back to Community. Alerts never stop.
The authoritative list is the catalog in [`internal/feature`](./internal/feature),
and the comparison is on [kwatch.dev/pro](https://kwatch.dev/pro).

### Get Pro

1. Sign in at [app.kwatch.dev](https://app.kwatch.dev) with your email and
   press **Get a free Pro license**. Your key appears right away.
2. Run the installer, open its **License / Pro** menu and choose **Add or
   replace key**.

Licenses are managed only through the installer. For a non-interactive
install, set `KWATCH_LICENSE_KEY` in the environment of the same command.

- One key runs one cluster. Move it to another up to 3 times in 30 days.
- kwatch checks in once a day and keeps Pro for up to 7 days without a
  connection.
- The license state is in `/status` and in the installer's **License / Pro**
  menu.

Details: [licensing](./docs/licensing.md).

## What leaves my cluster

- **Your alerts**, to the channels you configure. Secrets and tokens are
  redacted before anything is stored or sent.
- **An anonymous weekly ping**: a hashed cluster ID and the kwatch version.
  It is separate from licensing. Turn it off with `telemetry.enabled: false`.
- **With Pro**, a daily license check sends a hashed cluster ID, the kwatch
  version and the node count. Community without a key makes no license
  calls.

Nothing from your workloads. Separately, kwatch asks the GitHub API at
startup and then daily whether a newer release exists, and posts one notice
per release to your channels; turn that off with
`upgrader.disableUpdateCheck: true`. Both settings sit under `config:` in
the chart values.

## FAQ

<details>
<summary><b>Does it replace Prometheus or Grafana?</b></summary>

No. Keep your metrics. kwatch answers a different question: when something
breaks, what caused it and what to do. It needs no rules and works next to
them.
</details>

<details>
<summary><b>What access does it need?</b></summary>

Read access to the resources it watches. It writes only its own state lock
Lease, its state file, its license instance key Secret in its own namespace,
and permission self-checks that are not persisted. It never gets `pods/exec`
or `nodes/proxy`.

By default kwatch can read Secrets, to catch missing references and expiring
certificates; it keeps only hashes, never values. Set `watch.secrets: false`
(Helm: `--set watch.secrets=false`) to remove Secret access, except the
license instance key.
</details>

<details>
<summary><b>What happens when kwatch restarts?</b></summary>

With Pro, it picks up where it left off: open incidents are not announced
again and changes made while it was down still explain failures. Community
keeps state in memory, so after a restart it takes a fresh baseline and
alerts on new failures or ones that get worse. See
[production operations](./docs/production-operations.md).
</details>

<details>
<summary><b>Can I tune what it sends?</b></summary>

Yes: namespace filters, silences, maintenance annotations, severity
overrides, runbook links and a message budget per channel. See the
[configuration guide](./docs/configuration.md).
</details>

<details>
<summary><b>Why is Pro free?</b></summary>

We want kwatch proven on real clusters before we charge anything. Each
license is free for 3 months and we may renew it, with up to 2 clusters per
account or company. There are no payment details. If that changes, you will
hear well in advance.
</details>

## Tested like a product

Every change is replayed against a library of real failure scenarios, plus a
hidden set nobody tunes for, and the build fails if it gets worse. CI checks
that kwatch names the right cause, never alerts on normal operations, and
stays small. See the
[latest scorecard](./internal/scenarios/SCORECARD.md).

## Contributing

Report a bug, suggest an idea or send a pull request. Start with
[CONTRIBUTING.md](./CONTRIBUTING.md) and the
[plain-English architecture guide](./docs/contributor-architecture.md), then
follow a step-by-step guide to add a
[detector](./docs/contributing-detector.md), a
[propagation rule](./docs/contributing-propagation-rule.md) or a
[scenario](./docs/contributing-scenario.md).

## Security

Report vulnerabilities privately, as described in
[SECURITY.md](./SECURITY.md).

## Community

[Open an issue](https://github.com/abahmed/kwatch/issues) ·
[Join Discord](https://discord.gg/kzJszdKmJ7) ·
[Code of conduct](./CODE_OF_CONDUCT.md)

## License

kwatch is source available under the [Elastic License 2.0](./LICENSE)
(ELv2) since the release after `v1.0.0-rc.10`. You may use it for free in
your own clusters, read and change the code, and share it. You may not offer
kwatch as a hosted service or remove its license-key checks. Releases up to
and including `v1.0.0-rc.10` stay under the MIT License. See
[licensing](./docs/licensing.md).
