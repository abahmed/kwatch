package sendgrid

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const sendgridAPIURL = "https://api.sendgrid.com/v3/mail/send"

type sendgridEmail struct {
	Email string `json:"email"`
}

type sendgridContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sendgridPersonalization struct {
	To []sendgridEmail `json:"to"`
}

type sendgridPayload struct {
	Personalizations []sendgridPersonalization `json:"personalizations"`
	From             sendgridEmail             `json:"from"`
	Subject          string                    `json:"subject"`
	Content          []sendgridContent         `json:"content"`
}

type Sendgrid struct {
	sender  transport.Sender
	url     string
	apiKey  string
	from    string
	to      []string
	subject string

	clusterName string
}

// NewSendgrid returns a new Sendgrid object

func NewSendgrid(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Sendgrid {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing sendgrid with empty apiKey")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing sendgrid with empty from")
		return nil
	}

	to, ok := config["to"].([]interface{})
	if !ok || len(to) == 0 {
		klog.InfoS("initializing sendgrid with empty to")
		return nil
	}

	var recipients []string
	for _, t := range to {
		if s, ok := t.(string); ok && len(s) > 0 {
			recipients = append(recipients, s)
		}
	}
	if len(recipients) == 0 {
		klog.InfoS("initializing sendgrid with empty to")
		return nil
	}

	subject, _ := config["subject"].(string)

	klog.InfoS("initializing sendgrid", "from", from)

	return &Sendgrid{
		sender:      transport.NewSender(dependencies),
		url:         sendgridAPIURL,
		apiKey:      apiKey,
		from:        from,
		to:          recipients,
		subject:     subject,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Sendgrid) Name() string {
	return "Sendgrid"
}

// SendEvent sends event to the provider
func (s *Sendgrid) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Sendgrid) SendMessage(ctx context.Context, msg string) error {
	subject := s.subject
	if len(subject) == 0 {
		subject = "kwatch alert"
	}

	personalization := sendgridPersonalization{}
	for _, recipient := range s.to {
		personalization.To = append(personalization.To, sendgridEmail{Email: recipient})
	}

	payload := sendgridPayload{
		Personalizations: []sendgridPersonalization{personalization},
		From:             sendgridEmail{Email: s.from},
		Subject:          subject,
		Content: []sendgridContent{
			{Type: "text/plain", Value: msg},
		},
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
