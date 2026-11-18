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
Service `<fullname>-dashboard` (`kwatch-dashboard` for a release named
`kwatch`, `<release>-kwatch-dashboard` otherwise) and never exposes it
outside the cluster:
use the installer's Dashboard menu, or point your own Ingress, Gateway or
Route at that Service.

## Access

- **Local** (the default): `kubectl port-forward svc/<fullname>-dashboard
  8061:8061`, then open `http://localhost:8061`. Plain http is allowed.
- **Shared**: the dashboard is shared, and https only, when it is behind
  proxies you list in `dashboard.trustedProxies`, or when kwatch serves TLS
  itself (`dashboard.tls.secretName`). Sign-in over plain http is refused,
  the cookie is `Secure` and responses carry HSTS. There is no separate
  mode setting.

`dashboard.trustedProxies` takes CIDRs or addresses of the proxies that end
TLS in front of kwatch (for example `["10.0.0.0/8"]`). `X-Forwarded-For`,
`X-Forwarded-Proto` and `X-Forwarded-Host` are believed only from those
peers. The browser's `Origin` must match the request `Host`, or the
`X-Forwarded-Host` of a trusted proxy, so sign-in and push work behind a
proxy that rewrites `Host`. The client is
the first `X-Forwarded-For` hop, read from the right, that is not itself a
trusted proxy. Without the list, the headers are ignored, kwatch logs a
warning once, and every client behind a proxy shares one sign-in failure
limit. List only the proxies: any peer inside a listed range can name any
client and claim https, so an entry wider than /16 (IPv4) or /48 (IPv6),
such as a whole pod network, is accepted with a warning at start.

## Authentication

Two credentials live in the Secret `<release>-dashboard` (keys `password`
and `token`), or in `dashboard.existingSecret`:

- **Password**: people sign in on the web page (`POST /login`). The session
  cookie lasts 12 hours from sign-in, is `HttpOnly`, `SameSite=Strict`
  and, in shared mode, `Secure`. Sessions are kept in the state file, so a
  kwatch restart does not sign anyone out. Signing out ends the session at
  once, also for later restarts; a new password or token signs everyone
  out. The state file keeps only a random signing seed, a slow (PBKDF2)
  digest of the password and token, and the SHA-256 of each session ID
  with its expiry: never the password or a session ID. At most 1,024
  sessions are kept; past that a new sign-in ends the oldest. A new or
  reset state file, a standby replica, or a kwatch that could not read
  its state file keeps sessions in memory only, so they end when kwatch
  restarts; the sign-in page then says "You were signed out because
  kwatch restarted."
- **Token**: API clients send `Authorization: Bearer <token>`. In shared
  mode the token must arrive over https.

The chart ships the Secret empty and kwatch fills both values itself the
first time it starts (it may only get and update that one Secret), so
`helm template`, Argo CD and Flux render the same manifest every time and
the password never changes by itself. Edits to the Secret apply when kwatch
restarts.

Wrong credentials are limited per client (10 failures a minute; an IPv6
client counts under its /64). Through one trusted proxy, at most 1,000
failures a minute pass in all, so a peer that keeps naming new clients is
still bounded; past that its sign-ins wait for the next minute. When the
limiter's memory is full the oldest client is forgotten, so a flood of
new clients never locks out one it has not seen. A correct token or
session is never held back by these limits.

The generated password (`XXXX-XXXX-XXXX-XXXX`) is accepted in any case and
with or without its dashes. A password you choose (`existingSecret`) must
be typed exactly as set.

## How to get the password

The sign-in page asks for one password and does not show how to find it.
It is in the Secret `<release>-dashboard` (key `password`), or in
`dashboard.existingSecret`. To read it:

```sh
kubectl -n <namespace> get secret <release>-dashboard \
  -o jsonpath='{.data.password}' | base64 -d
```

or open the installer, choose Dashboard, then "Show my access link and
password":

```sh
bash <(curl -fsSL https://kwatch.dev/kwatch.sh)
```

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

## Browser alerts

