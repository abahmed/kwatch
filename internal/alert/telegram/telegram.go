package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// telegramTextLimit is Telegram's maximum message length. Escaping and the
// output block can push a delivered Note past it.
const telegramTextLimit = 4096

const (
	telegramAPIURL   = "https://api.telegram.org/bot%s/sendMessage"
	telegramGetMeURL = "https://api.telegram.org/bot%s/getMe"
)

func maskString(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + strings.Repeat("*", len(s)-4)
}

type telegramPayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type Telegram struct {
	sender transport.Sender
	token  string
	chatId string
	url    string

	// reference for general app configuration
	clusterName string
	clockSource clock.Clock
}

// NewTelegram returns a new Telegram object

func NewTelegram(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Telegram {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing telegram with empty token")
		return nil
	}

	chatId, ok := config["chatId"].(string)
	if !ok || len(chatId) == 0 {
		klog.InfoS("initializing telegram with empty chat_id")
		return nil
	}

	klog.InfoS(
		"initializing telegram",
		"token", maskString(token),
		"chatId", maskString(chatId))

	// returns a new telegram object
	return &Telegram{
		sender:      transport.NewSender(dependencies),
		token:       token,
		chatId:      chatId,
		url:         telegramAPIURL,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (t *Telegram) Name() string {
	return "Telegram"
}

// Verify checks credentials via Telegram getMe API.
func (t *Telegram) Verify(ctx context.Context) error {
	url := fmt.Sprintf(telegramGetMeURL, t.token)
	_, err := t.sender.Send(ctx, transport.Request{
		Provider: "Telegram",
		Method:   "GET",
		URL:      url,
	})
	return err
}

// SendIncident sends the incident narrative in Telegram's Markdown parse
// mode. Every event-derived character is escaped; the application output
// follows in a code block.
func (t *Telegram) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	klog.V(4).InfoS("sending incident to telegram",
		"conversation", m.Key, "revision", m.Revision)
	return t.sendByTelegramApi(ctx, t.incidentBody(m))
}

// SendMessage sends a plain operator message without a parse mode.
func (t *Telegram) SendMessage(ctx context.Context, msg string) error {
	klog.V(4).InfoS(
		"sending message to telegram",
		"messageLength", len(msg),
	)
	return t.sendByTelegramApi(ctx, t.body(telegramPayload{
		ChatID: t.chatId, Text: msg,
	}))
}

func (t *Telegram) incidentBody(m notification.Message) string {
	return t.body(telegramPayload{
		ChatID: t.chatId, ParseMode: "MARKDOWN",
		Text: incidentText(m, telegramTextLimit),
	})
}

// incidentText is the escaped narrative followed by the output in a code
// block, within limit bytes. Each part is cut before the fences are added,
// so a long message can never lose its closing fence and break parsing.
// The output gets at most half of the limit; the narrative comes first.
func incidentText(m notification.Message, limit int) string {
	note := escapeMarkdown(m.NoteText())
	if len(m.Output) == 0 {
		return notification.Truncate(note, limit)
	}
	const open, closing = "\n\n```\n", "\n```"
	output := strings.ReplaceAll(strings.Join(m.Output, "\n"), "`", "'")
	outputBudget := min(len(output), limit/2)
	note = notification.Truncate(note,
		limit-outputBudget-len(open)-len(closing))
	outputBudget = limit - len(note) - len(open) - len(closing)
	return note + open + notification.Truncate(output, outputBudget) + closing
}

func (t *Telegram) body(payload telegramPayload) string {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(bodyBytes)
}

func (t *Telegram) sendByTelegramApi(
	ctx context.Context,
	reqBody string,
) error {
	_, err := t.sender.Send(ctx, transport.Request{
		Provider: "Telegram",
		URL:      fmt.Sprintf(t.url, t.token),
		Body:     []byte(reqBody),
		// Telegram reports the back-off in the JSON body, not the header.
		RetryAfterFromBody: func(body []byte) time.Duration {
			var p struct {
				Parameters *struct {
					RetryAfter int `json:"retry_after"`
				} `json:"parameters"`
			}
			if json.Unmarshal(body, &p) == nil && p.Parameters != nil &&
				p.Parameters.RetryAfter > 0 {
				return time.Duration(p.Parameters.RetryAfter) * time.Second
			}
			return 0
		},
	})
	return err
}

// markdownEscaper escapes the legacy Telegram Markdown entity characters so
// pod names, logs and messages cannot break parsing or inject formatting.
var markdownEscaper = strings.NewReplacer(
	"_", "\\_", "*", "\\*", "`", "\\`", "[", "\\[",
)

func escapeMarkdown(value string) string {
	return markdownEscaper.Replace(value)
}
