<p align="center">
  <a href="https://kwatch.dev">
    <img src="./assets/logo.svg" width="260" alt="kwatch" />
  </a>
</p>

<p align="center">
  <strong>See what broke. Understand why. Know what to do next. 👀🧠⚡</strong>
</p>

<p align="center">
  <a href="https://kwatch.dev">Website</a> ·
  <a href="https://kwatch.dev/docs/getting-started">Docs</a> ·
  <a href="https://discord.gg/kzJszdKmJ7">Discord</a> ·
  <a href="https://github.com/abahmed/kwatch/issues">Issues</a>
</p>

# kwatch

kwatch is an open-source Kubernetes incident monitor. It turns failures into
clear alerts that explain **what broke, why it happened, and what to check
next**.

It runs inside your own cluster. There is no hosted account, agent platform,
or required observability stack.

## Why teams use kwatch

Kubernetes can tell you that a Pod is failing. kwatch adds the context needed
to act:

| Kubernetes signal | What kwatch adds |
| --- | --- |
| `CrashLoopBackOff` | Reason, logs, events, owner, and next step |
| `Pending` | Scheduling clues and unschedulable duration |
| Unhealthy node | Affected workloads and dependency impact |
| Many related failures | One grouped incident instead of alert noise |

Alerts can include the workload, owner, namespace, Pod, container, node,
recent logs, Kubernetes events, related dependencies, and a suggested action.

## 🚀 Install interactively

The supported path is the interactive `kwatch.sh` manager. It:

1. Lets you choose the kubeconfig context without changing your current one.
2. Asks where to send alerts.
3. Stores credentials in a Kubernetes Secret.
4. Installs the CRD and hardened kwatch workload.
5. Waits for the deployment to become ready and checks its security posture.

Run it with no version parameters:

```bash
/bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

You need Bash, `curl`, `kubectl`, and permission to install the required
namespace-scoped and cluster-scoped resources.

The manager selects the latest stable release by default. During installation
and upgrade it offers the newest release candidate interactively when one is
available.

<!-- stable-install:start -->

✅ **Stable:** **v0.10.5**

Stable is the recommended channel for normal use. Credentials are stored in a
Kubernetes Secret and the manager waits for kwatch to become ready.

<!-- stable-install:end -->

### 🧪 Preview builds

<!-- rc-install:start -->

The current preview is **v0.11.0-rc.7**. Select it interactively when you want
to test preview changes. Preview builds are for testing; read the
[release notes](https://github.com/abahmed/kwatch/releases) first.

<!-- rc-install:end -->

Use the manager again after installation to configure alerts, change settings,
upgrade, check status, or uninstall kwatch. Do not apply `deploy.yaml` or
`config.yaml` manually; that bypasses guided Secret handling and verification.

For a different namespace or release name, set environment variables before
starting the manager:

```bash
KWATCH_NAMESPACE=platform-monitoring \
KWATCH_RELEASE=kwatch-prod \
  /bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

## 🔔 What an alert looks like

```text
🚨 OOMKilled — production / orders-api
   Pod: orders-api-7ffc9d4f9-x9p4t
   Node: worker-3 · severity: high

💡 Cause: the container exceeded its 512Mi memory limit.
➡️ Next step: increase limits.memory or reduce memory usage.

📄 Recent logs and Kubernetes events are included.
```

## 🔎 What kwatch monitors

Most monitors are enabled by default:

| Area | Examples |
| --- | --- |
| Pods and containers | Crashes, OOM kills, restarts, and readiness |
| Scheduling | Pending Pods, unschedulable workloads, and delay |
| Workloads | Deployments, StatefulSets, DaemonSets, Jobs, and CronJobs |
| Infrastructure | Nodes, resource pressure, disk, and inode usage |
| Storage | PVC usage and persistent-volume failures |
| Traffic | Services, Ingress, webhooks, and NetworkPolicies |
| Scaling | HPA and cluster-autoscaler signals |
| Platform health | Control plane, kubelet telemetry, and cluster resources |
| Security | TLS expiry, PDB issues, RBAC, and Pod Security findings |

