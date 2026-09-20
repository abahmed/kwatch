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

> Kubernetes incidents, explained.

kwatch is an open-source Kubernetes incident monitor. It turns failures into
clear alerts that explain **what broke, why it happened, and what to do next**.

It runs in your own cluster. No hosted account is required.

## ✨ Why teams choose kwatch

| 🚨 Detect | 🧠 Explain | 🔕 Reduce noise | ✅ Act |
| --- | --- | --- | --- |
| Find issues | See causes | Group failures | Know what to do |

Kubernetes shows symptoms. kwatch shows the story.

It helps new teams understand incidents faster. It gives experienced operators
the context they need in one place.

| Kubernetes signal | What kwatch adds |
| --- | --- |
| `CrashLoopBackOff` | Reason, logs, events, owner, and next step |
| `Pending` | Scheduling clues and unschedulable duration |
| Unhealthy node | Affected workloads and dependency impact |
| Many related failures | One grouped incident instead of alert noise |

Alerts give your team useful context and a clear next step.

## 🚀 Install with kwatch

The recommended path is the interactive `kwatch.sh` installer. It:

1. Connects to the cluster you choose.
2. Sets up your alert destination.
3. Stores credentials safely.
4. Installs a hardened kwatch deployment.
5. Verifies the installation.

Run it with no version parameters:

```bash
/bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

You need Bash, `curl`, `kubectl`, and cluster install permissions.

The installer uses the latest stable release by default.

Run it again to change settings, upgrade, check status, or uninstall kwatch.

## 🔔 One alert. The full story.

```text
🚨 OOMKilled — production / orders-api
   Pod: orders-api-7ffc9d4f9-x9p4t
   Node: worker-3 · severity: high

💡 Cause: the container exceeded its 512Mi memory limit.
➡️ Next step: increase limits.memory or reduce memory usage.

📄 Recent logs and Kubernetes events are included.
```

## 🔎 What kwatch monitors

Start with safe defaults. Add optional checks as your needs grow:

| Area | What kwatch finds |
| --- | --- |
| Pods | Crashes, OOM kills, restarts, and readiness issues |
| Scheduling | Pending and unschedulable workloads |
| Workloads | Rollouts, jobs, schedules, and availability issues |
| Infrastructure | Node, disk, memory, and CPU pressure |
| Storage | Persistent storage usage and volume failures |
| Networking | Service, Ingress, webhook, and policy issues |
| Platform health | Control plane and cluster problems |
| Security | TLS, RBAC, admission, and policy findings |

Heartbeat notifications, Metrics Server usage, TLS checks, and active probes
are available when you need them.

## 🛡️ Reliable monitoring by design

Kwatch keeps monitoring if a Pod goes down. The installer runs two replicas
by default with leader election. One monitors your cluster. The other takes
over automatically.

One-replica mode is available for smaller environments. It has no failover.

## 📣 Send alerts where your team works

Choose from **56 notification integrations**. Popular options include Slack,
Discord, Microsoft Teams, Google Chat, Telegram, email, PagerDuty, Opsgenie,
Mattermost, Rocket.Chat, Matrix, webhooks, Jira, and Datadog.

Connect one or more channels in a few steps. See the
[alert channel guide](https://kwatch.dev/docs/channels) for the full list.

## 🔐 Safe by default

Credentials stay in Kubernetes Secrets. The installer handles them for you.
Sensitive values are never stored in plain text configuration.

## ⚙️ Make kwatch fit your team

| Feature | Use it to... |
| --- | --- |
| Namespace filters | Focus on the workloads that matter |
| Silences | Keep planned changes quiet |
| Smart grouping | Turn alert storms into clear incidents |
| Logs and events | Add context to every alert |
| Runbooks | Give responders a direct next step |
| Cluster names | Know which cluster needs attention |

See the [configuration guide](https://kwatch.dev/docs/general-configuration)
for all options.

## 🛠️ Easy to run

The installer handles upgrades, configuration, status checks, and removal.
It protects your settings during upgrades.

## 🎯 Focused on incidents

Use kwatch when something changes and your team needs answers quickly. Pair it
with Prometheus, Grafana, or Loki for long-term metrics and logs.

## 📚 Documentation

- [Getting started](https://kwatch.dev/docs/getting-started)
- [`kwatch.sh` installer](https://kwatch.dev/docs/kwatch-manager)
- [Configuration](https://kwatch.dev/docs/general-configuration)
- [Alert channels](https://kwatch.dev/docs/channels)
- [Kubernetes coverage](./docs/kubernetes-coverage.md)

## 🤝 Contribute

Bug report, new idea, or documentation improvement? Open an issue or pull
request.

- [Open an issue](https://github.com/abahmed/kwatch/issues)
- [Join Discord](https://discord.gg/kzJszdKmJ7)
- [CONTRIBUTING.md](./CONTRIBUTING.md)

## 📄 License

kwatch is available under the [MIT License](./LICENSE).
