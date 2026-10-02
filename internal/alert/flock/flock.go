package flock

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

type flockPayload struct {
	Text string `json:"text"`
}

type Flock struct {
	sender  transport.Sender
	webhook string

	clusterName string
}

// NewFlock returns a new Flock object

func NewFlock(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Flock {
	webhook, ok := config["webhook"].(string)
	if !ok || len(webhook) == 0 {
		klog.InfoS("initializing flock with empty webhook")
		return nil
	}

	if !transport.ValidEndpoint(webhook) {
		klog.InfoS("initializing flock with an invalid webhook",
			"setting", "webhook")
		return nil
	}

	klog.InfoS("initializing flock with webhook configured")

	return &Flock{
		sender:      transport.NewSender(dependencies),
		webhook:     webhook,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Flock) Name() string {
	return "Flock"
}

// SendIncident sends the incident narrative as plain text, followed by
// the application output as a quoted block.
func (s *Flock) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return s.SendMessage(ctx, plainNote(m))
}

// SendMessage sends text message to the provider
func (s *Flock) SendMessage(ctx context.Context, msg string) error {
	payload := flockPayload{
		Text: msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.webhook, Body: body,
		ContentType: "application/json",
	})
	return err
}

// plainNote is the Note with the application output quoted after it.
func plainNote(m notification.Message) string {
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n\n> " + strings.Join(m.Output, "\n> ")
	}
	return text
}