Heartbeat notifications, Metrics Server usage, TLS monitoring, active probes,
and custom-resource watching are opt-in. See the
[configuration reference](https://kwatch.dev/docs/general-configuration) for
defaults, permissions, and thresholds.

## 📣 Send alerts where your team works

kwatch supports **56 notification integrations**, including Slack, Discord,
Microsoft Teams, Google Chat, Telegram, email, PagerDuty, Opsgenie,
Mattermost, Rocket.Chat, Matrix, webhooks, Jira, and Datadog.

Configure one or more channels under `alert:`. The
[alert channel guide](https://kwatch.dev/docs/channels) has setup examples,
routing, retries, fallbacks, and the complete provider list.

## 🔐 Credentials stay in Secrets

Provider credentials, diagnostic tokens, and heartbeat URLs must be mounted
from a Kubernetes Secret. Use an exact file reference in `config.yaml`:

```yaml
app:
  clusterName: production

alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
```

Plain credentials and `${ENV_VAR}` substitutions are rejected for sensitive
fields. The manager creates the Secret and writes only file references to the
configuration.

## ⚙️ Useful configuration

| Setting | Use it to... |
| --- | --- |
| `namespaces` | Watch only selected namespaces |
| `reasons` | Include or exclude alert reasons |
| `silences` | Suppress known, intentional failures, including matching Event messages |
| `includeLogs` / `includeEvents` | Add Kubernetes context to alerts |
| `smartGrouping` | Combine related symptoms |
| `correlation` | Track, resolve, cool down, and re-notify incidents |
| `app.clusterName` | Identify the cluster in every alert |

Run `kwatch lint` before applying a configuration. Add `--check` to verify
credentials for providers that support checks.

## 🛠️ Manage the installation

Run the same manager command again after installation:

```text
install          Install kwatch
configure-alert  Change the notification destination
configure        Change monitors, thresholds, and silences
upgrade          Upgrade to stable or choose an available RC
status           Show deployment and manager state
features         Show the installed feature catalog
uninstall        Remove the workload and notification Secret
```

The manager backs up configuration before upgrades and keeps the `KwatchConfig`,
backups, namespace, and CRD during uninstall so a future reinstall can recover
the existing setup.

## 💻 CLI tools

The container also includes a small CLI for operators and automation:

| Command | Purpose |
| --- | --- |
| `kwatch --version` | Print the short version |
| `kwatch version --json` | Print build information as JSON |
| `kwatch lint --strict` | Validate config and reject unknown fields |
| `kwatch lint --check` | Validate config and provider checks |
| `kwatch replay --dry-run < events.jsonl` | Preview replay without sending |

`kwatch replay` sends real notifications by default. Use `--dry-run` when you
only want to preview the result.

## 🧭 Focused, not a full observability stack

kwatch is an alerting and diagnosis layer. It is not a metrics database, log
store, dashboard, or query language. Use Prometheus/Grafana for long-term
metrics and Loki for log search; use kwatch when something changes and you
need a useful explanation quickly.

## 📚 Documentation

- [Repository documentation index](./docs/README.md)
- [Getting started](https://kwatch.dev/docs/getting-started)
- [`kwatch.sh` manager](https://kwatch.dev/docs/kwatch-manager)
- [Configuration](https://kwatch.dev/docs/general-configuration)
- [Alert channels](https://kwatch.dev/docs/channels)
- [CLI commands](https://kwatch.dev/docs/cli-commands)
- [Kubernetes coverage](./docs/kubernetes-coverage.md)
- [Architecture](https://kwatch.dev/docs/architecture/overview)
- [Release integrity](./docs/release-integrity.md)

## 🤝 Contribute

Bug report, new idea, or documentation improvement? Open an issue or pull
request.

- [Open an issue](https://github.com/abahmed/kwatch/issues)
- [Join Discord](https://discord.gg/kzJszdKmJ7)
- [CONTRIBUTING.md](./CONTRIBUTING.md)

## 📄 License

kwatch is available under the [MIT License](./LICENSE).
