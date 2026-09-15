package webex

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const webexAPIURL = "https://webexapis.com/v1/messages"

type webexPayload struct {
	RoomID        string `json:"roomId,omitempty"`
	ToPersonEmail string `json:"toPersonEmail,omitempty"`
	Markdown      string `json:"markdown,omitempty"`
	Text          string `json:"text,omitempty"`
}

type Webex struct {
	sender        transport.Sender
	url           string
	accessToken   string
	roomID        string
	toPersonEmail string

	clusterName string
}

// NewWebex returns a new Webex object

func NewWebex(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Webex {
	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing webex with empty accessToken")
		return nil
	}

	roomID, _ := config["roomId"].(string)
	toPersonEmail, _ := config["toPersonEmail"].(string)
	if len(roomID) == 0 && len(toPersonEmail) == 0 {
		klog.InfoS("initializing webex with empty roomId and toPersonEmail")
		return nil
	}

	klog.InfoS("initializing webex", "roomId", roomID, "toPersonEmail", toPersonEmail)

	return &Webex{
		sender:        transport.NewSender(dependencies),
		url:           webexAPIURL,
		accessToken:   accessToken,
		roomID:        roomID,
		toPersonEmail: toPersonEmail,
		clusterName:   clusterName,
	}
}

// Name returns name of the provider
func (w *Webex) Name() string {
	return "Webex"
}

// SendEvent sends event to the provider
func (w *Webex) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatMarkdown(w.clusterName, "", "\n\n")
	return w.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (w *Webex) SendMessage(ctx context.Context, msg string) error {
	payload := webexPayload{
		RoomID:        w.roomID,
		ToPersonEmail: w.toPersonEmail,
		Markdown:      msg,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = w.sender.Send(ctx, transport.Request{
		Provider: w.Name(), URL: w.url, Body: body,
		ContentType: "application/json", Headers: map[string]string{
			"Authorization": "Bearer " + w.accessToken,
		},
	})
	return err
}
