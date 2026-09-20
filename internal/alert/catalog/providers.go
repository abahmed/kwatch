package catalog

import (
	"fmt"
	"sort"

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
	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

type factoryFunc func(
	map[string]interface{},
	transport.ProviderContext,
) deliveryapi.Provider

var factories = map[string]factoryFunc{
	"slack": slackFactory, "discord": discordFactory,
	"pagerduty": factory(pagerduty.NewPagerDuty),
	"telegram":  factory(telegram.NewTelegram),
	"teams":     factory(teams.NewTeams), "email": emailFactory,
	"rocketchat": factory(rocketchat.NewRocketChat),
	"mattermost": factory(mattermost.NewMattermost),
	"opsgenie":   factory(opsgenie.NewOpsgenie),
	"matrix":     factory(matrix.NewMatrix),
	"dingtalk":   factory(dingtalk.NewDingTalk),
	"feishu":     factory(feishu.NewFeiShu),
	"webhook":    factory(webhook.NewWebhook),
	"zenduty":    factory(zenduty.NewZenduty),
	"googlechat": factory(googlechat.NewGoogleChat),
	"gotify":     factory(gotify.NewGotify), "ntfy": factory(ntfy.NewNtfy),
	"pushover": factory(pushover.NewPushover),
	"webex":    factory(webex.NewWebex), "github": factory(github.NewGithub),
	"line": factory(line.NewLine), "gitlab": factory(gitlab.NewGitlab),
	"gitea": factory(gitea.NewGitea), "zapier": factory(zapier.NewZapier),
	"n8n": factory(n8n.NewN8n), "ifttt": factory(ifttt.NewIfttt),
	"teamsworkflow": factory(teamsworkflow.NewTeamsWorkflow),
	"zulip":         factory(zulip.NewZulip),
	"homeassistant": factory(homeassistant.NewHomeAssistant),
	"splunk":        factory(splunk.NewSplunk),
	"datadog":       factory(datadog.NewDatadog),
	"newrelic":      factory(newrelic.NewNewRelic),
	"clickup":       factory(clickup.NewClickup), "ilert": factory(ilert.NewIlert),
	"incidentio":  factory(incidentio.NewIncidentio),
	"incident.io": factory(incidentio.NewIncidentio),
	"squadcast":   factory(squadcast.NewSquadcast),
	"signl4":      factory(signl4.NewSignl4), "twilio": factory(twilio.NewTwilio),
	"vonage": factory(vonage.NewVonage), "plivo": factory(plivo.NewPlivo),
	"messagebird": factory(messagebird.NewMessagebird),
	"signal":      factory(signal.NewSignal),
	"sendgrid":    factory(sendgrid.NewSendgrid),
	"ses":         factory(ses.NewSes), "sns": factory(sns.NewSns),
	"jira": factory(jira.NewJira), "wecom": factory(wecom.NewWecom),
	"splunkoncall": factory(splunkoncall.NewSplunkOncall),
	"mailgun":      factory(mailgun.NewMailgun),
	"resend":       factory(resend.NewResend),
	"goalert":      factory(goalert.NewGoalert),
	"alerta":       factory(alerta.NewAlerta),
	"threema":      factory(threema.NewThreema),
	"flock":        factory(flock.NewFlock),
	"pushbullet":   factory(pushbullet.NewPushbullet),
	"sensugo":      factory(sensugo.NewSensugo),
}

func factory[T deliveryapi.Provider](
	create func(
		map[string]interface{}, string,
		transport.Dependencies,
	) T,
) factoryFunc {
	return func(
		values map[string]interface{}, providerContext transport.ProviderContext,
	) deliveryapi.Provider {
		return create(
			values,
			providerContext.ClusterName,
			providerContext.Dependencies,
		)
	}
}

func emailFactory(
	values map[string]interface{}, providerContext transport.ProviderContext,
) deliveryapi.Provider {
	return email.NewEmail(
		values,
		providerContext.ClusterName,
	)
}

func slackFactory(
	values map[string]interface{}, providerContext transport.ProviderContext,
) deliveryapi.Provider {
	return slack.NewSlack(
		values,
		providerContext.ClusterName,
		providerContext.Dependencies,
	)
}

func discordFactory(
	values map[string]interface{}, providerContext transport.ProviderContext,
) deliveryapi.Provider {
	return discord.NewDiscord(
		values,
		providerContext.ClusterName,
		providerContext.Dependencies,
	)
}

// NewProvider creates a statically linked provider for a normalized key.
func NewProvider(
	key string,
	values map[string]interface{},
	providerContext transport.ProviderContext,
) deliveryapi.Provider {
	if create, ok := factories[key]; ok {
		return create(values, providerContext)
	}
	return nil
}

// ProviderNames returns catalog keys in deterministic order.
func ProviderNames() []string {
	keys := make([]string, 0, len(factories))
	for key := range factories {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Validate verifies that every provider identity has a statically linked
// factory and that no factory is missing from the identity catalog.
func Validate() error {
	factoryNames := ProviderNames()
	identityNames := providercatalog.Names()
	if len(factoryNames) != len(identityNames) {
		return fmt.Errorf(
			"provider catalog size mismatch: %d factories, %d identities",
			len(factoryNames), len(identityNames),
		)
	}
	for i := range factoryNames {
		if factoryNames[i] != identityNames[i] {
			return fmt.Errorf(
				"provider catalog mismatch at %d: factory=%q identity=%q",
				i, factoryNames[i], identityNames[i],
			)
		}
	}
	return nil
}
