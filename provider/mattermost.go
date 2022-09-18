package provider

import (
	"bytes"
	"encoding/json"
	"errors"
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
)

type mattermost struct {
	webhook string
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
func NewMattermost(url string) Provider {
	if len(url) == 0 {
		logrus.Warnf("initializing mattermost with empty webhook url")
	} else {
		logrus.Infof("initializing mattermost with webhook url: %s", url)
	}

	return &mattermost{
		webhook: url,
	}
}

// Name returns name of the provider
func (m *mattermost) Name() string {
	return "Mattermost"
}

// SendMessage sends text message to the provider
func (m *mattermost) SendMessage(msg string) error {
	logrus.Debugf("sending to mattermost msg: %s", msg)

	// validate webhook url
	_, err := m.validateWebhook()
	if err != nil {
		return err
	}

	reqBody, err := m.buildMessage(nil, &msg)
	if err != nil {
		return err
	}
	return m.sendAPI(reqBody)
}

// SendEvent sends event to the provider
func (m *mattermost) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to mattermost event: %v", e)

	// validate webhook url
	_, err := m.validateWebhook()
	if err != nil {
		return err
	}
	reqBody, err := m.buildMessage(e, nil)
	if err != nil {
		return err
	}
	return m.sendAPI(reqBody)
}

func (m *mattermost) validateWebhook() (bool, error) {
	if len(m.webhook) == 0 {
		return false, errors.New("webhook url is empty")
	}
	return true, nil
}

func (m *mattermost) sendAPI(content []byte) error {
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

	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to mattermost alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	if err != nil {
		return err
	}

	return err
}

func (m *mattermost) buildMessage(e *event.Event, msg *string) ([]byte, error) {
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

	return json.Marshal(payload)
}
