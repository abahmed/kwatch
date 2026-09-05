<p align="center">
  <a href="https://kwatch.dev">
    <img src="./assets/logo.svg" width="260" alt="kwatch" />
  </a>
</p>

<p align="center">
  <strong>When Kubernetes breaks, know why. 👀🧠📣</strong>
</p>

<p align="center">
  <a href="https://kwatch.dev">Website</a> ·
  <a href="https://kwatch.dev/docs/getting-started">Docs</a> ·
  <a href="https://discord.gg/kzJszdKmJ7">Discord</a> ·
  <a href="https://github.com/abahmed/kwatch/issues">Issues</a>
</p>

> **🚧 Unreleased** — this branch documents the next development build.
> `kwatch.sh` is the supported install path. Stable and preview status are
> shown below.

# kwatch

kwatch is an open-source Kubernetes monitor that turns cluster problems into
clear, actionable alerts.

It watches your cluster, connects symptoms to likely causes, and sends the
right context to the channel your team already uses. No hosted account is
required: kwatch runs in your own cluster.

## ✨ Why kwatch?

Kubernetes can tell you that a Pod is failing. kwatch helps explain what to do
next.

| Kubernetes says... | kwatch adds... |
| --- | --- |
| `CrashLoopBackOff` | The reason, recent logs, events, and useful context |
| `Pending` | Scheduling clues and the resources that may be blocking it |
| A node is unhealthy | The affected workloads and the wider impact |
| Many Pods fail together | One grouped incident instead of alert noise |

The goal is simple: answer **what broke, why it happened, and what to check
next**. 🧭

## 🚀 Install in one command

The interactive `kwatch.sh` manager is the recommended way to start. It asks
where to send alerts, creates the Kubernetes resources, checks the rollout,
and can manage the installation later.

```bash
/bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

You need Bash, `curl`, `kubectl`, and access to a supported Kubernetes cluster.
The manager lets you choose a kubeconfig context without changing your current
context.

For a different namespace or release name:

```bash
KWATCH_NAMESPACE=platform-monitoring \
KWATCH_RELEASE=kwatch \
  /bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

<!-- stable-install:start -->

✅ **Stable:** **v0.10.5**

`kwatch.sh` always installs the latest stable release. It stores notification
credentials in a Kubernetes Secret and waits for kwatch to become ready.

<!-- stable-install:end -->

### 🧪 Preview builds

<!-- rc-install:start -->

