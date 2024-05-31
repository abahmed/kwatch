package opsgenie

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

const (
	defaultOpsgenieTitle = "kwatch detected a crash in pod: %s"
	defaultOpsgenieText  = "There is an issue with container (%s) in pod (%s)"
	opsgenieAPIURL       = "https://api.opsgenie.com/v2/alerts"
)

type Opsgenie struct {
	apikey string
	url    string
	title  string
	text   string

	// reference for general app configuration
	appCfg *config.App
}

type ogPayload struct {
	Message     string      `json:"message"`
	Description string      `json:"description"`
	Details     interface{} `json:"details"`
	Priority    string      `json:"priority"`
}

// NewOpsgenie returns new opsgenie instance
func NewOpsgenie(config map[string]interface{}, appCfg *config.App) *Opsgenie {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		logrus.Warnf("initializing opsgenie with empty webhook url")
		return nil
	}

	logrus.Infof("initializing opsgenie with secret apikey")

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Opsgenie{
		apikey: apiKey,
		url:    opsgenieAPIURL,
		title:  title,
		text:   text,
		appCfg: appCfg,
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
	defer response.Body.Close()

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
		title = fmt.Sprintf(defaultOpsgenieTitle, e.PodName)
	}
	payload.Message = title

	// use custom text if it's provided, otherwise use default
	text := m.text
	if len(text) == 0 {
		text = fmt.Sprintf(defaultOpsgenieText, e.ContainerName, e.PodName)
	}

	payload.Description = text
	payload.Details = map[string]string{
		"Cluster":   m.appCfg.ClusterName,
		"Name":      e.PodName,
		"Container": e.ContainerName,
		"Namespace": e.Namespace,
		"Reason":    e.Reason,
		"Events":    events,
		"Logs":      logs,
	}

	str, _ := json.Marshal(payload)
	return str
}
