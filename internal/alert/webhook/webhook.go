package webhook

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"

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
	sender      transport.Sender
	webhook     string
	headers     []KeyValue
	username    string
	password    string
	clusterName string
	clockSource clock.Clock
}

func (w *Webhook) SendMessage(ctx context.Context, msg string) error {
	payload, err := json.Marshal(map[string]string{
		"Cluster": w.clusterName,
		"Message": msg,
	})
	if err != nil {
		return fmt.Errorf("marshal webhook message: %w", err)
	}
	_, err = w.sender.Send(ctx, w.request(payload))
	return err
}

// NewWebhook constructs a Webhook provider from its settings.
func NewWebhook(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Webhook {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing webhook with empty url")
		return nil
	}

	if !transport.ValidEndpoint(url) {
		klog.InfoS("initializing webhook with an invalid url",
			"setting", "url")
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

	klog.InfoS("initializing webhook with configured authentication")

	return &Webhook{
		sender:      transport.NewSender(dependencies),
		webhook:     url,
		headers:     headers,
		username:    a.UserName,
		password:    a.Password,
		clusterName: clusterName,
		clockSource: clock.Require(dependencies.Clock),
	}
}

// Name returns name of the provider
func (w *Webhook) Name() string {
	return "Webhook"
}

// request builds the call every webhook delivery makes: the user's headers
// and optional basic auth on top of a JSON POST.
func (w *Webhook) request(body []byte) transport.Request {
	r := transport.Request{
		Provider: "Webhook",
		URL:      w.webhook,
		Body:     body,
		Headers:  make(map[string]string, len(w.headers)),
	}
	for _, header := range w.headers {
		r.Headers[header.Name] = header.Value
	}
	if len(w.username) > 0 && len(w.password) > 0 {
		r.BasicAuth = &transport.BasicAuth{
			Username: w.username,
			Password: w.password,
		}
	}
	return r
}
