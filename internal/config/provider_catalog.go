package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ProviderField describes one field supported by the guided installer.
// Type is one of string, integer, boolean, list, json, or headers.
type ProviderField struct {
	Provider    string
	DisplayName string
	Field       string
	Type        string
	Required    bool
	Secret      bool
	Validation  string
	Default     string
	Description string
}

const providerCatalogData = "" +
	"slack|Slack|webhook|string|false|true|url||Slack webhook URL\n" +
	"slack|Slack|channel|string|false|false|||Override channel\n" +
	"slack|Slack|title|string|false|false|||Custom title\n" +
	"slack|Slack|text|string|false|false|||Custom text\n" +
	"slack|Slack|compact|boolean|false|false|boolean|false|Single-line mo" +
	"de\n" +
	"slack|Slack|token|string|false|true|||Bot token (xoxb-...)\n" +
	"discord|Discord|webhook|string|true|true|url||Discord webhook URL\n" +
	"discord|Discord|title|string|false|false|||Custom title\n" +
	"discord|Discord|text|string|false|false|||Custom text\n" +
	"email|Email|from|string|true|false|||From address\n" +
	"email|Email|password|string|true|true|||From password\n" +
	"email|Email|host|string|true|false|||SMTP host\n" +
	"email|Email|port|string|true|false|port||SMTP port\n" +
	"email|Email|to|string|true|false|||Receiver email\n" +
	"line|LINE|token|string|true|true|||LINE Notify access token\n" +
	"pagerduty|PagerDuty|integrationKey|string|true|true|||PagerDuty inte" +
	"gration key\n" +
	"telegram|Telegram|token|string|true|true|||Bot token\n" +
	"telegram|Telegram|chatId|string|true|false|telegram-chat-id||Chat ID\n" +
	"teams|Teams|webhook|string|true|true|url||Webhook URL\n" +
	"teams|Teams|title|string|false|false|||Custom title\n" +
	"teams|Teams|text|string|false|false|||Custom text\n" +
	"rocketchat|Rocket.Chat|webhook|string|true|true|url||Webhook URL\n" +
	"rocketchat|Rocket.Chat|text|string|false|false|||Custom text\n" +
	"mattermost|Mattermost|webhook|string|true|true|url||Webhook URL\n" +
	"mattermost|Mattermost|title|string|false|false|||Custom title\n" +
	"mattermost|Mattermost|text|string|false|false|||Custom text\n" +
	"opsgenie|Opsgenie|apiKey|string|true|true|||API Key\n" +
	"opsgenie|Opsgenie|title|string|false|false|||Custom title\n" +
	"opsgenie|Opsgenie|text|string|false|false|||Custom text\n" +
	"matrix|Matrix|homeServer|string|true|false|url||HomeServer URL\n" +
	"matrix|Matrix|accessToken|string|true|true|||Access token\n" +
	"matrix|Matrix|internalRoomId|string|true|false|||Room ID\n" +
	"matrix|Matrix|title|string|false|false|||Custom title\n" +
	"matrix|Matrix|text|string|false|false|||Custom text\n" +
	"dingtalk|Dingtalk|accessToken|string|true|true|||Access token\n" +
	"dingtalk|Dingtalk|secret|string|false|true|||Signing secret\n" +
	"dingtalk|Dingtalk|title|string|false|false|||Custom title\n" +
	"feishu|Feishu|webhook|string|true|true|url||Webhook URL\n" +
	"feishu|Feishu|title|string|false|false|||Custom title\n" +
	"zenduty|Zenduty|integrationKey|string|true|true|||Integration Key\n" +
	"zenduty|Zenduty|alertType|string|false|false||critical|Alert type (d" +
	"efault: critical)\n" +
	"googlechat|Google Chat|webhook|string|true|true|url||Webhook URL\n" +
	"googlechat|Google Chat|text|string|false|false|||Custom text\n" +
	"gotify|Gotify|url|string|true|false|url||Gotify server URL\n" +
	"gotify|Gotify|token|string|true|true|||App token\n" +
	"gotify|Gotify|priority|integer|false|false|integer||Priority (option" +
	"al)\n" +
	"gotify|Gotify|title|string|false|false|||Custom title\n" +
	"ntfy|ntfy|topic|string|true|true|||Topic to publish to\n" +
	"ntfy|ntfy|url|string|false|false|url|https://ntfy.sh|Server URL (def" +
	"ault: https://ntfy.sh)\n" +
	"ntfy|ntfy|token|string|false|true|||Optional auth token\n" +
	"ntfy|ntfy|title|string|false|false|||Custom title\n" +
	"ntfy|ntfy|priority|integer|false|false|integer|4|Priority 1-5 (defau" +
	"lt: 4)\n" +
	"pushover|Pushover|token|string|true|true|||Application token\n" +
	"pushover|Pushover|user|string|true|true|||User or group key\n" +
	"pushover|Pushover|priority|integer|false|false|integer||Priority (op" +
	"tional)\n" +
	"pushover|Pushover|title|string|false|false|||Custom title\n" +
	"webex|Webex|accessToken|string|true|true|||Bot access token\n" +
	"webex|Webex|roomId|string|false|false|||Room ID (optional)\n" +
	"webex|Webex|toPersonEmail|string|false|false|||Person email (optiona" +
	"l)\n" +
	"github|GitHub|token|string|true|true|||Personal access token\n" +
	"github|GitHub|owner|string|true|false|||Repository owner\n" +
	"github|GitHub|repo|string|true|false|||Repository name\n" +
	"github|GitHub|url|string|false|false|url||Optional endpoint override" +
	" (e.g. GitHub Enterprise)\n" +
	"gitlab|GitLab|token|string|true|true|||Personal access token\n" +
	"gitlab|GitLab|projectId|string|true|false|||Project ID\n" +
	"gitlab|GitLab|url|string|false|false|url||Optional endpoint override" +
	" (e.g. self-hosted GitLab)\n" +
	"gitea|Gitea|token|string|true|true|||Access token\n" +
	"gitea|Gitea|owner|string|true|false|||Repository owner\n" +
	"gitea|Gitea|repo|string|true|false|||Repository name\n" +
	"gitea|Gitea|url|string|false|false|url||Optional endpoint override (" +
	"e.g. self-hosted Gitea)\n" +
	"zapier|Zapier|url|string|true|true|url||Zap webhook URL\n" +
	"zapier|Zapier|token|string|false|true|||Optional token\n" +
	"zapier|Zapier|title|string|false|false|||Custom title\n" +
	"n8n|n8n|url|string|true|true|url||Workflow webhook URL\n" +
	"n8n|n8n|token|string|false|true|||Optional auth header value\n" +
	"n8n|n8n|title|string|false|false|||Custom title\n" +
	"ifttt|IFTTT|key|string|true|true|||Webhooks key\n" +
	"ifttt|IFTTT|event|string|false|false||kwatch|Event name (default: kw" +
	"atch)\n" +
	"teamsworkflow|Teams Workflow|webhook|string|true|true|url||Power Aut" +
	"omate / Teams Workflow URL\n" +
	"zulip|Zulip|email|string|true|false|||Bot email\n" +
	"zulip|Zulip|token|string|true|true|||Bot API key\n" +
	"zulip|Zulip|channel|string|true|false|||Channel/stream to post to\n" +
	"zulip|Zulip|url|string|false|false|url|https://zulip.example.com/api" +
	"/v1/messages|Server URL (default: https://zulip.example.com/api/v1/m" +
	"essages)\n" +
	"zulip|Zulip|title|string|false|false|||Custom title\n" +
	"homeassistant|Home Assistant|token|string|true|true|||Long-lived acc" +
	"ess token\n" +
	"homeassistant|Home Assistant|url|string|false|false|url|http://local" +
	"host:8123|Server URL (default: http://localhost:8123)\n" +
	"homeassistant|Home Assistant|service|string|false|false||notify|Noti" +
	"fication service (default: notify)\n" +
	"splunk|Splunk|url|string|true|false|url||HEC endpoint URL\n" +
	"splunk|Splunk|token|string|true|true|||HEC token\n" +
	"splunk|Splunk|source|string|false|false|||Source name (optional)\n" +
	"splunk|Splunk|sourcetype|string|false|false|||Source type (optional)\n" +
	"splunk|Splunk|index|string|false|false|||Index name (optional)\n" +
	"splunk|Splunk|host|string|false|false|||Host name (optional)\n" +
	"datadog|Datadog|apiKey|string|true|true|||API key\n" +
	"datadog|Datadog|site|string|false|false||datadoghq.com|Datadog site " +
	"(default: datadoghq.com)\n" +
	"datadog|Datadog|applicationKey|string|false|true|||Optional applicat" +
	"ion key\n" +
	"datadog|Datadog|title|string|false|false|||Custom title\n" +
	"datadog|Datadog|alertType|string|false|false||error|Alert type (defa" +
	"ult: error)\n" +
	"datadog|Datadog|tags|list|false|false|list||Comma-separated tags\n" +
	"newrelic|New Relic|apiKey|string|true|true|||User API key\n" +
	"newrelic|New Relic|accountId|string|true|false|||Account ID\n" +
	"clickup|ClickUp|token|string|true|true|||Personal API token\n" +
	"clickup|ClickUp|listId|string|true|false|||List ID to create tasks i" +
	"n\n" +
	"clickup|ClickUp|priority|integer|false|false|integer||Optional task " +
	"priority (1-4)\n" +
	"ilert|iLert|integrationKey|string|true|true|||Integration key\n" +
	"ilert|iLert|priority|integer|false|false|integer||Priority (LOW/HIGH" +
	"/CRITICAL, default: HIGH)\n" +
	"incidentio|Incident.io|url|string|true|true|url||Incident.io URL\n" +
	"incidentio|Incident.io|apiKey|string|false|true|||Optional API key\n" +
	"squadcast|Squadcast|serviceKey|string|true|true|||Service key\n" +
	"signl4|SIGNL4|teamSecret|string|true|true|||Team secret\n" +
	"signl4|SIGNL4|title|string|false|false|||Custom title\n" +
	"signl4|SIGNL4|user|string|false|false|||Optional alerting user\n" +
	"signl4|SIGNL4|url|string|false|false|url||Optional endpoint override\n" +
	"twilio|Twilio|accountSid|string|true|true|||Account SID\n" +
	"twilio|Twilio|authToken|string|true|true|||Auth token\n" +
	"twilio|Twilio|from|string|true|false|||Sender phone number\n" +
	"twilio|Twilio|to|string|true|false|||Recipient phone number\n" +
	"vonage|Vonage|apiKey|string|true|true|||API key\n" +
	"vonage|Vonage|apiSecret|string|true|true|||API secret\n" +
	"vonage|Vonage|from|string|true|false|||Sender name/number\n" +
	"vonage|Vonage|to|string|true|false|||Recipient phone number\n" +
	"plivo|Plivo|authId|string|true|true|||Auth ID\n" +
	"plivo|Plivo|authToken|string|true|true|||Auth token\n" +
	"plivo|Plivo|from|string|true|false|||Sender number\n" +
	"plivo|Plivo|to|string|true|false|||Recipient phone number\n" +
	"messagebird|MessageBird|accessKey|string|true|true|||Access key\n" +
	"messagebird|MessageBird|from|string|true|false|||Sender number\n" +
	"messagebird|MessageBird|to|string|true|false|||Recipient phone numbe" +
	"r\n" +
	"signal|Signal|number|string|true|false|||Sender phone number\n" +
	"signal|Signal|to|string|true|false|||Recipient phone number\n" +
	"signal|Signal|url|string|false|false|url|http://localhost:8080|REST " +
	"API URL (default: http://localhost:8080)\n" +
	"sendgrid|SendGrid|apiKey|string|true|true|||API key\n" +
	"sendgrid|SendGrid|from|string|true|false|||From address\n" +
	"sendgrid|SendGrid|to|list|true|false|list||Recipients (list of addre" +
	"sses)\n" +
	"sendgrid|SendGrid|subject|string|false|false|||Email subject\n" +
	"ses|SES|accessKeyId|string|true|true|||AWS access key ID\n" +
	"ses|SES|secretAccessKey|string|true|true|||AWS secret access key\n" +
	"ses|SES|region|string|false|false||us-east-1|AWS region (default: us" +
	"-east-1)\n" +
	"ses|SES|from|string|true|false|||Verified sender address\n" +
	"ses|SES|to|string|true|false|||Recipients (comma-separated)\n" +
	"ses|SES|subject|string|false|false|||Email subject\n" +
	"sns|SNS|accessKeyId|string|true|true|||AWS access key ID\n" +
	"sns|SNS|secretAccessKey|string|true|true|||AWS secret access key\n" +
	"sns|SNS|region|string|false|false||us-east-1|AWS region (default: us" +
	"-east-1)\n" +
	"sns|SNS|topicArn|string|false|false|||SNS topic ARN (or targetArn)\n" +
	"sns|SNS|targetArn|string|false|false|||SNS target ARN (alternative t" +
	"o topicArn).\n" +
	"sns|SNS|subject|string|false|false|||Optional subject (email subscri" +
	"ptions)\n" +
	"jira|Jira|url|string|true|false|url||Jira base URL\n" +
	"jira|Jira|user|string|true|false|||Email or username\n" +
	"jira|Jira|apiToken|string|true|true|||API token\n" +
	"jira|Jira|projectKey|string|true|false|||Project key\n" +
	"jira|Jira|issueType|string|false|false||Task|Issue type (default: Ta" +
	"sk)\n" +
	"wecom|WeCom|webhook|string|true|true|url||Group robot webhook URL\n" +
	"splunkoncall|Splunk On-Call|apiKey|string|true|true|||API key\n" +
	"splunkoncall|Splunk On-Call|routingKey|string|true|true|||Routing ke" +
	"y\n" +
	"splunkoncall|Splunk On-Call|url|string|false|false|url||Optional end" +
	"point override\n" +
	"mailgun|Mailgun|apiKey|string|true|true|||API key\n" +
	"mailgun|Mailgun|domain|string|true|false|||Sending domain\n" +
	"mailgun|Mailgun|from|string|true|false|||From address\n" +
	"mailgun|Mailgun|to|string|true|false|||Recipients (comma-separated)\n" +
	"mailgun|Mailgun|subject|string|false|false|||Email subject\n" +
	"mailgun|Mailgun|url|string|false|false|url||Optional endpoint overri" +
	"de (e.g. EU region)\n" +
	"resend|Resend|apiKey|string|true|true|||API key\n" +
	"resend|Resend|from|string|true|false|||From address\n" +
	"resend|Resend|to|string|true|false|||Recipients (comma-separated)\n" +
	"resend|Resend|subject|string|false|false|||Email subject\n" +
	"goalert|GoAlert|url|string|false|false|url|https://goalert.example.c" +
	"om|GoAlert URL (default: https://goalert.example.com)\n" +
	"goalert|GoAlert|token|string|true|true|||API token\n" +
	"goalert|GoAlert|serviceId|string|true|false|||Service ID\n" +
	"alerta|Alerta|url|string|true|false|url||Alerta server URL\n" +
	"alerta|Alerta|apiKey|string|true|true|||API key\n" +
	"alerta|Alerta|environment|string|false|false||Production|Environment" +
	" (default: Production)\n" +
	"alerta|Alerta|service|string|false|false||kwatch|Service name (defau" +
	"lt: kwatch)\n" +
	"threema|Threema|gatewayId|string|true|true|||Threema Gateway ID\n" +
	"threema|Threema|secret|string|true|true|||Gateway secret\n" +
	"threema|Threema|to|string|true|false|||Recipient Threema ID\n" +
	"flock|Flock|webhook|string|true|true|url||Incoming webhook URL\n" +
	"pushbullet|Pushbullet|accessToken|string|true|true|||Access token\n" +
	"sensugo|SensiGo|url|string|true|false|url||Sensu Go API URL\n" +
	"sensugo|SensiGo|apiKey|string|true|true|||API key\n" +
	"sensugo|SensiGo|namespace|string|false|false||default|Namespace (def" +
	"ault: default)\n" +
	"sensugo|SensiGo|entity|string|false|false||kwatch|Entity name (defau" +
	"lt: kwatch)\n" +
	"webhook|Generic Webhook|url|string|true|true|url||Webhook URL\n" +
	"webhook|Generic Webhook|headers|headers|false|false|||Custom headers\n" +
	"webhook|Generic Webhook|basicAuth.username|string|false|false|||Basi" +
	"c-auth username.\n" +
	"webhook|Generic Webhook|basicAuth.password|string|false|true|||Basic" +
	"-auth password.\n" +
	"slack|Slack|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"slack|Slack|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"slack|Slack|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"slack|Slack|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"discord|Discord|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"discord|Discord|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"discord|Discord|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"discord|Discord|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"email|Email|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"email|Email|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"email|Email|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"email|Email|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"line|LINE|routes|json|false|false|json||Optional JSON route filters.\n" +
	"line|LINE|retry.maxAttempts|integer|false|false|integer||Optional ma" +
	"ximum retry attempts.\n" +
	"line|LINE|retry.delay|string|false|false|||Optional retry delay, for" +
	" example 5s.\n" +
	"line|LINE|fallback|string|false|false|||Optional fallback provider n" +
	"ame.\n" +
	"pagerduty|PagerDuty|routes|json|false|false|json||Optional JSON rout" +
	"e filters.\n" +
	"pagerduty|PagerDuty|retry.maxAttempts|integer|false|false|integer||O" +
	"ptional maximum retry attempts.\n" +
	"pagerduty|PagerDuty|retry.delay|string|false|false|||Optional retry " +
	"delay, for example 5s.\n" +
	"pagerduty|PagerDuty|fallback|string|false|false|||Optional fallback " +
	"provider name.\n" +
	"telegram|Telegram|routes|json|false|false|json||Optional JSON route " +
	"filters.\n" +
	"telegram|Telegram|retry.maxAttempts|integer|false|false|integer||Opt" +
	"ional maximum retry attempts.\n" +
	"telegram|Telegram|retry.delay|string|false|false|||Optional retry de" +
	"lay, for example 5s.\n" +
	"telegram|Telegram|fallback|string|false|false|||Optional fallback pr" +
	"ovider name.\n" +
	"teams|Teams|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"teams|Teams|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"teams|Teams|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"teams|Teams|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"rocketchat|Rocket.Chat|routes|json|false|false|json||Optional JSON r" +
	"oute filters.\n" +
	"rocketchat|Rocket.Chat|retry.maxAttempts|integer|false|false|integer" +
	"||Optional maximum retry attempts.\n" +
	"rocketchat|Rocket.Chat|retry.delay|string|false|false|||Optional ret" +
	"ry delay, for example 5s.\n" +
	"rocketchat|Rocket.Chat|fallback|string|false|false|||Optional fallba" +
	"ck provider name.\n" +
	"mattermost|Mattermost|routes|json|false|false|json||Optional JSON ro" +
	"ute filters.\n" +
	"mattermost|Mattermost|retry.maxAttempts|integer|false|false|integer|" +
	"|Optional maximum retry attempts.\n" +
	"mattermost|Mattermost|retry.delay|string|false|false|||Optional retr" +
	"y delay, for example 5s.\n" +
	"mattermost|Mattermost|fallback|string|false|false|||Optional fallbac" +
	"k provider name.\n" +
	"opsgenie|Opsgenie|routes|json|false|false|json||Optional JSON route " +
	"filters.\n" +
	"opsgenie|Opsgenie|retry.maxAttempts|integer|false|false|integer||Opt" +
	"ional maximum retry attempts.\n" +
	"opsgenie|Opsgenie|retry.delay|string|false|false|||Optional retry de" +
	"lay, for example 5s.\n" +
	"opsgenie|Opsgenie|fallback|string|false|false|||Optional fallback pr" +
	"ovider name.\n" +
	"matrix|Matrix|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"matrix|Matrix|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"matrix|Matrix|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"matrix|Matrix|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"dingtalk|Dingtalk|routes|json|false|false|json||Optional JSON route " +
	"filters.\n" +
	"dingtalk|Dingtalk|retry.maxAttempts|integer|false|false|integer||Opt" +
	"ional maximum retry attempts.\n" +
	"dingtalk|Dingtalk|retry.delay|string|false|false|||Optional retry de" +
	"lay, for example 5s.\n" +
	"dingtalk|Dingtalk|fallback|string|false|false|||Optional fallback pr" +
	"ovider name.\n" +
	"feishu|Feishu|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"feishu|Feishu|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"feishu|Feishu|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"feishu|Feishu|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"zenduty|Zenduty|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"zenduty|Zenduty|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"zenduty|Zenduty|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"zenduty|Zenduty|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"googlechat|Google Chat|routes|json|false|false|json||Optional JSON r" +
	"oute filters.\n" +
	"googlechat|Google Chat|retry.maxAttempts|integer|false|false|integer" +
	"||Optional maximum retry attempts.\n" +
	"googlechat|Google Chat|retry.delay|string|false|false|||Optional ret" +
	"ry delay, for example 5s.\n" +
	"googlechat|Google Chat|fallback|string|false|false|||Optional fallba" +
	"ck provider name.\n" +
	"gotify|Gotify|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"gotify|Gotify|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"gotify|Gotify|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"gotify|Gotify|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"ntfy|ntfy|routes|json|false|false|json||Optional JSON route filters.\n" +
	"ntfy|ntfy|retry.maxAttempts|integer|false|false|integer||Optional ma" +
	"ximum retry attempts.\n" +
	"ntfy|ntfy|retry.delay|string|false|false|||Optional retry delay, for" +
	" example 5s.\n" +
	"ntfy|ntfy|fallback|string|false|false|||Optional fallback provider n" +
	"ame.\n" +
	"pushover|Pushover|routes|json|false|false|json||Optional JSON route " +
	"filters.\n" +
	"pushover|Pushover|retry.maxAttempts|integer|false|false|integer||Opt" +
	"ional maximum retry attempts.\n" +
	"pushover|Pushover|retry.delay|string|false|false|||Optional retry de" +
	"lay, for example 5s.\n" +
	"pushover|Pushover|fallback|string|false|false|||Optional fallback pr" +
	"ovider name.\n" +
	"webex|Webex|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"webex|Webex|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"webex|Webex|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"webex|Webex|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"github|GitHub|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"github|GitHub|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"github|GitHub|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"github|GitHub|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"gitlab|GitLab|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"gitlab|GitLab|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"gitlab|GitLab|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"gitlab|GitLab|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"gitea|Gitea|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"gitea|Gitea|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"gitea|Gitea|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"gitea|Gitea|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"zapier|Zapier|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"zapier|Zapier|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"zapier|Zapier|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"zapier|Zapier|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"n8n|n8n|routes|json|false|false|json||Optional JSON route filters.\n" +
	"n8n|n8n|retry.maxAttempts|integer|false|false|integer||Optional maxi" +
	"mum retry attempts.\n" +
	"n8n|n8n|retry.delay|string|false|false|||Optional retry delay, for e" +
	"xample 5s.\n" +
	"n8n|n8n|fallback|string|false|false|||Optional fallback provider nam" +
	"e.\n" +
	"ifttt|IFTTT|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"ifttt|IFTTT|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"ifttt|IFTTT|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"ifttt|IFTTT|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"teamsworkflow|Teams Workflow|routes|json|false|false|json||Optional " +
	"JSON route filters.\n" +
	"teamsworkflow|Teams Workflow|retry.maxAttempts|integer|false|false|i" +
	"nteger||Optional maximum retry attempts.\n" +
	"teamsworkflow|Teams Workflow|retry.delay|string|false|false|||Option" +
	"al retry delay, for example 5s.\n" +
	"teamsworkflow|Teams Workflow|fallback|string|false|false|||Optional " +
	"fallback provider name.\n" +
	"zulip|Zulip|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"zulip|Zulip|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"zulip|Zulip|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"zulip|Zulip|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"homeassistant|Home Assistant|routes|json|false|false|json||Optional " +
	"JSON route filters.\n" +
	"homeassistant|Home Assistant|retry.maxAttempts|integer|false|false|i" +
	"nteger||Optional maximum retry attempts.\n" +
	"homeassistant|Home Assistant|retry.delay|string|false|false|||Option" +
	"al retry delay, for example 5s.\n" +
	"homeassistant|Home Assistant|fallback|string|false|false|||Optional " +
	"fallback provider name.\n" +
	"splunk|Splunk|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"splunk|Splunk|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"splunk|Splunk|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"splunk|Splunk|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"datadog|Datadog|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"datadog|Datadog|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"datadog|Datadog|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"datadog|Datadog|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"newrelic|New Relic|routes|json|false|false|json||Optional JSON route" +
	" filters.\n" +
	"newrelic|New Relic|retry.maxAttempts|integer|false|false|integer||Op" +
	"tional maximum retry attempts.\n" +
	"newrelic|New Relic|retry.delay|string|false|false|||Optional retry d" +
	"elay, for example 5s.\n" +
	"newrelic|New Relic|fallback|string|false|false|||Optional fallback p" +
	"rovider name.\n" +
	"clickup|ClickUp|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"clickup|ClickUp|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"clickup|ClickUp|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"clickup|ClickUp|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"ilert|iLert|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"ilert|iLert|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"ilert|iLert|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"ilert|iLert|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"incidentio|Incident.io|routes|json|false|false|json||Optional JSON r" +
	"oute filters.\n" +
	"incidentio|Incident.io|retry.maxAttempts|integer|false|false|integer" +
	"||Optional maximum retry attempts.\n" +
	"incidentio|Incident.io|retry.delay|string|false|false|||Optional ret" +
	"ry delay, for example 5s.\n" +
	"incidentio|Incident.io|fallback|string|false|false|||Optional fallba" +
	"ck provider name.\n" +
	"squadcast|Squadcast|routes|json|false|false|json||Optional JSON rout" +
	"e filters.\n" +
	"squadcast|Squadcast|retry.maxAttempts|integer|false|false|integer||O" +
	"ptional maximum retry attempts.\n" +
	"squadcast|Squadcast|retry.delay|string|false|false|||Optional retry " +
	"delay, for example 5s.\n" +
	"squadcast|Squadcast|fallback|string|false|false|||Optional fallback " +
	"provider name.\n" +
	"signl4|SIGNL4|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"signl4|SIGNL4|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"signl4|SIGNL4|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"signl4|SIGNL4|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"twilio|Twilio|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"twilio|Twilio|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"twilio|Twilio|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"twilio|Twilio|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"vonage|Vonage|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"vonage|Vonage|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"vonage|Vonage|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"vonage|Vonage|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"plivo|Plivo|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"plivo|Plivo|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"plivo|Plivo|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"plivo|Plivo|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"messagebird|MessageBird|routes|json|false|false|json||Optional JSON " +
	"route filters.\n" +
	"messagebird|MessageBird|retry.maxAttempts|integer|false|false|intege" +
	"r||Optional maximum retry attempts.\n" +
	"messagebird|MessageBird|retry.delay|string|false|false|||Optional re" +
	"try delay, for example 5s.\n" +
	"messagebird|MessageBird|fallback|string|false|false|||Optional fallb" +
	"ack provider name.\n" +
	"signal|Signal|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"signal|Signal|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"signal|Signal|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"signal|Signal|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"sendgrid|SendGrid|routes|json|false|false|json||Optional JSON route " +
	"filters.\n" +
	"sendgrid|SendGrid|retry.maxAttempts|integer|false|false|integer||Opt" +
	"ional maximum retry attempts.\n" +
	"sendgrid|SendGrid|retry.delay|string|false|false|||Optional retry de" +
	"lay, for example 5s.\n" +
	"sendgrid|SendGrid|fallback|string|false|false|||Optional fallback pr" +
	"ovider name.\n" +
	"ses|SES|routes|json|false|false|json||Optional JSON route filters.\n" +
	"ses|SES|retry.maxAttempts|integer|false|false|integer||Optional maxi" +
	"mum retry attempts.\n" +
	"ses|SES|retry.delay|string|false|false|||Optional retry delay, for e" +
	"xample 5s.\n" +
	"ses|SES|fallback|string|false|false|||Optional fallback provider nam" +
	"e.\n" +
	"sns|SNS|routes|json|false|false|json||Optional JSON route filters.\n" +
	"sns|SNS|retry.maxAttempts|integer|false|false|integer||Optional maxi" +
	"mum retry attempts.\n" +
	"sns|SNS|retry.delay|string|false|false|||Optional retry delay, for e" +
	"xample 5s.\n" +
	"sns|SNS|fallback|string|false|false|||Optional fallback provider nam" +
	"e.\n" +
	"jira|Jira|routes|json|false|false|json||Optional JSON route filters.\n" +
	"jira|Jira|retry.maxAttempts|integer|false|false|integer||Optional ma" +
	"ximum retry attempts.\n" +
	"jira|Jira|retry.delay|string|false|false|||Optional retry delay, for" +
	" example 5s.\n" +
	"jira|Jira|fallback|string|false|false|||Optional fallback provider n" +
	"ame.\n" +
	"wecom|WeCom|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"wecom|WeCom|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"wecom|WeCom|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"wecom|WeCom|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"splunkoncall|Splunk On-Call|routes|json|false|false|json||Optional J" +
	"SON route filters.\n" +
	"splunkoncall|Splunk On-Call|retry.maxAttempts|integer|false|false|in" +
	"teger||Optional maximum retry attempts.\n" +
	"splunkoncall|Splunk On-Call|retry.delay|string|false|false|||Optiona" +
	"l retry delay, for example 5s.\n" +
	"splunkoncall|Splunk On-Call|fallback|string|false|false|||Optional f" +
	"allback provider name.\n" +
	"mailgun|Mailgun|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"mailgun|Mailgun|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"mailgun|Mailgun|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"mailgun|Mailgun|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"resend|Resend|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"resend|Resend|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"resend|Resend|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"resend|Resend|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"goalert|GoAlert|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"goalert|GoAlert|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"goalert|GoAlert|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"goalert|GoAlert|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"alerta|Alerta|routes|json|false|false|json||Optional JSON route filt" +
	"ers.\n" +
	"alerta|Alerta|retry.maxAttempts|integer|false|false|integer||Optiona" +
	"l maximum retry attempts.\n" +
	"alerta|Alerta|retry.delay|string|false|false|||Optional retry delay," +
	" for example 5s.\n" +
	"alerta|Alerta|fallback|string|false|false|||Optional fallback provid" +
	"er name.\n" +
	"threema|Threema|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"threema|Threema|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"threema|Threema|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"threema|Threema|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"flock|Flock|routes|json|false|false|json||Optional JSON route filter" +
	"s.\n" +
	"flock|Flock|retry.maxAttempts|integer|false|false|integer||Optional " +
	"maximum retry attempts.\n" +
	"flock|Flock|retry.delay|string|false|false|||Optional retry delay, f" +
	"or example 5s.\n" +
	"flock|Flock|fallback|string|false|false|||Optional fallback provider" +
	" name.\n" +
	"pushbullet|Pushbullet|routes|json|false|false|json||Optional JSON ro" +
	"ute filters.\n" +
	"pushbullet|Pushbullet|retry.maxAttempts|integer|false|false|integer|" +
	"|Optional maximum retry attempts.\n" +
	"pushbullet|Pushbullet|retry.delay|string|false|false|||Optional retr" +
	"y delay, for example 5s.\n" +
	"pushbullet|Pushbullet|fallback|string|false|false|||Optional fallbac" +
	"k provider name.\n" +
	"sensugo|SensiGo|routes|json|false|false|json||Optional JSON route fi" +
	"lters.\n" +
	"sensugo|SensiGo|retry.maxAttempts|integer|false|false|integer||Optio" +
	"nal maximum retry attempts.\n" +
	"sensugo|SensiGo|retry.delay|string|false|false|||Optional retry dela" +
	"y, for example 5s.\n" +
	"sensugo|SensiGo|fallback|string|false|false|||Optional fallback prov" +
	"ider name.\n" +
	"webhook|Generic Webhook|routes|json|false|false|json||Optional JSON " +
	"route filters.\n" +
	"webhook|Generic Webhook|retry.maxAttempts|integer|false|false|intege" +
	"r||Optional maximum retry attempts.\n" +
	"webhook|Generic Webhook|retry.delay|string|false|false|||Optional re" +
	"try delay, for example 5s.\n" +
	"webhook|Generic Webhook|fallback|string|false|false|||Optional fallb" +
	"ack provider name.\n"

var providerCatalog = parseProviderCatalog(providerCatalogData)

func parseProviderCatalog(raw string) []ProviderField {
	var fields []ProviderField
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 9)
		if len(parts) != 9 {
			panic(fmt.Sprintf("invalid provider catalog row: %q", line))
		}
		required, err := strconv.ParseBool(parts[4])
		if err != nil {
			panic(fmt.Sprintf("invalid provider catalog required flag: %q", line))
		}
		secret, err := strconv.ParseBool(parts[5])
		if err != nil {
			panic(fmt.Sprintf("invalid provider catalog secret flag: %q", line))
		}
		fields = append(fields, ProviderField{
			Provider: parts[0], DisplayName: parts[1], Field: parts[2],
			Type: parts[3], Required: required, Secret: secret,
			Validation: parts[6], Default: parts[7], Description: parts[8],
		})
	}
	return fields
}

// ProviderCatalog returns a copy of the complete guided installer schema.
func ProviderCatalog() []ProviderField {
	result := make([]ProviderField, len(providerCatalog))
	copy(result, providerCatalog)
	return result
}