The manager installs stable releases only. The current preview is
**v0.11.0-rc.7**. Preview builds are for testing; see the
[release notes](https://github.com/abahmed/kwatch/releases) before using one.

<!-- rc-install:end -->

Run the manager again after installation to configure alerts, change settings,
upgrade, check status, or uninstall kwatch.

> 🧭 **Recommended:** use `kwatch.sh` for the normal installation. This README
> intentionally does not include Helm or `kubectl apply` commands. Manual,
> Helm, and direct manifest lifecycles are not supported; use the manager.

## 🔔 What an alert looks like

```text
🚨 OOMKilled — production / orders-api
   Pod: orders-api-7ffc9d4f9-x9p4t
   Node: worker-3 · severity: high

💡 Cause: the container exceeded its 512Mi memory limit.
➡️ Next step: increase limits.memory or reduce memory usage.

📄 Recent logs and Kubernetes events are included.
```

Alerts can include the affected workload, owner, namespace, Pod, container,
node, recent logs, Kubernetes events, related dependencies, and a suggested
next step when the signal provides enough information.

## 🔎 What does kwatch monitor?

Most monitors are enabled by default:

- 💥 Pods and containers: crashes, OOM kills, restart patterns, and probes
- ⏳ Scheduling: Pending Pods, unschedulable workloads, and scheduling delay
- 🧱 Workloads: Deployments, StatefulSets, DaemonSets, Jobs, and CronJobs
- 🖥️ Infrastructure: Nodes, resource pressure, disk, and inode usage
- 💾 Storage: PVC usage and persistent-volume related failures
- 🌐 Traffic: Services, Ingress, admission webhooks, and NetworkPolicies
- 📈 Scaling: HPA and cluster-autoscaler signals
- 🧭 Platform health: control plane, kubelet telemetry, and cluster resources
- 🔐 Security: TLS expiry, PDB issues, RBAC findings, and Pod Security labels
- 🧩 Optional extensions: active probes and custom-resource status conditions

Heartbeat notifications, runtime Metrics Server usage, TLS monitoring, active
probes, and custom-resource watching are opt-in. See the
[configuration reference](https://kwatch.dev/docs/general-configuration) for
defaults, permissions, and thresholds.

## 📣 Send alerts where your team works

kwatch supports **56 notification integrations**:

Slack · Discord · Microsoft Teams · Google Chat · Telegram · email · PagerDuty
· Opsgenie · Mattermost · Rocket.Chat · Matrix · webhooks · Jira · Datadog ·
and many more.

Configure one or more channels under `alert:`. For examples, routing, retries,
fallbacks, and the complete provider list, see the
[alert channel guide](https://kwatch.dev/docs/channels).

## ⚙️ Start with a small config

The manager creates the Secret for you. A simple Slack configuration looks like
this:

```yaml
app:
  clusterName: production

alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
```

`/config/slack-webhook` must be a file mounted from a Kubernetes Secret.
Plain credentials and environment substitutions are rejected at startup.

Common settings:

| Setting | Purpose |
| --- | --- |
| `namespaces` | Monitor only selected namespaces |
| `reasons` | Include or exclude alert reasons |
| `silences` | Suppress known, intentional failures |
| `includeLogs` / `includeEvents` | Add useful Kubernetes context |
| `smartGrouping` | Combine related symptoms |
| `correlation` | Track, resolve, cool down, and re-notify incidents |
| `app.clusterName` | Identify the cluster in every alert |

Use `kwatch lint` before applying a config. Add `--check` to verify credentials
for providers that support checks. Read the full
[configuration reference](https://kwatch.dev/docs/general-configuration) for
all settings and safe credential storage.

## 🛠️ Manage the installation

`kwatch.sh` is also the day-to-day manager:

```text
install          Install kwatch
configure-alert  Change the notification destination
configure        Change monitors, thresholds, and silences
upgrade          Upgrade to the latest stable release
status           Show deployment and manager state
features         Show the installed feature catalog
uninstall        Remove the workload and notification Secret
```

It keeps the `KwatchConfig`, backups, namespace, and CRD during uninstall so a
future reinstall can keep the existing configuration. Run `kwatch.sh --help`
for the command list.

## 💻 CLI tools

The container includes a small CLI for operators and automation:

| Command | Purpose |
| --- | --- |
| `kwatch` | Run the monitor |
| `kwatch --version` | Print the short version |
| `kwatch version --json` | Print build information as JSON |
| `kwatch lint --strict` | Validate config and catch unknown fields |
| `kwatch lint --check` | Validate config and supported provider checks |
| `kwatch replay --dry-run < events.jsonl` | Preview replay without sending |

`kwatch replay` sends real notifications by default. Use `--dry-run` when you
only want to preview the result.

## 🧭 Focused, not a full observability stack

kwatch is an alerting and diagnosis layer. It is not a metrics database, log
store, dashboard, or query language.

Use Prometheus/Grafana for long-term metrics and Loki for log search. Use kwatch
when something changes and you need a useful explanation quickly.

## 📚 Documentation

- [Getting started](https://kwatch.dev/docs/getting-started)
- [`kwatch.sh` manager](https://kwatch.dev/docs/kwatch-manager)
- [Configuration](https://kwatch.dev/docs/general-configuration)
- [Alert channels](https://kwatch.dev/docs/channels)
- [CLI commands](https://kwatch.dev/docs/cli-commands)
- [Kubernetes coverage](./docs/kubernetes-coverage.md)
- [Architecture](https://kwatch.dev/docs/architecture/overview)
- [Release integrity](./docs/release-integrity.md)

## 🤝 Contribute

Bug report? New idea? Documentation improvement? Please open an issue or pull
request.

- [Open an issue](https://github.com/abahmed/kwatch/issues)
- [Join Discord](https://discord.gg/kzJszdKmJ7)
- [CONTRIBUTING.md](./CONTRIBUTING.md)

## 📄 License

kwatch is available under the [MIT License](./LICENSE).
