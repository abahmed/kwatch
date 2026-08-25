package alert

import (
	"github.com/abahmed/kwatch/internal/alert/alerta"
	"github.com/abahmed/kwatch/internal/alert/clickup"
	"github.com/abahmed/kwatch/internal/alert/datadog"
	"github.com/abahmed/kwatch/internal/alert/dingtalk"
	"github.com/abahmed/kwatch/internal/alert/discord"
	"github.com/abahmed/kwatch/internal/alert/email"
	"github.com/abahmed/kwatch/internal/alert/feishu"
	"github.com/abahmed/kwatch/internal/alert/flock"
	"github.com/abahmed/kwatch/internal/alert/gitea"
	"github.com/abahmed/kwatch/internal/alert/github"
	"github.com/abahmed/kwatch/internal/alert/gitlab"
	"github.com/abahmed/kwatch/internal/alert/goalert"
	"github.com/abahmed/kwatch/internal/alert/googlechat"
	"github.com/abahmed/kwatch/internal/alert/gotify"
	"github.com/abahmed/kwatch/internal/alert/homeassistant"
	"github.com/abahmed/kwatch/internal/alert/ifttt"
	"github.com/abahmed/kwatch/internal/alert/ilert"
	"github.com/abahmed/kwatch/internal/alert/incidentio"
	"github.com/abahmed/kwatch/internal/alert/jira"
	"github.com/abahmed/kwatch/internal/alert/line"
	"github.com/abahmed/kwatch/internal/alert/mailgun"
	"github.com/abahmed/kwatch/internal/alert/matrix"
	"github.com/abahmed/kwatch/internal/alert/mattermost"
	"github.com/abahmed/kwatch/internal/alert/messagebird"
	"github.com/abahmed/kwatch/internal/alert/n8n"
	"github.com/abahmed/kwatch/internal/alert/newrelic"
	"github.com/abahmed/kwatch/internal/alert/ntfy"
	"github.com/abahmed/kwatch/internal/alert/opsgenie"
	"github.com/abahmed/kwatch/internal/alert/pagerduty"
	"github.com/abahmed/kwatch/internal/alert/plivo"
	"github.com/abahmed/kwatch/internal/alert/pushbullet"
	"github.com/abahmed/kwatch/internal/alert/pushover"
	"github.com/abahmed/kwatch/internal/alert/resend"
	"github.com/abahmed/kwatch/internal/alert/rocketchat"
	"github.com/abahmed/kwatch/internal/alert/sendgrid"
	"github.com/abahmed/kwatch/internal/alert/sensugo"
	"github.com/abahmed/kwatch/internal/alert/ses"
	"github.com/abahmed/kwatch/internal/alert/signal"
	"github.com/abahmed/kwatch/internal/alert/signl4"
	"github.com/abahmed/kwatch/internal/alert/slack"
	"github.com/abahmed/kwatch/internal/alert/sns"
	"github.com/abahmed/kwatch/internal/alert/splunk"
	"github.com/abahmed/kwatch/internal/alert/splunkoncall"
	"github.com/abahmed/kwatch/internal/alert/squadcast"
	"github.com/abahmed/kwatch/internal/alert/teams"
	"github.com/abahmed/kwatch/internal/alert/teamsworkflow"
	"github.com/abahmed/kwatch/internal/alert/telegram"
	"github.com/abahmed/kwatch/internal/alert/threema"
	"github.com/abahmed/kwatch/internal/alert/twilio"
	"github.com/abahmed/kwatch/internal/alert/vonage"
	"github.com/abahmed/kwatch/internal/alert/webex"
	"github.com/abahmed/kwatch/internal/alert/webhook"
	"github.com/abahmed/kwatch/internal/alert/wecom"
	"github.com/abahmed/kwatch/internal/alert/zapier"
	"github.com/abahmed/kwatch/internal/alert/zenduty"
	"github.com/abahmed/kwatch/internal/alert/zulip"
	"github.com/abahmed/kwatch/internal/config"
)

