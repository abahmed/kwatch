package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/ratelimit"

	"k8s.io/klog/v2"
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
		klog.InfoS("initializing webhook with empty url")
		return nil
	}
	rawHeaders, ok := config["headers"]
	var headers []KeyValue
	if ok {
		headerArray, ok := rawHeaders.([]interface{})
		if ok {
			for _, header := range headerArray {
				headerJson, err := json.Marshal(header)
				if err != nil {
					klog.InfoS("skipping invalid header", "error", err)
					continue
				}
				var k KeyValue
				if err := json.Unmarshal(headerJson, &k); err != nil {
					klog.InfoS("skipping invalid webhook header", "error", err)
					continue
				}
				headers = append(headers, k)
			}
		}
	}

	basicAuth := config["basicAuth"]
	basicAuthJson, err := json.Marshal(basicAuth)
	if err != nil {
		klog.InfoS("invalid basic auth config", "error", err)
		basicAuthJson = []byte("{}")
	}

	var a Authentication
	if err := json.Unmarshal(basicAuthJson, &a); err != nil {
		klog.InfoS("invalid webhook basicAuth, ignoring", "error", err)
		a = Authentication{}
	}

	klog.InfoS("initializing webhook",
		"url", url,
		"headers", headers,
		"username", a.UserName)

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
	client := k8s.GetDefaultClient()

	reqBody, err := w.buildRequestBody(ev)
	if err != nil {
		return err
	}
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

	if response.StatusCode == http.StatusTooManyRequests {
		return &ratelimit.Error{
			Provider:   "Webhook",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: ratelimit.ParseRetryAfter(response),
		}
	}
	if response.StatusCode > 299 {
		return fmt.Errorf(
			"call to webhook returned status code %d",
			response.StatusCode)
	}

	return nil
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message, then POSTs it as JSON.
func (w *Webhook) SendIncident(inc *model.Incident, action model.IncidentAction) error {
	text := util.RenderIncident(inc, action, message.NewPlainTextRenderer(), w.appCfg.ClusterName)
	if text == "" {
		return nil
	}

	client := k8s.GetDefaultClient()

	payload, err := json.Marshal(map[string]interface{}{
		"Cluster": w.appCfg.ClusterName,
		"Name":    inc.Name,
		"Reason":  inc.Reason,
		"Message": text,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal webhook incident payload: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, w.webhook, bytes.NewBuffer(payload))
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

	if response.StatusCode == http.StatusTooManyRequests {
		return &ratelimit.Error{
			Provider:   "Webhook",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: ratelimit.ParseRetryAfter(response),
		}
	}
	if response.StatusCode > 299 {
		return fmt.Errorf(
			"call to webhook returned status code %d",
			response.StatusCode)
	}

	return nil
}

func (w *Webhook) buildRequestBody(
	ev *event.Event,
) ([]byte, error) {
	eventsText := ""
	if ev.IncludeEvents {
		eventsText = strings.TrimSpace(ev.Events)
	}

	logsText := ""
	if ev.IncludeLogs {
		logsText = strings.TrimSpace(ev.Logs)
	}

	postBody, err := json.Marshal(map[string]interface{}{
		"Cluster":   w.appCfg.ClusterName,
		"Name":      ev.PodName,
		"Container": ev.ContainerName,
		"Namespace": ev.Namespace,
		"Node":      ev.NodeName,
		"Reason":    ev.Reason,
		"Events":    eventsText,
		"Logs":      logsText,
		"Labels":    ev.Labels,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	return postBody, nil

}
