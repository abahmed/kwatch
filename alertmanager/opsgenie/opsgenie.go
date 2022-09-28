package opsgenie

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
	defaultOpsgenieTitle = "kwatch detected a crash in pod: %s"
	defaultOpsgenieText  = "There is an issue with container (%s) in pod (%s)"
	opsgenieAPIURL       = "https://api.opsgenie.com/v2/alerts"
	defaultTitle         = ":red_circle: kwatch detected a crash in pod"
	defaultText          = "There is an issue with container in a pod!"
	defaultLogs          = "No logs captured"
	defaultEvents        = "No events captured"
	defaultTeamsTitle    = "&#9937; Kwatch detected a crash in pod"
)

type Opsgenie struct {
	apikey string
	url    string
}

type ogPayload struct {
	Message     string      `json:"message"`
	Description string      `json:"description"`
	Details     interface{} `json:"details"`
	Priority    string      `json:"priority"`
}

// NewOpsgenie returns new opsgenie instance
func NewOpsgenie(config map[string]string) *Opsgenie {
	apiKey, ok := config["apikey"]
	if !ok || len(apiKey) == 0 {
		logrus.Warnf("initializing opsgenie with empty webhook url")
		return nil
	}

	logrus.Infof("initializing opsgenie with secret apikey")

	return &Opsgenie{
		apikey: apiKey,
		url:    opsgenieAPIURL,
	}
}

// Name returns name of the provider
func (m *Opsgenie) Name() string {
	return "Opsgenie"
}

// SendMessage sends text message to the provider
func (m *Opsgenie) SendMessage(msg string) error {
	return nil
}

// SendEvent sends event to the provider
func (m *Opsgenie) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to opsgenie event: %v", e)

	return m.sendAPI(m.buildMessage(e))
}

// sendAPI sends http request to Opsgenie API
func (m *Opsgenie) sendAPI(content []byte) error {
	client := &http.Client{}
	buffer := bytes.NewBuffer(content)
	request, err := http.NewRequest(http.MethodPost, m.url, buffer)
	if err != nil {
		return err
	}

	// set request headers
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "GenieKey "+m.apikey)

	response, err := client.Do(request)
	if err != nil {
		return err
	}

	if response.StatusCode != 202 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to opsgenie alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

func (m *Opsgenie) buildMessage(e *event.Event) []byte {
	payload := ogPayload{
		Priority: "P1",
	}

	logs := defaultLogs
	if len(e.Logs) > 0 {
		logs = (e.Logs)
	}

	events := defaultEvents
	if len(e.Events) > 0 {
		events = (e.Events)
	}

	// use custom title if it's provided, otherwise use default
	title := viper.GetString("alert.opsgenie.title")
	if len(title) == 0 {
		title = fmt.Sprintf(defaultOpsgenieTitle, e.Name)
	}
	payload.Message = title

	// use custom text if it's provided, otherwise use default
	text := viper.GetString("alert.opsgenie.text")
	if len(text) == 0 {
		text = fmt.Sprintf(defaultOpsgenieText, e.Container, e.Name)
	}

	payload.Description = text
	payload.Details = map[string]string{
		"Name":      e.Name,
		"Container": e.Container,
		"Namespace": e.Namespace,
		"Reason":    e.Reason,
		"Events":    events,
		"Logs":      logs,
	}

	str, _ := json.Marshal(payload)
	return str
}
