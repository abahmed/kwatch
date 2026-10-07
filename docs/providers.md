# 📣 Alert providers

This is the complete provider reference for kwatch. If you are choosing a
channel for the first time, start with the [quick-start guide](../README.md)
or the [channel picker on kwatch.dev](https://kwatch.dev/docs/channels).

In simple terms: configure a provider under `alert:`, give kwatch its webhook
or credential, and it will send problems to that destination.

Provider credentials must never appear directly in `config.yaml`. Put every
webhook, token, key, password, and other credential in a mounted Kubernetes
Secret, then use an exact `${file:/absolute/path}` reference. kwatch rejects
plain credentials and `${ENV_VAR}` substitutions for sensitive fields.

## How providers work

- **One config block per provider.** Every provider is configured under `alert.<name>`,
  e.g. `alert.slack`, `alert.discord`, `alert.email`. You can enable as many as you like —
  kwatch sends every alert to all of them.
- **Two flavors.** Most providers need a **webhook URL** (a URL you copy from the provider's
  settings — the provider does the authentication). A few need a **token or key** (an API
  credential you create for kwatch). The tables below say which, and always tell you exactly
  what to fill in.
- **Pick the one your team already lives in.** Team chat for day-to-day noise (Slack,
  Discord, Teams), paging for serious stuff (PagerDuty, Opsgenie, SIGNL4, Squadcast), email
  or SMS if you want a paper trail, and the **Custom Webhook** if you have anything else in
  mind.
- **Every provider says the same thing.** Every provider receives the same
  incident message and renders it itself (see
  [What each provider receives](#what-each-provider-receives)). No provider is
  a second-class citizen.
- **Reliability is built in.** Every provider shares the same routing, retry, and fallback
  controls (shown at the top under Slack — they apply to all providers).
- **One HTTP path.** Every provider that talks HTTP sends through the same helper
  (`delivery/transport`), so a `429` is always honoured with its `Retry-After`, a `4xx` is never
  retried (the payload will not get better), and a `5xx` or network error always is. A
  provider cannot have its own idea of what a status code means — the linter rejects raw
  `net/http` calls under `internal/alert/`.

## What each provider receives

kwatch writes one message per incident update. It has two texts:

- **Note** is the full narrative in a few plain sentences: what is wrong, the
  proof, the consequence and one suggested command.
- **Short** is the first sentence on its own, for places with little room.

Both start with exactly one status marker and carry no other emoji:
🔴 page, 🟠 notify, 🟡 low, ✅ resolved. Every message of one incident shares a
stable key, so a provider can update and close the same alert, thread or
issue. Paging providers and webhooks use `kwatch-<cluster>-<key>` as that
id (spaces in the cluster name become `-`; without `app.clusterName` it is
`kwatch-<key>`), so two clusters that share one service never merge or resolve
each other's alerts. Plain operator messages (startup, upgrade, test) share
`kwatch-<cluster>-notice`.

Plain operator messages are not incidents. Providers that open an alert
someone must close — PagerDuty, Opsgenie, GoAlert, ilert, incident.io,
Squadcast, Zenduty and SIGNL4 — skip them, so a startup banner never pages
anyone. Sensu Go records them as a passing (OK) check.

| Group | Providers | What they send |
|:--|:--|:--|
| 💬 Chat | Slack, Discord, Microsoft Teams, Teams Workflow, Mattermost, Rocket.Chat, Google Chat, Webex, Matrix, Telegram, Zulip, Feishu, DingTalk, WeCom, Flock, Threema, Signal | The Note as the message text, escaped for the chat's markup, with recent application output as a code or quoted block when there is any. Broadcast mentions such as `@channel` are neutralized. Matrix sends the Note as `body` and HTML-escaped as `formatted_body`. Slack with a bot token announces each incident as one root message holding the whole Note and its output, keeps every update and the resolve in its thread, and edits the root to the same announcement under the new status marker when the status changes (a thread reply that is not a status change leaves the root alone; after a restart the root cannot be rebuilt, so only the thread grows). Application output is fenced with a code fence longer than any run of backticks in it, so it cannot close the block; it is cut before the closing fence is added. WeCom `<@userid>` and Zulip `@*group*` mentions in workload text are neutralized too. |
| 🚨 Paging | PagerDuty, Opsgenie, Splunk On-Call, GoAlert, ilert, incident.io, Squadcast, Zenduty, SIGNL4, Alerta, Datadog, New Relic, AWS SNS, Splunk HEC, Sensu Go | The Short as the alert title or summary, the Note (plus output) as details, the incident id as the dedup or alert key, and the provider's resolve or close action when the incident resolves. Severity follows the incident severity. |
| 📋 Issue trackers | GitHub, GitLab, Gitea, Jira, ClickUp | One issue per incident: the Short as title and the Note (plus output and the cluster name) as body. Updates add a comment. On resolve kwatch comments and closes the issue (GitHub, GitLab, Gitea; Jira moves it through `closeTransition` and ClickUp to `closeStatus` when you set one, otherwise they only comment, because workflows are defined per project or list). A resolved incident that fails again within its reopen window comments on the same issue and reopens it on GitHub, GitLab and Gitea. Jira (`reopenTransition`) and ClickUp (`reopenStatus`) reopen only when you configure both the close and the reopen name; otherwise the mapping is forgotten at resolve and a recurrence opens a new issue. Pod-controlled text in the Note has its `@mentions` neutralized, and Jira wiki markup in it is escaped. A GitHub 403 that says the rate limit was hit is retried like a 429. |
| 📱 SMS and push | Twilio, Plivo, Vonage, MessageBird, Pushover, Pushbullet, Gotify, ntfy, IFTTT, Home Assistant | The Short. A resolve is sent at normal priority. IFTTT also sends the Note as `value2` and the status as `value3`. |
| 📧 Email | Email (SMTP), AWS SES, SendGrid, Mailgun, Resend | Subject is the Short on one line; the body is the Note followed by recent output as a `> ` quoted block. |
| 🔗 Structured | Custom Webhook, n8n, Zapier | The whole message as JSON (fields below). |

Long texts are cut at the provider's size limit on a character boundary and end
in `…`. The cut is deterministic, so a retried delivery sends the same bytes.

## How to pick

| You want... | Start here |
|:--|:--|
| Team chat alerts | 💬 Slack · 💬 Discord · 💼 Microsoft Teams · 🚀 Rocket Chat · 🌐 Mattermost |
| A page when it's serious | 🚨 PagerDuty · 🔔 Opsgenie · 🆘 SIGNL4 · 📟 Squadcast · 🔭 ilert · 🆘 Splunk OnCall |
| An email or SMS trail | 📧 Email (SMTP) · ✈️ SendGrid · ☁️ AWS SES · ✉️ Twilio |
| Alerts to your own system | 🔗 Custom Webhook · 🔔 Ntfy · 📳 Gotify · 🏠 HomeAssistant |
| An issue/ticket system | 📋 Jira · 📋 ClickUp · 🐙 GitHub · 🦊 GitLab · 🦘 Gitea |

Everything below is a reference — one section per provider, with the parameter table you need.

### 💬 Slack

**What it is:** the classic team chat. The quickest way to get an alert into a channel.

**Webhook mode:**
| Parameter | What it does |
|:---|---|
| `alert.slack.webhook` | 🔗 Slack webhook URL |
| `alert.slack.channel` | 📢 Override channel |
| `alert.slack.compact` | 📏 Single-line mode |

**Bot Token mode:**
| Parameter | What it does |
|:---|---|
| `alert.slack.token` | 🔑 Bot token (`xoxb-...`) |
| `alert.slack.channel` | 📢 Channel to post to |
| `alert.slack.compact` | 📏 Single-line mode |

**Compact mode:**
```yaml
alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
    compact: true
```

> 💡 **Pro tip:** When using bot token mode, alerts become threaded conversations — the full alert is the root message, updates and the resolve are replies, and the root's status marker follows the incident. Clean and organized! 🧹

#### 📮 Routing, Retry & Fallback (applies to every provider)

In plain words: the same options work for all providers, not just Slack.

- **`routes`** — if you have several *channels* for one provider, send only some alerts to
  each (filtered by `namespaces`, `severities`, `reasons` or `owners`, the
  `kwatch.io/owner` of the workload or namespace). A route severity is one of
  `critical` (pages), `warning` (notifies) or `info` (digest); any other value is rejected
  at startup because it would never match. Other route keys are ignored with a warning.
- **`retry`** — how hard kwatch tries before giving up: `maxAttempts` times, waiting `delay`
  between tries. Only failures that *can* succeed on a retry are retried — a timeout, a 5xx,
  a rate limit (which waits exactly as long as the provider's `Retry-After` asks). A failure
  that says the request itself is wrong — a 4xx, a rejected payload, an unknown channel, a
  revoked token — is given up on immediately and goes straight to the dead-letter queue, so
  it cannot hold up the alerts queued behind it. The retry options are
  `maxAttempts` (default 3, 1 to 20), `delay` (default `1s`), `maxBackoff`
  (cap on the growing wait, default `30s`), `jitterEnabled` (default `false`)
  and `jitterFactor` (0 to 1, default `0.25`, used only when jitter is on).
  Durations are strings such as `5s`.
- **`fallback`** — if this provider fails for good, hand the alert to another provider
  (e.g. Slack → PagerDuty when Slack is down). It must name a provider that is
  configured under `alert`; startup and `kwatch lint` reject an unknown name.
- **`templates`** — message text per reason for this provider, overriding the
  global `templates` (see the configuration reference). An invalid template is a
  configuration error.
- **`hourlyBudget`** — at most this many new conversations per hour
  (default 60, `0` is unlimited); the rest go into one overflow digest.

```yaml
alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
    routes:
      - namespaces: ["production"]
        severities: ["critical"]
    retry:
      maxAttempts: 3
      delay: 5s
```

Need a backup? Set a fallback:
```yaml
alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
    fallback: "pagerduty"    # 🆘 tries PagerDuty if Slack fails
    retry:
      maxAttempts: 3
```

### 💬 Discord

**What it is:** chat with pretty message embeds — the easiest one-click setup of them all
(one webhook URL from your server's channel settings).

| Parameter | What it does |
|:---|---|
| `alert.discord.webhook` | 🔗 Discord webhook URL |

### 📧 Email

**What it is:** good old SMTP email to one or more inboxes — a simple paper trail anyone can
search.

> With the chart's NetworkPolicy enabled, add your SMTP port (25, 465 or
> 587) to `networkPolicy.extraEgressPorts`; only 443 is open by default.

| Parameter | What it does |
|:---|---|
| `alert.email.from` | 📤 From address |
| `alert.email.password` | 🔑 SMTP password (optional for a relay without authentication) |
| `alert.email.username` | 👤 SMTP username (optional, defaults to `from`) |
| `alert.email.tls` | 🔒 `required` (default) or `none` for a trusted relay |
| `alert.email.host` | 🖥️ SMTP host |
| `alert.email.port` | 🔌 SMTP port |
| `alert.email.to` | 📥 Receiver email |

### 🚨 PagerDuty

**What it is:** real paging with on-call schedules and escalation — for when an alert means
someone should be woken up.

| Parameter | What it does |
|:---|---|
| `alert.pagerduty.integrationKey` | 🔑 PagerDuty integration key |

### ✈️ Telegram

**What it is:** instant push straight to your phone or desktop — create a bot with the Bot
Father to get a `token` and a `chatId`.

| Parameter | What it does |
|:---|---|
| `alert.telegram.token` | 🔑 Bot token |
| `alert.telegram.chatId` | 💬 Chat ID |

### 💼 Microsoft Teams

| Parameter | What it does |
|:---|---|
| `alert.teams.webhook` | 🔗 Webhook URL |
| `alert.teams.title` | ✏️ Custom title |

> `alert.teams.maxRetries` is ignored: Teams used to retry inside the provider on top of the
> shared delivery retry, so a rate-limited flow was hammered twice. Retries are now governed
> only by the shared retry settings above, like every other provider.

### 🚀 Rocket Chat

| Parameter | What it does |
|:---|---|
| `alert.rocketchat.webhook` | 🔗 Webhook URL |

### 🌐 Mattermost

| Parameter | What it does |
|:---|---|
| `alert.mattermost.webhook` | 🔗 Webhook URL |

### 🔔 Opsgenie

| Parameter | What it does |
|:---|---|
| `alert.opsgenie.apiKey` | 🔑 API Key |
| `alert.opsgenie.region` | 🌍 API region: `us` (default) or `eu` |

### 🏗️ Matrix

| Parameter | What it does |
|:---|---|
| `alert.matrix.homeServer` | 🖥️ HomeServer URL (a trailing `/` is ignored). A 429 is retried after the server's `retry_after_ms` |
| `alert.matrix.accessToken` | 🔑 Access token |
| `alert.matrix.internalRoomId` | 🆔 Room ID |

### 🔔 DingTalk

| Parameter | What it does |
|:---|---|
| `alert.dingtalk.accessToken` | 🔑 Access token |
| `alert.dingtalk.secret` | 🔐 Signing secret |
| `alert.dingtalk.title` | ✏️ Custom title |

### 🐦 FeiShu

| Parameter | What it does |
|:---|---|
| `alert.feishu.webhook` | 🔗 Webhook URL |
| `alert.feishu.title` | ✏️ Custom card title (default: `kwatch`) |
| `alert.feishu.secret` | 🔑 Signing secret. Set it when the bot has signature verification on; every message is then signed, and a bot with verification on rejects unsigned ones |

### 🛡️ Zenduty

| Parameter | What it does |
|:---|---|
| `alert.zenduty.integrationKey` | 🔑 Integration Key |
| `alert.zenduty.alertType` | 🏷️ Alert type (default: critical) |

### 💬 Google Chat

| Parameter | What it does |
|:---|---|
| `alert.googlechat.webhook` | 🔗 Webhook URL |

### 📳 Gotify

| Parameter | What it does |
|:---|---|
| `alert.gotify.url` | 🔗 Gotify server URL |
| `alert.gotify.token` | 🔑 App token |
| `alert.gotify.priority` | 🎚️ Priority (optional) |
| `alert.gotify.title` | ✏️ Custom title |

```yaml
alert:
  gotify:
    url: "https://gotify.example.com"
    token: "${file:/config/gotify-token}"
```

### 🔔 Ntfy

| Parameter | What it does |
|:---|---|
| `alert.ntfy.topic` | 📢 Topic to publish to. kwatch publishes JSON to the server root with the topic in the body, as ntfy documents |
| `alert.ntfy.url` | 🔗 Server URL (default: `https://ntfy.sh`) |
| `alert.ntfy.token` | 🔑 Optional auth token |
| `alert.ntfy.priority` | 🎚️ Priority 1-5 (default: 4) |
| `alert.ntfy.title` | ✏️ Custom title |

```yaml
alert:
  ntfy:
    topic: "${file:/config/ntfy-topic}"
```

### 📲 Pushover

| Parameter | What it does |
|:---|---|
| `alert.pushover.token` | 🔑 Application token |
| `alert.pushover.user` | 👤 User or group key |
| `alert.pushover.priority` | 🎚️ Priority from -2 to 2 (optional). It applies to the announcement of an incident only; priority 2 (emergency) only to a page-tier announcement, other tiers are held at 1. Updates, summaries, digests, startup and plain messages are normal priority. A resolve cancels the emergency receipt first, so the alarm stops repeating, and then sends the normal-priority "resolved" push; if the cancel fails transiently nothing is sent and the whole resolve is retried |
| `alert.pushover.retry` | ⏱️ Emergency retry interval in seconds |
| `alert.pushover.expire` | ⌛ Emergency expiration in seconds |
| `alert.pushover.title` | ✏️ Custom title |

### 🟣 Webex

| Parameter | What it does |
|:---|---|
| `alert.webex.accessToken` | 🔑 Bot access token |
| `alert.webex.roomId` | 🚪 Room ID (provide this or `toPersonEmail`) |
| `alert.webex.toPersonEmail` | ✉️ Person email (provide this or `roomId`) |

### 🐙 GitHub

| Parameter | What it does |
|:---|---|
| `alert.github.token` | 🔑 Personal access token |
| `alert.github.owner` | 👤 Repository owner |
| `alert.github.repo` | 📦 Repository name |
| `alert.github.url` | 🔗 Optional endpoint override (e.g. GitHub Enterprise) |

```yaml
alert:
  github:
    token: "${file:/config/github-token}"
    owner: "acme"
    repo: "infra"
```

### 🦊 GitLab

| Parameter | What it does |
|:---|---|
| `alert.gitlab.token` | 🔑 Personal access token |
| `alert.gitlab.projectId` | 🆔 Project ID |
| `alert.gitlab.url` | 🔗 Optional endpoint override (e.g. self-hosted GitLab) |

```yaml
alert:
  gitlab:
    token: "${file:/config/gitlab-token}"
    projectId: "12345"
```

### 🦘 Gitea

| Parameter | What it does |
|:---|---|
| `alert.gitea.token` | 🔑 Access token |
| `alert.gitea.owner` | 👤 Repository owner |
| `alert.gitea.repo` | 📦 Repository name |
| `alert.gitea.url` | 🔗 Optional endpoint override (e.g. self-hosted Gitea) |

### 🧩 Zapier

| Parameter | What it does |
|:---|---|
| `alert.zapier.url` | 🔗 Zap webhook URL |
| `alert.zapier.token` | 🔑 Optional token |
| `alert.zapier.title` | ✏️ Title (plain messages only) |

### ⚡ n8n

| Parameter | What it does |
|:---|---|
| `alert.n8n.url` | 🔗 Workflow webhook URL |
| `alert.n8n.token` | 🔑 Optional auth header value |
| `alert.n8n.title` | ✏️ Title (plain messages only) |

### 🧙 IFTTT

| Parameter | What it does |
|:---|---|
| `alert.ifttt.key` | 🔑 Webhooks key |
| `alert.ifttt.event` | 🎯 Event name (default: `kwatch`) |

```yaml
alert:
  ifttt:
    key: "${file:/config/ifttt-key}"
```

### 🗒️ Microsoft Teams Workflow

| Parameter | What it does |
|:---|---|
| `alert.teamsworkflow.webhook` | 🔗 Power Automate / Teams Workflow URL |

### 👑 Zulip

| Parameter | What it does |
|:---|---|
| `alert.zulip.email` | ✉️ Bot email |
| `alert.zulip.token` | 🔑 Bot API key |
| `alert.zulip.channel` | 📢 Channel/stream to post to |
| `alert.zulip.url` | 🔗 Server URL (required; example hosts are rejected) |
| `alert.zulip.title` | ✏️ Custom title |

### 🏠 HomeAssistant

| Parameter | What it does |
|:---|---|
| `alert.homeassistant.token` | 🔑 Long-lived access token |
| `alert.homeassistant.url` | 🔗 Server URL (default: `http://localhost:8123`) |
| `alert.homeassistant.service` | 🔧 Notification service (default: `notify`) |

### 🔆 Splunk

| Parameter | What it does |
|:---|---|
| `alert.splunk.url` | 🔗 HEC endpoint URL |
| `alert.splunk.token` | 🔑 HEC token |
| `alert.splunk.source` | 🏷️ Source name (optional) |
| `alert.splunk.sourcetype` | 🏷️ Source type (optional) |
| `alert.splunk.index` | 📚 Index name (optional) |
| `alert.splunk.host` | 🖥️ Host name (optional) |

```yaml
alert:
  splunk:
    url: "https://splunk.example.com:8088/services/collector/event"
    token: "${file:/config/splunk-token}"
```

### 🐕 Datadog

| Parameter | What it does |
|:---|---|
| `alert.datadog.apiKey` | 🔑 API key |
| `alert.datadog.site` | 🌍 Datadog site as a bare host name (default: `datadoghq.com`). Examples: `datadoghq.eu`, `us3.datadoghq.com`, `us5.datadoghq.com`, `ap1.datadoghq.com`, `ddog-gov.com`. Events go to `https://api.<site>`. A value with a scheme, path, port or space fails config validation (lint and startup) |
| `alert.datadog.applicationKey` | 🔑 Optional application key |
| `alert.datadog.alertType` | 🏷️ Fixed alert type for every incident: `error`, `warning`, `info`, `success`, `user_update`, `recommendation` or `snapshot`. Unset, each incident uses its own severity; an unknown value is ignored with a log line |
| `alert.datadog.title` | 🏷️ Fixed event title, cut to Datadog's 100-byte limit |
| `alert.datadog.tags` | 🏷️ Comma-separated tags. Every event also carries `cluster:<clusterName>` unless you set a `cluster:` tag yourself |

### 📈 New Relic

| Parameter | What it does |
|:---|---|
| `alert.newrelic.apiKey` | 🔑 User API key |
| `alert.newrelic.accountId` | 🆔 Account ID |
| `alert.newrelic.region` | 🌍 Data-center region: `us` (default) or `eu`. An EU account must set `eu`, or New Relic rejects the key |

```yaml
alert:
  newrelic:
    apiKey: "${file:/config/newrelic-api-key}"
    accountId: "1234567"
```

### 📋 ClickUp

| Parameter | What it does |
|:---|---|
| `alert.clickup.token` | 🔑 Personal API token |
| `alert.clickup.listId` | 🆔 List ID to create tasks in |
| `alert.clickup.priority` | 🎚️ Optional task priority (1-4) |
| `alert.clickup.closeStatus` | ✅ List status a resolve moves the task to, for example `complete`. Unset, a resolve only comments. A name the list does not have is logged and ignored |
| `alert.clickup.reopenStatus` | 🔁 List status a closed task returns to when its incident fails again, for example `to do`. Needs `closeStatus`. Unset, a recurrence opens a new task |

```yaml
alert:
  clickup:
    token: "${file:/config/clickup-token}"
    listId: "901234567"
```

### 🔭 ilert

| Parameter | What it does |
|:---|---|
| `alert.ilert.integrationKey` | 🔑 Integration key |
| `alert.ilert.priority` | 🎚️ Priority (LOW/HIGH/CRITICAL, default: HIGH) |

### 🚨 Incident.io

| Parameter | What it does |
|:---|---|
| `alert.incidentio.url` | 🔗 Incident.io URL |
| `alert.incidentio.apiKey` | 🔑 Optional API key |

> 💡 Also accepted as `incident.io` in config.

### 📟 Squadcast

| Parameter | What it does |
|:---|---|
| `alert.squadcast.serviceKey` | 🔑 Service key |

### 🆘 SIGNL4

| Parameter | What it does |
|:---|---|
| `alert.signl4.teamSecret` | 🔑 Team secret |
| `alert.signl4.title` | ✏️ Custom title |
| `alert.signl4.user` | 👤 Optional alerting user |
| `alert.signl4.url` | 🔗 Optional endpoint override |

### ✉️ Twilio

| Parameter | What it does |
|:---|---|
| `alert.twilio.accountSid` | 🔑 Account SID |
| `alert.twilio.authToken` | 🔑 Auth token |
| `alert.twilio.from` | 📤 Sender phone number |
| `alert.twilio.to` | 📥 Recipient phone number |

```yaml
alert:
  twilio:
    accountSid: "${file:/config/twilio-account-sid}"
    authToken: "${file:/config/twilio-auth-token}"
    from: "+12025550100"
    to: "+12025550101"
```

### 📱 Vonage

| Parameter | What it does |
|:---|---|
| `alert.vonage.apiKey` | 🔑 API key |
| `alert.vonage.apiSecret` | 🔑 API secret |
| `alert.vonage.from` | 📤 Sender name/number |
| `alert.vonage.to` | 📥 Recipient phone number |

### 📱 Plivo

| Parameter | What it does |
|:---|---|
| `alert.plivo.authId` | 🔑 Auth ID |
| `alert.plivo.authToken` | 🔑 Auth token |
| `alert.plivo.from` | 📤 Sender number |
| `alert.plivo.to` | 📥 Recipient phone number |

### 🐦 MessageBird

| Parameter | What it does |
|:---|---|
| `alert.messagebird.accessKey` | 🔑 Access key |
| `alert.messagebird.from` | 📤 Sender number |
| `alert.messagebird.to` | 📥 Recipient phone number |

### 🟡 Signal

| Parameter | What it does |
|:---|---|
| `alert.signal.number` | 📤 Sender phone number |
| `alert.signal.to` | 📥 Recipient phone number |
| `alert.signal.url` | 🔗 REST API URL (default: `http://localhost:8080`) |

### ✈️ SendGrid

| Parameter | What it does |
|:---|---|
| `alert.sendgrid.apiKey` | 🔑 API key |
| `alert.sendgrid.from` | 📤 From address |
| `alert.sendgrid.to` | 📥 Recipients (list of addresses) |
| `alert.sendgrid.subject` | ✏️ Email subject (plain messages only) |

```yaml
alert:
  sendgrid:
    apiKey: "${file:/config/sendgrid-api-key}"
    from: "kwatch@example.com"
    to:
      - "ops@example.com"
      - "oncall@example.com"
```

### ☁️ AWS SES

| Parameter | What it does |
|:---|---|
| `alert.ses.accessKeyId` | 🔑 AWS access key ID |
| `alert.ses.secretAccessKey` | 🔑 AWS secret access key |
| `alert.ses.sessionToken` | 🎟️ Session token for temporary credentials (optional) |
| `alert.ses.region` | 🌍 AWS region (default: `us-east-1`) |
| `alert.ses.from` | 📤 Verified sender address |
| `alert.ses.to` | 📥 Recipients (comma-separated) |
| `alert.ses.subject` | ✏️ Email subject (plain messages only) |

```yaml
alert:
  ses:
    accessKeyId: "${file:/config/ses-access-key-id}"
    secretAccessKey: "${file:/config/ses-secret-access-key}"
    region: "us-east-1"
    from: "kwatch@example.com"
    to: "ops@example.com, oncall@example.com"
```

### 📣 AWS SNS

| Parameter | What it does |
|:---|---|
| `alert.sns.accessKeyId` | 🔑 AWS access key ID |
| `alert.sns.secretAccessKey` | 🔑 AWS secret access key |
| `alert.sns.sessionToken` | 🎟️ Session token for temporary credentials (optional) |
| `alert.sns.region` | 🌍 AWS region (default: `us-east-1`) |
| `alert.sns.topicArn` | 📢 SNS topic ARN (optional when using `targetArn`) |
| `alert.sns.targetArn` | 📢 SNS endpoint or target ARN (alternative to `topicArn`) |
| `alert.sns.subject` | ✏️ Optional subject (email subscriptions) |

```yaml
alert:
  sns:
    accessKeyId: "${file:/config/sns-access-key-id}"
    secretAccessKey: "${file:/config/sns-secret-access-key}"
    region: "us-east-1"
    topicArn: "arn:aws:sns:us-east-1:123456789012:kwatch"
```

### 📋 Jira

| Parameter | What it does |
|:---|---|
| `alert.jira.url` | 🔗 Jira base URL |
| `alert.jira.user` | 👤 Email or username |
| `alert.jira.apiToken` | 🔑 API token |
| `alert.jira.projectKey` | 🆔 Project key |
| `alert.jira.issueType` | 🏷️ Issue type (default: `Task`) |
| `alert.jira.closeTransition` | ✅ Workflow transition a resolve applies, for example `Done` (matched against the transition name or its target status). Unset (the default), a resolve only comments, because workflows differ per project. A project without that transition is logged and left as it is |
| `alert.jira.reopenTransition` | 🔁 Transition that reopens a closed issue when its incident fails again, for example `Reopen`. Needs `closeTransition`. Unset, a recurrence opens a new issue |

```yaml
alert:
  jira:
    url: "https://kwatch.atlassian.net"
    user: "ops@example.com"
    apiToken: "${file:/config/jira-api-token}"
    projectKey: "OPS"
```

### 🟩 WeCom (WeChat Work)

| Parameter | What it does |
|:---|---|
| `alert.wecom.webhook` | 🔗 Group robot webhook URL |

```yaml
alert:
  wecom:
    webhook: "${file:/config/wecom-webhook}"
```

### 🆘 Splunk OnCall (VictorOps)

| Parameter | What it does |
|:---|---|
| `alert.splunkoncall.apiKey` | 🔑 API key |
| `alert.splunkoncall.routingKey` | 🔀 Routing key |
| `alert.splunkoncall.url` | 🔗 Optional endpoint override |

```yaml
alert:
  splunkoncall:
    apiKey: "${file:/config/splunk-oncall-api-key}"
    routingKey: "${file:/config/splunk-oncall-routing-key}"
```

### ✉️ Mailgun

| Parameter | What it does |
|:---|---|
| `alert.mailgun.apiKey` | 🔑 API key |
| `alert.mailgun.domain` | 📦 Sending domain |
| `alert.mailgun.from` | 📤 From address |
| `alert.mailgun.to` | 📥 Recipients (comma-separated) |
| `alert.mailgun.subject` | ✏️ Email subject (plain messages only) |
| `alert.mailgun.url` | 🔗 Optional endpoint override (e.g. EU region) |

```yaml
alert:
  mailgun:
    apiKey: "${file:/config/mailgun-api-key}"
    domain: "mg.example.com"
    from: "kwatch@mg.example.com"
    to: "ops@example.com"
```

### ✉️ Resend

| Parameter | What it does |
|:---|---|
| `alert.resend.apiKey` | 🔑 API key |
| `alert.resend.from` | 📤 From address |
| `alert.resend.to` | 📥 Recipients (comma-separated) |
| `alert.resend.subject` | ✏️ Email subject (plain messages only) |

```yaml
alert:
  resend:
    apiKey: "${file:/config/resend-api-key}"
    from: "kwatch@example.com"
    to: "ops@example.com, oncall@example.com"
```

### 🚨 GoAlert

| Parameter | What it does |
|:---|---|
| `alert.goalert.url` | 🔗 GoAlert server URL (required) |
| `alert.goalert.token` | 🔑 API token |
| `alert.goalert.serviceId` | 🆔 Service ID |

```yaml
alert:
  goalert:
    url: "https://goalert.example.invalid"
    token: "${file:/config/goalert-token}"
    serviceId: "SVC123"
```

### 🚦 Alerta

| Parameter | What it does |
|:---|---|
| `alert.alerta.url` | 🔗 Alerta server URL |
| `alert.alerta.apiKey` | 🔑 API key |
| `alert.alerta.environment` | 🌍 Environment (default: `Production`) |
| `alert.alerta.service` | 🏷️ Service name (default: `kwatch`) |

```yaml
alert:
  alerta:
    url: "https://alerta.example.com"
    apiKey: "${file:/config/alerta-api-key}"
```

### 🟩 Threema Gateway

| Parameter | What it does |
|:---|---|
| `alert.threema.gatewayId` | 🔑 Threema Gateway ID |
| `alert.threema.secret` | 🔑 Gateway secret |
| `alert.threema.to` | 📥 Recipient Threema ID |

### 💬 Flock

| Parameter | What it does |
|:---|---|
| `alert.flock.webhook` | 🔗 Incoming webhook URL |

### 🔵 Pushbullet

| Parameter | What it does |
|:---|---|
| `alert.pushbullet.accessToken` | 🔑 Access token |

### 📟 Sensu Go

| Parameter | What it does |
|:---|---|
| `alert.sensugo.url` | 🔗 Sensu Go API URL |
| `alert.sensugo.apiKey` | 🔑 API key |
| `alert.sensugo.namespace` | 🗂️ Namespace (default: `default`) |
| `alert.sensugo.entity` | 🖥️ Entity name (default: `kwatch`) |

```yaml
alert:
  sensugo:
    url: "http://sensu.example.com:8080"
    apiKey: "${file:/config/sensugo-api-key}"
```

### 🔗 Custom Webhook

**What it is:** any URL that accepts a POST. Your catch-all for tools kwatch doesn't have a
dedicated page for — an IRC bot, a home-grown dashboard, a Zapier-style glue job.

| Parameter | What it does |
|:---|---|
| `alert.webhook.url` | 🔗 Webhook URL |
| `alert.webhook.headers` | 📋 Custom headers |
| `alert.webhook.basicAuth.username` | 👤 Basic-auth username |
| `alert.webhook.basicAuth.password` | 🔑 Basic-auth password |

> Requests are sent as `POST` with `Content-Type: application/json` unless one of your
> `headers` sets `Content-Type` itself.

Incidents are posted as one JSON object. n8n and Zapier receive the same
object. Plain operator messages keep their own small shape
(`{"Cluster","Message"}` for the webhook).

| Field | Meaning |
|:--|:--|
| `cluster` | Configured cluster name |
| `key` | Incident key, the same for every message of one incident |
| `alertKey` | `kwatch-<cluster>-<key>`, the id to deduplicate on |
| `revision` | Increasing number that orders messages of one incident |
| `status` | `critical`, `warning`, `flapping`, `resolved` or `info` |
| `resolved` | `true` only on the message that closes the incident |
| `marker` | The one status emoji |
| `short` | The marker and the lead sentence |
| `note` | The full narrative as one plain paragraph, starting with the marker |
| `markdown` | The same narrative as CommonMark: short lines, bold names, code spans and a code block for the command (omitted when the message has no structure) |
| `title` | One line: what is wrong and where |
| `lines` | Explanation sentences (omitted when empty) |
| `timeline` | Relevant events, oldest first (omitted when empty) |
| `output` | The application's recent output, redacted (omitted when empty) |
| `steps` | Suggested actions as `{"Text","Command","Mutating"}` (omitted when empty) |
| `confidence` | How sure the cause is (omitted when empty) |
| `route` | `{"Namespaces","Reasons","Severity"}` used by routing rules |
| `opens` | `true` on the message that announces the incident (omitted otherwise) |
| `pagingOnly` | `true` on a message meant for receivers that track incidents by `alertKey`: the close of an incident whose failures another incident took over. If you opened that `alertKey`, close it (omitted otherwise) |
| `skipPaging` | `true` on the resolve of an incident whose announcement was never sent to receivers that track alerts by key, because a digest or summary carried it. There may be nothing for you to close (omitted otherwise) |
| `carrier` | `digest`, `roll-up` or `startup summary`: the message that carries this one to people (omitted otherwise) |
| `reopenWithinSeconds` | On a resolve that may reopen: a failure within this many seconds continues the same incident (omitted otherwise) |

The same flag fields (`opens`, `pagingOnly`, `skipPaging`, `carrier`,
`reopenWithinSeconds`) are added to Splunk events when set.
