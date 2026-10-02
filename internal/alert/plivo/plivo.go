package plivo

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const plivoAPIURL = "https://api.plivo.com/v1/Account/%s/Message/"

type Plivo struct {
	sender    transport.Sender
	url       string
	authID    string
	authToken string
	from      string
	to        string

	clusterName string
}

// NewPlivo returns a new Plivo object

func NewPlivo(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Plivo {
	authID, ok := config["authId"].(string)
	if !ok || len(authID) == 0 {
		klog.InfoS("initializing plivo with empty authId")
		return nil
	}

	authToken, ok := config["authToken"].(string)
	if !ok || len(authToken) == 0 {
		klog.InfoS("initializing plivo with empty authToken")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing plivo with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing plivo with empty to")
		return nil
	}

	klog.InfoS("initializing plivo", "from", from, "to", to)

	return &Plivo{
		sender:      transport.NewSender(dependencies),
		url:         fmt.Sprintf(plivoAPIURL, authID),
		authID:      authID,
		authToken:   authToken,
		from:        from,
		to:          to,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (p *Plivo) Name() string {
	return "Plivo"
}

// SendIncident sends the incident's one-line lead as the SMS body.
// The lead starts with the status marker, the only emoji in the payload.
func (p *Plivo) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return p.SendMessage(ctx, m.ShortText())
}

// SendMessage sends text message to the provider
func (p *Plivo) SendMessage(ctx context.Context, msg string) error {
	form := url.Values{}
	form.Set("src", p.from)
	form.Set("dst", p.to)
	form.Set("text", msg)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(p.authID+":"+p.authToken))

	_, err := p.sender.Send(ctx, transport.Request{
		Provider: p.Name(), URL: p.url, Body: []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded", Headers: map[string]string{
			"Authorization": auth,
		},
	})
	return err
}
