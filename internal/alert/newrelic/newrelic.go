package newrelic

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const newRelicAPIURL = "https://insights-collector.newrelic.com/v1/accounts/%s/events"

type NewRelic struct {
	sender    transport.Sender
	url       string
	apiKey    string
	accountID string

	clusterName string
}

// NewNewRelic returns a new NewRelic object

func NewNewRelic(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *NewRelic {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing newrelic with empty apiKey")
		return nil
	}

	accountID, ok := config["accountId"].(string)
	if !ok || len(accountID) == 0 {
		klog.InfoS("initializing newrelic with empty accountId")
		return nil
	}

	klog.InfoS("initializing newrelic", "accountId", accountID)

	return &NewRelic{
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf(newRelicAPIURL, accountID),
		apiKey:      apiKey,
		accountID:   accountID,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (n *NewRelic) Name() string {
	return "New Relic"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes problems through SendEvent, which carries the
// problem key and action as queryable event attributes.
func (n *NewRelic) UsesEventDelivery() {}

// SendEvent records one KwatchAlert event with the problem key and action,
// so New Relic queries and alert conditions can follow a problem.
func (n *NewRelic) SendEvent(ctx context.Context, e *event.Event) error {
	action := e.Action
	if e.IsNotice() {
		action = "notice"
	}
	return n.send(ctx, map[string]interface{}{
		"eventType":   "KwatchAlert",
		"cluster":     n.clusterName,
		"message":     truncateMessage(e.AlertBody(n.clusterName), 64*1024),
		"title":       e.AlertTitle(250),
		"incidentKey": e.AlertKey(),
		"action":      action,
		"reason":      e.Reason,
		"namespace":   e.Namespace,
		"severity":    string(e.Severity),
	})
}

// SendMessage records a plain notice.
func (n *NewRelic) SendMessage(ctx context.Context, msg string) error {
	return n.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}

func (n *NewRelic) send(
	ctx context.Context, eventPayload map[string]interface{},
) error {
	body, err := json.Marshal([]interface{}{eventPayload})
	if err != nil {
		return err
	}
	_, err = n.sender.Send(ctx, transport.Request{
		Provider: n.Name(), URL: n.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Api-Key": n.apiKey,
		},
	})
	return err
}

func truncateMessage(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes - len("\n…(truncated)")
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "\n…(truncated)"
}
