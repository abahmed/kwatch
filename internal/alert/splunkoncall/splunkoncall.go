package splunkoncall

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const splunkOnCallAPIURL = "https://alert.victorops.com/integrations/generic/20131114/alert"

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

// SendEvent sends event to the provider
func (s *SplunkOncall) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *SplunkOncall) SendMessage(ctx context.Context, msg string) error {
	entityID := "kwatch"
	if len(s.clusterName) > 0 {
		entityID = s.clusterName
	}

	payload := splunkOnCallPayload{
		MessageType:       "CRITICAL",
		EntityID:          entityID,
		EntityDisplayName: "kwatch alert",
		StateMessage:      msg,
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
