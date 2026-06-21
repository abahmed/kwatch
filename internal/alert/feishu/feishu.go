package feishu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/ratelimit"
	"k8s.io/klog/v2"
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
func (r *FeiShu) Name() string {
	return "Fei Shu"
}

// SendEvent sends event to the provider
func (r *FeiShu) SendEvent(e *event.Event) error {
	body, err := r.buildRequestBodyFeiShu(e.FormatMarkdown(r.appCfg.ClusterName, "", ""))
	if err != nil {
		return err
	}
	return r.sendByFeiShuApi(body)
}

func (r *FeiShu) sendByFeiShuApi(reqBody string) error {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer([]byte(reqBody))
	request, err := http.NewRequest(http.MethodPost, r.webhook, buffer)
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
func (r *FeiShu) SendMessage(msg string) error {
	body, err := r.buildRequestBodyFeiShu(msg)
	if err != nil {
		return err
	}
	return r.sendByFeiShuApi(body)
}

func (r *FeiShu) buildRequestBodyFeiShu(
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
					Content: r.title,
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
