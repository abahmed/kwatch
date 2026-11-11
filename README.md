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
  what started it, and sends one message: what broke, why, what changed it,
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
  <b>Website:</b> <a href="https://kwatch.dev">kwatch.dev</a> ·
  <b>Docs:</b> <a href="https://docs.kwatch.dev">docs.kwatch.dev</a> ·
  <a href="https://kwatch.dev/pro">Pro, free during early access</a> ·
  <a href="https://discord.gg/kzJszdKmJ7">Discord</a>
</p>

```bash
bash <(curl -fsSL https://kwatch.dev/kwatch.sh)
```

One command, and it walks you through the rest: which cluster, where alerts
should go. Run it again any time to upgrade, change settings or manage your
license.

<p align="center">
  <a href="https://kwatch.dev"><img src="./assets/kwatch-demo.gif" width="900" alt="kwatch demo: one-command install, Discord alerts and Pro set up, then a crashing app triggers a kwatch alert in Discord with its root cause"></a>
</p>

---

<table>
<tr>
<td width="33%" valign="top"><b>🔍 Root cause first</b><br>kwatch follows a failure upstream and names the one cause.</td>
<td width="33%" valign="top"><b>🔒 Your data stays in your cluster</b><br>kwatch analyzes events and logs where they live.</td>
<td width="33%" valign="top"><b>🔕 Quiet by default</b><br>One message, then silence until something new is true.</td>
</tr>
<tr>
<td valign="top"><b>🕵️ What changed</b><br>See what changed, when, and which tool did it (Pro).</td>
<td valign="top"><b>👉 The next step</b><br>Every message ends with the command to run (Pro).</td>
<td valign="top"><b>📣 50+ channels</b><br>Slack, Teams, PagerDuty, email, webhooks and more (one on Community, all on Pro).</td>
</tr>
</table>

## 🎯 One incident instead of fifty alerts

It is 03:07. A release went out, pods (the running copies of your apps) are
crash looping (starting, failing and restarting over and over), a Service has no
endpoints, and two other apps fail their health checks. Every alert is true.
None of them says why.

These samples show Pro. Community says what is failing and the type of cause.

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

Messages look like this in your channel. This one is real kwatch output
(from the release-regression scenario, as a chat shows it):

```text
🟠 **checkout** in **shop** (**prod-eu-1**): rollout 14 restarted 4 times in its first 2 minutes; rollout 13 did not restart.
Only change in revision 14: image registry.example.com/checkout:5.1 → registry.example.com/checkout:5.2; pods restarting more than before since.
The apps that use it failed soon after it changed.
Fix: change the image of container app in deployment/checkout (now registry.example.com/checkout:5.2).
Rolling back to revision 13, the one before this release, undoes it (changes the cluster):
    kubectl rollout undo deployment/checkout -n shop --to-revision=13
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
- 🔒 **Your data stays in your cluster.** kwatch analyzes events and logs
  where they live. One pod and a small volume for state.
- 🔕 **Quiet by default.** No repeats every five minutes and no flapping
  storms. kwatch waits for failures to settle, merges what belongs together
  and speaks when there is news. Successful rollouts, scaling and node
  drains never wake anyone.
- 🕵️ **What changed (Pro).** Rollouts, config edits and image updates sit on a
  timeline with the tool or GitOps app that made them, so a message can say
  "after the 14:02 release by argocd-controller".
- 👉 **The next step (Pro).** A message ends with the command to run: read the crash
  log, describe the node, or roll back.
- 📣 **50+ channels.** Slack, Microsoft Teams, Discord, PagerDuty, Opsgenie,
  email, Jira, webhooks and more. Paging tools get a stable key, so the same
  problem updates the same page and closes it when it is fixed.
  [All channels](https://kwatch.dev/docs/channels).

<details>
<summary><b>📨 More example messages</b></summary>

Samples show Pro.

One node runs low on memory and starts evicting pods. Instead of one alert
per pod, you get one message with the cause, then one when it is over.

Messages are written as short lines, with the resource names in bold, text
from your pods in code and the command in a code block. Slack, Discord,
Telegram, Jira and the other chat and ticket providers show it as below;
SMS, push and pager providers get the same lines as plain text.

```text
🟠 **Node n1** (**prod-eu-1**) is low on memory. All of its dependents are failing.
Replicas that do not depend on it are healthy.
Affected: **cart**, **orders** and **api** in **shop**.
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
🟡 **kwatch summary** · prod-eu-1 — 1 problem

**Problems**
- Service **web** (**shop**) — failed to deploy its load balancer: FailedDeployModel ×5 in 15 min
```

```text
🔴 **kwatch roll-up** · prod-eu-1 — 2 new problems at the same time

**Problems**
- **api** in **billing** keeps crashing
- **api** in **shop** keeps crashing

*Each gets its own message when it changes or resolves.*
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
Affected: cart, search, auth and one other. To read the output
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
   and every change made to them, with the tool or GitOps app that made it.
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

<sub>The chart's default `rbac.mode: least-privilege` grants read access to
the built-in kinds only, so **custom resources are not watched** unless
`rbac.mode` is `full`. kwatch warns once in its log and `/health` lists what
it cannot watch.</sub>

## 🚀 Install

