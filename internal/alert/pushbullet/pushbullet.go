package pushbullet

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const pushbulletAPIURL = "https://api.pushbullet.com/v2/pushes"

type pushbulletPayload struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Pushbullet struct {
	sender      transport.Sender
	url         string
	accessToken string

	clusterName string
}

// NewPushbullet returns a new Pushbullet object

func NewPushbullet(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Pushbullet {
	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing pushbullet with empty accessToken")
		return nil
	}

	klog.InfoS("initializing pushbullet")

	return &Pushbullet{
		sender:      transport.NewSender(dependencies),
		url:         pushbulletAPIURL,
		accessToken: accessToken,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (s *Pushbullet) Name() string {
	return "Pushbullet"
}

// SendEvent sends event to the provider
func (s *Pushbullet) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(s.clusterName, "")
	return s.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (s *Pushbullet) SendMessage(ctx context.Context, msg string) error {
	title := "kwatch alert"
	if len(s.clusterName) > 0 {
		title = "kwatch alert: " + s.clusterName
	}

	payload := pushbulletPayload{
		Type:  "note",
		Title: title,
		Body:  msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = s.sender.Send(ctx, transport.Request{
		Provider: s.Name(), URL: s.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Access-Token": s.accessToken,
		},
	})
	return err
}
