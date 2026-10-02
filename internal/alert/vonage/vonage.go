package vonage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

const vonageAPIURL = "https://rest.nexmo.com/sms/json"

type Vonage struct {
	sender    transport.Sender
	url       string
	apiKey    string
	apiSecret string
	from      string
	to        string

	clusterName string
}

type response struct {
	Messages []struct {
		Status string `json:"status"`
		Error  string `json:"error-text"`
	} `json:"messages"`
}

// NewVonage returns a new Vonage object

func NewVonage(
	config map[string]interface{},
	clusterName string,
	dependencies transport.Dependencies,
) *Vonage {
	apiKey, ok := config["apiKey"].(string)
	if !ok || len(apiKey) == 0 {
		klog.InfoS("initializing vonage with empty apiKey")
		return nil
	}

	apiSecret, ok := config["apiSecret"].(string)
	if !ok || len(apiSecret) == 0 {
		klog.InfoS("initializing vonage with empty apiSecret")
		return nil
	}

	from, ok := config["from"].(string)
	if !ok || len(from) == 0 {
		klog.InfoS("initializing vonage with empty from")
		return nil
	}

	to, ok := config["to"].(string)
	if !ok || len(to) == 0 {
		klog.InfoS("initializing vonage with empty to")
		return nil
	}

	klog.InfoS("initializing vonage", "from", from, "to", to)

	return &Vonage{
		sender:      transport.NewSender(dependencies),
		url:         vonageAPIURL,
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		from:        from,
		to:          to,
		clusterName: clusterName,
	}
}

// Name returns name of the provider
func (v *Vonage) Name() string {
	return "Vonage"
}

// SendIncident sends the incident's one-line lead as the SMS body.
// The lead starts with the status marker, the only emoji in the payload.
func (v *Vonage) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	return v.SendMessage(ctx, m.ShortText())
}

// SendMessage sends text message to the provider
func (v *Vonage) SendMessage(ctx context.Context, msg string) error {
	form := url.Values{}
	form.Set("api_key", v.apiKey)
	form.Set("api_secret", v.apiSecret)
	form.Set("from", v.from)
	form.Set("to", v.to)
	form.Set("text", msg)

	body, err := v.sender.Send(ctx, transport.Request{
		Provider: v.Name(), URL: v.url, Body: []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
	})
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	var result response
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("vonage returned invalid response")
	}
	for _, message := range result.Messages {
		if message.Status != "0" {
			return fmt.Errorf("vonage message failed with status %s", message.Status)
		}
	}
	return nil
}
