package mattermost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"net/http"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
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
		logrus.Warnf("initializing mattermost with empty webhook url")
		return nil
	}

	logrus.Infof("initializing mattermost with webhook url: %s", webhook)

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
	logrus.Debugf("sending to mattermost msg: %s", msg)

	return m.sendAPI(m.buildMessage(nil, &msg))
}

// SendEvent sends event to the provider
func (m *Mattermost) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to mattermost event: %v", e)

	return m.sendAPI(m.buildMessage(e, nil))
}

func (m *Mattermost) sendAPI(content []byte) error {
	client := &http.Client{}
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

	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to mattermost alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

func (m *Mattermost) buildMessage(e *event.Event, msg *string) []byte {
	payload := mmPayload{}

	if msg != nil && len(*msg) > 0 {
		payload.Text = *msg
	}

	if e != nil {
		logs := constant.DefaultLogs
		if len(e.Logs) > 0 {
			logs = (e.Logs)
		}

		events := constant.DefaultEvents
		if len(e.Events) > 0 {
			events = (e.Events)
		}

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

		payload.Attachments = []mmAttachment{
			{
				Title: title,
				Text:  text,
				Fields: []mmField{
					{
						Title: "Cluster",
						Value: m.appCfg.ClusterName,
						Short: true,
					},
					{
						Title: "Name",
						Value: e.PodName,
						Short: true,
					},
					{
						Title: "Container",
						Value: e.ContainerName,
						Short: true,
					},
					{
						Title: "Namespace",
						Value: e.Namespace,
						Short: true,
					},
					{
						Title: "Reason",
						Value: e.Reason,
						Short: true,
					},
					{
						Title: ":mag: Events",
						Value: "```\n" + events + " \n```",
						Short: false,
					},
					{
						Title: ":memo: Logs",
						Value: "```\n" + logs + "\n```",
						Short: false,
					},
				},
			},
		}
	}

	str, _ := json.Marshal(payload)
	return str
}
