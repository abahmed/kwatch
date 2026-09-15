package newrelic

import (
	"context"
	"encoding/json"
	"fmt"

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
func (n *NewRelic) SendEvent(ctx context.Context, e *event.Event) error {
	return n.SendMessage(ctx, e.FormatText(n.clusterName, ""))
}

// SendMessage sends text message to the provider
func (n *NewRelic) SendMessage(ctx context.Context, msg string) error {
	eventPayload := map[string]interface{}{
		"eventType": "KwatchAlert",
		"cluster":   n.clusterName,
		"message":   msg,
	}

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
