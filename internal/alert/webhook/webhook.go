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
	headers, err := parseHeaders(config["headers"])
	if err != nil {
		klog.ErrorS(err, "invalid webhook headers setting")
		return nil
	}
	a, err := parseBasicAuth(config["basicAuth"])
	if err != nil {
		klog.ErrorS(err, "invalid webhook basicAuth setting")
		return nil
	}
	warnPlainHTTP(url, len(headers) > 0 || a.UserName != "")

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
	if w.username != "" && w.password != "" {
		r.BasicAuth = &transport.BasicAuth{
			Username: w.username,
			Password: w.password,
		}
	}
	return r
}
