package webex

import (
	"context"
	"encoding/json"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const webexAPIURL = "https://webexapis.com/v1/messages"

// webexMarkdownLimit is Webex's documented cap on a message's text or
// markdown, 7439 bytes.
const webexMarkdownLimit = 7439

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

// SendIncident posts the incident narrative as Markdown, followed by the
// workload's last output as a code block when there is one.
func (w *Webex) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	d := notification.MarkdownDialect(
		notification.NeutralizeWebexMentions, "\n\n")
	return w.SendMessage(ctx,
		safetext.RichWithOutput(m, d, "\n\n", webexMarkdownLimit))
}

// SendMessage sends text message to the provider
func (w *Webex) SendMessage(ctx context.Context, msg string) error {
	payload := webexPayload{
		RoomID:        w.roomID,
		ToPersonEmail: w.toPersonEmail,
		Markdown:      notification.Truncate(msg, webexMarkdownLimit),
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
