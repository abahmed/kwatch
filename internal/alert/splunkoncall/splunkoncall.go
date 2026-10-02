package splunkoncall

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	splunkOnCallAPIURL = "https://alert.victorops.com/" +
		"integrations/generic/20131114/alert"
	// titleLimit bounds entity_display_name.
	titleLimit = 250
)

type splunkOnCallPayload struct {
	MessageType       string `json:"message_type"`
	EntityID          string `json:"entity_id"`
	EntityDisplayName string `json:"entity_display_name"`
	StateMessage      string `json:"state_message"`
}

type SplunkOncall struct {
	sender     transport.Sender
	url        string
	apiKey     string
	routingKey string

	clusterName string
}

// NewSplunkOncall returns a new SplunkOncall object

func NewSplunkOncall(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *SplunkOncall {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing splunkoncall with empty apiKey")
		return nil
	}

	routingKey, ok := config["routingKey"].(string)
	if !ok || len(routingKey) == 0 {
		klog.InfoS("initializing splunkoncall with empty routingKey")
		return nil
	}

	server := splunkOnCallAPIURL
	if u, ok := config["url"].(string); ok && len(u) > 0 {
		if !transport.ValidEndpoint(u) {
			klog.InfoS("initializing splunkoncall with an invalid url",
				"setting", "url")
			return nil
		}
		server = u
	}

	klog.InfoS("initializing splunkoncall", "routingKey", routingKey)

	return &SplunkOncall{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + "/" + routingKey + "/" + apiKey,
		apiKey:      apiKey,
		routingKey:  routingKey,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *SplunkOncall) Name() string {
	return "Splunk OnCall"
}

// SendIncident opens, updates or recovers one Splunk On-Call incident per
// kwatch incident, keyed by entity_id.
func (s *SplunkOncall) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	payload := splunkOnCallPayload{
		MessageType:       messageType(m),
		EntityID:          m.AlertKey(s.clusterName),
		EntityDisplayName: notification.Truncate(m.ShortText(), titleLimit),
		StateMessage:      stateMessage(m),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json",
	})
	return err
}

// messageType maps an incident onto Splunk On-Call's message types.
// Plain notices are INFO so they never open an incident.
func messageType(m notification.Message) string {
	switch {
	case m.Resolved():
		return "RECOVERY"
	case m.IsNotice(),
		m.Route.Severity == "info":
		return "INFO"
	case m.Route.Severity == "warning":
		return "WARNING"
	}
	return "CRITICAL"
}

func stateMessage(m notification.Message) string {
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n\n" + strings.Join(m.Output, "\n")
	}
	return text
}

// SendMessage sends a plain notice as an informational alert.
func (s *SplunkOncall) SendMessage(ctx context.Context, msg string) error {
	return s.SendIncident(ctx, notification.Notice(msg))
}
