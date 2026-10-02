package goalert

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const (
	goalertAPIPath = "/api/v2/generic/incoming"
	// summaryLimit bounds the alert summary GoAlert shows in lists.
	summaryLimit = 250
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

// SendIncident creates, updates or closes one GoAlert alert per kwatch
// incident, keyed by dedup.
func (s *Goalert) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	// A plain notice (startup, upgrade, test) or the startup summary is
	// not an incident. Sending it would page for problems that already
	// have their own alerts, so it is skipped.
	if m.IsInformational() {
		klog.V(4).InfoS("skipping informational message",
			"component", "delivery", "provider", s.Name())
		return nil
	}
	details := m.NoteText()
	if len(m.Output) > 0 {
		details += "\n\n" + strings.Join(m.Output, "\n")
	}
	payload := goalertPayload{
		Summary: notification.Truncate(m.ShortText(), summaryLimit),
		Details: details,
		Dedup:   m.AlertKey(s.clusterName),
	}
	if m.Resolved() {
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

// SendMessage skips plain notices: on a paging service they would open an
// alert that nothing resolves.
func (s *Goalert) SendMessage(ctx context.Context, msg string) error {
	return s.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (s *Goalert) SkipsPlainMessages() bool { return true }

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
