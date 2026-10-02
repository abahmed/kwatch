package line

import (
	"context"
	"net/url"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const lineAPIURL = "https://notify-api.line.me/api/notify"

// lineTextLimit is LINE Notify's message limit.
const lineTextLimit = 1000

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

// SendIncident sends the incident narrative as plain text, followed by
// the application output as a quoted block.
func (l *Line) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	text := notification.Truncate(plainNote(m), lineTextLimit)
	return l.SendMessage(ctx, text)
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

// plainNote is the Note with the application output quoted after it.
func plainNote(m notification.Message) string {
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n\n> " + strings.Join(m.Output, "\n> ")
	}
	return text
}
