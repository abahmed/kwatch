package signal

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const defaultSignalURL = "http://localhost:8080"

type signalPayload struct {
	Message    string   `json:"message"`
	Number     string   `json:"number,omitempty"`
	Recipients []string `json:"recipients"`
}

type Signal struct {
	sender transport.Sender
	url    string
	number string
	to     string

	clusterName string
}

// NewSignal returns a new Signal object

func NewSignal(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Signal {
	number, ok := config["number"].(string)
	if !ok || len(number) == 0 {
		klog.InfoS("initializing signal with empty number")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing signal with empty to")
		return nil
	}

	server := defaultSignalURL
	if s, ok := config["url"].(string); ok && len(s) > 0 {
		server = s
	}

	klog.InfoS("initializing signal", "url", server, "number", number)

	return &Signal{
		sender:      transport.NewSender(dependencies),
		url:         strings.TrimRight(server, "/") + "/v2/send",
		number:      number,
		to:          to,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Signal) Name() string {
	return "Signal"
}

// SendEvent sends event to the provider
func (s *Signal) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Signal) SendMessage(ctx context.Context, msg string) error {
	payload := signalPayload{
		Message:    msg,
		Number:     s.number,
		Recipients: []string{s.to},
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
