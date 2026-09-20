package twilio

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

const twilioAPIURL = "https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json"

type Twilio struct {
	sender     transport.Sender
	url        string
	accountSID string
	authToken  string
	from       string
	to         string

	clusterName string
}

// NewTwilio returns a new Twilio object

func NewTwilio(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Twilio {
	accountSID, ok := config["accountSid"].(string)
	if !ok || len(accountSID) == 0 {
		klog.InfoS("initializing twilio with empty accountSid")
		return nil
	}

	authToken, ok := config["authToken"].(string)
	if !ok || len(authToken) == 0 {
		klog.InfoS("initializing twilio with empty authToken")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing twilio with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing twilio with empty to")
		return nil
	}

	klog.InfoS("initializing twilio", "from", from, "to", to)

	return &Twilio{
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf(twilioAPIURL, accountSID),
		accountSID:  accountSID,
		authToken:   authToken,
		from:        from,
		to:          to,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (t *Twilio) Name() string {
	return "Twilio"
}

// SendEvent sends event to the provider
func (t *Twilio) SendEvent(ctx context.Context, e *event.Event) error {
	msg := e.FormatText(t.clusterName, "")
	return t.SendMessage(ctx, msg)
}

// SendMessage sends text message to the provider
func (t *Twilio) SendMessage(ctx context.Context, msg string) error {
	form := url.Values{}
	form.Set("From", t.from)
	form.Set("To", t.to)
	form.Set("Body", msg)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(t.accountSID+":"+t.authToken))

	_, err := t.sender.Send(ctx, transport.Request{
		Provider: t.Name(), URL: t.url, Body: []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded", Headers: map[string]string{
			"Authorization": auth,
		},
	})
	return err
}
