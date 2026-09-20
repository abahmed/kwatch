package resend

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const resendAPIURL = "https://api.resend.com/emails"

type Resend struct {
	sender  transport.Sender
	url     string
	apiKey  string
	from    string
	to      []string
	subject string

	clusterName string
}

// NewResend returns a new Resend object

func NewResend(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Resend {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing resend with empty apiKey")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing resend with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing resend with empty to")
		return nil
	}

	var recipients []string
	for _, t := range strings.Split(to, ",") {
		if r := strings.TrimSpace(t); len(r) > 0 {
			recipients = append(recipients, r)
		}
	}
	if len(recipients) == 0 {
		klog.InfoS("initializing resend with empty to")
		return nil
	}

	subject, _ := config["subject"].(string)

	klog.InfoS("initializing resend", "from", from)

	return &Resend{
		sender:      transport.NewSender(dependencies),
		url:         resendAPIURL,
		apiKey:      apiKey,
		from:        from,
		to:          recipients,
		subject:     subject,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Resend) Name() string {
	return "Resend"
}

// SendEvent sends event to the provider
func (s *Resend) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Resend) SendMessage(ctx context.Context, msg string) error {
	subject := s.subject
	if len(subject) == 0 {
		subject = "kwatch alert"
	}

	payload := map[string]interface{}{
		"from":    s.from,
		"to":      s.to,
		"subject": subject,
		"text":    msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + s.apiKey,
		},
	})
	return err
}