**Browser alerts** in the side bar (under **Settings** on a phone) turns on
browser notifications for this browser: kwatch sends each change at the
level you pick through the browser's push service (Web Push, encrypted
for that browser), so they arrive with every dashboard tab closed and
stay on screen until closed. They are for confirmed issues only: an
issue still settling is told once it is confirmed, and one that settles
before alerting is never told, neither new nor resolved. With browser
alerts off a tab reads nothing for them. **Send a test notification** checks the
path. Signing out, turning them off, 30 days unseen or a new password
stops them; at most 100 browsers are kept. A service worker (`/sw.js`)
shows them and caches nothing.

## Pages

- **Overview** (`/`): what is wrong now, in one status line and a short
  list of the issues to look at first, then the week in numbers and
  charts. While nodes come and go the status line adds "Paging held
  while the cluster scales · until ~14:20" (not on a non-production
  cluster, where nothing pages). One quiet line, "3 old problems not
  shown (already failing for 30+ days before kwatch started
  watching)", opens the list of them with how long each has failed;
  the number of days is your `oldProblemDays`, and the line is left
  out when there are none. An object is either an issue or an old
  problem, never both: an issue made only of old problems (one from
  before you set `oldProblemDays`, or kept from before a restart) moves
  to this list without a message.
- **Issues** (`/issues`): every issue in one table, with search, filters
  and saved views. Active, Resolved and All are tabs on it. An issue that
  groups the objects a removed controller left behind carries a small
  "12 objects" chip.
