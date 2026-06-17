package googlechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"k8s.io/klog/v2"
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
		klog.InfoS("initializing Google Chat with empty webhook url")
		return nil
	}

	klog.InfoS("initializing Google Chat with webhook url", "webhook", webhook)

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
	b, err := r.buildRequestBody(formattedMsg)
	if err != nil {
		return err
	}
	return r.sendAPI(b)
}

func (r *GoogleChat) sendAPI(reqBody []byte) error {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer(reqBody)
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
	b, err := r.buildRequestBody(msg)
	if err != nil {
		return err
	}
	return r.sendAPI(b)
}

func (r *GoogleChat) buildRequestBody(text string) ([]byte, error) {
	msgPayload := &payload{
		Text: text,
	}

	jsonBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal google chat payload: %w", err)
	}
	return jsonBytes, nil
}