**Installer (recommended).** One command, and it walks you through it. It checks your cluster before it changes
anything, asks where alerts should go, stores credentials in a Kubernetes
Secret, installs kwatch and checks that it is running.

```bash
bash <(curl -fsSL https://kwatch.dev/kwatch.sh)
```

Prefer Helm? See the [chart guide](./deploy/chart/README.md).

**Requirements:** Kubernetes 1.23 or newer; on older clusters HPA, PodDisruptionBudget, CronJob and EndpointSlice checks may be unavailable.

## ⭐ Editions

One binary, two editions. Community needs no account and no key. Pro is free
during early access. No credit card required.

| | Community | Pro |
| --- | --- | --- |
| Detection | Pods: crash loops, OOM kills, Pending and unschedulable pods, image pull errors, config errors, evictions, init-container failures, liveness and startup probe failures; node NotReady | Adds node pressure and draining, readiness, rollouts, Jobs and CronJobs, PDBs, HPAs, storage, network, admission, certificates, quota, custom resources, control plane, probes and unusual events |
| Root cause | What is failing and the type of cause ("Most likely cause: a config change.") | The exact cause (which object or change), full proof and confidence, shared-factor suspects, recent changes and the tool or GitOps app that made them, affected services, log excerpts, fix commands and the setting to change, recurrence ("This is the third time this week") |
| Channels | 1 provider (pick with `app.communityProvider`), namespace filters and silences | All providers, routing by scope, threads, text templates, audit log |
| Noise control | Related alerts arrive as one incident, in Community and Pro. Dedup and settling; failures already present at start are announced one by one if still failing after the startup window; announced incidents survive a restart, so their resolves are still sent | Flapping detection, startup summary, state kept on disk across restarts, changes made while kwatch was down, gap report |
| Operations | RBAC audit, secret redaction, health (`/health`) | Heartbeat monitor (dead man's switch), Prometheus `/metrics`, severity overrides, maintenance holds, runbook links, `/status` (text and JSON), flight recorder and `replay-export` |
| Dashboard | None | Read-only web dashboard: active and resolved issues, who was told, and an overview of the cluster and of kwatch itself (`dashboard.enabled`) |
| Account | None | Email only |
| Support | GitHub issues and Discussions | Dedicated support: installation help, and your feature requests and fixes come first |

Pro includes dedicated support. Go to [**Get help**](https://app.kwatch.dev/help).
Community support is on
[GitHub](https://github.com/abahmed/kwatch/issues).

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
- kwatch checks its license about once a day (see
  [what leaves my cluster](#-what-leaves-my-cluster)). If it can't reach
  kwatch.dev, Pro keeps working for up to 7 days after the last successful
  check, then falls back to Community. Monitoring continues.
- The license state is in the startup log, in `/status` (Pro) and in the
  installer's **License / Pro** menu.

Details: [licensing](./docs/licensing.md).

## 🔒 What leaves my cluster

**Root cause, without your data leaving the cluster.**
kwatch analyzes events and logs where they live.

Only optional anonymous usage stats and Pro license checks reach kwatch.dev.
Never cluster names, pod names, logs or events. Alerts go only to the
channels you set up.

- **Your alerts**, to the channels you configure. Secrets and tokens are
  redacted before anything is stored or sent.
- **Usage stats, weekly (opt-out)**: an anonymous ID derived from your
  cluster (a one-way hash that can't be traced back to it or matched to your
  license) and the kwatch version.
  Turn it off with `telemetry.enabled: false` or `KWATCH_TELEMETRY=false`.
- **Pro license renewal, daily**: a hashed cluster ID, the license ID, the
  kwatch version and the node count. Community without a key makes no
  license calls.
- **Nothing else goes to kwatch.dev.** To know when kwatch itself stops,
  kwatch Pro can ping your own monitoring (`heartbeatMonitor.url`).
- The license, the telemetry and the update check always verify TLS, even if
  `app.insecureSkipTLSVerify` is set for your alert providers.

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
<summary><b>What does Pro support include?</b></summary>

Community questions go to GitHub issues and Discussions. Pro includes dedicated support: installation help, and
your feature requests and kwatch fixes come first.
</details>

<details>
<summary><b>What access does it need?</b></summary>

Read access to the resources it watches (this is set with RBAC, Kubernetes permissions). It writes only its own state lock
Lease, its state file, its license instance key Secret in its own namespace
(only when a license key is configured), and permission self-checks that are
not persisted. It never gets `pods/exec`
or `nodes/proxy`.

By default kwatch can read Secrets, to catch missing references and expiring
certificates; it keeps only hashes, never values. Set `watch.secrets: false`
to remove Secret access, except the
license instance key.
</details>

<details>
<summary><b>What happens when kwatch restarts?</b></summary>

With Pro, it picks up where it left off: open incidents are not announced
again and changes made while it was down still explain failures. After a
gap of 5 minutes or more it sends a gap report: what changed and what
crashed while it was down, compared with before. Community saves
only the incidents it announced that are still open, and restores them
so their resolves are still sent; history and baselines start fresh. Any
other failure already present is announced on its own if it is still
failing once the two-minute startup window ends (Pro sends one startup
summary instead), and one that recovers inside the window is never
mentioned. See
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
