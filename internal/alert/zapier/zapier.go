package zapier

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

type zapierPayload struct {
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Text    string `json:"text,omitempty"`
}

type Zapier struct {
	sender transport.Sender
	url    string
	title  string

	clusterName string
}

// NewZapier returns a new Zapier object

func NewZapier(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Zapier {
	url, ok := config["url"].(string)
	if !ok || len(url) == 0 {
		klog.InfoS("initializing zapier with empty url")
		return nil
	}

	title, _ := config["title"].(string)

	klog.InfoS("initializing zapier", "title", title)

	return &Zapier{
		sender:      transport.NewSender(dependencies),
		url:         url,
		title:       title,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (z *Zapier) Name() string {
	return "Zapier"
}

// SendEvent sends event to the provider
func (z *Zapier) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(z.clusterName, "")
	return z.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (z *Zapier) SendMessage(ctx context.Context, msg string) error {
	payload := zapierPayload{
		Title:   z.title,
		Message: msg,
		Text:    msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = z.sender.Send(ctx, transport.Request{
		Provider: z.Name(), URL: z.url, Body: body,
		ContentType: "application/json",
	})
	return err
}
