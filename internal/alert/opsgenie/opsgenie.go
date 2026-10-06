package opsgenie

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	opsgenieAPIURL   = "https://api.opsgenie.com/v2/alerts"
	opsgenieEUAPIURL = "https://api.eu.opsgenie.com/v2/alerts"
	// messageLimit is Opsgenie's maximum alert message length.
	messageLimit = 130
	// descriptionLimit is Opsgenie's maximum description length.
	descriptionLimit = 15000
)

// Opsgenie opens one alert per incident, keyed by its alias, and closes
// it when the incident resolves.
type Opsgenie struct {
	sender transport.Sender
	apikey string
	// url is the alerts endpoint; per-alert actions live below it.
	url string

	clusterName string
	// wait pauses between update attempts; tests replace it.
	wait func(ctx context.Context, d time.Duration) error
}

type ogPayload struct {
	Message     string            `json:"message"`
	Description string            `json:"description"`
	Details     map[string]string `json:"details,omitempty"`
	Priority    string            `json:"priority"`
	Alias       string            `json:"alias"`
}

// NewOpsgenie returns new opsgenie instance
func NewOpsgenie(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Opsgenie {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing opsgenie with empty webhook url")
		return nil
	}

	klog.InfoS("initializing opsgenie with secret apikey")

	apiURL := opsgenieAPIURL
	switch region, _ := config["region"].(string); region {
	case "", "us":
	case "eu":
		apiURL = opsgenieEUAPIURL
	default:
		klog.InfoS("initializing opsgenie with an invalid region",
			"region", region)
		return nil
	}

	return &Opsgenie{
		sender:      transport.NewSender(dependencies),
		apikey:      apiKey,
		url:         apiURL,
		clusterName: clusterName,
		wait:        sleepContext,
	}
}

// sleepContext waits for d or until ctx ends.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Name returns name of the provider
func (o *Opsgenie) Name() string {
	return "Opsgenie"
}

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (o *Opsgenie) SendMessage(ctx context.Context, msg string) error {
	return o.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (o *Opsgenie) SkipsPlainMessages() bool { return true }

// SendIncident creates or updates the incident's alert, or closes it when
// the incident resolved.
func (o *Opsgenie) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	// A plain notice (startup, upgrade, test) or the startup summary is
	// not an incident. Sending it would page for problems that already
	// have their own alerts, so it is skipped.
	if m.IsInformational() {
		klog.V(4).InfoS("skipping informational message",
			"component", "delivery", "provider", o.Name())
		return nil
	}
	alias := m.AlertKey(o.clusterName)
	if m.Resolved() {
		err := o.send(ctx, "POST", o.actionURL(alias, "close"), []byte(`{}`))
		if transport.IsNotFound(err) {
			// No such alert: it is already closed, or was never opened.
			// Either way there is nothing left to close.
			return nil
		}
		return err
	}
	payload := o.buildPayload(m)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal opsgenie payload: %w", err)
	}
	if err := o.send(ctx, "POST", o.url, body); err != nil {
		return err
	}
	if m.Revision > 1 {
		return o.updateAlert(ctx, alias, payload)
	}
	return nil
}

// updateAlert copies a later revision onto the open alert. Opsgenie's
// create call only deduplicates an alert that already exists and never
// changes its priority, message or description, so without this a warning
// that becomes critical would keep its old, lower priority.
func (o *Opsgenie) updateAlert(
	ctx context.Context, alias string, p ogPayload,
) error {
	updates := []struct {
		action string
		body   map[string]string
	}{
		{"priority", map[string]string{"priority": p.Priority}},
		{"message", map[string]string{"message": p.Message}},
		{"description", map[string]string{"description": p.Description}},
	}
	for _, update := range updates {
		body, err := json.Marshal(update.body)
		if err != nil {
			return fmt.Errorf("failed to marshal opsgenie %s: %w",
				update.action, err)
		}
		target := o.actionURL(alias, update.action)
		if err := o.sendUpdate(ctx, target, body); err != nil {
			return fmt.Errorf("update opsgenie %s: %w", update.action, err)
		}
	}
	return nil
}

// updateAttempts and updateRetryDelay bound how long an update waits for
// an alert that Opsgenie is still creating.
const (
	updateAttempts   = 3
	updateRetryDelay = time.Second
)

// sendUpdate sends one PUT. Opsgenie creates alerts asynchronously, so an
// update that follows the create at once can answer 404 for a moment. That
// is retried a few times; if the alert is still unknown the error is
// returned as retryable, so delivery tries again later instead of
// treating the 404 as permanent.
func (o *Opsgenie) sendUpdate(
	ctx context.Context, target string, body []byte,
) error {
	var err error
	for attempt := 1; attempt <= updateAttempts; attempt++ {
		err = o.send(ctx, "PUT", target, body)
		if err == nil || !transport.IsNotFound(err) {
			return err
		}
		if attempt == updateAttempts {
			break
		}
		delay := updateRetryDelay * time.Duration(attempt)
		if waitErr := o.wait(ctx, delay); waitErr != nil {
			return waitErr
		}
	}
	// A new error without the permanent marker: the alert is probably
	// still being created.
	return errors.New(err.Error() + " (alert not visible yet, will retry)")
}

// actionURL addresses one alert by its alias, e.g. .../{alias}/close.
func (o *Opsgenie) actionURL(alias, action string) string {
	return o.url + "/" + url.PathEscape(alias) + "/" + action +
		"?identifierType=alias"
}

func (o *Opsgenie) send(
	ctx context.Context, method, target string, body []byte,
) error {
	_, err := o.sender.Send(ctx, transport.Request{
		Provider: "Opsgenie",
		Method:   method,
		URL:      target,
		Body:     body,
		Headers:  map[string]string{"Authorization": "GenieKey " + o.apikey},
	})
	return err
}

// opsgeniePriority maps the incident severity onto Opsgenie's P1-P5 scale.
// Only critical incidents are P1, the level that pages immediately; an
// unknown severity is P3, which notifies without paging.
func opsgeniePriority(m notification.Message) string {
	switch {
	case m.Route.Severity == "critical":
		return "P1"
	case m.Route.Severity == "warning":
		return "P3"
	case m.Route.Severity == "info":
		return "P4"
	case m.Status == notification.StatusCritical:
		return "P1"
	}
	return "P3"
}

func (o *Opsgenie) buildPayload(m notification.Message) ogPayload {
	details := map[string]string{}
	if o.clusterName != "" {
		details["Cluster"] = o.clusterName
	}
	if len(m.Route.Namespaces) > 0 {
		details["Namespace"] = strings.Join(m.Route.Namespaces, ",")
	}
	if len(m.Route.Reasons) > 0 {
		details["Reason"] = strings.Join(m.Route.Reasons, ",")
	}
	description := m.NoteText()
	if len(m.Output) > 0 {
		description += "\n\n" + strings.Join(m.Output, "\n")
	}
	return ogPayload{
		Message:     notification.Truncate(m.ShortText(), messageLimit),
		Description: notification.Truncate(description, descriptionLimit),
		Details:     details,
		Priority:    opsgeniePriority(m),
		Alias:       m.AlertKey(o.clusterName),
	}
}