- **Issue page** (`/issues/<id>`): one issue, as plain questions, with the
  tabs Timeline, Proof, Impact, Alerts and Details. When kwatch lowered or
  capped the alert level, or holds the page while the cluster scales, one
  line under the level says why ("Lowered to Low: already failing for
  about 2 years when kwatch first saw it …"). When kwatch has held the
  issue's findings for 10 minutes because it can no longer read their
  kind, that line says "kwatch can't verify this right now: …" instead;
  no alert is sent for it. A group of left-behind
  objects lists them in Impact, under **Objects left behind**: kind,
  namespace/name and what is wrong, the first ten shown and "Show all".
- **Activity** (`/activity`): what changed recently, newest first.
- **kwatch health** (`/kwatch`): whether kwatch itself is working, what it
  watches and where its alerts go. A cluster set as non-production says
  "Non-production cluster: nothing pages" under the verdict.
- **Sign-in**: a password field, shown when nobody is signed in. It links to
  "How to get the password" below.

## Overview and Issues

**Overview** (`/`) is four blocks. A status line says how many issues
are happening now by level, such as "1 urgent · 10 need attention · 29
low" (each part opens that level's issues); it is red only while
something is urgent, amber while something needs attention, and calm
otherwise. **Look at these first** lists up to five issues, most urgent
first and then the longest running, each with a short cause, where it
is, its level and how long ago it started. **Bad releases (last 24 h)
and changes behind active issues** joins the releases of the last 24
hours that failed, went down or got worse with the active issues that
started right after a change; each release links the issue it opened
(resolved or not), or says none opened in the 30 minutes after it and
links its workload's issues. It shows only when there is one. **This
week** has four numbers, all over the issues someone was alerted about
(alerted, resolved this week, the urgent issues: "Alerted the
on-call person" when a pager such as PagerDuty or Opsgenie is
connected, else "Sent as urgent" with "No pager is connected, so urgent
issues go to chat only.", the median time to resolve those same
resolved issues), one line saying how many issues settled on their own before
anyone was alerted, the day-by-day chart, and the top causes beside
where the trouble is, each with a small colour key and each number
saying what it counts ("105 issues this week").

**Issues** (`/issues`) shows one toolbar row: the Active, Resolved and
All tabs, the search box and **Filters (N)**, which opens the other
filters, the severity chips, the quick views, View options and Columns.
Filters in use stay visible as chips under the row. On a wide screen the
toolbar stays put and only the table scrolls, with the table taking
most of the height. The
Status column appears only when a row is in a state other than "Happening
now" (and always on All). A repeat reads "failed again 3 times", the same
words as the issue page. On a phone every issue is a small card: the
title, where it is ("staging · deployment/api", two lines at most), then
its level, how long, and its marks ("failed again 2 times", "12
objects") on the same line where they fit; the page never scrolls
sideways.

## kwatch health and Activity

**kwatch health** (`/kwatch`) opens with one sentence: "kwatch is working
normally" in green, or what is wrong in plain words in amber or red
("kwatch is running, but some alerts aren't getting out", "kwatch is
almost out of memory"), with the reasons under it. Below it, in pairs:
kwatch itself (running since, the last restart, "Last heard from the
Kubernetes API", and "Alerts waiting to be sent" only when some wait),
Resources (CPU and disk against their limits: amber from 70%, red from
90%, said in words too; memory as the memory kwatch itself holds against
what it can use, with the container's figure and the state-file cache
the system can free under it; the memory warning comes when kwatch
holds 80% of what it can use (the Go memory limit, 90% of the
container's limit) for 3 minutes, turns red at 95%, and clears only
below 70%, also after 3 minutes, so it does not flap), Where alerts go (each channel, what it
sent in the last 24 hours, failures only when there are some) and What
kwatch is watching ("Watching 233 of 233 resource types", with only the
counts that are not zero). Version and plan sit in a slim card at the
bottom. A part of kwatch that does not work (node stats from the
kubelets, permissions, the license, and the other optional parts
`/health` reports as degraded) gets its own notice at the top, counts
in the side bar's dot, and keeps the verdict from saying "working
normally". So do issues not announced yet only because kwatch has not
read their namespace's labels ("3 issues are waiting: kwatch hasn't
read their namespace's labels yet"); nothing is shown when none wait.

The kwatch card also says how long kwatch was not watching in the last 24
hours ("Not watching for about 5 min in the last 24 hours."), added up from
the gaps between each stop and the next start. A restart with no recorded
stop time (a crash) is not guessed; the page says how many it left out.
Its **Heartbeat** line says whether anyone is told when kwatch itself
stops: "Pinging every 5 min · last ping 14:02", "The monitoring URL
refused the ping (HTTP 404) · failing since 14:05", or, when
`heartbeatMonitor` is not set up, "Not set up. Nobody is told when kwatch
itself stops." with the setting to use. Not set up is information: it
does not turn the verdict amber. When kwatch was not watching for 15
minutes or more of the last 24 hours, the green verdict says so under
it.

kwatch was not running is never shown as a quiet time. The Overview's
"Day by day" chart hatches each day kwatch was off for 15 minutes or
more, with "6 h off" under the day (a day off all along: "off", and "not
running" across it), says it in each day's tooltip and in a "kwatch not
running" column of the table view, and the week's numbers add "Based on
5½ of 7 days of watching: kwatch was not running for 1 day 14 h this
week". Activity and an issue's timeline put a "Not watching: kwatch was
not running from 02:10 to 08:40 (6 h 30 min)" marker where the gap
falls, and the memory and CPU chart of an issue hatches the hours with no
reading. The times come from the stop and start of each restart kwatch
keeps (32 restarts, at most a week back); a stop with no stop record (a
crash, a lost node) is bounded by kwatch's last liveness stamp, at most
a minute before it stopped.
When kwatch's own node carries a known spot or preemptible label
(`karpenter.sh/capacity-type=spot`, `eks.amazonaws.com/capacityType=SPOT`,
`cloud.google.com/gke-spot=true`, `cloud.google.com/gke-preemptible=true`,
`kubernetes.azure.com/scalesetpriority=spot`,
`node.kubernetes.io/lifecycle=spot`), a notice says the cloud can take the
node back at any time and kwatch is then not watching for a few minutes. It
suggests, as optional steps, a priority class (chart value
`priorityClass.create`) and, when the cluster has stable nodes,
`nodeSelector` or `affinity`. The check is best effort: no known label, no
notice.

Once signed in, the side bar and the browser tab name the cluster
(`app.clusterName`, or "unnamed cluster"; the kwatch health page says where the name came from, or how to set one) and show "Non-production" when
`app.environment` says so. The sign-in page names no cluster: the name
may say more than the address does.

**Activity** (`/activity`) lists what changed in the last hour or 24
hours, newest first. Each row is a sentence: "api in shop: resolved after
12 min", "warehouse in shop: became Urgent (alerted the on-call person)" (with
no pager connected: "sent as urgent"),
"tracking in staging: settled on its own". The line at the top counts
the changes in words ("25 issues started, 4 got more urgent and 21
resolved"). Four or more of the same change in one minute fold behind "8
more resolved at 14:55". A resolve carries `lasted`, the seconds the issue
went on, in `/api/activity`.

## Issue pages

The top of an issue page answers five plain questions, each fact once:
a short title (the whole one under "Full title") with a status line
("Happening now · for 6 h · users are still served by the old version"), **What's wrong?**, **Why?** (the cause
in a sentence, the release it followed on one line, the decisive log line
or check with Copy and the key number, such as how often it restarted),
**What should I do?** (the release's undo first, then the steps, one Copy
per command) and **Who was alerted?** (the first alert and how long after
the start, the last update, whether anyone acknowledged it; if not, the
command that acknowledges it, to copy). **Copy summary** also carries the
proof line, who was alerted and when, and the cluster name. The tabs
under it are **Timeline** (what changed just before first, each change
with a short muted line saying why it is listed, such as "The failing
pods mount this ConfigMap", or nothing when kwatch knows no link;
repeated failures and recoveries grouped into one line), **Proof** (the logs,
checks, containers, warnings and node; healthy containers stay folded),
**Impact**, **Alerts** (the first alert, the updates and the resolve;
repeats folded) and **Details** (has it happened before, is it happening
anywhere else, the issue ID). On a phone the tabs are one row that scrolls
sideways (an arrow and a fade show when more wait off the edge), and the
proof line and log lines wrap. Press `t`, `p`, `i`, `a` or `d` to open a tab. The words are the
same on every page: Urgent, Needs attention, Low and Quiet for how urgent
an issue is; "Alerted" for what kwatch did to Slack or a pager; "Settled
on its own" for an issue that recovered before anyone was alerted.

When kwatch has no cause for an issue, **Why?** says "Cause not found
yet." and lists what kwatch checked for this issue and found fine, only
what it really checked: for example "its node is healthy", "its ConfigMaps
and Secrets are there and did not change", "its dependencies show no
errors" and "nothing near it was changed in the 30 minutes before it
started". It then says what it could not check, such as "kwatch cannot
read pod logs here." When the cause is only possible, the same list
follows it as "Also ruled out: its node, its image". When nothing was
checked there is no list.

A change, in **What changed just before**, on the release line of
**Why?** and on a blamed change's proof, links to where it came from:
"Source: Argo CD app shop · commit 1a2b3c4 · image api:1.5". The GitOps
application is linked only when `app.gitOpsLinks` is set, a commit only on
a known repository host, an image only on Docker Hub or Quay; otherwise
each is shown as text (see "Links to where a change came from" in
configuration.md).

An issue's **Impact** tab has **How it connects** when kwatch knows the
connections of the objects it touches: those objects and the ones next to
them (route to Service, Service to workload, the Services a workload
calls), colored by the worst open issue, the issue's own object
outlined, and the same as a list. A box with another issue opens it.

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
| `GET /api/activity` | Recent changes (`since` 1h or 24h, `untold`, `limit` 1 to 200), with totals by kind for the whole window. |
| `GET /api/push`; `POST /api/push`, `/api/push/status`, `/api/push/remove`, `/api/push/test` | Browser notifications: the push key; subscribe this browser, read its state, unsubscribe, send a test. |
| `GET /api/coverage` | What kwatch watches and the gaps. |
| `GET /api/events` | Server-sent events that say when issues changed. |

Other paths answer `404`. The API is unversioned.
