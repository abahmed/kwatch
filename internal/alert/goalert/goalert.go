package goalert

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	goalertAPIPath = "/api/v2/generic/incoming"
)

// goalertPayload is GoAlert's generic incoming alert. The integration token
// selects the service; dedup identifies the alert and action=close ends it.
type goalertPayload struct {
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
	Dedup   string `json:"dedup"`
}

type Goalert struct {
	sender    transport.Sender
	url       string
	token     string
	serviceID string

	clusterName string
}

// NewGoalert returns a new Goalert object

func NewGoalert(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Goalert {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing goalert with empty token")
		return nil
	}

	serviceID, ok := config["serviceId"].(string)
	if !ok || len(serviceID) == 0 {
		klog.InfoS("initializing goalert with empty serviceId")
		return nil
	}

	server, ok := config["url"].(string)
	if !ok || strings.TrimSpace(server) == "" {
		klog.InfoS("initializing goalert with empty url")
		return nil
	}

	host, valid := goalertHost(server)
	if !valid {
		klog.InfoS("initializing goalert with an invalid or example url")
		return nil
	}
	klog.InfoS("initializing goalert", "host", host, "serviceID", serviceID)

	return &Goalert{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + goalertAPIPath,
		token:       token,
		serviceID:   serviceID,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Goalert) Name() string {
	return "GoAlert"
}

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so GoAlert can close the alert.
func (s *Goalert) UsesEventDelivery() {}

// SendEvent creates or closes one GoAlert alert per kwatch incident, keyed by
// dedup.
func (s *Goalert) SendEvent(ctx context.Context, e *event.Event) error {
	payload := goalertPayload{
		Summary: e.AlertTitle(250),
		Details: e.AlertBody(s.clusterName),
		Dedup:   e.AlertKey(),
	}
	if e.IsResolve() {
		payload.Action = "close"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + s.token,
		},
	})
	return err
}

// SendMessage sends a plain notice as one deduplicated alert.
func (s *Goalert) SendMessage(ctx context.Context, msg string) error {
	return s.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}

// goalertHost validates the configured server. Reserved example domains
// (RFC 2606) are rejected so a copied sample config never sends the
// integration token to a host nobody controls.
func goalertHost(server string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(server))
	if err != nil || parsed.Hostname() == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, reserved := range []string{
		"example.com", "example.net", "example.org", "example",
	} {
		if host == reserved || strings.HasSuffix(host, "."+reserved) {
			return "", false
		}
	}
	return host, true
}
