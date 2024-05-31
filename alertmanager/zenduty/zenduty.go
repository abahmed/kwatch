package zenduty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/event"
	"github.com/sirupsen/logrus"
)

const (
	defaultZendutyTitle = "kwatch detected a crash in pod: %s"
	defaultZendutyText  = "There is an issue with container (%s) in pod (%s)"
	zendutyAPIURL       = "https://www.zenduty.com/api/events"
)

var AlertTypes = []string{
	"critical",
	"acknowledged",
	"resolved",
	"error",
	"warning",
	"info",
}

type Zenduty struct {
	integrationkey string
	url            string
	alertType      string

	// reference for general app configuration
	appCfg *config.App
}

type zendutyPayload struct {
	Message   string `json:"message"`
	Summary   string `json:"summary"`
	AlertType string `json:"alert_type"`
}

// NewZenduty returns new zenduty instance
func NewZenduty(config map[string]interface{}, appCfg *config.App) *Zenduty {
	integrationKey, ok := config["integrationKey"].(string)
	if !ok || len(integrationKey) == 0 {
		logrus.Warnf("initializing zenduty with empty webhook url")
		return nil
	}

	logrus.Infof("initializing zenduty with secret apikey")

	// If alert type is not provided, or provided with invalid value
	// it will fallback to critical type
	alertType, ok := config["alertType"].(string)
	if !ok || !slices.Contains(AlertTypes, alertType) {
		alertType = "critical"
	}

	return &Zenduty{
		integrationkey: integrationKey,
		url:            zendutyAPIURL,
		alertType:      alertType,
		appCfg:         appCfg,
	}
}

// Name returns name of the provider
func (m *Zenduty) Name() string {
	return "Zenduty"
}

// SendMessage sends text message to the provider
func (m *Zenduty) SendMessage(msg string) error {
	return nil
}

// SendEvent sends event to the provider
func (m *Zenduty) SendEvent(e *event.Event) error {
	return m.sendAPI(m.buildMessage(e))
}

// sendAPI sends http request to Zenduty API
func (m *Zenduty) sendAPI(content []byte) error {
	client := &http.Client{}
	buffer := bytes.NewBuffer(content)
	url := m.url + "/" + m.integrationkey + "/"
	request, err := http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return err
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 201 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to zenduty alert returned status code %d: %s",
			response.StatusCode,
			string(body))
	}

	return nil
}

func (m *Zenduty) buildMessage(e *event.Event) []byte {
	payload := zendutyPayload{
		AlertType: m.alertType,
	}

	logs := constant.DefaultLogs
	if len(e.Logs) > 0 {
		logs = (e.Logs)
	}

	events := constant.DefaultEvents
	if len(e.Events) > 0 {
		events = (e.Events)
	}

	payload.Message = fmt.Sprintf(defaultZendutyTitle, e.PodName)
	payload.Summary = fmt.Sprintf(
		"An alert has been triggered for\n\n"+
			"cluster: %s\n"+
			"Pod Name: %s\n"+
			"Container: %s\n"+
			"Namespace: %s\n"+
			"Reason: %s\n\n"+
			"Events:\n%s\n\n"+
			"Logs:\n%s\n\n",
		m.appCfg.ClusterName,
		e.PodName,
		e.ContainerName,
		e.Namespace,
		e.Reason,
		events,
		logs,
	)

	str, _ := json.Marshal(payload)
	return str
}
