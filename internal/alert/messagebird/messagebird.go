package messagebird

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const messagebirdAPIURL = "https://rest.messagebird.com/messages"

type messagebirdPayload struct {
	Originator string   `json:"originator"`
	Recipients []string `json:"recipients"`
	Body       string   `json:"body"`
}

type Messagebird struct {
	sender    transport.Sender
	url       string
	accessKey string
	from      string
	to        string

	clusterName string
}

// NewMessagebird returns a new Messagebird object

func NewMessagebird(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Messagebird {
	accessKey, ok := config["accessKey"].(string)
	if !ok || len(accessKey) == 0 {
		klog.InfoS("initializing messagebird with empty accessKey")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing messagebird with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing messagebird with empty to")
		return nil
	}

	klog.InfoS("initializing messagebird", "from", from, "to", to)

	return &Messagebird{
		sender:      transport.NewSender(dependencies),
		url:         messagebirdAPIURL,
		accessKey:   accessKey,
		from:        from,
		to:          to,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (m *Messagebird) Name() string {
	return "Messagebird"
}

// SendEvent sends event to the provider
func (m *Messagebird) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(m.clusterName, "")
	return m.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (m *Messagebird) SendMessage(ctx context.Context, msg string) error {
	payload := messagebirdPayload{
		Originator: m.from,
		Recipients: []string{m.to},
		Body:       msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = m.sender.Send(ctx, transport.Request{
		Provider: m.Name(), URL: m.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "AccessKey " + m.accessKey,
		},
	})
	return err
}
