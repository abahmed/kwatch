package rocketchat

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

type RocketChat struct {
	webhook string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type rocketChatWebhookPayload struct {
	Text string `json:"text"`
}

// NewRocketChat returns new rocket chat instance
func NewRocketChat(config map[string]interface{}, appCfg *config.App) *RocketChat {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing Rocket Chat with empty webhook url")
		return nil
	}

	klog.InfoS("initializing Rocket Chat with webhook url", "webhook", webhook)

	text, _ := config["text"].(string)

	return &RocketChat{
		webhook: webhook,
		text:    text,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (r *RocketChat) Name() string {
	return "Rocket Chat"
}

// SendEvent sends event to the provider
func (r *RocketChat) SendEvent(e *event.Event) error {
	formattedMsg := e.FormatMarkdown(r.appCfg.ClusterName, r.text, "")
	b, err := r.buildRequestBodyRocketChat(formattedMsg)
	if err != nil {
		return err
	}
	return r.sendByRocketChatApi(b)
}

func (r *RocketChat) sendByRocketChatApi(reqBody []byte) error {
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
			"call to rocket chat alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

// SendMessage sends text message to the provider
func (r *RocketChat) SendMessage(msg string) error {
	b, err := r.buildRequestBodyRocketChat(msg)
	if err != nil {
		return err
	}
	return r.sendByRocketChatApi(b)
}

func (r *RocketChat) buildRequestBodyRocketChat(text string) ([]byte, error) {
	msgPayload := &rocketChatWebhookPayload{
		Text: text,
	}

	jsonBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rocketchat payload: %w", err)
	}
	return jsonBytes, nil
}
