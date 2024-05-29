package googlechat

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

type GoogleChat struct {
	webhook string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type payload struct {
	Text string `json:"text"`
}

// NewGoogleChat returns new google chat instance
func NewGoogleChat(config map[string]interface{}, appCfg *config.App) *GoogleChat {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing Google Chat with empty webhook url")
		return nil
	}

	logrus.Infof("initializing Google Chat with webhook url: %s", webhook)

	text, _ := config["text"].(string)

	return &GoogleChat{
		webhook: webhook,
		text:    text,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (r *GoogleChat) Name() string {
	return "Google Chat"
}

// SendEvent sends event to the provider
func (r *GoogleChat) SendEvent(e *event.Event) error {
	formattedMsg := e.FormatText(r.appCfg.ClusterName, r.text)
	return r.sendAPI(r.buildRequestBody(formattedMsg))
}

func (r *GoogleChat) sendAPI(reqBody string) error {
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
			"call to google chat alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

// SendMessage sends text message to the provider
func (r *GoogleChat) SendMessage(msg string) error {
	return r.sendAPI(r.buildRequestBody(msg))
}

func (r *GoogleChat) buildRequestBody(text string) string {
	msgPayload := &payload{
		Text: text,
	}

	jsonBytes, _ := json.Marshal(msgPayload)
	return string(jsonBytes)
}
