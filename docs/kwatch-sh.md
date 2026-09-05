# 🧭 The `kwatch.sh` manager

`kwatch.sh` is the easiest way to install and manage kwatch. It is a small
Bash script that uses your existing `kubectl` access; it does not install a
second package manager.

## 🚀 Start here

Requirements:

- `kubectl` connected to a Kubernetes cluster
- `curl`
- permission to install namespace-scoped resources and cluster-scoped CRD/RBAC
  resources

```bash
/bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

To inspect it before running it:

```bash
curl -fsSL https://kwatch.dev/kwatch.sh -o kwatch.sh
less kwatch.sh
bash kwatch.sh
```

The script is plain Bash and is published in
[`kwatch.dev/static/kwatch.sh`](https://github.com/abahmed/kwatch.dev/blob/main/static/kwatch.sh).

The manager checks the cluster, asks for one alert destination, stores the
credential in a Kubernetes Secret, installs the CRD and hardened workload, and
waits for the deployment to become ready. It also verifies restricted Pod
Security labels, non-root/read-only execution, dropped capabilities,
`RuntimeDefault` seccomp, and the `0400` Secret volume mode before reporting
success. It applies the namespace's restricted Pod Security labels itself, so
the normal install does not require a separate `kubectl apply` or label step.

This is the supported installation path. It downloads and applies the matching
release resources itself; do not apply `deploy.yaml` or `config.yaml` manually,
because that bypasses the guided Secret handling and security verification.

## 🎯 Choose a cluster safely

If your kubeconfig has more than one context, the manager shows the contexts
and asks you to choose one. It passes that context explicitly to `kubectl`; it
does not change your current context.

Use another namespace or release name with environment variables:

```bash
KWATCH_NAMESPACE=platform-monitoring \
KWATCH_RELEASE=kwatch-prod \
  /bin/bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)"
```

## 📋 Commands

The same URL can run a command directly:

```bash
bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)" -- status
bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)" -- configure
bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)" -- upgrade
bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)" -- uninstall
bash -c "$(curl -fsSL https://kwatch.dev/kwatch.sh)" -- --help
```

| Command | What it does |
| --- | --- |
| `install` | Install the latest stable release |
| `configure-alert` | Change the alert provider and credential |
| `configure` | Change settings from the generated catalog |
| `upgrade` | Upgrade to the latest stable release |
| `status` | Show the workload and manager state |
| `features` | Show the capabilities of the installed release |
| `uninstall` | Remove the workload and notification Secret |
| `--help` | Show manager usage and exit |

## 🔐 What the manager protects

- Credentials go into a Secret, not a ConfigMap.
- The generated `config.yaml` contains only `${file:/config/...}` references;
  a plain credential is rejected by the kwatch process.
- The manager validates names, versions, and required permissions.
- Config is backed up before an upgrade.
- A failed rollout restores the previous config and attempts a rollback.
- Optional TLS monitoring is enabled only after you approve its Secret access.
- Uninstall preserves the CRD, configuration resource, backups, and namespace.

## 🧪 Version-aware settings

The manager loads the configuration, feature, and guided-provider catalogs for
the installed release. It caches them in the cluster and falls back to its
embedded catalogs when GitHub is not reachable. The provider catalog covers
every supported notification provider, defines each documented prompt, and
marks whether its value must be stored as a Secret file.

The release workflow generates these catalogs from Go definitions. When a new
setting or guided provider is added, update its source definition and regenerate
the release artifacts.

## 🆘 Troubleshooting

```bash
kubectl get pods -n kwatch
kubectl logs -n kwatch deployment/kwatch
```

Run the manager again and choose **Show status**. For manual installation and
Helm, see [Installation](https://kwatch.dev/docs/installation).
