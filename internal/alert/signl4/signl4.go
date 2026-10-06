package signl4

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const signl4APIURL = "https://connect.signl4.com/webhook"

type signl4Payload struct {
	Title      string `json:"title"`
	Message    string `json:"message"`
	Severity   string `json:"severity,omitempty"`
	User       string `json:"user,omitempty"`
	XS4Status  string `json:"X-S4-Status,omitempty"`
	ExternalID string `json:"X-S4-ExternalID,omitempty"`
}

type Signl4 struct {
	sender     transport.Sender
	url        string
	teamSecret string
	title      string
	user       string

	clusterName string
}

// NewSignl4 returns a new Signl4 object

func NewSignl4(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Signl4 {
	teamSecret, ok := config["teamSecret"].(string)
	if !ok || len(teamSecret) == 0 {
		klog.InfoS("initializing signl4 with empty teamSecret")
		return nil
	}

	server := signl4APIURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		if !transport.ValidEndpoint(s) {
			klog.InfoS("initializing signl4 with an invalid url",
				"setting", "url")
			return nil
		}
		server = s
	}

	title, _ := config["title"].(string)
	user, _ := config["user"].(string)

	klog.InfoS("initializing signl4", "title", title)

	return &Signl4{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + "/" + teamSecret,
		teamSecret:  teamSecret,
		title:       title,
		user:        user,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Signl4) Name() string {
	return "SIGNL4"
}

// maxTitleBytes keeps the alert title within what SIGNL4 displays.
const maxTitleBytes = 250

// SendIncident raises or resolves one SIGNL4 alert per kwatch incident,
// keyed by X-S4-ExternalID.
func (s *Signl4) SendIncident(
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
	title := s.title
	if len(title) == 0 {
		title = notification.Truncate(m.ShortText(), maxTitleBytes)
	}
	status := "new"
	if m.Resolved() {
		status = "resolved"
	}
	payload := signl4Payload{
		Title:      title,
		Message:    alertBody(m, s.clusterName),
		Severity:   severityFor(m),
		User:       s.user,
		XS4Status:  status,
		ExternalID: m.AlertKey(s.clusterName),
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

// severityFor is the incident's routing severity; a message without one
// (a plain notice) is informational unless its status is critical.
func severityFor(m notification.Message) string {
	if m.Route.Severity != "" {
		return m.Route.Severity
	}
	if m.Status == notification.StatusCritical {
		return "critical"
	}
	return "info"
}

// alertBody is the narrative, the recent output and the cluster.
func alertBody(m notification.Message, clusterName string) string {
	body := safetext.PlainWithOutput(
		m.NoteText(), safetext.LastOutput(m.Output), "\n\n", safetext.DetailsLimit)
	if clusterName != "" {
		body += "\n\nCluster: " + clusterName
	}
	return body
}

// SendMessage skips plain notices; SIGNL4 alerts are incidents only.
func (s *Signl4) SendMessage(ctx context.Context, msg string) error {
	return s.SendIncident(ctx, notification.Notice(msg))
}

// SkipsPlainMessages implements api.PlainMessageSkipper: plain messages
// become notices, which SendIncident skips.
func (s *Signl4) SkipsPlainMessages() bool { return true }
