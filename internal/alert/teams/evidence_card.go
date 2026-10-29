package teams

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// card is one Adaptive Card element.
type card = map[string]interface{}

// evidenceCardAttachments lays out a Pro message the way the evidence
// spec does: the cause, what kwatch found, the log quote in monospace
// and the fix, each command in its own monospace block. There is no
// action: the card links nowhere.
func evidenceCardAttachments(
	title string, m notification.Message,
) []map[string]interface{} {
	ev := m.Evidence
	body := []card{
		{"type": "TextBlock", "text": cardTitle(title), "weight": "Bolder",
			"wrap": true},
		{"type": "TextBlock", "text": cardText(m.ShortText()),
			"weight": "Bolder", "wrap": true},
	}
	if len(ev.Facts) > 0 {
		body = append(body, heading("What kwatch found"))
		for _, f := range ev.Facts {
			body = append(body, card{"type": "TextBlock",
				"text": "- " + cardText(f), "wrap": true, "spacing": "None"})
		}
	}
	if q := ev.Quote; q != nil && len(q.Lines) > 0 {
		body = append(body, card{"type": "Container", "items": []card{
			{"type": "TextBlock", "text": cardText(quoteLabel(q)),
				"isSubtle": true, "wrap": true},
			{"type": "TextBlock", "text": cardText(strings.Join(q.Lines, "\n")),
				"fontType": "Monospace", "wrap": false,
				"maxLines": notification.MaxQuoteLines},
		}})
	}
	body = append(body, fixBlocks(ev.Fix)...)
	return []map[string]interface{}{{
		"contentType": "application/vnd.microsoft.card.adaptive",
		"content": card{
			"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
			"type":    "AdaptiveCard", "version": "1.2", "body": body,
		},
	}}
}

func heading(text string) card {
	return card{"type": "TextBlock", "text": text, "weight": "Bolder",
		"wrap": true, "spacing": "Medium"}
}

func fixBlocks(fix []notification.EvidenceStep) []card {
	if len(fix) == 0 {
		return nil
	}
	out := []card{heading("Fix (changes the cluster)")}
	for _, s := range fix {
		out = append(out, card{"type": "TextBlock",
			"text": cardText(s.Text), "wrap": true})
		if s.Command != "" {
			out = append(out, card{"type": "TextBlock",
				"text": cardText(s.Command), "fontType": "Monospace",
				"wrap": true})
		}
	}
	return out
}

// quoteLabel is "Log, container app, previous run".
func quoteLabel(q *notification.Quote) string {
	label := "Log"
	if q.Container != "" {
		label += ", container " + q.Container
	}
	if q.Run != "" {
		label += ", " + q.Run + " run"
	}
	return label
}

// cardText escapes text from the cluster for a TextBlock, like titles.
func cardText(text string) string { return cardTitle(text) }
