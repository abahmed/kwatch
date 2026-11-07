# Dashboard

The dashboard is a read-only web UI and JSON API for incidents. It is a Pro
feature, off by default, and runs on its own port, never on the health
port. Community refuses it: the port is never opened. How the pages are
built is documented next to the code, in
`internal/dashboard/UI_NOTES.md`.

## Enabling

```yaml
dashboard:
  enabled: true
  port: 8061          # must differ from healthCheck.port
```

With the Helm chart set `dashboard.enabled=true`. The chart adds a ClusterIP
Service `<release>-dashboard` and never exposes it outside the cluster:
use the installer's Dashboard menu, or point your own Ingress, Gateway or
Route at that Service.

## Access

- **Local** (the default): `kubectl port-forward svc/<release>-dashboard
  8061:8061`, then open `http://localhost:8061`. Plain http is allowed.
- **Shared**: the dashboard is shared, and https only, when it is behind
  proxies you list in `dashboard.trustedProxies`, or when kwatch serves TLS
  itself (`dashboard.tls.secretName`). Sign-in over plain http is refused,
  the cookie is `Secure` and responses carry HSTS. There is no separate
  mode setting.

`dashboard.trustedProxies` takes CIDRs or addresses of the proxies that end
TLS in front of kwatch (for example `["10.0.0.0/8"]`). `X-Forwarded-For`
and `X-Forwarded-Proto` are believed only from those peers. The client is
the first `X-Forwarded-For` hop, read from the right, that is not itself a
trusted proxy. Without the list, the headers are ignored, kwatch logs a
warning once, and every client behind a proxy shares one sign-in failure
limit.

## Authentication

Two credentials live in the Secret `<release>-dashboard` (keys `password`
and `token`), or in `dashboard.existingSecret`:

- **Password**: people sign in on the web page (`POST /login`). The session
  cookie lasts 12 hours, is `HttpOnly` and `SameSite=Strict`, and ends when
  kwatch restarts; the sign-in page then says "You were signed out because
  kwatch restarted."
- **Token**: API clients send `Authorization: Bearer <token>`. In shared
  mode the token must arrive over https.

The chart ships the Secret empty and kwatch fills both values itself the
first time it starts (it may only get and update that one Secret), so
`helm template`, Argo CD and Flux render the same manifest every time and
the password never changes by itself. Edits to the Secret apply when kwatch
restarts.

Wrong credentials are limited per client (10 failures a minute). A correct
token or session is never held back by that limit.

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `dashboard.enabled` | `false` | Serve the dashboard. |
| `dashboard.port` | `8061` | Listener port. |
| `dashboard.trustedProxies` | `[]` | Proxies whose forwarded headers count; any entry makes the dashboard https only. |
| `dashboard.tlsCertFile`, `dashboard.tlsKeyFile` | empty | kwatch serves TLS itself (chart: `dashboard.tls.secretName`). |
| `dashboard.existingSecret` (chart) | empty | Your own Secret with `password` and `token`. |

Environment overrides set by the chart: `KWATCH_DASHBOARD_ENABLED`,
`KWATCH_DASHBOARD_PORT`, `KWATCH_DASHBOARD_TRUSTED_PROXIES` (comma
separated), `KWATCH_DASHBOARD_TLS_CERT`, `KWATCH_DASHBOARD_TLS_KEY`.

## API

All `/api/*` calls need a session cookie or the bearer token and answer
JSON. Lists carry an `ETag`; send `If-None-Match` to get `304`.

| Endpoint | Returns |
|---|---|
| `GET /api/session` | `204` when signed in; `{"signedIn":false}` (with `"reason":"restarted"` after a restart) otherwise. |
| `GET /api/incidents` | Paged, filtered and sorted issue list (`status`, `severity`, `namespace`, `q`, `sort`, `page`, `pageSize`, ...). |
| `GET /api/incidents/{id}` | One issue with its timeline, evidence, delivery trail and similar issues. |
| `GET /api/overview` | Counts, what to look at first, releases, and the week's insights and trends (`tz`, `tzName`). |
| `GET /api/status` | Version, edition, last scan, scope, per-channel delivery health (including failing channels), kwatch's own state, and the `kwatch` part: coverage totals, sends per channel, state file and resources. |
| `GET /api/activity` | Recent changes. |
| `GET /api/map` | The object map around issues. |
| `GET /api/coverage` | What kwatch watches and the gaps. |
| `GET /api/events` | Server-sent events that say when issues changed. |

Other paths answer `404`. The API is unversioned.
