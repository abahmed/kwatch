package webhook

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"

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
	reqBody, err := w.buildRequestBody(ev)
	if err != nil {
		return err
	}
	_, err = util.Send(w.request(reqBody))
	return err
}

// request builds the call every webhook delivery makes: the user's headers
// and optional basic auth on top of a JSON POST.
func (w *Webhook) request(body []byte) util.Request {
	r := util.Request{
		Provider: "Webhook",
		URL:      w.webhook,
		Body:     body,
		Headers:  make(map[string]string, len(w.headers)),
	}
	for _, header := range w.headers {
		r.Headers[header.Name] = header.Value
	}
	if len(w.username) > 0 && len(w.password) > 0 {
		r.BasicAuth = &util.BasicAuth{
			Username: w.username,
			Password: w.password,
		}
	}
	return r
}

// SendIncident implements alert.ThreadProvider.
// It renders the incident using the Report model and PlaintextRenderer,
// producing a context-adaptive text message, then POSTs it as JSON.
func (w *Webhook) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	return w.SendIncidentWithInsight(inc, action, nil)
}

// SendIncidentWithInsight implements alert.InsightThreadProvider, so the
// diagnosis — likely cause, impact, recent changes — is rendered rather than
// dropped on the way to this provider.
func (w *Webhook) SendIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
) error {
	text := util.RenderIncidentWithInsight(
		inc,
		action,
		ins,
		message.NewPlainTextRenderer(),
		w.appCfg.ClusterName,
	)
	if text == "" {
		return nil
	}

	payload, err := json.Marshal(map[string]interface{}{
		"Cluster": w.appCfg.ClusterName,
		"Name":    inc.Name,
		"Reason":  inc.Reason,
		"Message": text,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal webhook incident payload: %w", err)
	}
	_, err = util.Send(w.request(payload))
	return err
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
