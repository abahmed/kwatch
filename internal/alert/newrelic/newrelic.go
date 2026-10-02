package newrelic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
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

// New Relic event attribute limits.
const (
	maxTitleBytes   = 250
	maxMessageBytes = 4096
)

// SendIncident records one KwatchAlert event carrying the incident key and
// state, so New Relic queries and alert conditions can follow an incident
// from firing to resolved.
func (n *NewRelic) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return n.send(ctx, buildEvent(m, n.clusterName))
}

func buildEvent(
	m notification.Message, clusterName string,
) map[string]interface{} {
	state := "firing"
	switch {
	case m.Resolved():
		state = "resolved"
	case m.IsNotice():
		state = "notice"
	}
	message := m.NoteText()
	if len(m.Output) > 0 {
		message += "\n\nLast output:\n" + strings.Join(m.Output, "\n")
	}
	return map[string]interface{}{
		"eventType":   "KwatchAlert",
		"cluster":     clusterName,
		"title":       notification.Truncate(m.ShortText(), maxTitleBytes),
		"message":     notification.Truncate(message, maxMessageBytes),
		"incidentKey": m.AlertKey(clusterName),
		"revision":    m.Revision,
		"state":       state,
		"action":      state,
		"status":      m.Status.String(),
		"reason":      strings.Join(m.Route.Reasons, ","),
		"namespace":   strings.Join(m.Route.Namespaces, ","),
		"severity":    m.Route.Severity,
	}
}

// SendMessage records a plain notice.
func (n *NewRelic) SendMessage(ctx context.Context, msg string) error {
	return n.SendIncident(ctx, notification.Notice(msg))
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
