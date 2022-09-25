package mattermost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"net/http"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	defaultMattermostLogs   = "No logs captured"
	defaultMattermostEvents = "No events captured"
	footer                  = "<https://github.com/abahmed/kwatch|kwatch>"
	defaultTitle            = ":red_circle: kwatch detected a crash in pod"
	defaultText             = "There is an issue with container in a pod!"
	chunkSize               = 80
)

type Mattermost struct {
	webhook string
	send    func(content []byte) error
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
func NewMattermost(config map[string]string) *Mattermost {
	webhook, ok := config["webhook"]
	if !ok || len(webhook) == 0 {
		logrus.Warnf("initializing mattermost with empty webhook url")
		return nil
	}

	logrus.Infof("initializing mattermost with webhook url: %s", webhook)

	return &Mattermost{
		webhook: webhook,
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
	request, _ := http.NewRequest(http.MethodPost, m.webhook, buffer)
	request.Header.Set("Content-Type", "application/json")

	response, _ := client.Do(request)
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
		logs := defaultMattermostLogs
		if len(e.Logs) > 0 {
			logs = (e.Logs)
		}

		events := defaultMattermostEvents
		if len(e.Events) > 0 {
			events = (e.Events)
		}

		// use custom title if it's provided, otherwise use default
		title := viper.GetString("alert.mattermost.title")
		if len(title) == 0 {
			title = defaultTitle
		}

		// use custom text if it's provided, otherwise use default
		text := viper.GetString("alert.mattermost.text")
		if len(text) == 0 {
			text = defaultText
		}

		payload.Attachments = []mmAttachment{
			{
				Title: title,
				Text:  text,
				Fields: []mmField{
					{
						Title: "Name",
						Value: e.Name,
						Short: true,
					},
					{
						Title: "Container",
						Value: e.Container,
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
