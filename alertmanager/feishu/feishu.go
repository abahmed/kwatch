package feishu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
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

// NewFeiShu returns new feishu web bot instance
func NewFeiShu(config map[string]string, appCfg *config.App) *FeiShu {
	webhook, ok := config["webhook"]
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing Fei Shu with empty webhook url")
		return nil
	}

	logrus.Infof("initializing Fei Shu with webhook url: %s", webhook)

	return &FeiShu{
		webhook: webhook,
		title:   config["title"],
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (r *FeiShu) Name() string {
	return "Fei Shu"
}

// SendEvent sends event to the provider
func (r *FeiShu) SendEvent(e *event.Event) error {
	return r.sendByFeiShuApi(r.buildRequestBodyFeiShu(e, ""))
}

func (r *FeiShu) sendByFeiShuApi(reqBody string) error {
	client := &http.Client{}
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

	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to rocket chat alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

// SendMessage sends text message to the provider
func (r *FeiShu) SendMessage(msg string) error {
	return r.sendByFeiShuApi(
		r.buildRequestBodyFeiShu(new(event.Event), msg),
	)
}

func (r *FeiShu) buildRequestBodyFeiShu(
	e *event.Event,
	customMsg string) string {
	// add events part if it exists
	eventsText := constant.DefaultEvents
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = e.Events
	}

	// add logs part if it exist
	logsText := constant.DefaultLogs
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = e.Logs
	}

	// build text will be sent in the message use custom text if it's provided,
	// otherwise use default
	text := ""
	if len(customMsg) <= 0 {
		text = fmt.Sprintf(
			"**Cluster:** %s\n"+
				"**Pod:** %s\n"+
				"**Container:** %s\n"+
				"**Namespace:** %s\n"+
				"**Reason:** %s\n"+
				"**Events:**\n```\n%s\n```\n"+
				"**Logs:**\n```\n%s\n```",
			r.appCfg.ClusterName,
			e.Name,
			e.Container,
			e.Namespace,
			e.Reason,
			eventsText,
			logsText,
		)
	} else {
		text = customMsg
	}
	var content = []feiShuWebhookContent{
		{
			Tag:     "markdown",
			Content: text,
		},
	}

	jsonBytes, _ := json.Marshal(content)

	body := "{\"msg_type\": \"interactive\",\"card\": {\"config\": {\"wide_screen_mode\": true},\"header\": {\"title\": {\"tag\": \"plain_text\",\"content\": \"" +
		r.title +
		"\"},\"template\": \"blue\"},\"elements\": " + string(jsonBytes) + "}}"
	return body
}
