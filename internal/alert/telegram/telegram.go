package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

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
	ParseMode string `json:"parse_mode"`
}

type Telegram struct {
	sender transport.Sender
	token  string
	chatId string
	url    string

	// reference for general app configuration
	clusterName string
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

// SendEvent sends event to the provider
func (t *Telegram) SendEvent(ctx context.Context, e *event.Event) error {
	klog.V(4).InfoS(
		"sending to telegram event",
		"namespace", e.Namespace,
		"name", e.PodName,
		"reason", e.Reason,
		"action", e.Action,
	)

	reqBody := t.buildRequestBodyTelegram(e, t.chatId, "")
	return t.sendByTelegramApi(ctx, reqBody)
}

// SendMessage sends text message to the provider
func (t *Telegram) SendMessage(ctx context.Context, msg string) error {
	klog.V(4).InfoS(
		"sending message to telegram",
		"messageLength", len(msg),
	)

	reqBody := t.buildRequestBodyTelegram(new(event.Event), t.chatId, msg)
	return t.sendByTelegramApi(ctx, reqBody)
}

// SendIncident implements delivery.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (t *Telegram) SendIncident(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return t.SendIncidentWithInsight(ctx, inc, action, nil)
}

// SendIncidentWithInsight implements delivery.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (t *Telegram) SendIncidentWithInsight(
	ctx context.Context,
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := message.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		t.clusterName,
	)
	if text == "" {
		return nil
	}
	return t.SendMessage(ctx, text)
}

func (t *Telegram) buildRequestBodyTelegram(
	e *event.Event,
	chatId string,
	customMsg string) string {
	// build text will be sent in the message
	txt := ""
	if len(customMsg) == 0 {
		var parts []string
		parts = append(
			parts,
			fmt.Sprintf("*Reason:* %s", format.OrDefault(e.Reason, "unknown")),
		)

		if e.PodName != "" {
			parts = append(parts, fmt.Sprintf("*Pod:* %s", e.PodName))
		}
		if e.ContainerName != "" {
			parts = append(
				parts,
				fmt.Sprintf("*Container:* %s", e.ContainerName),
			)
		}
		if e.Namespace != "" {
			parts = append(parts, fmt.Sprintf("*Namespace:* %s", e.Namespace))
		}
		if e.NodeName != "" {
			parts = append(parts, fmt.Sprintf("*Node:* %s", e.NodeName))
		}
		if t.clusterName != "" {
			parts = append(
				parts,
				fmt.Sprintf("*Cluster:* %s", t.clusterName),
			)
		}

		txt = "⛑ Kwatch alert\n" + strings.Join(parts, "\n")

		if e.IncludeLogs {
			logs := strings.TrimSpace(e.Logs)
			if len(logs) > 0 {
				txt += "\n\n*Logs:*\n" + logs
			}
		}

		if e.IncludeEvents {
			events := strings.TrimSpace(e.Events)
			if len(events) > 0 {
				txt += "\n\n*Events:*\n" + events
			}
		}
	} else {
		txt = customMsg
	}

	payload := telegramPayload{
		ChatID:    chatId,
		Text:      txt,
		ParseMode: "MARKDOWN",
	}

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
