package alert

import (
	"github.com/abahmed/kwatch/internal/alert/dingtalk"
	"github.com/abahmed/kwatch/internal/alert/discord"
	"github.com/abahmed/kwatch/internal/alert/feishu"
	"github.com/abahmed/kwatch/internal/alert/googlechat"
	"github.com/abahmed/kwatch/internal/alert/matrix"
	"github.com/abahmed/kwatch/internal/alert/mattermost"
	"github.com/abahmed/kwatch/internal/alert/rocketchat"
	"github.com/abahmed/kwatch/internal/alert/slack"
	"github.com/abahmed/kwatch/internal/alert/teams"
	"github.com/abahmed/kwatch/internal/alert/telegram"
	"github.com/abahmed/kwatch/internal/alert/webhook"
)

// Every provider with its own incident path must accept the diagnosis. A
// provider that implements ThreadProvider but not InsightThreadProvider gets
// the incident without its cause, impact or recent changes — which is how the
// diagnosis went missing from ten providers for a release. This fails to
// compile if one regresses.
var (
	_ InsightThreadProvider = (*dingtalk.DingTalk)(nil)
	_ InsightThreadProvider = (*discord.Discord)(nil)
	_ InsightThreadProvider = (*feishu.FeiShu)(nil)
	_ InsightThreadProvider = (*googlechat.GoogleChat)(nil)
	_ InsightThreadProvider = (*matrix.Matrix)(nil)
	_ InsightThreadProvider = (*mattermost.Mattermost)(nil)
	_ InsightThreadProvider = (*rocketchat.RocketChat)(nil)
	_ InsightThreadProvider = (*slack.Slack)(nil)
	_ InsightThreadProvider = (*teams.Teams)(nil)
	_ InsightThreadProvider = (*telegram.Telegram)(nil)
	_ InsightThreadProvider = (*webhook.Webhook)(nil)
)
