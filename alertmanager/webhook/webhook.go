package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/util"

	"github.com/sirupsen/logrus"
)

type KeyValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Authentication struct {
	UserName string `json:"username"`
	Password string `json:"password"`
}

type Webhook struct {
	webhook  string
	headers  []KeyValue
	username string
	password string
	appCfg   *config.App
}

func (w *Webhook) SendMessage(msg string) error {
	return nil
}

// NewSlack returns new Slack instance
func NewWebhook(config map[string]interface{}, appCfg *config.App) *Webhook {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		logrus.Warnf("initializing  with empty webhook url")
		return nil
	}
	rawHeaders, ok := config["headers"]
	var headers []KeyValue
	if ok {
		headerArray := rawHeaders.([]interface{})
		for _, header := range headerArray {
			headerJson, _ := json.Marshal(header)
			var k KeyValue
			json.Unmarshal(headerJson, &k)
			headers = append(headers, k)
		}
	}

	basicAuth, ok := config["basicAuth"]
	basicAuthJson, _ := json.Marshal(basicAuth)

	var a Authentication
	json.Unmarshal(basicAuthJson, &a)

	logrus.Infof("initializing  with webhook url: %s "+
		"with headers: %s and username: %s", url, headers, a.UserName)

	return &Webhook{
		webhook:  url,
		headers:  headers,
		username: a.UserName,
		password: a.Password,
		appCfg:   appCfg,
	}
}

// Name returns name of the provider
func (w *Webhook) Name() string {
	return "Webhook"
}

// SendEvent sends event to the provider
func (w *Webhook) SendEvent(ev *event.Event) error {
	client := &http.Client{}

	reqBody := w.buildRequestBody(ev)
	buffer := bytes.NewBuffer(reqBody)

	request, err := http.NewRequest(http.MethodPost, w.webhook, buffer)
	if err != nil {
		return err
	}

	for _, header := range w.headers {
		request.Header.Set(header.Name, header.Value)
	}
	if len(w.username) > 0 && len(w.password) > 0 {
		request.SetBasicAuth(w.username, w.password)
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode > 202 {
		return fmt.Errorf(
			"call to teams alert returned status code %d",
			response.StatusCode)
	}

	return nil
}

func (w *Webhook) buildRequestBody(
	ev *event.Event,
) []byte {
	eventsText := "No events captured"
	logsText := "No logs captured"

	// add events part if it exists
	events := strings.TrimSpace(ev.Events)
	if len(events) > 0 {
		eventsText = util.JsonEscape(ev.Events)
	}

	// add logs part if it exists
	logs := strings.TrimSpace(ev.Logs)
	if len(logs) > 0 {
		logsText = util.JsonEscape(ev.Logs)
	}

	postBody, _ := json.Marshal(map[string]interface{}{
		"Cluster":   w.appCfg.ClusterName,
		"Name":      ev.PodName,
		"Container": ev.ContainerName,
		"Namespace": ev.Namespace,
		"Reason":    ev.Reason,
		"Events":    eventsText,
		"Logs":      logsText,
		"Labels":    ev.Labels,
	})

	return postBody
}
