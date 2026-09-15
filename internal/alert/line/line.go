package line

import (
	"context"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const lineAPIURL = "https://notify-api.line.me/api/notify"

type Line struct {
	sender transport.Sender
	url    string
	token  string

	clusterName string
}

// NewLine returns a new Line object

func NewLine(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Line {
	token, ok := config["token"].(string)
	if !ok || len(token) == 0 {
		klog.InfoS("initializing line with empty token")
		return nil
	}

	klog.InfoS("initializing line")

	return &Line{
		sender:      transport.NewSender(dependencies),
		url:         lineAPIURL,
		token:       token,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (l *Line) Name() string {
	return "Line"
}

// SendEvent sends event to the provider
func (l *Line) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(l.clusterName, "")
	return l.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (l *Line) SendMessage(ctx context.Context, msg string) error {
	form := url.Values{}
	form.Set("message", msg)

	_, err := l.sender.Send(ctx, transport.Request{
		Provider: l.Name(), URL: l.url, Body: []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded", Headers: map[string]string{
			"Authorization": "Bearer " + l.token,
		},
	})
	return err
}
