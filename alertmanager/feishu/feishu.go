package feishu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/abahmed/kwatch/config"
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
func NewFeiShu(config map[string]interface{}, appCfg *config.App) *FeiShu {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing Fei Shu with empty webhook url")
		return nil
	}

	logrus.Infof("initializing Fei Shu with webhook url: %s", webhook)

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
	formattedMsg := e.FormatMarkdown(r.appCfg.ClusterName, "", "")
	return r.sendByFeiShuApi(r.buildRequestBodyFeiShu(formattedMsg))
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
	return r.sendByFeiShuApi(r.buildRequestBodyFeiShu(msg))
}

func (r *FeiShu) buildRequestBodyFeiShu(
	text string) string {
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
