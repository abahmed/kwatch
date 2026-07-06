package mattermost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/ratelimit"
	"k8s.io/klog/v2"
)

type Mattermost struct {
	webhook string
	title   string
	text    string

	// reference for general app configuration
	appCfg *config.App
}

type mmField struct {
	Short bool        `json:"short"`
	Title string      `json:"title"`
	Value interface{} `json:"value"`
}

type mmAttachment struct {
	Title  string    `json:"title"`
	Text   string    `json:"text"`
	Fields []mmField `json:"fields"`
}

type mmPayload struct {
	Text        string         `json:"text"`
	Attachments []mmAttachment `json:"attachments"`
}

// NewMattermost returns new mattermost instance
func NewMattermost(config map[string]interface{}, appCfg *config.App) *Mattermost {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing mattermost with empty webhook url")
		return nil
	}

	klog.InfoS("initializing mattermost with webhook url", "webhook", webhook)

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Mattermost{
		webhook: webhook,
		title:   title,
		text:    text,
		appCfg:  appCfg,
	}
}

// Name returns name of the provider
func (m *Mattermost) Name() string {
	return "Mattermost"
}

// SendMessage sends text message to the provider
func (m *Mattermost) SendMessage(msg string) error {
	klog.V(4).InfoS("sending to mattermost msg", "msg", msg)

	b, err := m.buildMessage(nil, &msg)
	if err != nil {
		return err
	}
	return m.sendAPI(b)
}

// SendEvent sends event to the provider
func (m *Mattermost) SendEvent(e *event.Event) error {
	klog.V(4).InfoS("sending to mattermost event", "event", e)

	b, err := m.buildMessage(e, nil)
	if err != nil {
		return err
	}
	return m.sendAPI(b)
}

func (m *Mattermost) sendAPI(content []byte) error {
	client := k8s.GetDefaultClient()
	buffer := bytes.NewBuffer(content)
	request, err := http.NewRequest(http.MethodPost, m.webhook, buffer)
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
			Provider:   "Mattermost",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: ratelimit.ParseRetryAfter(response),
		}
	}
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to mattermost alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

func (m *Mattermost) buildMessage(e *event.Event, msg *string) ([]byte, error) {
	payload := mmPayload{}

	if msg != nil && len(*msg) > 0 {
		payload.Text = *msg
	}

	if e != nil {
		logs := strings.TrimSpace(e.Logs)
		events := strings.TrimSpace(e.Events)

		// use custom title if it's provided, otherwise use default
		title := m.title
		if len(title) == 0 {
			title = constant.DefaultTitle
		}

		// use custom text if it's provided, otherwise use default
		text := m.text
		if len(text) == 0 {
			text = constant.DefaultText
		}

		mmFields := []mmField{}
		if m.appCfg.ClusterName != "" {
			mmFields = append(mmFields, mmField{Title: "Cluster", Value: m.appCfg.ClusterName, Short: true})
		}
		if e.PodName != "" {
			mmFields = append(mmFields, mmField{Title: "Name", Value: e.PodName, Short: true})
		}
		if e.ContainerName != "" {
			mmFields = append(mmFields, mmField{Title: "Container", Value: e.ContainerName, Short: true})
		}
		if e.Namespace != "" {
			mmFields = append(mmFields, mmField{Title: "Namespace", Value: e.Namespace, Short: true})
		}
		if e.NodeName != "" {
			mmFields = append(mmFields, mmField{Title: "Node", Value: e.NodeName, Short: true})
		}
		if e.Reason != "" {
			mmFields = append(mmFields, mmField{Title: "Reason", Value: e.Reason, Short: true})
		}
		if logs != "" {
			mmFields = append(mmFields, mmField{Title: ":memo: Logs", Value: "```\n" + logs + "\n```", Short: false})
		}
		if events != "" {
			mmFields = append(mmFields, mmField{Title: ":mag: Events", Value: "```\n" + events + " \n```", Short: false})
		}

		payload.Attachments = []mmAttachment{
			{
				Title:  title,
				Text:   text,
				Fields: mmFields,
			},
		}
	}

	str, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mattermost payload: %w", err)
	}
	return str, nil
}
