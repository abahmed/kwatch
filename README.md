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
  <b>When something breaks in your Kubernetes cluster, kwatch tells you what
  broke, why, and what to do, in Slack, Discord, email and 50+ other
  channels.</b>
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

One command, and it walks you through the rest: which cluster, where alerts
should go. Run it again any time to upgrade, change settings or manage your
license.

---

<table>
<tr>
<td width="33%" valign="top"><b>🔍 Root cause first</b><br>kwatch follows a failure upstream and names the one cause.</td>
<td width="33%" valign="top"><b>🔕 Quiet by default</b><br>One message, then silence until something new is true.</td>
<td width="33%" valign="top"><b>🕵️ Who changed what</b><br>See who changed what, and when.</td>
</tr>
<tr>
<td valign="top"><b>👉 The next step</b><br>Every message ends with the command to run.</td>
<td valign="top"><b>📣 50+ channels</b><br>Slack, Teams, PagerDuty, email, webhooks and more.</td>
<td valign="top"><b>🔒 Your data stays in your cluster</b><br>One pod. No kwatch cloud.</td>
</tr>
</table>

## 🎯 One incident instead of fifty alerts

It is 03:07. A release went out, pods (the running copies of your apps) are
crash looping (starting, failing and restarting over and over), a Service has no
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

## 🧭 New to Kubernetes?

You do not need to be an expert. kwatch in 30 seconds:

```text
1. Install    Run one command. It asks a few questions.
2. It watches kwatch runs as one small pod in your cluster and keeps a
              live map of what depends on what.
3. You hear   When something breaks you get one clear message:
              what broke, why, and what to do.
```

<details>
<summary><b>Words you may meet</b></summary>

- **Cluster:** the group of machines that runs your apps.
- **Node:** one machine in the cluster.
- **Pod:** a running copy of an app.
- **Namespace:** a named area of the cluster, often one per team or app.
- **CrashLoopBackOff:** a pod keeps starting, failing and restarting.
- **OOMKilled:** a pod used more memory than allowed and was stopped.
- **RBAC:** Kubernetes permissions. It decides what kwatch may read.
- **Helm:** a package manager for Kubernetes, an alternative way to install.
- **CRD:** a custom resource definition, an extra kind of object that a tool
  adds to the cluster.

