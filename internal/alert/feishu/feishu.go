package feishu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

type FeiShu struct {
	webhook string
	title   string

	// reference for general app configuration
	appCfg *config.App
}

type feiShuWebhookContent struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feiShuCardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

type feiShuHeaderTitle struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feiShuHeader struct {
	Title    feiShuHeaderTitle `json:"title"`
	Template string            `json:"template"`
}

type feiShuCard struct {
	Config   feiShuCardConfig       `json:"config"`
	Header   feiShuHeader           `json:"header"`
	Elements []feiShuWebhookContent `json:"elements"`
}

type feiShuRequestBody struct {
	MsgType string     `json:"msg_type"`
	Card    feiShuCard `json:"card"`
}

// NewFeiShu returns new feishu web bot instance
func NewFeiShu(config map[string]interface{}, appCfg *config.App) *FeiShu {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Fei Shu with empty webhook url")
		return nil
	}

	klog.InfoS("initializing Fei Shu with webhook url", "webhook", webhook)

	title, _ := config["title"].(string)

	return &FeiShu{
		webhook: webhook,
		title:   title,
		appCfg:  appCfg,
	}

}

// Name returns name of the provider
func (f *FeiShu) Name() string {
	return "Fei Shu"
}

// SendEvent sends event to the provider
func (f *FeiShu) SendEvent(e *event.Event) error {
	body, err := f.buildRequestBodyFeiShu(e.FormatMarkdown(f.appCfg.ClusterName, "", ""))
	if err != nil {
		return err
	}
	return f.sendByFeiShuApi(body)
}

func (f *FeiShu) sendByFeiShuApi(reqBody string) error {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer([]byte(reqBody))
	request, err := http.NewRequest(http.MethodPost, f.webhook, buffer)
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
		return &ratelimit.Error{
			Provider:   "Feishu",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: ratelimit.ParseRetryAfter(response),
		}
	}
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to feishu alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

// SendMessage sends text message to the provider
func (f *FeiShu) SendMessage(msg string) error {
	body, err := f.buildRequestBodyFeiShu(msg)
	if err != nil {
		return err
	}
	return f.sendByFeiShuApi(body)
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message.
func (f *FeiShu) SendIncident(inc *model.Incident, action model.IncidentAction) error {
	text := util.RenderIncident(inc, action, message.NewPlainTextRenderer(), f.appCfg.ClusterName)
	if text == "" {
		return nil
	}
	return f.SendMessage(text)
}

func (f *FeiShu) buildRequestBodyFeiShu(
	text string) (string, error) {
	body := feiShuRequestBody{
		MsgType: "interactive",
		Card: feiShuCard{
			Config: feiShuCardConfig{
				WideScreenMode: true,
			},
			Header: feiShuHeader{
				Title: feiShuHeaderTitle{
					Tag:     "plain_text",
					Content: f.title,
				},
				Template: "blue",
			},
			Elements: []feiShuWebhookContent{
				{
					Tag:     "markdown",
					Content: text,
				},
			},
		},
	}
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal feishu body: %w", err)
	}
	return string(jsonBytes), nil
}
