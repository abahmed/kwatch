# 📣 Alert providers

This is the complete provider reference for kwatch. If you are choosing a
channel for the first time, start with the [quick-start guide](../README.md)
or the [channel picker on kwatch.dev](https://kwatch.dev/docs/channels).

In simple terms: configure a provider under `alert:`, give kwatch its webhook
or credential, and it will send incidents to that destination.

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
- **Every provider says the same thing.** All 56 providers render from one report — the
  plain-English headline, the diagnosis (why, impact, what changed), the deduplicated hint,
  the short image and node names, the evidence. Slack in token mode lays it out as blocks;
  everyone else gets the same content as text. No provider is a second-class citizen.
- **Reliability is built in.** Every provider shares the same routing, retry, and fallback
  controls (shown at the top under Slack — they apply to all providers).
- **One HTTP path.** Every provider that talks HTTP sends through the same helper
  (`alert/util.Send`), so a `429` is always honoured with its `Retry-After`, a `4xx` is never
  retried (the payload will not get better), and a `5xx` or network error always is. A
  provider cannot have its own idea of what a status code means — the linter rejects raw
  `net/http` calls under `internal/alert/`.

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
| `alert.slack.title` | ✏️ Custom title |
| `alert.slack.text` | ✏️ Custom text |
| `alert.slack.compact` | 📏 Single-line mode |

**Bot Token mode:**
| Parameter | What it does |
|:---|---|
| `alert.slack.token` | 🔑 Bot token (`xoxb-...`) |
| `alert.slack.channel` | 📢 Channel to post to |
| `alert.slack.title` | ✏️ Custom title |
| `alert.slack.text` | ✏️ Custom text |
| `alert.slack.compact` | 📏 Single-line mode |

**Compact mode:**
```yaml
alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
    compact: true
```

> 💡 **Pro tip:** When using bot token mode, alerts become threaded conversations — root message on first alert, updates as replies. Clean and organized! 🧹

#### 📮 Routing, Retry & Fallback (applies to every provider)

In plain words: the same options work for all providers, not just Slack.

- **`routes`** — if you have several *channels* for one provider, send only some alerts to
  each (filtered by namespace or severity).
- **`retry`** — how hard kwatch tries before giving up: `maxAttempts` times, waiting `delay`
  between tries. Only failures that *can* succeed on a retry are retried — a timeout, a 5xx,
  a rate limit (which waits exactly as long as the provider's `Retry-After` asks). A failure
  that says the request itself is wrong — a 4xx, a rejected payload, an unknown channel, a
  revoked token — is given up on immediately and goes straight to the dead-letter queue, so
  it cannot hold up the alerts queued behind it.
- **`fallback`** — if this provider fails for good, hand the alert to another provider
  (e.g. Slack → PagerDuty when Slack is down).

```yaml
alert:
  slack:
    webhook: "${file:/config/slack-webhook}"
    routes:
      - namespaces: ["production"]
        severities: ["high", "critical"]
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
| `alert.discord.title` | ✏️ Custom title |
| `alert.discord.text` | ✏️ Custom text |

### 📧 Email

**What it is:** good old SMTP email to one or more inboxes — a simple paper trail anyone can
search.

| Parameter | What it does |
|:---|---|
| `alert.email.from` | 📤 From address |
| `alert.email.password` | 🔑 From password |
| `alert.email.host` | 🖥️ SMTP host |
| `alert.email.port` | 🔌 SMTP port |
| `alert.email.to` | 📥 Receiver email |

### 💬 LINE

| Parameter | What it does |
|:---|---|
| `alert.line.token` | 🔑 LINE Notify access token |

```yaml
alert:
  line:
    token: "${file:/config/line-token}"
```

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
| `alert.teams.text` | ✏️ Custom text |

> `alert.teams.maxRetries` is ignored: Teams used to retry inside the provider on top of the
> shared delivery retry, so a rate-limited flow was hammered twice. Retries are now governed
> only by the shared retry settings above, like every other provider.

### 🚀 Rocket Chat

| Parameter | What it does |
|:---|---|
| `alert.rocketchat.webhook` | 🔗 Webhook URL |
| `alert.rocketchat.text` | ✏️ Custom text |

### 🌐 Mattermost

| Parameter | What it does |
|:---|---|
| `alert.mattermost.webhook` | 🔗 Webhook URL |
| `alert.mattermost.title` | ✏️ Custom title |
| `alert.mattermost.text` | ✏️ Custom text |

### 🔔 Opsgenie

| Parameter | What it does |
|:---|---|
| `alert.opsgenie.apiKey` | 🔑 API Key |
| `alert.opsgenie.title` | ✏️ Custom title |
| `alert.opsgenie.text` | ✏️ Custom text |

### 🏗️ Matrix

| Parameter | What it does |
|:---|---|
| `alert.matrix.homeServer` | 🖥️ HomeServer URL |
| `alert.matrix.accessToken` | 🔑 Access token |
| `alert.matrix.internalRoomId` | 🆔 Room ID |
| `alert.matrix.title` | ✏️ Custom title |
| `alert.matrix.text` | ✏️ Custom text |

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
| `alert.feishu.title` | ✏️ Custom title |

### 🛡️ Zenduty

| Parameter | What it does |
|:---|---|
| `alert.zenduty.integrationKey` | 🔑 Integration Key |
| `alert.zenduty.alertType` | 🏷️ Alert type (default: critical) |

### 💬 Google Chat

| Parameter | What it does |
|:---|---|
| `alert.googlechat.webhook` | 🔗 Webhook URL |
| `alert.googlechat.text` | ✏️ Custom text |

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
| `alert.ntfy.topic` | 📢 Topic to publish to |
| `alert.ntfy.url` | 🔗 Server URL (default: `https://ntfy.sh`) |
| `alert.ntfy.token` | 🔑 Optional auth token |
| `alert.ntfy.priority` | 🎚️ Priority 1-5 (default: 4) |

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
| `alert.pushover.priority` | 🎚️ Priority (optional) |
| `alert.pushover.title` | ✏️ Custom title |

### 🟣 Webex

| Parameter | What it does |
|:---|---|
| `alert.webex.accessToken` | 🔑 Bot access token |
| `alert.webex.roomId` | 🚪 Room ID (optional) |
| `alert.webex.toPersonEmail` | ✉️ Person email (optional) |

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

### ⚡ n8n

| Parameter | What it does |
|:---|---|
| `alert.n8n.url` | 🔗 Workflow webhook URL |
| `alert.n8n.token` | 🔑 Optional auth header value |

### 🧙 IFTTT

| Parameter | What it does |
|:---|---|
| `alert.ifttt.key` | 🔑 Webhooks key |
| `alert.ifttt.event` | 🎯 Event name (default: `kwatch`) |

```yaml
alert:
  ifttt:
    key: "d3L..."
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
| `alert.zulip.url` | 🔗 Server URL (default: `https://zulip.example.com/api/v1/messages`) |

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
| `alert.datadog.site` | 🌍 Datadog site (default: `datadoghq.com`) |
| `alert.datadog.applicationKey` | 🔑 Optional application key |
| `alert.datadog.alertType` | 🏷️ Alert type (default: `error`) |
| `alert.datadog.tags` | 🏷️ Comma-separated tags |

### 📈 New Relic

| Parameter | What it does |
|:---|---|
| `alert.newrelic.apiKey` | 🔑 User API key |
| `alert.newrelic.accountId` | 🆔 Account ID |

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
| `alert.sendgrid.subject` | ✏️ Email subject |

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
| `alert.ses.region` | 🌍 AWS region (default: `us-east-1`) |
| `alert.ses.from` | 📤 Verified sender address |
| `alert.ses.to` | 📥 Recipients (comma-separated) |
| `alert.ses.subject` | ✏️ Email subject |

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
| `alert.mailgun.subject` | ✏️ Email subject |
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
| `alert.resend.subject` | ✏️ Email subject |

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
| `alert.goalert.url` | 🔗 GoAlert URL (default: `https://goalert.example.com`) |
| `alert.goalert.token` | 🔑 API token |
| `alert.goalert.serviceId` | 🆔 Service ID |

```yaml
alert:
  goalert:
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
| `alert.webhook.basicAuth` | 🔐 Username + password |

> Requests are sent as `POST` with `Content-Type: application/json` unless one of your
> `headers` sets `Content-Type` itself.
