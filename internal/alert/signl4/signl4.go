package signl4

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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

// SendEvent sends event to the provider
// UsesEventDelivery routes incidents through SendEvent, which carries the
// action and a stable key so SIGNL4 can close the alert.
func (s *Signl4) UsesEventDelivery() {}

// SendEvent raises or resolves one SIGNL4 alert per kwatch incident, keyed by
// X-S4-ExternalID.
func (s *Signl4) SendEvent(ctx context.Context, e *event.Event) error {
	title := s.title
	if len(title) == 0 {
		title = e.AlertTitle(250)
	}
	status, severity := "new", "critical"
	if e.IsResolve() {
		status = "resolved"
	} else if e.IsNotice() {
		severity = "info"
	}
	payload := signl4Payload{
		Title:      title,
		Message:    e.AlertBody(s.clusterName),
		Severity:   severity,
		User:       s.user,
		XS4Status:  status,
		ExternalID: e.AlertKey(),
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

// SendMessage sends a plain notice as an informational alert.
func (s *Signl4) SendMessage(ctx context.Context, msg string) error {
	return s.SendEvent(ctx, &event.Event{PodName: msg, Reason: "notify"})
}
