package threema

import (
	"context"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const threemaAPIURL = "https://gateway.threema.ch/push_simple"

type Threema struct {
	sender    transport.Sender
	url       string
	gatewayID string
	secret    string
	to        string

	clusterName string
}

// NewThreema returns a new Threema object

func NewThreema(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Threema {
	gatewayID, ok := config["gatewayId"].(string)
	if !ok || len(gatewayID) == 0 {
		klog.InfoS("initializing threema with empty gatewayId")
		return nil
	}

	secret, ok := config["secret"].(string)
	if !ok || len(secret) == 0 {
		klog.InfoS("initializing threema with empty secret")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing threema with empty to")
		return nil
	}

	klog.InfoS("initializing threema", "gatewayId", gatewayID)

	return &Threema{
		sender:      transport.NewSender(dependencies),
		url:         threemaAPIURL,
		gatewayID:   gatewayID,
		secret:      secret,
		to:          to,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Threema) Name() string {
	return "Threema"
}

// SendEvent sends event to the provider
func (s *Threema) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Threema) SendMessage(ctx context.Context, msg string) error {
	form := url.Values{}
	form.Set("from", s.gatewayID)
	form.Set("to", s.to)
	form.Set("secret", s.secret)
	form.Set("text", msg)

	body := []byte(form.Encode())
	_, err := s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/x-www-form-urlencoded",
	})
	return err
}
