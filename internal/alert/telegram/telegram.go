package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/ratelimit"
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
	token  string
	chatId string
	url    string

	// reference for general app configuration
	appCfg *config.App
}

// NewTelegram returns a new Telegram object
func NewTelegram(config map[string]interface{}, appCfg *config.App) *Telegram {
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
		token:  token,
		chatId: chatId,
		url:    telegramAPIURL,
		appCfg: appCfg,
	}
}

// Name returns name of the provider
func (t *Telegram) Name() string {
	return "Telegram"
}

// Verify checks credentials via Telegram getMe API.
func (t *Telegram) Verify() error {
	client := k8s.GetDefaultClient()
	url := fmt.Sprintf(telegramGetMeURL, t.token)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram getMe returned status %d", resp.StatusCode)
	}
	return nil
}

// SendEvent sends event to the provider
func (t *Telegram) SendEvent(e *event.Event) error {
	klog.V(4).InfoS("sending to telegram event", "event", e)

	reqBody := t.buildRequestBodyTelegram(e, t.chatId, "")
	return t.sendByTelegramApi(reqBody)
}

// SendMessage sends text message to the provider
func (t *Telegram) SendMessage(msg string) error {
	klog.V(4).InfoS("sending to telegram msg", "msg", msg)

	reqBody := t.buildRequestBodyTelegram(new(event.Event), t.chatId, msg)
	return t.sendByTelegramApi(reqBody)
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (t *Telegram) SendIncident(inc *model.Incident, action model.IncidentAction) error {
	text := util.RenderIncident(inc, action, message.NewPlainTextRenderer(), t.appCfg.ClusterName)
	if text == "" {
		return nil
	}
	return t.SendMessage(text)
}

func (t *Telegram) buildRequestBodyTelegram(
	e *event.Event,
	chatId string,
	customMsg string) string {
	// build text will be sent in the message
	txt := ""
	if len(customMsg) == 0 {
		var parts []string
		parts = append(parts, fmt.Sprintf("*Reason:* %s", util.OrDefault(e.Reason, "unknown")))

		if e.PodName != "" {
			parts = append(parts, fmt.Sprintf("*Pod:* %s", e.PodName))
		}
		if e.ContainerName != "" {
			parts = append(parts, fmt.Sprintf("*Container:* %s", e.ContainerName))
		}
		if e.Namespace != "" {
			parts = append(parts, fmt.Sprintf("*Namespace:* %s", e.Namespace))
		}
		if e.NodeName != "" {
			parts = append(parts, fmt.Sprintf("*Node:* %s", e.NodeName))
		}
		if t.appCfg.ClusterName != "" {
			parts = append(parts, fmt.Sprintf("*Cluster:* %s", t.appCfg.ClusterName))
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

func (t *Telegram) sendByTelegramApi(reqBody string) error {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer([]byte(reqBody))
	url := fmt.Sprintf(t.url, t.token)

	request, err := http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusTooManyRequests {
		d := ratelimit.ParseRetryAfter(response)
		if d == 0 {
			body, _ := io.ReadAll(response.Body)
			var p struct {
				Parameters *struct {
					RetryAfter int `json:"retry_after"`
				} `json:"parameters"`
			}
			if json.Unmarshal(body, &p) == nil && p.Parameters != nil && p.Parameters.RetryAfter > 0 {
				d = time.Duration(p.Parameters.RetryAfter) * time.Second
			}
		}
		return &ratelimit.Error{
			Provider:   "Telegram",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: d,
		}
	}
	if response.StatusCode > 299 {
		return fmt.Errorf(
			"call to telegram alert returned status code %d",
			response.StatusCode)
	}

	return nil
}
