package discord

import (
	"strings"
	"time"

	discordgo "github.com/bwmarrin/discordgo"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/notification"
)

// Discord embed limits. The whole message (title, description, footer)
// must stay within maxEmbedTotal. The narrative goes in the description,
// never in one field per section: fields hold only 1024 characters each.
const (
	maxEmbedTitle       = 256
	maxEmbedDescription = 4096
	maxEmbedFooter      = 2048
	maxEmbedTotal       = 6000
)

// Embed colours by status, the brand's severity tokens.
const (
	colorCritical = 0xB42318
	colorWarning  = 0x8A5300
	colorResolved = 0x1E6B3A
	colorInfo     = 0x5B6270
)

func statusColor(s notification.Status) int {
	switch s {
	case notification.StatusCritical:
		return colorCritical
	case notification.StatusWarning, notification.StatusFlapping:
		return colorWarning
	case notification.StatusResolved:
		return colorResolved
	}
	return colorInfo
}

// discordDialect writes Markdown with mentions neutralized, so a pod's
// log line can never ping a server even if allowed mentions change.
func discordDialect() notification.Dialect {
	return notification.MarkdownDialect(notification.NeutralizeMentions, "\n")
}

// incidentEmbed lays the incident out as one embed: the headline as the
// title, the cause, proof, log quote and fix in the description (a Pro
// fix is pinned, so the proof is cut first and the fix never), and the
// cluster and time in the footer.
func incidentEmbed(
	m notification.Message, cluster string, now time.Time,
) *discordgo.MessageEmbed {
	title := notification.Truncate(
		notification.NeutralizeMentions(m.ShortText()), maxEmbedTitle)
	footer := "kwatch"
	if cluster != "" {
		footer += " · " + notification.NeutralizeMentions(cluster)
	}
	footer = notification.Truncate(footer, maxEmbedFooter)
	room := min(maxEmbedDescription,
		maxEmbedTotal-len(title)-len(footer))
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: description(m, room),
		Color:       statusColor(m.Status),
		Footer:      &discordgo.MessageEmbedFooter{Text: footer},
		Timestamp:   now.UTC().Format(time.RFC3339),
	}
}

// description is the narrative without its headline (the title says it)
// plus the output, within limit bytes.
func description(m notification.Message, limit int) string {
	if len(m.Doc) > 1 {
		m.Doc = m.Doc[1:]
	}
	text := safetext.RichWithOutput(m, discordDialect(), "\n", limit)
	return strings.TrimSpace(text)
}
