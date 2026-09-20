package splunk

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

type splunkPayload struct {
	Event      map[string]interface{} `json:"event"`
	Source     string                 `json:"source,omitempty"`
	Sourcetype string                 `json:"sourcetype,omitempty"`
	Index      string                 `json:"index,omitempty"`
	Host       string                 `json:"host,omitempty"`
}

type Splunk struct {
	sender     transport.Sender
	url        string
	token      string
	source     string
	sourcetype string
	index      string
	host       string

	clusterName string
}

// NewSplunk returns a new Splunk object

func NewSplunk(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Splunk {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing splunk with empty url")
		return nil
	}

	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing splunk with empty token")
		return nil
	}

	source, _ := config["source"].(string)
	sourcetype, _ := config["sourcetype"].(string)
	index, _ := config["index"].(string)
	host, _ := config["host"].(string)

	klog.InfoS("initializing splunk", "url", url, "source", source)

	return &Splunk{
		sender:      transport.NewSender(dependencies),
		url:         url,
		token:       token,
		source:      source,
		sourcetype:  sourcetype,
		index:       index,
		host:        host,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Splunk) Name() string {
	return "Splunk"
}

// SendEvent sends event to the provider
func (s *Splunk) SendEvent(ctx context.Context, e *event.Event) error {
	return s.SendMessage(ctx, e.FormatText(s.clusterName, ""))
}

// SendMessage sends text message to the provider
func (s *Splunk) SendMessage(ctx context.Context, msg string) error {
	payload := splunkPayload{
		Event: map[string]interface{}{
			"message": msg,
			"source":  "kwatch",
		},
		Source:     s.source,
		Sourcetype: s.sourcetype,
		Index:      s.index,
		Host:       s.host,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Splunk " + s.token,
		},
	})
	return err
}