// providerFactories maps a normalized provider key to its constructor.
// Providers are configured via the `alert` map in the config file.
var providerFactories = map[string]func(v map[string]interface{}, appCfg *config.App) Provider{
	"slack":     func(v map[string]interface{}, appCfg *config.App) Provider { return slack.NewSlack(v, appCfg) },
	"pagerduty": func(v map[string]interface{}, appCfg *config.App) Provider { return pagerduty.NewPagerDuty(v, appCfg) },
	"discord":   func(v map[string]interface{}, appCfg *config.App) Provider { return discord.NewDiscord(v, appCfg) },
	"telegram":  func(v map[string]interface{}, appCfg *config.App) Provider { return telegram.NewTelegram(v, appCfg) },
	"teams":     func(v map[string]interface{}, appCfg *config.App) Provider { return teams.NewTeams(v, appCfg) },
	"email":     func(v map[string]interface{}, appCfg *config.App) Provider { return email.NewEmail(v, appCfg) },
	"rocketchat": func(v map[string]interface{}, appCfg *config.App) Provider {
		return rocketchat.NewRocketChat(v, appCfg)
	},
	"mattermost": func(v map[string]interface{}, appCfg *config.App) Provider {
		return mattermost.NewMattermost(v, appCfg)
	},
	"opsgenie": func(v map[string]interface{}, appCfg *config.App) Provider { return opsgenie.NewOpsgenie(v, appCfg) },
	"matrix":   func(v map[string]interface{}, appCfg *config.App) Provider { return matrix.NewMatrix(v, appCfg) },
	"dingtalk": func(v map[string]interface{}, appCfg *config.App) Provider { return dingtalk.NewDingTalk(v, appCfg) },
	"feishu":   func(v map[string]interface{}, appCfg *config.App) Provider { return feishu.NewFeiShu(v, appCfg) },
	"webhook":  func(v map[string]interface{}, appCfg *config.App) Provider { return webhook.NewWebhook(v, appCfg) },
	"zenduty":  func(v map[string]interface{}, appCfg *config.App) Provider { return zenduty.NewZenduty(v, appCfg) },
	"googlechat": func(v map[string]interface{}, appCfg *config.App) Provider {
		return googlechat.NewGoogleChat(v, appCfg)
	},
	"gotify":   func(v map[string]interface{}, appCfg *config.App) Provider { return gotify.NewGotify(v, appCfg) },
	"ntfy":     func(v map[string]interface{}, appCfg *config.App) Provider { return ntfy.NewNtfy(v, appCfg) },
	"pushover": func(v map[string]interface{}, appCfg *config.App) Provider { return pushover.NewPushover(v, appCfg) },
	"webex":    func(v map[string]interface{}, appCfg *config.App) Provider { return webex.NewWebex(v, appCfg) },
	"github":   func(v map[string]interface{}, appCfg *config.App) Provider { return github.NewGithub(v, appCfg) },
	"line":     func(v map[string]interface{}, appCfg *config.App) Provider { return line.NewLine(v, appCfg) },
	"gitlab":   func(v map[string]interface{}, appCfg *config.App) Provider { return gitlab.NewGitlab(v, appCfg) },
	"gitea":    func(v map[string]interface{}, appCfg *config.App) Provider { return gitea.NewGitea(v, appCfg) },
	"zapier":   func(v map[string]interface{}, appCfg *config.App) Provider { return zapier.NewZapier(v, appCfg) },
	"n8n":      func(v map[string]interface{}, appCfg *config.App) Provider { return n8n.NewN8n(v, appCfg) },
	"ifttt":    func(v map[string]interface{}, appCfg *config.App) Provider { return ifttt.NewIfttt(v, appCfg) },
	"teamsworkflow": func(v map[string]interface{}, appCfg *config.App) Provider {
		return teamsworkflow.NewTeamsWorkflow(v, appCfg)
	},
	"zulip": func(v map[string]interface{}, appCfg *config.App) Provider { return zulip.NewZulip(v, appCfg) },
	"homeassistant": func(v map[string]interface{}, appCfg *config.App) Provider {
		return homeassistant.NewHomeAssistant(v, appCfg)
	},
	"splunk":   func(v map[string]interface{}, appCfg *config.App) Provider { return splunk.NewSplunk(v, appCfg) },
	"datadog":  func(v map[string]interface{}, appCfg *config.App) Provider { return datadog.NewDatadog(v, appCfg) },
	"newrelic": func(v map[string]interface{}, appCfg *config.App) Provider { return newrelic.NewNewRelic(v, appCfg) },
	"clickup":  func(v map[string]interface{}, appCfg *config.App) Provider { return clickup.NewClickup(v, appCfg) },
	"ilert":    func(v map[string]interface{}, appCfg *config.App) Provider { return ilert.NewIlert(v, appCfg) },
	"incidentio": func(v map[string]interface{}, appCfg *config.App) Provider {
		return incidentio.NewIncidentio(v, appCfg)
	},
	"incident.io": func(v map[string]interface{}, appCfg *config.App) Provider {
		return incidentio.NewIncidentio(v, appCfg)
	},
	"squadcast": func(v map[string]interface{}, appCfg *config.App) Provider { return squadcast.NewSquadcast(v, appCfg) },
	"signl4":    func(v map[string]interface{}, appCfg *config.App) Provider { return signl4.NewSignl4(v, appCfg) },
	"twilio":    func(v map[string]interface{}, appCfg *config.App) Provider { return twilio.NewTwilio(v, appCfg) },
	"vonage":    func(v map[string]interface{}, appCfg *config.App) Provider { return vonage.NewVonage(v, appCfg) },
	"plivo":     func(v map[string]interface{}, appCfg *config.App) Provider { return plivo.NewPlivo(v, appCfg) },
	"messagebird": func(v map[string]interface{}, appCfg *config.App) Provider {
		return messagebird.NewMessagebird(v, appCfg)
	},
	"signal":   func(v map[string]interface{}, appCfg *config.App) Provider { return signal.NewSignal(v, appCfg) },
	"sendgrid": func(v map[string]interface{}, appCfg *config.App) Provider { return sendgrid.NewSendgrid(v, appCfg) },
	"ses":      func(v map[string]interface{}, appCfg *config.App) Provider { return ses.NewSes(v, appCfg) },
	"sns":      func(v map[string]interface{}, appCfg *config.App) Provider { return sns.NewSns(v, appCfg) },
	"jira":     func(v map[string]interface{}, appCfg *config.App) Provider { return jira.NewJira(v, appCfg) },
	"wecom":    func(v map[string]interface{}, appCfg *config.App) Provider { return wecom.NewWecom(v, appCfg) },
	"splunkoncall": func(v map[string]interface{}, appCfg *config.App) Provider {
		return splunkoncall.NewSplunkOncall(v, appCfg)
	},
	"mailgun": func(v map[string]interface{}, appCfg *config.App) Provider { return mailgun.NewMailgun(v, appCfg) },
	"resend":  func(v map[string]interface{}, appCfg *config.App) Provider { return resend.NewResend(v, appCfg) },
	"goalert": func(v map[string]interface{}, appCfg *config.App) Provider { return goalert.NewGoalert(v, appCfg) },
	"alerta":  func(v map[string]interface{}, appCfg *config.App) Provider { return alerta.NewAlerta(v, appCfg) },
	"threema": func(v map[string]interface{}, appCfg *config.App) Provider { return threema.NewThreema(v, appCfg) },
	"flock":   func(v map[string]interface{}, appCfg *config.App) Provider { return flock.NewFlock(v, appCfg) },
	"pushbullet": func(v map[string]interface{}, appCfg *config.App) Provider {
		return pushbullet.NewPushbullet(v, appCfg)
	},
	"sensugo": func(v map[string]interface{}, appCfg *config.App) Provider { return sensugo.NewSensugo(v, appCfg) },
}

// newProvider constructs a provider for the given normalized (lowercased)
// provider key, or returns nil when the key is not a known provider.
func newProvider(key string, v map[string]interface{}, appCfg *config.App) Provider {
	if factory, ok := providerFactories[key]; ok {
		return factory(v, appCfg)
	}
	return nil
}
