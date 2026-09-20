package flock

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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

// SendEvent sends event to the provider
func (s *Flock) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
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