More in the [glossary](https://docs.kwatch.dev/glossary) and the
[docs](https://docs.kwatch.dev).
</details>

## ✨ Why kwatch

- 🔍 **Root cause first.** kwatch follows a failure upstream through
  everything it depends on and names the one cause: a bad release, a full
  node, an expired certificate, a database outside the cluster. Symptoms
  join one incident.
- 🔕 **Quiet by default.** No repeats every five minutes and no flapping
  storms. kwatch waits for failures to settle, merges what belongs together
  and speaks when there is news. Successful rollouts, scaling and node
  drains never wake anyone.
- 🕵️ **Who changed what.** Rollouts, config edits and image updates sit on a
  timeline with who made them, so a message can say "after alice's 14:02
  release".
- 👉 **The next step.** A message ends with the command to run: read the crash
  log, describe the node, or roll back.
- 📣 **50+ channels.** Slack, Microsoft Teams, Discord, PagerDuty, Opsgenie,
  email, Jira, webhooks and more. Paging tools get a stable key, so the same
  problem updates the same page and closes it when it is fixed.
  [All channels](https://kwatch.dev/docs/channels).
- 🔒 **Your data stays in your cluster.** One pod and a small volume for state.
  There is no kwatch cloud reading your logs.

<details>
<summary><b>📨 More example messages</b></summary>

One node runs low on memory and starts evicting pods. Instead of one alert
per pod, you get one message with the cause, then one when it is over.

Messages are written as short lines, with the resource names in bold, text
from your pods in code and the command in a code block. Slack, Discord,
Telegram, Jira and the other chat and ticket providers show it as below;
SMS, push and pager providers get the same lines as plain text.

```text
🟠 **Node n1** (**prod-eu-1**) is low on memory. All of its dependents are failing.
Replicas that do not depend on it are healthy.
**cart**, **orders** and **api** in **shop** are affected as well.
To check the node's conditions and recent events, run
    kubectl describe node n1
```

```text
✅ **Node n1** (**prod-eu-1**) is ready again. It was failing for ten minutes.
```

The same goes for the other incidents you care about:

```text
🟠 **api** is failing in **shop** (**prod-eu-1**) after the 10:01 release of registry.example.com/api:2.3.
The previous image was registry.example.com/api:2.2.
It fails with `panic: missing key DB_PASSWORD_V2`.
To see the rollout state, run
    kubectl rollout status deployment/api -n shop
```

Messages that list several problems are short lists, the ones that need
attention first and what resolved on one line. kwatch reports what is
happening, not what could happen: advice such as a single replica or a missing
probe is never listed:

```text
🟡 **kwatch digest** · staging — 2 problems · 5 resolved

**Problems**
• Node **ip-10-0-67-211** — failing again
• Service **ingress-nginx** (**kube-addons**) — Kubernetes reported FailedDeployModel 3 times in the last quarter hour

✅ 5 resolved since last digest: accounts, assets, comms +2
```

```text
🟠 **kwatch** · prod-eu-1 — 2 new problems at the same time

**Problems**
• **ledger** in **billing** keeps crashing
• **checkout** in **orders** keeps crashing

_Each gets its own message when it changes or resolves._
```

One message per problem, with its cause and the command to fix or check it.
Known and low-priority problems stay quiet, and the resolve message tells you
what happened.

When five apps fail because a database outside the cluster is down, kwatch
blames the database, not the apps:

```text
🟠 api in shop (prod-eu-1) keeps crashing because external endpoint
db.example.com:5432 refuses connections. It fails with "dial tcp
db.example.com:5432: connect: connection refused". Every error mentions it.
cart, search, auth and one other are affected as well. To read the output
of the last crash, run
kubectl logs api-6d4f-0 -c app -n shop --previous
```

When a node disappears:

```text
🔴 Node n1 (prod-eu-1) has not reported for about two minutes. It reports
"Kubelet stopped posting node status". To check the node's conditions and
recent events, run kubectl describe node n1
```

When kwatch is not sure, it says so instead of guessing. One emoji tells you
how urgent a message is: 🔴 page · 🟠 notify · 🟡 low · ✅ resolved.
</details>

## ⚙️ How it works

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

## 🔎 What it catches

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

## 🚀 Install

**Installer (recommended).** One command, and it walks you through it. It checks your cluster before it changes
anything, asks where alerts should go, stores credentials in a Kubernetes
Secret, installs kwatch and checks that it is running.

```bash
/bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

**Already use Helm?** You can install the chart instead:

```bash
helm repo add kwatch https://kwatch.dev/charts
helm repo update
helm upgrade --install kwatch kwatch/kwatch \
  --namespace kwatch --create-namespace
```

<details>
<summary><b>Helm permissions (RBAC) and more</b></summary>

The chart defaults to `rbac.mode: least-privilege`, an explicit read set for
the kinds kwatch watches. To also watch custom resources and kinds added
later, add `--set rbac.mode=full`. See the
[chart guide](./deploy/chart/README.md) for the full install, values and
the Secret-based configuration, and [kwatch.sh](./docs/kwatch-sh.md) for
what the installer does.
</details>

## ⭐ Editions

One binary, two editions. Community needs no account and no key. Pro is free
during early access.

| | Community | Pro |
| --- | --- | --- |
| Detection | Pod crash loops, OOM kills and Pending pods; node NotReady | Adds node pressure and draining, readiness, rollouts, Jobs and CronJobs, PDBs, HPAs, storage, network, admission, certificates, quota, custom resources, control plane, probes and unusual events |
| Root cause | One-line headline | Full proof and confidence, shared-factor suspects, recent changes and who made them, affected services, log excerpts |
| Channels | 1 provider (pick with `app.communityProvider`), namespace filters and silences | All providers, routing by scope, threads, text templates, audit log |
| Noise control | Dedup and settling, state in memory | Flapping detection, startup summary, state kept on disk across restarts, changes made while kwatch was down |
| Operations | RBAC audit, secret redaction, health, metrics, text `/status` | Severity overrides, maintenance holds, runbook links, heartbeat monitor, `/status` JSON, flight recorder and `replay-export` |
| Account | None | Email only |

When a Pro license ends, kwatch falls back to Community. Alerts never stop.
The authoritative list is the catalog in [`internal/feature`](./internal/feature),
and the comparison is on [kwatch.dev/pro](https://kwatch.dev/pro).

### 🔑 Get Pro

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

## 🔒 What leaves my cluster

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

## ❓ FAQ

<details>
<summary><b>Does it replace Prometheus or Grafana?</b></summary>

No. Keep your metrics. kwatch answers a different question: when something
breaks, what caused it and what to do. It needs no rules and works next to
them.
</details>

<details>
<summary><b>What access does it need?</b></summary>

Read access to the resources it watches (this is set with RBAC, Kubernetes permissions). It writes only its own state lock
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

## 📏 Tested like a product

Every change is replayed against a library of real failure scenarios, plus a
hidden set nobody tunes for, and the build fails if it gets worse. CI checks
that kwatch names the right cause, never alerts on normal operations, and
stays small. See the
[latest scorecard](./internal/scenarios/SCORECARD.md).

## 🤝 Contributing

Report a bug, suggest an idea or send a pull request. Start with
[CONTRIBUTING.md](./CONTRIBUTING.md) and the
[plain-English architecture guide](./docs/contributor-architecture.md), then
follow a step-by-step guide to add a
[detector](./docs/contributing-detector.md), a
[propagation rule](./docs/contributing-propagation-rule.md) or a
[scenario](./docs/contributing-scenario.md).

## 🛡️ Security

Report vulnerabilities privately, as described in
[SECURITY.md](./SECURITY.md).

## 💬 Community

[Open an issue](https://github.com/abahmed/kwatch/issues) ·
[Join Discord](https://discord.gg/kzJszdKmJ7) ·
[Code of conduct](./CODE_OF_CONDUCT.md)

## ⭐ Star us

If kwatch saves you a night, star the repo so others can find it, and come
say hello on [Discord](https://discord.gg/kzJszdKmJ7).

## 📄 License

kwatch is source available under the [Elastic License 2.0](./LICENSE)
(ELv2) since the release after `v1.0.0-rc.10`. You may use it for free in
your own clusters, read and change the code, and share it. You may not offer
kwatch as a hosted service or remove its license-key checks. Releases up to
and including `v1.0.0-rc.10` stay under the MIT License. See
[licensing](./docs/licensing.md).
