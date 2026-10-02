package sendgrid

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
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
}

// NewSendgrid returns a new Sendgrid object

func NewSendgrid(
	config map[string]interface{},
	_ string,
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

	recipients := parseRecipients(config["to"])
	if len(recipients) == 0 {
		klog.InfoS("initializing sendgrid with empty to")
		return nil
	}

	subject, _ := config["subject"].(string)

	klog.InfoS("initializing sendgrid", "from", from)

	return &Sendgrid{
		sender:  transport.NewSender(dependencies),
		url:     sendgridAPIURL,
		apiKey:  apiKey,
		from:    from,
		to:      recipients,
		subject: subject,
	}
}

// Name returns name of the provider
func (s *Sendgrid) Name() string {
	return "Sendgrid"
}

// SendIncident mails one incident message: the Short lead is the subject
// and the narrative Note, plus any recent output, is the body.
func (s *Sendgrid) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return s.send(ctx, m.MailSubject(), m.MailBody())
}

// SendMessage mails a plain operator message under the configured subject.
func (s *Sendgrid) SendMessage(ctx context.Context, msg string) error {
	subject := s.subject
	if len(subject) == 0 {
		subject = "kwatch alert"
	}
	return s.send(ctx, subject, msg)
}

func (s *Sendgrid) send(ctx context.Context, subject, msg string) error {
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

// parseRecipients accepts a YAML list or, like the other email providers, a
// comma-separated string.
func parseRecipients(value interface{}) []string {
	var raw []string
	switch to := value.(type) {
	case string:
		raw = strings.Split(to, ",")
	case []interface{}:
		for _, item := range to {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	recipients := make([]string, 0, len(raw))
	for _, r := range raw {
		if r = strings.TrimSpace(r); r != "" {
			recipients = append(recipients, r)
		}
	}
	return recipients
}
