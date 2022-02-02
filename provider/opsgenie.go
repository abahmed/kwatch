package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	defaultOpsgenieTitle = "kwatch detected a crash in pod: %s"
	defaultOpsgenieText  = "There is an issue with container (%s) in pod (%s)"
	opsgenieAPIURL       = "https://api.opsgenie.com/v2/alerts"
)

type opsgenie struct {
	apikey string
}

type ogPayload struct {
	Message     string      `json:"message"`
	Description string      `json:"description"`
	Details     interface{} `json:"details"`
	Priority    string      `json:"priority"`
}

// NewOpsgenie returns new opsgenie instance
func NewOpsgenie(apikey string) Provider {
	if len(apikey) == 0 {
		logrus.Warnf("initializing opsgenie with empty webhook url")
	} else {
		logrus.Infof("initializing opsgenie with secret apikey")
	}

	return &opsgenie{
		apikey: apikey,
	}
}

// Name returns name of the provider
func (m *opsgenie) Name() string {
	return "Opsgenie"
}

// SendMessage sends text message to the provider
func (m *opsgenie) SendMessage(msg string) error {
	return nil
}

// SendEvent sends event to the provider
func (m *opsgenie) SendEvent(e *event.Event) error {
	logrus.Debugf("sending to opsgenie event: %v", e)

	// validate webhook url
	if len(m.apikey) == 0 {
		errors.New("apikey url is empty")
	}

	reqBody, err := m.buildMessage(e)
	if err != nil {
		return err
	}
	return m.sendAPI(reqBody)
}

// sendAPI sends http request to Opsgenie API
func (m *opsgenie) sendAPI(content []byte) error {
	client := &http.Client{}
	buffer := bytes.NewBuffer(content)
	request, err := http.NewRequest(http.MethodPost, opsgenieAPIURL, buffer)

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
		body, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf(
			"call to opsgenie alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	if err != nil {
		return err
	}

	return err
}

func (m *opsgenie) buildMessage(e *event.Event) ([]byte, error) {
	payload := ogPayload{
		Priority: "P1",
	}

	if e == nil {
		return nil, errors.New("trying to send empty event")
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
	return json.Marshal(payload)
}
